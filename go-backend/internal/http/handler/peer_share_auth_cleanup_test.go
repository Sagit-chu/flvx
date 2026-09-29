package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go-backend/internal/http/response"
	"go-backend/internal/store/repo"
)

func TestPeerShareRestrictedShareAllowsOnlyAuthenticatedCleanup(t *testing.T) {
	for _, state := range []string{"disabled", "expired", "over-quota"} {
		t.Run(state, func(t *testing.T) {
			agent := newCleanupAgent(t)
			runtime := roleRuntimeFixture(t, agent, "exit")
			changes := map[string]interface{}{"allowed_ips": "203.0.113.10", "allowed_domains": "owner.example"}
			switch state {
			case "disabled":
				changes["is_active"] = 0
			case "expired":
				changes["expiry_time"] = time.Now().Add(-time.Hour).UnixMilli()
			case "over-quota":
				changes["max_bandwidth"] = 1
				changes["current_flow"] = 2
			}
			if err := agent.h.repo.DB().Model(&repo.PeerShare{}).Where("id = ?", runtime.ShareID).Updates(changes).Error; err != nil {
				t.Fatal(err)
			}
			tests := []struct {
				command string
				allowed bool
			}{
				{"release-role", true}, {"DeleteService", true}, {"DeleteChains", true}, {"DeleteLimiters", true}, {"DeleteCLimiters", true},
				{"AddService", false}, {"UpdateService", false}, {"ResumeService", false}, {"PauseService", false}, {"DeleteEverything", false},
			}
			for _, test := range tests {
				t.Run(test.command, func(t *testing.T) {
					path := "/api/v1/federation/runtime/command"
					body := fmt.Sprintf(`{"commandType":%q,"data":{"services":["70_1_0_tcp"]}}`, test.command)
					if test.command == "release-role" {
						path = "/api/v1/federation/runtime/release-role"
						body = `{"reservationId":"role-reservation"}`
					}
					req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
					req.Header.Set("Authorization", "Bearer role-recovery-token")
					req.Header.Set("X-Panel-Domain", "owner.example")
					req.RemoteAddr = "203.0.113.10:12345"
					reached := false
					next := func(w http.ResponseWriter, r *http.Request) {
						reached = true
						received, err := io.ReadAll(r.Body)
						if err != nil || string(received) != body {
							t.Errorf("auth consumed request body: %s %v", received, err)
						}
						response.WriteJSON(w, response.OKEmpty())
					}
					agent.h.authPeer(next)(httptest.NewRecorder(), req)
					if reached != test.allowed {
						t.Fatalf("restricted %s %s allowed=%t want=%t", state, test.command, reached, test.allowed)
					}
				})
			}
			for _, invalid := range []string{"token", "domain", "ip"} {
				t.Run("reject-"+invalid, func(t *testing.T) {
					req := httptest.NewRequest(http.MethodPost, "/api/v1/federation/runtime/release-role", strings.NewReader(`{"reservationId":"role-reservation"}`))
					req.Header.Set("Authorization", "Bearer role-recovery-token")
					req.Header.Set("X-Panel-Domain", "owner.example")
					req.RemoteAddr = "203.0.113.10:12345"
					switch invalid {
					case "token":
						req.Header.Set("Authorization", "Bearer wrong-token")
					case "domain":
						req.Header.Set("X-Panel-Domain", "other.example")
					case "ip":
						req.RemoteAddr = "198.51.100.1:12345"
					}
					reached := false
					agent.h.authPeer(func(http.ResponseWriter, *http.Request) { reached = true })(httptest.NewRecorder(), req)
					if reached {
						t.Fatalf("cleanup bypassed %s authentication", invalid)
					}
				})
			}
			// Exercise the actual release handler through auth, not only the gate.
			req := httptest.NewRequest(http.MethodPost, "/api/v1/federation/runtime/release-role", strings.NewReader(`{"reservationId":"role-reservation"}`))
			req.Header.Set("Authorization", "Bearer role-recovery-token")
			req.Header.Set("X-Panel-Domain", "owner.example")
			req.RemoteAddr = "203.0.113.10:12345"
			res := httptest.NewRecorder()
			agent.h.authPeer(agent.h.federationRuntimeReleaseRole)(res, req)
			var result response.R
			if err := json.Unmarshal(res.Body.Bytes(), &result); err != nil || result.Code != 0 {
				t.Fatalf("authenticated cleanup did not complete: %s %v", res.Body.String(), err)
			}
			stored, err := agent.h.repo.GetPeerShareRuntimeByID(runtime.ID)
			if err != nil || stored.Status != 0 {
				t.Fatalf("cleanup did not release runtime: %+v %v", stored, err)
			}
		})
	}
}

func TestPeerShareOldReleaseIdentityCannotDeleteReusedReservation(t *testing.T) {
	agent := newCleanupAgent(t)
	old := roleRuntimeFixture(t, agent, "exit")
	if err := agent.h.releasePeerShareRuntime(old); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/federation/runtime/reserve-port", strings.NewReader(`{"resourceKey":"role-resource","requestedPort":31000,"protocol":"tls"}`))
	req.Header.Set("Authorization", "Bearer role-recovery-token")
	res := httptest.NewRecorder()
	agent.h.federationRuntimeReservePort(res, req)
	var result response.R
	if err := json.Unmarshal(res.Body.Bytes(), &result); err != nil || result.Code != 0 {
		t.Fatalf("new generation reserve failed: %s %v", res.Body.String(), err)
	}
	fresh, err := agent.h.repo.GetPeerShareRuntimeByID(old.ID)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.ReservationID == old.ReservationID {
		t.Fatal("reservation identity was reused")
	}
	if code := roleRuntimeRequest(t, agent.h, fmt.Sprintf(`{"reservationId":%q,"role":"exit"}`, fresh.ReservationID), false); code != 0 {
		t.Fatal("new generation apply failed")
	}
	fresh, err = agent.h.repo.GetPeerShareRuntimeByID(old.ID)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.BindingID == old.BindingID || fresh.BindingID == "" {
		t.Fatal("binding identity was reused")
	}
	deletesBefore := len(agent.commandsOfType("DeleteService"))
	for _, body := range []string{
		fmt.Sprintf(`{"bindingId":%q,"reservationId":%q,"resourceKey":"role-resource"}`, old.BindingID, old.ReservationID),
		fmt.Sprintf(`{"reservationId":%q,"resourceKey":"role-resource"}`, old.ReservationID),
	} {
		if code := roleRuntimeRequest(t, agent.h, body, true); code != 0 {
			t.Fatal("obsolete release should acknowledge completion")
		}
	}
	// Also cover an old lookup snapshot waiting behind reservation renewal.
	if err := agent.h.releasePeerShareRuntime(old); err != nil {
		t.Fatal(err)
	}
	after, err := agent.h.repo.GetPeerShareRuntimeByID(old.ID)
	if err != nil || after.Status != 1 || after.BindingID != fresh.BindingID {
		t.Fatalf("old cleanup released new generation: %+v %v", after, err)
	}
	if len(agent.commandsOfType("DeleteService")) != deletesBefore {
		t.Fatal("old cleanup sent deletion for new generation")
	}
}
