package socket

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// This subprocess stands in for the running agent. The rc-service double kills
// its entire process group, not merely a fake PID, while the real helper runs.
func TestAgentDetachedParent(t *testing.T) {
	dir := os.Getenv("FLVX_TEST_DETACH_ROOT")
	if dir == "" {
		t.Skip("subprocess helper")
	}
	binary := filepath.Join(dir, "agent")
	control := filepath.Join(dir, "rc-service")
	writeAgentFixture(t, control, "#!/bin/sh\nif [ \"$2\" = stop ]; then kill -TERM -"+strconv.Itoa(os.Getpid())+"; else echo started > "+shellQuote(filepath.Join(dir, "started"))+"; fi\n")
	reporter := &WebSocketReporter{upgradeBinaryPath: binary, detectAgentManager: func() (agentServiceManager, error) { return agentServiceManager{kind: "openrc", control: control}, nil }}
	response := routeAgentOperation(t, reporter, "RollbackAgent", nil)
	if !response.Success {
		t.Fatal(response.Message)
	}
	time.Sleep(15 * time.Second)
	t.Fatal("original agent was not stopped")
}

func TestOpenRCHelperSurvivesAgentProcessGroupStop(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "agent")
	writeAgentFixture(t, binary, "current")
	writeAgentFixture(t, binary+".old", "backup")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestAgentDetachedParent$", "-test.v")
	cmd.Env = append(os.Environ(), "FLVX_TEST_DETACH_ROOT="+dir)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	output, err := cmd.CombinedOutput()
	if err == nil || ctx.Err() != nil {
		t.Fatalf("parent termination = %v, timeout=%v, %s", err, ctx.Err(), output)
	}
	exit, ok := err.(*exec.ExitError)
	if !ok || exit.Sys().(syscall.WaitStatus).Signal() != syscall.SIGTERM {
		t.Fatalf("parent did not receive SIGTERM: %v %s", err, output)
	}
	waitAgentFixture(t, filepath.Join(dir, "started"), "started\n")
	if readAgentFixture(t, binary) != "backup" || readAgentFixture(t, binary+".old") != "backup" {
		t.Fatal("rollback failed after parent termination")
	}
	fmt.Println("detached OpenRC helper survived original agent process-group SIGTERM")
}

// Run only in an explicitly opted-in disposable Alpine container. The docker
// invocation mounts only the test binary, never a host /bin directory. Removing
// the container's /bin/sh symlink forces the real detached cmd.Start to fail.
func TestOpenRCCommandsCleanStageOnHelperExecFailure(t *testing.T) {
	if os.Getenv("FLVX_TEST_DISPOSABLE_ALPINE_SHELL_FAILURE") != "1" {
		t.Skip("requires explicit disposable Alpine container opt-in")
	}
	for _, path := range []string{"/etc/alpine-release", "/.dockerenv"} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("refusing shell mutation outside disposable Alpine: %s: %v", path, err)
		}
	}
	info, err := os.Lstat("/bin/sh")
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("refusing to rename a non-symlink /bin/sh: %v", err)
	}
	saved := "/bin/.flvx-test-sh-" + strconv.Itoa(os.Getpid())
	if _, err := os.Lstat(saved); !os.IsNotExist(err) {
		t.Fatalf("refusing to overwrite shell backup: %v", err)
	}
	if err := os.Rename("/bin/sh", saved); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Rename(saved, "/bin/sh"); err != nil {
			t.Errorf("restore container shell: %v", err)
		}
	}()
	for _, operation := range []string{"UpgradeAgent", "RollbackAgent"} {
		t.Run(operation, func(t *testing.T) {
			dir := t.TempDir()
			binary := filepath.Join(dir, "agent")
			writeAgentFixture(t, binary, "current")
			writeAgentFixture(t, binary+".old", "prior backup")
			writeAgentFixture(t, filepath.Join(dir, "rc-service"), "#!/bin/sh\nexit 0\n")
			t.Setenv("PATH", dir)
			// No manager or launcher injection: Alpine detection and exec.LookPath are
			// production code too. Only the installation path points to an isolated dir.
			reporter := &WebSocketReporter{upgradeBinaryPath: binary}
			fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/checksum" {
					fmt.Fprint(w, abcSHA256)
				} else {
					fmt.Fprint(w, "abc")
				}
			}))
			defer fixture.Close()
			response := routeAgentOperation(t, reporter, operation, map[string]string{"downloadUrl": fixture.URL + "/binary", "checksumUrl": fixture.URL + "/checksum"})
			if response.Success || !strings.Contains(response.Message, "/bin/sh") || !strings.Contains(response.Message, "no such file") {
				t.Fatalf("expected real helper exec failure, got %+v", response)
			}
			if readAgentFixture(t, binary) != "current" {
				t.Fatal("live binary changed")
			}
			backup := "prior backup"
			if operation == "UpgradeAgent" {
				backup = "current"
			}
			if readAgentFixture(t, binary+".old") != backup {
				t.Fatal("unexpected backup contents")
			}
			matches, err := filepath.Glob(filepath.Join(dir, ".flux-agent-*"))
			if err != nil || len(matches) != 0 {
				t.Fatalf("rollback staging residue: %v %v", matches, err)
			}
			if _, err := os.Stat(binary + ".new"); !os.IsNotExist(err) {
				t.Fatalf("upgrade staging residue: %v", err)
			}
		})
	}
}
