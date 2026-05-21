package api

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

type NodeIP struct {
	ID              int64      `json:"id"`
	NodeID          int64      `json:"node_id"`
	IPAddress       string     `json:"ip_address"`
	Status          string     `json:"status"`
	PurchasedFrom   *string    `json:"purchased_from,omitempty"`
	LeaseBlock      *string    `json:"lease_block,omitempty"`
	LeaseExpires    *time.Time `json:"lease_expires,omitempty"`
	MonthlyCost     *float64   `json:"monthly_cost,omitempty"`
	Rdns            *string    `json:"rdns,omitempty"`
	ReputationScore *int       `json:"reputation_score,omitempty"`
	CurrentCalls    int        `json:"current_calls"`
	MaxCalls        int        `json:"max_calls"`
	AutoDiscovered  bool       `json:"auto_discovered"`
	CreatedAt       time.Time  `json:"created_at"`
}

func (s *Server) listNodeIPs(c *gin.Context) {
	var (
		rows pgx.Rows
		err  error
	)
	if q := c.Query("node_id"); q != "" {
		nodeID, perr := strconv.ParseInt(q, 10, 64)
		if perr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "bad node_id"})
			return
		}
		rows, err = s.deps.PG.Query(c.Request.Context(), `
			SELECT id, node_id, host(ip_address), status, purchased_from, lease_block,
			       lease_expires, monthly_cost, rdns, reputation_score, current_calls,
			       max_calls, auto_discovered, created_at
			  FROM node_ips WHERE node_id = $1 ORDER BY ip_address
		`, nodeID)
	} else {
		rows, err = s.deps.PG.Query(c.Request.Context(), `
			SELECT id, node_id, host(ip_address), status, purchased_from, lease_block,
			       lease_expires, monthly_cost, rdns, reputation_score, current_calls,
			       max_calls, auto_discovered, created_at
			  FROM node_ips ORDER BY node_id, ip_address
		`)
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	out := []NodeIP{}
	for rows.Next() {
		var n NodeIP
		if err := rows.Scan(&n.ID, &n.NodeID, &n.IPAddress, &n.Status,
			&n.PurchasedFrom, &n.LeaseBlock, &n.LeaseExpires, &n.MonthlyCost,
			&n.Rdns, &n.ReputationScore, &n.CurrentCalls, &n.MaxCalls,
			&n.AutoDiscovered, &n.CreatedAt); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		out = append(out, n)
	}
	c.JSON(http.StatusOK, out)
}

type createNodeIPRequest struct {
	NodeID        int64    `json:"node_id" binding:"required,gt=0"`
	IPAddress     string   `json:"ip_address" binding:"required,ip"`
	PurchasedFrom string   `json:"purchased_from"`
	LeaseBlock    string   `json:"lease_block"`
	MonthlyCost   *float64 `json:"monthly_cost"`
}

func (s *Server) createNodeIP(c *gin.Context) {
	var req createNodeIPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var purchasedFrom, leaseBlock *string
	if req.PurchasedFrom != "" {
		purchasedFrom = &req.PurchasedFrom
	}
	if req.LeaseBlock != "" {
		leaseBlock = &req.LeaseBlock
	}
	var n NodeIP
	err := s.deps.PG.QueryRow(c.Request.Context(), `
		INSERT INTO node_ips (node_id, ip_address, status, purchased_from, lease_block, monthly_cost)
		VALUES ($1, $2::inet, 'active', $3, $4, $5)
		RETURNING id, node_id, host(ip_address), status, purchased_from, lease_block,
		          lease_expires, monthly_cost, rdns, reputation_score, current_calls,
		          auto_discovered, created_at
	`, req.NodeID, req.IPAddress, purchasedFrom, leaseBlock, req.MonthlyCost).Scan(
		&n.ID, &n.NodeID, &n.IPAddress, &n.Status, &n.PurchasedFrom, &n.LeaseBlock,
		&n.LeaseExpires, &n.MonthlyCost, &n.Rdns, &n.ReputationScore, &n.CurrentCalls,
		&n.MaxCalls, &n.AutoDiscovered, &n.CreatedAt,
	)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, n)
}

type bulkNodeIPRequest struct {
	NodeID        int64    `json:"node_id" binding:"required,gt=0"`
	CIDR          string   `json:"cidr"` // e.g. 192.0.2.0/28 — expands to every usable host
	IPs           []string `json:"ips"`  // OR an explicit list
	PurchasedFrom string   `json:"purchased_from"`
	LeaseBlock    string   `json:"lease_block"`
}

