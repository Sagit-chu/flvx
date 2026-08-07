package mtls

import (
	"context"
	"errors"
	"net"
	"testing"
)

func TestRetiredDialerRejectsNewConnections(t *testing.T) {
	dialer := NewDialer().(*mtlsDialer)
	dialer.Retire()

	if _, err := dialer.Dial(context.Background(), "127.0.0.1:1"); !errors.Is(err, net.ErrClosed) {
		t.Fatalf("Dial error = %v, want net.ErrClosed", err)
	}
}

func TestSessionCloseReleasesPreHandshakeConnection(t *testing.T) {
	conn, peer := net.Pipe()
	defer peer.Close()
	session := &muxSession{conn: conn}

	if err := session.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !session.IsClosed() {
		t.Fatal("pre-handshake session still reports open after Close")
	}
}
