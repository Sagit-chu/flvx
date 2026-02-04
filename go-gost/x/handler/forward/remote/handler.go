package remote

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/go-gost/core/chain"
	"github.com/go-gost/core/handler"
	"github.com/go-gost/core/hop"
	mdata "github.com/go-gost/core/metadata"
	"github.com/go-gost/core/observer/stats"
	"github.com/go-gost/core/recorder"
	ctxvalue "github.com/go-gost/x/ctx"
	xnet "github.com/go-gost/x/internal/net"
	"github.com/go-gost/x/internal/net/proxyproto"
	"github.com/go-gost/x/internal/util/forwarder"
	"github.com/go-gost/x/internal/util/sniffing"
	tls_util "github.com/go-gost/x/internal/util/tls"
	rate_limiter "github.com/go-gost/x/limiter/rate"
	mdutil "github.com/go-gost/x/metadata/util"
	xstats "github.com/go-gost/x/observer/stats"
	stats_wrapper "github.com/go-gost/x/observer/stats/wrapper"
	xrecorder "github.com/go-gost/x/recorder"
	"github.com/go-gost/x/registry"
)

func init() {
	registry.HandlerRegistry().Register("rtcp", NewHandler)
	registry.HandlerRegistry().Register("rudp", NewHandler)
}

type forwardHandler struct {
	hop      hop.Hop
	md       metadata
	options  handler.Options
	recorder recorder.RecorderObject
	certPool tls_util.CertPool
}

func NewHandler(opts ...handler.Option) handler.Handler {
	options := handler.Options{}
	for _, opt := range opts {
		opt(&options)
	}

	return &forwardHandler{
		options: options,
	}
}

func (h *forwardHandler) Init(md mdata.Metadata) (err error) {
	if err = h.parseMetadata(md); err != nil {
		return
	}

	for _, ro := range h.options.Recorders {
		if ro.Record == xrecorder.RecorderServiceHandler {
			h.recorder = ro
			break
		}
	}

	if h.md.certificate != nil && h.md.privateKey != nil {
		h.certPool = tls_util.NewMemoryCertPool()
	}

	return
}

// Forward implements handler.Forwarder.
func (h *forwardHandler) Forward(hop hop.Hop) {
	h.hop = hop
}

