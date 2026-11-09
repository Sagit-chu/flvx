package socket

import (
	"os/exec"
	"syscall"
)

func checkAgentDetach() error { return nil }

func startDetachedAgentHelper(script string) error {
	cmd := exec.Command("/bin/sh", "-c", script)
	// The installed OpenRC service stops the agent PID. A separate session also
	// survives a process-group stop. No inherited stdio or parent-death signal.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
