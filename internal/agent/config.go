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

	// --- scale-tuning knobs --------------------------------------------
	// These shape how big the Kamailio worker pool is and how much
	// per-node capacity you can squeeze out before needing a second
	// SipProxy / MediaNode. See TUNING.md for the capacity matrix.

	// KamailioChildren: number of UDP-handling worker processes per
	// listen socket. Each worker can handle one INVITE at a time while
	// the http_async_query is in flight (~100ms RTT to /route). Default
	// 16 sustains ~150 INVITE/s with the cache disabled, ~1500/s with
	// it enabled. Bump to 32 for >5k concurrent calls. Memory cost is
	// negligible (~5MB per worker). Set 0 to use the template default.
	KamailioChildren int `yaml:"kamailio_children"`

	// KamailioTCPChildren: TCP worker count. Most operators do UDP only;
	// 4 is plenty unless you have a carrier requiring TCP transport.
	KamailioTCPChildren int `yaml:"kamailio_tcp_children"`

	// RTPEnginePortMin / RTPEnginePortMax: UDP port range rtpengine uses
	// for RTP streams. Default 30000-60000 = 30k ports = ~7.5k concurrent
	// calls (each call uses 4 ports for RTP+RTCP × 2 directions).
	// For higher concurrency expand the range; the practical max is
	// 1024-65535 giving ~16k concurrent. Beyond that you need a second
	// MediaNode.
	RTPEnginePortMin int `yaml:"rtpengine_port_min"`
	RTPEnginePortMax int `yaml:"rtpengine_port_max"`

	// RouteCacheSeconds: TTL for the in-Kamailio /route lookup cache
	// (htable). When non-zero, repeated INVITEs from the same client
	// to the same DNIS (and matching prefix length) hit the cache
	// instead of round-tripping to the control plane. Default 5s —
	// safe for human-driven dialing (the routing rarely changes within
	// 5s and stale entries just self-expire). Set -1 to disable.
	RouteCacheSeconds int `yaml:"route_cache_seconds"`

	// RouteCacheKeyLen: how many leading digits of the DNIS to include
	// in the cache key. 0 (default) means full DNIS — safe regardless
	// of how your routes table is configured, lower hit rate (only
	// re-dials of the same number cache-hit). Lower this to 3 if your
	// routing rules are configured by country code (any 31xxx → carrier
	// A, any 33xxx → carrier B), to boost hit rate dramatically (>50%).
	// Going too low can cause WRONG routing if you have more specific
	// rules — see TUNING.md.
	RouteCacheKeyLen int `yaml:"route_cache_key_len"`

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
	if c.KamailioChildren <= 0 {
		c.KamailioChildren = 16
	}
	if c.KamailioTCPChildren <= 0 {
		c.KamailioTCPChildren = 4
	}
	if c.RTPEnginePortMin <= 0 {
		c.RTPEnginePortMin = 30000
	}
	if c.RTPEnginePortMax <= 0 {
		c.RTPEnginePortMax = 60000
	}
	// RouteCacheSeconds = 0 means disabled (legitimate value). To
	// distinguish "operator wants 0" from "operator left it blank",
	// use -1 in YAML to explicitly disable; default to 5 otherwise.
	if c.RouteCacheSeconds == 0 {
		c.RouteCacheSeconds = 5
	} else if c.RouteCacheSeconds < 0 {
		c.RouteCacheSeconds = 0
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
