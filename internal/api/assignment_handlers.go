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

type patchAssignmentRequest struct {
	RotationStrategy *string `json:"rotation_strategy" binding:"omitempty,oneof=round_robin random sticky least_used health_weighted"`
}

// PATCH /api/v1/assignments/:id — update an active assignment's
// rotation_strategy. Only active rows can be patched; ended rows are
// immutable for audit. Returns the updated row so the UI can refresh
// without a separate GET.
func (s *Server) patchAssignment(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad id"})
		return
	}
	var req patchAssignmentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var a Assignment
	err = s.deps.PG.QueryRow(c.Request.Context(), `
		UPDATE assignments
		   SET rotation_strategy = COALESCE($2, rotation_strategy),
		       rotation_cursor   = 0
		 WHERE id = $1 AND status = 'active'
		 RETURNING id, group_id, client_id, carrier_id, rotation_strategy, status, assigned_by, assigned_at
	`, id, req.RotationStrategy).Scan(
		&a.ID, &a.GroupID, &a.ClientID, &a.CarrierID, &a.RotationStrategy, &a.Status, &a.AssignedBy, &a.AssignedAt,
	)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "assignment not found or not active"})
		return
	}
	c.JSON(http.StatusOK, a)
}

// endAssignment is a two-stage delete:
//   - active row → soft-end (status='ended') so the audit trail keeps the
//     row + assigned_at. Routing immediately stops considering it.
//   - already-ended row → hard delete. Operator must End first, then click
//     Delete again — protects against accidentally removing a live rotation
//     with one stray click.
func (s *Server) endAssignment(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad id"})
		return
	}
	// Check current status to decide soft-end vs hard-delete.
	var status string
	err = s.deps.PG.QueryRow(c.Request.Context(),
		`SELECT status FROM assignments WHERE id = $1`, id).Scan(&status)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	if status == "ended" {
		if _, err := s.deps.PG.Exec(c.Request.Context(),
			`DELETE FROM assignments WHERE id = $1 AND status = 'ended'`, id); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.Status(http.StatusNoContent)
		return
	}
	if _, err := s.deps.PG.Exec(c.Request.Context(),
		`UPDATE assignments SET status = 'ended' WHERE id = $1`, id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}
