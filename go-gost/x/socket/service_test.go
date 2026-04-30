package socket

import (
	"net"
	"testing"

	corelogger "github.com/go-gost/core/logger"
	"github.com/go-gost/core/service"
	"github.com/go-gost/x/config"
	xlogger "github.com/go-gost/x/logger"
	"github.com/go-gost/x/registry"
)

type recordingService struct {
	closed int
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
