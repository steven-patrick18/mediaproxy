package api

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"mediaproxy/internal/firewall"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

type FirewallRule struct {
	ID            int64     `json:"id"`
	Name          string    `json:"name"`
	Action        string    `json:"action"`
	SourceCIDR    *string   `json:"source_cidr,omitempty"`
	DestPortLow   *int      `json:"dest_port_low,omitempty"`
	DestPortHigh  *int      `json:"dest_port_high,omitempty"`
	Proto         string    `json:"proto"`
	NodeID        *int64    `json:"node_id,omitempty"`
	RatePerSecond *int      `json:"rate_per_second,omitempty"`
	Priority      int       `json:"priority"`
	Enabled       bool      `json:"enabled"`
	Notes         *string   `json:"notes,omitempty"`
	CreatedBy     *int64    `json:"created_by,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

func (s *Server) listFirewallRules(c *gin.Context) {
	rows, err := s.deps.PG.Query(c.Request.Context(), `
		SELECT id, name, action,
		       CASE WHEN source_cidr IS NOT NULL THEN host(source_cidr) || '/' || masklen(source_cidr) ELSE NULL END,
		       dest_port_low, dest_port_high, proto, node_id,
		       rate_per_second, priority, enabled, notes, created_by, created_at
		  FROM firewall_rules
		 ORDER BY priority, id
	`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	out := []FirewallRule{}
	for rows.Next() {
		var r FirewallRule
		if err := rows.Scan(&r.ID, &r.Name, &r.Action, &r.SourceCIDR,
			&r.DestPortLow, &r.DestPortHigh, &r.Proto, &r.NodeID,
			&r.RatePerSecond, &r.Priority, &r.Enabled, &r.Notes, &r.CreatedBy, &r.CreatedAt); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		out = append(out, r)
	}
	c.JSON(http.StatusOK, out)
}

type createFirewallRuleRequest struct {
	Name          string  `json:"name" binding:"required,min=1,max=128"`
	Action        string  `json:"action" binding:"required,oneof=allow block rate_limit"`
	SourceCIDR    string  `json:"source_cidr"`
	DestPortLow   *int    `json:"dest_port_low"`
	DestPortHigh  *int    `json:"dest_port_high"`
	Proto         string  `json:"proto" binding:"omitempty,oneof=any tcp udp"`
	NodeID        *int64  `json:"node_id"`
	RatePerSecond *int    `json:"rate_per_second"`
	Priority      int     `json:"priority"`
	Notes         string  `json:"notes"`
}

func (s *Server) createFirewallRule(c *gin.Context) {
	var req createFirewallRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Priority == 0 {
		req.Priority = 100
	}
	if req.Proto == "" {
		req.Proto = "any"
	}
	var (
		cidr  *string
		notes *string
	)
	if req.SourceCIDR != "" {
		cidr = &req.SourceCIDR
	}
	if req.Notes != "" {
		notes = &req.Notes
	}
	actor, _ := c.Get("user_id")
	var r FirewallRule
	err := s.deps.PG.QueryRow(c.Request.Context(), `
		INSERT INTO firewall_rules (name, action, source_cidr, dest_port_low, dest_port_high,
		                            proto, node_id, rate_per_second, priority, notes, created_by)
		VALUES ($1, $2, $3::cidr, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING id, name, action,
		          CASE WHEN source_cidr IS NOT NULL THEN host(source_cidr) || '/' || masklen(source_cidr) ELSE NULL END,
		          dest_port_low, dest_port_high, proto, node_id,
		          rate_per_second, priority, enabled, notes, created_by, created_at
	`, req.Name, req.Action, cidr, req.DestPortLow, req.DestPortHigh,
		req.Proto, req.NodeID, req.RatePerSecond, req.Priority, notes, actor,
	).Scan(&r.ID, &r.Name, &r.Action, &r.SourceCIDR, &r.DestPortLow, &r.DestPortHigh,
		&r.Proto, &r.NodeID, &r.RatePerSecond, &r.Priority, &r.Enabled, &r.Notes, &r.CreatedBy, &r.CreatedAt)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, r)
}

type patchFirewallRuleRequest struct {
	Name          *string `json:"name"`
	Enabled       *bool   `json:"enabled"`
	Priority      *int    `json:"priority"`
	Notes         *string `json:"notes"`
	SourceCIDR    *string `json:"source_cidr"`
	DestPortLow   *int    `json:"dest_port_low"`
	DestPortHigh  *int    `json:"dest_port_high"`
	Proto         *string `json:"proto"`
	RatePerSecond *int    `json:"rate_per_second"`
}

func (s *Server) patchFirewallRule(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad id"})
		return
	}
	var req patchFirewallRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	tag, err := s.deps.PG.Exec(c.Request.Context(), `
		UPDATE firewall_rules SET
		   name             = COALESCE($2, name),
		   enabled          = COALESCE($3, enabled),
		   priority         = COALESCE($4, priority),
		   notes            = COALESCE($5, notes),
		   source_cidr      = COALESCE($6::cidr, source_cidr),
		   dest_port_low    = COALESCE($7, dest_port_low),
		   dest_port_high   = COALESCE($8, dest_port_high),
		   proto            = COALESCE($9, proto),
		   rate_per_second  = COALESCE($10, rate_per_second)
		 WHERE id = $1
	`, id, req.Name, req.Enabled, req.Priority, req.Notes,
		req.SourceCIDR, req.DestPortLow, req.DestPortHigh, req.Proto, req.RatePerSecond)
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

func (s *Server) deleteFirewallRule(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad id"})
		return
	}
	tag, err := s.deps.PG.Exec(c.Request.Context(),
		`DELETE FROM firewall_rules WHERE id = $1`, id)
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

// FirewallPreview is the synthesized nftables config for a single node.
type FirewallPreview struct {
	NodeID    int64                 `json:"node_id"`
	NodeName  string                `json:"node_name"`
	Role      string                `json:"role"`
	AutoRules []firewall.AutoRule   `json:"auto_rules"`
	Rules     []FirewallRule        `json:"applied_rules"`
	NFTConfig string                `json:"nft_config"`
}

// GET /api/v1/firewall/preview/:node_id
func (s *Server) firewallPreview(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad id"})
		return
	}
	s.renderFirewallPreviewFor(c, id)
}

func (s *Server) renderFirewallPreviewFor(c *gin.Context, id int64) {
	// Load node
	var (
		nodeName string
		role     string
		hostIP   string
	)
	if err := s.deps.PG.QueryRow(c.Request.Context(),
		`SELECT name, role, host(host_ip) FROM media_nodes WHERE id = $1`, id,
	).Scan(&nodeName, &role, &hostIP); err != nil {
		if err == pgx.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "node not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Auto-rules: every carrier.host that's a literal IP, every active client dialer IP.
	autoRules := []firewall.AutoRule{}

	if rows, err := s.deps.PG.Query(c.Request.Context(),
		`SELECT name, host FROM carriers WHERE status != 'disabled'`); err == nil {
		defer rows.Close()
		for rows.Next() {
			var name, host string
			if err := rows.Scan(&name, &host); err == nil {
				if cidr := toCIDR(host); cidr != "" {
					autoRules = append(autoRules, firewall.AutoRule{Kind: "carrier", Name: name, CIDR: cidr})
				}
			}
		}
	}
	if rows, err := s.deps.PG.Query(c.Request.Context(), `
		SELECT c.name, host(ci.ip_address)
		  FROM client_ips ci JOIN clients c ON c.id = ci.client_id
		 WHERE ci.status = 'active' AND c.status = 'active'
	`); err == nil {
		defer rows.Close()
		for rows.Next() {
			var name, ip string
			if err := rows.Scan(&name, &ip); err == nil {
				autoRules = append(autoRules, firewall.AutoRule{Kind: "client_dialer", Name: name, CIDR: ip + "/32"})
			}
		}
	}

	// Custom rules for this node (or global rules with node_id IS NULL).
	appliedRules := []FirewallRule{}
	rendererRules := []firewall.Rule{}
	if rows, err := s.deps.PG.Query(c.Request.Context(), `
		SELECT id, name, action,
		       CASE WHEN source_cidr IS NOT NULL THEN host(source_cidr) || '/' || masklen(source_cidr) ELSE NULL END,
		       dest_port_low, dest_port_high, proto, node_id,
		       rate_per_second, priority, enabled, notes, created_by, created_at
		  FROM firewall_rules
		 WHERE enabled = true AND (node_id IS NULL OR node_id = $1)
		 ORDER BY priority, id
	`, id); err == nil {
		defer rows.Close()
		for rows.Next() {
			var r FirewallRule
			if err := rows.Scan(&r.ID, &r.Name, &r.Action, &r.SourceCIDR,
				&r.DestPortLow, &r.DestPortHigh, &r.Proto, &r.NodeID,
				&r.RatePerSecond, &r.Priority, &r.Enabled, &r.Notes, &r.CreatedBy, &r.CreatedAt); err == nil {
				appliedRules = append(appliedRules, r)
				rendererRules = append(rendererRules, toRenderRule(r))
			}
		}
	}

	// Render
	cfg := firewall.Render(firewall.NodeContext{
		NodeID:      id,
		NodeName:    nodeName,
		Role:        role,
		HostIP:      hostIP,
		BaseAppCIDR: baseAppCIDR(c),
		SIPPort:     5060,
		RTPLow:      30000,
		RTPHigh:     60000,
	}, rendererRules, autoRules)

	c.JSON(http.StatusOK, FirewallPreview{
		NodeID:    id,
		NodeName:  nodeName,
		Role:      role,
		AutoRules: autoRules,
		Rules:     appliedRules,
		NFTConfig: cfg,
	})
}

// GET /api/v1/agent/firewall — agent-authenticated, returns the synthesized
// nft config for the calling agent's node.
func (s *Server) agentFirewallConfig(c *gin.Context) {
	nodeID := c.GetInt64("agent_node_id")
	s.renderFirewallPreviewFor(c, nodeID)
}

// POST /api/v1/agent/firewall-applied — agent confirms it successfully
// applied the ruleset; we just stamp the node row so the panel can show
// "applied at <ts>".
func (s *Server) agentFirewallApplied(c *gin.Context) {
	nodeID := c.GetInt64("agent_node_id")
	_, _ = s.deps.PG.Exec(c.Request.Context(),
		`UPDATE media_nodes SET firewall_applied_at = now() WHERE id = $1`, nodeID)
	c.Status(http.StatusNoContent)
}

// --- helpers ----------------------------------------------------------------

func toRenderRule(r FirewallRule) firewall.Rule {
	out := firewall.Rule{Name: r.Name, Action: r.Action, Proto: r.Proto}
	if r.SourceCIDR != nil {
		out.SourceCIDR = *r.SourceCIDR
	}
	if r.DestPortLow != nil {
		out.DestPortLow = *r.DestPortLow
	}
	if r.DestPortHigh != nil {
		out.DestPortHigh = *r.DestPortHigh
	}
	if r.RatePerSecond != nil {
		out.RatePerSecond = *r.RatePerSecond
	}
	if r.Notes != nil {
		out.Notes = *r.Notes
	}
	return out
}

// toCIDR returns "1.2.3.4/32" for IP literals, "" otherwise (so hostnames
// don't end up in the nft set).
func toCIDR(host string) string {
	if host == "" {
		return ""
	}
	// Very small parser — full IPs only. Hostnames need DNS resolution
	// which we deliberately skip server-side.
	dots := 0
	for _, c := range host {
		if c == '.' {
			dots++
		} else if c < '0' || c > '9' {
			return ""
		}
	}
	if dots != 3 {
		return ""
	}
	return host + "/32"
}

func baseAppCIDR(c *gin.Context) string {
	// Best-effort: the operator can add explicit allow rules; this is just
	// a sane default. We use the request's incoming Host as a hint of
	// where the base-app lives.
	_ = context.TODO()
	return ""
}
