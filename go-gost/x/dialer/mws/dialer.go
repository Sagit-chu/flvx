package mws

import (
	"context"
	"errors"
	"net"
	"net/url"
	"sync"
	"time"

	"github.com/go-gost/core/dialer"
	"github.com/go-gost/core/logger"
	md "github.com/go-gost/core/metadata"
	"github.com/go-gost/x/internal/util/mux"
	"github.com/go-gost/x/internal/util/sessionretire"
	ws_util "github.com/go-gost/x/internal/util/ws"
	"github.com/go-gost/x/registry"
	"github.com/gorilla/websocket"
)

func init() {
	registry.DialerRegistry().Register("mws", NewDialer)
	registry.DialerRegistry().Register("mwss", NewTLSDialer)
}

type mwsDialer struct {
	sessions     map[string]*muxSession
	sessionMutex sync.Mutex
	retired      bool
	tlsEnabled   bool
	md           metadata
	options      dialer.Options
}

func NewDialer(opts ...dialer.Option) dialer.Dialer {
	options := dialer.Options{}
	for _, opt := range opts {
		opt(&options)
	}

	return &mwsDialer{
		sessions: make(map[string]*muxSession),
		options:  options,
	}
}

func NewTLSDialer(opts ...dialer.Option) dialer.Dialer {
	options := dialer.Options{}
	for _, opt := range opts {
		opt(&options)
	}

	return &mwsDialer{
		tlsEnabled: true,
		sessions:   make(map[string]*muxSession),
		options:    options,
	}
}
func (d *mwsDialer) Init(md md.Metadata) (err error) {
	if err = d.parseMetadata(md); err != nil {
		return
	}

	return nil
}

// Multiplex implements dialer.Multiplexer interface.
func (d *mwsDialer) Multiplex() bool {
	return true
}

func (d *mwsDialer) Dial(ctx context.Context, addr string, opts ...dialer.DialOption) (conn net.Conn, err error) {
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
func (d *mwsDialer) Handshake(ctx context.Context, conn net.Conn, options ...dialer.HandshakeOption) (net.Conn, error) {
	opts := &dialer.HandshakeOptions{}
	for _, option := range options {
		option(opts)
	}

	log := d.options.Logger.WithFields(map[string]any{
		"local":  conn.LocalAddr().String(),
		"remote": conn.RemoteAddr().String(),
	})

	d.sessionMutex.Lock()
	defer d.sessionMutex.Unlock()
	if d.retired {
		conn.Close()
		return nil, net.ErrClosed
	}

	session, ok := d.sessions[opts.Addr]
	if session != nil && session.conn != conn {
		err := errors.New("mws: unrecognized connection")
		log.Error(err)
		conn.Close()
		return nil, err
	}

	if !ok || session.session == nil {
		host := d.md.host
		if host == "" {
			host = opts.Addr
		}
		s, err := d.initSession(ctx, host, conn, log)
		if err != nil {
			log.Error(err)
			conn.Close()
			delete(d.sessions, opts.Addr)
			return nil, err
		}
		session = s
		d.sessions[opts.Addr] = session
	}
	cc, err := session.GetConn()
	if err != nil {
		log.Error(err)
		session.Close()
		delete(d.sessions, opts.Addr)
		return nil, err
	}

	return cc, nil
}

func (d *mwsDialer) initSession(ctx context.Context, host string, conn net.Conn, log logger.Logger) (*muxSession, error) {
	dialer := websocket.Dialer{
		HandshakeTimeout:  d.md.handshakeTimeout,
		ReadBufferSize:    d.md.readBufferSize,
		WriteBufferSize:   d.md.writeBufferSize,
		EnableCompression: d.md.enableCompression,
		NetDial: func(net, addr string) (net.Conn, error) {
			return conn, nil
		},
	}

	url := url.URL{Scheme: "ws", Host: host, Path: d.md.path}
	if d.tlsEnabled {
		url.Scheme = "wss"
		dialer.TLSClientConfig = d.options.TLSConfig
	}

	if d.md.handshakeTimeout > 0 {
		conn.SetReadDeadline(time.Now().Add(d.md.handshakeTimeout))
	}

	c, resp, err := dialer.DialContext(ctx, url.String(), d.md.header)
	if err != nil {
		return nil, err
	}
	resp.Body.Close()

	if d.md.handshakeTimeout > 0 {
		conn.SetReadDeadline(time.Time{})
	}

	cc := ws_util.Conn(c)

	if d.md.keepaliveInterval > 0 {
		log.Debugf("keepalive is enabled, ttl: %v", d.md.keepaliveInterval)
		c.SetReadDeadline(time.Now().Add(d.md.keepaliveInterval * 2))
		c.SetPongHandler(func(string) error {
			c.SetReadDeadline(time.Now().Add(d.md.keepaliveInterval * 2))
			return nil
		})
		go d.keepAlive(cc)
	}

	// stream multiplex
	session, err := mux.ClientSession(cc, d.md.muxCfg)
	if err != nil {
		log.Error(err)
		return nil, err
	}
	return &muxSession{conn: cc, session: session}, nil
}

func (d *mwsDialer) keepAlive(conn ws_util.WebsocketConn) {
	ticker := time.NewTicker(d.md.keepaliveInterval)
	defer ticker.Stop()

	for range ticker.C {
		d.options.Logger.Debug("send ping")
		conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
		if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
			return
		}
		conn.SetWriteDeadline(time.Time{})
	}
}

func (d *mwsDialer) Retire() {
	for _, session := range d.detachSessions() {
		sessionretire.Gracefully(session)
	}
}

func (d *mwsDialer) Close() error {
	var errs []error
	for _, session := range d.detachSessions() {
		errs = append(errs, session.Close())
	}
	return errors.Join(errs...)
}

func (d *mwsDialer) detachSessions() []*muxSession {
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
