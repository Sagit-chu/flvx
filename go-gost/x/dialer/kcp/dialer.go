package kcp

import (
	"context"
	"errors"
	"net"
	"sync"
	"time"

	"github.com/go-gost/core/dialer"
	"github.com/go-gost/core/logger"
	md "github.com/go-gost/core/metadata"
	kcp_util "github.com/go-gost/x/internal/util/kcp"
	"github.com/go-gost/x/internal/util/sessionretire"
	mdutil "github.com/go-gost/x/metadata/util"
	"github.com/go-gost/x/registry"
	"github.com/xtaci/kcp-go/v5"
	"github.com/xtaci/smux"
	"github.com/xtaci/tcpraw"
)

func init() {
	registry.DialerRegistry().Register("kcp", NewDialer)
}

type kcpDialer struct {
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

	return &kcpDialer{
		sessions: make(map[string]*muxSession),
		logger:   options.Logger,
		options:  options,
	}
}

func (d *kcpDialer) Init(md md.Metadata) (err error) {
	if err = d.parseMetadata(md); err != nil {
		return
	}

	d.md.config.Init()
	if md != nil && md.IsExists("kcp.nc") {
		d.md.config.NoCongestion = mdutil.GetInt(md, "kcp.nc")
	}

	return nil
}

func (d *kcpDialer) Dial(ctx context.Context, addr string, opts ...dialer.DialOption) (conn net.Conn, err error) {
	raddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, err
	}

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

		var pc net.PacketConn
		if d.md.config.TCP {
			pc, err = tcpraw.Dial("tcp", addr)
			if err != nil {
				return nil, err
			}
			pc = &fakeTCPConn{
				raddr:      raddr,
				PacketConn: pc,
			}
		} else {
			c, err := options.Dialer.Dial(ctx, "udp", "")
			if err != nil {
				return nil, err
			}

			var ok bool
			pc, ok = c.(net.PacketConn)
			if !ok {
				c.Close()
				return nil, errors.New("quic: wrong connection type")
			}
		}

		session, err = d.initSession(ctx, raddr, pc)
		if err != nil {
			d.logger.Error(err)
			pc.Close()
			return nil, err
		}
		d.sessions[addr] = session
	}

	conn, err = session.GetConn()
	if err != nil {
		session.Close()
		delete(d.sessions, addr)
		return nil, err
	}

	return
}

func (d *kcpDialer) initSession(ctx context.Context, addr net.Addr, conn net.PacketConn) (*muxSession, error) {
	config := d.md.config

	kcpconn, err := kcp.NewConn(addr.String(),
		kcp_util.BlockCrypt(config.Key, config.Crypt, kcp_util.DefaultSalt),
		config.DataShard, config.ParityShard, conn)
	if err != nil {
		return nil, err
	}

	kcpconn.SetStreamMode(true)
	kcpconn.SetWriteDelay(false)
	kcpconn.SetNoDelay(config.NoDelay, config.Interval, config.Resend, config.NoCongestion)
	kcpconn.SetWindowSize(config.SndWnd, config.RcvWnd)
	kcpconn.SetMtu(config.MTU)
	kcpconn.SetACKNoDelay(config.AckNodelay)

	if config.DSCP > 0 {
		if er := kcpconn.SetDSCP(config.DSCP); er != nil {
			d.logger.Warn("SetDSCP: ", er)
		}
	}
	if er := kcpconn.SetReadBuffer(config.SockBuf); er != nil {
		d.logger.Warn("SetReadBuffer: ", er)
	}
	if er := kcpconn.SetWriteBuffer(config.SockBuf); er != nil {
		d.logger.Warn("SetWriteBuffer: ", er)
	}

	// stream multiplex
	smuxConfig := smux.DefaultConfig()
	smuxConfig.Version = config.SmuxVer
	smuxConfig.MaxReceiveBuffer = config.SmuxBuf
	smuxConfig.MaxStreamBuffer = config.StreamBuf
	if config.KeepAlive > 0 {
		smuxConfig.KeepAliveInterval = time.Duration(config.KeepAlive) * time.Second
	}
	var cc net.Conn = kcpconn
	if !config.NoComp {
		cc = kcp_util.CompStreamConn(kcpconn)
	}
	session, err := smux.Client(cc, smuxConfig)
	if err != nil {
		return nil, err
	}
	return &muxSession{session: session}, nil
}

// Multiplex implements dialer.Multiplexer interface.
func (d *kcpDialer) Multiplex() bool {
	return true
}

// Retire drains existing streams and closes their backing sessions once idle.
func (d *kcpDialer) Retire() {
	for _, session := range d.detachSessions() {
		sessionretire.Gracefully(session)
	}
}

// Close immediately releases all cached multiplex sessions.
func (d *kcpDialer) Close() error {
	var errs []error
	for _, session := range d.detachSessions() {
		errs = append(errs, session.Close())
	}
	return errors.Join(errs...)
}

func (d *kcpDialer) detachSessions() []*muxSession {
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
