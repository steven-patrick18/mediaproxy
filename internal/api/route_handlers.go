package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

type Route struct {
	ID          int64   `json:"id"`
	ClientID    int64   `json:"client_id"`
	MatchPrefix *string `json:"match_prefix,omitempty"`
	CarrierID   int64   `json:"carrier_id"`
	Priority    int     `json:"priority"`
	Status      string  `json:"status"`
}

func (s *Server) listRoutes(c *gin.Context) {
	var (
		rows pgx.Rows
		err  error
	)
	if q := c.Query("client_id"); q != "" {
		clientID, perr := strconv.ParseInt(q, 10, 64)
		if perr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "bad client_id"})
			return
		}
		rows, err = s.deps.PG.Query(c.Request.Context(), `
			SELECT id, client_id, match_prefix, carrier_id, priority, status
			  FROM routes WHERE client_id = $1 ORDER BY priority, id
		`, clientID)
	} else {
		rows, err = s.deps.PG.Query(c.Request.Context(), `
			SELECT id, client_id, match_prefix, carrier_id, priority, status
			  FROM routes ORDER BY client_id, priority, id
		`)
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	out := []Route{}
	for rows.Next() {
		var r Route
		if err := rows.Scan(&r.ID, &r.ClientID, &r.MatchPrefix, &r.CarrierID, &r.Priority, &r.Status); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		out = append(out, r)
	}
	c.JSON(http.StatusOK, out)
}

type createRouteRequest struct {
	ClientID    int64  `json:"client_id" binding:"required,gt=0"`
	MatchPrefix string `json:"match_prefix"`
	CarrierID   int64  `json:"carrier_id" binding:"required,gt=0"`
	Priority    int    `json:"priority"`
}

func (s *Server) createRoute(c *gin.Context) {
	var req createRouteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Priority == 0 {
		req.Priority = 100
	}
	var prefix *string
	if req.MatchPrefix != "" {
		prefix = &req.MatchPrefix
	}
	var r Route
	err := s.deps.PG.QueryRow(c.Request.Context(), `
		INSERT INTO routes (client_id, match_prefix, carrier_id, priority)
		VALUES ($1, $2, $3, $4)
		RETURNING id, client_id, match_prefix, carrier_id, priority, status
	`, req.ClientID, prefix, req.CarrierID, req.Priority).Scan(
		&r.ID, &r.ClientID, &r.MatchPrefix, &r.CarrierID, &r.Priority, &r.Status,
	)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, r)
}

type patchRouteRequest struct {
	MatchPrefix *string `json:"match_prefix"`
	CarrierID   *int64  `json:"carrier_id"`
	Priority    *int    `json:"priority"`
	Status      *string `json:"status"`
}

func (s *Server) patchRoute(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad id"})
		return
	}
	var req patchRouteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Status != nil {
		switch *req.Status {
		case "active", "disabled":
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid status"})
			return
		}
	}
	tag, err := s.deps.PG.Exec(c.Request.Context(), `
		UPDATE routes
		   SET match_prefix = COALESCE($2, match_prefix),
		       carrier_id   = COALESCE($3, carrier_id),
		       priority     = COALESCE($4, priority),
		       status       = COALESCE($5, status)
		 WHERE id = $1
	`, id, req.MatchPrefix, req.CarrierID, req.Priority, req.Status)
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

func (s *Server) deleteRoute(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad id"})
		return
	}
	tag, err := s.deps.PG.Exec(c.Request.Context(),
		`DELETE FROM routes WHERE id = $1`, id)
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
