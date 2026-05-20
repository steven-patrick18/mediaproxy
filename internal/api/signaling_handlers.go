package api

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

type SignalingIP struct {
	ID               int64     `json:"id"`
	IPAddress        string    `json:"ip_address"`
	SipProxyNodeID   int64     `json:"sip_proxy_node_id"`
	Status           string    `json:"status"`
	AssignedClientID *int64    `json:"assigned_client_id,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
}

func (s *Server) listSignalingIPs(c *gin.Context) {
	rows, err := s.deps.PG.Query(c.Request.Context(), `
		SELECT id, host(ip_address), sip_proxy_node_id, status, assigned_client_id, created_at
		  FROM signaling_ips
		 ORDER BY id
	`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	out := []SignalingIP{}
	for rows.Next() {
		var s SignalingIP
		if err := rows.Scan(&s.ID, &s.IPAddress, &s.SipProxyNodeID, &s.Status, &s.AssignedClientID, &s.CreatedAt); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		out = append(out, s)
	}
	c.JSON(http.StatusOK, out)
}

type createSignalingIPRequest struct {
	IPAddress      string `json:"ip_address" binding:"required,ip"`
	SipProxyNodeID int64  `json:"sip_proxy_node_id" binding:"required,gt=0"`
}

func (s *Server) createSignalingIP(c *gin.Context) {
	var req createSignalingIPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// verify the node is a sip_proxy
	var role string
	if err := s.deps.PG.QueryRow(c.Request.Context(),
		`SELECT role FROM media_nodes WHERE id = $1`, req.SipProxyNodeID).Scan(&role); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "node not found"})
		return
	}
	if role != "sip_proxy" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "signaling IPs must live on a sip_proxy node"})
		return
	}

	var out SignalingIP
	err := s.deps.PG.QueryRow(c.Request.Context(), `
		INSERT INTO signaling_ips (ip_address, sip_proxy_node_id, status)
		VALUES ($1::inet, $2, 'available')
		RETURNING id, host(ip_address), sip_proxy_node_id, status, assigned_client_id, created_at
	`, req.IPAddress, req.SipProxyNodeID).Scan(
		&out.ID, &out.IPAddress, &out.SipProxyNodeID, &out.Status, &out.AssignedClientID, &out.CreatedAt,
	)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, out)
}

func (s *Server) deleteSignalingIP(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad id"})
		return
	}
	// detach from any client first; cascade via the FK ON DELETE will already null clients.signaling_ip_id
	var clientID *int64
	if err := s.deps.PG.QueryRow(c.Request.Context(),
		`SELECT assigned_client_id FROM signaling_ips WHERE id = $1`, id).Scan(&clientID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if _, err := s.deps.PG.Exec(c.Request.Context(),
		`DELETE FROM signaling_ips WHERE id = $1`, id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if clientID != nil {
		_ = s.deps.SigCache.SyncClient(c.Request.Context(), *clientID)
	}
	c.Status(http.StatusNoContent)
}

type assignSignalingIPRequest struct {
	SignalingIPID int64 `json:"signaling_ip_id" binding:"required,gt=0"`
}

// POST /api/v1/clients/:id/signaling-ip
func (s *Server) assignSignalingIP(c *gin.Context) {
	clientID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad client id"})
		return
	}
	var req assignSignalingIPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	tx, err := s.deps.PG.Begin(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer tx.Rollback(c.Request.Context())

	// Release previously-assigned IP, if any
	if _, err := tx.Exec(c.Request.Context(), `
		UPDATE signaling_ips
		   SET status = 'available', assigned_client_id = NULL
		 WHERE assigned_client_id = $1
	`, clientID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Reserve the requested IP atomically; only succeeds if it's currently available.
	tag, err := tx.Exec(c.Request.Context(), `
		UPDATE signaling_ips
		   SET status = 'assigned', assigned_client_id = $1
		 WHERE id = $2 AND status = 'available' AND assigned_client_id IS NULL
	`, clientID, req.SignalingIPID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if tag.RowsAffected() == 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "signaling IP is not available"})
		return
	}

	if _, err := tx.Exec(c.Request.Context(),
		`UPDATE clients SET signaling_ip_id = $1 WHERE id = $2`,
		req.SignalingIPID, clientID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := tx.Commit(c.Request.Context()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if err := s.deps.SigCache.SyncClient(c.Request.Context(), clientID); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"status":     "assigned",
			"cache_warn": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "assigned"})
}

// DELETE /api/v1/clients/:id/signaling-ip
func (s *Server) unassignSignalingIP(c *gin.Context) {
	clientID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad client id"})
		return
	}
	tx, err := s.deps.PG.Begin(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer tx.Rollback(c.Request.Context())

	if _, err := tx.Exec(c.Request.Context(), `
		UPDATE signaling_ips
		   SET status = 'available', assigned_client_id = NULL
		 WHERE assigned_client_id = $1
	`, clientID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if _, err := tx.Exec(c.Request.Context(),
		`UPDATE clients SET signaling_ip_id = NULL WHERE id = $1`, clientID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := tx.Commit(c.Request.Context()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	_ = s.deps.SigCache.SyncClient(c.Request.Context(), clientID)
	c.Status(http.StatusNoContent)
}
