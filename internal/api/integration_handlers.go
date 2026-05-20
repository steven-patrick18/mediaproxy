package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"mediaproxy/internal/signalwire"

	"github.com/gin-gonic/gin"
)

type Integration struct {
	ID             int64           `json:"id"`
	Name           string          `json:"name"`
	Provider       string          `json:"provider"`
	Config         json.RawMessage `json:"config"`
	Status         string          `json:"status"`
	LastVerifiedAt *time.Time      `json:"last_verified_at,omitempty"`
	LastError      *string         `json:"last_error,omitempty"`
	CreatedBy      *int64          `json:"created_by,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
}

// maskConfig redacts secret-ish fields in the JSON config before sending it
// to the client. The original is preserved in the DB.
func maskConfig(raw []byte) json.RawMessage {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return raw
	}
	secrets := []string{"api_token", "api_key", "auth_token", "password", "secret"}
	for _, k := range secrets {
		if v, ok := m[k]; ok {
			s, _ := v.(string)
			if len(s) <= 4 {
				m[k] = "***"
			} else {
				m[k] = "***" + s[len(s)-4:]
			}
		}
	}
	out, _ := json.Marshal(m)
	return out
}

func (s *Server) listIntegrations(c *gin.Context) {
	rows, err := s.deps.PG.Query(c.Request.Context(), `
		SELECT id, name, provider, config::text, status,
		       last_verified_at, last_error, created_by, created_at
		  FROM external_integrations ORDER BY id
	`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	out := []Integration{}
	for rows.Next() {
		var (
			x          Integration
			configText string
		)
		if err := rows.Scan(&x.ID, &x.Name, &x.Provider, &configText, &x.Status,
			&x.LastVerifiedAt, &x.LastError, &x.CreatedBy, &x.CreatedAt); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		x.Config = maskConfig([]byte(configText))
		out = append(out, x)
	}
	c.JSON(http.StatusOK, out)
}

type createIntegrationRequest struct {
	Name     string         `json:"name" binding:"required,min=1,max=128"`
	Provider string         `json:"provider" binding:"required,oneof=signalwire freeswitch twilio other"`
	Config   map[string]any `json:"config" binding:"required"`
}

func (s *Server) createIntegration(c *gin.Context) {
	var req createIntegrationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	configBytes, _ := json.Marshal(req.Config)
	actor, _ := c.Get("user_id")
	var x Integration
	var configText string
	err := s.deps.PG.QueryRow(c.Request.Context(), `
		INSERT INTO external_integrations (name, provider, config, created_by)
		VALUES ($1, $2, $3, $4)
		RETURNING id, name, provider, config::text, status,
		          last_verified_at, last_error, created_by, created_at
	`, req.Name, req.Provider, configBytes, actor).Scan(
		&x.ID, &x.Name, &x.Provider, &configText, &x.Status,
		&x.LastVerifiedAt, &x.LastError, &x.CreatedBy, &x.CreatedAt,
	)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	x.Config = maskConfig([]byte(configText))
	c.JSON(http.StatusCreated, x)
}

type patchIntegrationRequest struct {
	Name   *string         `json:"name"`
	Config *map[string]any `json:"config"`
	Status *string         `json:"status"`
}

func (s *Server) patchIntegration(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad id"})
		return
	}
	var req patchIntegrationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Status != nil {
		switch *req.Status {
		case "unverified", "verified", "failed", "disabled":
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid status"})
			return
		}
	}

	// Merge config: incoming keys overwrite stored keys; fields with value
	// "***..." (placeholders we showed in GET) are dropped so the client
	// can submit the whole config without overwriting real secrets.
	if req.Config != nil {
		var cur map[string]any
		var stored string
		if err := s.deps.PG.QueryRow(c.Request.Context(),
			`SELECT config::text FROM external_integrations WHERE id = $1`, id).Scan(&stored); err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		_ = json.Unmarshal([]byte(stored), &cur)
		if cur == nil {
			cur = map[string]any{}
		}
		for k, v := range *req.Config {
			if s, ok := v.(string); ok && (s == "***" || (len(s) >= 5 && s[:3] == "***")) {
				continue // placeholder; keep stored
			}
			cur[k] = v
		}
		merged, _ := json.Marshal(cur)
		if _, err := s.deps.PG.Exec(c.Request.Context(),
			`UPDATE external_integrations SET config = $2 WHERE id = $1`, id, merged); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	if req.Name != nil || req.Status != nil {
		if _, err := s.deps.PG.Exec(c.Request.Context(), `
			UPDATE external_integrations
			   SET name = COALESCE($2, name), status = COALESCE($3, status)
			 WHERE id = $1
		`, id, req.Name, req.Status); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}
	c.Status(http.StatusNoContent)
}

func (s *Server) deleteIntegration(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad id"})
		return
	}
	tag, err := s.deps.PG.Exec(c.Request.Context(),
		`DELETE FROM external_integrations WHERE id = $1`, id)
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

type verifyResponse struct {
	OK         bool   `json:"ok"`
	Status     string `json:"status"`
	StatusCode int    `json:"status_code,omitempty"`
	Error      string `json:"error,omitempty"`
}

// POST /api/v1/integrations/:id/verify
func (s *Server) verifyIntegration(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad id"})
		return
	}
	var (
		provider   string
		configText string
	)
	if err := s.deps.PG.QueryRow(c.Request.Context(),
		`SELECT provider, config::text FROM external_integrations WHERE id = $1`, id,
	).Scan(&provider, &configText); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()

	var (
		newStatus string
		lastErr   string
		out       verifyResponse
	)
	switch provider {
	case "signalwire":
		var cfg struct {
			SpaceURL  string `json:"space_url"`
			ProjectID string `json:"project_id"`
			APIToken  string `json:"api_token"`
		}
		_ = json.Unmarshal([]byte(configText), &cfg)
		r := signalwire.Verify(ctx, signalwire.Creds{
			SpaceURL:  cfg.SpaceURL,
			ProjectID: cfg.ProjectID,
			APIToken:  cfg.APIToken,
		})
		out.OK = r.OK
		out.StatusCode = r.StatusCode
		if r.OK {
			newStatus = "verified"
		} else {
			newStatus = "failed"
			lastErr = r.Error
			if lastErr == "" {
				lastErr = "unknown error"
			}
			out.Error = lastErr
		}
	default:
		newStatus = "unverified"
		out.Error = "verification is not implemented for provider " + provider + " — set status manually after confirming connectivity"
		out.OK = false
	}

	var errArg *string
	if lastErr != "" {
		errArg = &lastErr
	}
	_, _ = s.deps.PG.Exec(c.Request.Context(), `
		UPDATE external_integrations
		   SET status = $2,
		       last_verified_at = now(),
		       last_error = $3
		 WHERE id = $1
	`, id, newStatus, errArg)

	out.Status = newStatus
	code := http.StatusOK
	if !out.OK {
		code = http.StatusBadGateway
	}
	c.JSON(code, out)
}
