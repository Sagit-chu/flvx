package loader_test

import (
	"net"
	"testing"
	"time"

	"github.com/go-gost/x/config"
	"github.com/go-gost/x/config/loader"
	_ "github.com/go-gost/x/handler/auto"
	_ "github.com/go-gost/x/listener/tcp"
	"github.com/go-gost/x/registry"
)

func TestReloadRestoresPreviousRuntime(t *testing.T) {
	for _, failure := range []string{"listener", "handler", "partial", "run"} {
		t.Run(failure, func(t *testing.T) {
			unlock := config.LockMutation()
			defer unlock()
			original := config.Global()
			defer config.Set(original)
			defer func() {
				for name := range registry.ServiceRegistry().GetAll() {
					registry.ServiceRegistry().Unregister(name)
				}
			}()
			old := &config.Config{Services: []*config.ServiceConfig{
				{Name: "shared-rule", Addr: "127.0.0.1:0", Handler: &config.HandlerConfig{Type: "auto"}, Listener: &config.ListenerConfig{Type: "tcp"}},
				{Name: "paused-rule", Addr: "127.0.0.1:0", Metadata: map[string]any{"paused": true}},
			}}
			if err := loader.Load(old); err != nil {
				t.Fatal(err)
			}
			addr := registry.ServiceRegistry().Get("shared-rule").Addr().String()
			old.Services[0].Addr = addr
			config.Set(old)
			serve := func(*config.Config) error {
				for _, svc := range registry.ServiceRegistry().GetAll() {
					go svc.Serve()
				}
				return nil
			}
			serve(old)
			replacement := config.Global()
			replacement.Services = replacement.Services[:1]
			switch failure {
			case "listener":
				replacement.Services[0].Listener.Type = "invalid-listener"
			case "handler":
				replacement.Services[0].Handler.Type = "invalid-handler"
			case "partial":
				replacement.Services = append(replacement.Services, &config.ServiceConfig{Name: "broken", Listener: &config.ListenerConfig{Type: "invalid-listener"}})
			}
			run := serve
			if failure == "run" {
				run = func(cfg *config.Config) error {
					if cfg == replacement {
						return &net.AddrError{Err: "auxiliary listener failed", Addr: "test"}
					}
					return serve(cfg)
				}
			}
			if err := loader.Reload(replacement, run); err == nil {
				t.Fatal("expected reload failure")
			}
			restored := registry.ServiceRegistry().Get("shared-rule")
			if restored == nil || restored.Addr().String() != addr {
				t.Fatal("previous service was not restored on its original port")
			}
			if registry.ServiceRegistry().Get("paused-rule") != nil {
				t.Fatal("rollback resumed a paused service")
			}
			conn, err := net.DialTimeout("tcp", addr, time.Second)
			if err != nil {
				t.Fatalf("restored listener unavailable: %v", err)
			}
			conn.Close()
			got := config.Global()
			if len(got.Services) != 2 || got.Services[0].Listener.Type != "tcp" || got.Services[0].Handler.Type != "auto" {
				t.Fatalf("failed config was committed: %+v", got.Services)
			}
		})
	}
}
