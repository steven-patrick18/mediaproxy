package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Client struct {
	baseURL string
	token   string
	httpC   *http.Client
}

func NewClient(baseURL, token string, timeout time.Duration) *Client {
	return &Client{
		baseURL: baseURL,
		token:   token,
		httpC:   &http.Client{Timeout: timeout},
	}
}

func (c *Client) post(ctx context.Context, path string, body any, out any) error {
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			return err
		}
	}
	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+path, &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	res, err := c.httpC.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	respBody, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 300 {
		return fmt.Errorf("control plane %s -> %d: %s", path, res.StatusCode, string(respBody))
	}
	if out != nil && len(respBody) > 0 {
		return json.Unmarshal(respBody, out)
	}
	return nil
}

type RegisterReq struct {
	Hostname         string `json:"hostname"`
	Cores            int    `json:"cores"`
	RAMMB            int    `json:"ram_mb"`
	RTPEngineVersion string `json:"rtpengine_version"`
	AgentVersion     string `json:"agent_version"`
}

type DirectiveResp struct {
	NodeID      int64    `json:"node_id"`
	Role        string   `json:"role"`
	ExpectedIPs []string `json:"expected_ips"`
}

func (c *Client) Register(ctx context.Context, r RegisterReq) (*DirectiveResp, error) {
	var out DirectiveResp
	if err := c.post(ctx, "/api/v1/agent/register", r, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

type HeartbeatReq struct {
	BoundIPs      []string `json:"bound_ips"`
	ActiveCalls   int      `json:"active_calls"`
	CPUPct        float64  `json:"cpu_pct"`
	RAMPct        float64  `json:"ram_pct"`
	NetInMbps     float64  `json:"net_in_mbps"`
	NetOutMbps    float64  `json:"net_out_mbps"`
	PacketLossPct float64  `json:"packet_loss_pct"`
	UptimeSeconds int64    `json:"uptime_seconds"`
	AgentVersion  string   `json:"agent_version"`
}

type Command struct {
	ID      string          `json:"id"`
	Type    string          `json:"type"`
	IP      string          `json:"ip,omitempty"`
	CIDR    int             `json:"cidr,omitempty"`
	Iface   string          `json:"iface,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

type HeartbeatResp struct {
	ExpectedIPs []string  `json:"expected_ips"`
	Commands    []Command `json:"commands"`
}

func (c *Client) Heartbeat(ctx context.Context, r HeartbeatReq) (*HeartbeatResp, error) {
	var out HeartbeatResp
	if err := c.post(ctx, "/api/v1/agent/heartbeat", r, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

type CommandResult struct {
	CommandID string `json:"command_id"`
	Status    string `json:"status"`
	Detail    string `json:"detail,omitempty"`
}

func (c *Client) AckCommand(ctx context.Context, r CommandResult) error {
	return c.post(ctx, "/api/v1/agent/command-result", r, nil)
}
