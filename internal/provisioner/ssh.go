// Package provisioner installs the node-agent on a remote Linux host
// over SSH. The base-app serves the agent binary at /agent-binary, so the
// remote needs only outbound HTTPS to fetch it.
package provisioner

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

type Request struct {
	Host           string // 1.2.3.4
	Port           int    // 22
	User           string // root
	Password       string // SSH password (used only in memory)
	NodeID         int64
	Role           string // media | sip_proxy
	AgentToken     string
	ControlPlaneURL string
	BinaryURL      string // e.g. https://mediaproxy.voipzap.com/agent-binary
}

type Result struct {
	Log string
	OK  bool
}

func Run(ctx context.Context, r Request) Result {
	var b strings.Builder
	log := func(format string, args ...any) {
		fmt.Fprintf(&b, "[+] "+format+"\n", args...)
	}
	fail := func(format string, args ...any) Result {
		fmt.Fprintf(&b, "[!] "+format+"\n", args...)
		return Result{Log: b.String(), OK: false}
	}

	port := r.Port
	if port == 0 {
		port = 22
	}

	cfg := &ssh.ClientConfig{
		User: r.User,
		Auth: []ssh.AuthMethod{ssh.Password(r.Password)},
		// We accept any host key here because the operator is establishing
		// trust by entering a root password.
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	log("Connecting to %s@%s:%d", r.User, r.Host, port)
	dialer := net.Dialer{Timeout: 10 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", fmt.Sprintf("%s:%d", r.Host, port))
	if err != nil {
		return fail("dial: %v", err)
	}
	sshConn, chans, reqs, err := ssh.NewClientConn(conn, fmt.Sprintf("%s:%d", r.Host, port), cfg)
	if err != nil {
		return fail("ssh handshake: %v", err)
	}
	client := ssh.NewClient(sshConn, chans, reqs)
	defer client.Close()

	if err := run(client, "uname -m", &b); err != nil {
		return fail("uname: %v", err)
	}
	if err := run(client, "lsb_release -d || cat /etc/os-release | head -2", &b); err != nil {
		return fail("os check: %v", err)
	}

	log("Preparing directories")
	if err := run(client,
		"mkdir -p /etc/node-agent /var/log/mediaproxy /etc/mediaproxy && touch /var/log/mediaproxy/agent.log",
		&b); err != nil {
		return fail("mkdir: %v", err)
	}

	log("Installing nftables + at (firewall auto-apply needs these)")
	if err := run(client,
		"DEBIAN_FRONTEND=noninteractive apt-get update -qq && DEBIAN_FRONTEND=noninteractive apt-get install -y -qq nftables at && systemctl enable --now atd",
		&b); err != nil {
		// Non-fatal — the agent and SIP service still work without these,
		// just the firewall-apply command won't.
		log("WARNING: apt install failed (continuing): %v", err)
	}

	log("Downloading agent binary from %s", r.BinaryURL)
	// Download to /tmp first then atomically install. This avoids
	// curl exit 23 ("write error to destination") that some hosts hit
	// when piping a redirected download straight into /usr/local/bin
	// (the parent dir may be missing, mount may be noexec on first boot, etc.).
	dl := fmt.Sprintf(`set -e
mkdir -p /usr/local/bin
TMP=$(mktemp /tmp/node-agent.XXXXXX)
curl -fSL --retry 3 --retry-delay 2 -o "$TMP" %q
install -m 0755 "$TMP" /usr/local/bin/node-agent
rm -f "$TMP"
/usr/local/bin/node-agent --help >/dev/null 2>&1 || true
ls -l /usr/local/bin/node-agent`, r.BinaryURL)
	if err := run(client, dl, &b); err != nil {
		return fail("download agent: %v", err)
	}

	log("Writing /etc/node-agent/config.yaml")
	yaml := fmt.Sprintf(`node_id: %d
role: %s
control_plane_url: %s
agent_token: "%s"
iface: %s
read_only: false
heartbeat_seconds: 10
protect_ips: ["%s"]
`, r.NodeID, r.Role, r.ControlPlaneURL, r.AgentToken, "eth0", r.Host)
	// Detect the primary iface — replace eth0 with whatever the box uses.
	if name, err := primaryIface(client); err == nil && name != "" {
		yaml = strings.Replace(yaml, "iface: eth0", "iface: "+name, 1)
	}

	writeYaml := fmt.Sprintf(`cat > /etc/node-agent/config.yaml <<'EOF'
%sEOF`, yaml)
	if err := run(client, writeYaml, &b); err != nil {
		return fail("write config: %v", err)
	}

	log("Installing systemd unit")
	unit := `[Unit]
Description=mediaproxy Node Agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/node-agent --config /etc/node-agent/config.yaml
Restart=always
RestartSec=3
LimitNOFILE=1000000
User=root
Group=root
StandardOutput=append:/var/log/mediaproxy/agent.log
StandardError=append:/var/log/mediaproxy/agent.log

[Install]
WantedBy=multi-user.target
`
	writeUnit := fmt.Sprintf(`cat > /etc/systemd/system/node-agent.service <<'EOF'
%sEOF`, unit)
	if err := run(client, writeUnit, &b); err != nil {
		return fail("write unit: %v", err)
	}

	log("Enabling + starting node-agent")
	if err := run(client,
		"systemctl daemon-reload && systemctl enable --now node-agent && sleep 2 && systemctl is-active node-agent",
		&b); err != nil {
		return fail("systemd: %v", err)
	}

	log("Tailing agent log (5 lines)")
	_ = run(client, "tail -n 5 /var/log/mediaproxy/agent.log || true", &b)

	log("Provisioning complete. The base-app will mark this node online once the first heartbeat lands.")
	return Result{Log: b.String(), OK: true}
}

func run(client *ssh.Client, cmd string, out *strings.Builder) error {
	sess, err := client.NewSession()
	if err != nil {
		return err
	}
	defer sess.Close()
	var stdout, stderr bytes.Buffer
	sess.Stdout = &stdout
	sess.Stderr = &stderr
	fmt.Fprintf(out, "    $ %s\n", oneLine(cmd))
	if err := sess.Run(cmd); err != nil {
		// Include captured output even on failure.
		if s := strings.TrimSpace(stdout.String()); s != "" {
			fmt.Fprintf(out, "%s\n", indent(s))
		}
		if s := strings.TrimSpace(stderr.String()); s != "" {
			fmt.Fprintf(out, "%s\n", indent(s))
		}
		return err
	}
	if s := strings.TrimSpace(stdout.String()); s != "" {
		fmt.Fprintf(out, "%s\n", indent(s))
	}
	return nil
}

func oneLine(s string) string {
	if i := strings.Index(s, "\n"); i >= 0 {
		return s[:i] + " ..."
	}
	return s
}
func indent(s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = "      " + l
	}
	return strings.Join(lines, "\n")
}

// primaryIface returns the interface name carrying the default route.
func primaryIface(client *ssh.Client) (string, error) {
	sess, err := client.NewSession()
	if err != nil {
		return "", err
	}
	defer sess.Close()
	var out bytes.Buffer
	sess.Stdout = &out
	if err := sess.Run("ip -j route show default | head -c 4000"); err != nil {
		return "", err
	}
	// Parse minimally — the JSON is an array of route objects; find "dev":"X".
	s := out.String()
	const tag = `"dev":"`
	i := strings.Index(s, tag)
	if i < 0 {
		return "", nil
	}
	rest := s[i+len(tag):]
	end := strings.Index(rest, `"`)
	if end < 0 {
		return "", nil
	}
	return rest[:end], nil
}
