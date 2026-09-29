package service

import (
	"net"
	"net/http"

	"github.com/go-gost/core/auth"
	"github.com/go-gost/core/service"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	DefaultPath = "/metrics"
)

type options struct {
	path   string
	auther auth.Authenticator
}

type Option func(*options)

func PathOption(path string) Option {
	return func(o *options) {
		o.path = path
	}
}

func AutherOption(auther auth.Authenticator) Option {
	return func(o *options) {
		o.auther = auther
	}
}

type metricService struct {
	s      *http.Server
	ln     net.Listener
	cclose chan struct{}
}

func NewService(network, addr string, opts ...Option) (service.Service, error) {
	if network == "" {
		network = "tcp"
	}
	ln, err := net.Listen(network, addr)
	if err != nil {
		return nil, err
	}

	var options options
	for _, opt := range opts {
		opt(&options)
	}
	if options.path == "" {
		options.path = DefaultPath
	}

	mux := http.NewServeMux()
	mux.Handle(options.path, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if options.auther != nil {
			u, p, _ := r.BasicAuth()
			if _, ok := options.auther.Authenticate(r.Context(), u, p); !ok {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
		}
		promhttp.Handler().ServeHTTP(w, r)
	}))
	return &metricService{
		s: &http.Server{
			Handler: mux,
		},
		ln:     ln,
		cclose: make(chan struct{}),
	}, nil
}

func (s *metricService) Serve() error {
	return s.s.Serve(s.ln)
}

func (s *metricService) Addr() net.Addr {
	return s.ln.Addr()
}

func (s *metricService) Close() error {
	err := s.s.Close()
	s.ln.Close()
	return err
}

func (s *metricService) IsClosed() bool {
	select {
	case <-s.cclose:
		return true
	default:
		return false
	}
}
