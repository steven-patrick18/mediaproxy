package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

type IPGroup struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	Notes     *string   `json:"notes,omitempty"`
	CreatedBy *int64    `json:"created_by,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	IPCount   int       `json:"ip_count"`
}

type IPGroupMember struct {
	IPID      int64  `json:"ip_id"`
	IPAddress string `json:"ip_address"`
	NodeID    int64  `json:"node_id"`
	Active    bool   `json:"active"`
}

func (s *Server) listIPGroups(c *gin.Context) {
	rows, err := s.deps.PG.Query(c.Request.Context(), `
		SELECT g.id, g.name, g.status, g.notes, g.created_by, g.created_at,
		       COUNT(m.id) FILTER (WHERE m.active)
		  FROM ip_groups g
		  LEFT JOIN ip_group_members m ON m.group_id = g.id
		 GROUP BY g.id ORDER BY g.id
	`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	out := []IPGroup{}
	for rows.Next() {
		var g IPGroup
		if err := rows.Scan(&g.ID, &g.Name, &g.Status, &g.Notes, &g.CreatedBy, &g.CreatedAt, &g.IPCount); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		out = append(out, g)
	}
	c.JSON(http.StatusOK, out)
}

type createIPGroupRequest struct {
	Name  string  `json:"name" binding:"required,min=1,max=128"`
	Notes string  `json:"notes"`
	IPIDs []int64 `json:"ip_ids"`
}

func (s *Server) createIPGroup(c *gin.Context) {
	var req createIPGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	tx, err := s.deps.PG.Begin(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer tx.Rollback(c.Request.Context())

	actor, _ := c.Get("user_id")
	var notes *string
	if req.Notes != "" {
		notes = &req.Notes
	}
	var groupID int64
	if err := tx.QueryRow(c.Request.Context(), `
		INSERT INTO ip_groups (name, notes, created_by) VALUES ($1, $2, $3) RETURNING id
	`, req.Name, notes, actor).Scan(&groupID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	for _, ipID := range req.IPIDs {
		if _, err := tx.Exec(c.Request.Context(), `
			INSERT INTO ip_group_members (group_id, ip_id, active) VALUES ($1, $2, true)
		`, groupID, ipID); err != nil {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error(), "ip_id": ipID})
			return
		}
	}
	if err := tx.Commit(c.Request.Context()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": groupID, "members_added": len(req.IPIDs)})
}

func (s *Server) listIPGroupMembers(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad id"})
		return
	}
	rows, err := s.deps.PG.Query(c.Request.Context(), `
		SELECT m.ip_id, host(n.ip_address), n.node_id, m.active
		  FROM ip_group_members m JOIN node_ips n ON n.id = m.ip_id
		 WHERE m.group_id = $1 ORDER BY n.ip_address
	`, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	out := []IPGroupMember{}
	for rows.Next() {
		var m IPGroupMember
		if err := rows.Scan(&m.IPID, &m.IPAddress, &m.NodeID, &m.Active); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		out = append(out, m)
	}
	c.JSON(http.StatusOK, out)
}

type addGroupMemberRequest struct {
	IPID int64 `json:"ip_id" binding:"required,gt=0"`
}

func (s *Server) addIPGroupMember(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad id"})
		return
	}
	var req addGroupMemberRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if _, err := s.deps.PG.Exec(c.Request.Context(), `
		INSERT INTO ip_group_members (group_id, ip_id, active) VALUES ($1, $2, true)
	`, id, req.IPID); err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusCreated)
}

func (s *Server) removeIPGroupMember(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad id"})
		return
	}
	ipID, err := strconv.ParseInt(c.Param("ip_id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad ip_id"})
		return
	}
	tag, err := s.deps.PG.Exec(c.Request.Context(),
		`DELETE FROM ip_group_members WHERE group_id = $1 AND ip_id = $2`, id, ipID)
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

type patchIPGroupRequest struct {
	Name   *string `json:"name"`
	Notes  *string `json:"notes"`
	Status *string `json:"status"`
}

func (s *Server) patchIPGroup(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad id"})
		return
	}
	var req patchIPGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Status != nil {
		switch *req.Status {
		case "active", "paused", "ended":
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid status"})
			return
		}
	}
	tag, err := s.deps.PG.Exec(c.Request.Context(), `
		UPDATE ip_groups
		   SET name   = COALESCE($2, name),
		       notes  = COALESCE($3, notes),
		       status = COALESCE($4, status)
		 WHERE id = $1
	`, id, req.Name, req.Notes, req.Status)
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

func (s *Server) deleteIPGroup(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad id"})
		return
	}
	tag, err := s.deps.PG.Exec(c.Request.Context(), `DELETE FROM ip_groups WHERE id = $1`, id)
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
