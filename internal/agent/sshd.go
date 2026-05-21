package agent

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// SetPasswordAuth toggles sshd's PasswordAuthentication directive (and
// PermitRootLogin) on the host. Strategy:
//
//   1. Write our desired directives to /etc/ssh/sshd_config.d/01-mediaproxy.conf.
//      OpenSSH uses FIRST-match-wins per directive across drop-in files,
//      processed in lexical (numerical) order. We sort early (01-) so we
//      beat distro defaults like 50-cloud-init.conf, which on fresh Ubuntu
//      cloud images sets PasswordAuthentication=yes and would otherwise
//      win over a later 99-* file.
//   2. Clean up any legacy 99-mediaproxy.conf left by older agents so the
//      two files don't drift.
//   3. `sshd -t` to syntax-check before reloading.
//   4. `systemctl reload ssh` to pick up the change without dropping
//      existing sessions.
//
// PubkeyAuthentication is always set to yes so the agent never locks
// itself out of an admin's key-based SSH access.
func SetPasswordAuth(enable bool) error {
	const dropIn = "/etc/ssh/sshd_config.d/01-mediaproxy.conf"
	const legacyDropIn = "/etc/ssh/sshd_config.d/99-mediaproxy.conf"

	if err := os.MkdirAll("/etc/ssh/sshd_config.d", 0o755); err != nil {
		return fmt.Errorf("mkdir sshd_config.d: %w", err)
	}

	var b strings.Builder
	fmt.Fprintln(&b, "# Managed by mediaproxy node-agent. Do not edit by hand.")
	fmt.Fprintln(&b, "PubkeyAuthentication yes")
	if enable {
		fmt.Fprintln(&b, "PasswordAuthentication yes")
		fmt.Fprintln(&b, "PermitRootLogin yes")
		fmt.Fprintln(&b, "KbdInteractiveAuthentication yes")
		fmt.Fprintln(&b, "ChallengeResponseAuthentication yes")
	} else {
		fmt.Fprintln(&b, "PasswordAuthentication no")
		fmt.Fprintln(&b, "PermitRootLogin prohibit-password")
		fmt.Fprintln(&b, "KbdInteractiveAuthentication no")
		fmt.Fprintln(&b, "ChallengeResponseAuthentication no")
	}
	// Write atomically.
	tmp := dropIn + ".tmp"
	if err := os.WriteFile(tmp, []byte(b.String()), 0o644); err != nil {
		return fmt.Errorf("write drop-in: %w", err)
	}
	if err := os.Rename(tmp, dropIn); err != nil {
		return fmt.Errorf("rename drop-in: %w", err)
	}
	// Drop any leftover 99-mediaproxy.conf from an older agent build so the
	// 01-* file is unambiguously authoritative.
	_ = os.Remove(legacyDropIn)

	// Syntax check.
	if out, err := exec.Command("sshd", "-t").CombinedOutput(); err != nil {
		// Roll back: remove our drop-in so a future reload doesn't break.
		_ = os.Remove(dropIn)
		return fmt.Errorf("sshd -t rejected new config: %s", strings.TrimSpace(string(out)))
	}

	// Reload — does NOT drop existing sessions.
	if out, err := exec.Command("systemctl", "reload", "ssh").CombinedOutput(); err != nil {
		// Fall back to ssh.service name (some distros use just 'sshd').
		if out2, err2 := exec.Command("systemctl", "reload", "sshd").CombinedOutput(); err2 != nil {
			return fmt.Errorf("systemctl reload ssh: %s / sshd: %s",
				strings.TrimSpace(string(out)), strings.TrimSpace(string(out2)))
		}
	}
	return nil
}

// VerifyPasswordAuth returns the effective PasswordAuthentication value
// after sshd processes its includes. Useful in heartbeat reporting so
// the UI can show the actual state (not just the desired state).
func VerifyPasswordAuth() (bool, error) {
	out, err := exec.Command("sshd", "-T").Output()
	if err != nil {
		return false, err
	}
	sc := bufio.NewScanner(strings.NewReader(string(out)))
	for sc.Scan() {
		line := strings.ToLower(strings.TrimSpace(sc.Text()))
		if strings.HasPrefix(line, "passwordauthentication ") {
			return strings.HasSuffix(line, " yes"), nil
		}
	}
	return false, fmt.Errorf("no PasswordAuthentication line in sshd -T")
}

// Sleep wraps time.Sleep so test code can override.
var Sleep = func(d time.Duration) { time.Sleep(d) }
