package service

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/go-gost/core/admission"
	"github.com/go-gost/core/handler"
	"github.com/go-gost/core/listener"
	"github.com/go-gost/core/logger"
	"github.com/go-gost/core/metrics"
	"github.com/go-gost/core/observer"
	"github.com/go-gost/core/observer/stats"
	"github.com/go-gost/core/recorder"
	"github.com/go-gost/core/service"
	ctxvalue "github.com/go-gost/x/ctx"
	xnet "github.com/go-gost/x/internal/net"
	xmetrics "github.com/go-gost/x/metrics"
	xstats "github.com/go-gost/x/observer/stats"
	"github.com/rs/xid"
)

type options struct {
	admission      admission.Admission
	recorders      []recorder.RecorderObject
	preUp          []string
	postUp         []string
	preDown        []string
	postDown       []string
	stats          stats.Stats
	observer       observer.Observer
	observerPeriod time.Duration
	logger         logger.Logger
}

var isTls = 0

var isHttp = 0

var isSocks = 0

var needWrap = false

// SetProtocolBlock sets protocol blocking switches and recomputes wrapper need
func SetProtocolBlock(httpOn int, tlsOn int, socksOn int) {
	isHttp = httpOn
	isTls = tlsOn
	isSocks = socksOn
	needWrap = isTls+isSocks+isHttp > 0
}

type Option func(opts *options)

func init() {
	_, err := LoadConfig("config.json")
	fmt.Println("config.json loaded")
	if err != nil {
		log.Fatal(err)
	}
	needWrap = isTls+isSocks+isHttp > 0
}

func AdmissionOption(admission admission.Admission) Option {
	return func(opts *options) {
		opts.admission = admission
	}
}

func RecordersOption(recorders ...recorder.RecorderObject) Option {
	return func(opts *options) {
		opts.recorders = recorders
	}
}

func PreUpOption(cmds []string) Option {
	return func(opts *options) {
		opts.preUp = cmds
	}
}

func PreDownOption(cmds []string) Option {
	return func(opts *options) {
		opts.preDown = cmds
	}
}

func PostUpOption(cmds []string) Option {
	return func(opts *options) {
		opts.postUp = cmds
	}
}

func PostDownOption(cmds []string) Option {
	return func(opts *options) {
		opts.postDown = cmds
	}
}

func StatsOption(stats stats.Stats) Option {
	return func(opts *options) {
		opts.stats = stats
	}
}

func ObserverOption(observer observer.Observer) Option {
	return func(opts *options) {
		opts.observer = observer
	}
}

func ObserverPeriodOption(period time.Duration) Option {
	return func(opts *options) {
		opts.observerPeriod = period
	}
}

func LoggerOption(logger logger.Logger) Option {
	return func(opts *options) {
		opts.logger = logger
	}
}

type defaultService struct {
	name     string
	listener listener.Listener
	handler  handler.Handler
	status   *Status
	options  options
}

func NewService(name string, ln listener.Listener, h handler.Handler, opts ...Option) service.Service {
	var options options
	for _, opt := range opts {
		opt(&options)
	}
	s := &defaultService{
		name:     name,
		listener: ln,
		handler:  h,
		options:  options,
		status: &Status{
			createTime: time.Now(),
			events:     make([]Event, 0, MaxEventSize),
			stats:      options.stats,
		},
	}
	s.setState(StateRunning)

	s.execCmds("pre-up", s.options.preUp)

	return s
}

func (s *defaultService) Addr() net.Addr {
	return s.listener.Addr()
}

