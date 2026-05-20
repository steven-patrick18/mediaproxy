package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"mediaproxy/internal/auth"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

type loginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=1"`
}

type loginResponse struct {
	Token string  `json:"token"`
	User  meUser  `json:"user"`
	ExpAt int64   `json:"exp_at"`
}

type meUser struct {
	ID    int64  `json:"id"`
	Email string `json:"email"`
	Role  string `json:"role"`
}

func (s *Server) login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))

	var (
		id     int64
		hash   string
		role   string
		status string
	)
	err := s.deps.PG.QueryRow(c.Request.Context(),
		`SELECT id, password_hash, role, status FROM admin_users WHERE email = $1`,
		req.Email,
	).Scan(&id, &hash, &role, &status)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && status != "active") || (err == nil && !auth.VerifyPassword(hash, req.Password)) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "lookup failed"})
		return
	}

	ttl := 24 * time.Hour
	tok, err := auth.SignJWT(id, role, s.deps.JWTSecret, ttl)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "sign failed"})
		return
	}
	c.JSON(http.StatusOK, loginResponse{
		Token: tok,
		User:  meUser{ID: id, Email: req.Email, Role: role},
		ExpAt: time.Now().Add(ttl).Unix(),
	})
}

func (s *Server) me(c *gin.Context) {
	uid := c.GetInt64("user_id")
	var (
		email  string
		role   string
		status string
	)
	err := s.deps.PG.QueryRow(c.Request.Context(),
		`SELECT email, role, status FROM admin_users WHERE id = $1`, uid,
	).Scan(&email, &role, &status)
	if err != nil || status != "active" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not found"})
		return
	}
	c.JSON(http.StatusOK, meUser{ID: uid, Email: email, Role: role})
}
