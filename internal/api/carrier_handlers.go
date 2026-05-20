package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

type Carrier struct {
	ID             int64     `json:"id"`
	Name           string    `json:"name"`
	Host           string    `json:"host"`
	Port           int       `json:"port"`
	Transport      string    `json:"transport"`
	AssignedNodeID *int64    `json:"assigned_node_id,omitempty"`
	CodecPref      *string   `json:"codec_pref,omitempty"`
	Status         string    `json:"status"`
	CreatedAt      time.Time `json:"created_at"`
}

func (s *Server) listCarriers(c *gin.Context) {
	rows, err := s.deps.PG.Query(c.Request.Context(), `
		SELECT id, name, host, port, transport, assigned_node_id, codec_pref, status, created_at
		  FROM carriers ORDER BY id
	`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	out := []Carrier{}
	for rows.Next() {
		var x Carrier
		if err := rows.Scan(&x.ID, &x.Name, &x.Host, &x.Port, &x.Transport,
			&x.AssignedNodeID, &x.CodecPref, &x.Status, &x.CreatedAt); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		out = append(out, x)
	}
	c.JSON(http.StatusOK, out)
}

type createCarrierRequest struct {
	Name           string `json:"name" binding:"required,min=1,max=128"`
	Host           string `json:"host" binding:"required"`
	Port           int    `json:"port"`
	Transport      string `json:"transport" binding:"oneof=udp tcp tls ''"`
	AssignedNodeID *int64 `json:"assigned_node_id"`
	CodecPref      string `json:"codec_pref"`
}

func (s *Server) createCarrier(c *gin.Context) {
	var req createCarrierRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Port == 0 {
		req.Port = 5060
	}
	if req.Transport == "" {
		req.Transport = "udp"
	}
	var codecPref *string
	if req.CodecPref != "" {
		codecPref = &req.CodecPref
	}

	tx, err := s.deps.PG.Begin(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer tx.Rollback(c.Request.Context())

	var x Carrier
	err = tx.QueryRow(c.Request.Context(), `
		INSERT INTO carriers (name, host, port, transport, assigned_node_id, codec_pref)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, name, host, port, transport, assigned_node_id, codec_pref, status, created_at
	`, req.Name, req.Host, req.Port, req.Transport, req.AssignedNodeID, codecPref).Scan(
		&x.ID, &x.Name, &x.Host, &x.Port, &x.Transport, &x.AssignedNodeID, &x.CodecPref, &x.Status, &x.CreatedAt,
	)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// Initial history row
	if req.AssignedNodeID != nil {
		actor, _ := c.Get("user_id")
		if _, err := tx.Exec(c.Request.Context(), `
			INSERT INTO carrier_node_history (carrier_id, old_node_id, new_node_id, changed_by, reason)
			VALUES ($1, NULL, $2, $3, 'initial setup')
		`, x.ID, *req.AssignedNodeID, actor); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	if err := tx.Commit(c.Request.Context()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, x)
}

type updateCarrierRequest struct {
	AssignedNodeID *int64  `json:"assigned_node_id"`
	Reason         string  `json:"reason"`
	Status         *string `json:"status"`
}

// PATCH /api/v1/carriers/:id — currently supports reassigning the node
// (with history) and status changes.
func (s *Server) patchCarrier(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad id"})
		return
	}
	var req updateCarrierRequest
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

	if req.AssignedNodeID != nil {
		var oldNode *int64
		if err := tx.QueryRow(c.Request.Context(),
			`SELECT assigned_node_id FROM carriers WHERE id = $1`, id).Scan(&oldNode); err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "carrier not found"})
			return
		}
		if _, err := tx.Exec(c.Request.Context(),
			`UPDATE carriers SET assigned_node_id = $1 WHERE id = $2`,
			*req.AssignedNodeID, id); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		actor, _ := c.Get("user_id")
		reason := req.Reason
		if reason == "" {
			reason = "manual reassignment"
		}
		if _, err := tx.Exec(c.Request.Context(), `
			INSERT INTO carrier_node_history (carrier_id, old_node_id, new_node_id, changed_by, reason)
			VALUES ($1, $2, $3, $4, $5)
		`, id, oldNode, *req.AssignedNodeID, actor, reason); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	if req.Status != nil {
		switch *req.Status {
		case "active", "paused", "disabled":
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid status"})
			return
		}
		if _, err := tx.Exec(c.Request.Context(),
			`UPDATE carriers SET status = $1 WHERE id = $2`, *req.Status, id); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	if err := tx.Commit(c.Request.Context()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

func (s *Server) deleteCarrier(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad id"})
		return
	}
	tag, err := s.deps.PG.Exec(c.Request.Context(), `DELETE FROM carriers WHERE id = $1`, id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if tag.RowsAffected() == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	c.Status(http.StatusNoContent)
}

type CarrierHistoryEntry struct {
	ID                  int64     `json:"id"`
	OldNodeID           *int64    `json:"old_node_id,omitempty"`
	NewNodeID           *int64    `json:"new_node_id,omitempty"`
	ChangedBy           *int64    `json:"changed_by,omitempty"`
	ChangedAt           time.Time `json:"changed_at"`
	Reason              *string   `json:"reason,omitempty"`
	ActiveCallsAtSwitch *int      `json:"active_calls_at_switch,omitempty"`
}

func (s *Server) carrierHistory(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad id"})
		return
	}
	rows, err := s.deps.PG.Query(c.Request.Context(), `
		SELECT id, old_node_id, new_node_id, changed_by, changed_at, reason, active_calls_at_switch
		  FROM carrier_node_history WHERE carrier_id = $1 ORDER BY changed_at DESC
	`, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	out := []CarrierHistoryEntry{}
	for rows.Next() {
		var h CarrierHistoryEntry
		if err := rows.Scan(&h.ID, &h.OldNodeID, &h.NewNodeID, &h.ChangedBy, &h.ChangedAt, &h.Reason, &h.ActiveCallsAtSwitch); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		out = append(out, h)
	}
	c.JSON(http.StatusOK, out)
}
