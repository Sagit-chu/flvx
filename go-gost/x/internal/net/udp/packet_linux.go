package udp

import (
	"encoding/binary"
	"errors"
	"net"

	"golang.org/x/net/ipv6"
	"golang.org/x/sys/unix"
)

// PreserveDestination pins wildcard UDP replies to the address they arrived on.
func PreserveDestination(c *net.UDPConn) (net.PacketConn, error) {
	local := c.LocalAddr().(*net.UDPAddr)
	if !local.IP.IsUnspecified() {
		return c, nil
	}
	raw, err := c.SyscallConn()
	if err != nil {
		return nil, err
	}
	var sockErr error
	if err = raw.Control(func(fd uintptr) {
		if local.IP.To4() != nil {
			sockErr = unix.SetsockoptInt(int(fd), unix.IPPROTO_IP, unix.IP_PKTINFO, 1)
			return
		}
		if sockErr = unix.SetsockoptInt(int(fd), unix.IPPROTO_IPV6, unix.IPV6_RECVPKTINFO, 1); sockErr != nil {
			return
		}
		var onlyV6 int
		onlyV6, sockErr = unix.GetsockoptInt(int(fd), unix.IPPROTO_IPV6, unix.IPV6_V6ONLY)
		if sockErr == nil && onlyV6 == 0 {
			sockErr = unix.SetsockoptInt(int(fd), unix.IPPROTO_IP, unix.IP_PKTINFO, 1)
		}
	}); err != nil {
		return nil, err
	}
	if sockErr != nil {
		return nil, sockErr
	}
	return &destinationConn{UDPConn: c}, nil
}

type destinationConn struct {
	*net.UDPConn
}

func (c *destinationConn) ReadFrom(b []byte) (int, net.Addr, error) {
	var oob [128]byte
	n, oobn, flags, remote, err := c.ReadMsgUDP(b, oob[:])
	if err != nil {
		return 0, remote, err
	}
	if flags&unix.MSG_CTRUNC != 0 {
		return 0, nil, errors.New("UDP destination control message truncated")
	}
	var dst net.IP
	var control []byte
	var ifIndex int
	if remote.IP.To4() != nil {
		messages, err := unix.ParseSocketControlMessage(oob[:oobn])
		if err != nil {
			return 0, nil, err
		}
		for _, message := range messages {
			if message.Header.Level != unix.IPPROTO_IP || message.Header.Type != unix.IP_PKTINFO {
				continue
			}
			var info unix.Inet4Pktinfo
			if _, err := binary.Decode(message.Data, binary.NativeEndian, &info); err != nil {
				return 0, nil, err
			}
			dst, ifIndex = net.IP(info.Addr[:]), int(info.Ifindex)
			// Spec_dst remains a local unicast address for broadcast destinations.
			control = unix.PktInfo4(&unix.Inet4Pktinfo{Ifindex: info.Ifindex, Spec_dst: info.Spec_dst})
			break
		}
	} else {
		var cm ipv6.ControlMessage
		if err = cm.Parse(oob[:oobn]); err != nil {
			return 0, nil, err
		}
		dst, ifIndex = cm.Dst, cm.IfIndex
		source := dst
		if dst.IsMulticast() {
			source = nil
		}
		control = (&ipv6.ControlMessage{Src: source, IfIndex: ifIndex}).Marshal()
	}
	if len(dst) == 0 || dst.IsUnspecified() {
		return 0, nil, errors.New("UDP destination address missing")
	}
	local := &net.UDPAddr{IP: append(net.IP(nil), dst...), Port: c.LocalAddr().(*net.UDPAddr).Port}
	if dst.IsLinkLocalUnicast() || dst.IsLinkLocalMulticast() {
		if iface, lookupErr := net.InterfaceByIndex(ifIndex); lookupErr == nil {
			local.Zone = iface.Name
		}
	}
	return n, &packetAddr{Addr: remote, local: local, control: control}, nil
}

func (c *destinationConn) WriteTo(b []byte, addr net.Addr) (int, error) {
	if packet, ok := addr.(*packetAddr); ok {
		remote, ok := packet.Addr.(*net.UDPAddr)
		if !ok {
			return 0, errors.New("invalid UDP peer address")
		}
		n, _, err := c.WriteMsgUDP(b, packet.control, remote)
		if n < 0 {
			n = 0
		}
		return n, err
	}
	return c.UDPConn.WriteTo(b, addr)
}
