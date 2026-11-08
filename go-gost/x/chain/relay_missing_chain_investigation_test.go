package chain_test

import (
	"context"
	corechain "github.com/go-gost/core/chain"
	"github.com/go-gost/core/connector"
	"github.com/go-gost/core/handler"
	corelogger "github.com/go-gost/core/logger"
	xchain "github.com/go-gost/x/chain"
	"github.com/go-gost/x/config"
	parser "github.com/go-gost/x/config/parsing/chain"
	relayconnector "github.com/go-gost/x/connector/relay"
	_ "github.com/go-gost/x/dialer/tcp"
	relayhandler "github.com/go-gost/x/handler/relay"
	xlogger "github.com/go-gost/x/logger"
	"github.com/go-gost/x/metadata"
	"github.com/go-gost/x/registry"
	"io"
	"net"
	"sync/atomic"
	"testing"
	"time"
)

// This test documents existing fail-open behavior, not desired secure behavior.
// Every socket is bound to 127.0.0.1, no production config or external target.
func TestRelayMissingChainDataPathInvestigation(t *testing.T) {
	log := xlogger.Nop()
	originalLogger := corelogger.Default()
	corelogger.SetDefault(log)
	if originalLogger != nil {
		defer corelogger.SetDefault(originalLogger)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	target, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	go func() {
		for {
			c, e := target.Accept()
			if e != nil {
				return
			}
			go func() { defer c.Close(); io.Copy(c, c) }()
		}
	}()
	type relay struct {
		addr  string
		count *atomic.Int32
	}
	start := func(name, chainName string) relay {
		ln, e := net.Listen("tcp", "127.0.0.1:0")
		if e != nil {
			t.Fatal(e)
		}
		t.Cleanup(func() { ln.Close() })
		router := xchain.NewRouter(corechain.ChainRouterOption(registry.ChainRegistry().Get(chainName)), corechain.LoggerRouterOption(log))
		h := relayhandler.NewHandler(handler.RouterOption(router), handler.LoggerOption(log), handler.ServiceOption(name))
		if e := h.Init(metadata.NewMetadata(map[string]any{"nodelay": true})); e != nil {
			t.Fatal(e)
		}
		t.Cleanup(func() {
			if closer, ok := h.(io.Closer); ok {
				closer.Close()
			}
		})
		count := &atomic.Int32{}
		go func() {
			for {
				c, e := ln.Accept()
				if e != nil {
					return
				}
				count.Add(1)
				go h.Handle(ctx, c)
			}
		}()
		return relay{ln.Addr().String(), count}
	}
	replace := func(name, addr string) {
		c, e := parser.ParseChain(&config.ChainConfig{Name: name, Hops: []*config.HopConfig{{Name: name + "-hop", Nodes: []*config.NodeConfig{{Name: name + "-next", Addr: addr, Connector: &config.ConnectorConfig{Type: "relay", Metadata: map[string]any{"nodelay": true}}, Dialer: &config.DialerConfig{Type: "tcp"}}}}}}, log)
		if e != nil {
			t.Fatal(e)
		}
		if e = registry.ReplaceChain(name, c); e != nil {
			t.Fatal(e)
		}
	}
	for _, name := range []string{"investigation-A", "investigation-B", "investigation-C"} {
		name := name
		t.Cleanup(func() { registry.ChainRegistry().Unregister(name) })
	}
	d := start("D", "")
	c := start("C", "investigation-C")
	replace("investigation-C", d.addr)
	b := start("B", "investigation-B")
	replace("investigation-B", d.addr)
	a := start("A", "investigation-A")
	replace("investigation-A", b.addr)
	request := func() {
		raw, e := net.DialTimeout("tcp", a.addr, time.Second)
		if e != nil {
			t.Fatal(e)
		}
		defer raw.Close()
		raw.SetDeadline(time.Now().Add(3 * time.Second))
		client := relayconnector.NewConnector(connector.LoggerOption(log))
		if e := client.Init(metadata.NewMetadata(map[string]any{"nodelay": true})); e != nil {
			t.Fatal(e)
		}
		conn, e := client.Connect(ctx, raw, "tcp", target.Addr().String())
		if e != nil {
			t.Fatal(e)
		}
		if _, e = conn.Write([]byte("probe")); e != nil {
			t.Fatal(e)
		}
		buf := make([]byte, 5)
		if _, e = io.ReadFull(conn, buf); e != nil {
			t.Fatal(e)
		}
		if string(buf) != "probe" {
			t.Fatalf("bad response %q", buf)
		}
	}
	request()
	if b.count.Load() != 1 || c.count.Load() != 0 || d.count.Load() != 1 {
		t.Fatalf("initial B/C/D: %d/%d/%d", b.count.Load(), c.count.Load(), d.count.Load())
	}
	replace("investigation-B", c.addr)
	request()
	if b.count.Load() != 2 || c.count.Load() != 1 || d.count.Load() != 2 {
		t.Fatalf("insert C B/C/D: %d/%d/%d", b.count.Load(), c.count.Load(), d.count.Load())
	}
	// Deterministic cleanup boundary: DeleteChains has completed, while the B
	// relay service is still alive (before a subsequent DeleteService command).
	registry.ChainRegistry().Unregister("investigation-B")
	request()
	if b.count.Load() != 3 || c.count.Load() != 1 || d.count.Load() != 2 {
		t.Fatalf("missing chain B/C/D: %d/%d/%d", b.count.Load(), c.count.Load(), d.count.Load())
	}
	t.Log("Confirmed real relay data flow: A-B-D, then A-B-C-D, then A-B-target with B chain missing; no credentials supplied")
	replace("investigation-B", c.addr)
	request()
	if b.count.Load() != 4 || c.count.Load() != 2 || d.count.Load() != 3 {
		t.Fatalf("restored B/C/D: %d/%d/%d", b.count.Load(), c.count.Load(), d.count.Load())
	}
}
