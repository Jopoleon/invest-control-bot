package app

import (
	"context"
	"fmt"
	"net/http"

	"github.com/Jopoleon/invest-control-bot/internal/config"
	"github.com/Jopoleon/invest-control-bot/internal/store"
)

// NewHTTPHandler builds the full HTTP handler tree without starting a TCP listener.
func NewHTTPHandler(cfg config.Config, st store.Store) (http.Handler, error) {
	appCtx, err := newApplication(cfg, st, appInitOptions{ensureTelegramSetup: true, ensureMAXSetup: true})
	if err != nil {
		return nil, err
	}
	return appCtx.newRouter(), nil
}

// RunLifecyclePassOnce executes one lifecycle pass without starting a background scheduler.
func RunLifecyclePassOnce(ctx context.Context, cfg config.Config, st store.Store) error {
	appCtx, err := newApplication(cfg, st, appInitOptions{ensureTelegramSetup: false, ensureMAXSetup: false})
	if err != nil {
		return fmt.Errorf("new application for lifecycle pass: %w", err)
	}
	runSubscriptionLifecyclePass(ctx, appCtx)
	return nil
}
