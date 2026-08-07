package chain

import (
	"context"
	"testing"

	corechain "github.com/go-gost/core/chain"
	corehop "github.com/go-gost/core/hop"
)

type lifecycleTestHop struct {
	selected int
	retired  int
	closed   int
}

func (h *lifecycleTestHop) Select(context.Context, ...corehop.SelectOption) *corechain.Node {
	h.selected++
	return nil
}

func (h *lifecycleTestHop) Retire() {
	h.retired++
}

func (h *lifecycleTestHop) Close() error {
	h.closed++
	return nil
}

func TestChainRoutesThroughSharedHopWithoutOwningLifecycle(t *testing.T) {
	hop := &lifecycleTestHop{}
	chain := NewChain("shared-hop")
	chain.AddHop(hop, false)

	if route := chain.Route(context.Background(), "tcp", "example.com:443"); route == nil {
		t.Fatal("route is nil")
	}
	if hop.selected != 1 {
		t.Fatalf("shared hop selected %d times, want 1", hop.selected)
	}

	chain.Retire()
	if err := chain.Close(); err != nil {
		t.Fatalf("close chain: %v", err)
	}
	if hop.retired != 0 || hop.closed != 0 {
		t.Fatalf("shared hop lifecycle changed: retired=%d closed=%d", hop.retired, hop.closed)
	}
}

func TestChainRetiresAndClosesOwnedHop(t *testing.T) {
	hop := &lifecycleTestHop{}
	chain := NewChain("owned-hop")
	chain.AddHop(hop)

	chain.Retire()
	if err := chain.Close(); err != nil {
		t.Fatalf("close chain: %v", err)
	}
	if hop.retired != 1 || hop.closed != 1 {
		t.Fatalf("owned hop lifecycle: retired=%d closed=%d, want 1/1", hop.retired, hop.closed)
	}
}
