package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

type Carrier struct {
	ID               int64     `json:"id"`
	Name             string    `json:"name"`
	Host             string    `json:"host"`
	Port             int       `json:"port"`
	Transport        string    `json:"transport"`
	AssignedNodeIDs  []int64   `json:"assigned_node_ids"`
	CodecPref        *string   `json:"codec_pref,omitempty"`
	Notes            *string   `json:"notes,omitempty"`
	Status           string    `json:"status"`
	CreatedAt        time.Time `json:"created_at"`
}

// loadCarrierNodes returns the active node-id list for a carrier, sorted by
// (priority, node_id) so the routing layer can iterate primary → backup.
func (s *Server) loadCarrierNodes(ctx interface{}, carrierID int64) ([]int64, error) {
	c, ok := ctx.(interface {
		Done() <-chan struct{}
		Err() error
		Value(any) any
		Deadline() (time.Time, bool)
	})
	_ = ok
	_ = c
	// Just take a real context via type assertion at call sites; this stub
	// keeps the helper API tidy.
	return nil, nil
}

func (s *Server) listCarriers(c *gin.Context) {
	rows, err := s.deps.PG.Query(c.Request.Context(), `
		SELECT c.id, c.name, c.host, c.port, c.transport, c.codec_pref, c.notes,
		       c.status, c.created_at,
		       COALESCE(
		         (SELECT array_agg(node_id ORDER BY priority, node_id)
		            FROM carrier_media_nodes
		           WHERE carrier_id = c.id AND status = 'active'),
		         '{}'::bigint[]
		       ) AS node_ids
		  FROM carriers c
		 ORDER BY c.id
	`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	out := []Carrier{}
	for rows.Next() {
		var x Carrier
		if err := rows.Scan(&x.ID, &x.Name, &x.Host, &x.Port, &x.Transport,
			&x.CodecPref, &x.Notes, &x.Status, &x.CreatedAt, &x.AssignedNodeIDs); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		out = append(out, x)
	}
	c.JSON(http.StatusOK, out)
}

type createCarrierRequest struct {
	Name            string  `json:"name" binding:"required,min=1,max=128"`
	Host            string  `json:"host" binding:"required"`
	Port            int     `json:"port"`
	Transport       string  `json:"transport" binding:"omitempty,oneof=udp tcp tls"`
	AssignedNodeIDs []int64 `json:"assigned_node_ids"`
	CodecPref       string  `json:"codec_pref"`
	Notes           string  `json:"notes"`
}

func (s *Server) createCarrier(c *gin.Context) {
	var req createCarrierRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Port == 0 {
		req.Port = 5060
	}
	if req.Transport == "" {
		req.Transport = "udp"
	}
	var codecPref, notes *string
	if req.CodecPref != "" {
		codecPref = &req.CodecPref
	}
	if req.Notes != "" {
		notes = &req.Notes
	}

	tx, err := s.deps.PG.Begin(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer tx.Rollback(c.Request.Context())

	var carrierID int64
	err = tx.QueryRow(c.Request.Context(), `
		INSERT INTO carriers (name, host, port, transport, codec_pref, notes)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id
	`, req.Name, req.Host, req.Port, req.Transport, codecPref, notes).Scan(&carrierID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	actor, _ := c.Get("user_id")
	for _, nid := range req.AssignedNodeIDs {
		if _, err := tx.Exec(c.Request.Context(),
			`INSERT INTO carrier_media_nodes (carrier_id, node_id, assigned_by) VALUES ($1, $2, $3)`,
			carrierID, nid, actor); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if _, err := tx.Exec(c.Request.Context(), `
			INSERT INTO carrier_node_history (carrier_id, old_node_id, new_node_id, changed_by, reason)
			VALUES ($1, NULL, $2, $3, 'initial setup')
		`, carrierID, nid, actor); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}

	if err := tx.Commit(c.Request.Context()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Re-read the row to return the full shape (including node_ids).
	s.returnCarrier(c, carrierID)
}

type updateCarrierRequest struct {
	Name            *string  `json:"name"`
	Host            *string  `json:"host"`
	Port            *int     `json:"port"`
	Transport       *string  `json:"transport"`
	CodecPref       *string  `json:"codec_pref"`
	Notes           *string  `json:"notes"`
	AssignedNodeIDs *[]int64 `json:"assigned_node_ids"`
	Reason          string   `json:"reason"`
	Status          *string  `json:"status"`
}

func (s *Server) patchCarrier(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad id"})
		return
	}
	var req updateCarrierRequest
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

	if req.Transport != nil {
		switch *req.Transport {
		case "udp", "tcp", "tls":
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid transport"})
			return
		}
	}
	if req.Name != nil || req.Host != nil || req.Port != nil || req.Transport != nil ||
		req.CodecPref != nil || req.Notes != nil {
		if _, err := tx.Exec(c.Request.Context(), `
			UPDATE carriers SET
			   name       = COALESCE($2, name),
			   host       = COALESCE($3, host),
			   port       = COALESCE($4, port),
			   transport  = COALESCE($5, transport),
			   codec_pref = COALESCE($6, codec_pref),
			   notes      = COALESCE($7, notes)
			 WHERE id = $1
		`, id, req.Name, req.Host, req.Port, req.Transport, req.CodecPref, req.Notes); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}

	if req.Status != nil {
		switch *req.Status {
		case "active", "paused", "disabled":
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid status"})
			return
		}
		if _, err := tx.Exec(c.Request.Context(),
			`UPDATE carriers SET status = $1 WHERE id = $2`, *req.Status, id); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}

	// Diff node assignments and log ADD / REMOVE rows.
	if req.AssignedNodeIDs != nil {
		desired := map[int64]bool{}
		for _, n := range *req.AssignedNodeIDs {
			desired[n] = true
		}
		currentRows, err := tx.Query(c.Request.Context(),
			`SELECT node_id FROM carrier_media_nodes WHERE carrier_id = $1`, id)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		current := map[int64]bool{}
		for currentRows.Next() {
			var n int64
			if err := currentRows.Scan(&n); err == nil {
				current[n] = true
			}
		}
		currentRows.Close()

		actor, _ := c.Get("user_id")
		reason := req.Reason
		if reason == "" {
			reason = "carrier edit"
		}
		// Add new
		for n := range desired {
			if !current[n] {
				if _, err := tx.Exec(c.Request.Context(),
					`INSERT INTO carrier_media_nodes (carrier_id, node_id, assigned_by) VALUES ($1, $2, $3)`,
					id, n, actor); err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
					return
				}
				if _, err := tx.Exec(c.Request.Context(), `
					INSERT INTO carrier_node_history (carrier_id, old_node_id, new_node_id, changed_by, reason)
					VALUES ($1, NULL, $2, $3, $4)
				`, id, n, actor, reason); err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
			}
		}
		// Remove old
		for n := range current {
			if !desired[n] {
				if _, err := tx.Exec(c.Request.Context(),
					`DELETE FROM carrier_media_nodes WHERE carrier_id = $1 AND node_id = $2`, id, n); err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
					return
				}
				if _, err := tx.Exec(c.Request.Context(), `
					INSERT INTO carrier_node_history (carrier_id, old_node_id, new_node_id, changed_by, reason)
					VALUES ($1, $2, NULL, $3, $4)
				`, id, n, actor, reason); err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
			}
		}
	}

	if err := tx.Commit(c.Request.Context()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

func (s *Server) deleteCarrier(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad id"})
		return
	}
	tag, err := s.deps.PG.Exec(c.Request.Context(), `DELETE FROM carriers WHERE id = $1`, id)
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

type CarrierHistoryEntry struct {
	ID                  int64     `json:"id"`
	OldNodeID           *int64    `json:"old_node_id,omitempty"`
	NewNodeID           *int64    `json:"new_node_id,omitempty"`
	ChangedBy           *int64    `json:"changed_by,omitempty"`
	ChangedAt           time.Time `json:"changed_at"`
	Reason              *string   `json:"reason,omitempty"`
	ActiveCallsAtSwitch *int      `json:"active_calls_at_switch,omitempty"`
}

func (s *Server) carrierHistory(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad id"})
		return
	}
	rows, err := s.deps.PG.Query(c.Request.Context(), `
		SELECT id, old_node_id, new_node_id, changed_by, changed_at, reason, active_calls_at_switch
		  FROM carrier_node_history WHERE carrier_id = $1 ORDER BY changed_at DESC
	`, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	out := []CarrierHistoryEntry{}
	for rows.Next() {
		var h CarrierHistoryEntry
		if err := rows.Scan(&h.ID, &h.OldNodeID, &h.NewNodeID, &h.ChangedBy, &h.ChangedAt, &h.Reason, &h.ActiveCallsAtSwitch); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		out = append(out, h)
	}
	c.JSON(http.StatusOK, out)
}

// returnCarrier reads a single carrier row + its node_ids and returns it.
func (s *Server) returnCarrier(c *gin.Context, id int64) {
	var x Carrier
	err := s.deps.PG.QueryRow(c.Request.Context(), `
		SELECT c.id, c.name, c.host, c.port, c.transport, c.codec_pref, c.notes,
		       c.status, c.created_at,
		       COALESCE(
		         (SELECT array_agg(node_id ORDER BY priority, node_id)
		            FROM carrier_media_nodes
		           WHERE carrier_id = c.id AND status = 'active'),
		         '{}'::bigint[]
		       )
		  FROM carriers c WHERE c.id = $1
	`, id).Scan(&x.ID, &x.Name, &x.Host, &x.Port, &x.Transport, &x.CodecPref, &x.Notes,
		&x.Status, &x.CreatedAt, &x.AssignedNodeIDs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, x)
}
