package api

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"github.com/gin-gonic/gin"
)

const baseAppSSHConfigPath = "/etc/ssh/sshd_config.d/99-mediaproxy.conf"

// GET /api/v1/system/ssh — returns the base-app host's effective SSH config.
func (s *Server) getSystemSSH(c *gin.Context) {
	body, err := os.ReadFile(baseAppSSHConfigPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"password_auth_enabled": strings.Contains(string(body), "PasswordAuthentication yes"),
		"config":                string(body),
	})
}

type setSystemSSHReq struct {
	PasswordAuth bool `json:"password_auth"`
}

// POST /api/v1/system/ssh — flips PasswordAuthentication on the base-app's
// own sshd. Admin-only. Same safety as the agent's set_ssh_auth: validates
// with `sshd -t` before reload. PermitRootLogin stays **no** regardless.
func (s *Server) setSystemSSH(c *gin.Context) {
	if c.GetString("role") != "admin" {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin only"})
		return
	}
	var req setSystemSSHReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	cfg := buildBaseAppSSHConfig(req.PasswordAuth)

	// Write via sudo tee (the base-app runs as deploy; deploy has NOPASSWD sudo).
	tee := exec.Command("sudo", "tee", baseAppSSHConfigPath)
	tee.Stdin = strings.NewReader(cfg)
	if out, err := tee.CombinedOutput(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("tee: %v (%s)", err, strings.TrimSpace(string(out)))})
		return
	}
	if out, err := exec.Command("sudo", "sshd", "-t").CombinedOutput(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("sshd -t rejected new config: %s", strings.TrimSpace(string(out)))})
		return
	}
	if out, err := exec.Command("sudo", "systemctl", "reload", "ssh").CombinedOutput(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("systemctl reload ssh: %s", strings.TrimSpace(string(out)))})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"password_auth_enabled": req.PasswordAuth,
		"applied":               true,
	})
}

func buildBaseAppSSHConfig(passwordAuth bool) string {
	var b strings.Builder
	b.WriteString("# Managed by mediaproxy panel. Do not edit by hand.\n")
	b.WriteString("PubkeyAuthentication yes\n")
	// PermitRootLogin is hard-locked to no on the base-app regardless of
	// password setting. Root login over SSH should never be possible here.
	b.WriteString("PermitRootLogin no\n")
	if passwordAuth {
		b.WriteString("PasswordAuthentication yes\n")
		b.WriteString("KbdInteractiveAuthentication yes\n")
		b.WriteString("ChallengeResponseAuthentication yes\n")
	} else {
		b.WriteString("PasswordAuthentication no\n")
		b.WriteString("KbdInteractiveAuthentication no\n")
		b.WriteString("ChallengeResponseAuthentication no\n")
	}
	// Hardening that stays on regardless.
	b.WriteString("PermitEmptyPasswords no\n")
	b.WriteString("X11Forwarding no\n")
	b.WriteString("AllowTcpForwarding no\n")
	b.WriteString("ClientAliveInterval 300\n")
	b.WriteString("ClientAliveCountMax 2\n")
	b.WriteString("MaxAuthTries 3\n")
	b.WriteString("LoginGraceTime 30\n")
	return b.String()
}
