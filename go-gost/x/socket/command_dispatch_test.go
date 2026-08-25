package socket

import (
	"net"
	"sync"
	"testing"
	"time"

	coreservice "github.com/go-gost/core/service"
	"github.com/go-gost/x/config"
	"github.com/go-gost/x/registry"
)

type blockingCommandService struct {
	started     chan struct{}
	release     chan struct{}
	startedOnce sync.Once
}

func (s *blockingCommandService) Serve() error   { return nil }
func (s *blockingCommandService) Addr() net.Addr { return nil }
func (s *blockingCommandService) Close() error {
	s.startedOnce.Do(func() { close(s.started) })
	<-s.release
	return nil
}

func TestMutationCommandsAreSerialized(t *testing.T) {
	mutations := []string{
		"AddService", "UpdateService", "DeleteService", "PauseService", "ResumeService",
		"AddChains", "UpdateChains", "DeleteChains",
		"AddLimiters", "UpdateLimiters", "DeleteLimiters",
		"AddCLimiters", "UpdateCLimiters", "DeleteCLimiters",
		"SetProtocol", "UpgradeAgent", "RollbackAgent", "reload",
	}
	for _, command := range mutations {
		if !isMutationCommand(command) {
			t.Fatalf("expected %s to use the serialized mutation queue", command)
		}
	}
}

func TestReadOnlyCommandsRemainBoundedAsync(t *testing.T) {
	for _, command := range []string{"TcpPing", "UdpPing", "ServiceMonitorCheck"} {
		if isMutationCommand(command) {
			t.Fatalf("expected %s to remain a read-only command", command)
		}
	}
}

func TestCommandResponseTypePreservesRequestContract(t *testing.T) {
	if got := commandResponseType("UpdateService"); got != "UpdateServiceResponse" {
		t.Fatalf("unexpected response type: %s", got)
	}
	if got := commandResponseType(""); got != "UnknownCommandResponse" {
		t.Fatalf("unexpected empty command response type: %s", got)
	}
}

func TestMutationQueueExecutesCommandsInArrivalOrder(t *testing.T) {
	originalConfig := config.Global()
	t.Cleanup(func() { config.Set(originalConfig) })

	firstName := "mutation_queue_first_tdd"
	secondName := "mutation_queue_second_tdd"
	first := &blockingCommandService{started: make(chan struct{}), release: make(chan struct{})}
	second := &blockingCommandService{started: make(chan struct{}), release: make(chan struct{})}
	for _, name := range []string{firstName, secondName} {
		registry.ServiceRegistry().Unregister(name)
	}
	t.Cleanup(func() {
		select {
		case <-first.release:
		default:
			close(first.release)
		}
		select {
		case <-second.release:
		default:
			close(second.release)
		}
		registry.ServiceRegistry().Unregister(firstName)
		registry.ServiceRegistry().Unregister(secondName)
	})
	if err := registry.ServiceRegistry().Register(firstName, coreservice.Service(first)); err != nil {
		t.Fatalf("register first service: %v", err)
	}
	if err := registry.ServiceRegistry().Register(secondName, coreservice.Service(second)); err != nil {
		t.Fatalf("register second service: %v", err)
	}
	config.Set(&config.Config{Services: []*config.ServiceConfig{{Name: firstName}, {Name: secondName}}})

	reporter := NewWebSocketReporter("", "mutation-queue-test-secret")
	go reporter.runMutationCommands()
	t.Cleanup(reporter.Stop)
	reporter.dispatchCommand(CommandMessage{Type: "DeleteService", Data: map[string]any{"services": []string{firstName}}})
	reporter.dispatchCommand(CommandMessage{Type: "DeleteService", Data: map[string]any{"services": []string{secondName}}})

	select {
	case <-first.started:
	case <-time.After(time.Second):
		t.Fatal("first mutation did not start")
	}
	select {
	case <-second.started:
		t.Fatal("second mutation started before first mutation completed")
	case <-time.After(100 * time.Millisecond):
	}

	close(first.release)
	select {
	case <-second.started:
	case <-time.After(time.Second):
		t.Fatal("second mutation did not start after first mutation completed")
	}
	close(second.release)
}