func (h *forwardHandler) Handle(ctx context.Context, conn net.Conn, opts ...handler.HandleOption) (err error) {
	defer conn.Close()

	start := time.Now()

	ro := &xrecorder.HandlerRecorderObject{
		Service:    h.options.Service,
		RemoteAddr: conn.RemoteAddr().String(),
		LocalAddr:  conn.LocalAddr().String(),
		Network:    "tcp",
		Time:       start,
		SID:        string(ctxvalue.SidFromContext(ctx)),
	}

	ro.ClientIP = conn.RemoteAddr().String()
	if clientAddr := ctxvalue.ClientAddrFromContext(ctx); clientAddr != "" {
		ro.ClientIP = string(clientAddr)
	} else {
		ctx = ctxvalue.ContextWithClientAddr(ctx, ctxvalue.ClientAddr(conn.RemoteAddr().String()))
	}

	if h, _, _ := net.SplitHostPort(ro.ClientIP); h != "" {
		ro.ClientIP = h
	}

	log := h.options.Logger.WithFields(map[string]any{
		"remote": conn.RemoteAddr().String(),
		"local":  conn.LocalAddr().String(),
		"sid":    ro.SID,
		"client": ro.ClientIP,
	})
	log.Infof("%s <> %s", conn.RemoteAddr(), conn.LocalAddr())

	network := "tcp"
	if conn.RemoteAddr().Network() == "udp" {
		network = "udp"
	}
	ro.Network = network

	pStats := xstats.Stats{}
	conn = stats_wrapper.WrapConn(conn, &pStats)

	defer func() {
		if err != nil {
			ro.Err = err.Error()
		}
		ro.InputBytes = pStats.Get(stats.KindInputBytes)
		ro.OutputBytes = pStats.Get(stats.KindOutputBytes)
		ro.Duration = time.Since(start)
		if err := ro.Record(ctx, h.recorder.Recorder); err != nil {
			log.Errorf("record: %v", err)
		}

		log.WithFields(map[string]any{
			"duration":    time.Since(start),
			"inputBytes":  ro.InputBytes,
			"outputBytes": ro.OutputBytes,
		}).Infof("%s >< %s", conn.RemoteAddr(), conn.LocalAddr())
	}()

	if !h.checkRateLimit(conn.RemoteAddr()) {
		return rate_limiter.ErrRateLimit
	}

	var host string
	if md, ok := conn.(mdata.Metadatable); ok {
		if v := mdutil.GetString(md.Metadata(), "host"); v != "" {
			host = v
		}
	}

	var proto string
	if network == "tcp" && h.md.sniffing {
		if h.md.sniffingTimeout > 0 {
			conn.SetReadDeadline(time.Now().Add(h.md.sniffingTimeout))
		}

		br := bufio.NewReader(conn)
		proto, _ = sniffing.Sniff(ctx, br)
		ro.Proto = proto

		if h.md.sniffingTimeout > 0 {
			conn.SetReadDeadline(time.Time{})
		}

		dial := func(ctx context.Context, network, address string) (net.Conn, error) {
			var buf bytes.Buffer
			cc, err := h.options.Router.Dial(ctxvalue.ContextWithBuffer(ctx, &buf), "tcp", address)
			ro.Route = buf.String()
			return cc, err
		}
		sniffer := &forwarder.Sniffer{
			Websocket:           h.md.sniffingWebsocket,
			WebsocketSampleRate: h.md.sniffingWebsocketSampleRate,
			Recorder:            h.recorder.Recorder,
			RecorderOptions:     h.recorder.Options,
			Certificate:         h.md.certificate,
			PrivateKey:          h.md.privateKey,
			NegotiatedProtocol:  h.md.alpn,
			CertPool:            h.certPool,
			MitmBypass:          h.md.mitmBypass,
			ReadTimeout:         h.md.readTimeout,
		}

		conn = xnet.NewReadWriteConn(br, conn, conn)
		switch proto {
		case sniffing.ProtoHTTP:
			return sniffer.HandleHTTP(ctx, conn,
				forwarder.WithDial(dial),
				forwarder.WithHop(h.hop),
				forwarder.WithBypass(h.options.Bypass),
				forwarder.WithHTTPKeepalive(h.md.httpKeepalive),
				forwarder.WithRecorderObject(ro),
				forwarder.WithLog(log),
			)
		case sniffing.ProtoTLS:
			return sniffer.HandleTLS(ctx, conn,
				forwarder.WithDial(dial),
				forwarder.WithHop(h.hop),
				forwarder.WithBypass(h.options.Bypass),
				forwarder.WithRecorderObject(ro),
				forwarder.WithLog(log),
			)
		}
	}

	// Determine max retry attempts
	maxRetries := h.md.maxRetries
	if maxRetries <= 0 {
		// Default: try all available nodes
		if nl, ok := h.hop.(hop.NodeList); ok {
			maxRetries = len(nl.Nodes())
		}
		if maxRetries <= 0 {
			maxRetries = 1
		}
	}

	var triedNodes []string
	var lastErr error
	var cc net.Conn

	for attempt := 0; attempt < maxRetries; attempt++ {
		// Select a target node, excluding previously tried nodes
		selectCtx := ctxvalue.ContextWithExcludeNodes(ctx, triedNodes)
		var target *chain.Node
		if host != "" {
			target = chain.NewNode("", host)
		}
		if h.hop != nil {
			target = h.hop.Select(selectCtx,
				hop.ProtocolSelectOption(proto),
			)
		}
		if target == nil {
			if lastErr != nil {
				return lastErr
			}
			err := errors.New("node not available")
			log.Error(err)
			return err
		}

		// Track this node as tried
		triedNodes = append(triedNodes, target.Addr)

		if opts := target.Options(); opts != nil {
			switch opts.Network {
			case "unix":
				network = opts.Network
			default:
			}
		}

		ro.Network = network
		ro.Host = target.Addr

		targetLog := log.WithFields(map[string]any{
			"node": target.Name,
			"dst":  fmt.Sprintf("%s/%s", target.Addr, network),
		})

		targetLog.Debugf("%s >> %s", conn.RemoteAddr(), target.Addr)

		var buf bytes.Buffer
		cc, err = h.options.Router.Dial(ctxvalue.ContextWithBuffer(ctx, &buf), network, target.Addr)
		ro.Route = buf.String()
		if err != nil {
			targetLog.Error(err)
			// Mark node as failed for future selections
			if marker := target.Marker(); marker != nil {
				marker.Mark()
			}
			lastErr = err
			// Try next node
			continue
		}

		// Success - reset marker and proceed
		if marker := target.Marker(); marker != nil {
			marker.Reset()
		}
		defer cc.Close()

		cc = proxyproto.WrapClientConn(h.md.proxyProtocol, conn.RemoteAddr(), convertAddr(conn.LocalAddr()), cc)

		t := time.Now()
		targetLog.Infof("%s <-> %s", conn.RemoteAddr(), target.Addr)
		xnet.Transport(conn, cc)
		targetLog.WithFields(map[string]any{
			"duration": time.Since(t),
		}).Infof("%s >-< %s", conn.RemoteAddr(), target.Addr)

		return nil
	}

	// All retries exhausted
	if lastErr != nil {
		return lastErr
	}
	return errors.New("all nodes failed")
}

func (h *forwardHandler) checkRateLimit(addr net.Addr) bool {
	if h.options.RateLimiter == nil {
		return true
	}
	host, _, _ := net.SplitHostPort(addr.String())
	if limiter := h.options.RateLimiter.Limiter(host); limiter != nil {
		return limiter.Allow(1)
	}

	return true
}

func convertAddr(addr net.Addr) net.Addr {
	host, sp, _ := net.SplitHostPort(addr.String())
	ip := net.ParseIP(host)
	port, _ := strconv.Atoi(sp)

	if ip == nil || ip.Equal(net.IPv6zero) {
		ip = net.IPv4zero
	}

	switch addr.Network() {
	case "tcp", "tcp4", "tcp6":
		return &net.TCPAddr{
			IP:   ip,
			Port: port,
		}

	default:
		return &net.UDPAddr{
			IP:   ip,
			Port: port,
		}
	}
}
