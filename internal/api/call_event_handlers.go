package api

import (
	"encoding/base64"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// Call events come from Kamailio's `acc` module via http_post or from the
// agent forwarding them. All three of these endpoints take a node-scoped
// agent token (so the agent is the legitimate poster); admins also can
// reach them via the JWT-authed group.

type callStartReq struct {
	CallID        string `json:"call_id" binding:"required"`
	ClientID      *int64 `json:"client_id"`
	CarrierID     *int64 `json:"carrier_id"`
	MediaIP       string `json:"media_ip"`
	SignalingFrom string `json:"signaling_from"`
	ANI           string `json:"ani"`
	DNIS          string `json:"dnis"`
	// SDPb64 is the INVITE's SDP body, base64-encoded. Kamailio encodes it
	// so we don't have to JSON-escape SDP content (newlines, slashes, etc).
	// Empty / absent when the call wasn't initiated by an INVITE we proxied,
	// or when the SipProxy is running an old kamailio template.
	SDPb64 string `json:"sdp_b64"`
}

// sdpFields holds the fields we extract from an SDP body for the Privacy
// Monitor. All three are best-effort and may be empty if the SDP didn't
// include the expected lines.
type sdpFields struct {
	Transport   string // e.g. "RTP/AVP", "RTP/SAVP", "UDP/TLS/RTP/SAVP"
	EndpointIP  string // c=IN IP4 <addr>
	CryptoSuite string // a=crypto:N <SUITE> ...
}

// derefStr returns *s or "" if s is nil. Lets us write the same INSERT
// regardless of whether the upstream RETURNING ... gave us a NULL.
func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// parseSDP pulls out the three fields the Privacy page cares about. The
// parser is intentionally permissive — bogus / partial SDP is common in
// the real world and we'd rather report what we found than reject.
func parseSDP(body string) sdpFields {
	var f sdpFields
	for _, raw := range strings.Split(body, "\n") {
		line := strings.TrimRight(raw, "\r")
		switch {
		case strings.HasPrefix(line, "m=audio "):
			// m=audio <port> <transport> <fmt>...
			parts := strings.Fields(line)
			if len(parts) >= 3 && f.Transport == "" {
				f.Transport = parts[2]
			}
		case strings.HasPrefix(line, "c=IN IP4 "):
			rest := strings.TrimSpace(strings.TrimPrefix(line, "c=IN IP4 "))
			// c=IN IP4 192.0.2.1/127 (TTL for multicast); strip the /N part.
			if idx := strings.IndexByte(rest, '/'); idx >= 0 {
				rest = rest[:idx]
			}
			if rest != "" && f.EndpointIP == "" {
				f.EndpointIP = rest
			}
		case strings.HasPrefix(line, "a=crypto:"):
			// a=crypto:<tag> <suite> inline:<key>...
			rest := strings.TrimPrefix(line, "a=crypto:")
			parts := strings.Fields(rest)
			if len(parts) >= 2 && f.CryptoSuite == "" {
				f.CryptoSuite = parts[1]
			}
		}
	}
	return f
}

// POST /api/v1/agent/call-start — Kamailio fires this on 200 OK.
func (s *Server) callStart(c *gin.Context) {
	nodeID := c.GetInt64("agent_node_id")
	var req callStartReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Best-effort SDP parse. Failure here never blocks the call record
	// itself — if base64 decode fails or the body isn't SDP-shaped, we
	// just log and proceed with empty SDP fields.
	var sdp sdpFields
	if req.SDPb64 != "" {
		if raw, err := base64.StdEncoding.DecodeString(req.SDPb64); err == nil {
			sdp = parseSDP(string(raw))
		} else {
			slog.Warn("call-start: SDP base64 decode failed", "err", err, "call_id", req.CallID)
		}
	}

	_, err := s.deps.PG.Exec(c.Request.Context(), `
		INSERT INTO active_calls (call_id, client_id, carrier_id, node_id,
		                          media_ip, signaling_from, ani, dnis,
		                          media_transport, media_endpoint_ip, crypto_suite,
		                          started_at)
		VALUES ($1, $2, $3, $4, NULLIF($5,'')::inet, NULLIF($6,'')::inet,
		        NULLIF($7,''), NULLIF($8,''),
		        NULLIF($9,''), NULLIF($10,'')::inet, NULLIF($11,''),
		        now())
		ON CONFLICT (call_id) DO UPDATE SET last_seen_at = now()
	`, req.CallID, req.ClientID, req.CarrierID, nodeID,
		req.MediaIP, req.SignalingFrom, req.ANI, req.DNIS,
		sdp.Transport, sdp.EndpointIP, sdp.CryptoSuite)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.MediaIP != "" {
		if _, err := s.deps.PG.Exec(c.Request.Context(),
			`UPDATE node_ips SET current_calls = current_calls + 1 WHERE ip_address = $1::inet`, req.MediaIP); err != nil {
			// Counter drift will bypass max_calls cap until next reconcile —
			// loud log so ops can spot DB hiccups before they distort routing.
			slog.Error("call-start: bump node_ips.current_calls failed",
				"err", err, "call_id", req.CallID, "media_ip", req.MediaIP, "node_id", nodeID)
		}
	}
	c.Status(http.StatusNoContent)
}

type callEndReq struct {
	CallID      string `json:"call_id" binding:"required"`
	Disposition string `json:"disposition"`
	SipCode     int    `json:"sip_code"`
	DurationSec int    `json:"duration_sec"`
}

// POST /api/v1/agent/call-end — Kamailio fires this on BYE / final
// non-2xx. Moves the row from active_calls into call_records.
func (s *Server) callEnd(c *gin.Context) {
	nodeID := c.GetInt64("agent_node_id")
	var req callEndReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	dispo := req.Disposition
	if dispo == "" {
		switch {
		case req.SipCode == 200:
			dispo = "answered"
		case req.SipCode == 486:
			dispo = "busy"
		case req.SipCode == 487:
			dispo = "canceled"
		case req.SipCode >= 400 && req.SipCode < 500:
			dispo = "failed"
		case req.SipCode >= 500:
			dispo = "failed"
		default:
			dispo = "no_answer"
		}
	}

	// Pull the active row, write CDR, delete.
	row := s.deps.PG.QueryRow(c.Request.Context(), `
		DELETE FROM active_calls WHERE call_id = $1 AND node_id = $2
		 RETURNING client_id, carrier_id, media_ip, signaling_from, ani, dnis, started_at,
		           media_transport, host(media_endpoint_ip), crypto_suite
	`, req.CallID, nodeID)
	var (
		clientID, carrierID                    *int64
		mediaIP, sigFrom                       *string
		ani, dnis                              *string
		startedAt                              time.Time
		mediaTransport, mediaEndpoint, crypto  *string
	)
	if err := row.Scan(&clientID, &carrierID, &mediaIP, &sigFrom, &ani, &dnis, &startedAt,
		&mediaTransport, &mediaEndpoint, &crypto); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "active call not found"})
		return
	}

	dur := req.DurationSec
	if dur == 0 && !startedAt.IsZero() {
		dur = int(time.Since(startedAt).Seconds())
	}

	var sipCode *int
	if req.SipCode != 0 {
		sipCode = &req.SipCode
	}
	_, err := s.deps.PG.Exec(c.Request.Context(), `
		INSERT INTO call_records
		    (call_id, client_id, carrier_id, node_id, media_ip, signaling_from,
		     ani, dnis, started_at, ended_at, duration_sec, disposition, sip_code,
		     media_transport, media_endpoint_ip, crypto_suite)
		VALUES ($1, $2, $3, $4, $5::inet, $6::inet, $7, $8, $9, now(), $10, $11, $12,
		        $13, NULLIF($14,'')::inet, $15)
	`, req.CallID, clientID, carrierID, nodeID, mediaIP, sigFrom, ani, dnis,
		startedAt, dur, dispo, sipCode,
		mediaTransport, derefStr(mediaEndpoint), crypto)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if mediaIP != nil && *mediaIP != "" {
		if _, err := s.deps.PG.Exec(c.Request.Context(),
			`UPDATE node_ips SET current_calls = GREATEST(current_calls - 1, 0) WHERE host(ip_address) = $1`, *mediaIP); err != nil {
			slog.Error("call-end: decrement node_ips.current_calls failed",
				"err", err, "call_id", req.CallID, "media_ip", *mediaIP, "node_id", nodeID)
		}
	}
	c.Status(http.StatusNoContent)
}
