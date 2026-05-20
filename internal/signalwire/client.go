// Package signalwire is a thin client for verifying credentials against a
// SignalWire space. It hits the LaML compatibility "Account" endpoint which
// returns the account metadata if the credentials are valid.
package signalwire

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Creds struct {
	SpaceURL  string // e.g. "myspace.signalwire.com"  (no scheme)
	ProjectID string
	APIToken  string
}

type VerifyResult struct {
	OK         bool
	StatusCode int
	Body       string // truncated to first 500 bytes
	Error      string
}

// Verify performs an authenticated GET on the LaML compatibility Account
// resource. 200 = creds are valid; anything else surfaces the error.
func Verify(ctx context.Context, c Creds) VerifyResult {
	if c.SpaceURL == "" || c.ProjectID == "" || c.APIToken == "" {
		return VerifyResult{Error: "space_url, project_id, and api_token are required"}
	}
	host := strings.TrimSuffix(strings.TrimPrefix(c.SpaceURL, "https://"), "/")
	url := fmt.Sprintf("https://%s/api/laml/2010-04-01/Accounts/%s.json", host, c.ProjectID)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return VerifyResult{Error: err.Error()}
	}
	req.SetBasicAuth(c.ProjectID, c.APIToken)
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return VerifyResult{Error: "network: " + err.Error()}
	}
	defer resp.Body.Close()
	buf, _ := io.ReadAll(io.LimitReader(resp.Body, 500))
	r := VerifyResult{
		StatusCode: resp.StatusCode,
		Body:       string(buf),
		OK:         resp.StatusCode == 200,
	}
	switch resp.StatusCode {
	case 200:
		// nothing
	case 401:
		r.Error = "invalid project_id or api_token"
	case 404:
		r.Error = "account not found (check space_url and project_id)"
	default:
		r.Error = fmt.Sprintf("unexpected status %d", resp.StatusCode)
	}
	return r
}
