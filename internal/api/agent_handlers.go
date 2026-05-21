package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
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
	AgentVersion     string `json:"agent_version"`
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
	if err := c.ShouldBindJSON(&req); err != nil {
		// Old agents may post extra fields; tolerate but log so we notice
		// a true schema mismatch.
		slog.Warn("agent register: body bind failed", "node_id", nodeID, "err", err)
	}

	if _, err := s.deps.PG.Exec(c.Request.Context(), `
		UPDATE media_nodes
		   SET cpu_cores         = COALESCE(NULLIF($2, 0), cpu_cores),
		       ram_gb            = COALESCE(NULLIF($3, 0), ram_gb),
		       rtpengine_version = COALESCE(NULLIF($4, ''), rtpengine_version),
		       agent_version     = COALESCE(NULLIF($5, ''), agent_version),
		       last_seen_at      = now(),
		       status            = CASE WHEN status = 'draining' THEN 'draining' ELSE 'online' END
		 WHERE id = $1
	`, nodeID, req.Cores, req.RAMMB/1024, req.RTPEngineVersion, req.AgentVersion); err != nil {
		slog.Error("agent register: update media_nodes failed", "node_id", nodeID, "err", err)
	}

	expected, err := s.expectedIPs(c.Request.Context(), nodeID, role)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, agentDirectiveResponse{NodeID: nodeID, Role: role, ExpectedIPs: expected})
}

type agentHeartbeatRequest struct {
	BoundIPs       []string `json:"bound_ips"`
	ActiveCalls    int      `json:"active_calls"`
	CPUPct         float64  `json:"cpu_pct"`
	RAMPct         float64  `json:"ram_pct"`
	NetInMbps      float64  `json:"net_in_mbps"`
	NetOutMbps     float64  `json:"net_out_mbps"`
	PacketLossPct  float64  `json:"packet_loss_pct"`
	UptimeSeconds  int64    `json:"uptime_seconds"`
	AgentVersion   string   `json:"agent_version"`
}

