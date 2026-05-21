package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// homer_proxy: panel-side proxy to the local HOMER instance. We avoid
// shipping users to HOMER's standalone UI (jarring tab switch, requires
// their second login, slow). Instead we fetch the call's SIP messages
// over HOMER's REST API and render our own compact ladder in a modal.
//
// HOMER auth is JWT with a 1h lifetime — cached per-process so we don't
// log-in on every modal open. Credentials come from env vars
// MEDIAPROXY_HOMER_USER / MEDIAPROXY_HOMER_PASS with the default
// sipcapture/sipcapture pair the docker-compose ships with.
//
// The handler is read-only and safe to expose to any logged-in admin —
// HOMER itself enforces no per-call ACLs.

// SipMessage is the per-message row we return to the UI. Mapped from
// HOMER's calldata[] entries — we keep only fields a ladder rendering
// actually needs.
type SipMessage struct {
	ID         int64  `json:"id"`
	Timestamp  int64  `json:"timestamp_ms"` // micro_ts from HOMER, converted to ms
	SrcIP      string `json:"src_ip"`
	SrcPort    int    `json:"src_port"`
	DstIP      string `json:"dst_ip"`
	DstPort    int    `json:"dst_port"`
	Method     string `json:"method"` // "INVITE", "100", "200", "BYE", etc.
	MethodText string `json:"method_text"`
	RURIPreview string `json:"ruri_preview"`
	Color      string `json:"color"` // ladder color hint
}

// SipEndpoint is a human-friendly label for one of the IPs that appears
// in the message list. Resolved server-side so the modal doesn't have
// to pull clients / carriers / media_nodes separately.
type SipEndpoint struct {
	IP    string `json:"ip"`
	Role  string `json:"role"`  // "dialer" | "sip_proxy" | "media" | "carrier" | "unknown"
	Name  string `json:"name"`  // friendly label, e.g. "Alfa Dialer", "RTNG05", "Our SIP Proxy"
	Emoji string `json:"emoji"` // 📞 / 🔒 / 🌐 / ❓ — purely UI decoration
}

// homerToken keeps a cached JWT for the lifetime of the process. Refresh on
// 401 or when older than refreshAfter.
var (
	homerTokenMu  sync.Mutex
	homerToken    string
	homerTokenExp time.Time
)

const (
	homerBaseURL    = "http://127.0.0.1:9080"
	homerRefreshTTL = 50 * time.Minute // HOMER tokens last 1h; refresh a bit early
)

func homerCreds() (string, string) {
	u := os.Getenv("MEDIAPROXY_HOMER_USER")
	if u == "" {
		u = "admin"
	}
	p := os.Getenv("MEDIAPROXY_HOMER_PASS")
	if p == "" {
		p = "sipcapture"
	}
	return u, p
}

func homerLogin(ctx context.Context, httpC *http.Client) (string, error) {
	u, p := homerCreds()
	body, _ := json.Marshal(map[string]string{"username": u, "password": p})
	req, _ := http.NewRequestWithContext(ctx, "POST", homerBaseURL+"/api/v3/auth", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	res, err := httpC.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	// HOMER returns 201 Created on successful auth (not 200). Accept any
	// 2xx — checking against a single status code rejected legitimate
	// logins and surfaced the token JSON as an error message in the UI.
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		buf, _ := io.ReadAll(res.Body)
		return "", fmt.Errorf("homer auth %d: %s", res.StatusCode, string(buf))
	}
	var out struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return "", err
	}
	if out.Token == "" {
		return "", errors.New("homer auth: empty token")
	}
	return out.Token, nil
}

func getHomerToken(ctx context.Context, httpC *http.Client, force bool) (string, error) {
	homerTokenMu.Lock()
	defer homerTokenMu.Unlock()
	if !force && homerToken != "" && time.Now().Before(homerTokenExp) {
		return homerToken, nil
	}
	tok, err := homerLogin(ctx, httpC)
	if err != nil {
		return "", err
	}
	homerToken = tok
	homerTokenExp = time.Now().Add(homerRefreshTTL)
	return tok, nil
}