func (s *defaultService) Serve() error {

	s.execCmds("post-up", s.options.postUp)
	s.setState(StateReady)
	s.status.addEvent(Event{
		Time:    time.Now(),
		Message: fmt.Sprintf("service %s is listening on %s", s.name, s.listener.Addr()),
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if s.status.Stats() != nil {
		go s.observeStats(ctx)
	}

	if v := xmetrics.GetGauge(
		xmetrics.MetricServicesGauge,
		metrics.Labels{}); v != nil {
		v.Inc()
		defer v.Dec()
	}

	var wg sync.WaitGroup
	defer wg.Wait()

	var tempDelay time.Duration
	for {
		conn, e := s.listener.Accept()
		if e != nil {

			// TODO: remove Temporary checking
			if ne, ok := e.(net.Error); ok && ne.Temporary() {
				if tempDelay == 0 {
					tempDelay = 1 * time.Second
				} else {
					tempDelay *= 2
				}
				if max := 5 * time.Second; tempDelay > max {
					tempDelay = max
				}

				s.setState(StateFailed)

				s.options.logger.Warnf("accept: %v, retrying in %v", e, tempDelay)
				time.Sleep(tempDelay)
				continue
			}
			s.setState(StateClosed)

			if !errors.Is(e, net.ErrClosed) {
				s.options.logger.Errorf("accept: %v", e)
			}

			return e
		}

		if tempDelay > 0 {
			tempDelay = 0
			s.setState(StateReady)
		}

		clientAddr := conn.RemoteAddr().String()
		if ca, ok := conn.(xnet.ClientAddr); ok {
			if addr := ca.ClientAddr(); addr != nil {
				clientAddr = addr.String()
			}
		}
		clientIP := clientAddr
		if h, _, _ := net.SplitHostPort(clientAddr); h != "" {
			clientIP = h
		}

		sid := xid.New().String()
		ctx := ctxvalue.ContextWithSid(ctx, ctxvalue.Sid(sid))
		ctx = ctxvalue.ContextWithClientAddr(ctx, ctxvalue.ClientAddr(clientAddr))
		ctx = ctxvalue.ContextWithHash(ctx, &ctxvalue.Hash{Source: clientIP})

		log := s.options.logger.WithFields(map[string]any{
			"sid": sid,
		})

		for _, rec := range s.options.recorders {
			if rec.Record == recorder.RecorderServiceClientAddress {
				if err := rec.Recorder.Record(ctx, []byte(clientIP)); err != nil {
					log.Errorf("record %s: %v", rec.Record, err)
				}
				break
			}
		}
		if s.options.admission != nil &&
			!s.options.admission.Admit(ctx, clientAddr) {
			conn.Close()
			log.Debugf("admission: %s is denied", clientAddr)
			continue
		}

		wg.Add(1)

		go func() {
			defer wg.Done()

			if v := xmetrics.GetCounter(xmetrics.MetricServiceRequestsCounter,
				metrics.Labels{"service": s.name, "client": clientIP}); v != nil {
				v.Inc()
			}

			if v := xmetrics.GetGauge(xmetrics.MetricServiceRequestsInFlightGauge,
				metrics.Labels{"service": s.name, "client": clientIP}); v != nil {
				v.Inc()
				defer v.Dec()
			}

			start := time.Now()
			if v := xmetrics.GetObserver(xmetrics.MetricServiceRequestsDurationObserver,
				metrics.Labels{"service": s.name}); v != nil {
				defer func() {
					v.Observe(float64(time.Since(start).Seconds()))
				}()
			}

			if needWrap {
				conn = wrapConnPDetection(conn)
			}

			if err := s.handler.Handle(ctx, conn); err != nil {
				if !errors.Is(err, net.ErrClosed) {
					log.Error(err)
				}
				if v := xmetrics.GetCounter(xmetrics.MetricServiceHandlerErrorsCounter,
					metrics.Labels{"service": s.name, "client": clientIP}); v != nil {
					v.Inc()
				}
				if sts := s.status.stats; sts != nil {
					sts.Add(stats.KindTotalErrs, 1)
				}
			}
		}()
	}
}

func (s *defaultService) Status() *Status {
	return s.status
}

func (s *defaultService) Close() error {
	s.execCmds("pre-down", s.options.preDown)
	defer s.execCmds("post-down", s.options.postDown)

	if closer, ok := s.handler.(io.Closer); ok {
		closer.Close()
	}
	return s.listener.Close()
}

func (s *defaultService) execCmds(phase string, cmds []string) {
	for _, cmd := range cmds {
		cmd := strings.TrimSpace(cmd)
		if cmd == "" {
			continue
		}
		s.options.logger.Info(cmd)

		if err := exec.Command("/bin/sh", "-c", cmd).Run(); err != nil {
			s.options.logger.Warnf("[%s] %s: %v", phase, cmd, err)
		}
	}
}

func (s *defaultService) setState(state State) {
	s.status.setState(state)

	msg := fmt.Sprintf("service %s is %s", s.name, state)
	s.status.addEvent(Event{
		Time:    time.Now(),
		Message: msg,
	})

	if obs := s.options.observer; obs != nil {
		obs.Observe(context.Background(), []observer.Event{ServiceEvent{
			Kind:    "service",
			Service: s.name,
			State:   state,
			Msg:     msg,
		}})
	}
}

func (s *defaultService) observeStats(ctx context.Context) {
	if s.options.observer == nil {
		return
	}

	d := s.options.observerPeriod
	if d == 0 {
		d = 5 * time.Second
	}
	if d < time.Second {
		d = 1 * time.Second
	}

	var events []observer.Event

	ticker := time.NewTicker(d)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:

			// First, try to send any pending events
			if len(events) > 0 {
				if err := s.options.observer.Observe(ctx, events); err == nil {
					events = nil
				}
				continue
			}

			st := s.status.Stats()
			if st == nil {
				continue
			}

			isUpdated := st.IsUpdated()
			if isUpdated {
				inputBytes := st.Get(stats.KindInputBytes)
				outputBytes := st.Get(stats.KindOutputBytes)

				evs := []observer.Event{
					xstats.StatsEvent{
						Kind:         "service",
						Service:      s.name,
						TotalConns:   st.Get(stats.KindTotalConns),
						CurrentConns: st.Get(stats.KindCurrentConns),
						InputBytes:   inputBytes,
						OutputBytes:  outputBytes,
						TotalErrs:    st.Get(stats.KindTotalErrs),
					},
				}

				// 将流量累积到全局管理器，而不是立即上报
				if outputBytes > 0 || inputBytes > 0 {
					globalManager := GetGlobalTrafficManager()
					globalManager.AddTraffic(s.name, int64(outputBytes), int64(inputBytes))

					// 立即重置流量计数（因为已经记录到全局管理器中）
					if xstats, ok := st.(*xstats.Stats); ok {
						xstats.ResetTraffic(st.Get(stats.KindInputBytes)-inputBytes, st.Get(stats.KindOutputBytes)-outputBytes)
					}
				}

				if err := s.options.observer.Observe(ctx, evs); err != nil {
					fmt.Printf("发送观察器事件失败: %v", err)
					events = evs
				}
			}

		case <-ctx.Done():
			return
		}
	}
}

