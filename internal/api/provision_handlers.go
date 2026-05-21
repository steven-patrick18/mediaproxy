package api

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"mediaproxy/internal/provisioner"

	"github.com/gin-gonic/gin"
)

type provisionRequest struct {
	SSHHost          string `json:"ssh_host" binding:"required"`
	SSHPort          int    `json:"ssh_port"`
	SSHUser          string `json:"ssh_user" binding:"required"`
	SSHPassword      string `json:"ssh_password"`
	SSHKey           string `json:"ssh_key"`
	SSHKeyPassphrase string `json:"ssh_key_passphrase"`
}

type provisionResponse struct {
	OK  bool   `json:"ok"`
	Log string `json:"log"`
}

// POST /api/v1/nodes/:id/provision
//
// Connects to the node's host over SSH (root + password are used only in
// memory for the duration of the request, never persisted), installs the
// node-agent binary, writes its config + systemd unit, and starts it.
// Returns the full install log so the UI can show it.
func (s *Server) provisionNode(c *gin.Context) {
	if c.GetString("role") != "admin" {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin only"})
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad id"})
		return
	}
	var req provisionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.SSHPassword == "" && req.SSHKey == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "either ssh_password or ssh_key is required"})
		return
	}

	var (
		role       string
		agentToken string
		hostIP     string
	)
	if err := s.deps.PG.QueryRow(c.Request.Context(),
		`SELECT role, agent_token, host(host_ip) FROM media_nodes WHERE id = $1`, id,
	).Scan(&role, &agentToken, &hostIP); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "node not found"})
		return
	}

	if req.SSHHost == "" {
		req.SSHHost = hostIP
	}

	// Provisioning can take ~30s; give it a generous ceiling.
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Minute)
	defer cancel()

	// nginx terminates TLS before the request reaches us — so c.Request.TLS
	// is always nil. Trust X-Forwarded-Proto when present (set by nginx).
	scheme := "https"
	if p := c.GetHeader("X-Forwarded-Proto"); p != "" {
		scheme = p
	} else if c.Request.TLS == nil {
		scheme = "http"
	}
	host := c.Request.Host
	if h := c.GetHeader("X-Forwarded-Host"); h != "" {
		host = h
	}
	binaryURL := scheme + "://" + host + "/agent-binary"
	controlPlaneURL := scheme + "://" + host

	result := provisioner.Run(ctx, provisioner.Request{
		Host:                 req.SSHHost,
		Port:                 req.SSHPort,
		User:                 req.SSHUser,
		Password:             req.SSHPassword,
		PrivateKey:           req.SSHKey,
		PrivateKeyPassphrase: req.SSHKeyPassphrase,
		NodeID:               id,
		Role:                 role,
		AgentToken:           agentToken,
		ControlPlaneURL:      controlPlaneURL,
		BinaryURL:            binaryURL,
	})
	c.JSON(http.StatusOK, provisionResponse{OK: result.OK, Log: result.Log})
}
