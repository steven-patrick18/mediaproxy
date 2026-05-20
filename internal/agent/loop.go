package agent

import (
	"context"
	"log/slog"
	"os"
	"runtime"
	"time"
)

type Agent struct {
	Cfg    *Config
	API    *Client
	hostnm string
}

func New(cfg *Config) *Agent {
	h, _ := os.Hostname()
	return &Agent{
		Cfg:    cfg,
		API:    NewClient(cfg.ControlPlaneURL, cfg.AgentToken, cfg.HTTPTimeout),
		hostnm: h,
	}
}

// Run executes the boot sequence then enters the heartbeat loop.
// Returns only on ctx cancellation.
func (a *Agent) Run(ctx context.Context) error {
	if err := a.boot(ctx); err != nil {
		slog.Error("agent boot failed", "err", err)
		// continue into heartbeat loop anyway — control plane may come back
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
	slog.Info("registering with control plane", "url", a.Cfg.ControlPlaneURL, "node_id", a.Cfg.NodeID)
	dir, err := a.API.Register(ctx, RegisterReq{
		Hostname: a.hostnm,
		Cores:    runtime.NumCPU(),
	})
	if err != nil {
		return err
	}
	slog.Info("registered", "expected_ips", len(dir.ExpectedIPs))
	a.reconcile(ctx, dir.ExpectedIPs)
	return nil
}

func (a *Agent) tick(ctx context.Context) error {
	bound, err := ScanIPs(a.Cfg.Iface)
	if err != nil {
		return err
	}
	hb, err := a.API.Heartbeat(ctx, HeartbeatReq{
		BoundIPs: bound,
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
//   - add IPs that are expected but missing (self-heal)
//   - do NOT auto-delete extras; just report (control plane logs the drift)
func (a *Agent) reconcile(_ context.Context, expected []string) {
	bound, err := ScanIPs(a.Cfg.Iface)
	if err != nil {
		slog.Error("scan iface", "err", err)
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

	// Add missing
	missing := []string{}
	for e := range expectedSet {
		if !boundSet[e] {
			missing = append(missing, e)
		}
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

	// Extras — log only.
	for b := range boundSet {
		if protect[b] {
			continue
		}
		if !expectedSet[b] {
			slog.Warn("extra ip on nic (not in expected set)", "ip", b)
		}
	}

	// Persist + update services with the full expected set.
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
	var detail string
	status := "ok"
	switch cmd.Type {
	case "add_ip":
		if err := AddIP(a.Cfg.Iface, cmd.IP, cmd.CIDR); err != nil {
			status = "error"
			detail = err.Error()
		}
	case "remove_ip":
		for _, p := range a.Cfg.ProtectIPs {
			if p == cmd.IP {
				status = "error"
				detail = "ip is protected"
			}
		}
		if status == "ok" {
			if err := RemoveIP(a.Cfg.Iface, cmd.IP, cmd.CIDR); err != nil {
				status = "error"
				detail = err.Error()
			}
		}
	default:
		status = "error"
		detail = "unsupported command type: " + cmd.Type
	}
	_ = a.API.AckCommand(ctx, CommandResult{
		CommandID: cmd.ID,
		Status:    status,
		Detail:    detail,
	})
}
