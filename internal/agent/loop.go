package agent

import (
	"context"
	"log/slog"
	"os"
	"runtime"
	"time"
)

type Agent struct {
	Cfg     *Config
	API     *Client
	hostnm  string
	sampler *Sampler
}

func New(cfg *Config) *Agent {
	h, _ := os.Hostname()
	return &Agent{
		Cfg:     cfg,
		API:     NewClient(cfg.ControlPlaneURL, cfg.AgentToken, cfg.HTTPTimeout),
		hostnm:  h,
		sampler: NewSampler(cfg.Iface),
	}
}

func (a *Agent) Run(ctx context.Context) error {
	if err := a.boot(ctx); err != nil {
		slog.Error("agent boot failed", "err", err)
	}

	t := time.NewTicker(time.Duration(a.Cfg.HeartbeatSeconds) * time.Second)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
			if err := a.tick(ctx); err != nil {
				slog.Error("heartbeat tick", "err", err)
			}
		}
	}
}

func (a *Agent) boot(ctx context.Context) error {
	slog.Info("registering with control plane",
		"url", a.Cfg.ControlPlaneURL, "node_id", a.Cfg.NodeID, "read_only", a.Cfg.ReadOnly)
	dir, err := a.API.Register(ctx, RegisterReq{
		Hostname:     a.hostnm,
		Cores:        runtime.NumCPU(),
		AgentVersion: Version,
	})
	if err != nil {
		return err
	}
	slog.Info("registered", "expected_ips", len(dir.ExpectedIPs))
	// Prime the CPU + network samplers so the first real tick has deltas.
	_ = a.sampler.Sample()
	// Boot reconcile: no heartbeat yet, so no tunables / rtpengine sets.
	// Render with whatever the agent has in cfg.yaml; the first real
	// heartbeat will deliver server-side overrides and trigger a regen.
	a.reconcile(ctx, &HeartbeatResp{ExpectedIPs: dir.ExpectedIPs})
	return nil
}

func (a *Agent) tick(ctx context.Context) error {
	// Worker-stuck watchdog (sip_proxy only). Checks Kamailio s UDP recv-Q
	// every tick. If the queue stays full for >60s the agent restarts
	// kamailio, healing the wedge without operator intervention.
	if a.Cfg.Role == "sip_proxy" && !a.Cfg.ReadOnly {
		checkKamailioStuck(a.Cfg.HeartbeatSeconds)
	}

	// rtpengine wedge watchdog (media only). Probes the NG control
	// socket; if it stops replying for >60s, restart rtpengine-daemon.
	// Fixes the "rtpengine active per systemctl but not relaying" case
	// where Kamailio's rtpengine_offer() times out and INVITEs silently
	// fail mid-flight.
	if a.Cfg.Role == "media" && !a.Cfg.ReadOnly {
		checkRTPEngineStuck(a.Cfg.HeartbeatSeconds)
	}

	// Before we scan, opportunistically bind every host in tight CIDR blocks
	// the kernel knows about. This is what makes a dedicated-server with an
	// "extra IP block" (e.g. RackNerd /26) self-populate without any panel
	// step. Disabled if AutoClaimMaxPrefix is 0; safe at default 26 because
	// cloud-VPS shared subnets are /20-/24 which don't trigger.
	if !a.Cfg.ReadOnly && a.Cfg.AutoClaimMaxPrefix > 0 {
		// AutoClaimLocalBlocks logs its own summary every call, including
		// when nothing matched (so operators can tell whether the code path
		// even ran). We only need to surface a hard error here.
		if _, err := AutoClaimLocalBlocks(a.Cfg.Iface, a.Cfg.AutoClaimMaxPrefix); err != nil {
			slog.Warn("auto-claim local CIDR blocks failed", "err", err)
		}
	}

	bound, err := ScanIPs(a.Cfg.Iface)
	if err != nil {
		// On a host without the configured iface, just report empty.
		slog.Warn("scan iface", "err", err)
		bound = []string{}
	}
	snap := a.sampler.Sample()

	// Only media nodes run rtpengine; querying on a sip_proxy would just
	// hit a closed UDP port and waste a second per heartbeat.
	activeCalls := 0
	if a.Cfg.Role == "media" {
		if n, err := QueryRTPEngineSessions(); err != nil {
			slog.Debug("rtpengine session query failed", "err", err)
		} else {
			activeCalls = n
		}
	}

	hb, err := a.API.Heartbeat(ctx, HeartbeatReq{
		BoundIPs:      bound,
		ActiveCalls:   activeCalls,
		CPUPct:        snap.CPUPct,
		RAMPct:        snap.RAMPct,
		NetInMbps:     snap.NetInMbps,
		NetOutMbps:    snap.NetOutMbps,
		UptimeSeconds: snap.UptimeSeconds,
		AgentVersion:  Version,
	})
	if err != nil {
		return err
	}
	a.reconcile(ctx, hb)
	for _, cmd := range hb.Commands {
		a.runCommand(ctx, cmd)
	}

	// Phase 3 of route quality: per-call RTP stats. Media-role only, since
	// only media nodes run rtpengine. Best-effort — failure here never
	// touches the heartbeat path.
	if a.Cfg.Role == "media" {
		if entries := SampleRTPEngineQuality(); len(entries) > 0 {
			if err := a.API.PostCallQuality(ctx, entries); err != nil {
				slog.Debug("post call-quality failed", "err", err, "samples", len(entries))
			}
		}
	}
	return nil
}

