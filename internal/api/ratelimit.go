package api

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// loginRateLimit guards POST /api/v1/auth/login against credential brute-force
// using a Redis-backed sliding-window counter keyed by client IP.
//
// Policy: at most 5 unsuccessful attempts per IP per 15-minute window.
// We count only *attempts* — every request increments the counter regardless
// of outcome — which is the conservative choice: an attacker who guesses a
// valid password on attempt #1 also locks themselves out for 15 minutes after
// 4 more invalid tries on other accounts from the same IP.
//
// On overage we return 429 with Retry-After (in seconds) so well-behaved
// clients back off. The window is sliding because the Redis key carries the
// remaining TTL, so the user is only locked out as long as the most recent
// burst is "fresh".
func loginRateLimit(rdb *redis.Client) gin.HandlerFunc {
	const (
		maxAttempts = 5
		window      = 15 * time.Minute
	)
	return func(c *gin.Context) {
		ip := clientIP(c)
		key := "auth:login:fails:" + ip
		ctx, cancel := context.WithTimeout(c.Request.Context(), 500*time.Millisecond)
		defer cancel()

		// INCR + EXPIRE atomically: first hit sets TTL, subsequent hits keep
		// the same window. Redis returns the post-increment value.
		count, err := rdb.Incr(ctx, key).Result()
		if err != nil {
			// Fail-open on Redis outage. We log via the standard request log;
			// blocking logins on Redis hiccups would be worse than carrying
			// on without rate limiting for that one request.
			c.Next()
			return
		}
		if count == 1 {
			// First request in the window — set TTL. If this fails we don't
			// care; next request will INCR a still-existing key without TTL
			// and we'd never reset, so worst case is one extra long lockout.
			_, _ = rdb.Expire(ctx, key, window).Result()
		}
		if count > maxAttempts {
			ttl, _ := rdb.TTL(ctx, key).Result()
			retryAfter := int(ttl.Seconds())
			if retryAfter < 1 {
				retryAfter = int(window.Seconds())
			}
			c.Header("Retry-After", strconv.Itoa(retryAfter))
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error":       "too many login attempts; try again later",
				"retry_after": retryAfter,
			})
			return
		}
		c.Next()
	}
}

// clientIP honours X-Forwarded-For from nginx but only the last hop (nginx is
// the trusted reverse proxy in front of the base-app). Falls back to the
// direct peer.
func clientIP(c *gin.Context) string {
	if xff := c.GetHeader("X-Forwarded-For"); xff != "" {
		// Trust only the rightmost entry — that's the IP nginx saw. Earlier
		// entries are client-controllable and would let attackers rotate
		// their effective key by spoofing the header.
		parts := strings.Split(xff, ",")
		ip := strings.TrimSpace(parts[len(parts)-1])
		if ip != "" {
			return ip
		}
	}
	return c.ClientIP()
}