// POST /api/v1/node-ips/bulk — supports either CIDR (192.0.2.0/28 → 16 IPs)
// or an explicit list. Duplicates are silently skipped.
func (s *Server) bulkCreateNodeIPs(c *gin.Context) {
	var req bulkNodeIPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	addrs := req.IPs
	if req.CIDR != "" {
		expanded, err := expandCIDR(req.CIDR)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		addrs = append(addrs, expanded...)
	}
	if len(addrs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "either cidr or ips required"})
		return
	}

	var purchasedFrom, leaseBlock *string
	if req.PurchasedFrom != "" {
		purchasedFrom = &req.PurchasedFrom
	}
	if req.LeaseBlock != "" {
		leaseBlock = &req.LeaseBlock
	}

	created := 0
	skipped := 0
	for _, ip := range addrs {
		tag, err := s.deps.PG.Exec(c.Request.Context(), `
			INSERT INTO node_ips (node_id, ip_address, status, purchased_from, lease_block)
			VALUES ($1, $2::inet, 'active', $3, $4)
			ON CONFLICT (ip_address) DO NOTHING
		`, req.NodeID, ip, purchasedFrom, leaseBlock)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "ip": ip})
			return
		}
		if tag.RowsAffected() == 1 {
			created++
		} else {
			skipped++
		}
	}
	// Auto-create a default IP group for this node if one doesn't already
	// exist, and add every active IP that isn't yet in any active group.
	// Saves the operator a 4-click workflow after a bulk add: "create
	// group → name it → pick members → save".
	if created > 0 {
		if err := s.ensureDefaultGroupForNode(c.Request.Context(), req.NodeID); err != nil {
			// Soft-fail: the IPs are in, the operator can just create a
			// group manually if this didn't fire. Don't reject the bulk
			// import for this.
			c.JSON(http.StatusOK, gin.H{
				"created": created, "skipped": skipped, "total": len(addrs),
				"group_warning": err.Error(),
			})
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"created": created, "skipped": skipped, "total": len(addrs)})
}

// ensureDefaultGroupForNode is idempotent: if a group named "<node> default"
// already exists, reuse it; otherwise create one. Then mark every active
// node_ip for this node that isn't yet a member of any active group as a
// member of the default group.
//
// Naming convention: "<NodeName> default" — visible in the UI and easy to
// rename. Operators are free to create additional groups for finer slicing
// (region, transcoding tier, etc) and reassign IPs between them; this
// function only fills the empty case.
func (s *Server) ensureDefaultGroupForNode(ctx context.Context, nodeID int64) error {
	var nodeName string
	if err := s.deps.PG.QueryRow(ctx,
		`SELECT name FROM media_nodes WHERE id = $1`, nodeID,
	).Scan(&nodeName); err != nil {
		return fmt.Errorf("lookup node: %w", err)
	}
	groupName := nodeName + " default"

	var groupID int64
	err := s.deps.PG.QueryRow(ctx, `
		INSERT INTO ip_groups (name, status, notes)
		VALUES ($1, 'active', 'Auto-created default pool for ' || $1)
		ON CONFLICT (name) DO UPDATE SET name = EXCLUDED.name
		RETURNING id
	`, groupName).Scan(&groupID)
	if err != nil {
		return fmt.Errorf("ensure group: %w", err)
	}

	// Add every active IP on this node that isn't yet an active member of
	// any group. WHERE NOT EXISTS lets us run this idempotently without
	// duplicating memberships.
	if _, err := s.deps.PG.Exec(ctx, `
		INSERT INTO ip_group_members (group_id, ip_id, active)
		SELECT $1, ni.id, true
		  FROM node_ips ni
		 WHERE ni.node_id = $2
		   AND ni.status = 'active'
		   AND NOT EXISTS (
		       SELECT 1 FROM ip_group_members m
		        WHERE m.ip_id = ni.id AND m.active = true
		   )
	`, groupID, nodeID); err != nil {
		return fmt.Errorf("seed group members: %w", err)
	}
	return nil
}

type patchNodeIPRequest struct {
	Status        *string  `json:"status"`
	PurchasedFrom *string  `json:"purchased_from"`
	LeaseBlock    *string  `json:"lease_block"`
	MonthlyCost   *float64 `json:"monthly_cost"`
	Rdns          *string  `json:"rdns"`
	MaxCalls      *int     `json:"max_calls"`
}