// reconcile makes the NIC + persistence layers match the expected set.
// Safety rules:
//   - never touch IPs in ProtectIPs
//   - in read_only mode, only log drift, never change anything
//   - add IPs that are expected but missing (self-heal)
//   - never auto-delete extras; just report
//
// Now also carries through the heartbeat's tunables + rtpengine sets so
// persistAndServices can render kamailio.cfg with operator-controlled
// values (workers count, cache TTL, multi-MediaNode map).
func (a *Agent) reconcile(_ context.Context, hb *HeartbeatResp) {
	expected := hb.ExpectedIPs
	bound, err := ScanIPs(a.Cfg.Iface)
	if err != nil {
		// Already warned in tick(); avoid a second log line here.
		return
	}
	protect := map[string]bool{}
	for _, p := range a.Cfg.ProtectIPs {
		protect[p] = true
	}
	boundSet := map[string]bool{}
	for _, b := range bound {
		boundSet[b] = true
	}
	expectedSet := map[string]bool{}
	for _, e := range expected {
		expectedSet[e] = true
	}

	missing := []string{}
	for e := range expectedSet {
		if !boundSet[e] {
			missing = append(missing, e)
		}
	}
	extras := []string{}
	for b := range boundSet {
		if !expectedSet[b] && !protect[b] {
			extras = append(extras, b)
		}
	}

	if a.Cfg.ReadOnly {
		if len(missing) > 0 {
			slog.Info("[read-only] would add missing IPs", "ips", missing)
		}
		if len(extras) > 0 {
			slog.Debug("[read-only] extra IPs on nic (not in expected set)", "ips", extras)
		}
		return
	}

	for i, ip := range missing {
		if protect[ip] {
			continue
		}
		if err := AddIP(a.Cfg.Iface, ip, a.Cfg.CIDR); err != nil {
			slog.Error("add ip", "ip", ip, "err", err)
			continue
		}
		slog.Info("added ip", "ip", ip)
		// Throttle between adds — see comment on addIPThrottleMs in nic.go.
		// Skip sleep on the last one so single-IP adds aren't slowed.
		if i < len(missing)-1 {
			time.Sleep(addIPThrottleMs * time.Millisecond)
		}
	}
	for _, ip := range extras {
		slog.Warn("extra ip on nic (not in expected set)", "ip", ip)
	}

	persistAndServices(a.Cfg, expected, hb.Tunables, hb.RTPEngineSets)
}

