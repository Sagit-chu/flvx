package socket

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func routeAgentOperation(t *testing.T, reporter *WebSocketReporter, operation string, data interface{}) CommandResponse {
	t.Helper()
	peer := make(chan *websocket.Conn, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err == nil {
			peer <- c
		}
	}))
	defer server.Close()
	c, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	reader := <-peer
	defer reader.Close()
	reporter.ctx = context.Background()
	reporter.conn = c
	reporter.connected = true
	reporter.routeCommand(CommandMessage{Type: operation, RequestId: "restart-request", Data: data})
	reader.SetReadDeadline(time.Now().Add(3 * time.Second))
	for {
		var response CommandResponse
		if err := reader.ReadJSON(&response); err != nil {
			t.Fatal(err)
		}
		if response.Type == operation+"Response" {
			if response.RequestId != "restart-request" {
				t.Fatalf("request id lost: %s", response.RequestId)
			}
			return response
		}
	}
}

func TestRollbackAgentStagesBackupBeforeRestart(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "flux_agent")
	for path, body := range map[string]string{binary: "current", binary + ".old": "backup"} {
		if err := os.WriteFile(path, []byte(body), 0755); err != nil {
			t.Fatal(err)
		}
	}
	called := false
	reporter := &WebSocketReporter{upgradeBinaryPath: binary, startUpgradeRestart: func(staged, dest string) error {
		called = true
		body, err := os.ReadFile(staged)
		if err != nil || string(body) != "backup" || staged == binary+".old" || dest != binary {
			t.Fatalf("bad rollback staging: %s %s %q %v", staged, dest, body, err)
		}
		return os.Rename(staged, dest)
	}}
	response := routeAgentOperation(t, reporter, "RollbackAgent", nil)
	if !response.Success || !called {
		t.Fatalf("rollback failed: called=%v %s", called, response.Message)
	}
	body, err := os.ReadFile(binary + ".old")
	if err != nil || string(body) != "backup" {
		t.Fatalf("backup consumed: %q %v", body, err)
	}
}

func TestAgentServiceManagerDetection(t *testing.T) {
	for _, tc := range []struct {
		name            string
		alpine, systemd bool
		tools           string
		want            string
	}{
		{"alpine shim", true, true, "rc-service systemctl systemd-run", "openrc"},
		{"alpine missing openrc", true, true, "systemctl systemd-run", ""},
		{"systemd", false, true, "rc-service systemctl systemd-run", "systemd"},
		{"missing runner", false, true, "systemctl", ""},
		{"openrc fallback", false, false, "rc-service", "openrc"},
		{"unmanaged", false, false, "systemctl systemd-run", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			manager, err := detectAgentServiceManager(tc.alpine, tc.systemd, func(name string) (string, error) {
				if strings.Contains(" "+tc.tools+" ", " "+name+" ") {
					return name, nil
				}
				return "", os.ErrNotExist
			})
			if manager.kind != tc.want || (err != nil) != (tc.want == "") {
				t.Fatalf("manager=%+v err=%v", manager, err)
			}
		})
	}
}

func writeAgentFixture(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0755); err != nil {
		t.Fatal(err)
	}
}
func readAgentFixture(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}
func waitAgentFixture(t *testing.T, path, want string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		body, _ := os.ReadFile(path)
		if string(body) == want {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	body, _ := os.ReadFile(path)
	t.Fatalf("%s = %q, want %q", path, body, want)
}

func TestAgentCommandsManagerLaunch(t *testing.T) {
	for _, operation := range []string{"UpgradeAgent", "RollbackAgent"} {
		for _, mode := range []string{"systemd", "openrc", "missing", "rejected"} {
			t.Run(operation+"/"+mode, func(t *testing.T) {
				if mode == "openrc" && runtime.GOOS != "linux" {
					t.Skip("detached launcher is Linux-only")
				}
				dir := t.TempDir()
				binary := filepath.Join(dir, "flux 'agent")
				log := filepath.Join(dir, "events")
				writeAgentFixture(t, binary, "current")
				writeAgentFixture(t, binary+".old", "backup")
				control := filepath.Join(dir, "manager")
				writeAgentFixture(t, control, "#!/bin/sh\nprintf '%s\\n' \"$*\" >> "+shellQuote(log)+"\n")
				runner := filepath.Join(dir, "runner")
				writeAgentFixture(t, runner, "#!/bin/sh\nshift\n\"$@\" >/dev/null 2>&1 &\n")
				if mode == "rejected" {
					writeAgentFixture(t, runner, "#!/bin/sh\necho rejected >&2\nexit 9\n")
				}
				reporter := &WebSocketReporter{upgradeBinaryPath: binary, detectAgentManager: func() (agentServiceManager, error) {
					return detectAgentServiceManager(mode == "openrc", mode != "missing", func(name string) (string, error) {
						if mode == "missing" {
							return "", os.ErrNotExist
						}
						if name == "systemd-run" {
							return runner, nil
						}
						return control, nil
					})
				}}
				fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/checksum" {
						fmt.Fprint(w, abcSHA256)
					} else {
						fmt.Fprint(w, "abc")
					}
				}))
				defer fixture.Close()
				response := routeAgentOperation(t, reporter, operation, map[string]string{"downloadUrl": fixture.URL + "/binary", "checksumUrl": fixture.URL + "/checksum"})
				if mode == "missing" || mode == "rejected" {
					if response.Success {
						t.Fatal("accepted failed manager launch")
					}
					if readAgentFixture(t, binary) != "current" {
						t.Fatal("live binary mutated")
					}
					if mode == "missing" && readAgentFixture(t, binary+".old") != "backup" {
						t.Fatal("backup mutated before manager validation")
					}
					matches, _ := filepath.Glob(filepath.Join(dir, ".flux-agent-*"))
					if len(matches) != 0 {
						t.Fatalf("staging residue %v", matches)
					}
					if _, err := os.Stat(binary + ".new"); !os.IsNotExist(err) {
						t.Fatal("upgrade residue")
					}
					return
				}
				if !response.Success {
					t.Fatal(response.Message)
				}
				want, backup := "abc", "current"
				if operation == "RollbackAgent" {
					want, backup = "backup", "backup"
				}
				waitAgentFixture(t, binary, want)
				events := "stop flux_agent\nstart flux_agent\n"
				if mode == "openrc" {
					events = "flux_agent stop\nflux_agent start\n"
				}
				waitAgentFixture(t, log, events)
				if readAgentFixture(t, binary+".old") != backup {
					t.Fatal("wrong backup")
				}
			})
		}
	}
}