// GET /api/v1/cdrs/:call_id/sip-trace?started_at=<unix_ms>
//
// Returns chronologically-sorted SIP messages for the call, fetched from
// HOMER. started_at is optional and used to narrow the time window — if
// omitted we look back 7 days (HOMER queries get slow if the window is
// huge, but 7d is comfortably small for our retention).
func (s *Server) cdrSipTrace(c *gin.Context) {
	callID := c.Param("call_id")
	if callID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "call_id required"})
		return
	}
	// HOMER paths use slashes; the gin param matcher already URL-decoded
	// our segment, so the literal "@" and other chars are intact.

	var toMs int64 = time.Now().UnixMilli()
	var fromMs int64 = toMs - 7*24*60*60*1000
	if v := c.Query("started_at"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			// Search ±15 minutes around the call's start time. That comfortably
			// covers ringback through call completion + a few BYE retries.
			fromMs = n - 15*60*1000
			toMs = n + 15*60*1000
		}
	}

	httpC := &http.Client{Timeout: 10 * time.Second}
	token, err := getHomerToken(c.Request.Context(), httpC, false)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "homer auth: " + err.Error()})
		return
	}

	calls, err := homerCallTransaction(c.Request.Context(), httpC, token, callID, fromMs, toMs)
	if errors.Is(err, errHomerUnauthorized) {
		// Token expired between cache and use. Force-refresh and retry once.
		token, err = getHomerToken(c.Request.Context(), httpC, true)
		if err == nil {
			calls, err = homerCallTransaction(c.Request.Context(), httpC, token, callID, fromMs, toMs)
		}
	}
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "homer query: " + err.Error()})
		return
	}

	// Collect unique IPs and resolve each to a human-friendly role/name.
	seen := map[string]struct{}{}
	for _, m := range calls {
		seen[m.SrcIP] = struct{}{}
		seen[m.DstIP] = struct{}{}
	}
	endpoints := s.resolveSipEndpoints(c.Request.Context(), seen)

	c.JSON(http.StatusOK, gin.H{
		"call_id":   callID,
		"messages":  calls,
		"endpoints": endpoints,
		"from_ms":   fromMs,
		"to_ms":     toMs,
	})
}

// resolveSipEndpoints walks the IP set and tags each one as dialer /
// sip_proxy / media / carrier / unknown by querying the relevant tables.
// Single query per table (IN-list) so this stays cheap even with many
// IPs in one call.
func (s *Server) resolveSipEndpoints(ctx context.Context, ipSet map[string]struct{}) []SipEndpoint {
	ips := make([]string, 0, len(ipSet))
	for ip := range ipSet {
		if ip != "" {
			ips = append(ips, ip)
		}
	}
	out := make(map[string]SipEndpoint, len(ips))

	// 1) Client dialer IPs → "📞 Dialer (client name)"
	if rows, err := s.deps.PG.Query(ctx, `
		SELECT host(ci.ip_address), c.name
		  FROM client_ips ci JOIN clients c ON c.id = ci.client_id
		 WHERE host(ci.ip_address) = ANY($1)
	`, ips); err == nil {
		defer rows.Close()
		for rows.Next() {
			var ip, name string
			if rows.Scan(&ip, &name) == nil {
				out[ip] = SipEndpoint{IP: ip, Role: "dialer", Name: "Dialer · " + name, Emoji: "📞"}
			}
		}
	}

	// 2) media_nodes — both the host_ip (mgmt) and any signaling_ips.
	// sip_proxy → "🔒 Our SIP Proxy", media → "🎵 Media Node".
	if rows, err := s.deps.PG.Query(ctx, `
		SELECT host(host_ip), name, role FROM media_nodes WHERE host(host_ip) = ANY($1)
	`, ips); err == nil {
		defer rows.Close()
		for rows.Next() {
			var ip, name, role string
			if rows.Scan(&ip, &name, &role) == nil {
				if role == "sip_proxy" {
					out[ip] = SipEndpoint{IP: ip, Role: role, Name: "Our SIP Proxy · " + name, Emoji: "🔒"}
				} else {
					out[ip] = SipEndpoint{IP: ip, Role: "media", Name: "Media Node · " + name, Emoji: "🎵"}
				}
			}
		}
	}
	if rows, err := s.deps.PG.Query(ctx, `
		SELECT host(sip.ip_address), mn.name
		  FROM signaling_ips sip
		  JOIN media_nodes mn ON mn.id = sip.sip_proxy_node_id
		 WHERE host(sip.ip_address) = ANY($1)
	`, ips); err == nil {
		defer rows.Close()
		for rows.Next() {
			var ip, name string
			if rows.Scan(&ip, &name) == nil {
				out[ip] = SipEndpoint{IP: ip, Role: "sip_proxy", Name: "Our SIP Proxy (signaling) · " + name, Emoji: "🔒"}
			}
		}
	}

	// 3) Carriers — match on the host field. Carriers may have a hostname
	// instead of an IP (e.g. carrier.example.com), so this only resolves
	// literal-IP entries. Hostnames fall through to "unknown".
	if rows, err := s.deps.PG.Query(ctx, `
		SELECT host, name FROM carriers WHERE host = ANY($1)
	`, ips); err == nil {
		defer rows.Close()
		for rows.Next() {
			var ip, name string
			if rows.Scan(&ip, &name) == nil {
				out[ip] = SipEndpoint{IP: ip, Role: "carrier", Name: "Carrier · " + name, Emoji: "🌐"}
			}
		}
	}

	// 4) Fill the gaps with unknown — better UX than missing entries.
	result := make([]SipEndpoint, 0, len(ips))
	for _, ip := range ips {
		if e, ok := out[ip]; ok {
			result = append(result, e)
		} else {
			result = append(result, SipEndpoint{IP: ip, Role: "unknown", Name: ip, Emoji: "❓"})
		}
	}
	return result
}