type agentCommand struct {
	ID      string          `json:"id"`
	Type    string          `json:"type"`
	IP      string          `json:"ip,omitempty"`
	CIDR    int             `json:"cidr,omitempty"`
	Iface   string          `json:"iface,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

type agentHeartbeatResponse struct {
	ExpectedIPs []string       `json:"expected_ips"`
	Commands    []agentCommand `json:"commands"`
}

func (s *Server) agentHeartbeat(c *gin.Context) {
	nodeID := c.GetInt64("agent_node_id")
	role := c.GetString("agent_node_role")

	var req agentHeartbeatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		slog.Warn("agent heartbeat: body bind failed", "node_id", nodeID, "err", err)
	}

	// Defensive dedup: older agents (or weird kernel/cloud-init setups)
	// can report the same IP twice in bound_ips. Track unique entries so
	// counts and downstream INSERTs are honest.
	seen := map[string]struct{}{}
	uniqIPs := make([]string, 0, len(req.BoundIPs))
	for _, ip := range req.BoundIPs {
		if ip == "" {
			continue
		}
		if _, dup := seen[ip]; dup {
			continue
		}
		seen[ip] = struct{}{}
		uniqIPs = append(uniqIPs, ip)
	}
	req.BoundIPs = uniqIPs

	// Update the latest-snapshot columns + bump last_seen_at.
	if _, err := s.deps.PG.Exec(c.Request.Context(), `
		UPDATE media_nodes
		   SET last_seen_at    = now(),
		       status          = CASE WHEN status = 'draining' THEN 'draining' ELSE 'online' END,
		       active_calls    = $2,
		       cpu_pct         = $3,
		       ram_pct         = $4,
		       net_in_mbps     = $5,
		       net_out_mbps    = $6,
		       packet_loss_pct = $7,
		       uptime_seconds  = NULLIF($8, 0),
		       agent_version   = COALESCE(NULLIF($9, ''), agent_version),
		       ips_bound       = $10
		 WHERE id = $1
	`,
		nodeID, req.ActiveCalls, req.CPUPct, req.RAMPct,
		req.NetInMbps, req.NetOutMbps, req.PacketLossPct,
		req.UptimeSeconds, req.AgentVersion, len(req.BoundIPs),
	); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Append a time-series row.
	if _, err := s.deps.PG.Exec(c.Request.Context(), `
		INSERT INTO node_metrics (node_id, active_calls, cpu_pct, ram_pct,
		                          net_in_mbps, net_out_mbps, packet_loss_pct)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, nodeID, req.ActiveCalls, req.CPUPct, req.RAMPct, req.NetInMbps, req.NetOutMbps, req.PacketLossPct); err != nil {
		slog.Error("heartbeat: append node_metrics failed", "node_id", nodeID, "err", err)
	}

	// Auto-discover every IP reported by the agent. The operator can later
	// disable IPs they don't want to use (e.g. the host's management IP)
	// via the Signaling IPs / IP Pool pages — but we don't second-guess
	// what's bound on the NIC at discovery time.
	switch role {
	case "media":
		for _, ip := range req.BoundIPs {
			if _, err := s.deps.PG.Exec(c.Request.Context(), `
				INSERT INTO node_ips (node_id, ip_address, status, auto_discovered)
				VALUES ($1, $2::inet, 'active', true)
				ON CONFLICT (ip_address) DO UPDATE
				   SET node_id = EXCLUDED.node_id,
				       last_health_check = now()
			`, nodeID, ip); err != nil {
				slog.Error("heartbeat: upsert node_ips failed", "node_id", nodeID, "ip", ip, "err", err)
			}
		}
	case "sip_proxy":
		for _, ip := range req.BoundIPs {
			if _, err := s.deps.PG.Exec(c.Request.Context(), `
				INSERT INTO signaling_ips (ip_address, sip_proxy_node_id, status, auto_discovered)
				VALUES ($2::inet, $1, 'available', true)
				ON CONFLICT (ip_address) DO UPDATE
				   SET sip_proxy_node_id = EXCLUDED.sip_proxy_node_id
			`, nodeID, ip); err != nil {
				slog.Error("heartbeat: upsert signaling_ips failed", "node_id", nodeID, "ip", ip, "err", err)
			}
		}
	}

	// Touch last_health_check on IPs the agent is currently binding.
	if _, err := s.deps.PG.Exec(c.Request.Context(),
		`UPDATE node_ips SET last_health_check = now() WHERE node_id = $1`, nodeID); err != nil {
		slog.Error("heartbeat: touch node_ips.last_health_check failed", "node_id", nodeID, "err", err)
	}

	// Auto-create the default IP group for this node on first IP-discovery
	// so the operator never has to "wire IPs into a pool" by hand for the
	// common case. Skips silently if the group already exists or there's
	// nothing to add. Cheap (one COALESCE INSERT + one conditional INSERT).
	// Only meaningful on media-role nodes.
	if role == "media" && len(req.BoundIPs) > 0 {
		if err := s.ensureDefaultGroupForNode(c.Request.Context(), nodeID); err != nil {
			slog.Warn("heartbeat: ensure default IP group failed", "node_id", nodeID, "err", err)
		}
	}

	expected, err := s.expectedIPs(c.Request.Context(), nodeID, role)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Pull any queued commands for this node and flip them to 'sent'.
	commands := []agentCommand{}
	if rows, err := s.deps.PG.Query(c.Request.Context(), `
		UPDATE node_commands
		   SET status = 'sent', sent_at = now()
		 WHERE id IN (
		   SELECT id FROM node_commands
		    WHERE node_id = $1 AND status = 'queued'
		    ORDER BY id LIMIT 20
		 )
		 RETURNING id, type, payload::text
	`, nodeID); err == nil {
		defer rows.Close()
		for rows.Next() {
			var (
				cid     int64
				ctype   string
				payload string
			)
			if err := rows.Scan(&cid, &ctype, &payload); err != nil {
				continue
			}
			cmd := agentCommand{ID: strconv.FormatInt(cid, 10), Type: ctype}
			if payload != "" && payload != "{}" {
				cmd.Payload = json.RawMessage(payload)
				// Convenience-extract the common shape so existing command
				// handlers (add_ip / remove_ip) still see populated fields.
				var p struct {
					IP    string `json:"ip"`
					CIDR  int    `json:"cidr"`
					Iface string `json:"iface"`
				}
				_ = json.Unmarshal([]byte(payload), &p)
				cmd.IP = p.IP
				cmd.CIDR = p.CIDR
				cmd.Iface = p.Iface
			}
			commands = append(commands, cmd)
		}
	}

	c.JSON(http.StatusOK, agentHeartbeatResponse{ExpectedIPs: expected, Commands: commands})
}

type agentCommandResultRequest struct {
	CommandID string `json:"command_id" binding:"required"`
	Status    string `json:"status" binding:"required,oneof=ok error"`
	Detail    string `json:"detail"`
}

func (s *Server) agentCommandResult(c *gin.Context) {
	nodeID := c.GetInt64("agent_node_id")
	var req agentCommandResultRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	cmdID, err := strconv.ParseInt(req.CommandID, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad command_id"})
		return
	}
	finalStatus := "done"
	if req.Status == "error" {
		finalStatus = "error"
	}
	if _, err := s.deps.PG.Exec(c.Request.Context(), `
		UPDATE node_commands
		   SET status = $3, detail = NULLIF($4, ''), completed_at = now()
		 WHERE id = $1 AND node_id = $2
	`, cmdID, nodeID, finalStatus, req.Detail); err != nil {
		slog.Error("command-result: update node_commands failed", "cmd_id", cmdID, "node_id", nodeID, "err", err)
	}
	c.Status(http.StatusNoContent)
}

func (s *Server) expectedIPs(ctx context.Context, nodeID int64, role string) ([]string, error) {
	var query string
	switch role {
	case "sip_proxy":
		query = `SELECT host(ip_address) FROM signaling_ips
		          WHERE sip_proxy_node_id = $1 AND status != 'disabled'
		          ORDER BY ip_address`
	default:
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
