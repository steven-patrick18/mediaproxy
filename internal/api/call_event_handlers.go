package api

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// Call events come from Kamailio's `acc` module via http_post or from the
// agent forwarding them. All three of these endpoints take a node-scoped
// agent token (so the agent is the legitimate poster); admins also can
// reach them via the JWT-authed group.

type callStartReq struct {
	CallID        string `json:"call_id" binding:"required"`
	ClientID      *int64 `json:"client_id"`
	CarrierID     *int64 `json:"carrier_id"`
	MediaIP       string `json:"media_ip"`
	SignalingFrom string `json:"signaling_from"`
	ANI           string `json:"ani"`
	DNIS          string `json:"dnis"`
}

// POST /api/v1/agent/call-start — Kamailio fires this on 200 OK.
func (s *Server) callStart(c *gin.Context) {
	nodeID := c.GetInt64("agent_node_id")
	var req callStartReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	_, err := s.deps.PG.Exec(c.Request.Context(), `
		INSERT INTO active_calls (call_id, client_id, carrier_id, node_id, media_ip, signaling_from, ani, dnis, started_at)
		VALUES ($1, $2, $3, $4, NULLIF($5,'')::inet, NULLIF($6,'')::inet, NULLIF($7,''), NULLIF($8,''), now())
		ON CONFLICT (call_id) DO UPDATE SET last_seen_at = now()
	`, req.CallID, req.ClientID, req.CarrierID, nodeID, req.MediaIP, req.SignalingFrom, req.ANI, req.DNIS)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.MediaIP != "" {
		if _, err := s.deps.PG.Exec(c.Request.Context(),
			`UPDATE node_ips SET current_calls = current_calls + 1 WHERE ip_address = $1::inet`, req.MediaIP); err != nil {
			// Counter drift will bypass max_calls cap until next reconcile —
			// loud log so ops can spot DB hiccups before they distort routing.
			slog.Error("call-start: bump node_ips.current_calls failed",
				"err", err, "call_id", req.CallID, "media_ip", req.MediaIP, "node_id", nodeID)
		}
	}
	c.Status(http.StatusNoContent)
}

type callEndReq struct {
	CallID      string `json:"call_id" binding:"required"`
	Disposition string `json:"disposition"`
	SipCode     int    `json:"sip_code"`
	DurationSec int    `json:"duration_sec"`
}

// POST /api/v1/agent/call-end — Kamailio fires this on BYE / final
// non-2xx. Moves the row from active_calls into call_records.
func (s *Server) callEnd(c *gin.Context) {
	nodeID := c.GetInt64("agent_node_id")
	var req callEndReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	dispo := req.Disposition
	if dispo == "" {
		switch {
		case req.SipCode == 200:
			dispo = "answered"
		case req.SipCode == 486:
			dispo = "busy"
		case req.SipCode == 487:
			dispo = "canceled"
		case req.SipCode >= 400 && req.SipCode < 500:
			dispo = "failed"
		case req.SipCode >= 500:
			dispo = "failed"
		default:
			dispo = "no_answer"
		}
	}

	// Pull the active row, write CDR, delete.
	row := s.deps.PG.QueryRow(c.Request.Context(), `
		DELETE FROM active_calls WHERE call_id = $1 AND node_id = $2
		 RETURNING client_id, carrier_id, media_ip, signaling_from, ani, dnis, started_at
	`, req.CallID, nodeID)
	var (
		clientID, carrierID *int64
		mediaIP, sigFrom    *string
		ani, dnis           *string
		startedAt           time.Time
	)
	if err := row.Scan(&clientID, &carrierID, &mediaIP, &sigFrom, &ani, &dnis, &startedAt); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "active call not found"})
		return
	}

	dur := req.DurationSec
	if dur == 0 && !startedAt.IsZero() {
		dur = int(time.Since(startedAt).Seconds())
	}

	var sipCode *int
	if req.SipCode != 0 {
		sipCode = &req.SipCode
	}
	_, err := s.deps.PG.Exec(c.Request.Context(), `
		INSERT INTO call_records
		    (call_id, client_id, carrier_id, node_id, media_ip, signaling_from,
		     ani, dnis, started_at, ended_at, duration_sec, disposition, sip_code)
		VALUES ($1, $2, $3, $4, $5::inet, $6::inet, $7, $8, $9, now(), $10, $11, $12)
	`, req.CallID, clientID, carrierID, nodeID, mediaIP, sigFrom, ani, dnis,
		startedAt, dur, dispo, sipCode)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if mediaIP != nil && *mediaIP != "" {
		if _, err := s.deps.PG.Exec(c.Request.Context(),
			`UPDATE node_ips SET current_calls = GREATEST(current_calls - 1, 0) WHERE host(ip_address) = $1`, *mediaIP); err != nil {
			slog.Error("call-end: decrement node_ips.current_calls failed",
				"err", err, "call_id", req.CallID, "media_ip", *mediaIP, "node_id", nodeID)
		}
	}
	c.Status(http.StatusNoContent)
}
