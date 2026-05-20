package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type createResellerRequest struct {
	Name string `json:"name" binding:"required,min=1,max=128"`
}

func (s *Server) createReseller(c *gin.Context) {
	var req createResellerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var r Reseller
	err := s.deps.PG.QueryRow(c.Request.Context(), `
		INSERT INTO resellers (name) VALUES ($1)
		RETURNING id, name, status, created_at
	`, req.Name).Scan(&r.ID, &r.Name, &r.Status, &r.CreatedAt)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, r)
}

type patchResellerRequest struct {
	Name   *string `json:"name"`
	Status *string `json:"status"`
	Notes  *string `json:"notes"`
}

func (s *Server) patchReseller(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad id"})
		return
	}
	var req patchResellerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Status != nil {
		switch *req.Status {
		case "active", "suspended", "deleted":
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid status"})
			return
		}
	}
	tag, err := s.deps.PG.Exec(c.Request.Context(), `
		UPDATE resellers
		   SET name   = COALESCE($2, name),
		       status = COALESCE($3, status),
		       notes  = COALESCE($4, notes)
		 WHERE id = $1
	`, id, req.Name, req.Status, req.Notes)
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

func (s *Server) deleteReseller(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad id"})
		return
	}
	var n int
	if err := s.deps.PG.QueryRow(c.Request.Context(),
		`SELECT count(*) FROM clients WHERE reseller_id = $1`, id).Scan(&n); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if n > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "reseller still owns clients"})
		return
	}
	tag, err := s.deps.PG.Exec(c.Request.Context(),
		`DELETE FROM resellers WHERE id = $1`, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if tag.RowsAffected() == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	c.Status(http.StatusNoContent)
}

type createClientRequest struct {
	ResellerID int64  `json:"reseller_id" binding:"required,gt=0"`
	Name       string `json:"name"        binding:"required,min=1,max=128"`
}

func (s *Server) createClient(c *gin.Context) {
	var req createClientRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var cl Client
	err := s.deps.PG.QueryRow(c.Request.Context(), `
		INSERT INTO clients (reseller_id, name) VALUES ($1, $2)
		RETURNING id, reseller_id, name, status, created_at
	`, req.ResellerID, req.Name).Scan(&cl.ID, &cl.ResellerID, &cl.Name, &cl.Status, &cl.CreatedAt)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, cl)
}

type patchClientRequest struct {
	Name       *string `json:"name"`
	ResellerID *int64  `json:"reseller_id"`
	Status     *string `json:"status"`
	Notes      *string `json:"notes"`
}

func (s *Server) patchClient(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad id"})
		return
	}
	var req patchClientRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Status != nil {
		switch *req.Status {
		case "active", "suspended", "deleted":
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid status"})
			return
		}
	}
	tag, err := s.deps.PG.Exec(c.Request.Context(), `
		UPDATE clients
		   SET name        = COALESCE($2, name),
		       reseller_id = COALESCE($3, reseller_id),
		       status      = COALESCE($4, status),
		       notes       = COALESCE($5, notes)
		 WHERE id = $1
	`, id, req.Name, req.ResellerID, req.Status, req.Notes)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if tag.RowsAffected() == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	// Suspension or rename may affect downstream cache
	_ = s.deps.SigCache.SyncClient(c.Request.Context(), id)
	c.Status(http.StatusNoContent)
}

func (s *Server) deleteClient(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad id"})
		return
	}
	// Unassign signaling IP first so the pool entry frees up.
	if _, err := s.deps.PG.Exec(c.Request.Context(),
		`UPDATE signaling_ips SET status='available', assigned_client_id=NULL WHERE assigned_client_id=$1`, id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	tag, err := s.deps.PG.Exec(c.Request.Context(), `DELETE FROM clients WHERE id = $1`, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if tag.RowsAffected() == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	// Best-effort cache cleanup
	_ = s.deps.SigCache.SyncClient(c.Request.Context(), id)
	c.Status(http.StatusNoContent)
}
