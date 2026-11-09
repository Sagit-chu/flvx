package socket

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// All writes are confined to TempDir; the restart callback never starts a process.
func exerciseUpgrade(t *testing.T, checksum string, serveChecksum http.HandlerFunc, valid bool) {
	t.Helper()
	dir := t.TempDir()
	binary := filepath.Join(dir, "flux_agent")
	for path, body := range map[string]string{binary: "old binary", binary + ".old": "prior backup"} {
		if err := os.WriteFile(path, []byte(body), 0700); err != nil {
			t.Fatal(err)
		}
	}
	downloads := 0
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/binary-"+runtime.GOARCH {
			downloads++
			fmt.Fprint(w, "abc")
			return
		}
		if r.URL.Path != "/checksum-"+runtime.GOARCH {
			t.Errorf("unexpected checksum path: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		serveChecksum(w, r)
	}))
	defer fixture.Close()
	if checksum == "fixture" {
		checksum = fixture.URL + "/checksum-{ARCH}"
	}
	peer := make(chan *websocket.Conn, 1)
	ws := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		peer <- c
	}))
	defer ws.Close()
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(ws.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	reader := <-peer
	defer reader.Close()
	restarted := false
	reporter := &WebSocketReporter{ctx: context.Background(), conn: conn, connected: true, upgradeBinaryPath: binary,
		startUpgradeRestart: func(staged, destination string) error {
			restarted = true
			if destination != binary {
				t.Errorf("wrong restart destination: %s", destination)
			}
			b, e := os.ReadFile(staged)
			if e != nil || string(b) != "abc" {
				t.Errorf("staged binary = %q, %v", b, e)
			}
			return nil
		},
	}
	reporter.routeCommand(CommandMessage{Type: "UpgradeAgent", RequestId: "upgrade-test-request", Data: map[string]any{"downloadUrl": fixture.URL + "/binary-{ARCH}", "checksumUrl": checksum}})
	reader.SetReadDeadline(time.Now().Add(2 * time.Second))
	verified, installing := false, false
	for {
		var response CommandResponse
		if err := reader.ReadJSON(&response); err != nil {
			t.Fatal(err)
		}
		if response.Type == "UpgradeAgentResponse" {
			if response.RequestId != "upgrade-test-request" {
				t.Errorf("response request ID = %q", response.RequestId)
			}
			if response.Success != valid {
				t.Errorf("success = %v, want %v: %s", response.Success, valid, response.Message)
			}
			break
		}
		if response.Type == "UpgradeProgress" {
			data := response.Data.(map[string]interface{})
			if data["stage"] == "verifying" && data["percent"] == float64(100) {
				verified = true
			}
			if data["stage"] == "installing" {
				installing = true
			}
		}
	}
	if restarted != valid || verified != valid || installing != valid {
		t.Errorf("restart=%v verified=%v installing=%v, want %v", restarted, verified, installing, valid)
	}
	b, e := os.ReadFile(binary)
	if e != nil || string(b) != "old binary" {
		t.Errorf("old binary changed: %q %v", b, e)
	}
	if !valid {
		b, e = os.ReadFile(binary + ".old")
		if e != nil || string(b) != "prior backup" {
			t.Errorf("backup changed: %q %v", b, e)
		}
		if _, err := os.Stat(binary + ".new"); !os.IsNotExist(err) {
			t.Errorf("staged binary remains: %v", err)
		}
	}
	if strings.TrimSpace(checksum) == "" && downloads != 0 {
		t.Errorf("downloaded without checksum URL")
	}
}

func TestUpgradeAgentRequiresChecksumURL(t *testing.T) {
	exerciseUpgrade(t, "", func(w http.ResponseWriter, r *http.Request) { t.Error("unexpected checksum request") }, false)
}

func TestUpgradeAgentRejectsChecksumHTTPFailure(t *testing.T) {
	exerciseUpgrade(t, "fixture", func(w http.ResponseWriter, r *http.Request) { http.Error(w, "unavailable", http.StatusNotFound) }, false)
}

const abcSHA256 = "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"

func TestUpgradeAgentRejectsAmbiguousChecksum(t *testing.T) {
	exerciseUpgrade(t, "fixture", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, abcSHA256+"  gost-amd64\n"+abcSHA256+"  gost-arm64\n")
	}, false)
}

func TestUpgradeAgentRejectsOversizedChecksum(t *testing.T) {
	exerciseUpgrade(t, "fixture", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, abcSHA256+"  "+strings.Repeat("a", 4096)) }, false)
}

func TestUpgradeAgentChecksumRecords(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		valid      bool
	}{
		{"bare", abcSHA256, true},
		{"GNU text", abcSHA256 + "  gost-arm64\n", true},
		{"GNU binary", abcSHA256 + " *gost-arm64\n", true},
		{"uppercase CRLF surrounding whitespace", " \t" + strings.ToUpper(abcSHA256) + "  gost-arm64\r\n", true},
		{"bare surrounding whitespace", "\r\n\t" + abcSHA256 + " \r\n", true},
		{"empty", "", false},
		{"whitespace", " \r\n\t", false},
		{"truncated", abcSHA256[:63], false},
		{"nonhex", "z" + abcSHA256[1:], false},
		{"mismatch", strings.Repeat("0", 64), false},
		{"missing GNU marker", abcSHA256 + " gost-arm64", false},
		{"invalid GNU marker", abcSHA256 + " ?gost-arm64", false},
		{"missing filename", abcSHA256 + " *", false},
		{"appended garbage", abcSHA256 + "garbage", false},
		{"embedded CR", abcSHA256 + "  gost\rarm64", false},
		{"NUL", abcSHA256 + "  gost\x00arm64", false},
		{"two bare digests", abcSHA256 + "\n" + abcSHA256, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			exerciseUpgrade(t, "fixture", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, tc.body) }, tc.valid)
		})
	}
}

func TestUpgradeAgentRejectsChecksumTransportFailure(t *testing.T) {
	exerciseUpgrade(t, "://invalid", nil, false)
}

func TestUpgradeAgentRejectsChecksumReadFailure(t *testing.T) {
	exerciseUpgrade(t, "fixture", func(w http.ResponseWriter, r *http.Request) {
		// A valid-looking record is incomplete relative to the declared body length.
		w.Header().Set("Content-Length", "999")
		fmt.Fprint(w, abcSHA256)
	}, false)
}

func TestUpgradeAgentRejectsBlankChecksumURL(t *testing.T) {
	exerciseUpgrade(t, " \t\r\n", nil, false)
}

func TestUpgradeAgentRejectsChecksumConnectionFailure(t *testing.T) {
	exerciseUpgrade(t, "fixture", func(w http.ResponseWriter, r *http.Request) {
		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		conn.Close()
	}, false)
}
