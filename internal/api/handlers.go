package api

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// serveAgentBinary streams the node-agent linux/amd64 binary. The binary
// is just a Go program; it's harmless without a matching agent_token, so
// this endpoint doesn't require auth.
func (s *Server) serveAgentBinary(c *gin.Context) {
	const path = "/opt/mediaproxy/bin/node-agent"
	c.Header("Content-Disposition", `attachment; filename="node-agent"`)
	c.Header("Content-Type", "application/octet-stream")
	c.File(path)
}

func (s *Server) healthz(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (s *Server) readyz(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	if err := s.deps.PG.Ping(ctx); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "db_down", "err": err.Error()})
		return
	}
	if err := s.deps.Redis.Ping(ctx).Err(); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "redis_down", "err": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ready"})
}

type Reseller struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *Server) listResellers(c *gin.Context) {
	rows, err := s.deps.PG.Query(c.Request.Context(),
		`SELECT id, name, status, created_at FROM resellers ORDER BY id`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	out := []Reseller{}
	for rows.Next() {
		var r Reseller
		if err := rows.Scan(&r.ID, &r.Name, &r.Status, &r.CreatedAt); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		out = append(out, r)
	}
	c.JSON(http.StatusOK, out)
}

type Client struct {
	ID         int64     `json:"id"`
	ResellerID int64     `json:"reseller_id"`
	Name       string    `json:"name"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
}

func (s *Server) listClients(c *gin.Context) {
	rows, err := s.deps.PG.Query(c.Request.Context(),
		`SELECT id, reseller_id, name, status, created_at FROM clients ORDER BY id`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	out := []Client{}
	for rows.Next() {
		var x Client
		if err := rows.Scan(&x.ID, &x.ResellerID, &x.Name, &x.Status, &x.CreatedAt); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		out = append(out, x)
	}
	c.JSON(http.StatusOK, out)
}
