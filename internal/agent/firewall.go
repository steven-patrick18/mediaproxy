package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// applyFirewall fetches the synthesized nftables config from the control
// plane, runs it through a confirm-or-rollback dance, and reports the
// outcome. If the new config breaks connectivity to the base-app, an `at`
// job restores the previous ruleset 90 seconds later.
func (a *Agent) applyFirewall(ctx context.Context) (string, error) {
	const (
		cfgPath       = "/etc/mediaproxy/firewall.nft"
		rollbackPath  = "/etc/mediaproxy/firewall.rollback.nft"
		rollbackAfter = "now + 2 minutes"
	)
	if err := os.MkdirAll("/etc/mediaproxy", 0o755); err != nil {
		return "", fmt.Errorf("mkdir /etc/mediaproxy: %w", err)
	}

	// 1) Fetch the desired config from the base-app.
	cfgText, err := a.fetchFirewallConfig(ctx)
	if err != nil {
		return "", fmt.Errorf("fetch config: %w", err)
	}

	// 2) Save current ruleset to a rollback file. If something on the
	// box doesn't have nftables yet, this is just an empty ruleset and
	// reverting "drops everything" — which is *worse* than the broken
	// config. So we bail out if nft is missing instead of pretending.
	if _, err := exec.LookPath("nft"); err != nil {
		return "", fmt.Errorf("nft binary not found; apt install nftables on the node first")
	}
	curRules, err := exec.Command("nft", "list", "ruleset").Output()
	if err != nil {
		return "", fmt.Errorf("read current ruleset: %w", err)
	}
	if err := os.WriteFile(rollbackPath, []byte("flush ruleset\n"+string(curRules)), 0o600); err != nil {
		return "", fmt.Errorf("save rollback: %w", err)
	}

	// 3) Write the new config.
	if err := os.WriteFile(cfgPath, []byte(cfgText), 0o600); err != nil {
		return "", fmt.Errorf("write firewall.nft: %w", err)
	}

	// 4) Syntax-check before doing anything reversible-via-at.
	if out, err := exec.Command("nft", "-c", "-f", cfgPath).CombinedOutput(); err != nil {
		return "", fmt.Errorf("nft -c failed (config rejected): %s", strings.TrimSpace(string(out)))
	}

	// 5) Schedule a revert via `at`. If we can't schedule it, refuse to apply.
	if _, err := exec.LookPath("at"); err != nil {
		return "", fmt.Errorf("at binary not found; apt install at on the node first (needed for safety-revert)")
	}
	atCmd := exec.Command("at", rollbackAfter)
	atCmd.Stdin = strings.NewReader(fmt.Sprintf("nft -f %s\n", rollbackPath))
	var atOut bytes.Buffer
	atCmd.Stderr = &atOut
	if err := atCmd.Run(); err != nil {
		return "", fmt.Errorf("schedule revert: %w (%s)", err, strings.TrimSpace(atOut.String()))
	}
	jobID := parseAtJobID(atOut.String())

	cancelRevert := func() {
		if jobID != "" {
			_ = exec.Command("atrm", jobID).Run()
		}
	}

	// 6) Actually load the new ruleset.
	if out, err := exec.Command("nft", "-f", cfgPath).CombinedOutput(); err != nil {
		cancelRevert() // rules never loaded; no point reverting nothing
		return "", fmt.Errorf("nft -f failed: %s", strings.TrimSpace(string(out)))
	}

	// 7) Confirm we can still reach the base-app. If we can't, leave the
	// revert scheduled — it will save us in 2 minutes.
	confirmCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := a.confirmFirewallApplied(confirmCtx); err != nil {
		return "", fmt.Errorf("applied OK but base-app unreachable (revert scheduled in 2 min): %w", err)
	}

	// 8) All good — cancel the scheduled revert.
	cancelRevert()
	return fmt.Sprintf("firewall applied; %d bytes loaded; rollback canceled (at job %s)", len(cfgText), jobID), nil
}

func (a *Agent) fetchFirewallConfig(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", a.Cfg.ControlPlaneURL+"/api/v1/agent/firewall", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+a.Cfg.AgentToken)
	res, err := a.API.httpC.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return "", fmt.Errorf("base-app returned %d", res.StatusCode)
	}
	var payload struct {
		NFTConfig string `json:"nft_config"`
	}
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		return "", err
	}
	if payload.NFTConfig == "" {
		return "", fmt.Errorf("empty nft_config in response")
	}
	return payload.NFTConfig, nil
}

func (a *Agent) confirmFirewallApplied(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "POST", a.Cfg.ControlPlaneURL+"/api/v1/agent/firewall-applied", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+a.Cfg.AgentToken)
	res, err := a.API.httpC.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		return fmt.Errorf("base-app returned %d", res.StatusCode)
	}
	return nil
}

// at writes "job N at <date>" to stderr. Extract N.
var atJobRE = regexp.MustCompile(`job (\d+) at`)

func parseAtJobID(out string) string {
	if m := atJobRE.FindStringSubmatch(out); len(m) > 1 {
		return m[1]
	}
	return ""
}
