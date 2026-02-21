package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"mcp-panel/internal/audit"
	"mcp-panel/internal/config"
	"mcp-panel/internal/panelclient"
	"mcp-panel/internal/policy"
	"mcp-panel/internal/tools"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	cfg, err := config.FromEnv()
	if err != nil {
		log.Fatalf("invalid config: %v", err)
	}

	server := mcp.NewServer(&mcp.Implementation{
		Name:    cfg.ServerName,
		Version: cfg.ServerVersion,
	}, nil)

	panel := panelclient.New(cfg.PanelBaseURL)
	confirmPolicy := policy.NewConfirmPolicy(cfg.ConfirmToken)
	idempotencyStore := policy.NewIdempotencyStore(cfg.IdempotencyTTL)
	auditLogger := audit.NewLogger(cfg.AuditEnabled)
	tools.NewRegistry(panel, confirmPolicy, idempotencyStore, auditLogger).Register(server)

	switch cfg.MCPTransport {
	case "stdio":
		runStdio(server)
	case "http":
		runHTTP(server, cfg)
	}
}

func runStdio(server *mcp.Server) {
	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatalf("mcp stdio server exited: %v", err)
	}
}

func runHTTP(server *mcp.Server, cfg config.Config) {
	mcpHandler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		return server
	}, nil)

	mux := http.NewServeMux()
	mux.Handle("/mcp", mcpHandler)
	mux.HandleFunc(cfg.HealthPath, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write([]byte(`{"ok":true}`))
	})

	httpServer := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("mcp http listening on %s", cfg.HTTPAddr)
		errCh <- httpServer.ListenAndServe()
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigCh:
		log.Printf("received signal %s, shutting down", sig)
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("mcp http server failed: %v", err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		log.Fatalf("mcp http shutdown failed: %v", err)
	}
}
