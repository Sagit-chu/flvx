package udp

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	corelimiter "github.com/go-gost/core/limiter"
	coretraffic "github.com/go-gost/core/limiter/traffic"
	corelistener "github.com/go-gost/core/listener"
	corestats "github.com/go-gost/core/observer/stats"
	xadmission "github.com/go-gost/x/admission"
	xtraffic "github.com/go-gost/x/limiter/traffic"
	xlogger "github.com/go-gost/x/logger"
	xmetadata "github.com/go-gost/x/metadata"
	"github.com/go-gost/x/metrics"
	xstats "github.com/go-gost/x/observer/stats"
)

type countedTraffic struct {
	coretraffic.TrafficLimiter
	in, out atomic.Int64
}

func (l *countedTraffic) In(ctx context.Context, key string, opts ...corelimiter.Option) coretraffic.Limiter {
	l.in.Add(1)
	return l.TrafficLimiter.In(ctx, key, opts...)
}

func (l *countedTraffic) Out(ctx context.Context, key string, opts ...corelimiter.Option) coretraffic.Limiter {
	l.out.Add(1)
	return l.TrafficLimiter.Out(ctx, key, opts...)
}

func TestDestinationPreservedThroughWrappers(t *testing.T) {
	v6 := strings.FieldsFunc(os.Getenv("FLUX_TEST_IPV6_ADDRS"), func(r rune) bool { return r == ',' })
	if len(v6) == 0 {
		v6 = []string{"::1"}
	}
	for _, tc := range []struct {
		name, bind string
		targets    []string
		clientBind string
	}{
		{"ipv4-wildcard", "0.0.0.0:0", []string{"127.0.0.1", "127.0.0.2"}, ""},
		{"dual-stack-wildcard", "[::]:0", append([]string{"127.0.0.1", "127.0.0.2"}, v6...), ""},
		{"fixed-ipv6-source", "[::]:0", v6, "::1"},
		{"explicit-ipv4", "127.0.0.1:0", []string{"127.0.0.1"}, ""},
		{"explicit-ipv6", "[::1]:0", []string{"::1"}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			metrics.Enable(true)
			defer metrics.Enable(false)
			log := xlogger.NewLogger(xlogger.OutputOption(io.Discard))
			stats := xstats.NewStats(false)
			lim := &countedTraffic{TrafficLimiter: xtraffic.NewTrafficLimiter(
				xtraffic.LimitsOption("$ 1MB 1MB", "$$ 1MB 1MB"), xtraffic.LoggerOption(log),
			)}
			ln := NewListener(corelistener.AddrOption(tc.bind), corelistener.LoggerOption(log),
				corelistener.ServiceOption(t.Name()), corelistener.StatsOption(stats),
				corelistener.TrafficLimiterOption(lim),
				corelistener.AdmissionOption(xadmission.NewAdmission(
					xadmission.WhitelistOption(true),
					xadmission.MatchersOption([]string{"127.0.0.0/8", "::1", "fd00::/8"}),
					xadmission.LoggerOption(log),
				)),
			)
			if err := ln.Init(xmetadata.NewMetadata(map[string]any{"keepAlive": true})); err != nil {
				t.Fatal(err)
			}
			defer ln.Close()
			clientIP := net.IPv6unspecified
			if tc.clientBind != "" {
				clientIP = net.ParseIP(tc.clientBind)
			}
			client, err := net.ListenUDP("udp", &net.UDPAddr{IP: clientIP})
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			client.SetDeadline(time.Now().Add(5 * time.Second))
			clientAddr := client.LocalAddr().(*net.UDPAddr)
			var accepted atomic.Int64
			errors := make(chan error, 128)
			go func() {
				for {
					c, err := ln.Accept()
					if err != nil {
						return
					}
					accepted.Add(1)
					go func() {
						defer c.Close()
						buf := make([]byte, 128)
						for {
							n, err := c.Read(buf)
							if err != nil {
								return
							}
							var round, target int
							if _, err := fmt.Sscanf(string(buf[:n]), "%d/%d", &round, &target); err != nil {
								errors <- err
								return
							}
							local := c.LocalAddr().(*net.UDPAddr)
							if !local.IP.Equal(net.ParseIP(tc.targets[target])) {
								errors <- fmt.Errorf("local destination = %s, want %s", local.IP, tc.targets[target])
								return
							}
							peer, ok := c.RemoteAddr().(*net.UDPAddr)
							if !ok {
								errors <- fmt.Errorf("peer type changed: %T", c.RemoteAddr())
								return
							}
							if tc.clientBind != "" && (!peer.IP.Equal(clientAddr.IP) || peer.Port != clientAddr.Port) {
								errors <- fmt.Errorf("peer endpoint = %s, want %s", peer, clientAddr)
								return
							}
							if _, err := c.Write(buf[:n]); err != nil {
								errors <- err
								return
							}
						}
					}()
				}
			}()
			port := ln.Addr().(*net.UDPAddr).Port
			var bytes int
			for round := 0; round < 3; round++ {
				for target, ip := range tc.targets {
					payload := []byte(fmt.Sprintf("%d/%d", round, target))
					bytes += len(payload)
					if _, err := client.WriteToUDP(payload, &net.UDPAddr{IP: net.ParseIP(ip), Port: port}); err != nil {
						t.Fatal(err)
					}
				}
				seen := make(map[int]bool)
				for range tc.targets {
					buf := make([]byte, 128)
					n, from, err := client.ReadFromUDP(buf)
					if err != nil {
						select {
						case serverErr := <-errors:
							t.Fatal(serverErr)
						default:
							t.Fatal(err)
						}
					}
					var gotRound, target int
					if _, err := fmt.Sscanf(string(buf[:n]), "%d/%d", &gotRound, &target); err != nil {
						t.Fatal(err)
					}
					if gotRound != round || seen[target] || from.Port != port || !from.IP.Equal(net.ParseIP(tc.targets[target])) {
						t.Fatalf("unexpected reply %q from %s", buf[:n], from)
					}
					seen[target] = true
				}
			}
			if accepted.Load() != int64(len(tc.targets)) {
				t.Fatalf("accepted %d sessions, want %d", accepted.Load(), len(tc.targets))
			}
			deadline := time.Now().Add(time.Second)
			for stats.Get(corestats.KindOutputBytes) < uint64(bytes) && time.Now().Before(deadline) {
				time.Sleep(time.Millisecond)
			}
			if stats.Get(corestats.KindInputBytes) != uint64(bytes) || stats.Get(corestats.KindOutputBytes) != uint64(bytes) {
				t.Fatalf("traffic stats lost: in=%d out=%d want=%d", stats.Get(corestats.KindInputBytes), stats.Get(corestats.KindOutputBytes), bytes)
			}
			if lim.in.Load() == 0 || lim.out.Load() == 0 {
				t.Fatal("traffic limiter bypassed")
			}
		})
	}
}

