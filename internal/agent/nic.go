package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os/exec"
	"strings"
	"time"
)

// nicAddr is a partial decode of `ip -j addr show`. Prefixlen is included
// so callers can also reason about the subnet mask of each bound address.
type nicLink struct {
	IfName   string `json:"ifname"`
	AddrInfo []struct {
		Family    string `json:"family"`
		Local     string `json:"local"`
		PrefixLen int    `json:"prefixlen"`
	} `json:"addr_info"`
}

// ScanIPs returns every unique IPv4 address bound on the host, across all
// interfaces. We deliberately scan everything (not just iface) because cloud
// providers commonly attach "extra IP" blocks to a second NIC — restricting
// the scan to one iface silently hid those IPs from the control plane.
//
// The iface argument is kept for the AddIP / RemoveIP reconcile path; ScanIPs
// itself ignores it. If iface is non-empty we still use it as a *preference*:
// IPs on that interface come first in the result list, so the operator's
// "primary" iface remains the conventional default in reports.
//
// We dedup (some kernels report the same address twice as primary + "scope
// host" secondary) and skip loopback / link-local.
func ScanIPs(iface string) ([]string, error) {
	out, err := exec.Command("ip", "-j", "addr", "show").Output()
	if err != nil {
		return nil, fmt.Errorf("ip addr show: %w", err)
	}
	var links []nicLink
	if err := json.Unmarshal(out, &links); err != nil {
		return nil, fmt.Errorf("parse ip json: %w", err)
	}
	seen := map[string]struct{}{}
	primary := []string{}
	rest := []string{}
	for _, l := range links {
		// Skip loopback by name as well — some kernels report 'lo' with
		// nothing in addr_info, others list 127.0.0.1.
		if l.IfName == "lo" {
			continue
		}
		for _, a := range l.AddrInfo {
			if a.Family != "inet" {
				continue
			}
			if a.Local == "" {
				continue
			}
			// Skip 127.0.0.0/8 and 169.254.0.0/16 — never managed.
			if strings.HasPrefix(a.Local, "127.") || strings.HasPrefix(a.Local, "169.254.") {
				continue
			}
			if _, dup := seen[a.Local]; dup {
				continue
			}
			seen[a.Local] = struct{}{}
			if iface != "" && l.IfName == iface {
				primary = append(primary, a.Local)
			} else {
				rest = append(rest, a.Local)
			}
		}
	}
	return append(primary, rest...), nil
}

// AddIP attaches ip/cidr to iface, treating "already exists" as success.
//
// CRITICAL safety guard: refuses to bind an IP that is currently the
// default-route next-hop ("via" IP). Many cloud / colo providers put the
// gateway inside the same CIDR block they sell you (e.g. RackNerd gives
// 67.215.233.64/26 with the gateway at .65). Binding the gateway as a
// local interface address breaks the default route: outbound packets
// resolve .65 to ourselves instead of going via the upstream router, and
// the host drops off the network in milliseconds.
//
// This check costs one cheap `ip route show default` invocation and is
// the difference between "agent works" and "host instantly dies" on
// every provider where the gateway is in-block.
func AddIP(iface, ip string, cidr int) error {
	if gw, _ := defaultGateway(); gw != "" && gw == ip {
		return fmt.Errorf("refusing to bind %s: it is the default-route gateway", ip)
	}
	out, err := exec.Command("ip", "addr", "add", fmt.Sprintf("%s/%d", ip, cidr), "dev", iface).CombinedOutput()
	if err == nil {
		return nil
	}
	if strings.Contains(string(out), "File exists") {
		return nil
	}
	return fmt.Errorf("ip addr add %s: %w (%s)", ip, err, strings.TrimSpace(string(out)))
}

// defaultGateway returns the next-hop IP for the IPv4 default route, or
// "" if there isn't one (or the parse fails). We parse `ip -j route show
// default` to keep this robust against minor format changes.
func defaultGateway() (string, error) {
	out, err := exec.Command("ip", "-j", "route", "show", "default").Output()
	if err != nil {
		return "", err
	}
	var routes []struct {
		Gateway string `json:"gateway"`
	}
	if err := json.Unmarshal(out, &routes); err != nil {
		return "", err
	}
	for _, r := range routes {
		if r.Gateway != "" {
			return r.Gateway, nil
		}
	}
	return "", nil
}

// RemoveIP detaches ip/cidr from iface, treating "not found" as success.
func RemoveIP(iface, ip string, cidr int) error {
	out, err := exec.Command("ip", "addr", "del", fmt.Sprintf("%s/%d", ip, cidr), "dev", iface).CombinedOutput()
	if err == nil {
		return nil
	}
	msg := string(out)
	if strings.Contains(msg, "Cannot assign requested address") ||
		strings.Contains(msg, "No such file or directory") ||
		strings.Contains(msg, "address not found") {
		return nil
	}
	return fmt.Errorf("ip addr del %s: %w (%s)", ip, err, strings.TrimSpace(msg))
}

// addIPThrottleMs is how long we sleep between individual `ip addr add`
// calls in batch operations. Sub-second is fine to avoid switch ARP
// storm/port-security trips at the upstream provider (RackNerd took us
// offline twice when 60+ IPs were added in a single burst). 100ms × 60 IPs
// = 6s total — acceptable.
const addIPThrottleMs = 100

