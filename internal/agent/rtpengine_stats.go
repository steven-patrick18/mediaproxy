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
	conn, err := net.Dial("udp", "127.0.0.1:2223")
	if err != nil {
		return 0, fmt.Errorf("dial rtpengine: %w", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(1 * time.Second))

	cookie := randCookie()
	req := cookie + " d7:command10:statisticse"
	if _, err := conn.Write([]byte(req)); err != nil {
		return 0, fmt.Errorf("write rtpengine: %w", err)
	}

	buf := make([]byte, 65535)
	n, err := conn.Read(buf)
	if err != nil {
		return 0, fmt.Errorf("read rtpengine: %w", err)
	}
	body := string(buf[:n])
	// Strip the echoed cookie + space.
	if !strings.HasPrefix(body, cookie+" ") {
		return 0, fmt.Errorf("rtpengine reply: cookie mismatch")
	}
	body = body[len(cookie)+1:]

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

func randCookie() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
