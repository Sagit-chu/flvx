package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	ServerName     string
	ServerVersion  string
	PanelBaseURL   string
	MCPTransport   string
	HTTPAddr       string
	HealthPath     string
	ConfirmToken   string
	AuditEnabled   bool
	IdempotencyTTL time.Duration
}

func FromEnv() (Config, error) {
	cfg := Config{
		ServerName:     getEnv("MCP_SERVER_NAME", "flvx-panel-mcp"),
		ServerVersion:  getEnv("MCP_SERVER_VERSION", "0.1.0"),
		PanelBaseURL:   strings.TrimRight(getEnv("PANEL_BASE_URL", "http://127.0.0.1:6365"), "/"),
		MCPTransport:   strings.ToLower(getEnv("MCP_TRANSPORT", "stdio")),
		HTTPAddr:       getEnv("MCP_HTTP_ADDR", ":8088"),
		HealthPath:     getEnv("MCP_HEALTH_PATH", "/healthz"),
		ConfirmToken:   getEnv("MCP_CONFIRM_TOKEN", ""),
		AuditEnabled:   getBoolEnv("MCP_AUDIT_ENABLED", true),
		IdempotencyTTL: getDurationFromSeconds("MCP_IDEMPOTENCY_TTL_SECONDS", 3600),
	}

	switch cfg.MCPTransport {
	case "stdio", "http":
	default:
		return Config{}, fmt.Errorf("invalid MCP_TRANSPORT %q, expected stdio or http", cfg.MCPTransport)
	}

	return cfg, nil
}

func getDurationFromSeconds(key string, fallbackSeconds int) time.Duration {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return time.Duration(fallbackSeconds) * time.Second
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return time.Duration(fallbackSeconds) * time.Second
	}
	return time.Duration(n) * time.Second
}

func getEnv(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func getBoolEnv(key string, fallback bool) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	if v == "" {
		return fallback
	}
	switch v {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}
