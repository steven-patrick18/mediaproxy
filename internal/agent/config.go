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
	CIDR                int           `yaml:"cidr"` // default 24
	ManagedNetplanFile  string        `yaml:"managed_netplan_file"`
	HeartbeatSeconds    int           `yaml:"heartbeat_seconds"`
	ProtectIPs          []string      `yaml:"protect_ips"`
	ProtectDefaultRoute bool          `yaml:"protect_default_route"`
	RTPEngineConfPath   string        `yaml:"rtpengine_conf_path"`
	KamailioListenPath  string        `yaml:"kamailio_listen_path"`
	HTTPTimeout         time.Duration `yaml:"http_timeout"`

	// RTPEngineNGListen: address rtpengine binds its NG control socket on.
	// On a media node where rtpengine is local-only, leave at default
	// "127.0.0.1:2223". On a media node that a remote SipProxy will talk
	// to, set to a reachable address (e.g. "0.0.0.0:2223" + UFW restriction,
	// or the node's management IP). Empty = default 127.0.0.1:2223.
	RTPEngineNGListen string `yaml:"rtpengine_ng_listen"`

	// RTPEngineSock: where Kamailio (on sip_proxy role) should send NG
	// commands. Format: "udp:host:port". Default "udp:127.0.0.1:2223"
	// (rtpengine running locally on the SipProxy). For split-host
	// topology where rtpengine lives on a separate MediaNode, set to
	// "udp:<media-node-ip>:2223" — and ensure the MediaNode's NG listen
	// + firewall allow it. When the value is the localhost default,
	// the template ALSO suppresses rtpengine_manage() calls (no point
	// calling a daemon that isn't there). Set to a non-localhost value
	// to enable rtpengine media handling in Kamailio.
	RTPEngineSock string `yaml:"rtpengine_sock"`

	// ReadOnly: if true, the agent only reports metrics + bound IPs and
	// never calls `ip addr add/del`, `netplan apply`, or touches
	// rtpengine/kamailio configs. Useful for running unprivileged
	// (development, smoke tests on a host that already has its own
	// networking). Defaults to false in production.
	ReadOnly bool `yaml:"read_only"`

	// AutoClaimMaxPrefix: enable "tight CIDR auto-discovery". When set to N,
	// the agent inspects each bound IP's netmask; if the prefix length is
	// >= N (i.e. the block is N bits or tighter), it enumerates every host
	// in that block and binds them on the interface. Default 26 covers
	// /26, /27, /28, /29, /30 — the typical dedicated-server "extra IP
	// block" sizes (RackNerd, OVH, Hetzner colo, etc.) — without
	// false-positive on cloud-VPS shared subnets (Vultr/DO/AWS use /20 or
	// /24 which won't trigger). Set to 0 to disable entirely (operator
	// uses Bulk add in the panel instead). Set to 24 to be aggressive
	// (claims any /24 it sees, dangerous on shared cloud subnets).
	AutoClaimMaxPrefix int `yaml:"auto_claim_max_prefix"`
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
	if c.RTPEngineNGListen == "" {
		c.RTPEngineNGListen = "127.0.0.1:2223"
	}
	if c.RTPEngineSock == "" {
		c.RTPEngineSock = "udp:127.0.0.1:2223"
	}
	// Zero in the file means "use default". Explicitly disabling is done
	// by setting auto_claim_max_prefix: -1 in YAML.
	if c.AutoClaimMaxPrefix == 0 {
		c.AutoClaimMaxPrefix = 26
	} else if c.AutoClaimMaxPrefix < 0 {
		c.AutoClaimMaxPrefix = 0
	}
	return &c, nil
}
