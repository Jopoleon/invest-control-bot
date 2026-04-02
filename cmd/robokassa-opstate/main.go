package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/config"
	"github.com/Jopoleon/invest-control-bot/internal/payment"
)

func main() {
	invoiceID := flag.String("invoice-id", "", "merchant-side Robokassa invoice id / InvId")
	flag.Parse()

	if strings.TrimSpace(*invoiceID) == "" {
		fmt.Fprintln(os.Stderr, "invoice-id is required")
		os.Exit(1)
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}

	service := payment.NewRobokassaService(payment.RobokassaConfig{
		MerchantLogin: cfg.Payment.Robokassa.MerchantLogin,
		Password1:     cfg.Payment.Robokassa.Password1,
		Password2:     cfg.Payment.Robokassa.Password2,
		IsTest:        cfg.Payment.Robokassa.IsTestMode,
		BaseURL:       cfg.Payment.Robokassa.CheckoutURL,
		RebillURL:     cfg.Payment.Robokassa.RebillURL,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	state, err := service.LookupOperationState(ctx, strings.TrimSpace(*invoiceID))
	if err != nil {
		fmt.Fprintf(os.Stderr, "lookup opstate: %v\n", err)
		os.Exit(1)
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(state)
}
