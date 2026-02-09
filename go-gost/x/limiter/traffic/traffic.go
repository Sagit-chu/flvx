package traffic

import (
	"bufio"
	"context"
	"io"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/alecthomas/units"
	"github.com/go-gost/core/limiter"
	"github.com/go-gost/core/limiter/traffic"
	"github.com/go-gost/core/logger"
	"github.com/go-gost/x/internal/loader"
	"github.com/patrickmn/go-cache"
	"github.com/yl2chen/cidranger"
)

const (
	ServiceLimitKey = "$"
	ConnLimitKey    = "$$"
)

const (
	defaultExpiration = 15 * time.Second
	cleanupInterval   = 30 * time.Second
)

type options struct {
	limits      []string
	fileLoader  loader.Loader
	redisLoader loader.Loader
	httpLoader  loader.Loader
	period      time.Duration
	logger      logger.Logger
}

type Option func(opts *options)

func LimitsOption(limits ...string) Option {
	return func(opts *options) {
		opts.limits = limits
	}
}

func ReloadPeriodOption(period time.Duration) Option {
	return func(opts *options) {
		opts.period = period
	}
}

func FileLoaderOption(fileLoader loader.Loader) Option {
	return func(opts *options) {
		opts.fileLoader = fileLoader
	}
}

func RedisLoaderOption(redisLoader loader.Loader) Option {
	return func(opts *options) {
		opts.redisLoader = redisLoader
	}
}

func HTTPLoaderOption(httpLoader loader.Loader) Option {
	return func(opts *options) {
		opts.httpLoader = httpLoader
	}
}

func LoggerOption(logger logger.Logger) Option {
	return func(opts *options) {
		opts.logger = logger
	}
}

type limitValue struct {
	in  int
	out int
}

type trafficLimiter struct {
	generators     sync.Map
	cidrGenerators cidranger.Ranger
	// connection level in/out limits
	connInLimits  *cache.Cache
	connOutLimits *cache.Cache
	// service level in/out limits
	inLimits   *cache.Cache
	outLimits  *cache.Cache
	mu         sync.RWMutex
	cancelFunc context.CancelFunc
	options    options
}

func NewTrafficLimiter(opts ...Option) traffic.TrafficLimiter {
	var options options
	for _, opt := range opts {
		opt(&options)
	}

	ctx, cancel := context.WithCancel(context.TODO())
	lim := &trafficLimiter{
		cidrGenerators: cidranger.NewPCTrieRanger(),
		connInLimits:   cache.New(defaultExpiration, cleanupInterval),
		connOutLimits:  cache.New(defaultExpiration, cleanupInterval),
		inLimits:       cache.New(defaultExpiration, cleanupInterval),
		outLimits:      cache.New(defaultExpiration, cleanupInterval),
		options:        options,
		cancelFunc:     cancel,
	}

	if err := lim.reload(ctx); err != nil {
		options.logger.Warnf("reload: %v", err)
	}
	if lim.options.period > 0 {
		go lim.periodReload(ctx)
	}
	return lim
}

