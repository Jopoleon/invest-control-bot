package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/config"
	"github.com/Jopoleon/invest-control-bot/internal/payment"
)

func main() {
	os.Exit(runOpStateMain(os.Args[1:], defaultOpStateDeps()))
}

type opStateLookupService interface {
	LookupOperationState(context.Context, string) (payment.RobokassaOpState, error)
}

type opStateDeps struct {
	loadConfig func() (config.Config, error)
	newService func(config.Config) opStateLookupService
	stdout     io.Writer
	stderr     io.Writer
}

func defaultOpStateDeps() opStateDeps {
	return opStateDeps{
		loadConfig: config.Load,
		newService: func(cfg config.Config) opStateLookupService {
			return payment.NewRobokassaService(payment.RobokassaConfig{
				MerchantLogin: cfg.Payment.Robokassa.MerchantLogin,
				Password1:     cfg.Payment.Robokassa.Password1,
				Password2:     cfg.Payment.Robokassa.Password2,
				IsTest:        cfg.Payment.Robokassa.IsTestMode,
				BaseURL:       cfg.Payment.Robokassa.CheckoutURL,
				RebillURL:     cfg.Payment.Robokassa.RebillURL,
			})
		},
		stdout: os.Stdout,
		stderr: os.Stderr,
	}
}

func runOpStateMain(args []string, deps opStateDeps) int {
	fs := flag.NewFlagSet("robokassa-opstate", flag.ContinueOnError)
	fs.SetOutput(deps.stderr)
	invoiceID := fs.String("invoice-id", "", "merchant-side Robokassa invoice id / InvId")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	if strings.TrimSpace(*invoiceID) == "" {
		fmt.Fprintln(deps.stderr, "invoice-id is required")
		return 1
	}

	cfg, err := deps.loadConfig()
	if err != nil {
		fmt.Fprintf(deps.stderr, "load config: %v\n", err)
		return 1
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	state, err := deps.newService(cfg).LookupOperationState(ctx, strings.TrimSpace(*invoiceID))
	if err != nil {
		fmt.Fprintf(deps.stderr, "lookup opstate: %v\n", err)
		return 1
	}

	enc := json.NewEncoder(deps.stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(state); err != nil {
		fmt.Fprintf(deps.stderr, "encode opstate: %v\n", err)
		return 1
	}
	return 0
}
