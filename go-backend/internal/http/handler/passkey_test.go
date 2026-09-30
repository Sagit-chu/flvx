package handler

import (
	"testing"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
)

func TestPasskeyChallengeExpiresAndIsSingleUse(t *testing.T) {
	h := &Handler{passkeyPending: make(map[string]passkeyCeremony)}
	id, ok := h.putPasskeyCeremony(passkeyCeremony{userID: 7, kind: "login", origin: "https://panel.example.test", session: webauthn.SessionData{Expires: time.Now().Add(time.Minute)}})
	if !ok {
		t.Fatal("could not create challenge")
	}
	if _, ok := h.takePasskeyCeremony(id, "login", "https://panel.example.test", 7); !ok {
		t.Fatal("valid challenge was rejected")
	}
	if _, ok := h.takePasskeyCeremony(id, "login", "https://panel.example.test", 7); ok {
		t.Fatal("challenge was reusable")
	}
	id, ok = h.putPasskeyCeremony(passkeyCeremony{userID: 7, kind: "login", origin: "https://panel.example.test", session: webauthn.SessionData{Expires: time.Now().Add(-time.Second)}})
	if !ok {
		t.Fatal("could not create expired challenge")
	}
	if _, ok := h.takePasskeyCeremony(id, "login", "https://panel.example.test", 7); ok {
		t.Fatal("expired challenge was accepted")
	}
}