// In obtains a traffic input limiter based on key.
// For connection scope, the key should be client connection address.
func (l *trafficLimiter) In(ctx context.Context, key string, opts ...limiter.Option) traffic.Limiter {
	var options limiter.Options
	for _, opt := range opts {
		opt(&options)
	}

	switch options.Scope {
	case limiter.ScopeService:
		if lim, ok := l.inLimits.Get(ServiceLimitKey); ok && lim != nil {
			return lim.(traffic.Limiter)
		}
		return nil

	case limiter.ScopeClient:
		if lim, ok := l.inLimits.Get(key); ok && lim != nil {
			return lim.(traffic.Limiter)
		}
		return nil

	case limiter.ScopeConn:
		fallthrough
	default:
	}

	var lims []traffic.Limiter

	// connection level limiter
	if lim, ok := l.connInLimits.Get(key); ok {
		if lim != nil {
			// cached connection level limiter
			lims = append(lims, lim.(traffic.Limiter))
			// reset expiration
			l.connInLimits.Set(key, lim, defaultExpiration)
		}
	} else {
		// generate a new connection level limiter and cache it
		if v, ok := l.generators.Load(ConnLimitKey); ok && v != nil {
			lim := v.(*limitGenerator).In()
			if lim != nil {
				lims = append(lims, lim)
				l.connInLimits.Set(key, lim, defaultExpiration)
			}
		}
	}

	host, _, _ := net.SplitHostPort(key)
	// IP level limiter
	if lim, ok := l.inLimits.Get(host); ok {
		// cached IP limiter
		if lim != nil {
			lims = append(lims, lim.(traffic.Limiter))
		}
	} else {
		l.mu.RLock()
		ranger := l.cidrGenerators
		l.mu.RUnlock()

		// CIDR level limiter
		if p, _ := ranger.ContainingNetworks(net.ParseIP(host)); len(p) > 0 {
			if v, _ := p[0].(*cidrLimitEntry); v != nil {
				if lim := v.generator.In(); lim != nil {
					lims = append(lims, lim)
					l.inLimits.Set(host, lim, cache.NoExpiration)
				}
			}
		}
	}

	var lim traffic.Limiter
	if len(lims) > 0 {
		lim = newLimiterGroup(lims...)
	}

	if lim != nil && l.options.logger != nil {
		l.options.logger.Debugf("input limit for %s: %s", key, lim)
	}

	return lim
}

// Out obtains a traffic output limiter based on key.
// For connection scope, the key should be client connection address.
func (l *trafficLimiter) Out(ctx context.Context, key string, opts ...limiter.Option) traffic.Limiter {
	var options limiter.Options
	for _, opt := range opts {
		opt(&options)
	}

	switch options.Scope {
	case limiter.ScopeService:
		if lim, ok := l.outLimits.Get(ServiceLimitKey); ok && lim != nil {
			return lim.(traffic.Limiter)
		}
		return nil

	case limiter.ScopeClient:
		if lim, ok := l.outLimits.Get(key); ok && lim != nil {
			return lim.(traffic.Limiter)
		}
		return nil

	case limiter.ScopeConn:
		fallthrough
	default:
	}

	var lims []traffic.Limiter

	// connection level limiter
	if lim, ok := l.connOutLimits.Get(key); ok {
		if lim != nil {
			// cached connection level limiter
			lims = append(lims, lim.(traffic.Limiter))
			// reset expiration
			l.connOutLimits.Set(key, lim, defaultExpiration)
		}
	} else {
		// generate a new connection level limiter
		if v, ok := l.generators.Load(ConnLimitKey); ok && v != nil {
			lim := v.(*limitGenerator).Out()
			if lim != nil {
				lims = append(lims, lim)
				l.connOutLimits.Set(key, lim, defaultExpiration)
			}
		}
	}

	host, _, _ := net.SplitHostPort(key)
	// IP level limiter
	if lim, ok := l.outLimits.Get(host); ok {
		if lim != nil {
			// cached IP level limiter
			lims = append(lims, lim.(traffic.Limiter))
		}
	} else {
		l.mu.RLock()
		ranger := l.cidrGenerators
		l.mu.RUnlock()

		// CIDR level limiter
		if p, _ := ranger.ContainingNetworks(net.ParseIP(host)); len(p) > 0 {
			if v, _ := p[0].(*cidrLimitEntry); v != nil {
				if lim := v.generator.Out(); lim != nil {
					lims = append(lims, lim)
					l.outLimits.Set(host, lim, cache.NoExpiration)
				}
			}
		}
	}

	var lim traffic.Limiter
	if len(lims) > 0 {
		lim = newLimiterGroup(lims...)
	}

	if lim != nil && l.options.logger != nil {
		l.options.logger.Debugf("output limit for %s: %s", key, lim)
	}

	return lim
}

