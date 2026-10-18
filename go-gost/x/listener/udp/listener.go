package udp

import (
	"net"
	"sync"
	"time"

	"github.com/go-gost/core/limiter"
	conn_limiter "github.com/go-gost/core/limiter/conn"
	"github.com/go-gost/core/listener"
	"github.com/go-gost/core/logger"
	md "github.com/go-gost/core/metadata"
	admission "github.com/go-gost/x/admission/wrapper"
	xnet "github.com/go-gost/x/internal/net"
	"github.com/go-gost/x/internal/net/udp"
	traffic_limiter "github.com/go-gost/x/limiter/traffic"
	limiter_wrapper "github.com/go-gost/x/limiter/traffic/wrapper"
	metrics "github.com/go-gost/x/metrics/wrapper"
	stats "github.com/go-gost/x/observer/stats/wrapper"
	"github.com/go-gost/x/registry"
)

func init() {
	registry.ListenerRegistry().Register("udp", NewListener)
}

type udpListener struct {
	ln      net.Listener
	logger  logger.Logger
	md      metadata
	options listener.Options
}

func NewListener(opts ...listener.Option) listener.Listener {
	options := listener.Options{}
	for _, opt := range opts {
		opt(&options)
	}
	return &udpListener{
		logger:  options.Logger,
		options: options,
	}
}

func (l *udpListener) Init(md md.Metadata) (err error) {
	if err = l.parseMetadata(md); err != nil {
		return
	}

	network := "udp"
	if xnet.IsIPv4(l.options.Addr) {
		network = "udp4"
	}
	laddr, err := net.ResolveUDPAddr(network, l.options.Addr)
	if err != nil {
		return
	}

	socket, err := net.ListenUDP(network, laddr)
	if err != nil {
		return
	}
	conn, err := udp.PreserveDestination(socket)
	if err != nil {
		socket.Close()
		return err
	}
	conn = metrics.WrapPacketConn(l.options.Service, conn)
	conn = stats.WrapPacketConn(conn, l.options.Stats)
	conn = admission.WrapPacketConn(l.options.Admission, conn)
	conn = limiter_wrapper.WrapPacketConn(
		conn,
		l.options.TrafficLimiter,
		traffic_limiter.ServiceLimitKey,
		limiter.ScopeOption(limiter.ScopeService),
		limiter.ServiceOption(l.options.Service),
		limiter.NetworkOption(conn.LocalAddr().Network()),
	)

	ln := udp.NewListener(conn, &udp.ListenConfig{
		Backlog:        l.md.backlog,
		ReadQueueSize:  l.md.readQueueSize,
		ReadBufferSize: l.md.readBufferSize,
		Keepalive:      l.md.keepalive,
		TTL:            l.md.ttl,
		Logger:         l.logger,
	})
	l.ln = ln
	return
}

func (l *udpListener) Accept() (conn net.Conn, err error) {
	conn, err = l.ln.Accept()
	if err != nil {
		return
	}

	if l.options.ConnLimiter != nil {
		host, _, _ := net.SplitHostPort(conn.RemoteAddr().String())
		if lim := l.options.ConnLimiter.Limiter(host); lim != nil {
			if !lim.Allow(1) {
				_ = conn.Close()
				return newClosedConn(conn), nil
			}
			conn = wrapConnLimiter(lim, conn)
		}
	}

	if pc, ok := conn.(net.PacketConn); ok {
		conn = limiter_wrapper.WrapUDPConn(
			pc,
			l.options.TrafficLimiter,
			conn.RemoteAddr().String(),
			limiter.ScopeOption(limiter.ScopeConn),
			limiter.ServiceOption(l.options.Service),
			limiter.NetworkOption(conn.LocalAddr().Network()),
			limiter.SrcOption(conn.RemoteAddr().String()),
		)
	}
	return
}

type connLimiterConn struct {
	net.Conn
	net.PacketConn
	limiter conn_limiter.Limiter
	once    sync.Once
}

func wrapConnLimiter(limiter conn_limiter.Limiter, conn net.Conn) net.Conn {
	pc, ok := conn.(net.PacketConn)
	if !ok {
		return conn
	}
	return &connLimiterConn{
		Conn:       conn,
		PacketConn: pc,
		limiter:    limiter,
	}
}

func (c *connLimiterConn) Close() (err error) {
	c.once.Do(func() {
		c.limiter.Allow(-1)
		err = c.Conn.Close()
	})
	return
}

func (c *connLimiterConn) LocalAddr() net.Addr {
	return c.Conn.LocalAddr()
}

func (c *connLimiterConn) SetDeadline(t time.Time) error {
	return c.Conn.SetDeadline(t)
}

func (c *connLimiterConn) SetReadDeadline(t time.Time) error {
	return c.Conn.SetReadDeadline(t)
}

func (c *connLimiterConn) SetWriteDeadline(t time.Time) error {
	return c.Conn.SetWriteDeadline(t)
}

type closedConn struct {
	net.Conn
	net.PacketConn
}

func newClosedConn(conn net.Conn) net.Conn {
	pc, _ := conn.(net.PacketConn)
	return closedConn{Conn: conn, PacketConn: pc}
}

func (c closedConn) Read([]byte) (int, error) {
	return 0, net.ErrClosed
}

func (c closedConn) Write([]byte) (int, error) {
	return 0, net.ErrClosed
}

func (c closedConn) ReadFrom([]byte) (int, net.Addr, error) {
	return 0, nil, net.ErrClosed
}

func (c closedConn) WriteTo([]byte, net.Addr) (int, error) {
	return 0, net.ErrClosed
}

func (c closedConn) Close() error {
	return c.Conn.Close()
}

func (c closedConn) LocalAddr() net.Addr {
	return c.Conn.LocalAddr()
}

func (c closedConn) SetDeadline(t time.Time) error {
	return c.Conn.SetDeadline(t)
}

func (c closedConn) SetReadDeadline(t time.Time) error {
	return c.Conn.SetReadDeadline(t)
}

func (c closedConn) SetWriteDeadline(t time.Time) error {
	return c.Conn.SetWriteDeadline(t)
}

func (l *udpListener) Addr() net.Addr {
	return l.ln.Addr()
}

func (l *udpListener) Close() error {
	return l.ln.Close()
}
