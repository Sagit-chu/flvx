package chain_test

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-gost/core/connector"
	corelogger "github.com/go-gost/core/logger"
	"github.com/go-gost/x/config"
	"github.com/go-gost/x/config/parsing"
	chainparser "github.com/go-gost/x/config/parsing/chain"
	serviceparser "github.com/go-gost/x/config/parsing/service"
	relayconnector "github.com/go-gost/x/connector/relay"
	_ "github.com/go-gost/x/handler/relay"
	_ "github.com/go-gost/x/listener/tcp"
	xlogger "github.com/go-gost/x/logger"
	"github.com/go-gost/x/metadata"
	"github.com/go-gost/x/registry"
)

func TestRequiredRelayChainCannotDialTargetWhenMissing(t *testing.T) {
	assertRequiredRelayCannotDial(t, "missing-required-chain", "tcp")
}

func TestRequiredRelayChainRejectsEmptySelectionAndAbsentConfiguration(t *testing.T) {
	corelogger.SetDefault(xlogger.Nop())
	const name = "empty-required-chain"
	c, err := chainparser.ParseChain(&config.ChainConfig{Name: name, Hops: []*config.HopConfig{{Name: "empty-hop", Nodes: []*config.NodeConfig{}}}}, xlogger.Nop())
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.ReplaceChain(name, c); err != nil {
		t.Fatal(err)
	}
	defer registry.ChainRegistry().Unregister(name)
	for _, network := range []string{"tcp", "udp"} {
		for _, chainName := range []string{name, ""} {
			t.Run(network+"/"+chainName, func(t *testing.T) { assertRequiredRelayCannotDial(t, chainName, network) })
		}
	}
}

func assertRequiredRelayCannotDial(t *testing.T, chainName, network string) {
	t.Helper()
	corelogger.SetDefault(xlogger.Nop())
	previousTLS := parsing.DefaultTLSConfig()
	parsing.SetDefaultTLSConfig(&tls.Config{})
	defer parsing.SetDefaultTLSConfig(previousTLS)
	target, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	var accepted atomic.Int32
	go func() {
		for {
			c, e := target.Accept()
			if e != nil {
				return
			}
			accepted.Add(1)
			c.Close()
		}
	}()
	svc, err := serviceparser.ParseService(&config.ServiceConfig{Name: "required-route-regression", Addr: "127.0.0.1:0", Listener: &config.ListenerConfig{Type: "tcp"}, Handler: &config.HandlerConfig{Type: "relay", Chain: chainName, Metadata: map[string]any{"chain.strict": true, "nodelay": true}}})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { defer close(done); svc.Serve() }()
	defer func() { svc.Close(); <-done }()
	raw, err := net.DialTimeout("tcp", svc.Addr().String(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	raw.SetDeadline(time.Now().Add(3 * time.Second))
	client := relayconnector.NewConnector(connector.LoggerOption(xlogger.Nop()))
	if err := client.Init(metadata.NewMetadata(map[string]any{"nodelay": true})); err != nil {
		t.Fatal(err)
	}
	conn, err := client.Connect(context.Background(), raw, network, target.Addr().String())
	if err == nil {
		if conn != nil {
			conn.Close()
		}
		t.Fatal("required chain is missing, but relay accepted a direct connection")
	}
	if accepted.Load() != 0 {
		t.Fatalf("ultimate target received %d connections without the required chain", accepted.Load())
	}
	if closer, ok := client.(io.Closer); ok {
		closer.Close()
	}
}
