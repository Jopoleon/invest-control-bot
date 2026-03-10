package main

import (
	"log"

	"github.com/Jopoleon/telega-bot-fedor/internal/app"
	"github.com/Jopoleon/telega-bot-fedor/internal/config"
	"github.com/Jopoleon/telega-bot-fedor/internal/store/memory"
)

// main is the backend entrypoint used for local/dev runs and VPS deployments.
func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	log.Printf("config loaded: %+v", cfg)

	st := memory.New()
	srv, err := app.New(cfg, st)
	if err != nil {
		log.Fatalf("create app server: %v", err)
	}

	log.Printf("service=%s env=%s http_addr=%s", cfg.AppName, cfg.Environment, cfg.HTTP.Address)
	if err := srv.Run(); err != nil {
		log.Fatalf("server stopped: %v", err)
	}
}
