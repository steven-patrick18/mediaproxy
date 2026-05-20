package api

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image/png"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/pquerna/otp/totp"
)

// POST /api/v1/mfa/setup
// Returns a freshly-generated TOTP secret + QR code (PNG, base64) for the
// caller to scan into Google Authenticator / Authy / 1Password.
// The secret is stored on the user row but enrolled=false until verified.
func (s *Server) mfaSetup(c *gin.Context) {
	uid := c.GetInt64("user_id")
	var email string
	if err := s.deps.PG.QueryRow(c.Request.Context(),
		`SELECT email FROM admin_users WHERE id = $1`, uid).Scan(&email); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not found"})
		return
	}
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      "mediaproxy",
		AccountName: email,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// Persist the secret; mark enrolled=false until verify succeeds.
	if _, err := s.deps.PG.Exec(c.Request.Context(), `
		UPDATE admin_users SET mfa_secret = $1, mfa_enrolled = false
		 WHERE id = $2
	`, key.Secret(), uid); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	img, err := key.Image(220, 220)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	var pngBuf bytes.Buffer
	if err := png.Encode(&pngBuf, img); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"secret":   key.Secret(),
		"otpauth":  key.URL(),
		"qr_png_b64": base64.StdEncoding.EncodeToString(pngBuf.Bytes()),
	})
}

type mfaVerifyReq struct {
	Code string `json:"code" binding:"required,len=6"`
}

// POST /api/v1/mfa/verify
// Operator pastes the 6-digit code from their authenticator app to enroll.
// Also generates 8 one-time recovery codes returned (only) on success.
func (s *Server) mfaVerify(c *gin.Context) {
	uid := c.GetInt64("user_id")
	var req mfaVerifyReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var secret string
	if err := s.deps.PG.QueryRow(c.Request.Context(),
		`SELECT COALESCE(mfa_secret, '') FROM admin_users WHERE id = $1`, uid).Scan(&secret); err != nil || secret == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no MFA setup pending"})
		return
	}
	if !totp.Validate(req.Code, secret) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid code"})
		return
	}
	// Generate 8 recovery codes (hex; one-time).
	codes := make([]string, 8)
	for i := range codes {
		codes[i] = randomCodePart(4) + "-" + randomCodePart(4)
	}
	// Store the codes (plain — they're meant to be shown once and used once;
	// a future iteration can hash them).
	codesJSON, _ := json.Marshal(codes)
	if _, err := s.deps.PG.Exec(c.Request.Context(), `
		UPDATE admin_users SET mfa_enrolled = true, mfa_recovery_codes = $1::jsonb WHERE id = $2
	`, string(codesJSON), uid); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"enrolled":       true,
		"recovery_codes": codes,
	})
}

// POST /api/v1/mfa/disable
// Operator disables MFA on their own account (requires a valid current code).
func (s *Server) mfaDisable(c *gin.Context) {
	uid := c.GetInt64("user_id")
	var req mfaVerifyReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var secret string
	if err := s.deps.PG.QueryRow(c.Request.Context(),
		`SELECT COALESCE(mfa_secret, '') FROM admin_users WHERE id = $1`, uid).Scan(&secret); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if secret == "" {
		c.Status(http.StatusNoContent)
		return
	}
	if !totp.Validate(req.Code, secret) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid code"})
		return
	}
	_, _ = s.deps.PG.Exec(c.Request.Context(), `
		UPDATE admin_users SET mfa_secret = NULL, mfa_enrolled = false, mfa_recovery_codes = NULL WHERE id = $1
	`, uid)
	c.Status(http.StatusNoContent)
}

// Tiny crypto-rand helper for recovery codes.
func randomCodePart(n int) string {
	const charset = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	b := make([]byte, n)
	if _, err := cryptoRand(b); err != nil {
		return "00000000"[:n]
	}
	for i := range b {
		b[i] = charset[int(b[i])%len(charset)]
	}
	return string(b)
}
