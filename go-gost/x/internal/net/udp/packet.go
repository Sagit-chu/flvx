package udp

import "net"

// packetAddr carries the received destination through the PacketConn wrappers.
// Its public address remains the real peer for admission, metrics and limits.
type packetAddr struct {
	net.Addr
	local   *net.UDPAddr
	control []byte
}
