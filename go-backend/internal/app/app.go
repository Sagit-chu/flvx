package app

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go-backend/internal/config"
	httpserver "go-backend/internal/http"
	"go-backend/internal/http/handler"
	"go-backend/internal/store/sqlite"
)

type App struct {
	cfg    config.Config
	server *http.Server
	repo   *sqlite.Repository
	h      *handler.Handler
}

func New(cfg config.Config) (*App, error) {
	var (
		repo *sqlite.Repository
		err  error
	)

	switch strings.ToLower(strings.TrimSpace(cfg.DBType)) {
	case "", "sqlite":
		repo, err = sqlite.Open(cfg.DBPath)
		if err != nil {
			return nil, fmt.Errorf("open sqlite: %w", err)
		}
	case "postgres", "postgresql":
		repo, err = sqlite.OpenPostgres(cfg.DatabaseURL)
		if err != nil {
			return nil, fmt.Errorf("open postgres: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported DB_TYPE %q", cfg.DBType)
	}

	h := handler.New(repo, cfg.JWTSecret)
	router := httpserver.NewRouter(h, cfg.JWTSecret)

	s := &http.Server{
		Addr:              cfg.Addr,
		Handler:           router,
		ReadTimeout:       30 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	return &App{cfg: cfg, server: s, repo: repo, h: h}, nil
}

func (a *App) Run() error {
	if a.h != nil {
		a.h.StartBackgroundJobs()
	}
	return a.server.ListenAndServe()
}

func (a *App) Shutdown(ctx context.Context) error {
	if a.h != nil {
		a.h.StopBackgroundJobs()
	}
	shutdownErr := a.server.Shutdown(ctx)
	closeErr := a.repo.Close()
	if shutdownErr != nil {
		return shutdownErr
	}
	return closeErr
}
