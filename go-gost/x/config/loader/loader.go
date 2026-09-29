package loader

import (
	"fmt"

	"github.com/go-gost/core/logger"
	"github.com/go-gost/x/config"
	"github.com/go-gost/x/config/parsing"
	admission_parser "github.com/go-gost/x/config/parsing/admission"
	auth_parser "github.com/go-gost/x/config/parsing/auth"
	bypass_parser "github.com/go-gost/x/config/parsing/bypass"
	chain_parser "github.com/go-gost/x/config/parsing/chain"
	hop_parser "github.com/go-gost/x/config/parsing/hop"
	hosts_parser "github.com/go-gost/x/config/parsing/hosts"
	ingress_parser "github.com/go-gost/x/config/parsing/ingress"
	limiter_parser "github.com/go-gost/x/config/parsing/limiter"
	logger_parser "github.com/go-gost/x/config/parsing/logger"
	observer_parser "github.com/go-gost/x/config/parsing/observer"
	recorder_parser "github.com/go-gost/x/config/parsing/recorder"
	resolver_parser "github.com/go-gost/x/config/parsing/resolver"
	router_parser "github.com/go-gost/x/config/parsing/router"
	sd_parser "github.com/go-gost/x/config/parsing/sd"
	service_parser "github.com/go-gost/x/config/parsing/service"
	"github.com/go-gost/x/registry"
)

var (
	defaultLoader *loader = &loader{}
)

func Load(cfg *config.Config) error {
	return defaultLoader.Load(cfg)
}

// Reload replaces the runtime and commits the config only after it starts.
// The caller must hold config.LockMutation, including while parsing cfg, so the
// rollback snapshot includes every previously acknowledged runtime command.
// Failed loads can partially replace registries and bind listeners; always
// rebuild the last successful snapshot before returning an error.
func Reload(cfg *config.Config, run func(*config.Config) error) error {
	previous := config.Global()
	if err := apply(cfg, run); err != nil {
		// A failed partial load may have left both old and new listeners behind.
		for name := range registry.ServiceRegistry().GetAll() {
			registry.ServiceRegistry().Unregister(name)
		}
		if rollbackErr := apply(previous, run); rollbackErr != nil {
			return fmt.Errorf("reload failed: %w; restore previous config failed: %v", err, rollbackErr)
		}
		return fmt.Errorf("reload failed (previous config restored): %w", err)
	}
	config.Set(cfg)
	return nil
}

func apply(cfg *config.Config, run func(*config.Config) error) error {
	if err := Load(cfg); err != nil {
		return err
	}
	return run(cfg)
}

type loader struct{}

func (l *loader) Load(cfg *config.Config) error {
	logCfg := cfg.Log
	if logCfg == nil {
		logCfg = &config.LogConfig{}
	}
	logger.SetDefault(logger_parser.ParseLogger(&config.LoggerConfig{Log: logCfg}))

	tlsCfg, err := parsing.BuildDefaultTLSConfig(cfg.TLS)
	if err != nil {
		return err
	}
	parsing.SetDefaultTLSConfig(tlsCfg)

	if err := register(cfg); err != nil {
		return err
	}

	return nil
}

