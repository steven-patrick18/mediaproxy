package agent

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

// QueryRTPEngineSessions asks rtpengine over its NG control socket how many
// call streams are currently active. Returns (count, nil) on success.
//
// Protocol: the NG socket speaks a tiny bencoded request/reply over UDP.
// We craft the smallest possible request:
//
//	<cookie> d7:command10:statisticse
//
// The cookie is an opaque ID echoed back in the reply; bencode 'd...e' is a
// dict containing key "command" with value "statistics". rtpengine answers
// with another bencoded dict; the relevant key is
// "currentstatistics" -> "sessions" -> "own" which is an integer.
//
// We don't pull in a bencode library — the surface is tiny and we only need
// to find one integer. We tolerate either a flat top-level "sessions" key
// (older rtpengine builds) or the nested "currentstatistics.sessions.own"
// (newer builds), returning the first one we find.
//
// Timeout is intentionally short (1s) so a stuck rtpengine never blocks a
// heartbeat tick.
func QueryRTPEngineSessions() (int, error) {
	body, err := ngRoundtrip("d7:command10:statisticse")
	if err != nil {
		return 0, err
	}
	// Two probes in order of preference. rtpengine has changed its stats
	// dict layout across versions; both forms have been observed on Ubuntu
	// 24.04 packages.
	if v, ok := findBencodedInt(body, "own_sessions"); ok {
		return v, nil
	}
	if v, ok := findBencodedInt(body, "currentcalls"); ok {
		return v, nil
	}
	if v, ok := findBencodedInt(body, "sessions"); ok {
		return v, nil
	}
	return 0, fmt.Errorf("rtpengine reply: no session counter found")
}

// findBencodedInt looks for `<keylen>:<key>i<value>e` inside body. This is
// sufficient for the integer fields we care about because bencode encodes
// dict values inline. We deliberately do NOT walk the dict tree — too much
// surface for too little benefit; a substring search is robust enough for
// our single use case.
func findBencodedInt(body, key string) (int, bool) {
	needle := strconv.Itoa(len(key)) + ":" + key + "i"
	idx := strings.Index(body, needle)
	if idx < 0 {
		return 0, false
	}
	start := idx + len(needle)
	end := strings.IndexByte(body[start:], 'e')
	if end < 0 {
		return 0, false
	}
	v, err := strconv.Atoi(body[start : start+end])
	if err != nil {
		return 0, false
	}
	return v, true
}

// findAllBencodedStrings finds every `<keylen>:<key><vallen>:<value>` for
// the given key, returning all values. Used to scan a `list` response for
// every "call-id" string entry.
func findAllBencodedStrings(body, key string) []string {
	needle := strconv.Itoa(len(key)) + ":" + key
	out := []string{}
	cursor := 0
	for {
		idx := strings.Index(body[cursor:], needle)
		if idx < 0 {
			break
		}
		i := cursor + idx + len(needle)
		// Next char should start a string: <len>:<string>
		colon := strings.IndexByte(body[i:], ':')
		if colon < 0 {
			break
		}
		vlen, err := strconv.Atoi(body[i : i+colon])
		if err != nil || vlen <= 0 || vlen > 1024 {
			cursor = i + colon + 1
			continue
		}
		start := i + colon + 1
		end := start + vlen
		if end > len(body) {
			break
		}
		out = append(out, body[start:end])
		cursor = end
	}
	return out
}

