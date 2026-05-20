package agent

import (
	"fmt"
	"os/exec"
	"strings"
)

// systemctlAction runs `systemctl <action> [service]`. service may be empty
// for actions that target the system (e.g. "reboot").
func systemctlAction(service, action string) error {
	args := []string{action}
	if service != "" {
		args = append(args, service)
	}
	out, err := exec.Command("systemctl", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("systemctl %s %s: %w (%s)", action, service, err, strings.TrimSpace(string(out)))
	}
	return nil
}
