package chain_test

import (
	"context"
	"io"
	"net"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	corechain "github.com/go-gost/core/chain"
	"github.com/go-gost/core/connector"
	"github.com/go-gost/core/handler"
	corelogger "github.com/go-gost/core/logger"
	xauth "github.com/go-gost/x/auth"
	xchain "github.com/go-gost/x/chain"
	"github.com/go-gost/x/config"
	parser "github.com/go-gost/x/config/parsing/chain"
	relayconnector "github.com/go-gost/x/connector/relay"
	_ "github.com/go-gost/x/dialer/tcp"
	relayhandler "github.com/go-gost/x/handler/relay"
	xlogger "github.com/go-gost/x/logger"
	"github.com/go-gost/x/metadata"
	"github.com/go-gost/x/registry"
)

// This regression uses real loopback GOST relay endpoints and verifies the required-route policy.
// Every socket is bound to 127.0.0.1, no production config or external target.
func TestRequiredRelayRouteRejectsMissingHopAndRecovers(t *testing.T) {
	testRequiredRelayRecovery(t, false)
}

func TestRequiredAuthenticatedRelayRouteRejectsMissingHopAndRecovers(t *testing.T) {
	testRequiredRelayRecovery(t, true)
}

func testRequiredRelayRecovery(t *testing.T, authenticated bool) {
	t.Helper()
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
		name  string
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
		if chainName != "" {
			router.WithRequiredChain()
		}
		opts := []handler.Option{handler.RouterOption(router), handler.LoggerOption(log), handler.ServiceOption(name)}
		if authenticated {
			au := xauth.NewAuthenticator(xauth.AuthsOption(map[string]string{name: "synthetic-local-password-for-" + name}))
			if closer, ok := au.(io.Closer); ok {
				t.Cleanup(func() { closer.Close() })
			}
			opts = append(opts, handler.AutherOption(au))
		}
		h := relayhandler.NewHandler(opts...)
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
		return relay{name, ln.Addr().String(), count}
	}
	replace := func(name string, next relay) {
		connectorConfig := &config.ConnectorConfig{Type: "relay", Metadata: map[string]any{"nodelay": true}}
		if authenticated {
			connectorConfig.Auth = &config.AuthConfig{Username: next.name, Password: "synthetic-local-password-for-" + next.name}
		}
		c, e := parser.ParseChain(&config.ChainConfig{Name: name, Hops: []*config.HopConfig{{Name: name + "-hop", Nodes: []*config.NodeConfig{{Name: name + "-next", Addr: next.addr, Connector: connectorConfig, Dialer: &config.DialerConfig{Type: "tcp"}}}}}}, log)
		if e != nil {
			t.Fatal(e)
		}
		if e = registry.ReplaceChain(name, c); e != nil {
			t.Fatal(e)
		}
	}
	for _, name := range []string{"strict-regression-A", "strict-regression-B", "strict-regression-C"} {
		name := name
		t.Cleanup(func() { registry.ChainRegistry().Unregister(name) })
	}
	d := start("D", "")
	c := start("C", "strict-regression-C")
	replace("strict-regression-C", d)
	b := start("B", "strict-regression-B")
	replace("strict-regression-B", d)
	a := start("A", "strict-regression-A")
	replace("strict-regression-A", b)
	request := func(wantSuccess bool) {
		raw, e := net.DialTimeout("tcp", a.addr, time.Second)
		if e != nil {
			t.Fatal(e)
		}
		defer raw.Close()
		raw.SetDeadline(time.Now().Add(3 * time.Second))
		opts := []connector.Option{connector.LoggerOption(log)}
		if authenticated {
			opts = append(opts, connector.AuthOption(url.UserPassword("A", "synthetic-local-password-for-A")))
		}
		client := relayconnector.NewConnector(opts...)
		if e := client.Init(metadata.NewMetadata(map[string]any{"nodelay": true})); e != nil {
			t.Fatal(e)
		}
		conn, e := client.Connect(ctx, raw, "tcp", target.Addr().String())
		if !wantSuccess {
			if e == nil {
				conn.Close()
				t.Fatal("relay connected through a missing required hop")
			}
			return
		}
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
	request(true)
	if b.count.Load() != 1 || c.count.Load() != 0 || d.count.Load() != 1 {
		t.Fatalf("initial B/C/D: %d/%d/%d", b.count.Load(), c.count.Load(), d.count.Load())
	}
	replace("strict-regression-B", c)
	request(true)
	if b.count.Load() != 2 || c.count.Load() != 1 || d.count.Load() != 2 {
		t.Fatalf("insert C B/C/D: %d/%d/%d", b.count.Load(), c.count.Load(), d.count.Load())
	}
	// Deterministic cleanup boundary: DeleteChains has completed, while the B
	// relay service is still alive (before a subsequent DeleteService command).
	registry.ChainRegistry().Unregister("strict-regression-B")
	request(false)
	if b.count.Load() != 3 || c.count.Load() != 1 || d.count.Load() != 2 {
		t.Fatalf("missing chain B/C/D: %d/%d/%d", b.count.Load(), c.count.Load(), d.count.Load())
	}
	t.Log("Required route refuses the missing B chain and recovers after replacement")
	replace("strict-regression-B", c)
	request(true)
	if b.count.Load() != 4 || c.count.Load() != 2 || d.count.Load() != 3 {
		t.Fatalf("restored B/C/D: %d/%d/%d", b.count.Load(), c.count.Load(), d.count.Load())
	}
}
