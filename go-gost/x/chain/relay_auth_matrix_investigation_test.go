package chain_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	corechain "github.com/go-gost/core/chain"
	"github.com/go-gost/core/connector"
	"github.com/go-gost/core/dialer"
	"github.com/go-gost/core/handler"
	"github.com/go-gost/core/listener"
	corelogger "github.com/go-gost/core/logger"
	xauth "github.com/go-gost/x/auth"
	xchain "github.com/go-gost/x/chain"
	relayconnector "github.com/go-gost/x/connector/relay"
	kcpdialer "github.com/go-gost/x/dialer/kcp"
	relayhandler "github.com/go-gost/x/handler/relay"
	netdialer "github.com/go-gost/x/internal/net/dialer"
	kcplistener "github.com/go-gost/x/listener/kcp"
	tcplistener "github.com/go-gost/x/listener/tcp"
	tlslistener "github.com/go-gost/x/listener/tls"
	xlogger "github.com/go-gost/x/logger"
	"github.com/go-gost/x/metadata"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// Local-only matrix: native GOST listeners, relay handler and connectors. The
// credential below is synthetic test data, never used outside loopback sockets.
func TestRelayAuthTransportMatrixInvestigation(t *testing.T) {
	log := xlogger.Nop()
	corelogger.SetDefault(log)
	certServer := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	cert := certServer.TLS.Certificates[0]
	leaf := certServer.Certificate()
	certServer.Close()
	roots := x509.NewCertPool()
	roots.AddCert(leaf)
	tcpTarget, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	defer tcpTarget.Close()
	go func() {
		for {
			c, e := tcpTarget.Accept()
			if e != nil {
				return
			}
			go func() { defer c.Close(); io.Copy(c, c) }()
		}
	}()
	udpTarget, e := net.ListenPacket("udp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	defer udpTarget.Close()
	go func() {
		buf := make([]byte, 1024)
		for {
			n, a, e := udpTarget.ReadFrom(buf)
			if e != nil {
				return
			}
			udpTarget.WriteTo(buf[:n], a)
		}
	}()
	for _, transport := range []string{"tcp", "tls", "kcp"} {
		for _, network := range []string{"tcp", "udp"} {
			for _, test := range []struct {
				name        string
				auth        bool
				credentials *url.Userinfo
				success     bool
			}{
				{"legacy-no-credentials", false, nil, true},
				{"legacy-valid-credentials", false, url.UserPassword("test-peer", "synthetic-test-secret"), true},
				{"secured-no-credentials", true, nil, false},
				{"secured-wrong-credentials", true, url.UserPassword("test-peer", "incorrect"), false},
				{"secured-valid-credentials", true, url.UserPassword("test-peer", "synthetic-test-secret"), true},
			} {
				t.Run(transport+"/"+network+"/"+test.name, func(t *testing.T) {
					opts := []listener.Option{listener.AddrOption("127.0.0.1:0"), listener.LoggerOption(log), listener.TLSConfigOption(&tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12})}
					var ln listener.Listener
					switch transport {
					case "tls":
						ln = tlslistener.NewListener(opts...)
					case "kcp":
						ln = kcplistener.NewListener(opts...)
					default:
						ln = tcplistener.NewListener(opts...)
					}
					md := metadata.NewMetadata(map[string]any{"kcp.tcp": false, "kcp.nocomp": true})
					if e := ln.Init(md); e != nil {
						t.Fatal(e)
					}
					defer ln.Close()
					hopts := []handler.Option{handler.RouterOption(xchain.NewRouter(corechain.LoggerRouterOption(log))), handler.LoggerOption(log), handler.ServiceOption("auth-investigation")}
					if test.auth {
						au := xauth.NewAuthenticator(xauth.AuthsOption(map[string]string{"test-peer": "synthetic-test-secret"}))
						if closer, ok := au.(io.Closer); ok {
							defer closer.Close()
						}
						hopts = append(hopts, handler.AutherOption(au))
					}
					h := relayhandler.NewHandler(hopts...)
					if e := h.Init(metadata.NewMetadata(map[string]any{"nodelay": true})); e != nil {
						t.Fatal(e)
					}
					if closer, ok := h.(io.Closer); ok {
						defer closer.Close()
					}
					handled := make(chan struct{})
					go func() {
						defer close(handled)
						c, e := ln.Accept()
						if e == nil {
							h.Handle(context.Background(), c)
						}
					}()
					var raw net.Conn
					var err error
					switch transport {
					case "tls":
						raw, err = tls.DialWithDialer(&net.Dialer{Timeout: time.Second}, "tcp", ln.Addr().String(), &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12})
					case "kcp":
						d := kcpdialer.NewDialer(dialer.LoggerOption(log))
						if e := d.Init(md); e != nil {
							t.Fatal(e)
						}
						if closer, ok := d.(io.Closer); ok {
							defer closer.Close()
						}
						raw, err = d.Dial(context.Background(), ln.Addr().String(), dialer.NetDialerDialOption(&netdialer.Dialer{Logger: log}))
					default:
						raw, err = net.DialTimeout("tcp", ln.Addr().String(), time.Second)
					}
					if err != nil {
						t.Fatal(err)
					}
					defer raw.Close()
					raw.SetDeadline(time.Now().Add(5 * time.Second))
					copts := []connector.Option{connector.LoggerOption(log)}
					if test.credentials != nil {
						copts = append(copts, connector.AuthOption(test.credentials))
					}
					client := relayconnector.NewConnector(copts...)
					if e := client.Init(metadata.NewMetadata(map[string]any{"nodelay": true})); e != nil {
						t.Fatal(e)
					}
					target := tcpTarget.Addr().String()
					if network == "udp" {
						target = udpTarget.LocalAddr().String()
					}
					conn, err := client.Connect(context.Background(), raw, network, target)
					if !test.success {
						if err == nil {
							t.Fatal("unauthorized client was accepted")
						}
						if !strings.Contains(strings.ToLower(err.Error()), "unauthorized") {
							t.Fatalf("expected authentication refusal, got %v", err)
						}
						raw.Close()
						<-handled
						return
					}
					if err != nil {
						t.Fatal(err)
					}
					if _, err = conn.Write([]byte("probe")); err != nil {
						t.Fatal(err)
					}
					b := make([]byte, 64)
					n, err := conn.Read(b)
					if err != nil {
						t.Fatal(err)
					}
					if string(b[:n]) != "probe" {
						t.Fatalf("unexpected response %q", b[:n])
					}
					raw.Close()
					select {
					case <-handled:
					case <-time.After(5 * time.Second):
						t.Fatal("handler did not close")
					}
				})
			}
		}
	}
}
