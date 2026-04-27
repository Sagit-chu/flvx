package local

import (
	"bufio"
	"context"
	"net"
	"testing"
	"time"

	"github.com/go-gost/core/chain"
	"github.com/go-gost/core/handler"
	"github.com/go-gost/core/hop"
	xlogger "github.com/go-gost/x/logger"
	xmd "github.com/go-gost/x/metadata"
	proxyproto "github.com/pires/go-proxyproto"
)

type proxyProtocolTestHop struct {
	node *chain.Node
}

func (h proxyProtocolTestHop) Select(context.Context, ...hop.SelectOption) *chain.Node {
	return h.node
}

func (h proxyProtocolTestHop) Nodes() []*chain.Node {
	return []*chain.Node{h.node}
}

type proxyProtocolTestRouter struct{}

func (r proxyProtocolTestRouter) Options() *chain.RouterOptions {
	return &chain.RouterOptions{}
}

func (r proxyProtocolTestRouter) Dial(ctx context.Context, network, address string) (net.Conn, error) {
	var d net.Dialer
	return d.DialContext(ctx, network, address)
}

func (r proxyProtocolTestRouter) Bind(context.Context, string, string, ...chain.BindOption) (net.Listener, error) {
	return nil, net.ErrClosed
}

func TestLocalForwardHandlerSendsProxyProtocolToTarget(t *testing.T) {
	targetListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen target: %v", err)
	}
	defer targetListener.Close()

	entryListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen entry: %v", err)
	}
	defer entryListener.Close()

	h := NewHandler(
		handler.RouterOption(proxyProtocolTestRouter{}),
		handler.LoggerOption(xlogger.Nop()),
	)
	forwarder := h.(handler.Forwarder)
	forwarder.Forward(proxyProtocolTestHop{node: chain.NewNode("target", targetListener.Addr().String())})
	if err := h.Init(xmd.NewMetadata(map[string]any{"proxyProtocol": 2})); err != nil {
		t.Fatalf("init handler: %v", err)
	}

	handleErr := make(chan error, 1)
	acceptErr := make(chan error, 1)
	go func() {
		serverConn, err := entryListener.Accept()
		if err != nil {
			acceptErr <- err
			return
		}
		handleErr <- h.Handle(context.Background(), serverConn)
	}()

	clientConn, err := net.Dial("tcp", entryListener.Addr().String())
	if err != nil {
		t.Fatalf("dial entry: %v", err)
	}
	defer clientConn.Close()

	targetConn, err := targetListener.Accept()
	if err != nil {
		t.Fatalf("accept target: %v", err)
	}
	defer targetConn.Close()
	if err := targetConn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set target deadline: %v", err)
	}

	header, err := proxyproto.Read(bufio.NewReader(targetConn))
	if err != nil {
		t.Fatalf("read proxy protocol header: %v", err)
	}
	if header.Version != 2 {
		t.Fatalf("expected proxy protocol v2, got v%d", header.Version)
	}

	_ = clientConn.Close()
	_ = targetConn.Close()
	select {
	case err := <-acceptErr:
		t.Fatalf("accept entry: %v", err)
	case <-handleErr:
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not return after closing connections")
	}
}
