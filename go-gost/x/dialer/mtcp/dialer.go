package mtcp

import (
	"context"
	"errors"
	"net"
	"sync"
	"time"

	"github.com/go-gost/core/dialer"
	"github.com/go-gost/core/logger"
	md "github.com/go-gost/core/metadata"
	"github.com/go-gost/x/internal/util/mux"
	"github.com/go-gost/x/internal/util/sessionretire"
	"github.com/go-gost/x/registry"
)

func init() {
	registry.DialerRegistry().Register("mtcp", NewDialer)
}

type mtcpDialer struct {
	sessions     map[string]*muxSession
	sessionMutex sync.Mutex
	retired      bool
	logger       logger.Logger
	md           metadata
	options      dialer.Options
}

func NewDialer(opts ...dialer.Option) dialer.Dialer {
	options := dialer.Options{}
	for _, opt := range opts {
		opt(&options)
	}

	return &mtcpDialer{
		sessions: make(map[string]*muxSession),
		logger:   options.Logger,
		options:  options,
	}
}

func (d *mtcpDialer) Init(md md.Metadata) (err error) {
	if err = d.parseMetadata(md); err != nil {
		return
	}

	return nil
}

// Multiplex implements dialer.Multiplexer interface.
func (d *mtcpDialer) Multiplex() bool {
	return true
}

func (d *mtcpDialer) Dial(ctx context.Context, addr string, opts ...dialer.DialOption) (conn net.Conn, err error) {
	d.sessionMutex.Lock()
	defer d.sessionMutex.Unlock()
	if d.retired {
		return nil, net.ErrClosed
	}

	session, ok := d.sessions[addr]
	if session != nil && session.IsClosed() {
		delete(d.sessions, addr) // session is dead
		ok = false
	}
	if !ok {
		var options dialer.DialOptions
		for _, opt := range opts {
			opt(&options)
		}

		conn, err = options.Dialer.Dial(ctx, "tcp", addr)
		if err != nil {
			return
		}

		session = &muxSession{conn: conn}
		d.sessions[addr] = session
	}

	return session.conn, err
}

// Handshake implements dialer.Handshaker
func (d *mtcpDialer) Handshake(ctx context.Context, conn net.Conn, options ...dialer.HandshakeOption) (net.Conn, error) {
	opts := &dialer.HandshakeOptions{}
	for _, option := range options {
		option(opts)
	}

	d.sessionMutex.Lock()
	defer d.sessionMutex.Unlock()
	if d.retired {
		conn.Close()
		return nil, net.ErrClosed
	}

	if d.md.handshakeTimeout > 0 {
		conn.SetDeadline(time.Now().Add(d.md.handshakeTimeout))
		defer conn.SetDeadline(time.Time{})
	}

	session, ok := d.sessions[opts.Addr]
	if session != nil && session.conn != conn {
		conn.Close()
		return nil, errors.New("mtls: unrecognized connection")
	}

	if !ok || session.session == nil {
		s, err := d.initSession(ctx, conn)
		if err != nil {
			d.logger.Error(err)
			conn.Close()
			delete(d.sessions, opts.Addr)
			return nil, err
		}
		session = s
		d.sessions[opts.Addr] = session
	}
	cc, err := session.GetConn()
	if err != nil {
		session.Close()
		delete(d.sessions, opts.Addr)
		return nil, err
	}

	return cc, nil
}

func (d *mtcpDialer) initSession(ctx context.Context, conn net.Conn) (*muxSession, error) {
	// stream multiplex
	session, err := mux.ClientSession(conn, d.md.muxCfg)
	if err != nil {
		return nil, err
	}
	return &muxSession{conn: conn, session: session}, nil
}

func (d *mtcpDialer) Retire() {
	for _, session := range d.detachSessions() {
		sessionretire.Gracefully(session)
	}
}

func (d *mtcpDialer) Close() error {
	var errs []error
	for _, session := range d.detachSessions() {
		errs = append(errs, session.Close())
	}
	return errors.Join(errs...)
}

func (d *mtcpDialer) detachSessions() []*muxSession {
	if d == nil {
		return nil
	}
	d.sessionMutex.Lock()
	d.retired = true
	sessions := make([]*muxSession, 0, len(d.sessions))
	for _, session := range d.sessions {
		if session != nil {
			sessions = append(sessions, session)
		}
	}
	d.sessions = make(map[string]*muxSession)
	d.sessionMutex.Unlock()
	return sessions
}
