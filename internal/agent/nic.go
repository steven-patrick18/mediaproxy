package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// nicAddr is a partial decode of `ip -j addr show`.
type nicLink struct {
	IfName   string `json:"ifname"`
	AddrInfo []struct {
		Family string `json:"family"`
		Local  string `json:"local"`
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
func AddIP(iface, ip string, cidr int) error {
	out, err := exec.Command("ip", "addr", "add", fmt.Sprintf("%s/%d", ip, cidr), "dev", iface).CombinedOutput()
	if err == nil {
		return nil
	}
	if strings.Contains(string(out), "File exists") {
		return nil
	}
	return fmt.Errorf("ip addr add %s: %w (%s)", ip, err, strings.TrimSpace(string(out)))
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
