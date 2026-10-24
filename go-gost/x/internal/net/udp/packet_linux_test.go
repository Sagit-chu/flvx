package udp

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"os"
	"testing"
	"time"

	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
	"golang.org/x/sys/unix"

	xlogger "github.com/go-gost/x/logger"
)

func TestMulticastDestinationRepliesFromUnicast(t *testing.T) {
	rxName, txName := os.Getenv("FLUX_TEST_MULTICAST_RX"), os.Getenv("FLUX_TEST_MULTICAST_TX")
	if rxName == "" || txName == "" {
		t.Skip("requires isolated multicast veth interfaces")
	}
	rx, err := net.InterfaceByName(rxName)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := net.InterfaceByName(txName)
	if err != nil {
		t.Fatal(err)
	}
	group := net.ParseIP("ff02::114")
	socket, err := net.ListenMulticastUDP("udp6", rx, &net.UDPAddr{IP: group})
	if err != nil {
		t.Fatal(err)
	}
	defer socket.Close()
	pc, err := PreserveDestination(socket)
	if err != nil {
		t.Fatal(err)
	}
	ln := NewListener(pc, &ListenConfig{
		Backlog: 1, ReadQueueSize: 1, ReadBufferSize: 128, TTL: time.Second, Keepalive: true,
		Logger: xlogger.NewLogger(xlogger.OutputOption(io.Discard)),
	})
	defer ln.Close()
	client, err := net.ListenUDP("udp6", &net.UDPAddr{IP: net.ParseIP("fe80::1"), Zone: txName})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if err := ipv6.NewPacketConn(client).SetMulticastInterface(tx); err != nil {
		t.Fatal(err)
	}
	client.SetDeadline(time.Now().Add(3 * time.Second))
	port := socket.LocalAddr().(*net.UDPAddr).Port
	serverResult := make(chan error, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			serverResult <- err
			return
		}
		defer c.Close()
		local, peer := c.LocalAddr().(*net.UDPAddr), c.RemoteAddr().(*net.UDPAddr)
		if !local.IP.Equal(group) || local.Port != port || local.Zone != rxName || !peer.IP.Equal(net.ParseIP("fe80::1")) ||
			peer.Port != client.LocalAddr().(*net.UDPAddr).Port || peer.Zone != rxName {
			serverResult <- fmt.Errorf("unexpected multicast session: local=%s peer=%s", local, peer)
			return
		}
		var buf [128]byte
		n, err := c.Read(buf[:])
		if err == nil {
			_, err = c.Write(buf[:n])
		}
		serverResult <- err
	}()
	if _, err := client.WriteToUDP([]byte("multicast"), &net.UDPAddr{IP: group, Port: port, Zone: txName}); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-serverResult:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("multicast session timed out")
	}
	var buf [128]byte
	n, from, err := client.ReadFromUDP(buf[:])
	if err != nil {
		t.Fatal(err)
	}
	if !from.IP.Equal(net.ParseIP("fe80::2")) || from.Port != port || from.Zone != txName || string(buf[:n]) != "multicast" {
		t.Fatalf("unexpected multicast reply %q from %s", buf[:n], from)
	}
}

func TestLinkLocalMulticastSessionsRetainZones(t *testing.T) {
	testScopedMulticastSessions(t, "ff02::114", false)
}

func TestIPv4DestinationSessionsRetainInterfaces(t *testing.T) {
	ifNames := []string{os.Getenv("FLUX_TEST_MULTICAST_TX"), os.Getenv("FLUX_TEST_MULTICAST_TX2")}
	if ifNames[0] == "" || ifNames[1] == "" {
		t.Skip("requires two isolated IPv4 multicast interfaces")
	}
	for _, network := range []string{"udp4", "udp"} {
		for _, destination := range []string{"239.255.0.114", "224.0.0.114", "255.255.255.255"} {
			t.Run(network+"/"+destination, func(t *testing.T) {
				bind := net.IPv4zero
				if network == "udp" {
					bind = net.IPv6unspecified
				}
				server, err := net.ListenUDP(network, &net.UDPAddr{IP: bind})
				if err != nil {
					t.Fatal(err)
				}
				defer server.Close()
				pc, err := PreserveDestination(server)
				if err != nil {
					t.Fatal(err)
				}
				client, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP("198.18.0.1")})
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
				target := &net.UDPAddr{IP: net.ParseIP(destination), Port: server.LocalAddr().(*net.UDPAddr).Port}
				for _, name := range ifNames {
					iface, err := net.InterfaceByName(name)
					if err != nil {
						t.Fatal(err)
					}
					if target.IP.IsMulticast() {
						if err := ipv4.NewPacketConn(server).JoinGroup(iface, target); err != nil {
							t.Fatal(err)
						}
					}
					if _, err := ipv4.NewPacketConn(client).WriteTo([]byte(name), &ipv4.ControlMessage{
						Src: client.LocalAddr().(*net.UDPAddr).IP, IfIndex: iface.Index,
					}, target); err != nil {
						t.Fatal(err)
					}
				}
				server.SetReadDeadline(time.Now().Add(3 * time.Second))
				ln := NewListener(pc, &ListenConfig{
					Backlog: 2, ReadQueueSize: 2, ReadBufferSize: 128, TTL: 10 * time.Second, Keepalive: true,
					Logger: xlogger.NewLogger(xlogger.OutputOption(io.Discard)),
				})
				defer ln.Close()
				seen := make(map[string]bool)
				for range ifNames {
					c, err := ln.Accept()
					if err != nil {
						t.Fatalf("interface did not get a separate session: %v", err)
					}
					defer c.Close()
					var buf [128]byte
					n, err := c.Read(buf[:])
					if err != nil {
						t.Fatal(err)
					}
					name := string(buf[:n])
					if (name != ifNames[0] && name != ifNames[1]) || seen[name] ||
						c.LocalAddr().String() != target.String() || c.RemoteAddr().String() != client.LocalAddr().String() {
						t.Fatalf("unexpected interface session: data=%q local=%s peer=%s", buf[:n], c.LocalAddr(), c.RemoteAddr())
					}
					iface, err := net.InterfaceByName(name)
					if err != nil {
						t.Fatal(err)
					}
					messages, err := unix.ParseSocketControlMessage(c.(*conn).packet.control)
					if err != nil || len(messages) != 1 {
						t.Fatalf("invalid reply control: %v", err)
					}
					var info unix.Inet4Pktinfo
					if _, err := binary.Decode(messages[0].Data, binary.NativeEndian, &info); err != nil {
						t.Fatal(err)
					}
					if int(info.Ifindex) != iface.Index {
						t.Fatalf("reply interface = %d, want %d", info.Ifindex, iface.Index)
					}
					seen[name] = true
				}
			})
		}
	}
}

