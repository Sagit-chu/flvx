package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadFileSetsPersistPath(t *testing.T) {
	originalPath := persistPath
	originalEnabled := persistEnable
	persistPath = ""
	persistEnable = false
	t.Cleanup(func() {
		persistPath = originalPath
		persistEnable = originalEnabled
	})

	dir := t.TempDir()
	configFile := filepath.Join(dir, "custom-gost.yaml")
	if err := os.WriteFile(configFile, []byte("services: []\n"), 0o644); err != nil {
		t.Fatalf("write config file: %v", err)
	}

	var cfg Config
	if err := cfg.ReadFile(configFile); err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if persistPath != configFile {
		t.Fatalf("expected persistPath %q, got %q", configFile, persistPath)
	}
}
