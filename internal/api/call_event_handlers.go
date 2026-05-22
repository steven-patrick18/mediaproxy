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
// Monitor. All are best-effort and may be empty if the SDP didn't include
// the expected lines.
type sdpFields struct {
	Transport   string // e.g. "RTP/AVP", "RTP/SAVP", "UDP/TLS/RTP/SAVP"
	EndpointIP  string // c=IN IP4 <addr>
	CryptoSuite string // a=crypto:N <SUITE> ...
	Codecs      string // comma-joined list, e.g. "PCMU/8000,PCMA/8000,G729/8000"
}

// derefStr returns *s or "" if s is nil. Lets us write the same INSERT
// regardless of whether the upstream RETURNING ... gave us a NULL.
func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// parseSDP pulls out the fields the Privacy page cares about. The parser
// is intentionally permissive — bogus / partial SDP is common in the real
// world and we'd rather report what we found than reject.
//
// Codec extraction walks the SDP twice: first pass builds an rtpmap dict
// (payload-type number -> "NAME/CLOCK"), second pass walks the audio m=
// line's payload-type list in order and looks each one up. This preserves
// the offerer's preference ordering. Static payload types not declared
// via a=rtpmap: get IANA-default names (PCMU=0, PCMA=8, G729=18, etc.).
func parseSDP(body string) sdpFields {
	var f sdpFields
	var mAudioPTs []string
	rtpmap := map[string]string{}

	for _, raw := range strings.Split(body, "\n") {
		line := strings.TrimRight(raw, "\r")
		switch {
		case strings.HasPrefix(line, "m=audio "):
			// m=audio <port> <transport> <pt1> <pt2> ...
			parts := strings.Fields(line)
			if len(parts) >= 3 && f.Transport == "" {
				f.Transport = parts[2]
			}
			if len(mAudioPTs) == 0 && len(parts) > 3 {
				mAudioPTs = append(mAudioPTs, parts[3:]...)
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
		case strings.HasPrefix(line, "a=rtpmap:"):
			// a=rtpmap:<pt> <encoding name>/<clock rate>[/<channels>]
			rest := strings.TrimPrefix(line, "a=rtpmap:")
			parts := strings.Fields(rest)
			if len(parts) >= 2 {
				rtpmap[parts[0]] = parts[1]
			}
		}
	}
	// Build codec list in offer order. Fall back to IANA defaults for static PTs.
	if len(mAudioPTs) > 0 {
		out := make([]string, 0, len(mAudioPTs))
		for _, pt := range mAudioPTs {
			if name, ok := rtpmap[pt]; ok {
				out = append(out, name)
				continue
			}
			if def := ianaStaticCodec(pt); def != "" {
				out = append(out, def)
				continue
			}
			out = append(out, "PT"+pt)
		}
		f.Codecs = strings.Join(out, ",")
	}
	return f
}

// ianaStaticCodec maps the well-known static payload-type numbers (RFC
// 3551) to their codec names. Only audio PTs that show up in real SIP
// trunks are listed; anything else falls through to a "PTn" placeholder
// so the operator can still see the call had something there.
func ianaStaticCodec(pt string) string {
	switch pt {
	case "0":
		return "PCMU/8000"
	case "3":
		return "GSM/8000"
	case "4":
		return "G723/8000"
	case "8":
		return "PCMA/8000"
	case "9":
		return "G722/8000"
	case "18":
		return "G729/8000"
	}
	return ""
}

// POST /api/v1/agent/call-progress
// Body: { "call_id": "...", "pdd_ms": <int> }
//
// Kamailio fires this from onreply_route on the first 18x for a dialog
// (180 Ringing or 183 Session Progress). PDD is the wall-clock delta in
// milliseconds between when we relayed the INVITE outbound and when the
// upstream carrier's first ring/progress reply arrived.
//
// We deliberately only accept the FIRST PDD report per call — subsequent
// 18x retransmissions or re-INVITEs shouldn't overwrite the original PDD.
// Implemented at the SQL level with `WHERE pdd_ms IS NULL`.
type callProgressReq struct {
	CallID string `json:"call_id" binding:"required"`
	PDDMs  int    `json:"pdd_ms" binding:"required,gte=0,lte=120000"`
}

// POST /api/v1/agent/call-quality
//
// Media-role agents push this every heartbeat with a batch of RTP-stats
// snapshots queried from RTPEngine's NG socket. We always take the LATEST
// snapshot per call_id (overwrite-on-update) — the snapshot at call-end
// is what gets propagated to call_records.
type callQualityReq struct {
	Entries []callQualityEntry `json:"entries" binding:"required,dive"`
}
type callQualityEntry struct {
	CallID        string  `json:"call_id" binding:"required"`
	JitterMs      float64 `json:"jitter_ms" binding:"gte=0,lte=10000"`
	PacketLossPct float64 `json:"packet_loss_pct" binding:"gte=0,lte=100"`
	MOSScore      float64 `json:"mos_score" binding:"gte=0,lte=5"`
}

func (s *Server) callQuality(c *gin.Context) {
	nodeID := c.GetInt64("agent_node_id")
	var req callQualityReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// Update each entry individually; a single bad row shouldn't reject the
	// rest. We could batch with VALUES + JOIN, but per-call this stays
	// readable and an agent typically reports <50 active calls per tick.
	// NB: we used to filter `WHERE call_id = $1 AND node_id = $2`. That
	// silently no-op'd because /call-start INSERTs node_id = SipProxy's id
	// (Kamailio runs there) while /call-quality is posted by the media
	// agent (node_id = MediaNode). Match by call_id only — there's a
	// unique constraint on call_id so we can't update a wrong row.
	// Also bump last_seen_at so the LiveCalls UI keeps showing the call
	// past the 2-minute freshness filter; otherwise rows that never get
	// a SIP event after INSERT silently disappear from the live view.
	_ = nodeID
	updated := 0
	for _, e := range req.Entries {
		tag, err := s.deps.PG.Exec(c.Request.Context(), `
			UPDATE active_calls
			   SET avg_jitter_ms       = $2,
			       avg_packet_loss_pct = $3,
			       mos_score           = NULLIF($4, 0),
			       last_seen_at        = now()
			 WHERE call_id = $1
		`, e.CallID, e.JitterMs, e.PacketLossPct, e.MOSScore)
		if err != nil {
			slog.Error("call-quality: update failed",
				"err", err, "call_id", e.CallID)
			continue
		}
		if tag.RowsAffected() > 0 {
			updated++
		}
	}
	c.JSON(http.StatusOK, gin.H{"received": len(req.Entries), "updated": updated})
}

// POST /api/v1/agent/call-reinvite
// Body: { "call_id": "...", "sdp_b64": "<base64 of new SDP>" }
//
// Kamailio fires this when an in-dialog INVITE arrives (loose_route() true
// AND method == INVITE). The new SDP's media endpoint may differ from the
// initial offer — that delta is the privacy-relevant signal (third-party
// bridge, fork, NAT roam). We increment reinvite_count and stamp the new
// endpoint for the Privacy page to show as an alert.
type callReinviteReq struct {
	CallID string `json:"call_id" binding:"required"`
	SDPB64 string `json:"sdp_b64"`
}

func (s *Server) callReinvite(c *gin.Context) {
	nodeID := c.GetInt64("agent_node_id")
	var req callReinviteReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// Parse the new SDP to extract the endpoint IP (if any). Empty body
	// re-INVITEs (session refresh) won't have an endpoint — that's fine.
	var endpoint *string
	if req.SDPB64 != "" {
		if raw, err := base64.StdEncoding.DecodeString(req.SDPB64); err == nil {
			if fields := parseSDP(string(raw)); fields.EndpointIP != "" {
				ep := fields.EndpointIP
				endpoint = &ep
			}
		}
	}
	_ = nodeID
	if _, err := s.deps.PG.Exec(c.Request.Context(), `
		UPDATE active_calls
		   SET reinvite_count         = reinvite_count + 1,
		       last_reinvite_at       = now(),
		       last_reinvite_endpoint = COALESCE($2::inet, last_reinvite_endpoint),
		       last_seen_at           = now()
		 WHERE call_id = $1
	`, req.CallID, endpoint); err != nil {
		slog.Error("call-reinvite: update failed", "err", err, "call_id", req.CallID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

func (s *Server) callProgress(c *gin.Context) {
	nodeID := c.GetInt64("agent_node_id")
	var req callProgressReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	_ = nodeID
	// pdd_ms only sets on first 18x (NULL guard); last_seen_at always
	// bumps so a long ringing call doesn't fall out of the LiveCalls
	// 2-minute freshness window. Dropped node_id filter — handlers in
	// /call-* shouldn't reject media-node-posted events; row uniqueness
	// is enforced by call_id alone.
	if _, err := s.deps.PG.Exec(c.Request.Context(), `
		UPDATE active_calls
		   SET pdd_ms       = COALESCE(pdd_ms, $2),
		       last_seen_at = now()
		 WHERE call_id = $1
	`, req.CallID, req.PDDMs); err != nil {
		slog.Error("call-progress: update failed",
			"err", err, "call_id", req.CallID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
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
		                          codecs_offered, started_at)
		VALUES ($1, $2, $3, $4, NULLIF($5,'')::inet, NULLIF($6,'')::inet,
		        NULLIF($7,''), NULLIF($8,''),
		        NULLIF($9,''), NULLIF($10,'')::inet, NULLIF($11,''),
		        NULLIF($12,''), now())
		ON CONFLICT (call_id) DO UPDATE SET last_seen_at = now()
	`, req.CallID, req.ClientID, req.CarrierID, nodeID,
		req.MediaIP, req.SignalingFrom, req.ANI, req.DNIS,
		sdp.Transport, sdp.EndpointIP, sdp.CryptoSuite, sdp.Codecs)
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

	// Pull the active row, write CDR, delete. inet columns are pulled as
	// host() text so pgx Scan into *string works — scanning raw inet into
	// *string returned NotFound silently and CDRs never landed.
	row := s.deps.PG.QueryRow(c.Request.Context(), `
		DELETE FROM active_calls WHERE call_id = $1 AND node_id = $2
		 RETURNING client_id, carrier_id, host(media_ip), host(signaling_from), ani, dnis, started_at,
		           media_transport, host(media_endpoint_ip), crypto_suite,
		           pdd_ms, codecs_offered,
		           avg_jitter_ms, avg_packet_loss_pct, mos_score,
		           reinvite_count, last_reinvite_at
	`, req.CallID, nodeID)
	var (
		clientID, carrierID                   *int64
		mediaIP, sigFrom                      *string
		ani, dnis                             *string
		startedAt                             time.Time
		mediaTransport, mediaEndpoint, crypto *string
		pddMs                                 *int
		codecsOffered                         *string
		avgJitterMs, avgLossPct, mosScore     *float64
		reinviteCount                         int
		lastReinviteAt                        *time.Time
	)
	if err := row.Scan(&clientID, &carrierID, &mediaIP, &sigFrom, &ani, &dnis, &startedAt,
		&mediaTransport, &mediaEndpoint, &crypto, &pddMs, &codecsOffered,
		&avgJitterMs, &avgLossPct, &mosScore,
		&reinviteCount, &lastReinviteAt); err != nil {
		// Log the real cause — was it ErrNoRows or a Scan/type error?
		slog.Warn("call-end: scan failed",
			"err", err, "call_id", req.CallID, "node_id", nodeID)
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
		     media_transport, media_endpoint_ip, crypto_suite,
		     pdd_ms, codecs_offered,
		     avg_jitter_ms, avg_packet_loss_pct, mos_score,
		     reinvite_count, last_reinvite_at)
		VALUES ($1, $2, $3, $4, $5::inet, $6::inet, $7, $8, $9, now(), $10, $11, $12,
		        $13, NULLIF($14,'')::inet, $15,
		        $16, $17,
		        $18, $19, $20,
		        $21, $22)
	`, req.CallID, clientID, carrierID, nodeID, mediaIP, sigFrom, ani, dnis,
		startedAt, dur, dispo, sipCode,
		mediaTransport, derefStr(mediaEndpoint), crypto,
		pddMs, codecsOffered,
		avgJitterMs, avgLossPct, mosScore,
		reinviteCount, lastReinviteAt)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if mediaIP != nil && *mediaIP != "" {
		if _, err := s.deps.PG.Exec(c.Request.Context(),
			`UPDATE node_ips SET current_calls = GREATEST(current_calls - 1, 0) WHERE ip_address = $1::inet`, *mediaIP); err != nil {
			slog.Error("call-end: decrement node_ips.current_calls failed",
				"err", err, "call_id", req.CallID, "media_ip", *mediaIP, "node_id", nodeID)
		}
	}
	c.Status(http.StatusNoContent)
}
