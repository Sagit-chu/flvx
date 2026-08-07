package chain

import (
	"context"
	"net"
	"testing"

	corechain "github.com/go-gost/core/chain"
)

type copyTestRoute struct{}

func (copyTestRoute) Dial(context.Context, string, string, ...corechain.DialOption) (net.Conn, error) {
	return nil, nil
}

func (copyTestRoute) Bind(context.Context, string, string, ...corechain.BindOption) (net.Listener, error) {
	return nil, nil
}

func (copyTestRoute) Nodes() []*corechain.Node {
	return nil
}

func TestTransportCopyReturnsIndependentTransport(t *testing.T) {
	originalRoute := copyTestRoute{}
	replacementRoute := &copyTestRoute{}
	original := NewTransport(nil, nil, corechain.RouteTransportOption(originalRoute))

	copied, ok := original.Copy().(*Transport)
	if !ok {
		t.Fatalf("copy type = %T, want *Transport", original.Copy())
	}
	if copied == original {
		t.Fatal("Copy returned the original transport")
	}

	copied.Options().Route = replacementRoute
	if original.Options().Route != originalRoute {
		t.Fatal("mutating copied transport changed original route")
	}
	if copied.Options().Route != replacementRoute {
		t.Fatal("copied transport did not retain its independent route")
	}
}
