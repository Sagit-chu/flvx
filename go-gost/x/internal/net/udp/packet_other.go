//go:build !linux

package udp

import "net"

func PreserveDestination(c *net.UDPConn) (net.PacketConn, error) {
	return c, nil
}
