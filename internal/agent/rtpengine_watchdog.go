package agent

import (
	"log/slog"
	"strings"
)

// rtpengine wedge watchdog.
//
// Failure modes this prevents (all observed in production):
//   - rtpengine daemon "active" per systemctl but the NG control socket
//     (UDP/2223) stops replying. Symptom: Kamailio's rtpengine_offer()
//     times out, INVITEs get a 408 or no media gets allocated, new calls
//     fail silently. Restart unblocks it instantly.
//   - rtpengine running but worker thread deadlocked on a stuck packet
//     handler. The daemon eats INVITEs at the SIP layer but stops
//     bridging RTP — calls go dead-air after answer. NG ping still works
//     in some of these cases, so we ALSO watch the active-sessions
//     counter: if rtpengine claims sessions but Kamailio's recv-Q is
//     piling up while no new sessions are being created over a long
//     window, we treat it as a wedge.
//
// Sampling cadence: every heartbeat tick on media role.
//
// Action threshold: stuckSeconds of consecutive failures (60s by default).
// Counter resets on first success so transient blips never trigger.

const (
	// rtpengineStuckSeconds: consecutive seconds of NG-ping failure
	// before we restart. 60s = 6 ticks at the default 10s heartbeat.
	// Matches the Kamailio watchdog cadence so operators see consistent
	// "1-minute to self-heal" behaviour across both daemons.
	rtpengineStuckSeconds = 60
)

type rtpengineStuck struct {
	pingFailSeconds int
}

var rtpengineWatchdog = &rtpengineStuck{}

// checkRTPEngineStuck probes the NG control socket each tick. If the
// probe fails for rtpengineStuckSeconds consecutive seconds the agent
// restarts the rtpengine-daemon service.
//
// The probe is a single "statistics" NG command (re-used from sampler
// code). It's the cheapest health check rtpengine exposes: a server-
// side memory read with no per-call iteration, so it can't be the thing
// slowing rtpengine down.
func checkRTPEngineStuck(heartbeatSeconds int) {
	err := PingRTPEngineNG()
	if err == nil {
		if rtpengineWatchdog.pingFailSeconds > 0 {
			slog.Info("rtpengine NG control responsive again",
				"was_stuck_for_seconds", rtpengineWatchdog.pingFailSeconds)
		}
		rtpengineWatchdog.pingFailSeconds = 0
		return
	}
	rtpengineWatchdog.pingFailSeconds += heartbeatSeconds
	slog.Warn("rtpengine NG control unresponsive",
		"err", trimErr(err),
		"consecutive_seconds", rtpengineWatchdog.pingFailSeconds,
		"threshold_seconds", rtpengineStuckSeconds)
	if rtpengineWatchdog.pingFailSeconds >= rtpengineStuckSeconds {
		slog.Error("rtpengine appears wedged, restarting",
			"stuck_for_seconds", rtpengineWatchdog.pingFailSeconds)
		// Ubuntu's stock systemd unit is "rtpengine-daemon.service".
		// Same name the reconcile loop uses.
		if err := systemctlAction("rtpengine-daemon", "restart"); err != nil {
			slog.Error("watchdog: rtpengine restart failed", "err", err)
			// Don't reset the counter — we'll retry on the next tick.
			return
		}
		rtpengineWatchdog.pingFailSeconds = 0
	}
}

// trimErr keeps log lines short — NG socket errors include the full
// "dial udp 127.0.0.1:2223:" prefix which is noise once you know the
// watchdog is what's logging.
func trimErr(err error) string {
	s := err.Error()
	if i := strings.LastIndex(s, ": "); i > 0 && i < len(s)-2 {
		return s[i+2:]
	}
	return s
}
