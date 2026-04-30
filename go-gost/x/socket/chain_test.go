package socket

import (
	"testing"

	corelogger "github.com/go-gost/core/logger"
	"github.com/go-gost/x/config"
	_ "github.com/go-gost/x/connector/relay"
	_ "github.com/go-gost/x/dialer/tcp"
	xlogger "github.com/go-gost/x/logger"
	"github.com/go-gost/x/registry"
)

func TestUpdateChainParseFailureKeepsExistingChainRegistered(t *testing.T) {
	corelogger.SetDefault(xlogger.Nop())

	name := "chain_update_parse_failure_tdd"
	originalConfig := config.Global()
	defer config.Set(originalConfig)
	registry.ChainRegistry().Unregister(name)
	defer registry.ChainRegistry().Unregister(name)
	config.Set(&config.Config{})

	valid := config.ChainConfig{
		Name: name,
		Hops: []*config.HopConfig{{
			Name: "hop-valid",
			Nodes: []*config.NodeConfig{{
				Name:      "node-valid",
				Addr:      "127.0.0.1:443",
				Connector: &config.ConnectorConfig{Type: "relay"},
				Dialer:    &config.DialerConfig{Type: "tcp"},
			}},
		}},
	}
	if err := createChain(createChainRequest{Data: valid}); err != nil {
		t.Fatalf("create valid chain: %v", err)
	}
	before := registry.ChainRegistry().Get(name)
	if before == nil || !registry.ChainRegistry().IsRegistered(name) {
		t.Fatalf("expected chain registered before update")
	}

	invalid := config.ChainConfig{
		Hops: []*config.HopConfig{{
			Name: "hop-invalid",
			Nodes: []*config.NodeConfig{{
				Name:      "node-invalid",
				Addr:      "127.0.0.1:443",
				Connector: &config.ConnectorConfig{Type: "connector-does-not-exist"},
				Dialer:    &config.DialerConfig{Type: "tcp"},
			}},
		}},
	}
	err := updateChain(updateChainRequest{Chain: name, Data: invalid})
	if err == nil {
		t.Fatalf("expected invalid chain update to fail")
	}
	if !registry.ChainRegistry().IsRegistered(name) {
		t.Fatalf("expected old chain to remain registered after failed update")
	}
	cfg := config.Global()
	if len(cfg.Chains) != 1 || cfg.Chains[0] == nil || cfg.Chains[0].Name != name {
		t.Fatalf("expected original chain config to remain, got %#v", cfg.Chains)
	}
	if got := cfg.Chains[0].Hops[0].Name; got != "hop-valid" {
		t.Fatalf("expected original chain config to remain, got hop %q", got)
	}
}
