package agent

import (
	"bufio"
	"log/slog"
	"os"
	"strconv"
	"strings"
)

// Kamailio worker-stuck watchdog.
//
// Failure mode this prevents: under sustained call load, Kamailio s
// worker pool can wedge. recv-Q on the UDP listen socket pins at
// rmem_max * 2 (~8 MB after our sysctl bump); workers go into
// __skb_wait_for_more_packets but stop pulling. systemctl reports the
// service as "active" because the parent PID is up — so external
// monitoring misses it. Symptom on prod: dialer marks the trunk
// UNREACHABLE because OPTIONS pings stop getting replies; call volume
// drops to zero until an operator notices and does `systemctl restart
// kamailio`.
//
// This watchdog samples recv-Q every heartbeat tick. If recv-Q stays
// above stuckRecvQBytes for more than stuckSeconds consecutive seconds,
// the agent restarts kamailio and resets the counter. Healthy state
// (recv-Q below threshold) resets the counter too — so legitimate
// bursts don t cause false-positive restarts.

const (
	// stuckRecvQBytes: above this is "the workers can t drain".
	// 1 MB was chosen so we don t false-trigger on the brief bursts that
	// come from /route HTTP RTT variability (cache misses can stack up a
	// few hundred KB transiently).
	stuckRecvQBytes = 1 * 1024 * 1024

	// stuckSeconds: how many consecutive ticks above threshold before
	// we restart. With heartbeat=10s, this is 6 ticks = 60s of "queue
	// is full and not moving" — enough to be sure it s a real wedge
	// and not just a 10-second backlog burst.
	stuckSeconds = 60
)

// stuckCounter persists across ticks so the watchdog can detect
// sustained-above-threshold. We mutate it in checkKamailioStuck.
type stuckCounter struct {
	secondsAbove int
}

var watchdogState = &stuckCounter{}

// checkKamailioStuck reads /proc/net/udp for kamailio s SIP listening
// sockets (UDP port 5060) and returns the max recv-Q across all of
// them. If that max stays above stuckRecvQBytes long enough, we
// restart kamailio. Callers fire this every heartbeat tick (every
// HeartbeatSeconds seconds).
func checkKamailioStuck(heartbeatSeconds int) {
	maxQ, ok := readMaxRecvQ()
	if !ok {
		// /proc/net/udp not parseable or no kamailio listener — skip,
		// don t crash the heartbeat loop.
		return
	}
	if maxQ > stuckRecvQBytes {
		watchdogState.secondsAbove += heartbeatSeconds
		slog.Warn("kamailio recv-Q above threshold",
			"recv_q_bytes", maxQ, "consecutive_seconds", watchdogState.secondsAbove,
			"threshold_bytes", stuckRecvQBytes)
		if watchdogState.secondsAbove >= stuckSeconds {
			slog.Error("kamailio appears wedged, restarting",
				"recv_q_bytes", maxQ, "stuck_for_seconds", watchdogState.secondsAbove)
			if err := systemctlAction("kamailio", "restart"); err != nil {
				slog.Error("watchdog: kamailio restart failed", "err", err)
				// Don t reset the counter — we ll retry on the next tick.
				return
			}
			watchdogState.secondsAbove = 0
		}
	} else {
		// Healthy. Reset the counter.
		if watchdogState.secondsAbove > 0 {
			slog.Info("kamailio recv-Q back below threshold",
				"recv_q_bytes", maxQ, "was_stuck_for_seconds", watchdogState.secondsAbove)
		}
		watchdogState.secondsAbove = 0
	}
}

// readMaxRecvQ parses /proc/net/udp and returns the largest recv-Q
// (column 5 of the udp table) across all rows for local port 0x13C4
// (= 5060 in hex). Returns (0, false) if the file can t be read or
// no SIP-bound sockets are found.
func readMaxRecvQ() (int, bool) {
	f, err := os.Open("/proc/net/udp")
	if err != nil {
		return 0, false
	}
	defer f.Close()
	const sipPortHex = "13C4" // 5060

	max := 0
	found := false
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		fields := strings.Fields(line)
		// /proc/net/udp columns:
		// 0: sl  1: local_addr:port  2: rem_addr:port  3: state
		// 4: tx_queue:rx_queue  ...
		if len(fields) < 5 {
			continue
		}
		// local_addr:port like "9EDCB040:13C4" — we just check suffix.
		if !strings.HasSuffix(fields[1], ":"+sipPortHex) {
			continue
		}
		// rx_queue is hex after the colon in field 4.
		qparts := strings.Split(fields[4], ":")
		if len(qparts) != 2 {
			continue
		}
		rx, err := strconv.ParseInt(qparts[1], 16, 64)
		if err != nil {
			continue
		}
		found = true
		if int(rx) > max {
			max = int(rx)
		}
	}
	return max, found
}
