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
	a.reconcile(ctx, dir.ExpectedIPs)
	return nil
}

func (a *Agent) tick(ctx context.Context) error {
	bound, err := ScanIPs(a.Cfg.Iface)
	if err != nil {
		// On a host without the configured iface, just report empty.
		slog.Warn("scan iface", "err", err)
		bound = []string{}
	}
	snap := a.sampler.Sample()

	hb, err := a.API.Heartbeat(ctx, HeartbeatReq{
		BoundIPs:      bound,
		ActiveCalls:   0, // TODO: query rtpengine when role=media
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
	a.reconcile(ctx, hb.ExpectedIPs)
	for _, cmd := range hb.Commands {
		a.runCommand(ctx, cmd)
	}
	return nil
}

// reconcile makes the NIC + persistence layers match the expected set.
// Safety rules:
//   - never touch IPs in ProtectIPs
//   - in read_only mode, only log drift, never change anything
//   - add IPs that are expected but missing (self-heal)
//   - never auto-delete extras; just report
func (a *Agent) reconcile(_ context.Context, expected []string) {
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

	for _, ip := range missing {
		if protect[ip] {
			continue
		}
		if err := AddIP(a.Cfg.Iface, ip, a.Cfg.CIDR); err != nil {
			slog.Error("add ip", "ip", ip, "err", err)
			continue
		}
		slog.Info("added ip", "ip", ip)
	}
	for _, ip := range extras {
		slog.Warn("extra ip on nic (not in expected set)", "ip", ip)
	}

	persistAndServices(a.Cfg, expected)
}

func persistAndServices(cfg *Config, expected []string) {
	if err := WriteNetplan(cfg.ManagedNetplanFile, cfg.Iface, cfg.CIDR, expected); err != nil {
		slog.Error("write netplan", "err", err)
	}
	switch cfg.Role {
	case "media":
		if err := UpdateRTPEngineInterfaces(cfg.RTPEngineConfPath, expected); err != nil {
			slog.Error("rtpengine update", "err", err)
		}
	case "sip_proxy":
		if err := UpdateKamailioListen(cfg.KamailioListenPath, expected); err != nil {
			slog.Error("kamailio listen update", "err", err)
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
		if err := systemctlAction("rtpengine", "restart"); err != nil {
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
