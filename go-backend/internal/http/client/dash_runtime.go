package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"go-backend/internal/dashruntime"
)

type DashRuntimeNode struct {
	ServerIP string
	Secret   string
}

type DashRuntimeClientConfig struct {
	Scheme          string
	Port            string
	Timeout         time.Duration
	BaseURLOverride string
}

type DashRule struct {
	ID string `json:"id"`
}

type DashRuleListResponse []DashRule

type dashRuntimeServiceControlRequest struct {
	Services []string `json:"services"`
}

type dashRuleUpsertRequest struct {
	Protocol    string                        `json:"protocol"`
	Listen      string                        `json:"listen"`
	Enabled     bool                          `json:"enabled"`
	Description *string                       `json:"description,omitempty"`
	ExitPool    map[string]interface{}        `json:"exit_pool,omitempty"`
	TargetPool  dashruntime.TargetPoolPayload `json:"target_pool"`
	Traffic     *dashruntime.TrafficPayload   `json:"traffic,omitempty"`
}

type DashServiceCheckRequest struct {
	Type       string `json:"type"`
	Target     string `json:"target"`
	TimeoutSec int    `json:"timeoutSec"`
}

type DashServiceCheckResponse struct {
	Success      bool    `json:"success"`
	LatencyMs    float64 `json:"latencyMs"`
	StatusCode   int     `json:"statusCode"`
	ErrorMessage string  `json:"errorMessage"`
}

type DashStatusResponse struct {
	RuleCount int           `json:"rule_count"`
	ExitState DashExitState `json:"exit_state"`
}

type DashExitState struct {
	Active *DashActiveBackend `json:"active"`
}

type DashActiveBackend struct {
	Server string `json:"server"`
	Token  string `json:"token"`
}

type DashRuntimeClient struct {
	client *http.Client
	cfg    DashRuntimeClientConfig
}

func NewDashRuntimeClient(cfg DashRuntimeClientConfig) *DashRuntimeClient {
	return &DashRuntimeClient{
		client: &http.Client{Timeout: cfg.Timeout},
		cfg:    cfg,
	}
}

func (c *DashRuntimeClient) ListRules(ctx context.Context, node DashRuntimeNode) (DashRuleListResponse, error) {
	var out DashRuleListResponse
	if err := c.doJSON(ctx, node, http.MethodGet, "/api/relay/rules", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *DashRuntimeClient) PauseServices(ctx context.Context, node DashRuntimeNode, services []string) error {
	return c.controlServices(ctx, node, "/api/relay/services/pause", services)
}

func (c *DashRuntimeClient) ResumeServices(ctx context.Context, node DashRuntimeNode, services []string) error {
	return c.controlServices(ctx, node, "/api/relay/services/resume", services)
}

func (c *DashRuntimeClient) CheckService(ctx context.Context, node DashRuntimeNode, req DashServiceCheckRequest) (DashServiceCheckResponse, error) {
	var out DashServiceCheckResponse
	if err := c.doJSON(ctx, node, http.MethodPost, "/api/monitor/check", req, &out); err != nil {
		return DashServiceCheckResponse{}, err
	}
	return out, nil
}

func (c *DashRuntimeClient) GetStatus(ctx context.Context, node DashRuntimeNode) (DashStatusResponse, error) {
	var out DashStatusResponse
	if err := c.doJSON(ctx, node, http.MethodGet, "/api/relay/status", nil, &out); err != nil {
		return DashStatusResponse{}, err
	}
	return out, nil
}

func (c *DashRuntimeClient) UpsertRule(ctx context.Context, node DashRuntimeNode, rule dashruntime.RelayRulePayload) error {
	body := dashRuleUpsertRequest{
		Protocol:    rule.Protocol,
		Listen:      rule.Listen,
		Enabled:     rule.Enabled,
		Description: rule.Description,
		TargetPool:  rule.TargetPool,
		Traffic:     rule.Traffic,
	}
	if len(rule.StagePools) > 0 {
		body.ExitPool = map[string]interface{}{
			"policy":   rule.StagePools[0].Policy,
			"backends": rule.StagePools[0].Backends,
		}
	}
	createBody := map[string]interface{}{
		"id":          rule.ID,
		"protocol":    body.Protocol,
		"listen":      body.Listen,
		"enabled":     body.Enabled,
		"target_pool": body.TargetPool,
	}
	if body.Description != nil {
		createBody["description"] = body.Description
	}
	if body.ExitPool != nil {
		createBody["exit_pool"] = body.ExitPool
	}
	if body.Traffic != nil {
		createBody["traffic"] = body.Traffic
	}
	err := c.doJSON(ctx, node, http.MethodPost, "/api/relay/rules", createBody, nil)
	if err == nil {
		return nil
	}
	if !strings.Contains(err.Error(), "already exists") && !strings.Contains(err.Error(), "409") {
		return err
	}
	updateErr := c.doJSON(ctx, node, http.MethodPut, "/api/relay/rules/"+rule.ID, body, nil)
	if updateErr != nil {
		return fmt.Errorf("dash upsert create/update failed: create=%v; update=%v", err, updateErr)
	}
	return nil
}

func (c *DashRuntimeClient) DeleteRule(ctx context.Context, node DashRuntimeNode, ruleID string) error {
	return c.doJSON(ctx, node, http.MethodDelete, "/api/relay/rules/"+ruleID, nil, nil)
}

func (c *DashRuntimeClient) controlServices(ctx context.Context, node DashRuntimeNode, path string, services []string) error {
	return c.doJSON(ctx, node, http.MethodPost, path, dashRuntimeServiceControlRequest{Services: services}, nil)
}

func (c *DashRuntimeClient) doJSON(ctx context.Context, node DashRuntimeNode, method, path string, in, out interface{}) error {
	baseURL := c.cfg.BaseURLOverride
	if baseURL == "" {
		baseURL = fmt.Sprintf("%s://%s:%s", c.cfg.Scheme, node.ServerIP, c.cfg.Port)
	}
	var body io.Reader
	var requestPayload []byte
	if in != nil {
		buf, err := json.Marshal(in)
		if err != nil {
			return err
		}
		requestPayload = append([]byte(nil), buf...)
		body = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimSuffix(baseURL, "/")+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+node.Secret)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		payload, _ := io.ReadAll(resp.Body)
		if len(requestPayload) > 0 {
			return fmt.Errorf("dash runtime error %d %s %s: %s | request=%s", resp.StatusCode, method, path, string(payload), string(requestPayload))
		}
		return fmt.Errorf("dash runtime error %d %s %s: %s", resp.StatusCode, method, path, string(payload))
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}
