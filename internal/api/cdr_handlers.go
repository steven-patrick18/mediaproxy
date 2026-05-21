package api

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type CDR struct {
	ID            int64      `json:"id"`
	CallID        string     `json:"call_id"`
	ClientID      *int64     `json:"client_id,omitempty"`
	CarrierID     *int64     `json:"carrier_id,omitempty"`
	NodeID        *int64     `json:"node_id,omitempty"`
	MediaIP       *string    `json:"media_ip,omitempty"`
	SignalingFrom *string    `json:"signaling_from,omitempty"`
	ANI           *string    `json:"ani,omitempty"`
	DNIS          *string    `json:"dnis,omitempty"`
	StartedAt     time.Time  `json:"started_at"`
	AnsweredAt    *time.Time `json:"answered_at,omitempty"`
	EndedAt       *time.Time `json:"ended_at,omitempty"`
	DurationSec   *int       `json:"duration_sec,omitempty"`
	Disposition   *string    `json:"disposition,omitempty"`
	SipCode       *int       `json:"sip_code,omitempty"`
}

// GET /api/v1/cdrs?client_id=&carrier_id=&node_id=&disposition=&from=&to=&limit=&offset=
func (s *Server) listCDRs(c *gin.Context) {
	w := whereBuilder{}
	if v := c.Query("client_id"); v != "" {
		w.add("client_id = $%d", v)
	}
	if v := c.Query("carrier_id"); v != "" {
		w.add("carrier_id = $%d", v)
	}
	if v := c.Query("node_id"); v != "" {
		w.add("node_id = $%d", v)
	}
	if v := c.Query("disposition"); v != "" {
		w.add("disposition = $%d", v)
	}
	if v := c.Query("from"); v != "" {
		w.add("started_at >= $%d::timestamptz", v)
	}
	if v := c.Query("to"); v != "" {
		w.add("started_at <= $%d::timestamptz", v)
	}
	if v := c.Query("dnis"); v != "" {
		w.add("dnis LIKE $%d", v+"%")
	}

	limit := 100
	if v, err := strconv.Atoi(c.Query("limit")); err == nil && v > 0 && v <= 1000 {
		limit = v
	}
	offset := 0
	if v, err := strconv.Atoi(c.Query("offset")); err == nil && v >= 0 {
		offset = v
	}

	q := `SELECT id, call_id, client_id, carrier_id, node_id,
	             host(media_ip), host(signaling_from), ani, dnis,
	             started_at, answered_at, ended_at, duration_sec,
	             disposition, sip_code
	        FROM call_records ` + w.sql() + `
	       ORDER BY started_at DESC
	       LIMIT $` + strconv.Itoa(len(w.args)+1) + ` OFFSET $` + strconv.Itoa(len(w.args)+2)
	args := append(w.args, limit, offset)

	rows, err := s.deps.PG.Query(c.Request.Context(), q, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	out := []CDR{}
	for rows.Next() {
		var r CDR
		if err := rows.Scan(&r.ID, &r.CallID, &r.ClientID, &r.CarrierID, &r.NodeID,
			&r.MediaIP, &r.SignalingFrom, &r.ANI, &r.DNIS,
			&r.StartedAt, &r.AnsweredAt, &r.EndedAt, &r.DurationSec,
			&r.Disposition, &r.SipCode); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		out = append(out, r)
	}
	c.JSON(http.StatusOK, out)
}

// GET /api/v1/cdrs/stats — aggregate ASR / ACD / count, optionally grouped.
type CDRStats struct {
	Total      int64    `json:"total"`
	Answered   int64    `json:"answered"`
	ASRPct     float64  `json:"asr_pct"`
	ACDSeconds *float64 `json:"acd_seconds,omitempty"`
}

func (s *Server) cdrStats(c *gin.Context) {
	w := whereBuilder{}
	if v := c.Query("client_id"); v != "" {
		w.add("client_id = $%d", v)
	}
	if v := c.Query("carrier_id"); v != "" {
		w.add("carrier_id = $%d", v)
	}
	if v := c.Query("from"); v != "" {
		w.add("started_at >= $%d::timestamptz", v)
	}
	if v := c.Query("to"); v != "" {
		w.add("started_at <= $%d::timestamptz", v)
	}
	q := `SELECT count(*),
	             count(*) FILTER (WHERE disposition = 'answered'),
	             AVG(duration_sec) FILTER (WHERE disposition = 'answered')
	        FROM call_records ` + w.sql()
	var st CDRStats
	var acd *float64
	if err := s.deps.PG.QueryRow(c.Request.Context(), q, w.args...).
		Scan(&st.Total, &st.Answered, &acd); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if st.Total > 0 {
		st.ASRPct = float64(st.Answered) / float64(st.Total) * 100
	}
	st.ACDSeconds = acd
	c.JSON(http.StatusOK, st)
}

// ActiveCall is the live-call view; rows older than 2 minutes are filtered out.
type ActiveCall struct {
	ID                   int64      `json:"id"`
	CallID               string     `json:"call_id"`
	ClientID             *int64     `json:"client_id,omitempty"`
	CarrierID            *int64     `json:"carrier_id,omitempty"`
	NodeID               *int64     `json:"node_id,omitempty"`
	MediaIP              *string    `json:"media_ip,omitempty"`
	SignalingFrom        *string    `json:"signaling_from,omitempty"`
	ANI                  *string    `json:"ani,omitempty"`
	DNIS                 *string    `json:"dnis,omitempty"`
	StartedAt            time.Time  `json:"started_at"`
	LastSeenAt           time.Time  `json:"last_seen_at"`
	DurationSec          int        `json:"duration_sec"`
	MediaTransport       *string    `json:"media_transport,omitempty"`
	MediaEndpointIP      *string    `json:"media_endpoint_ip,omitempty"`
	CryptoSuite          *string    `json:"crypto_suite,omitempty"`
	ReinviteCount        int        `json:"reinvite_count"`
	LastReinviteAt       *time.Time `json:"last_reinvite_at,omitempty"`
	LastReinviteEndpoint *string    `json:"last_reinvite_endpoint,omitempty"`
}

// GET /api/v1/calls/active?node_id=
func (s *Server) listActiveCalls(c *gin.Context) {
	w := whereBuilder{}
	w.add("last_seen_at >= now() - interval '2 minutes'", "")
	if v := c.Query("node_id"); v != "" {
		w.add("node_id = $%d", v)
	}
	q := `SELECT id, call_id, client_id, carrier_id, node_id,
	             host(media_ip), host(signaling_from), ani, dnis,
	             started_at, last_seen_at,
	             EXTRACT(EPOCH FROM (now() - started_at))::int AS dur,
	             media_transport, host(media_endpoint_ip), crypto_suite,
	             reinvite_count, last_reinvite_at, host(last_reinvite_endpoint)
	        FROM active_calls ` + w.sql() + ` ORDER BY started_at DESC LIMIT 500`
	rows, err := s.deps.PG.Query(c.Request.Context(), q, w.args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	out := []ActiveCall{}
	for rows.Next() {
		var a ActiveCall
		if err := rows.Scan(&a.ID, &a.CallID, &a.ClientID, &a.CarrierID, &a.NodeID,
			&a.MediaIP, &a.SignalingFrom, &a.ANI, &a.DNIS,
			&a.StartedAt, &a.LastSeenAt, &a.DurationSec,
			&a.MediaTransport, &a.MediaEndpointIP, &a.CryptoSuite,
			&a.ReinviteCount, &a.LastReinviteAt, &a.LastReinviteEndpoint); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		out = append(out, a)
	}
	c.JSON(http.StatusOK, out)
}

// --- helper -----------------------------------------------------------------

type whereBuilder struct {
	clauses []string
	args    []any
}

func (w *whereBuilder) add(clause string, val any) {
	// Empty string val means no parameter needed (clause is literal SQL).
	if s, ok := val.(string); ok && s == "" {
		w.clauses = append(w.clauses, clause)
		return
	}
	w.args = append(w.args, val)
	w.clauses = append(w.clauses, fmt.Sprintf(clause, len(w.args)))
}

func (w *whereBuilder) sql() string {
	if len(w.clauses) == 0 {
		return ""
	}
	return "WHERE " + strings.Join(w.clauses, " AND ")
}