type ServiceEvent struct {
	Kind    string
	Service string
	State   State
	Msg     string
}

func (ServiceEvent) Type() observer.EventType {
	return observer.EventStatus
}

func wrapConnPDetection(conn net.Conn) net.Conn {
	return &detectConn{
		Conn:   conn,
		reader: bufio.NewReader(conn),
	}
}

type detectConn struct {
	net.Conn
	reader   *bufio.Reader
	detected bool
}

func (c *detectConn) Read(b []byte) (int, error) {
	n, err := c.reader.Read(b)
	if n > 0 && !c.detected {
		c.detected = true
		if detectProtocol(b[:n], c.Conn) {
			return 0, fmt.Errorf("connection blocked")
		}
	}
	return n, err
}

func detectProtocol(data []byte, conn net.Conn) (blocked bool) {
	// 如果是 UDP，则不检测，直接放行
	if conn.RemoteAddr().Network() == "udp" || conn.RemoteAddr().Network() == "udp4" || conn.RemoteAddr().Network() == "udp6" {
		return false
	}

	if isHttp == 1 && detectHTTP(data) {
		conn.Close()
		return true
	}

	if isTls == 1 && detectTLS(data) {
		conn.Close()
		return true
	}

	if isSocks == 1 && detectSOCKS(data) {
		conn.Close()
		return true
	}

	return false
}

func detectHTTP(data []byte) bool {
	if len(data) < 3 {
		return false
	}
	switch {
	case len(data) >= 3 && data[0] == 'G' && data[1] == 'E' && data[2] == 'T':
		return true
	case len(data) >= 4 && data[0] == 'P' && data[1] == 'O' && data[2] == 'S' && data[3] == 'T':
		return true
	case len(data) >= 3 && data[0] == 'P' && data[1] == 'U' && data[2] == 'T':
		return true
	case len(data) >= 6 && data[0] == 'D' && data[1] == 'E' && data[2] == 'L' &&
		data[3] == 'E' && data[4] == 'T' && data[5] == 'E':
		return true
	case len(data) >= 4 && data[0] == 'H' && data[1] == 'E' && data[2] == 'A' && data[3] == 'D':
		return true
	case len(data) >= 7 && data[0] == 'O' && data[1] == 'P' && data[2] == 'T' &&
		data[3] == 'I' && data[4] == 'O' && data[5] == 'N' && data[6] == 'S':
		return true
	case len(data) >= 5 && data[0] == 'P' && data[1] == 'A' && data[2] == 'T' &&
		data[3] == 'C' && data[4] == 'H':
		return true
	case len(data) >= 7 && data[0] == 'C' && data[1] == 'O' && data[2] == 'N' &&
		data[3] == 'N' && data[4] == 'E' && data[5] == 'C' && data[6] == 'T': // HTTPS proxy
		return true
	default:
		return false
	}
}

func detectTLS(data []byte) bool {
	if len(data) < 5 {
		return false
	}
	if data[0] == 0x16 && data[1] == 0x03 && data[2] >= 0x01 && data[2] <= 0x04 {
		return true
	}
	return false
}

func detectSOCKS(data []byte) bool {
	if len(data) == 0 {
		return false
	}

	switch data[0] {
	case 0x04:
		if len(data) < 7 {
			return false
		}
		cmd := data[1]
		if cmd != 0x01 && cmd != 0x02 {
			return false
		}
		return true
	case 0x05:
		if len(data) < 2 {
			return false
		}
		nMethods := int(data[1])
		if len(data) < 2+nMethods {
			return false
		}
		for _, method := range data[2 : 2+nMethods] {
			if method == 0x00 || method == 0x02 {
				return true
			}
		}
	}
	return false
}

// Config 配置结构体
type Config struct {
	Addr   string `json:"addr"`
	Secret string `json:"secret"`
	Http   int    `json:"http"`
	Tls    int    `json:"tls"`
	Socks  int    `json:"socks"`
}

func LoadConfig(configPath string) (string, error) {
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return "", fmt.Errorf("配置文件不存在: %s", configPath)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return "", fmt.Errorf("读取配置文件失败: %v", err)
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return "", fmt.Errorf("解析配置文件失败: %v", err)
	}

	isTls = config.Tls
	isSocks = config.Socks
	isHttp = config.Http

	return "", nil

}