var errHomerUnauthorized = errors.New("homer 401")

func homerCallTransaction(ctx context.Context, httpC *http.Client, token, callID string, fromMs, toMs int64) ([]SipMessage, error) {
	// HOMER's call/transaction expects timestamps in MILLISECONDS as ints,
	// callid as an array of strings. We never pass an actual user input
	// directly — callID comes from the URL but we let json.Marshal escape.
	body := map[string]any{
		"timestamp": map[string]int64{"from": fromMs, "to": toMs},
		"param": map[string]any{
			"limit": 500,
			"search": map[string]any{
				"1_call": map[string]any{
					"callid": []string{callID},
				},
			},
			"location": map[string]any{},
			"transaction": map[string]any{
				"call":         true,
				"registration": false,
				"rest":         false,
			},
		},
	}
	buf, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(ctx, "POST", homerBaseURL+"/api/v3/call/transaction", bytes.NewReader(buf))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	res, err := httpC.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode == 401 || res.StatusCode == 403 {
		return nil, errHomerUnauthorized
	}
	// HOMER returns 201 Created on successful POST queries (not 200).
	// Accept any 2xx so we don't dump the entire response body as a
	// fake "error" in the panel modal.
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		raw, _ := io.ReadAll(res.Body)
		return nil, fmt.Errorf("homer %d: %s", res.StatusCode, strings.TrimSpace(string(raw)))
	}
	var wire struct {
		Data struct {
			CallData []struct {
				ID        int64  `json:"id"`
				MicroTS   int64  `json:"micro_ts"`
				SrcIP     string `json:"srcIp"`
				DstIP     string `json:"dstIp"`
				SrcPort   int    `json:"srcPort"`
				DstPort   int    `json:"dstPort"`
				Method    string `json:"method"`
				MethodTxt string `json:"method_text"`
				RURI      string `json:"ruri_user"`
				Color     string `json:"msg_color"`
			} `json:"calldata"`
		} `json:"data"`
	}
	if err := json.NewDecoder(res.Body).Decode(&wire); err != nil {
		return nil, err
	}
	out := make([]SipMessage, 0, len(wire.Data.CallData))
	for _, m := range wire.Data.CallData {
		out = append(out, SipMessage{
			ID:          m.ID,
			Timestamp:   m.MicroTS,
			SrcIP:       m.SrcIP,
			SrcPort:     m.SrcPort,
			DstIP:       m.DstIP,
			DstPort:     m.DstPort,
			Method:      m.Method,
			MethodText:  m.MethodTxt,
			RURIPreview: m.RURI,
			Color:       m.Color,
		})
	}
	return out, nil
}
