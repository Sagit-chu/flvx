package handler

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"gorm.io/gorm"
)

func TestConfigCleanupPreservesSharedDependencies(t *testing.T) {
	a := newCleanupAgent(t)
	a.addRuntime(t, "70_1_0", 1, 1, 1, time.Now())
	a.h.cleanNodeConfigs(1, `{
		"services": [{"name":"70_1_0_tcp", "handler":{"chain":"chains_88"}, "limiter":"13, rule_traffic_limit_70"}],
		"chains": [{"name":"fed_chain_17"}, {"name":"chains_88"}, {"name":"chains_999"}],
		"limiters": [{"name":"13"}, {"name":"rule_traffic_limit_70"}, {"name":"99"}]
	}`)
	a.probe(t)
	if commands := a.commandsOfType("DeleteService"); len(commands) != 0 {
		t.Fatalf("shared service was deleted: %+v", commands)
	}
	assertCleanupDependency(t, a, "DeleteChains", "chain", "chains_999")
	assertCleanupDependency(t, a, "DeleteLimiters", "limiter", "99")
}

func TestConfigCleanupRemovesOrphanedForwardLimiter(t *testing.T) {
	a := newCleanupAgent(t)
	a.h.cleanNodeConfigs(1, `{"limiters":[{"name":"rule_traffic_limit_70"}]}`)
	a.probe(t)
	assertCleanupDependency(t, a, "DeleteLimiters", "limiter", "rule_traffic_limit_70")
}

func TestConfigCleanupProtectsPendingSharedDependencies(t *testing.T) {
	for _, tc := range []struct {
		name string
		age  time.Duration
		keep bool
	}{
		{name: "pending", age: time.Minute, keep: true},
		{name: "expired-reservation", age: 11 * time.Minute},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := newCleanupAgent(t)
			a.addRuntime(t, "", 1, 1, 0, time.Now().Add(-tc.age))
			// Dependencies may arrive before the service and its runtime binding.
			a.h.cleanNodeConfigs(1, `{"chains":[{"name":"chains_88"}],"limiters":[{"name":"13"}]}`)
			a.probe(t)
			if tc.keep {
				for _, commandType := range []string{"DeleteChains", "DeleteLimiters"} {
					if commands := a.commandsOfType(commandType); len(commands) != 0 {
						t.Fatalf("pending shared dependencies deleted: %+v", commands)
					}
				}
				return
			}
			assertCleanupDependency(t, a, "DeleteChains", "chain", "chains_88")
			assertCleanupDependency(t, a, "DeleteLimiters", "limiter", "13")
		})
	}
}

func TestConfigCleanupPreservesDependenciesOnLookupFailure(t *testing.T) {
	for _, table := range []string{"peer_share_runtime", "tunnel", "speed_limit", "forward"} {
		t.Run(table, func(t *testing.T) {
			a := newCleanupAgent(t)
			callback := "test:config-cleanup-query-failure"
			injected := false
			if err := a.h.repo.DB().Callback().Query().Before("gorm:query").Register(callback, func(tx *gorm.DB) {
				if tx.Statement.Table == table {
					injected = true
					tx.AddError(errors.New("injected dependency lookup failure"))
				}
			}); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = a.h.repo.DB().Callback().Query().Remove(callback) })
			configs := map[string]string{
				"peer_share_runtime": `{"chains":[{"name":"chains_88"}],"limiters":[{"name":"13"}]}`,
				"tunnel":             `{"chains":[{"name":"chains_88"}]}`,
				"speed_limit":        `{"limiters":[{"name":"13"}]}`,
				"forward":            `{"limiters":[{"name":"rule_traffic_limit_70"}]}`,
			}
			a.h.cleanNodeConfigs(1, configs[table])
			a.probe(t)
			if !injected {
				t.Fatal("expected dependency lookup failure to be injected")
			}
			for _, commandType := range []string{"DeleteChains", "DeleteLimiters"} {
				if commands := a.commandsOfType(commandType); len(commands) != 0 {
					t.Fatalf("lookup failure must not authorize cleanup: %+v", commands)
				}
			}
		})
	}
}

func TestSharedForwardCleanupDuringRuntimeBinding(t *testing.T) {
	for _, mode := range []string{"single", "batch", "config"} {
		t.Run(mode, func(t *testing.T) {
			a := newCleanupAgent(t)
			a.addRuntime(t, "", 1, 1, 0, time.Now())
			callback := "test:bind-during-cleanup"
			bound := false
			if err := a.h.repo.DB().Callback().Query().After("gorm:query").Register(callback, func(tx *gorm.DB) {
				if tx.Statement.Table != "peer_share_runtime" || bound {
					return
				}
				bound = true
				// Reproduce binding immediately after the first ownership read.
				// Separate name/unbound queries would both miss this runtime.
				if err := a.h.repo.DB().Exec("UPDATE peer_share_runtime SET service_name = ?, applied = 1", "70_1_0").Error; err != nil {
					t.Errorf("bind shared runtime: %v", err)
				}
			}); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = a.h.repo.DB().Callback().Query().Remove(callback) })
			runCleanupPath(a.h, mode, []string{"70_1_0_tcp"})
			a.probe(t)
			if !bound {
				t.Fatal("binding transition did not run")
			}
			if commands := a.commandsOfType("DeleteService"); len(commands) != 0 {
				t.Fatalf("service was deleted during binding: %+v", commands)
			}
		})
	}
}

func assertCleanupDependency(t *testing.T, a *cleanupAgent, commandType, key, want string) {
	t.Helper()
	commands := a.commandsOfType(commandType)
	if len(commands) != 1 {
		t.Fatalf("expected one %s for %s, got %+v", commandType, want, commands)
	}
	var data map[string]string
	if err := json.Unmarshal(commands[0].Data, &data); err != nil {
		t.Fatal(err)
	}
	if data[key] != want {
		t.Fatalf("unexpected %s target: got %q, want %q", commandType, data[key], want)
	}
}
