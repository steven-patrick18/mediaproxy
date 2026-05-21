package api

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

// MediaNode is the list-and-create response shape. ListNodes also returns
// the latest snapshot of metrics so the UI doesn't need a separate call.
type MediaNode struct {
	ID                 int64      `json:"id"`
	Name               string     `json:"name"`
	Role               string     `json:"role"`
	HostIP             string     `json:"host_ip"`
	Region             *string    `json:"region,omitempty"`
	NicGbps            *int       `json:"nic_gbps,omitempty"`
	MaxCalls           int        `json:"max_calls"`
	TranscodingEnabled bool       `json:"transcoding_enabled"`
	Status             string     `json:"status"`
	AgentToken         string     `json:"agent_token,omitempty"`
	LastSeenAt         *time.Time `json:"last_seen_at,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	// Latest snapshot from heartbeat
	ActiveCalls    *int     `json:"active_calls,omitempty"`
	CPUPct         *float64 `json:"cpu_pct,omitempty"`
	RAMPct         *float64 `json:"ram_pct,omitempty"`
	NetInMbps      *float64 `json:"net_in_mbps,omitempty"`
	NetOutMbps     *float64 `json:"net_out_mbps,omitempty"`
	PacketLossPct  *float64 `json:"packet_loss_pct,omitempty"`
	UptimeSeconds  *int64   `json:"uptime_seconds,omitempty"`
	AgentVersion   *string  `json:"agent_version,omitempty"`
	IPsBound       int      `json:"ips_bound"`
	IPsTotal       int      `json:"ips_total"`
	FirewallAppliedAt *time.Time `json:"firewall_applied_at,omitempty"`
	SSHAuthMethod  string   `json:"ssh_auth_method"`
}

// computedStatus flips a node to "offline" if last_seen_at is older than
// staleAfter (in SQL). Draining stays draining (operator-driven). Without
// a last_seen_at row the node is offline.
const nodeStatusExpr = `
	CASE
	  WHEN n.status = 'draining' AND n.last_seen_at >= now() - interval '2 minutes' THEN 'draining'
	  WHEN n.last_seen_at IS NULL OR n.last_seen_at < now() - interval '2 minutes' THEN 'offline'
	  ELSE n.status
	END
`

func (s *Server) listNodes(c *gin.Context) {
	// active_calls is computed live from the active_calls table:
	//   - sip_proxy: count rows whose node_id matches (the SipProxy is
	//     the agent that posted /call-start so all rows it spawned carry
	//     its node_id).
	//   - media: count rows whose media_ip belongs to this media node
	//     (via node_ips.ip_address). The active_calls.node_id column
	//     reports the SIGNALING node, not the media-handling node, so a
	//     plain node_id match would always return 0 for media nodes.
	// The agent's rtpengine session-count path is separately broken
	// (parse keys mismatch), so we ignore the stale column entirely.
	q := `
		SELECT n.id, n.name, n.role, host(n.host_ip), n.region, n.nic_gbps, n.max_calls,
		       n.transcoding_enabled, ` + nodeStatusExpr + ` AS effective_status,
		       n.last_seen_at, n.created_at,
		       CASE
		         WHEN n.role = 'media' THEN
		           (SELECT COUNT(*) FROM active_calls a
		             JOIN node_ips ni ON ni.ip_address = a.media_ip
		            WHERE ni.node_id = n.id)
		         ELSE
		           (SELECT COUNT(*) FROM active_calls WHERE node_id = n.id)
		       END::int AS active_calls,
		       n.cpu_pct, n.ram_pct,
		       n.net_in_mbps, n.net_out_mbps, n.packet_loss_pct,
		       n.uptime_seconds, n.agent_version,
		       COALESCE(n.ips_bound, 0),
		       (SELECT count(*) FROM node_ips      WHERE node_id = n.id) +
		       (SELECT count(*) FROM signaling_ips WHERE sip_proxy_node_id = n.id) AS ips_total,
		       n.firewall_applied_at, n.ssh_auth_method
		  FROM media_nodes n
		 ORDER BY n.id
	`
	rows, err := s.deps.PG.Query(c.Request.Context(), q)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	out := []MediaNode{}
	for rows.Next() {
		var n MediaNode
		if err := rows.Scan(&n.ID, &n.Name, &n.Role, &n.HostIP, &n.Region, &n.NicGbps,
			&n.MaxCalls, &n.TranscodingEnabled, &n.Status, &n.LastSeenAt, &n.CreatedAt,
			&n.ActiveCalls, &n.CPUPct, &n.RAMPct, &n.NetInMbps, &n.NetOutMbps, &n.PacketLossPct,
			&n.UptimeSeconds, &n.AgentVersion, &n.IPsBound, &n.IPsTotal, &n.FirewallAppliedAt,
			&n.SSHAuthMethod); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		out = append(out, n)
	}
	c.JSON(http.StatusOK, out)
}

type createNodeRequest struct {
	Name               string `json:"name" binding:"required,min=1,max=64"`
	Role               string `json:"role" binding:"required,oneof=media sip_proxy"`
	HostIP             string `json:"host_ip" binding:"required,ip"`
	Region             string `json:"region"`
	NicGbps            int    `json:"nic_gbps" binding:"gte=0"`
	MaxCalls           int    `json:"max_calls" binding:"gte=0"`
	TranscodingEnabled bool   `json:"transcoding_enabled"`
	SSHAuthMethod      string `json:"ssh_auth_method" binding:"omitempty,oneof=password key"`
}

func (s *Server) createNode(c *gin.Context) {
	var req createNodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	token, err := randomToken(32)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "token gen failed"})
		return
	}
	var node MediaNode
	var region *string
	if req.Region != "" {
		region = &req.Region
	}
	var nicGbps *int
	if req.NicGbps > 0 {
		nicGbps = &req.NicGbps
	}
	authMethod := req.SSHAuthMethod
	if authMethod == "" {
		authMethod = "password"
	}
	err = s.deps.PG.QueryRow(c.Request.Context(), `
		INSERT INTO media_nodes (name, role, host_ip, region, nic_gbps, max_calls, transcoding_enabled, agent_token, status, ssh_auth_method)
		VALUES ($1, $2, $3::inet, $4, $5, $6, $7, $8, 'offline', $9)
		RETURNING id, name, role, host(host_ip), region, nic_gbps, max_calls, transcoding_enabled,
		          status, agent_token, last_seen_at, created_at, ssh_auth_method
	`, req.Name, req.Role, req.HostIP, region, nicGbps, req.MaxCalls, req.TranscodingEnabled, token, authMethod).Scan(
		&node.ID, &node.Name, &node.Role, &node.HostIP, &node.Region, &node.NicGbps,
		&node.MaxCalls, &node.TranscodingEnabled, &node.Status, &node.AgentToken,
		&node.LastSeenAt, &node.CreatedAt, &node.SSHAuthMethod,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "insert returned no row"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, node)
}

type patchNodeRequest struct {
	Name               *string `json:"name"`
	Region             *string `json:"region"`
	NicGbps            *int    `json:"nic_gbps"`
	MaxCalls           *int    `json:"max_calls"`
	TranscodingEnabled *bool   `json:"transcoding_enabled"`
	SSHAuthMethod      *string `json:"ssh_auth_method"`
}

func (s *Server) patchNode(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad id"})
		return
	}
	var req patchNodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.SSHAuthMethod != nil {
		switch *req.SSHAuthMethod {
		case "password", "key":
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": "ssh_auth_method must be 'password' or 'key'"})
			return
		}
	}
	tag, err := s.deps.PG.Exec(c.Request.Context(), `
		UPDATE media_nodes
		   SET name                = COALESCE($2, name),
		       region              = COALESCE($3, region),
		       nic_gbps            = COALESCE($4, nic_gbps),
		       max_calls           = COALESCE($5, max_calls),
		       transcoding_enabled = COALESCE($6, transcoding_enabled),
		       ssh_auth_method     = COALESCE($7, ssh_auth_method)
		 WHERE id = $1
	`, id, req.Name, req.Region, req.NicGbps, req.MaxCalls, req.TranscodingEnabled, req.SSHAuthMethod)
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

func (s *Server) drainNode(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad id"})
		return
	}
	tag, err := s.deps.PG.Exec(c.Request.Context(),
		`UPDATE media_nodes SET status = 'draining' WHERE id = $1`, id)
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

func (s *Server) undrainNode(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad id"})
		return
	}
	// agent will flip it back to 'online' on next heartbeat
	tag, err := s.deps.PG.Exec(c.Request.Context(),
		`UPDATE media_nodes SET status = 'offline' WHERE id = $1 AND status = 'draining'`, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if tag.RowsAffected() == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found or not draining"})
		return
	}
	c.Status(http.StatusNoContent)
}

func (s *Server) deleteNode(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad id"})
		return
	}
	// Guard: refuse if IPs or signaling IPs still live here.
	var n int
	if err := s.deps.PG.QueryRow(c.Request.Context(),
		`SELECT (SELECT count(*) FROM node_ips WHERE node_id = $1) +
		        (SELECT count(*) FROM signaling_ips WHERE sip_proxy_node_id = $1)`, id).Scan(&n); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if n > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "node still owns IPs; remove them first"})
		return
	}
	tag, err := s.deps.PG.Exec(c.Request.Context(), `DELETE FROM media_nodes WHERE id = $1`, id)
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

type MetricPoint struct {
	Ts             time.Time `json:"ts"`
	ActiveCalls    *int      `json:"active_calls,omitempty"`
	CPUPct         *float64  `json:"cpu_pct,omitempty"`
	RAMPct         *float64  `json:"ram_pct,omitempty"`
	NetInMbps      *float64  `json:"net_in_mbps,omitempty"`
	NetOutMbps     *float64  `json:"net_out_mbps,omitempty"`
	PacketLossPct  *float64  `json:"packet_loss_pct,omitempty"`
}

// GET /api/v1/nodes/:id/metrics?minutes=60
func (s *Server) nodeMetrics(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad id"})
		return
	}
	minutes := 60
	if q := c.Query("minutes"); q != "" {
		if v, err := strconv.Atoi(q); err == nil && v > 0 && v <= 24*60 {
			minutes = v
		}
	}
	rows, err := s.deps.PG.Query(c.Request.Context(), `
		SELECT ts, active_calls, cpu_pct, ram_pct, net_in_mbps, net_out_mbps, packet_loss_pct
		  FROM node_metrics
		 WHERE node_id = $1 AND ts >= now() - ($2 * interval '1 minute')
		 ORDER BY ts
	`, id, minutes)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	out := []MetricPoint{}
	for rows.Next() {
		var p MetricPoint
		if err := rows.Scan(&p.Ts, &p.ActiveCalls, &p.CPUPct, &p.RAMPct,
			&p.NetInMbps, &p.NetOutMbps, &p.PacketLossPct); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		out = append(out, p)
	}
	c.JSON(http.StatusOK, out)
}

func randomToken(nBytes int) (string, error) {
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
