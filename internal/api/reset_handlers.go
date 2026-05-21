package api

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
)

// resetRequest is the body for POST /api/v1/admin/reset. Operator must
// type the literal string "RESET" into Confirm to make the request
// actually do anything — guards against accidental destructive clicks
// reaching the endpoint from a misbehaving client or a confused operator.
type resetRequest struct {
	Scopes  []string `json:"scopes"`  // any subset of "active_calls", "cdrs", "metrics"
	Confirm string   `json:"confirm"` // must equal "RESET"
}

// POST /api/v1/admin/reset — destructive data wipe of operational tables.
// What this DOES touch:
//   - active_calls (clear in-flight rows, reset current_calls counter)
//   - call_records (CDR history)
//   - node_metrics (CPU/RAM/net heartbeat history)
//
// What this DOES NOT touch (by design — preserves configuration):
//   - clients, carriers, media_nodes, node_ips, signaling_ips
//   - assignments, ip_groups, ip_group_members
//   - admin_users, agent tokens
//   - firewall_rules
//
// Each scope is processed independently; a failure in one doesn't roll
// back the others. The response reports per-scope row counts so the UI
// can show "deleted 832 CDRs, reset 62 IP counters".
func (s *Server) adminReset(c *gin.Context) {
	var req resetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Confirm != "RESET" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "confirmation phrase must be 'RESET' (case-sensitive)"})
		return
	}
	if len(req.Scopes) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no scopes selected"})
		return
	}
	want := map[string]bool{}
	for _, s := range req.Scopes {
		switch s {
		case "active_calls", "cdrs", "metrics":
			want[s] = true
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": "unknown scope: " + s})
			return
		}
	}

	actor, _ := c.Get("user_id")
	ctx := c.Request.Context()
	result := map[string]int64{}

	if want["active_calls"] {
		// Delete then resync counters in case any non-zero remain (shouldn't,
		// but the GREATEST(...,0) guard in call-end means a counter could be
		// stuck if a row was lost.
		t1, err := s.deps.PG.Exec(ctx, `DELETE FROM active_calls`)
		if err != nil {
			slog.Error("admin reset: active_calls failed", "err", err, "actor", actor)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "active_calls delete: " + err.Error()})
			return
		}
		result["active_calls_deleted"] = t1.RowsAffected()
		t2, err := s.deps.PG.Exec(ctx, `UPDATE node_ips SET current_calls = 0 WHERE current_calls <> 0`)
		if err != nil {
			slog.Error("admin reset: current_calls reset failed", "err", err, "actor", actor)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "current_calls reset: " + err.Error()})
			return
		}
		result["current_calls_reset"] = t2.RowsAffected()
	}

	if want["cdrs"] {
		t, err := s.deps.PG.Exec(ctx, `DELETE FROM call_records`)
		if err != nil {
			slog.Error("admin reset: cdrs failed", "err", err, "actor", actor)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "call_records delete: " + err.Error()})
			return
		}
		result["cdrs_deleted"] = t.RowsAffected()
	}

	if want["metrics"] {
		t, err := s.deps.PG.Exec(ctx, `DELETE FROM node_metrics`)
		if err != nil {
			slog.Error("admin reset: metrics failed", "err", err, "actor", actor)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "node_metrics delete: " + err.Error()})
			return
		}
		result["metrics_deleted"] = t.RowsAffected()
	}

	slog.Warn("admin reset performed",
		"actor", actor, "scopes", req.Scopes, "result", result)
	c.JSON(http.StatusOK, gin.H{"ok": true, "result": result})
}
