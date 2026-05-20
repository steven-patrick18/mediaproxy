package api

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

type MediaNode struct {
	ID                 int64      `json:"id"`
	Name               string     `json:"name"`
	Role               string     `json:"role"`
	HostIP             string     `json:"host_ip"`
	Region             *string    `json:"region,omitempty"`
	MaxCalls           int        `json:"max_calls"`
	TranscodingEnabled bool       `json:"transcoding_enabled"`
	Status             string     `json:"status"`
	AgentToken         string     `json:"agent_token,omitempty"`
	LastSeenAt         *time.Time `json:"last_seen_at,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
}

func (s *Server) listNodes(c *gin.Context) {
	rows, err := s.deps.PG.Query(c.Request.Context(),
		`SELECT id, name, role, host_ip::text, region, max_calls,
		        transcoding_enabled, status, last_seen_at, created_at
		   FROM media_nodes ORDER BY id`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	out := []MediaNode{}
	for rows.Next() {
		var n MediaNode
		if err := rows.Scan(&n.ID, &n.Name, &n.Role, &n.HostIP, &n.Region,
			&n.MaxCalls, &n.TranscodingEnabled, &n.Status,
			&n.LastSeenAt, &n.CreatedAt); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		out = append(out, n)
	}
	c.JSON(http.StatusOK, out)
}

type createNodeRequest struct {
	Name               string `json:"name" binding:"required,min=1,max=64"`
	Role               string `json:"role" binding:"required,oneof=media sip_proxy"`
	HostIP             string `json:"host_ip" binding:"required,ip"`
	Region             string `json:"region"`
	MaxCalls           int    `json:"max_calls" binding:"gte=0"`
	TranscodingEnabled bool   `json:"transcoding_enabled"`
}

func (s *Server) createNode(c *gin.Context) {
	var req createNodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	token, err := randomToken(32)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "token gen failed"})
		return
	}
	var node MediaNode
	var region *string
	if req.Region != "" {
		region = &req.Region
	}
	err = s.deps.PG.QueryRow(c.Request.Context(), `
		INSERT INTO media_nodes (name, role, host_ip, region, max_calls, transcoding_enabled, agent_token, status)
		VALUES ($1, $2, $3::inet, $4, $5, $6, $7, 'offline')
		RETURNING id, name, role, host_ip::text, region, max_calls, transcoding_enabled, status, agent_token, last_seen_at, created_at
	`, req.Name, req.Role, req.HostIP, region, req.MaxCalls, req.TranscodingEnabled, token).Scan(
		&node.ID, &node.Name, &node.Role, &node.HostIP, &node.Region,
		&node.MaxCalls, &node.TranscodingEnabled, &node.Status, &node.AgentToken,
		&node.LastSeenAt, &node.CreatedAt,
	)
	if err != nil {
		// duplicate name?
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "insert returned no row"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, node)
}

func randomToken(nBytes int) (string, error) {
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
