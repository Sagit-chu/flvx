package kcp

import (
	"context"
	"errors"
	"net"
	"testing"
)

func TestRetiredDialerRejectsNewConnections(t *testing.T) {
	dialer := NewDialer().(*kcpDialer)
	dialer.Retire()

	if _, err := dialer.Dial(context.Background(), "127.0.0.1:1"); !errors.Is(err, net.ErrClosed) {
		t.Fatalf("Dial error = %v, want net.ErrClosed", err)
	}
}
