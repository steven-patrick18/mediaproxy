package agent

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	NodeID              int64         `yaml:"node_id"`
	Role                string        `yaml:"role"` // "media" | "sip_proxy"
	ControlPlaneURL     string        `yaml:"control_plane_url"`
	AgentToken          string        `yaml:"agent_token"`
	Iface               string        `yaml:"iface"`
	CIDR                int           `yaml:"cidr"`           // default 24
	ManagedNetplanFile  string        `yaml:"managed_netplan_file"`
	HeartbeatSeconds    int           `yaml:"heartbeat_seconds"`
	ProtectIPs          []string      `yaml:"protect_ips"`
	ProtectDefaultRoute bool          `yaml:"protect_default_route"`
	RTPEngineConfPath   string        `yaml:"rtpengine_conf_path"`
	KamailioListenPath  string        `yaml:"kamailio_listen_path"`
	HTTPTimeout         time.Duration `yaml:"http_timeout"`
}

func LoadConfig(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var c Config
	if err := yaml.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if c.NodeID == 0 {
		return nil, fmt.Errorf("node_id is required")
	}
	if c.Role != "media" && c.Role != "sip_proxy" {
		return nil, fmt.Errorf("role must be media or sip_proxy")
	}
	if c.ControlPlaneURL == "" || c.AgentToken == "" {
		return nil, fmt.Errorf("control_plane_url and agent_token are required")
	}
	if c.Iface == "" {
		c.Iface = "eth0"
	}
	if c.CIDR == 0 {
		c.CIDR = 24
	}
	if c.ManagedNetplanFile == "" {
		c.ManagedNetplanFile = "/etc/netplan/90-managed-ips.yaml"
	}
	if c.HeartbeatSeconds == 0 {
		c.HeartbeatSeconds = 60
	}
	if c.RTPEngineConfPath == "" {
		c.RTPEngineConfPath = "/etc/rtpengine/rtpengine.conf"
	}
	if c.KamailioListenPath == "" {
		c.KamailioListenPath = "/etc/kamailio/listen.cfg"
	}
	if c.HTTPTimeout == 0 {
		c.HTTPTimeout = 10 * time.Second
	}
	return &c, nil
}
