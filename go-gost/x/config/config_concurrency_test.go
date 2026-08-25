package config

import (
	"encoding/json"
	"path/filepath"
	"sync"
	"testing"
)

func TestGlobalReturnsDetachedSnapshot(t *testing.T) {
	original := Global()
	t.Cleanup(func() { Set(original) })

	Set(&Config{Services: []*ServiceConfig{{
		Name:     "snapshot-service",
		Metadata: map[string]any{"paused": false},
		Handler: &HandlerConfig{
			Type:     "relay",
			Metadata: map[string]any{"retries": 2},
		},
	}}})

	snapshot := Global()
	snapshot.Services[0].Name = "changed"
	snapshot.Services[0].Metadata["paused"] = true
	snapshot.Services[0].Handler.Metadata["retries"] = 9

	current := Global()
	if current.Services[0].Name != "snapshot-service" {
		t.Fatalf("snapshot mutated global service name: %q", current.Services[0].Name)
	}
	if paused, _ := current.Services[0].Metadata["paused"].(bool); paused {
		t.Fatalf("snapshot mutated global service metadata")
	}
	if retries, _ := current.Services[0].Handler.Metadata["retries"].(int); retries != 2 {
		t.Fatalf("snapshot mutated nested handler metadata: %v", current.Services[0].Handler.Metadata["retries"])
	}
}

func TestConcurrentGlobalSnapshotAndUpdate(t *testing.T) {
	original := Global()
	t.Cleanup(func() { Set(original) })

	Set(&Config{Services: []*ServiceConfig{{
		Name:     "concurrent-service",
		Metadata: map[string]any{"generation": 0},
	}}})

	var wg sync.WaitGroup
	for worker := 0; worker < 4; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for i := 0; i < 500; i++ {
				if err := OnUpdate(func(c *Config) error {
					c.Services[0] = &ServiceConfig{
						Name:     "concurrent-service",
						Metadata: map[string]any{"generation": worker*500 + i},
					}
					return nil
				}); err != nil {
					t.Errorf("OnUpdate: %v", err)
					return
				}
			}
		}(worker)
	}

	for i := 0; i < 2000; i++ {
		if _, err := json.Marshal(Global()); err != nil {
			t.Fatalf("marshal snapshot: %v", err)
		}
	}
	wg.Wait()
}

func TestConcurrentPersistProducesValidConfig(t *testing.T) {
	original := Global()
	originalPath := PersistPath()
	persistMu.Lock()
	originalEnabled := persistEnable
	persistMu.Unlock()
	t.Cleanup(func() {
		Set(original)
		SetPersistPath(originalPath)
		persistMu.Lock()
		persistEnable = originalEnabled
		persistMu.Unlock()
	})

	path := filepath.Join(t.TempDir(), "gost.json")
	SetPersistPath(path)
	EnablePersist()
	Set(&Config{Services: []*ServiceConfig{{Name: "persist-service"}}})

	var wg sync.WaitGroup
	for worker := 0; worker < 4; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				if err := OnUpdate(func(c *Config) error {
					c.Services[0].Metadata = map[string]any{"generation": worker*50 + i}
					return nil
				}); err != nil {
					t.Errorf("persist update: %v", err)
					return
				}
			}
		}(worker)
	}
	wg.Wait()

	var persisted Config
	if err := persisted.ReadFile(path); err != nil {
		t.Fatalf("read persisted config: %v", err)
	}
	if len(persisted.Services) != 1 || persisted.Services[0] == nil || persisted.Services[0].Name != "persist-service" {
		t.Fatalf("unexpected persisted services: %#v", persisted.Services)
	}
}
