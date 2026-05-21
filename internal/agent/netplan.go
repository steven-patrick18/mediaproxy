package agent

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

// WriteNetplan rewrites the managed netplan drop-in to contain exactly `ips`
// on `iface`. Idempotent. Provides reboot-time persistence ONLY — never calls
// `netplan apply`.
//
// Why no apply: `netplan apply` reconfigures the interface from the full
// merged set of /etc/netplan/*.yaml files. Any runtime address that isn't
// in ANY netplan file (commonly the case for /24 secondaries that cloud
// providers add via cloud-init scripts or DHCP enrich) gets removed, which
// can break ARP/routing for the broader subnet and take the box offline.
//
// Runtime correctness is maintained by the agent's `ip addr add` calls in
// AddIP / AutoClaimLocalBlocks. This file only matters at the next reboot,
// when netplan loads it alongside the distro's other netplan files. The
// merged set always includes cloud-init's primary-IP config, so the box
// stays reachable.
//
// Operators who want to verify the persisted set is correct can run
// `sudo netplan generate` (renders without applying) and inspect
// /run/systemd/network/.
func WriteNetplan(path, iface string, cidr int, ips []string) error {
	sorted := append([]string(nil), ips...)
	sort.Strings(sorted)

	var b strings.Builder
	b.WriteString("# Managed by node-agent. Do not edit by hand.\n")
	b.WriteString("# Loaded by netplan at boot, MERGED with cloud-init's config.\n")
	b.WriteString("# Runtime IP add/remove happens via 'ip addr' — this file is reboot-only.\n")
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
	return nil
}