func (s *Server) patchNodeIP(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad id"})
		return
	}
	var req patchNodeIPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Status != nil {
		switch *req.Status {
		case "active", "disabled", "flagged", "reserve":
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid status"})
			return
		}
	}
	tag, err := s.deps.PG.Exec(c.Request.Context(), `
		UPDATE node_ips SET
		   status         = COALESCE($2, status),
		   purchased_from = COALESCE($3, purchased_from),
		   lease_block    = COALESCE($4, lease_block),
		   monthly_cost   = COALESCE($5, monthly_cost),
		   rdns           = COALESCE($6, rdns),
		   max_calls      = COALESCE($7, max_calls)
		 WHERE id = $1
	`, id, req.Status, req.PurchasedFrom, req.LeaseBlock, req.MonthlyCost, req.Rdns, req.MaxCalls)
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

// POST /api/v1/node-ips/bulk-update
//
// Apply the same set of fields to many node_ips rows in one shot. Used by
// the IP Pool page's "Bulk apply" button (operators routinely want to set
// max_calls or status across all 60+ IPs on a freshly-imported block).
//
// Filters narrow the affected rows. node_id is required (you can't bulk-
// update across multiple nodes — different nodes belong to different
// operators/regions). status_filter optionally restricts to one state.
//
// At least one of max_calls / new_status must be set or we return 400
// (otherwise it's a no-op and the operator likely meant to do something).
type bulkUpdateNodeIPsRequest struct {
	NodeID       int64   `json:"node_id" binding:"required,gt=0"`
	StatusFilter string  `json:"status_filter"` // "" = all states
	MaxCalls     *int    `json:"max_calls"`
	NewStatus    *string `json:"new_status"`
}

func (s *Server) bulkUpdateNodeIPs(c *gin.Context) {
	var req bulkUpdateNodeIPsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.MaxCalls == nil && req.NewStatus == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "specify at least one of max_calls or new_status"})
		return
	}
	if req.MaxCalls != nil && *req.MaxCalls < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "max_calls must be >= 0"})
		return
	}
	if req.NewStatus != nil {
		switch *req.NewStatus {
		case "active", "disabled", "flagged", "reserve":
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": "new_status must be active/reserve/flagged/disabled"})
			return
		}
	}
	if req.StatusFilter != "" {
		switch req.StatusFilter {
		case "active", "disabled", "flagged", "reserve":
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": "status_filter must be active/reserve/flagged/disabled or omitted"})
			return
		}
	}

	// We always want the same UPDATE shape regardless of which fields the
	// caller wants to change. COALESCE(NULLIF(...), col) gives "set if
	// provided, otherwise leave alone" for each column.
	var statusFilter *string
	if req.StatusFilter != "" {
		statusFilter = &req.StatusFilter
	}
	tag, err := s.deps.PG.Exec(c.Request.Context(), `
		UPDATE node_ips
		   SET max_calls = COALESCE($3, max_calls),
		       status    = COALESCE($4, status)
		 WHERE node_id   = $1
		   AND ($2::text IS NULL OR status = $2)
	`, req.NodeID, statusFilter, req.MaxCalls, req.NewStatus)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"updated": tag.RowsAffected()})
}

func (s *Server) deleteNodeIP(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad id"})
		return
	}
	tag, err := s.deps.PG.Exec(c.Request.Context(),
		`DELETE FROM node_ips WHERE id = $1`, id)
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

// expandCIDR returns every usable host IP in a CIDR block.
// For /24 it skips the network and broadcast addresses; for /31 and /32 it
// returns the literal addresses.
func expandCIDR(cidr string) ([]string, error) {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, err
	}
	ones, bits := ipNet.Mask.Size()
	if bits != 32 {
		return nil, &cidrErr{"only IPv4 CIDRs are supported"}
	}
	if ones >= 31 {
		// /31 (2 addrs, both usable per RFC 3021) or /32 (1 addr)
		ip := ipNet.IP.Mask(ipNet.Mask)
		out := []string{ip.String()}
		if ones == 31 {
			ip2 := nextIP(ip)
			out = append(out, ip2.String())
		}
		return out, nil
	}
	if ones < 16 {
		// Stop people from accidentally creating millions of rows.
		return nil, &cidrErr{"CIDR too large; use /16 or smaller"}
	}
	first := ipNet.IP.Mask(ipNet.Mask)
	last := make(net.IP, len(first))
	copy(last, first)
	for i := range last {
		last[i] |= ^ipNet.Mask[i]
	}
	out := []string{}
	cur := nextIP(first) // skip network address
	for !cur.Equal(last) {
		out = append(out, cur.String())
		cur = nextIP(cur)
	}
	return out, nil
}

func nextIP(ip net.IP) net.IP {
	n := make(net.IP, len(ip))
	copy(n, ip)
	for i := len(n) - 1; i >= 0; i-- {
		n[i]++
		if n[i] != 0 {
			break
		}
	}
	return n
}

type cidrErr struct{ msg string }

func (e *cidrErr) Error() string { return e.msg }
