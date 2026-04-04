package main

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/Jopoleon/invest-control-bot/internal/config"
	"github.com/Jopoleon/invest-control-bot/internal/store"
)

func TestRunServerMain_Success(t *testing.T) {
	var (
		cleanupCalled bool
		runCalled     bool
	)

	exitCode := runServerMain(serverDeps{
		initLogger: func(level, filePath string) (string, error) {
			return "info", nil
		},
		loadConfig: func() (config.Config, error) {
			cfg := config.Config{Runtime: config.RuntimeServer, AppName: "invest-control-bot"}
			cfg.HTTP.Address = ":8080"
			return cfg, nil
		},
		openStore: func(config.Config) (store.Store, func(), error) {
			return nil, func() { cleanupCalled = true }, nil
		},
		newServer: func(config.Config, store.Store) (serverRunner, error) {
			return serverRunnerFunc(func(context.Context) error {
				runCalled = true
				return nil
			}), nil
		},
		notifyContext: func(parent context.Context, _ ...os.Signal) (context.Context, context.CancelFunc) {
			return parent, func() {}
		},
	})

	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0", exitCode)
	}
	if !runCalled {
		t.Fatal("expected server run to be called")
	}
	if !cleanupCalled {
		t.Fatal("expected store cleanup to be called")
	}
}

func TestRunServerMain_InvalidRuntime(t *testing.T) {
	exitCode := runServerMain(serverDeps{
		initLogger: func(level, filePath string) (string, error) { return "info", nil },
		loadConfig: func() (config.Config, error) {
			return config.Config{Runtime: config.RuntimeVercel}, nil
		},
	})

	if exitCode != 1 {
		t.Fatalf("exitCode = %d, want 1", exitCode)
	}
}

func TestRunServerMain_RunError(t *testing.T) {
	exitCode := runServerMain(serverDeps{
		initLogger: func(level, filePath string) (string, error) { return "info", nil },
		loadConfig: func() (config.Config, error) {
			cfg := config.Config{Runtime: config.RuntimeServer}
			return cfg, nil
		},
		openStore: func(config.Config) (store.Store, func(), error) {
			return nil, func() {}, nil
		},
		newServer: func(config.Config, store.Store) (serverRunner, error) {
			return serverRunnerFunc(func(context.Context) error {
				return errors.New("boom")
			}), nil
		},
		notifyContext: func(parent context.Context, _ ...os.Signal) (context.Context, context.CancelFunc) {
			return parent, func() {}
		},
	})

	if exitCode != 1 {
		t.Fatalf("exitCode = %d, want 1", exitCode)
	}
}

type serverRunnerFunc func(context.Context) error

func (fn serverRunnerFunc) Run(ctx context.Context) error {
	return fn(ctx)
}