func (l *trafficLimiter) periodReload(ctx context.Context) error {
	period := l.options.period
	if period < time.Second {
		period = time.Second
	}
	ticker := time.NewTicker(period)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := l.reload(ctx); err != nil {
				l.options.logger.Warnf("reload: %v", err)
				// return err
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (l *trafficLimiter) reload(ctx context.Context) error {
	values, err := l.load(ctx)
	if err != nil {
		return err
	}

	// service level limiter, never expired
	{
		value := values[ServiceLimitKey]
		if v, _ := l.inLimits.Get(ServiceLimitKey); v != nil {
			lim := v.(traffic.Limiter)
			if value.in <= 0 {
				l.inLimits.Delete(ServiceLimitKey)
			} else {
				lim.Set(value.in)
			}
		} else {
			if value.in > 0 {
				l.inLimits.Set(ServiceLimitKey, NewLimiter(value.in), cache.NoExpiration)
			}
		}

		if v, _ := l.outLimits.Get(ServiceLimitKey); v != nil {
			lim := v.(traffic.Limiter)
			if value.out <= 0 {
				l.outLimits.Delete(ServiceLimitKey)
			} else {
				lim.Set(value.out)
			}
		} else {
			if value.out > 0 {
				l.outLimits.Set(ServiceLimitKey, NewLimiter(value.out), cache.NoExpiration)
			}
		}
		delete(values, ServiceLimitKey)
	}

	// connection level limiters
	{
		value := values[ConnLimitKey]

		var in, out int
		if v, _ := l.generators.Load(ConnLimitKey); v != nil {
			in, out = v.(*limitGenerator).in, v.(*limitGenerator).out
		}
		l.generators.Store(ConnLimitKey, newLimitGenerator(value.in, value.out))

		if value.in <= 0 {
			l.connInLimits.Flush()
		} else {
			if in != value.in {
				for _, item := range l.connInLimits.Items() {
					if v := item.Object; v != nil {
						v.(traffic.Limiter).Set(in)
					}
				}
			}
		}

		if value.out <= 0 {
			l.connOutLimits.Flush()
		} else {
			if out != value.out {
				for _, item := range l.connOutLimits.Items() {
					if v := item.Object; v != nil {
						v.(traffic.Limiter).Set(out)
					}
				}
			}
		}
		delete(values, ConnLimitKey)
	}

	cidrGenerators := cidranger.NewPCTrieRanger()
	// IP/CIDR level limiters
	{
		// snapshot of the current limiters
		inLimits := l.inLimits.Items()
		outLimits := l.outLimits.Items()

		delete(inLimits, ServiceLimitKey)
		delete(outLimits, ServiceLimitKey)

		for key, value := range values {
			if _, ipNet, _ := net.ParseCIDR(key); ipNet != nil {
				cidrGenerators.Insert(&cidrLimitEntry{
					ipNet:     *ipNet,
					generator: newLimitGenerator(value.in, value.out),
				})
				continue
			}

			if v, _ := l.inLimits.Get(key); v != nil {
				lim := v.(traffic.Limiter)
				if value.in <= 0 {
					l.inLimits.Delete(key)
				} else {
					lim.Set(value.in)
				}
				delete(inLimits, key)
			} else {
				if value.in > 0 {
					l.inLimits.Set(key, NewLimiter(value.in), cache.NoExpiration)
				}
			}

			if v, _ := l.outLimits.Get(key); v != nil {
				lim := v.(traffic.Limiter)
				if value.out <= 0 {
					l.outLimits.Delete(key)
				} else {
					lim.Set(value.out)
				}
				delete(outLimits, key)
			} else {
				if value.out > 0 {
					l.outLimits.Set(key, NewLimiter(value.out), cache.NoExpiration)
				}
			}
		}

		// check the CIDR for remain limiters, clean the unmatched ones.
		for k, v := range inLimits {
			if p, _ := cidrGenerators.ContainingNetworks(net.ParseIP(k)); len(p) > 0 {
				if le, _ := p[0].(*cidrLimitEntry); le != nil {
					in := le.generator.in
					if in <= 0 {
						l.inLimits.Delete(k)
						continue
					}
					lim := v.Object.(traffic.Limiter)
					if lim.Limit() != in {
						lim.Set(in)
					}
				}
			} else {
				l.inLimits.Delete(k)
			}
		}
		for k, v := range outLimits {
			if p, _ := cidrGenerators.ContainingNetworks(net.ParseIP(k)); len(p) > 0 {
				if le, _ := p[0].(*cidrLimitEntry); le != nil {
					out := le.generator.out
					if out <= 0 {
						l.outLimits.Delete(k)
						continue
					}
					lim := v.Object.(traffic.Limiter)
					if lim.Limit() != out {
						lim.Set(out)
					}
					delete(outLimits, k)
				}
			} else {
				l.outLimits.Delete(k)
			}
		}
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	l.cidrGenerators = cidrGenerators

	return nil
}

func (l *trafficLimiter) load(ctx context.Context) (values map[string]limitValue, err error) {
	values = make(map[string]limitValue)

	for _, v := range l.options.limits {
		key, in, out := l.parseLimit(v)
		if key == "" {
			continue
		}
		values[key] = limitValue{in: in, out: out}
	}

	if l.options.fileLoader != nil {
		if lister, ok := l.options.fileLoader.(loader.Lister); ok {
			list, er := lister.List(ctx)
			if er != nil {
				l.options.logger.Warnf("file loader: %v", er)
			}
			for _, s := range list {
				key, in, out := l.parseLimit(l.parseLine(s))
				if key == "" {
					continue
				}
				values[key] = limitValue{in: in, out: out}
			}
		} else {
			r, er := l.options.fileLoader.Load(ctx)
			if er != nil {
				l.options.logger.Warnf("file loader: %v", er)
			}
			patterns, _ := l.parsePatterns(r)
			for _, s := range patterns {
				key, in, out := l.parseLimit(l.parseLine(s))
				if key == "" {
					continue
				}
				values[key] = limitValue{in: in, out: out}
			}
		}
	}
	if l.options.redisLoader != nil {
		if lister, ok := l.options.redisLoader.(loader.Lister); ok {
			list, er := lister.List(ctx)
			if er != nil {
				l.options.logger.Warnf("redis loader: %v", er)
			}
			for _, s := range list {
				key, in, out := l.parseLimit(l.parseLine(s))
				if key == "" {
					continue
				}
				values[key] = limitValue{in: in, out: out}
			}
		} else {
			r, er := l.options.redisLoader.Load(ctx)
			if er != nil {
				l.options.logger.Warnf("redis loader: %v", er)
			}
			patterns, _ := l.parsePatterns(r)
			for _, s := range patterns {
				key, in, out := l.parseLimit(l.parseLine(s))
				if key == "" {
					continue
				}
				values[key] = limitValue{in: in, out: out}
			}
		}
	}
	if l.options.httpLoader != nil {
		r, er := l.options.httpLoader.Load(ctx)
		if er != nil {
			l.options.logger.Warnf("http loader: %v", er)
		}
		patterns, _ := l.parsePatterns(r)
		for _, s := range patterns {
			key, in, out := l.parseLimit(l.parseLine(s))
			if key == "" {
				continue
			}
			values[key] = limitValue{in: in, out: out}
		}
	}

	l.options.logger.Debugf("load items %d", len(values))
	return
}

func (l *trafficLimiter) parsePatterns(r io.Reader) (patterns []string, err error) {
	if r == nil {
		return
	}

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		if line := l.parseLine(scanner.Text()); line != "" {
			patterns = append(patterns, line)
		}
	}

	err = scanner.Err()
	return
}

func (l *trafficLimiter) parseLine(s string) string {
	if n := strings.IndexByte(s, '#'); n >= 0 {
		s = s[:n]
	}
	return strings.TrimSpace(s)
}

func (l *trafficLimiter) parseLimit(s string) (key string, in, out int) {
	s = strings.Replace(s, "\t", " ", -1)
	s = strings.TrimSpace(s)
	if s == "" {
		return
	}

	var ss []string
	for _, v := range strings.Split(s, " ") {
		if v != "" {
			ss = append(ss, v)
		}
	}
	if len(ss) < 2 {
		return
	}

	key = ss[0]
	if v, _ := units.ParseBase2Bytes(ss[1]); v > 0 {
		in = int(v)
	}
	if len(ss) > 2 {
		if v, _ := units.ParseBase2Bytes(ss[2]); v > 0 {
			out = int(v)
		}
	}

	return
}

func (l *trafficLimiter) Close() error {
	l.cancelFunc()
	if l.options.fileLoader != nil {
		l.options.fileLoader.Close()
	}
	if l.options.redisLoader != nil {
		l.options.redisLoader.Close()
	}
	return nil
}

type cidrLimitEntry struct {
	ipNet     net.IPNet
	generator *limitGenerator
}

func (p *cidrLimitEntry) Network() net.IPNet {
	return p.ipNet
}