func persistAndServices(cfg *Config, expected []string, tunables Tunables, rtpengineSets []RTPEngineSet) {
	if err := WriteNetplan(cfg.ManagedNetplanFile, cfg.Iface, cfg.CIDR, expected); err != nil {
		slog.Error("write netplan", "err", err)
	}
	switch cfg.Role {
	case "media":
		// Only bounce rtpengine when the config actually changed. Kamailio's
		// & rtpengine's stock systemd units don't support real reload, so
		// every reload-or-restart is a full restart — without this guard
		// the agent would cycle the daemon every heartbeat (~10s) in
		// steady state and miss every other Asterisk qualify.
		body := GenRTPEngineConfig(expected, cfg.RTPEngineNGListen, cfg.RTPEnginePortMin, cfg.RTPEnginePortMax)
		changed, err := WriteRTPEngineConfig(cfg.RTPEngineConfPath, body)
		if err != nil {
			slog.Error("rtpengine write", "err", err)
		} else if changed {
			slog.Info("rtpengine config changed, restarting", "ips", len(expected))
			// Ubuntu's stock rtpengine package ships the daemon as
			// "rtpengine-daemon.service"; we still keep "rtpengine"
			// as an alias check in case future packagers rename it.
			if err := systemctlAction("rtpengine-daemon", "reload-or-restart"); err != nil {
				slog.Warn("rtpengine reload", "err", err)
			}
		}
	case "sip_proxy":
		// Server-side tunables (heartbeat-delivered) override the
		// YAML-config defaults. Operator-set values in the panel take
		// precedence over what s in /etc/node-agent/config.yaml — that
		// way you can adjust workers / cache TTL from the GUI without
		// SSHing in to edit YAML. nil = "no override, use yaml default".
		children := cfg.KamailioChildren
		if tunables.KamailioWorkers != nil && *tunables.KamailioWorkers > 0 {
			children = *tunables.KamailioWorkers
		}
		cacheSeconds := cfg.RouteCacheSeconds
		if tunables.RouteCacheSeconds != nil {
			cacheSeconds = *tunables.RouteCacheSeconds
		}
		cacheKeyLen := cfg.RouteCacheKeyLen
		if tunables.RouteCacheKeyLen != nil {
			cacheKeyLen = *tunables.RouteCacheKeyLen
		}

		// Convert rtpengine sets to the format the template expects.
		// Sets are pure pass-through; the template renders one
		// modparam("rtpengine", "rtpengine_sock", "<id> == <sock>") per
		// entry, and /route delivers media_node_id which matches set_id.
		var rtpSets []KamailioRTPEngineSet
		for _, s := range rtpengineSets {
			rtpSets = append(rtpSets, KamailioRTPEngineSet{SetID: s.SetID, Sock: s.Sock})
		}

		listenCfg, mainCfg := GenKamailioConfig(expected, cfg.ControlPlaneURL, cfg.AgentToken, cfg.NodeID, cfg.RTPEngineSock,
			KamailioGenOpts{
				Children:          children,
				TCPChildren:       cfg.KamailioTCPChildren,
				RouteCacheSeconds: cacheSeconds,
				RouteCacheKeyLen:  cacheKeyLen,
				RTPEngineSets:     rtpSets,
			})
		changed, err := WriteKamailioConfigs(cfg.KamailioListenPath, "/etc/kamailio/kamailio.cfg", listenCfg, mainCfg)
		if err != nil {
			slog.Error("kamailio write", "err", err)
		} else if changed {
			slog.Info("kamailio config changed, restarting", "ips", len(expected))
			if err := systemctlAction("kamailio", "reload-or-restart"); err != nil {
				slog.Warn("kamailio reload", "err", err)
			}
		}
	}
}

func (a *Agent) runCommand(ctx context.Context, cmd Command) {
	if a.Cfg.ReadOnly && cmd.Type != "apply" {
		_ = a.API.AckCommand(ctx, CommandResult{CommandID: cmd.ID, Status: "error", Detail: "agent is read-only"})
		return
	}
	var detail string
	status := "ok"
	switch cmd.Type {
	case "add_ip":
		if err := AddIP(a.Cfg.Iface, cmd.IP, cmd.CIDR); err != nil {
			status, detail = "error", err.Error()
		}
	case "remove_ip":
		for _, p := range a.Cfg.ProtectIPs {
			if p == cmd.IP {
				status, detail = "error", "ip is protected"
			}
		}
		if status == "ok" {
			if err := RemoveIP(a.Cfg.Iface, cmd.IP, cmd.CIDR); err != nil {
				status, detail = "error", err.Error()
			}
		}
	case "apply":
		// Trigger an immediate reconcile against the latest expected set.
		// We don't have the set here — the next reconcile call in the
		// next heartbeat tick will pick it up. For now just nudge.
		slog.Info("apply requested — will reconcile on this tick")
		detail = "reconcile scheduled"
	case "apply_firewall":
		msg, err := a.applyFirewall(ctx)
		if err != nil {
			status, detail = "error", err.Error()
		} else {
			detail = msg
		}
	case "restart_rtpengine":
		if err := systemctlAction("rtpengine-daemon", "restart"); err != nil {
			status, detail = "error", err.Error()
		}
	case "restart_kamailio":
		if err := systemctlAction("kamailio", "restart"); err != nil {
			status, detail = "error", err.Error()
		}
	case "reboot":
		// Ack first, then reboot in a goroutine so the response reaches the
		// control plane before the network goes down.
		_ = a.API.AckCommand(ctx, CommandResult{CommandID: cmd.ID, Status: "ok", Detail: "rebooting"})
		go func() {
			time.Sleep(2 * time.Second)
			_ = systemctlAction("", "reboot")
		}()
		return
	default:
		status, detail = "error", "unsupported command type: " + cmd.Type
	}
	_ = a.API.AckCommand(ctx, CommandResult{CommandID: cmd.ID, Status: status, Detail: detail})
}
