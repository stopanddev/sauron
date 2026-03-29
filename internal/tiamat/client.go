package tiamat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const hubDisabledSnippet = "hub API disabled"

// ErrHubDisabled is returned when Tiamat responds 503 with the hub-disabled payload.
var ErrHubDisabled = errors.New("hub API disabled on Tiamat")

// Client calls Tiamat's public HTTP API (healthz without auth; hub routes with Bearer).
type Client struct {
	baseURL    string
	hubToken   string
	httpClient *http.Client
}

// New constructs a Client. hubToken may be empty; hub JSON routes will fail until set.
func New(baseURL, hubToken string) *Client {
	baseURL = normalizeLocalhostToIPv4(strings.TrimSpace(baseURL))
	baseURL = strings.TrimRight(baseURL, "/")
	return &Client{
		baseURL:  baseURL,
		hubToken: strings.TrimSpace(hubToken),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// normalizeLocalhostToIPv4 rewrites http(s)://localhost:port to 127.0.0.1 so the Go
// client dials IPv4. On many Linux setups "localhost" resolves to [::1] first while
// the dev server only listens on 127.0.0.1, which produces connection refused.
func normalizeLocalhostToIPv4(raw string) string {
	if raw == "" {
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return raw
	}
	if !strings.EqualFold(u.Hostname(), "localhost") {
		return raw
	}
	port := u.Port()
	if port == "" {
		switch u.Scheme {
		case "https":
			port = "443"
		case "http":
			port = "80"
		default:
			return raw
		}
	}
	u.Host = net.JoinHostPort("127.0.0.1", port)
	return u.String()
}

func (c *Client) hubAuth() error {
	if c.hubToken == "" {
		return fmt.Errorf("TIAMAT_HUB_TOKEN is not set")
	}
	return nil
}

func (c *Client) get(ctx context.Context, path string, bearer bool, q url.Values) (*http.Response, error) {
	u := c.baseURL + path
	if q != nil && len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	if bearer {
		if err := c.hubAuth(); err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+c.hubToken)
	}
	return c.httpClient.Do(req)
}

func (c *Client) doJSON(ctx context.Context, method, path string, body any, out any) error {
	if err := c.hubAuth(); err != nil {
		return err
	}
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, rdr)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.hubToken)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return handleHubResponse(resp, out)
}

func handleHubResponse(resp *http.Response, out any) error {
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusServiceUnavailable {
		var er struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(b, &er) == nil && strings.Contains(strings.ToLower(er.Error), strings.ToLower(hubDisabledSnippet)) {
			return ErrHubDisabled
		}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("tiamat: %s: %s", resp.Status, strings.TrimSpace(string(b)))
	}
	if out == nil || len(b) == 0 {
		return nil
	}
	return json.Unmarshal(b, out)
}

// Healthz returns true if GET /healthz succeeds with 2xx and body typically "ok".
func (c *Client) Healthz(ctx context.Context) (bool, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/healthz", nil)
	if err != nil {
		return false, "", err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false, "", err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	body := strings.TrimSpace(string(b))
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return true, body, nil
	}
	return false, body, fmt.Errorf("healthz: %s", resp.Status)
}

// HubStatus is GET /api/hub/v1/status.
type HubStatus struct {
	UptimeSeconds float64 `json:"uptime_seconds"`
	Version       string  `json:"version"`
	DbOk          bool    `json:"db_ok"`
}

func (c *Client) Status(ctx context.Context) (*HubStatus, error) {
	resp, err := c.get(ctx, "/api/hub/v1/status", true, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var out HubStatus
	if err := handleHubResponse(resp, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Restart POST /api/hub/v1/restart (empty body).
func (c *Client) Restart(ctx context.Context) error {
	return c.doJSON(ctx, http.MethodPost, "/api/hub/v1/restart", nil, nil)
}

// User represents a hub user row.
type User struct {
	Username string `json:"username"`
	IsAdmin  bool   `json:"is_admin"`
	Disabled bool   `json:"disabled"`
}

// ListUsers GET /api/hub/v1/users.
func (c *Client) ListUsers(ctx context.Context) ([]User, error) {
	resp, err := c.get(ctx, "/api/hub/v1/users", true, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var out []User
	if err := handleHubResponse(resp, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// CreateUser POST /api/hub/v1/users.
type CreateUserBody struct {
	Username string `json:"username"`
	Password string `json:"password"`
	IsAdmin  *bool  `json:"is_admin,omitempty"`
}

func (c *Client) CreateUser(ctx context.Context, b CreateUserBody) error {
	return c.doJSON(ctx, http.MethodPost, "/api/hub/v1/users", b, nil)
}

// PatchUser PATCH /api/hub/v1/users/{username}.
type PatchUserBody struct {
	Password *string `json:"password,omitempty"`
	IsAdmin  *bool   `json:"is_admin,omitempty"`
	Disabled *bool   `json:"disabled,omitempty"`
}

func (c *Client) PatchUser(ctx context.Context, username string, b PatchUserBody) error {
	path := "/api/hub/v1/users/" + url.PathEscape(username)
	return c.doJSON(ctx, http.MethodPatch, path, b, nil)
}

// DeleteUser DELETE /api/hub/v1/users/{username} (soft-disable).
func (c *Client) DeleteUser(ctx context.Context, username string) error {
	if err := c.hubAuth(); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.baseURL+"/api/hub/v1/users/"+url.PathEscape(username), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.hubToken)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return handleHubResponse(resp, nil)
}

// StatsSummary GET /api/hub/v1/stats/summary?from=&to= (RFC3339).
func (c *Client) StatsSummary(ctx context.Context, from, to string) (json.RawMessage, error) {
	q := url.Values{}
	q.Set("from", from)
	q.Set("to", to)
	resp, err := c.get(ctx, "/api/hub/v1/stats/summary", true, q)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusServiceUnavailable {
		var er struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(b, &er) == nil && strings.Contains(strings.ToLower(er.Error), strings.ToLower(hubDisabledSnippet)) {
			return nil, ErrHubDisabled
		}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("tiamat: %s: %s", resp.Status, strings.TrimSpace(string(b)))
	}
	return json.RawMessage(b), nil
}

// StatsAssets GET /api/hub/v1/stats/assets (from, to must be RFC3339; use FormatHubStatsParam).
func (c *Client) StatsAssets(ctx context.Context, from, to, kind string, limit, offset int) (json.RawMessage, error) {
	q := url.Values{}
	q.Set("from", from)
	q.Set("to", to)
	if kind != "" {
		q.Set("kind", kind)
	}
	if limit > 0 {
		q.Set("limit", fmt.Sprintf("%d", limit))
	}
	if offset > 0 {
		q.Set("offset", fmt.Sprintf("%d", offset))
	}
	resp, err := c.get(ctx, "/api/hub/v1/stats/assets", true, q)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusServiceUnavailable {
		var er struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(b, &er) == nil && strings.Contains(strings.ToLower(er.Error), strings.ToLower(hubDisabledSnippet)) {
			return nil, ErrHubDisabled
		}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("tiamat: %s: %s", resp.Status, strings.TrimSpace(string(b)))
	}
	return json.RawMessage(b), nil
}
