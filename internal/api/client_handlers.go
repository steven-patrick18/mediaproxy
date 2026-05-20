package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

type ClientDetail struct {
	ID            int64     `json:"id"`
	ResellerID    int64     `json:"reseller_id"`
	Name          string    `json:"name"`
	Status        string    `json:"status"`
	SignalingIPID *int64    `json:"signaling_ip_id,omitempty"`
	SignalingIP   *string   `json:"signaling_ip,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

// Extends listClients to include signaling IP info.
// (handlers.go still has listClients with the raw struct; this is a richer view.)
func (s *Server) getClientDetail(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad id"})
		return
	}
	var d ClientDetail
	err = s.deps.PG.QueryRow(c.Request.Context(), `
		SELECT c.id, c.reseller_id, c.name, c.status, c.signaling_ip_id,
		       (SELECT host(ip_address) FROM signaling_ips WHERE id = c.signaling_ip_id),
		       c.created_at
		  FROM clients c WHERE c.id = $1
	`, id).Scan(&d.ID, &d.ResellerID, &d.Name, &d.Status, &d.SignalingIPID, &d.SignalingIP, &d.CreatedAt)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	c.JSON(http.StatusOK, d)
}

type DialerIP struct {
	ID        int64     `json:"id"`
	ClientID  int64     `json:"client_id"`
	IPAddress string    `json:"ip_address"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *Server) listDialerIPs(c *gin.Context) {
	clientID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad client id"})
		return
	}
	rows, err := s.deps.PG.Query(c.Request.Context(), `
		SELECT id, client_id, host(ip_address), status, created_at
		  FROM client_ips WHERE client_id = $1 ORDER BY id
	`, clientID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	out := []DialerIP{}
	for rows.Next() {
		var d DialerIP
		if err := rows.Scan(&d.ID, &d.ClientID, &d.IPAddress, &d.Status, &d.CreatedAt); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		out = append(out, d)
	}
	c.JSON(http.StatusOK, out)
}

type addDialerIPRequest struct {
	IPAddress string `json:"ip_address" binding:"required,ip"`
}

func (s *Server) addDialerIP(c *gin.Context) {
	clientID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad client id"})
		return
	}
	var req addDialerIPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var out DialerIP
	err = s.deps.PG.QueryRow(c.Request.Context(), `
		INSERT INTO client_ips (client_id, ip_address, status)
		VALUES ($1, $2::inet, 'active')
		RETURNING id, client_id, host(ip_address), status, created_at
	`, clientID, req.IPAddress).Scan(&out.ID, &out.ClientID, &out.IPAddress, &out.Status, &out.CreatedAt)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	_ = s.deps.SigCache.SyncClient(c.Request.Context(), clientID)
	c.JSON(http.StatusCreated, out)
}

func (s *Server) removeDialerIP(c *gin.Context) {
	clientID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad client id"})
		return
	}
	dialerID, err := strconv.ParseInt(c.Param("dialer_ip_id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad dialer ip id"})
		return
	}
	tag, err := s.deps.PG.Exec(c.Request.Context(),
		`DELETE FROM client_ips WHERE id = $1 AND client_id = $2`, dialerID, clientID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if tag.RowsAffected() == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	_ = s.deps.SigCache.SyncClient(c.Request.Context(), clientID)
	c.Status(http.StatusNoContent)
}
