package socket

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"time"
)

var (
	dashMu     sync.Mutex
	dashCancel context.CancelFunc
	dashPaused bool // Whether to temporarily pause the supervisor from restarting dash
)

func generateDashConfig() error {
	configBytes, err := os.ReadFile("/etc/flux_agent/config.json")
	if err != nil {
		return err
	}
	var cfg struct {
		Addr   string `json:"addr"`
		Secret string `json:"secret"`
	}
	if err := json.Unmarshal(configBytes, &cfg); err != nil {
		return err
	}

	dashYaml := `api:
  addr: :19090
log:
  level: info
`

	_ = os.Remove("/etc/flux_agent/exit.yaml")

	return os.WriteFile("/etc/flux_agent/dash.yaml", []byte(dashYaml), 0644)
}

// StartDashSupervisor starts a background goroutine that keeps the dash process running
// if the binary exists. It will exit when ctx is canceled.
func StartDashSupervisor(ctx context.Context) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			dashMu.Lock()
			paused := dashPaused
			dashMu.Unlock()

			if !paused {
				dashPath := "/etc/flux_agent/dash"
				if _, err := os.Stat(dashPath); err == nil {
					err := func() error {
						if err := generateDashConfig(); err != nil {
							return fmt.Errorf("生成 dash.yaml 失败: %w", err)
						}

						dashMu.Lock()
						ctxDash, cancel := context.WithCancel(ctx)
						dashCancel = cancel
						dashMu.Unlock()

						cmd := exec.CommandContext(ctxDash, dashPath, "--config", "/etc/flux_agent/dash.yaml")
						cmd.Stdout = os.Stdout
						cmd.Stderr = os.Stderr
						fmt.Println("🚀 启动 dash 内核进程 (由 flux_agent 托管)...")
						return cmd.Run()
					}()
					if err != nil && err.Error() != "signal: killed" {
						fmt.Printf("⚠️ dash 进程异常退出: %v\n", err)
					}
				}
			}

			select {
			case <-time.After(3 * time.Second):
			case <-ctx.Done():
				return
			}
		}
	}()
}

// ReplaceAndRestartDash cleanly stops the current dash process, replaces its binary with newBinaryPath,
// and lets the supervisor restart it.
func ReplaceAndRestartDash(newBinaryPath string) error {
	dashMu.Lock()
	dashPaused = true
	if dashCancel != nil {
		dashCancel() // Kill running instance
	}
	dashMu.Unlock()

	defer func() {
		dashMu.Lock()
		dashPaused = false
		dashMu.Unlock()
	}()

	// Wait a moment to ensure the process has actually terminated
	time.Sleep(1 * time.Second)

	dashPath := "/etc/flux_agent/dash"
	if err := os.Rename(newBinaryPath, dashPath); err != nil {
		return fmt.Errorf("覆盖 dash 二进制文件失败: %w", err)
	}

	fmt.Println("🔄 dash 内核二进制已替换，等待监控程序重启...")
	return nil
}
