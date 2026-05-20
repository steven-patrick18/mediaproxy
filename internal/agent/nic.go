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

// ScanIPs returns all IPv4 addresses currently bound to iface.
func ScanIPs(iface string) ([]string, error) {
	out, err := exec.Command("ip", "-j", "addr", "show", "dev", iface).Output()
	if err != nil {
		return nil, fmt.Errorf("ip addr show: %w", err)
	}
	var links []nicLink
	if err := json.Unmarshal(out, &links); err != nil {
		return nil, fmt.Errorf("parse ip json: %w", err)
	}
	addrs := []string{}
	for _, l := range links {
		for _, a := range l.AddrInfo {
			if a.Family == "inet" {
				addrs = append(addrs, a.Local)
			}
		}
	}
	return addrs, nil
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
