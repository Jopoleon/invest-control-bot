package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/config"
	"github.com/Jopoleon/invest-control-bot/internal/store"
	"github.com/go-co-op/gocron/v2"
)

// Server owns HTTP server with admin endpoints, Telegram webhook and payment routes.
type Server struct {
	httpServer          *http.Server
	lifecycleScheduler  gocron.Scheduler
	lifecycleRunOnStart func()
}

// New builds fully wired HTTP server with current dependencies.
func New(cfg config.Config, st store.Store) (*Server, error) {
	appCtx, err := newApplication(cfg, st)
	if err != nil {
		return nil, err
	}

	mux := appCtx.newMux()
	httpServer := &http.Server{
		Addr:         cfg.HTTP.Address,
		Handler:      loggingMiddleware(mux),
		ReadTimeout:  cfg.HTTP.ReadTimeout,
		WriteTimeout: cfg.HTTP.WriteTimeout,
	}
	lifecycleScheduler, err := newSubscriptionLifecycleScheduler(appCtx)
	if err != nil {
		return nil, fmt.Errorf("create subscription lifecycle scheduler: %w", err)
	}

	return &Server{
		httpServer:         httpServer,
		lifecycleScheduler: lifecycleScheduler,
		lifecycleRunOnStart: func() {
			runSubscriptionLifecyclePass(context.Background(), appCtx)
		},
	}, nil
}

// Run starts HTTP server and blocks until it stops.
func (s *Server) Run(ctx context.Context) error {
	if s.lifecycleScheduler != nil {
		s.lifecycleRunOnStart()
		s.lifecycleScheduler.Start()
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- s.httpServer.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := s.Shutdown(shutdownCtx); err != nil {
			return err
		}
		err := <-errCh
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

// Shutdown stops HTTP handling and background schedulers.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.lifecycleScheduler != nil {
		if err := s.lifecycleScheduler.Shutdown(); err != nil {
			return err
		}
	}
	return s.httpServer.Shutdown(ctx)
}