func TestInterfaceLocalMulticastSessionsRetainZones(t *testing.T) {
	testScopedMulticastSessions(t, "ff01::114", true)
}

func testScopedMulticastSessions(t *testing.T, destination string, interfaceLocal bool) {
	t.Helper()
	rxNames := []string{os.Getenv("FLUX_TEST_MULTICAST_RX"), os.Getenv("FLUX_TEST_MULTICAST_RX2")}
	txNames := []string{os.Getenv("FLUX_TEST_MULTICAST_TX"), os.Getenv("FLUX_TEST_MULTICAST_TX2")}
	for i := range rxNames {
		if rxNames[i] == "" || txNames[i] == "" {
			t.Skip("requires two isolated multicast veth pairs")
		}
	}
	if interfaceLocal {
		// Interface-local multicast is looped back on the sending interface.
		rxNames = txNames
	}
	group := net.ParseIP(destination)
	server, err := net.ListenUDP("udp6", &net.UDPAddr{IP: net.IPv6unspecified})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	clientIP := net.ParseIP("fd00:1::1")
	client, err := net.ListenUDP("udp6", &net.UDPAddr{IP: clientIP})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	pc, err := PreserveDestination(server)
	if err != nil {
		t.Fatal(err)
	}
	for i, rxName := range rxNames {
		rx, err := net.InterfaceByName(rxName)
		if err != nil {
			t.Fatal(err)
		}
		if err := ipv6.NewPacketConn(server).JoinGroup(rx, &net.UDPAddr{IP: group}); err != nil {
			t.Fatal(err)
		}
		tx, err := net.InterfaceByName(txNames[i])
		if err != nil {
			t.Fatal(err)
		}
		if err := ipv6.NewPacketConn(client).SetMulticastInterface(tx); err != nil {
			t.Fatal(err)
		}
		if _, err := client.WriteToUDP([]byte(rxName), &net.UDPAddr{
			IP: group, Port: server.LocalAddr().(*net.UDPAddr).Port, Zone: txNames[i],
		}); err != nil {
			t.Fatal(err)
		}
	}
	server.SetReadDeadline(time.Now().Add(3 * time.Second))
	ln := NewListener(pc, &ListenConfig{
		Backlog: 2, ReadQueueSize: 2, ReadBufferSize: 128, TTL: 10 * time.Second, Keepalive: true,
		Logger: xlogger.NewLogger(xlogger.OutputOption(io.Discard)),
	})
	defer ln.Close()
	first, err := ln.Accept()
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := ln.Accept()
	if err != nil {
		t.Fatalf("second interface did not get a separate session: %v", err)
	}
	defer second.Close()
	if first == second {
		t.Fatal("interface-scoped multicast sessions were shared")
	}
	seen := make(map[string]bool)
	for _, c := range []net.Conn{first, second} {
		var buf [128]byte
		n, err := c.Read(buf[:])
		if err != nil {
			t.Fatal(err)
		}
		zone := string(buf[:n])
		local, peer := c.LocalAddr().(*net.UDPAddr), c.RemoteAddr().(*net.UDPAddr)
		if (zone != rxNames[0] && zone != rxNames[1]) || seen[zone] || !local.IP.Equal(group) ||
			local.Port != server.LocalAddr().(*net.UDPAddr).Port || local.Zone != zone ||
			!peer.IP.Equal(clientIP) || peer.Port != client.LocalAddr().(*net.UDPAddr).Port || peer.Zone != "" {
			t.Fatalf("unexpected scoped session: data=%q local=%s peer=%s", buf[:n], local, peer)
		}
		var reply ipv6.ControlMessage
		if err := reply.Parse(c.(*conn).packet.control); err != nil {
			t.Fatal(err)
		}
		iface, err := net.InterfaceByName(zone)
		if err != nil {
			t.Fatal(err)
		}
		if reply.IfIndex != iface.Index {
			t.Fatalf("reply interface = %d, want %d", reply.IfIndex, iface.Index)
		}
		seen[zone] = true
	}
}
