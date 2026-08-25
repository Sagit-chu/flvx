package socket

import (
	"net"
	"testing"

	corelogger "github.com/go-gost/core/logger"
	"github.com/go-gost/core/service"
	"github.com/go-gost/x/config"
	_ "github.com/go-gost/x/handler/auto"
	_ "github.com/go-gost/x/listener/tcp"
	xlogger "github.com/go-gost/x/logger"
	"github.com/go-gost/x/registry"
)

type recordingService struct {
	closed int
}

func TestUpdateServicesParseFailureRestoresPreviousRuntime(t *testing.T) {
	corelogger.SetDefault(xlogger.Nop())

	name := "restore_after_failed_update_tdd"
	existing := &recordingService{}
	registry.ServiceRegistry().Unregister(name)
	t.Cleanup(func() { registry.ServiceRegistry().Unregister(name) })
	if err := registry.ServiceRegistry().Register(name, service.Service(existing)); err != nil {
		t.Fatalf("register existing service: %v", err)
	}

	originalConfig := config.Global()
	t.Cleanup(func() { config.Set(originalConfig) })
	serviceConfig := config.ServiceConfig{Name: name, Addr: "127.0.0.1:0"}
	config.Set(&config.Config{Services: []*config.ServiceConfig{&serviceConfig}})

	invalid := serviceConfig
	invalid.Listener = &config.ListenerConfig{Type: "listener-does-not-exist"}
	if err := updateServices(updateServicesRequest{Data: []config.ServiceConfig{invalid}}); err == nil {
		t.Fatalf("expected invalid service update to fail")
	}
	if existing.closed != 1 {
		t.Fatalf("expected old runtime to be closed once, got %d", existing.closed)
	}
	if registry.ServiceRegistry().Get(name) == nil {
		t.Fatalf("expected previous runtime to be restored")
	}

	cfg := config.Global()
	if len(cfg.Services) != 1 || cfg.Services[0] == nil || cfg.Services[0].Name != name {
		t.Fatalf("expected previous config to remain, got %#v", cfg.Services)
	}
	if cfg.Services[0].Listener != nil {
		t.Fatalf("expected previous listener config to remain unchanged")
	}
}

func (s *recordingService) Serve() error   { return nil }
func (s *recordingService) Addr() net.Addr { return nil }
func (s *recordingService) Close() error {
	s.closed++
	return nil
}

func TestUpdateServicesSkipsUnchangedServiceWithoutRestart(t *testing.T) {
	corelogger.SetDefault(xlogger.Nop())

	name := "unchanged_service_tdd"
	existing := &recordingService{}

	registry.ServiceRegistry().Unregister(name)
	defer registry.ServiceRegistry().Unregister(name)
	if err := registry.ServiceRegistry().Register(name, service.Service(existing)); err != nil {
		t.Fatalf("register existing service: %v", err)
	}

	originalConfig := config.Global()
	defer config.Set(originalConfig)
	serviceConfig := config.ServiceConfig{Name: name, Addr: "127.0.0.1:0"}
	config.Set(&config.Config{Services: []*config.ServiceConfig{&serviceConfig}})

	if err := updateServices(updateServicesRequest{Data: []config.ServiceConfig{serviceConfig}}); err != nil {
		t.Fatalf("unchanged update should succeed without parsing/restarting: %v", err)
	}
	if existing.closed != 0 {
		t.Fatalf("unchanged service was restarted, closed %d times", existing.closed)
	}
	if got := registry.ServiceRegistry().Get(name); got != service.Service(existing) {
		t.Fatalf("expected existing service to remain registered")
	}
}
