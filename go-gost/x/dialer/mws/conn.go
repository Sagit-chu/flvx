package mws

import (
	"net"

	"github.com/go-gost/x/internal/util/mux"
)

type muxSession struct {
	conn    net.Conn
	session *mux.Session
}

func (session *muxSession) GetConn() (net.Conn, error) {
	return session.session.GetConn()
}

func (session *muxSession) Accept() (net.Conn, error) {
	return session.session.Accept()
}

func (session *muxSession) Close() error {
	if session == nil {
		return nil
	}
	if session.session == nil {
		if session.conn != nil {
			conn := session.conn
			session.conn = nil
			return conn.Close()
		}
		return nil
	}
	return session.session.Close()
}

func (session *muxSession) IsClosed() bool {
	if session == nil {
		return true
	}
	if session.session == nil {
		return true
	}
	return session.session.IsClosed()
}

func (session *muxSession) NumStreams() int {
	if session == nil || session.session == nil {
		return 0
	}
	return session.session.NumStreams()
}
