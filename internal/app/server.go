package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/config"
	"github.com/Jopoleon/invest-control-bot/internal/store"
	"github.com/go-co-op/gocron/v2"
	"github.com/oklog/run"
)

// Server owns HTTP server with admin endpoints, Telegram webhook and payment routes.
type Server struct {
	httpServer          *http.Server
	lifecycleScheduler  gocron.Scheduler
	lifecycleRunOnStart func()
}

// New builds fully wired HTTP server with current dependencies.
func New(cfg config.Config, st store.Store) (*Server, error) {
	appCtx, err := newApplication(cfg, st, appInitOptions{ensureTelegramSetup: true, ensureMAXSetup: true, checkTransportHealth: true})
	if err != nil {
		return nil, err
	}

	router := appCtx.newRouter()
	httpServer := &http.Server{
		Addr:         cfg.HTTP.Address,
		Handler:      router,
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
	var group run.Group

	if s.lifecycleScheduler != nil {
		schedulerStop := make(chan struct{})
		group.Add(func() error {
			// The scheduler actor only owns background work. HTTP shutdown stays in the
			// shared server shutdown path so every actor tears down through one API.
			s.lifecycleRunOnStart()
			s.lifecycleScheduler.Start()
			<-schedulerStop
			return nil
		}, func(error) {
			close(schedulerStop)
		})
	}

	group.Add(func() error {
		err := s.httpServer.ListenAndServe()
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}, func(error) {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := s.Shutdown(shutdownCtx); err != nil && !errors.Is(err, context.Canceled) {
			slog.Error("server shutdown failed", "error", err)
		}
	})

	group.Add(func() error {
		<-ctx.Done()
		return nil
	}, func(error) {})

	return group.Run()
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