func register(cfg *config.Config) error {
	if cfg == nil {
		return nil
	}

	for name := range registry.LoggerRegistry().GetAll() {
		registry.LoggerRegistry().Unregister(name)
	}
	for _, loggerCfg := range cfg.Loggers {
		if err := registry.LoggerRegistry().Register(loggerCfg.Name, logger_parser.ParseLogger(loggerCfg)); err != nil {
			return err
		}
	}

	for name := range registry.AutherRegistry().GetAll() {
		registry.AutherRegistry().Unregister(name)
	}
	for _, autherCfg := range cfg.Authers {
		if err := registry.AutherRegistry().Register(autherCfg.Name, auth_parser.ParseAuther(autherCfg)); err != nil {
			return err
		}
	}

	for name := range registry.AdmissionRegistry().GetAll() {
		registry.AdmissionRegistry().Unregister(name)
	}
	for _, admissionCfg := range cfg.Admissions {
		if err := registry.AdmissionRegistry().Register(admissionCfg.Name, admission_parser.ParseAdmission(admissionCfg)); err != nil {
			return err
		}
	}

	for name := range registry.BypassRegistry().GetAll() {
		registry.BypassRegistry().Unregister(name)
	}
	for _, bypassCfg := range cfg.Bypasses {
		if err := registry.BypassRegistry().Register(bypassCfg.Name, bypass_parser.ParseBypass(bypassCfg)); err != nil {
			return err
		}
	}

	for name := range registry.ResolverRegistry().GetAll() {
		registry.ResolverRegistry().Unregister(name)
	}
	for _, resolverCfg := range cfg.Resolvers {
		r, err := resolver_parser.ParseResolver(resolverCfg)
		if err != nil {
			return err
		}
		if err := registry.ResolverRegistry().Register(resolverCfg.Name, r); err != nil {
			return err
		}
	}

	for name := range registry.HostsRegistry().GetAll() {
		registry.HostsRegistry().Unregister(name)
	}
	for _, hostsCfg := range cfg.Hosts {
		if err := registry.HostsRegistry().Register(hostsCfg.Name, hosts_parser.ParseHostMapper(hostsCfg)); err != nil {
			return err
		}
	}

	for name := range registry.IngressRegistry().GetAll() {
		registry.IngressRegistry().Unregister(name)
	}
	for _, ingressCfg := range cfg.Ingresses {
		if err := registry.IngressRegistry().Register(ingressCfg.Name, ingress_parser.ParseIngress(ingressCfg)); err != nil {
			return err
		}
	}

	for name := range registry.RouterRegistry().GetAll() {
		registry.RouterRegistry().Unregister(name)
	}
	for _, routerCfg := range cfg.Routers {
		if err := registry.RouterRegistry().Register(routerCfg.Name, router_parser.ParseRouter(routerCfg)); err != nil {
			return err
		}
	}

	for name := range registry.SDRegistry().GetAll() {
		registry.SDRegistry().Unregister(name)
	}
	for _, sdCfg := range cfg.SDs {
		if err := registry.SDRegistry().Register(sdCfg.Name, sd_parser.ParseSD(sdCfg)); err != nil {
			return err
		}
	}

	for name := range registry.ObserverRegistry().GetAll() {
		registry.ObserverRegistry().Unregister(name)
	}
	for _, observerCfg := range cfg.Observers {
		if err := registry.ObserverRegistry().Register(observerCfg.Name, observer_parser.ParseObserver(observerCfg)); err != nil {
			return err
		}
	}

	for name := range registry.RecorderRegistry().GetAll() {
		registry.RecorderRegistry().Unregister(name)
	}
	for _, recorderCfg := range cfg.Recorders {
		if err := registry.RecorderRegistry().Register(recorderCfg.Name, recorder_parser.ParseRecorder(recorderCfg)); err != nil {
			return err
		}
	}

	for name := range registry.TrafficLimiterRegistry().GetAll() {
		registry.TrafficLimiterRegistry().Unregister(name)
	}
	for _, limiterCfg := range cfg.Limiters {
		if err := registry.TrafficLimiterRegistry().Register(limiterCfg.Name, limiter_parser.ParseTrafficLimiter(limiterCfg)); err != nil {
			return err
		}
	}

	for name := range registry.ConnLimiterRegistry().GetAll() {
		registry.ConnLimiterRegistry().Unregister(name)
	}
	for _, limiterCfg := range cfg.CLimiters {
		if err := registry.ConnLimiterRegistry().Register(limiterCfg.Name, limiter_parser.ParseConnLimiter(limiterCfg)); err != nil {
			return err
		}
	}

	for name := range registry.RateLimiterRegistry().GetAll() {
		registry.RateLimiterRegistry().Unregister(name)
	}
	for _, limiterCfg := range cfg.RLimiters {
		if err := registry.RateLimiterRegistry().Register(limiterCfg.Name, limiter_parser.ParseRateLimiter(limiterCfg)); err != nil {
			return err
		}
	}

	for name := range registry.HopRegistry().GetAll() {
		registry.HopRegistry().Unregister(name)
	}
	for _, hopCfg := range cfg.Hops {
		hop, err := hop_parser.ParseHop(hopCfg, logger.Default())
		if err != nil {
			return err
		}
		if err := registry.HopRegistry().Register(hopCfg.Name, hop); err != nil {
			return err
		}
	}

	for name := range registry.ChainRegistry().GetAll() {
		registry.ChainRegistry().Unregister(name)
	}
	for _, chainCfg := range cfg.Chains {
		c, err := chain_parser.ParseChain(chainCfg, logger.Default())
		if err != nil {
			return err
		}
		if err := registry.ChainRegistry().Register(chainCfg.Name, c); err != nil {
			return err
		}
	}

	for name := range registry.ServiceRegistry().GetAll() {
		registry.ServiceRegistry().Unregister(name)
	}
	for _, svcCfg := range cfg.Services {
		if svcCfg == nil {
			return fmt.Errorf("service config is nil")
		}
		if paused, _ := svcCfg.Metadata["paused"].(bool); paused {
			continue
		}
		svc, err := service_parser.ParseService(svcCfg)
		if err != nil {
			return err
		}
		if svc != nil {
			if err := registry.ServiceRegistry().Register(svcCfg.Name, svc); err != nil {
				svc.Close()
				return err
			}
		}
	}

	return nil
}
