package api

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"mediaproxy/internal/auth"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// withTimeout bounds a handler's request context to d. We replace c.Request
// with one carrying the deadline, so every c.Request.Context() inside the
// handler (and every pgx call that uses it) inherits the timeout — a slow
// query or row-lock wait returns context.DeadlineExceeded and FREES the
// pooled DB connection instead of pinning a goroutine+connection. This is the
// deterministic shed valve (F3): under contention baseapp fails fast (Kamailio
// then 503s the call cleanly) rather than letting the pool drain and the
// process go live-but-frozen. The bound is independent of libcurl/Kamailio
// connection-close timing, which we cannot rely on.
func withTimeout(d time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), d)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

func requestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		slog.Info("http",
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"dur_ms", time.Since(start).Milliseconds(),
			"ip", c.ClientIP(),
		)
	}
}

// auditMiddleware records every successful state-changing request
// (POST/PATCH/DELETE) into audit_log. Reads (GET) are not recorded.
func auditMiddleware(pg *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		if c.Request.Method == http.MethodGet {
			return
		}
		if c.Writer.Status() >= 400 {
			return
		}
		actor, _ := c.Get("user_id")
		_, _ = pg.Exec(c.Request.Context(), `
			INSERT INTO audit_log (actor_id, action, target, ip)
			VALUES ($1, $2, $3, $4::inet)
		`, actor, c.Request.Method, c.Request.URL.Path, c.ClientIP())
	}
}

func requireAuth(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.GetHeader("Authorization")
		if !strings.HasPrefix(h, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing bearer token"})
			return
		}
		raw := strings.TrimPrefix(h, "Bearer ")
		claims, err := auth.VerifyJWT(raw, secret)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}
		c.Set("user_id", claims.UserID)
		c.Set("role", claims.Role)
		c.Next()
	}
}
