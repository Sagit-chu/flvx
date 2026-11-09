package socket

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type agentServiceManager struct{ kind, control, runner string }

// Alpine must never select a systemctl compatibility shim. Elsewhere prefer
// an active systemd instance, matching the installer, then OpenRC.
func detectAgentServiceManager(alpine, systemd bool, lookup func(string) (string, error)) (agentServiceManager, error) {
	if !alpine && systemd {
		control, err := lookup("systemctl")
		if err != nil {
			return agentServiceManager{}, fmt.Errorf("systemctl 不可用: %w", err)
		}
		runner, err := lookup("systemd-run")
		if err != nil {
			return agentServiceManager{}, fmt.Errorf("systemd-run 不可用: %w", err)
		}
		return agentServiceManager{kind: "systemd", control: control, runner: runner}, nil
	}
	control, err := lookup("rc-service")
	if err != nil {
		return agentServiceManager{}, fmt.Errorf("未检测到受支持的服务管理器 (systemd/OpenRC): %w", err)
	}
	return agentServiceManager{kind: "openrc", control: control}, nil
}

func (w *WebSocketReporter) agentRestartLauncher(cleanLegacy bool) (func(string, string) error, error) {
	// Existing per-reporter seam is also used by checksum-only tests.
	if w.startUpgradeRestart != nil {
		return w.startUpgradeRestart, nil
	}
	detect := w.detectAgentManager
	if detect == nil {
		detect = func() (agentServiceManager, error) {
			_, alpineErr := os.Stat("/etc/alpine-release")
			_, systemdErr := os.Stat("/run/systemd/system")
			return detectAgentServiceManager(alpineErr == nil, systemdErr == nil, exec.LookPath)
		}
	}
	manager, err := detect()
	if err != nil {
		return nil, err
	}
	if manager.kind == "openrc" {
		if err := checkAgentDetach(); err != nil {
			return nil, err
		}
	}
	return func(staged, binary string) error {
		script := buildManagedAgentRestartScript(manager, staged, binary, cleanLegacy)
		if manager.kind == "systemd" {
			// Wait only for submission, never for helper completion. A separate unit is
			// essential: systemctl stop kills every process in the agent's cgroup.
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			out, err := exec.CommandContext(ctx, manager.runner, "--quiet", "/bin/sh", "-c", script).CombinedOutput()
			if err != nil {
				return fmt.Errorf("提交重启任务失败: %w: %s", err, strings.TrimSpace(string(out)))
			}
			return nil
		}
		return startDetachedAgentHelper(script)
	}, nil
}

func shellQuote(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'" }

func buildManagedAgentRestartScript(manager agentServiceManager, staged, binary string, cleanLegacy bool) string {
	control := shellQuote(manager.control)
	stop, start := control+" stop flux_agent", control+" start flux_agent"
	if manager.kind == "openrc" {
		stop, start = control+" flux_agent stop", control+" flux_agent start"
	}
	legacy := ""
	if manager.kind == "systemd" && cleanLegacy {
		legacy = `legacy_service=''; for service_file in /etc/systemd/system/gost.service /lib/systemd/system/gost.service /usr/lib/systemd/system/gost.service; do if [ -f "$service_file" ] && grep -Fq "WorkingDirectory=/etc/gost" "$service_file" && (grep -Fq "ExecStart=/etc/gost/gost" "$service_file" || (grep -Fq "ExecStart=/usr/local/bin/gost" "$service_file" && [ -f /etc/gost/config.json ] && [ -f /etc/gost/gost.json ])); then legacy_service="$service_file"; break; fi; done; if [ -n "$legacy_service" ]; then (` + control + ` stop gost 2>/dev/null || true); (` + control + ` disable gost 2>/dev/null || true); rm -f /usr/local/bin/gost /etc/gost/gost "$legacy_service"; (` + control + ` daemon-reload 2>/dev/null || true); fi; `
	}
	// The caller owns staging until launch. Afterwards the helper always removes
	// it, including stop/replace failures. Never truncate the running executable.
	return "trap " + shellQuote("rm -f -- "+shellQuote(staged)) + " EXIT; sleep 1; " + stop + " || exit $?; " + legacy +
		"if [ ! -d " + shellQuote(binary) + " ] && mv -f -- " + shellQuote(staged) + " " + shellQuote(binary) + "; then " + start + "; else status=$?; " + start + "; exit \"$status\"; fi"
}

// Copy to a unique file on the destination filesystem, then let the caller
// atomically rename. Failure never truncates a previous usable backup.
func stageAgentCopy(source, destination string) (path string, err error) {
	in, err := os.Open(source)
	if err != nil {
		return "", err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("不是普通文件: %s", source)
	}
	out, err := os.CreateTemp(filepath.Dir(destination), ".flux-agent-*")
	if err != nil {
		return "", err
	}
	path = out.Name()
	defer func() {
		out.Close()
		if err != nil {
			os.Remove(path)
		}
	}()
	if _, err = io.Copy(out, in); err != nil {
		return path, err
	}
	if err = out.Chmod(0755); err != nil {
		return path, err
	}
	if err = out.Close(); err != nil {
		return path, err
	}
	return path, nil
}

func backupAgentBinary(binary string) error {
	staged, err := stageAgentCopy(binary, binary+".old")
	if err != nil {
		return err
	}
	defer os.Remove(staged)
	return os.Rename(staged, binary+".old")
}