// AutoClaimLocalBlocks looks at the netmask of every IP bound on iface and,
// for any block tighter than maxPrefix (e.g. maxPrefix=26 covers /26, /27,
// /28, /29, /30), binds every host in that block that isn't already bound.
//
// Why: dedicated-server providers (RackNerd, OVH, Hetzner colo) hand out
// "extra IP" allocations as a /26 or /27, configure ONLY the primary IP via
// cloud-init, and expect the operator to add the rest. This function makes
// the agent do that automatically — without ever guessing on cloud-VPS
// shared subnets (which are /20 or /24 and won't trigger when maxPrefix is
// 26 or tighter).
//
// Safety:
//   - maxPrefix outside [24, 32] is a no-op (refuses to enumerate huge
//     blocks even if the operator misconfigures).
//   - Loopback and link-local interfaces are skipped.
//   - Network + broadcast addresses are skipped.
//   - Errors from individual AddIP calls are logged via the returned
//     error count but never abort the whole pass.
//
// Returns (newlyClaimed, error).
func AutoClaimLocalBlocks(iface string, maxPrefix int) (int, error) {
	if maxPrefix < 24 || maxPrefix > 32 {
		return 0, nil
	}
	out, err := exec.Command("ip", "-j", "addr", "show").Output()
	if err != nil {
		return 0, fmt.Errorf("ip addr show: %w", err)
	}
	var links []nicLink
	if err := json.Unmarshal(out, &links); err != nil {
		return 0, fmt.Errorf("parse ip json: %w", err)
	}

	bound := map[string]bool{}
	for _, l := range links {
		for _, a := range l.AddrInfo {
			if a.Family == "inet" && a.Local != "" {
				bound[a.Local] = true
			}
		}
	}

	// Track which CIDRs we've already enumerated so we don't redo the same
	// block when two addresses sit inside it.
	seenBlocks := map[string]bool{}
	claimed := 0
	candidates := 0
	failed := 0
	matchedIfaceEntries := 0
	for _, l := range links {
		if l.IfName != iface {
			continue
		}
		for _, a := range l.AddrInfo {
			if a.Family != "inet" || a.Local == "" {
				continue
			}
			matchedIfaceEntries++
			if a.PrefixLen < maxPrefix || a.PrefixLen > 32 {
				continue
			}
			if strings.HasPrefix(a.Local, "127.") || strings.HasPrefix(a.Local, "169.254.") {
				continue
			}
			cidr := fmt.Sprintf("%s/%d", a.Local, a.PrefixLen)
			_, network, err := net.ParseCIDR(cidr)
			if err != nil {
				slog.Warn("auto-claim: ParseCIDR failed", "cidr", cidr, "err", err)
				continue
			}
			key := network.String()
			if seenBlocks[key] {
				continue
			}
			seenBlocks[key] = true
			hosts := hostsInCIDR(network)
			candidates += len(hosts)
			for _, host := range hosts {
				if bound[host] {
					continue
				}
				if err := AddIP(iface, host, a.PrefixLen); err != nil {
					failed++
					// Log only the first few so we don't flood the log on
					// genuinely broken setups (e.g. wrong gateway, full ARP
					// table). The summary at the end carries the count.
					if failed <= 3 {
						slog.Warn("auto-claim: AddIP failed", "iface", iface, "ip", host, "prefix", a.PrefixLen, "err", err)
					}
					continue
				}
				bound[host] = true
				claimed++
				// Throttle: gratuitous ARPs from a burst of ip addr add can
				// trip upstream switch port-security / ARP-storm protection
				// and drop the host from the network. 100ms × 60 IPs = 6s
				// total — slow enough that the switch is happy.
				time.Sleep(addIPThrottleMs * time.Millisecond)
			}
		}
	}
	// Always log a one-line summary — even claimed=0 is useful diagnostic info
	// the first time auto-claim runs on a node. Operators can grep this.
	slog.Info("auto-claim summary",
		"iface", iface, "max_prefix", maxPrefix,
		"iface_entries_matched", matchedIfaceEntries,
		"blocks_enumerated", len(seenBlocks),
		"candidates", candidates,
		"claimed", claimed,
		"failed", failed)
	return claimed, nil
}

// hostsInCIDR returns every usable host in network, skipping the network
// address and the broadcast address. For /31 and /32 it returns both/the
// single address (RFC 3021 + point-to-host conventions).
func hostsInCIDR(network *net.IPNet) []string {
	ones, bits := network.Mask.Size()
	if bits != 32 {
		return nil
	}
	if ones >= 31 {
		// /31 and /32: return every address; no network/broadcast in this convention.
		out := []string{}
		ip := network.IP.Mask(network.Mask).To4()
		if ip == nil {
			return nil
		}
		count := 1
		if ones == 31 {
			count = 2
		}
		for i := 0; i < count; i++ {
			a := make(net.IP, 4)
			copy(a, ip)
			a[3] += byte(i)
			out = append(out, a.String())
		}
		return out
	}
	// Standard case: skip the first (network) and last (broadcast) address.
	out := []string{}
	first := network.IP.Mask(network.Mask).To4()
	if first == nil {
		return nil
	}
	total := 1 << uint(32-ones)
	for i := 1; i < total-1; i++ {
		a := make(net.IP, 4)
		copy(a, first)
		// 32-bit add with carry, kept simple by treating as a uint32.
		v := uint32(a[0])<<24 | uint32(a[1])<<16 | uint32(a[2])<<8 | uint32(a[3])
		v += uint32(i)
		a[0] = byte(v >> 24)
		a[1] = byte(v >> 16)
		a[2] = byte(v >> 8)
		a[3] = byte(v)
		out = append(out, a.String())
	}
	return out
}

// HasDefaultRoute returns true if a default route is present (IPv4).
func HasDefaultRoute() (bool, error) {
	out, err := exec.Command("ip", "-4", "route", "show", "default").Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return false, nil
		}
		return false, fmt.Errorf("ip route show default: %w", err)
	}
	return strings.TrimSpace(string(out)) != "", nil
}
