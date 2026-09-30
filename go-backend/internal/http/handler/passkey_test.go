package handler

import (
	"testing"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
)

func TestPasskeyChallengeExpiresAndIsSingleUse(t *testing.T) {
	h := &Handler{passkeyPending: make(map[string]passkeyCeremony)}
	id, ok := h.putPasskeyCeremony(passkeyCeremony{userID: 7, kind: "register", origin: "https://panel.example.test", session: webauthn.SessionData{Expires: time.Now().Add(time.Minute)}})
	if !ok {
		t.Fatal("could not create challenge")
	}
	if _, ok := h.takePasskeyCeremony(id, "register", "https://panel.example.test", 7); !ok {
		t.Fatal("valid challenge was rejected")
	}
	if _, ok := h.takePasskeyCeremony(id, "register", "https://panel.example.test", 7); ok {
		t.Fatal("challenge was reusable")
	}
	id, ok = h.putPasskeyCeremony(passkeyCeremony{userID: 7, kind: "register", origin: "https://panel.example.test", session: webauthn.SessionData{Expires: time.Now().Add(-time.Second)}})
	if !ok {
		t.Fatal("could not create expired challenge")
	}
	if _, ok := h.takePasskeyCeremony(id, "register", "https://panel.example.test", 7); ok {
		t.Fatal("expired challenge was accepted")
	}
	id, ok = h.putPasskeyCeremony(passkeyCeremony{kind: "login-discoverable", origin: "https://panel.example.test", session: webauthn.SessionData{Expires: time.Now().Add(time.Minute)}})
	if !ok {
		t.Fatal("could not create discoverable challenge")
	}
	if _, ok := h.takePasskeyLoginCeremony(id, "https://wrong.example.test"); ok {
		t.Fatal("wrong origin was accepted")
	}
	if _, ok := h.takePasskeyLoginCeremony(id, "https://panel.example.test"); ok {
		t.Fatal("failed discoverable challenge was reusable")
	}
	id, ok = h.putPasskeyCeremony(passkeyCeremony{kind: "login-discoverable", origin: "https://panel.example.test", session: webauthn.SessionData{Expires: time.Now().Add(time.Minute)}})
	if !ok {
		t.Fatal("could not create discoverable challenge")
	}
	if _, ok := h.takePasskeyLoginCeremony(id, "https://panel.example.test"); !ok {
		t.Fatal("valid discoverable challenge was rejected")
	}
	if _, ok := h.takePasskeyLoginCeremony(id, "https://panel.example.test"); ok {
		t.Fatal("discoverable challenge was reusable")
	}
	id, ok = h.putPasskeyCeremony(passkeyCeremony{kind: "login-discoverable", origin: "https://panel.example.test", session: webauthn.SessionData{Expires: time.Now().Add(-time.Second)}})
	if !ok {
		t.Fatal("could not create expired discoverable challenge")
	}
	if _, ok := h.takePasskeyLoginCeremony(id, "https://panel.example.test"); ok {
		t.Fatal("expired discoverable challenge was accepted")
	}
}
