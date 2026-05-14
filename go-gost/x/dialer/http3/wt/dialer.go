package wt

import (
	"context"
	"crypto/tls"
	"net"
	"sync"

	"github.com/go-gost/core/dialer"
	md "github.com/go-gost/core/metadata"
	"github.com/go-gost/x/registry"
	"github.com/quic-go/quic-go"
	wt "github.com/quic-go/webtransport-go"
)

func init() {
	registry.DialerRegistry().Register("wt", NewDialer)
}

type wtDialer struct {
	clients     map[string]*Client
	clientMutex sync.Mutex
	md          metadata
	options     dialer.Options
}

func NewDialer(opts ...dialer.Option) dialer.Dialer {
	options := dialer.Options{}
	for _, opt := range opts {
		opt(&options)
	}

	return &wtDialer{
		clients: make(map[string]*Client),
		options: options,
	}
}

func (d *wtDialer) Init(md md.Metadata) (err error) {
	if err = d.parseMetadata(md); err != nil {
		return
	}
	return
}

// Multiplex implements dialer.Multiplexer interface.
func (d *wtDialer) Multiplex() bool {
	return true
}

func (d *wtDialer) Dial(ctx context.Context, addr string, opts ...dialer.DialOption) (conn net.Conn, err error) {
	d.clientMutex.Lock()
	defer d.clientMutex.Unlock()

	client := d.clients[addr]
	if client == nil {
		var options dialer.DialOptions
		for _, opt := range opts {
			opt(&options)
		}

		host := d.md.host
		if host == "" {
			host = options.Host
		}
		if h, _, _ := net.SplitHostPort(host); h != "" {
			host = h
		}

		client = &Client{
			log:    d.options.Logger,
			host:   host,
			path:   d.md.path,
			header: d.md.header,
			dialer: &wt.Dialer{
				TLSClientConfig: d.options.TLSConfig,
				DialAddr: func(ctx context.Context, adr string, tlsCfg *tls.Config, cfg *quic.Config) (*quic.Conn, error) {
					// d.options.Logger.Infof("dial: %s, %s, %s", addr, adr, host)
					udpAddr, err := net.ResolveUDPAddr("udp", addr)
					if err != nil {
						return nil, err
					}

					udpConn, err := options.Dialer.Dial(ctx, "udp", "")
					if err != nil {
						return nil, err
					}

					return quic.DialEarly(ctx, udpConn.(net.PacketConn), udpAddr, tlsCfg, cfg)
				},
				QUICConfig: &quic.Config{
					KeepAlivePeriod:      d.md.keepAlivePeriod,
					HandshakeIdleTimeout: d.md.handshakeTimeout,
					MaxIdleTimeout:       d.md.maxIdleTimeout,
					/*
						Versions: []quic.VersionNumber{
							quic.Version1,
						},
					*/
					MaxIncomingStreams: int64(d.md.maxStreams),
					EnableDatagrams:    true,
				},
			},
		}
		d.clients[addr] = client
	}

	return client.Dial(ctx, addr)
}
