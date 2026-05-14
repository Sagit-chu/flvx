package wt

import (
	"net"
	"time"

	wt "github.com/quic-go/webtransport-go"
)

type conn struct {
	session *wt.Session
	stream  *wt.Stream
}

func Conn(session *wt.Session, stream *wt.Stream) net.Conn {
	return &conn{
		session: session,
		stream:  stream,
	}
}

func (c *conn) Read(b []byte) (n int, err error) {
	return c.stream.Read(b)
}

func (c *conn) Write(b []byte) (n int, err error) {
	return c.stream.Write(b)
}

func (c *conn) Close() error {
	return c.stream.Close()
}

func (c *conn) LocalAddr() net.Addr {
	return c.session.LocalAddr()
}

func (c *conn) RemoteAddr() net.Addr {
	return c.session.RemoteAddr()
}

func (c *conn) SetDeadline(t time.Time) error {
	return c.stream.SetDeadline(t)
}

func (c *conn) SetReadDeadline(t time.Time) error {
	return c.stream.SetReadDeadline(t)
}

func (c *conn) SetWriteDeadline(t time.Time) error {
	return c.stream.SetWriteDeadline(t)
}
