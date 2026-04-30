package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

var (
	persistPath   string
	persistMu     sync.Mutex
	persistEnable bool
)

// SetPersistPath sets the file path where runtime config changes will be
// automatically persisted. Call this once during agent startup before any
// OnUpdate mutations occur.
func SetPersistPath(path string) {
	persistMu.Lock()
	defer persistMu.Unlock()
	persistPath = path
}

func PersistPath() string {
	persistMu.Lock()
	defer persistMu.Unlock()
	return persistPath
}

// EnablePersist turns on automatic persistence. Call this after the initial
// config has been loaded (e.g. after program.Start) so that startup loading
// does not trigger redundant disk writes.
func EnablePersist() {
	persistMu.Lock()
	defer persistMu.Unlock()
	persistEnable = true
}

// persist writes the current global config to the configured file atomically.
func persist() error {
	persistMu.Lock()
	path := persistPath
	enabled := persistEnable
	persistMu.Unlock()

	if !enabled || path == "" {
		return nil
	}

	cfg := Global()
	if cfg == nil {
		return nil
	}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(cfg); err != nil {
		fmt.Printf("⚠️ config persist: marshal failed: %v\n", err)
		return fmt.Errorf("config persist: marshal failed: %w", err)
	}

	// Atomic write: write to temp file then rename
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".gost-*.tmp")
	if err != nil {
		fmt.Printf("⚠️ config persist: create temp file failed: %v\n", err)
		return fmt.Errorf("config persist: create temp file failed: %w", err)
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(buf.Bytes()); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		fmt.Printf("⚠️ config persist: write failed: %v\n", err)
		return fmt.Errorf("config persist: write failed: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		fmt.Printf("⚠️ config persist: close temp file failed: %v\n", err)
		return fmt.Errorf("config persist: close temp file failed: %w", err)
	}

	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		fmt.Printf("⚠️ config persist: rename failed: %v\n", err)
		return fmt.Errorf("config persist: rename failed: %w", err)
	}

	fmt.Printf("💾 节点配置已持久化到 %s\n", path)
	return nil
}