func TestAgentRestartScriptFailureOrdering(t *testing.T) {
	for _, mode := range []string{"stop", "replace"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			binary := filepath.Join(dir, "agent")
			staged := filepath.Join(dir, "staged")
			log := filepath.Join(dir, "events")
			writeAgentFixture(t, binary, "current")
			writeAgentFixture(t, staged, "replacement")
			control := filepath.Join(dir, "rc-service")
			script := "#!/bin/sh\necho \"$*\" >> " + shellQuote(log) + "\n"
			if mode == "stop" {
				script += "[ \"$2\" != stop ]\n"
			}
			writeAgentFixture(t, control, script)
			if mode == "replace" {
				writeAgentFixture(t, filepath.Join(dir, "mv"), "#!/bin/sh\nexit 23\n")
				t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
			}
			cmd := exec.Command("/bin/sh", "-c", buildManagedAgentRestartScript(agentServiceManager{kind: "openrc", control: control}, staged, binary, true))
			if err := cmd.Run(); err == nil {
				t.Fatal("expected failed stop/replacement")
			}
			if readAgentFixture(t, binary) != "current" {
				t.Fatal("changed binary after failure")
			}
			want := "flux_agent stop\n"
			if mode == "replace" {
				want += "flux_agent start\n"
			}
			if readAgentFixture(t, log) != want {
				t.Fatalf("wrong sequence %q", readAgentFixture(t, log))
			}
			if _, err := os.Stat(staged); !os.IsNotExist(err) {
				t.Fatal("staging residue")
			}
		})
	}
}

func TestUpgradeAgentBackupFailurePreservesExistingBackup(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "agent")
	writeAgentFixture(t, binary+".old", "prior")
	called := false
	reporter := &WebSocketReporter{upgradeBinaryPath: binary, startUpgradeRestart: func(string, string) error { called = true; return nil }}
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/checksum" {
			fmt.Fprint(w, abcSHA256)
		} else {
			fmt.Fprint(w, "abc")
		}
	}))
	defer fixture.Close()
	response := routeAgentOperation(t, reporter, "UpgradeAgent", map[string]string{"downloadUrl": fixture.URL + "/binary", "checksumUrl": fixture.URL + "/checksum"})
	if response.Success || called {
		t.Fatal("upgrade accepted without backup")
	}
	if readAgentFixture(t, binary+".old") != "prior" {
		t.Fatal("backup lost")
	}
	if _, err := os.Stat(binary + ".new"); !os.IsNotExist(err) {
		t.Fatal("staging residue")
	}
}

func TestRollbackAgentRejectsDirectoryDestination(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "agent")
	if err := os.Mkdir(binary, 0755); err != nil {
		t.Fatal(err)
	}
	writeAgentFixture(t, binary+".old", "backup")
	called := false
	reporter := &WebSocketReporter{upgradeBinaryPath: binary, startUpgradeRestart: func(string, string) error { called = true; return nil }}
	response := routeAgentOperation(t, reporter, "RollbackAgent", nil)
	if response.Success || called {
		t.Fatal("accepted directory destination")
	}
	if readAgentFixture(t, binary+".old") != "backup" {
		t.Fatal("changed backup")
	}
	entries, err := os.ReadDir(binary)
	if err != nil || len(entries) != 0 {
		t.Fatalf("destination changed: %v %v", entries, err)
	}
}

func TestAgentRestartScriptRejectsDirectoryDestination(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "agent")
	staged := filepath.Join(dir, "stage")
	if err := os.Mkdir(binary, 0755); err != nil {
		t.Fatal(err)
	}
	writeAgentFixture(t, staged, "replacement")
	script := buildManagedAgentRestartScript(agentServiceManager{kind: "openrc", control: "/usr/bin/true"}, staged, binary, false)
	if err := exec.Command("/bin/sh", "-c", script).Run(); err == nil {
		t.Fatal("accepted directory destination")
	}
	entries, err := os.ReadDir(binary)
	if err != nil || len(entries) != 0 {
		t.Fatalf("binary moved inside directory: %v %v", entries, err)
	}
}

func TestOpenRCRestartNeverCleansSystemdLegacyService(t *testing.T) {
	script := buildManagedAgentRestartScript(agentServiceManager{kind: "openrc", control: "rc-service"}, "staged", "binary", true)
	for _, forbidden := range []string{"systemctl", "gost", "daemon-reload", "/etc/systemd"} {
		if strings.Contains(script, forbidden) {
			t.Fatalf("OpenRC helper contains %s: %s", forbidden, script)
		}
	}
}
