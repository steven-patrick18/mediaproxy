package api

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// StartAlertWorker watches for conditions worth alerting on and fires
// webhook events. Currently:
//   - node went offline (last_seen_at > 2 min ago AND status != 'offline')
//     → emit "node.offline"
//   - node back online (was tracked as offline, now status='online')
//     → emit "node.online"
//
// State is tracked in alert_state so we don't spam (one alert per
// (key) until resolved).
func (s *Server) StartAlertWorker(ctx context.Context) {
	go func() {
		t := time.NewTicker(30 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				s.runAlertChecks(ctx)
			}
		}
	}()
}

func (s *Server) runAlertChecks(ctx context.Context) {
	// Find nodes that look offline but haven't had an alert fired yet.
	rows, err := s.deps.PG.Query(ctx, `
		SELECT n.id, n.name, n.role
		  FROM media_nodes n
		 WHERE (n.last_seen_at IS NULL OR n.last_seen_at < now() - interval '2 minutes')
		   AND NOT EXISTS (
		     SELECT 1 FROM alert_state s
		      WHERE s.key = 'node.offline:' || n.id::text
		        AND s.resolved_at IS NULL
		   )
	`)
	if err != nil {
		return
	}
	type offlineNode struct {
		id   int64
		name string
		role string
	}
	off := []offlineNode{}
	for rows.Next() {
		var n offlineNode
		if err := rows.Scan(&n.id, &n.name, &n.role); err == nil {
			off = append(off, n)
		}
	}
	rows.Close()
	for _, n := range off {
		key := fmt.Sprintf("node.offline:%d", n.id)
		// Insert state and emit webhook event.
		if _, err := s.deps.PG.Exec(ctx,
			`INSERT INTO alert_state (key) VALUES ($1) ON CONFLICT (key) DO NOTHING`, key); err != nil {
			continue
		}
		slog.Warn("node went offline; firing webhook", "node_id", n.id, "name", n.name)
		s.QueueWebhookEvent(ctx, "node.offline", map[string]any{
			"event":      "node.offline",
			"node_id":    n.id,
			"node_name":  n.name,
			"role":       n.role,
			"timestamp":  time.Now().UTC().Format(time.RFC3339),
			"message":    fmt.Sprintf("Node %s has not heartbeated in 2+ minutes", n.name),
		})
	}

	// Resolve: nodes that are back online but still have an open alert.
	resolveRows, err := s.deps.PG.Query(ctx, `
		SELECT s.id, s.key, n.id AS node_id, n.name, n.role
		  FROM alert_state s
		  JOIN media_nodes n ON s.key = 'node.offline:' || n.id::text
		 WHERE s.resolved_at IS NULL
		   AND n.last_seen_at >= now() - interval '2 minutes'
	`)
	if err != nil {
		return
	}
	type back struct {
		stateID int64
		key     string
		nodeID  int64
		name    string
		role    string
	}
	backs := []back{}
	for resolveRows.Next() {
		var x back
		if err := resolveRows.Scan(&x.stateID, &x.key, &x.nodeID, &x.name, &x.role); err == nil {
			backs = append(backs, x)
		}
	}
	resolveRows.Close()
	for _, x := range backs {
		_, _ = s.deps.PG.Exec(ctx,
			`UPDATE alert_state SET resolved_at = now() WHERE id = $1`, x.stateID)
		slog.Info("node back online; firing webhook", "node_id", x.nodeID)
		s.QueueWebhookEvent(ctx, "node.online", map[string]any{
			"event":     "node.online",
			"node_id":   x.nodeID,
			"node_name": x.name,
			"role":      x.role,
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		})
	}
}
