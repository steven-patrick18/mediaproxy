package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

type NodeCommand struct {
	ID          int64           `json:"id"`
	NodeID      int64           `json:"node_id"`
	Type        string          `json:"type"`
	Payload     json.RawMessage `json:"payload,omitempty"`
	Status      string          `json:"status"`
	Detail      *string         `json:"detail,omitempty"`
	CreatedBy   *int64          `json:"created_by,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
	SentAt      *time.Time      `json:"sent_at,omitempty"`
	CompletedAt *time.Time      `json:"completed_at,omitempty"`
}

var validCommandTypes = map[string]bool{
	"apply":             true, // force reconcile expected IPs now
	"apply_firewall":    true, // fetch + apply nftables ruleset with safety rollback
	"reboot":            true, // exec `reboot` on the host
	"restart_rtpengine": true,
	"restart_kamailio":  true,
}

type createCommandRequest struct {
	Type    string         `json:"type" binding:"required"`
	Payload map[string]any `json:"payload"`
}

// POST /api/v1/nodes/:id/commands
func (s *Server) createNodeCommand(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad id"})
		return
	}
	var req createCommandRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !validCommandTypes[req.Type] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid command type"})
		return
	}
	payload := []byte("{}")
	if req.Payload != nil {
		payload, _ = json.Marshal(req.Payload)
	}
	actor, _ := c.Get("user_id")
	var cmd NodeCommand
	err = s.deps.PG.QueryRow(c.Request.Context(), `
		INSERT INTO node_commands (node_id, type, payload, created_by)
		VALUES ($1, $2, $3, $4)
		RETURNING id, node_id, type, payload, status, detail, created_by, created_at, sent_at, completed_at
	`, id, req.Type, payload, actor).Scan(
		&cmd.ID, &cmd.NodeID, &cmd.Type, &cmd.Payload, &cmd.Status,
		&cmd.Detail, &cmd.CreatedBy, &cmd.CreatedAt, &cmd.SentAt, &cmd.CompletedAt,
	)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, cmd)
}

// GET /api/v1/nodes/:id/commands
func (s *Server) listNodeCommands(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad id"})
		return
	}
	rows, err := s.deps.PG.Query(c.Request.Context(), `
		SELECT id, node_id, type, payload, status, detail,
		       created_by, created_at, sent_at, completed_at
		  FROM node_commands WHERE node_id = $1 ORDER BY id DESC LIMIT 50
	`, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	out := []NodeCommand{}
	for rows.Next() {
		var x NodeCommand
		if err := rows.Scan(&x.ID, &x.NodeID, &x.Type, &x.Payload, &x.Status,
			&x.Detail, &x.CreatedBy, &x.CreatedAt, &x.SentAt, &x.CompletedAt); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		out = append(out, x)
	}
	c.JSON(http.StatusOK, out)
}