func TestBroadcastDestinationRepliesFromUnicast(t *testing.T) {
	for _, bind := range []string{"0.0.0.0:0", "[::]:0"} {
		t.Run(bind, func(t *testing.T) {
			ln := NewListener(corelistener.AddrOption(bind),
				corelistener.LoggerOption(xlogger.NewLogger(xlogger.OutputOption(io.Discard))))
			if err := ln.Init(xmetadata.NewMetadata(map[string]any{"keepAlive": true})); err != nil {
				t.Fatal(err)
			}
			defer ln.Close()
			client, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			raw, err := client.SyscallConn()
			if err != nil {
				t.Fatal(err)
			}
			var sockErr error
			if err := raw.Control(func(fd uintptr) {
				sockErr = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_BROADCAST, 1)
			}); err != nil {
				t.Fatal(err)
			}
			if sockErr != nil {
				t.Fatal(sockErr)
			}
			client.SetDeadline(time.Now().Add(5 * time.Second))
			target := &net.UDPAddr{IP: net.IPv4(127, 255, 255, 255), Port: ln.Addr().(*net.UDPAddr).Port}
			if _, err := client.WriteToUDP([]byte("broadcast"), target); err != nil {
				t.Fatal(err)
			}
			conn, err := acceptWithTimeout(t, ln, time.Second)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			if conn.LocalAddr().String() != target.String() {
				t.Fatalf("local destination = %s, want %s", conn.LocalAddr(), target)
			}
			if conn.RemoteAddr().String() != client.LocalAddr().String() {
				t.Fatalf("peer endpoint = %s, want %s", conn.RemoteAddr(), client.LocalAddr())
			}
			var buf [128]byte
			n, err := conn.Read(buf[:])
			if err != nil {
				t.Fatal(err)
			}
			if _, err := conn.Write(buf[:n]); err != nil {
				t.Fatalf("reply to broadcast request: %v", err)
			}
			n, from, err := client.ReadFromUDP(buf[:])
			if err != nil {
				t.Fatal(err)
			}
			if !from.IP.Equal(net.IPv4(127, 0, 0, 1)) || from.Port != target.Port || string(buf[:n]) != "broadcast" {
				t.Fatalf("unexpected broadcast reply %q from %s", buf[:n], from)
			}
		})
	}
}

func TestDestinationSessionExpires(t *testing.T) {
	ln := NewListener(corelistener.AddrOption("0.0.0.0:0"),
		corelistener.LoggerOption(xlogger.NewLogger(xlogger.OutputOption(io.Discard))))
	if err := ln.Init(xmetadata.NewMetadata(map[string]any{"keepAlive": true, "ttl": "20ms"})); err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	client, err := net.DialUDP("udp4", nil, &net.UDPAddr{IP: net.IPv4(127, 0, 0, 2), Port: ln.Addr().(*net.UDPAddr).Port})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	client.Write([]byte("first"))
	first, err := acceptWithTimeout(t, ln, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	var b [16]byte
	first.Read(b[:])
	time.Sleep(80 * time.Millisecond)
	if _, err := first.Read(b[:]); err == nil {
		t.Fatal("idle session did not expire")
	}
	client.Write([]byte("second"))
	second, err := acceptWithTimeout(t, ln, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if first == second {
		t.Fatal("expired session reused")
	}
	if second.LocalAddr().(*net.UDPAddr).IP.String() != "127.0.0.2" {
		t.Fatalf("new session lost destination: %s", second.LocalAddr())
	}
}
