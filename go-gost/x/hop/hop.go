package hop

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/go-gost/core/bypass"
	"github.com/go-gost/core/chain"
	"github.com/go-gost/core/hop"
	"github.com/go-gost/core/logger"
	"github.com/go-gost/core/routing"
	"github.com/go-gost/core/selector"
	"github.com/go-gost/x/config"
	node_parser "github.com/go-gost/x/config/parsing/node"
	ctxvalue "github.com/go-gost/x/ctx"
	"github.com/go-gost/x/internal/loader"
)

type options struct {
	name        string
	nodes       []*chain.Node
	bypass      bypass.Bypass
	selector    selector.Selector[*chain.Node]
	fileLoader  loader.Loader
	redisLoader loader.Loader
	httpLoader  loader.Loader
	period      time.Duration
	logger      logger.Logger
}

type Option func(*options)

func NameOption(name string) Option {
	return func(o *options) {
		o.name = name
	}
}

func NodeOption(nodes ...*chain.Node) Option {
	return func(o *options) {
		o.nodes = nodes
	}
}
func BypassOption(bp bypass.Bypass) Option {
	return func(o *options) {
		o.bypass = bp
	}
}

func SelectorOption(s selector.Selector[*chain.Node]) Option {
	return func(o *options) {
		o.selector = s
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

type chainHop struct {
	nodes      []*chain.Node
	mu         sync.RWMutex
	cancelFunc context.CancelFunc
	stopOnce   sync.Once
	options    options
}

func NewHop(opts ...Option) hop.Hop {
	var options options
	for _, opt := range opts {
		if opt != nil {
			opt(&options)
		}
	}

	ctx, cancel := context.WithCancel(context.TODO())
	p := &chainHop{
		cancelFunc: cancel,
		options:    options,
	}

	if err := p.reload(ctx); err != nil {
		options.logger.Warnf("reload: %v", err)
	}
	if p.options.period > 0 {
		go p.periodReload(ctx)
	}

	return p
}

func (p *chainHop) Nodes() []*chain.Node {
	if p == nil {
		return nil
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.nodes
}

func (p *chainHop) Select(ctx context.Context, opts ...hop.SelectOption) *chain.Node {
	var options hop.SelectOptions
	for _, opt := range opts {
		opt(&options)
	}

	log := p.options.logger

	// hop level bypass
	if p.options.bypass != nil &&
		p.options.bypass.Contains(ctx, options.Network, options.Addr, bypass.WithHostOpton(options.Host)) {
		return nil
	}

	// Get list of nodes to exclude (for failover retry)
	excludeNodes := ctxvalue.ExcludeNodesFromContext(ctx)
	excludeSet := make(map[string]bool)
	for _, addr := range excludeNodes {
		excludeSet[addr] = true
	}

	// Debug logging for failover analysis
	log.Debugf("[hop.Select] excludeNodes=%v, totalNodes=%d", excludeNodes, len(p.Nodes()))

	var nodes []*chain.Node
	for _, node := range p.Nodes() {
		if node == nil {
			continue
		}

		// Skip nodes in the exclude list (failover retry)
		if excludeSet[node.Addr] || excludeSet[node.Name] {
			log.Debugf("node %s(%s) excluded for failover retry", node.Name, node.Addr)
			continue
		}

		// node level bypass
		if node.Options().Bypass != nil &&
			node.Options().Bypass.Contains(ctx, options.Network, options.Addr, bypass.WithHostOpton(options.Host)) {
			continue
		}

		if matcher := node.Options().Matcher; matcher != nil {
			req := routing.Request{
				ClientIP: options.ClientIP,
				Host:     options.Host,
				Protocol: options.Protocol,
				Method:   options.Method,
				Path:     options.Path,
				Query:    options.Query,
				Header:   options.Header,
			}
			if !matcher.Match(&req) {
				continue
			}
			log.Debugf("node %s match request %s %s, priority %d", node.Name, req.Protocol, req.Host, node.Options().Priority)
		} else {
			if !p.isEligible(node, &options) {
				continue
			}
		}

		nodes = append(nodes, node)
	}
	if len(nodes) == 0 {
		return nil
	}

	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].Options().Priority > nodes[j].Options().Priority
	})

	if nodes[0].Options().Priority > 0 {
		return nodes[0]
	}

	// Use selector with FailFilter for proper failover.
	// FailFilter will exclude recently-failed nodes, allowing traffic to
	// be routed to healthy alternatives.
	// Note: FailFilter has a safety guard (len <= 1 returns as-is) to ensure
	// the last remaining node is never permanently blocked.
	if s := p.options.selector; s != nil {
		log.Debugf("[hop.Select] calling selector.Select with %d nodes", len(nodes))
		if node := s.Select(ctx, nodes...); node != nil {
			log.Debugf("[hop.Select] selected node=%s addr=%s", node.Name, node.Addr)
			return node
		}
		// All nodes filtered out by FailFilter - all are marked as failed.
		// Return nil to signal "no healthy nodes available" to the caller.
		// The handler's retry loop will handle this appropriately.
		log.Debugf("all %d nodes filtered out by FailFilter, no healthy nodes available", len(nodes))
		return nil
	}

	// Fallback: return first node if no selector configured
	return nodes[0]
}

