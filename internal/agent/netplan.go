package agent

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
)

// WriteNetplan rewrites the managed netplan drop-in to contain exactly `ips`
// on `iface`. Idempotent. Calls `netplan apply` after writing.
func WriteNetplan(path, iface string, cidr int, ips []string) error {
	sorted := append([]string(nil), ips...)
	sort.Strings(sorted)

	var b strings.Builder
	b.WriteString("# Managed by node-agent. Do not edit by hand.\n")
	b.WriteString("network:\n")
	b.WriteString("  version: 2\n")
	b.WriteString("  ethernets:\n")
	b.WriteString(fmt.Sprintf("    %s:\n", iface))
	if len(sorted) == 0 {
		b.WriteString("      addresses: []\n")
	} else {
		b.WriteString("      addresses:\n")
		for _, ip := range sorted {
			b.WriteString(fmt.Sprintf("        - %s/%d\n", ip, cidr))
		}
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(b.String()), 0o600); err != nil {
		return fmt.Errorf("write tmp netplan: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("rename netplan: %w", err)
	}
	if out, err := exec.Command("netplan", "apply").CombinedOutput(); err != nil {
		return fmt.Errorf("netplan apply: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}
