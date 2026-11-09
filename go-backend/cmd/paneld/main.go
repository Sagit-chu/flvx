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

	"go-backend/internal/app"
	"go-backend/internal/config"
	"go-backend/internal/panelupgrade"
)

func main() {
	if len(os.Args) == 3 && os.Args[1] == "panel-upgrade-helper" {
		if panelupgrade.RunHelper(os.Args[2]) != nil {
			log.Print("panel upgrade helper stopped; inspect durable job status")
			os.Exit(1)
		}
		return
	}
	cfg := config.FromEnv()
	if cfg.JWTSecret == "" {
		log.Println("warning: JWT_SECRET is empty")
	}
	log.Printf("starting go-backend on %s (db=%s)", cfg.Addr, cfg.DBPath)

	a, err := app.New(cfg)
	if err != nil {
		log.Fatalf("failed to create app: %v", err)
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- a.Run()
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigCh:
		log.Printf("received signal %s, shutting down", sig)
	case runErr := <-errCh:
		if runErr != nil && !errors.Is(runErr, http.ErrServerClosed) {
			log.Fatalf("server stopped unexpectedly: %v", runErr)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := a.Shutdown(ctx); err != nil {
		log.Fatalf("shutdown failed: %v", err)
	}
}