// ngRoundtrip sends a single NG-protocol request to RTPEngine and returns
// the decoded body (cookie stripped). All public helpers route through this.
func ngRoundtrip(payload string) (string, error) {
	conn, err := net.Dial("udp", "127.0.0.1:2223")
	if err != nil {
		return "", fmt.Errorf("dial rtpengine: %w", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(1 * time.Second))

	cookie := randCookie()
	if _, err := conn.Write([]byte(cookie + " " + payload)); err != nil {
		return "", fmt.Errorf("write rtpengine: %w", err)
	}
	buf := make([]byte, 65535)
	n, err := conn.Read(buf)
	if err != nil {
		return "", fmt.Errorf("read rtpengine: %w", err)
	}
	body := string(buf[:n])
	if !strings.HasPrefix(body, cookie+" ") {
		return "", fmt.Errorf("rtpengine reply: cookie mismatch")
	}
	return body[len(cookie)+1:], nil
}

// CallQuality is one snapshot of RTP stats for a single active call. Posted
// in batches to /api/v1/agent/call-quality.
type CallQuality struct {
	CallID         string  `json:"call_id"`
	JitterMs       float64 `json:"jitter_ms"`
	PacketLossPct  float64 `json:"packet_loss_pct"`
	MOSScore       float64 `json:"mos_score"`
}

// SampleRTPEngineQuality lists every active call from RTPEngine, queries
// each for stats, and returns a snapshot suitable for batch POST. Returns
// an empty slice (no error) if RTPEngine is unreachable — the heartbeat
// must never block on rtpengine being unhappy.
func SampleRTPEngineQuality() []CallQuality {
	body, err := ngRoundtrip("d7:command4:liste")
	if err != nil {
		return nil
	}
	callIDs := findAllBencodedStrings(body, "call-id")
	if len(callIDs) == 0 {
		return nil
	}
	out := make([]CallQuality, 0, len(callIDs))
	for _, cid := range callIDs {
		// Build "d7:command5:query7:call-id<N>:<cid>e".
		payload := "d7:command5:query7:call-id" + strconv.Itoa(len(cid)) + ":" + cid + "e"
		reply, err := ngRoundtrip(payload)
		if err != nil {
			continue
		}
		// RTPEngine reports per-stream stats; we pull the *totals* keys when
		// they exist and fall back to per-stream summing. Different builds
		// expose slightly different naming.
		jitter := bencodeIntFloatMs(reply, "jitter")
		lossPct := bencodePacketLossPct(reply)
		mos := estimateMOS(jitter, lossPct)
		out = append(out, CallQuality{
			CallID:        cid,
			JitterMs:      jitter,
			PacketLossPct: lossPct,
			MOSScore:      mos,
		})
	}
	return out
}

// bencodeIntFloatMs reads an integer bencoded field interpreted as microseconds
// or milliseconds depending on RTPEngine build (newer versions report µs).
// Both are normalized to milliseconds. If the value is > 5000 we assume µs.
func bencodeIntFloatMs(body, key string) float64 {
	v, ok := findBencodedInt(body, key)
	if !ok {
		return 0
	}
	if v > 5000 { // microseconds
		return float64(v) / 1000.0
	}
	return float64(v)
}

// bencodePacketLossPct computes packet loss percent from RTPEngine's
// totals: packets_lost / (packets_received + packets_lost) * 100.
func bencodePacketLossPct(body string) float64 {
	lost, lok := findBencodedInt(body, "packets_lost")
	rcvd, rok := findBencodedInt(body, "packets_received")
	if !lok && !rok {
		return 0
	}
	total := lost + rcvd
	if total == 0 {
		return 0
	}
	return float64(lost) / float64(total) * 100.0
}

// estimateMOS turns jitter (ms) + loss (%) into a coarse MOS estimate
// using a simplified E-model. Anchored at 4.4 for "no impairment" (G.711
// best-case PSTN). Operators should treat as approximate; for clinical
// accuracy use a proper E-model implementation with codec-specific Ie.
func estimateMOS(jitterMs, lossPct float64) float64 {
	if jitterMs == 0 && lossPct == 0 {
		return 0 // no data -> let the UI render as "—"
	}
	mos := 4.4 - 0.04*jitterMs - 0.20*lossPct
	if mos < 1.0 {
		return 1.0
	}
	if mos > 4.5 {
		return 4.5
	}
	return mos
}

func randCookie() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
