package panelclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	baseURL string
	http    *http.Client
}

type Envelope struct {
	Code int             `json:"code"`
	Msg  string          `json:"msg"`
	TS   int64           `json:"ts"`
	Data json.RawMessage `json:"data,omitempty"`
}

type LoginResult struct {
	Token                 string `json:"token"`
	Name                  string `json:"name"`
	RoleID                int64  `json:"role_id"`
	RequirePasswordChange bool   `json:"requirePasswordChange"`
}

func New(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		http: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *Client) ListNodes(ctx context.Context, token string) ([]map[string]any, error) {
	var out []map[string]any
	if err := c.post(ctx, token, "/api/v1/node/list", map[string]any{}, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) ListUsers(ctx context.Context, token string, keyword string) ([]map[string]any, error) {
	var out []map[string]any
	payload := map[string]any{
		"current": 1,
		"size":    10000,
		"keyword": keyword,
	}
	if err := c.post(ctx, token, "/api/v1/user/list", payload, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) Login(ctx context.Context, username, password, captchaID string) (*LoginResult, error) {
	var out LoginResult
	payload := map[string]any{
		"username":  username,
		"password":  password,
		"captchaId": captchaID,
	}
	if err := c.post(ctx, "", "/api/v1/user/login", payload, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) UserPackage(ctx context.Context, token string) (map[string]any, error) {
	var out map[string]any
	if err := c.post(ctx, token, "/api/v1/user/package", map[string]any{}, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) ListTunnels(ctx context.Context, token string) ([]map[string]any, error) {
	var out []map[string]any
	if err := c.post(ctx, token, "/api/v1/tunnel/list", map[string]any{}, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) ListForwards(ctx context.Context, token string) ([]map[string]any, error) {
	var out []map[string]any
	if err := c.post(ctx, token, "/api/v1/forward/list", map[string]any{}, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) DeleteForward(ctx context.Context, token string, id int64) error {
	if err := c.postByID(ctx, token, "/api/v1/forward/delete", id); err != nil {
		return err
	}
	return nil
}

func (c *Client) PauseForward(ctx context.Context, token string, id int64) error {
	if err := c.postByID(ctx, token, "/api/v1/forward/pause", id); err != nil {
		return err
	}
	return nil
}

func (c *Client) ResumeForward(ctx context.Context, token string, id int64) error {
	if err := c.postByID(ctx, token, "/api/v1/forward/resume", id); err != nil {
		return err
	}
	return nil
}

func (c *Client) DeleteUser(ctx context.Context, token string, id int64) error {
	if err := c.postByID(ctx, token, "/api/v1/user/delete", id); err != nil {
		return err
	}
	return nil
}

func (c *Client) DeleteNode(ctx context.Context, token string, id int64) error {
	if err := c.postByID(ctx, token, "/api/v1/node/delete", id); err != nil {
		return err
	}
	return nil
}

func (c *Client) DeleteTunnel(ctx context.Context, token string, id int64) error {
	if err := c.postByID(ctx, token, "/api/v1/tunnel/delete", id); err != nil {
		return err
	}
	return nil
}

func (c *Client) postByID(ctx context.Context, token, path string, id int64) error {
	payload := map[string]any{"id": id}
	if err := c.post(ctx, token, path, payload, nil); err != nil {
		return err
	}
	return nil
}

func (c *Client) post(ctx context.Context, token, path string, payload any, out any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(token) != "" {
		req.Header.Set("Authorization", strings.TrimSpace(token))
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("call panel api: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("panel http status %d: %s", resp.StatusCode, string(raw))
	}

	var env Envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return fmt.Errorf("decode panel envelope: %w", err)
	}
	if env.Code != 0 {
		return fmt.Errorf("panel error code=%d msg=%s", env.Code, env.Msg)
	}

	if len(env.Data) == 0 || string(env.Data) == "null" {
		return nil
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(env.Data, out); err != nil {
		return fmt.Errorf("decode panel data: %w", err)
	}

	return nil
}
