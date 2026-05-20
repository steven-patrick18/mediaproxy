package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

type Assignment struct {
	ID               int64     `json:"id"`
	GroupID          int64     `json:"group_id"`
	ClientID         int64     `json:"client_id"`
	CarrierID        int64     `json:"carrier_id"`
	RotationStrategy string    `json:"rotation_strategy"`
	Status           string    `json:"status"`
	AssignedBy       *int64    `json:"assigned_by,omitempty"`
	AssignedAt       time.Time `json:"assigned_at"`
}

func (s *Server) listAssignments(c *gin.Context) {
	rows, err := s.deps.PG.Query(c.Request.Context(), `
		SELECT id, group_id, client_id, carrier_id, rotation_strategy, status, assigned_by, assigned_at
		  FROM assignments ORDER BY id DESC
	`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	out := []Assignment{}
	for rows.Next() {
		var a Assignment
		if err := rows.Scan(&a.ID, &a.GroupID, &a.ClientID, &a.CarrierID,
			&a.RotationStrategy, &a.Status, &a.AssignedBy, &a.AssignedAt); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		out = append(out, a)
	}
	c.JSON(http.StatusOK, out)
}

type createAssignmentRequest struct {
	GroupID          int64  `json:"group_id" binding:"required,gt=0"`
	ClientID         int64  `json:"client_id" binding:"required,gt=0"`
	CarrierID        int64  `json:"carrier_id" binding:"required,gt=0"`
	RotationStrategy string `json:"rotation_strategy" binding:"omitempty,oneof=round_robin random sticky least_used health_weighted"`
}

func (s *Server) createAssignment(c *gin.Context) {
	var req createAssignmentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.RotationStrategy == "" {
		req.RotationStrategy = "round_robin"
	}
	actor, _ := c.Get("user_id")
	var a Assignment
	err := s.deps.PG.QueryRow(c.Request.Context(), `
		INSERT INTO assignments (group_id, client_id, carrier_id, rotation_strategy, assigned_by)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, group_id, client_id, carrier_id, rotation_strategy, status, assigned_by, assigned_at
	`, req.GroupID, req.ClientID, req.CarrierID, req.RotationStrategy, actor).Scan(
		&a.ID, &a.GroupID, &a.ClientID, &a.CarrierID, &a.RotationStrategy, &a.Status, &a.AssignedBy, &a.AssignedAt,
	)
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, a)
}

// "Delete" marks the row ended rather than hard-deleting so we keep an audit trail.
func (s *Server) endAssignment(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad id"})
		return
	}
	tag, err := s.deps.PG.Exec(c.Request.Context(),
		`UPDATE assignments SET status = 'ended' WHERE id = $1 AND status != 'ended'`, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if tag.RowsAffected() == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found or already ended"})
		return
	}
	c.Status(http.StatusNoContent)
}