func (p *chainHop) isEligible(node *chain.Node, opts *hop.SelectOptions) bool {
	if node == nil {
		return false
	}
	if node.Options().Filter == nil {
		return true
	}

	if !p.checkHost(opts.Host, node) || !p.checkProtocol(opts.Protocol, node) || !p.checkPath(opts.Path, node) {
		return false
	}
	return true
}

func (p *chainHop) checkHost(host string, node *chain.Node) bool {
	var vhost string
	if filter := node.Options().Filter; filter != nil {
		vhost = filter.Host
	}
	if vhost == "" { // backup node
		return true
	}

	if host == "" {
		return false
	}

	if v, _, _ := net.SplitHostPort(host); v != "" {
		host = v
	}

	if vhost == host || vhost[0] == '.' && strings.HasSuffix(host, vhost[1:]) {
		return true
	}

	return false
}

func (p *chainHop) checkProtocol(protocol string, node *chain.Node) bool {
	var prot string
	if filter := node.Options().Filter; filter != nil {
		prot = filter.Protocol
	}
	if prot == "" {
		return true
	}
	return prot == protocol
}

func (p *chainHop) checkPath(path string, node *chain.Node) bool {
	var pathFilter string
	if filter := node.Options().Filter; filter != nil {
		pathFilter = filter.Path
	}

	if pathFilter == "" {
		return true
	}

	return strings.HasPrefix(path, pathFilter)
}

func (p *chainHop) periodReload(ctx context.Context) error {
	period := p.options.period
	if period < time.Second {
		period = time.Second
	}
	ticker := time.NewTicker(period)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := p.reload(ctx); err != nil {
				p.options.logger.Warnf("reload: %v", err)
				// return err
			}
			p.options.logger.Debug("hop reload done")
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (p *chainHop) reload(ctx context.Context) (err error) {
	nodes := p.options.nodes

	nl, err := p.load(ctx)

	nodes = append(nodes, nl...)

	p.options.logger.Debugf("load items %d", len(nodes))

	p.mu.Lock()
	defer p.mu.Unlock()

	p.nodes = nodes

	return
}

func (p *chainHop) load(ctx context.Context) (nodes []*chain.Node, err error) {
	if loader := p.options.fileLoader; loader != nil {
		r, er := loader.Load(ctx)
		if er != nil {
			p.options.logger.Warnf("file loader: %v", er)
		}
		nodes, _ = p.parseNode(r)
	}

	if loader := p.options.redisLoader; loader != nil {
		r, er := loader.Load(ctx)
		if er != nil {
			p.options.logger.Warnf("redis loader: %v", er)
		}
		ns, _ := p.parseNode(r)
		nodes = append(nodes, ns...)
	}

	if loader := p.options.httpLoader; loader != nil {
		r, er := loader.Load(ctx)
		if er != nil {
			p.options.logger.Warnf("http loader: %v", er)
		}
		if ns, _ := p.parseNode(r); ns != nil {
			nodes = append(nodes, ns...)
		}
	}

	return
}

func (p *chainHop) parseNode(r io.Reader) ([]*chain.Node, error) {
	if r == nil {
		return nil, nil
	}

	var ncs []*config.NodeConfig
	if err := json.NewDecoder(r).Decode(&ncs); err != nil {
		return nil, err
	}

	var nodes []*chain.Node
	for _, nc := range ncs {
		if nc == nil {
			continue
		}

		node, err := node_parser.ParseNode(p.options.name, nc, logger.Default())
		if err != nil {
			return nodes, err
		}
		nodes = append(nodes, node)
	}
	return nodes, nil
}

func (p *chainHop) stopReload() {
	if p == nil {
		return
	}
	p.stopOnce.Do(func() {
		p.cancelFunc()
		if p.options.fileLoader != nil {
			p.options.fileLoader.Close()
		}
		if p.options.redisLoader != nil {
			p.options.redisLoader.Close()
		}
		if p.options.httpLoader != nil {
			p.options.httpLoader.Close()
		}
	})
}

func (p *chainHop) Retire() {
	if p == nil {
		return
	}
	p.stopReload()
	for _, node := range p.Nodes() {
		if node == nil || node.Options().Transport == nil {
			continue
		}
		if retirer, ok := node.Options().Transport.(interface{ Retire() }); ok {
			retirer.Retire()
		}
	}
}

func (p *chainHop) Close() error {
	if p == nil {
		return nil
	}
	p.stopReload()
	var errs []error
	for _, node := range p.Nodes() {
		if node == nil || node.Options().Transport == nil {
			continue
		}
		if closer, ok := node.Options().Transport.(io.Closer); ok {
			errs = append(errs, closer.Close())
		}
	}
	return errors.Join(errs...)
}
