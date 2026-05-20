package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

type AuditEntry struct {
	ID       int64           `json:"id"`
	ActorID  *int64          `json:"actor_id,omitempty"`
	Action   string          `json:"action"`
	Target   *string         `json:"target,omitempty"`
	Before   json.RawMessage `json:"before,omitempty"`
	After    json.RawMessage `json:"after,omitempty"`
	IP       *string         `json:"ip,omitempty"`
	Ts       time.Time       `json:"ts"`
}

func (s *Server) listAudit(c *gin.Context) {
	limit := 100
	if q := c.Query("limit"); q != "" {
		if n, err := strconv.Atoi(q); err == nil && n > 0 && n <= 1000 {
			limit = n
		}
	}
	rows, err := s.deps.PG.Query(c.Request.Context(), `
		SELECT id, actor_id, action, target, before, after, host(ip), ts
		  FROM audit_log ORDER BY ts DESC LIMIT $1
	`, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	out := []AuditEntry{}
	for rows.Next() {
		var e AuditEntry
		if err := rows.Scan(&e.ID, &e.ActorID, &e.Action, &e.Target, &e.Before, &e.After, &e.IP, &e.Ts); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		out = append(out, e)
	}
	c.JSON(http.StatusOK, out)
}
