package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"mediaproxy/internal/auth"

	"github.com/gin-gonic/gin"
)

type AdminUser struct {
	ID        int64     `json:"id"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	Status    string    `json:"status"`
	HasMFA    bool      `json:"has_mfa"`
	CreatedAt time.Time `json:"created_at"`
}

const validRoles = "admin noc reseller viewer"

func (s *Server) listAdminUsers(c *gin.Context) {
	rows, err := s.deps.PG.Query(c.Request.Context(), `
		SELECT id, email, role, status, mfa_secret IS NOT NULL, created_at
		  FROM admin_users ORDER BY id
	`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	out := []AdminUser{}
	for rows.Next() {
		var u AdminUser
		if err := rows.Scan(&u.ID, &u.Email, &u.Role, &u.Status, &u.HasMFA, &u.CreatedAt); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		out = append(out, u)
	}
	c.JSON(http.StatusOK, out)
}

type createAdminUserRequest struct {
	Email    string `json:"email"    binding:"required,email"`
	Password string `json:"password" binding:"required,min=8"`
	Role     string `json:"role"     binding:"required,oneof=admin noc reseller viewer"`
}

func (s *Server) createAdminUser(c *gin.Context) {
	if c.GetString("role") != "admin" {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin only"})
		return
	}
	var req createAdminUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "hash failed"})
		return
	}
	var u AdminUser
	err = s.deps.PG.QueryRow(c.Request.Context(), `
		INSERT INTO admin_users (email, password_hash, role)
		VALUES ($1, $2, $3)
		RETURNING id, email, role, status, mfa_secret IS NOT NULL, created_at
	`, strings.ToLower(strings.TrimSpace(req.Email)), hash, req.Role).Scan(
		&u.ID, &u.Email, &u.Role, &u.Status, &u.HasMFA, &u.CreatedAt,
	)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, u)
}

type patchAdminUserRequest struct {
	Role     *string `json:"role"`
	Status   *string `json:"status"`
	Password *string `json:"password"`
}

func (s *Server) patchAdminUser(c *gin.Context) {
	if c.GetString("role") != "admin" {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin only"})
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad id"})
		return
	}
	var req patchAdminUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Role != nil && !strings.Contains(validRoles, *req.Role) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid role"})
		return
	}
	if req.Status != nil {
		switch *req.Status {
		case "active", "suspended":
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid status"})
			return
		}
	}
	var hash *string
	if req.Password != nil {
		if len(*req.Password) < 8 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "password must be 8+ chars"})
			return
		}
		h, err := auth.HashPassword(*req.Password)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "hash failed"})
			return
		}
		hash = &h
	}
	tag, err := s.deps.PG.Exec(c.Request.Context(), `
		UPDATE admin_users
		   SET role          = COALESCE($2, role),
		       status        = COALESCE($3, status),
		       password_hash = COALESCE($4, password_hash)
		 WHERE id = $1
	`, id, req.Role, req.Status, hash)
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

func (s *Server) deleteAdminUser(c *gin.Context) {
	if c.GetString("role") != "admin" {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin only"})
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad id"})
		return
	}
	if id == c.GetInt64("user_id") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot delete yourself"})
		return
	}
	tag, err := s.deps.PG.Exec(c.Request.Context(), `DELETE FROM admin_users WHERE id = $1`, id)
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
