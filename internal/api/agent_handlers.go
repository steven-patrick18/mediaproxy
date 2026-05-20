package api

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// requireAgentAuth pulls a bearer token off the Authorization header and
// looks up the corresponding media_nodes row. The matching node is stashed
// on the gin context so handlers don't need to re-query.
func requireAgentAuth(pg *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.GetHeader("Authorization")
		if !strings.HasPrefix(h, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing agent token"})
			return
		}
		tok := strings.TrimPrefix(h, "Bearer ")
		var (
			nodeID int64
			role   string
		)
		err := pg.QueryRow(c.Request.Context(),
			`SELECT id, role FROM media_nodes WHERE agent_token = $1`, tok,
		).Scan(&nodeID, &role)
		if errors.Is(err, pgx.ErrNoRows) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unknown agent"})
			return
		}
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.Set("agent_node_id", nodeID)
		c.Set("agent_node_role", role)
		c.Next()
	}
}

type agentRegisterRequest struct {
	Hostname         string `json:"hostname"`
	Cores            int    `json:"cores"`
	RAMMB            int    `json:"ram_mb"`
	RTPEngineVersion string `json:"rtpengine_version"`
}

type agentDirectiveResponse struct {
	NodeID      int64    `json:"node_id"`
	Role        string   `json:"role"`
	ExpectedIPs []string `json:"expected_ips"`
}

func (s *Server) agentRegister(c *gin.Context) {
	nodeID := c.GetInt64("agent_node_id")
	role := c.GetString("agent_node_role")

	var req agentRegisterRequest
	_ = c.ShouldBindJSON(&req) // tolerate empty body

	_, _ = s.deps.PG.Exec(c.Request.Context(), `
		UPDATE media_nodes
		   SET cpu_cores = COALESCE(NULLIF($2, 0), cpu_cores),
		       ram_gb    = COALESCE(NULLIF($3, 0), ram_gb),
		       rtpengine_version = COALESCE(NULLIF($4, ''), rtpengine_version),
		       last_seen_at = now(),
		       status = CASE WHEN status = 'draining' THEN 'draining' ELSE 'online' END
		 WHERE id = $1
	`, nodeID, req.Cores, req.RAMMB/1024, req.RTPEngineVersion)

	expected, err := s.expectedIPs(c.Request.Context(), nodeID, role)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, agentDirectiveResponse{
		NodeID:      nodeID,
		Role:        role,
		ExpectedIPs: expected,
	})
}

type agentHeartbeatRequest struct {
	BoundIPs    []string `json:"bound_ips"`
	ActiveCalls int      `json:"active_calls"`
	CPUPct      float64  `json:"cpu_pct"`
	RAMPct      float64  `json:"ram_pct"`
	NetInMbps   float64  `json:"net_in_mbps"`
	NetOutMbps  float64  `json:"net_out_mbps"`
}

type agentCommand struct {
	ID    string `json:"id"`
	Type  string `json:"type"`
	IP    string `json:"ip,omitempty"`
	CIDR  int    `json:"cidr,omitempty"`
	Iface string `json:"iface,omitempty"`
}

type agentHeartbeatResponse struct {
	ExpectedIPs []string       `json:"expected_ips"`
	Commands    []agentCommand `json:"commands"`
}

func (s *Server) agentHeartbeat(c *gin.Context) {
	nodeID := c.GetInt64("agent_node_id")
	role := c.GetString("agent_node_role")

	var req agentHeartbeatRequest
	_ = c.ShouldBindJSON(&req)

	if _, err := s.deps.PG.Exec(c.Request.Context(), `
		UPDATE media_nodes
		   SET last_seen_at = now(),
		       status = CASE WHEN status = 'draining' THEN 'draining' ELSE 'online' END
		 WHERE id = $1
	`, nodeID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Touch last_health_check on every IP this node owns.
	_, _ = s.deps.PG.Exec(c.Request.Context(),
		`UPDATE node_ips SET last_health_check = now() WHERE node_id = $1`, nodeID)

	expected, err := s.expectedIPs(c.Request.Context(), nodeID, role)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Commands queue is not yet persisted; return empty for now.
	c.JSON(http.StatusOK, agentHeartbeatResponse{
		ExpectedIPs: expected,
		Commands:    []agentCommand{},
	})
}

type agentCommandResultRequest struct {
	CommandID string `json:"command_id" binding:"required"`
	Status    string `json:"status" binding:"required,oneof=ok error"`
	Detail    string `json:"detail"`
}

func (s *Server) agentCommandResult(c *gin.Context) {
	var req agentCommandResultRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// TODO: persist into a node_commands table once it exists.
	c.Status(http.StatusNoContent)
}

// expectedIPs returns the authoritative set of IPs that should be bound on
// this node — node_ips for media role, signaling_ips for sip_proxy role.
func (s *Server) expectedIPs(ctx context.Context, nodeID int64, role string) ([]string, error) {
	var query string
	switch role {
	case "sip_proxy":
		query = `SELECT host(ip_address) FROM signaling_ips
		          WHERE sip_proxy_node_id = $1 AND status != 'disabled'
		          ORDER BY ip_address`
	default: // media
		query = `SELECT host(ip_address) FROM node_ips
		          WHERE node_id = $1 AND status IN ('active','reserve')
		          ORDER BY ip_address`
	}
	rows, err := s.deps.PG.Query(ctx, query, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var ip string
		if err := rows.Scan(&ip); err != nil {
			return nil, err
		}
		out = append(out, ip)
	}
	return out, nil
}
