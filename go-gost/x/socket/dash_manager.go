package socket

import (
	"context"
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
						// Ensure gost.json is initialized with at least the API service
						if err := saveConfig(); err != nil {
							return fmt.Errorf("ensure gost.json failed: %w", err)
						}

						dashMu.Lock()
						ctxDash, cancel := context.WithCancel(ctx)
						dashCancel = cancel
						dashMu.Unlock()

						// Launch dash using the unified gost.json config and explicit --api flag
						// Dash now supports empty config as long as an API is defined
						cmd := exec.CommandContext(ctxDash, dashPath, "-C", "gost.json", "--api", "127.0.0.1:19090")
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
