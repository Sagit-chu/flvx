package socket

import (
	"path/filepath"
	"testing"

	corelogger "github.com/go-gost/core/logger"
	"github.com/go-gost/x/config"
	xlogger "github.com/go-gost/x/logger"
	"github.com/go-gost/x/registry"
)

func TestCreateConnLimiterUpdatesGlobalConfig(t *testing.T) {
	corelogger.SetDefault(xlogger.Nop())

	name := "conn_limiter_tdd"
	originalConfig := config.Global()
	defer config.Set(originalConfig)
	registry.ConnLimiterRegistry().Unregister(name)
	defer registry.ConnLimiterRegistry().Unregister(name)
	config.Set(&config.Config{})

	err := createConnLimiter(createLimiterRequest{Data: config.LimiterConfig{Name: name, Limits: []string{"$ 1"}}})
	if err != nil {
		t.Fatalf("create conn limiter: %v", err)
	}

	cfg := config.Global()
	if len(cfg.CLimiters) != 1 || cfg.CLimiters[0] == nil || cfg.CLimiters[0].Name != name {
		t.Fatalf("expected conn limiter in global config, got %#v", cfg.CLimiters)
	}
}

func TestCreateLimiterReportsPersistFailure(t *testing.T) {
	corelogger.SetDefault(xlogger.Nop())

	name := "traffic_limiter_persist_tdd"
	originalConfig := config.Global()
	originalPersistPath := config.PersistPath()
	defer config.Set(originalConfig)
	defer config.SetPersistPath(originalPersistPath)
	registry.TrafficLimiterRegistry().Unregister(name)
	defer registry.TrafficLimiterRegistry().Unregister(name)
	config.Set(&config.Config{})
	config.SetPersistPath(filepath.Join(t.TempDir(), "missing", "gost.json"))
	config.EnablePersist()

	err := createLimiter(createLimiterRequest{Data: config.LimiterConfig{Name: name, Limits: []string{"$ 1"}}})
	if err == nil {
		t.Fatalf("expected persist failure to be returned")
	}
}
