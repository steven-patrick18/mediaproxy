// Package sigcache writes the per-dialer-IP signaling-IP lookup table into
// Redis so the SIP proxy (Kamailio + ndb_redis) can resolve it without
// hitting Postgres.
//
// Key layout (per spec):
//
//	SET sig:<dialer_ip> '{"client_id":N,"signaling_ip":"203.0.113.10"}'
package sigcache

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type Writer struct {
	PG    *pgxpool.Pool
	Redis *redis.Client
}

type entry struct {
	ClientID    int64  `json:"client_id"`
	SignalingIP string `json:"signaling_ip"`
}

// SyncClient rewrites all sig:<dialer_ip> keys for the given client.
// Call after: client signaling-IP assignment changes, dialer-IP add/remove,
// or client suspension.
func (w *Writer) SyncClient(ctx context.Context, clientID int64) error {
	// Resolve client status + signaling IP
	var (
		status      string
		signalingIP *string
	)
	err := w.PG.QueryRow(ctx, `
		SELECT c.status, host(s.ip_address)
		  FROM clients c
		  LEFT JOIN signaling_ips s ON s.id = c.signaling_ip_id
		 WHERE c.id = $1
	`, clientID).Scan(&status, &signalingIP)
	if err != nil {
		return fmt.Errorf("load client: %w", err)
	}

	// Gather all active dialer IPs for this client (current state in DB)
	rows, err := w.PG.Query(ctx, `
		SELECT host(ip_address) FROM client_ips
		 WHERE client_id = $1 AND status = 'active'
	`, clientID)
	if err != nil {
		return fmt.Errorf("load dialer ips: %w", err)
	}
	defer rows.Close()
	dialers := []string{}
	for rows.Next() {
		var ip string
		if err := rows.Scan(&ip); err != nil {
			return err
		}
		dialers = append(dialers, ip)
	}

	// Find any orphan dialer IPs currently keyed in Redis for this client
	// (so we can clean up if a dialer IP was removed).
	indexKey := fmt.Sprintf("client_dialers:%d", clientID)
	prev, _ := w.Redis.SMembers(ctx, indexKey).Result()
	prevSet := map[string]struct{}{}
	for _, p := range prev {
		prevSet[p] = struct{}{}
	}

	pipe := w.Redis.Pipeline()

	// Delete keys for dialer IPs that are no longer present, suspended, or unassigned.
	toRemove := map[string]struct{}{}
	for p := range prevSet {
		toRemove[p] = struct{}{}
	}

	if status == "active" && signalingIP != nil {
		payload, err := json.Marshal(entry{ClientID: clientID, SignalingIP: *signalingIP})
		if err != nil {
			return err
		}
		for _, d := range dialers {
			pipe.Set(ctx, "sig:"+d, payload, 0)
			delete(toRemove, d) // keep this one
		}
		// rebuild the dialer index for this client
		pipe.Del(ctx, indexKey)
		if len(dialers) > 0 {
			args := make([]any, 0, len(dialers))
			for _, d := range dialers {
				args = append(args, d)
			}
			pipe.SAdd(ctx, indexKey, args...)
		}
	} else {
		// inactive or unassigned: remove all keys for this client
		for _, d := range dialers {
			toRemove[d] = struct{}{}
		}
		pipe.Del(ctx, indexKey)
	}

	for d := range toRemove {
		pipe.Del(ctx, "sig:"+d)
	}

	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("redis pipeline: %w", err)
	}
	return nil
}
