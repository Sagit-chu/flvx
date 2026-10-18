package udp

import (
	"fmt"
	"io"
	"net"
	"os"
	"testing"
	"time"

	"golang.org/x/net/ipv6"

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
		if !local.IP.Equal(group) || local.Port != port || !peer.IP.Equal(net.ParseIP("fe80::1")) ||
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
