package registry

import (
	"context"
	"net"
	"testing"

	"github.com/go-gost/core/chain"
)

type testChainer struct {
	route chain.Route
}

func (c testChainer) Route(context.Context, string, string, ...chain.RouteOption) chain.Route {
	return c.route
}

type testRoute struct {
	nodes []*chain.Node
}

func (r testRoute) Dial(context.Context, string, string, ...chain.DialOption) (net.Conn, error) {
	return nil, nil
}

func (r testRoute) Bind(context.Context, string, string, ...chain.BindOption) (net.Listener, error) {
	return nil, nil
}

func (r testRoute) Nodes() []*chain.Node {
	return r.nodes
}

func TestReplaceChainOverwritesExistingRegistration(t *testing.T) {
	name := "replace_chain_tdd"
	ChainRegistry().Unregister(name)
	defer ChainRegistry().Unregister(name)

	if err := ChainRegistry().Register(name, testChainer{route: testRoute{nodes: []*chain.Node{{Name: "old"}}}}); err != nil {
		t.Fatalf("register old chain: %v", err)
	}
	if err := ReplaceChain(name, testChainer{route: testRoute{nodes: []*chain.Node{{Name: "new"}}}}); err != nil {
		t.Fatalf("replace chain: %v", err)
	}

	route := ChainRegistry().Get(name).Route(context.Background(), "tcp", "example.com:443")
	if route == nil || len(route.Nodes()) != 1 || route.Nodes()[0].Name != "new" {
		t.Fatalf("expected replacement chain route, got %#v", route)
	}
}
