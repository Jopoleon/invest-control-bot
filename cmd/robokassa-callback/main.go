package main

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/app"
	"github.com/Jopoleon/invest-control-bot/internal/config"
	"github.com/Jopoleon/invest-control-bot/internal/logger"
)

const defaultBaseURL = "http://localhost:8080"

// main sends synthetic Robokassa callbacks/redirects to local app endpoints.
func main() {
	mode := flag.String("mode", "result", "callback mode: result, success, fail")
	invoiceID := flag.String("invoice-id", "", "merchant-side Robokassa InvoiceID / InvId")
	amountRUB := flag.Int64("amount-rub", 0, "payment amount in RUB; if omitted, script tries to load it from DB by invoice-id")
	baseURL := flag.String("base-url", defaultBaseURL, "application base URL, e.g. http://localhost:8080")
	flag.Parse()

	if _, err := logger.Init("info", ""); err != nil {
		slog.Error("bootstrap logger init failed", "error", err)
		os.Exit(1)
	}

	if strings.TrimSpace(*invoiceID) == "" {
		slog.Error("invoice-id is required")
		os.Exit(1)
	}
	if !isSupportedMode(*mode) {
		slog.Error("unsupported mode", "mode", *mode)
		os.Exit(1)
	}

	cfg, err := config.Load()
	if err != nil {
		slog.Error("load config failed", "error", err)
		os.Exit(1)
	}

	effectiveLevel, err := logger.Init(cfg.Logging.Level, cfg.Logging.FilePath)
	if err != nil {
		slog.Error("logger init with file failed", "error", err, "file_path", cfg.Logging.FilePath)
		os.Exit(1)
	}
	slog.Info("config loaded", "effective_log_level", effectiveLevel, "log_file_path", cfg.Logging.FilePath)

	amount := *amountRUB
	if requiresAmount(*mode) && amount <= 0 {
		var lookupErr error
		amount, lookupErr = resolveAmountFromStore(cfg, strings.TrimSpace(*invoiceID))
		if lookupErr != nil {
			slog.Error("resolve payment amount failed", "invoice_id", *invoiceID, "error", lookupErr)
			os.Exit(1)
		}
	}

	endpoint, form := buildRequest(cfg, strings.TrimSpace(*mode), strings.TrimSpace(*invoiceID), amount)
	if err := sendRequest(strings.TrimSpace(*baseURL)+endpoint, form); err != nil {
		slog.Error("send robokassa callback failed", "mode", *mode, "invoice_id", *invoiceID, "error", err)
		os.Exit(1)
	}
}

func isSupportedMode(mode string) bool {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "result", "success", "fail":
		return true
	default:
		return false
	}
}

func requiresAmount(mode string) bool {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "result", "success":
		return true
	default:
		return false
	}
}

func resolveAmountFromStore(cfg config.Config, invoiceID string) (int64, error) {
	st, cleanup, err := app.OpenStore(cfg)
	if err != nil {
		return 0, fmt.Errorf("open store: %w", err)
	}
	defer cleanup()

	paymentRow, found, err := st.GetPaymentByToken(context.Background(), invoiceID)
	if err != nil {
		return 0, fmt.Errorf("get payment by token: %w", err)
	}
	if !found {
		return 0, fmt.Errorf("payment with token=%s not found", invoiceID)
	}
	if paymentRow.Provider != "" && paymentRow.Provider != "robokassa" {
		slog.Warn("payment provider is not robokassa", "provider", paymentRow.Provider, "invoice_id", invoiceID)
	}
	if paymentRow.AmountRUB <= 0 {
		return 0, fmt.Errorf("payment amount_rub is empty for token=%s", invoiceID)
	}
	return paymentRow.AmountRUB, nil
}

func buildRequest(cfg config.Config, mode, invoiceID string, amountRUB int64) (string, url.Values) {
	mode = strings.ToLower(strings.TrimSpace(mode))
	form := url.Values{}
	form.Set("InvId", invoiceID)

	switch mode {
	case "result":
		outSum := formatOutSum(amountRUB)
		form.Set("OutSum", outSum)
		// ResultURL uses merchant formula OutSum:InvId:Password#2.
		form.Set("SignatureValue", md5Hex(strings.Join([]string{
			outSum,
			invoiceID,
			cfg.Payment.Robokassa.Password2,
		}, ":")))
		return "/payment/result", form
	case "success":
		outSum := formatOutSum(amountRUB)
		form.Set("OutSum", outSum)
		form.Set("SignatureValue", md5Hex(strings.Join([]string{
			outSum,
			invoiceID,
			cfg.Payment.Robokassa.Password1,
		}, ":")))
		return "/payment/success", form
	default:
		return "/payment/fail", form
	}
}

func sendRequest(target string, form url.Values) error {
	body := strings.NewReader(form.Encode())
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, target, body)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("perform request: %w", err)
	}
	defer resp.Body.Close()

	payload, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	slog.Info("robokassa callback sent",
		"status_code", resp.StatusCode,
		"response_body", strings.TrimSpace(string(payload)),
		"url", target,
	)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	return nil
}

func formatOutSum(amountRUB int64) string {
	if amountRUB < 0 {
		amountRUB = 0
	}
	return fmt.Sprintf("%d.00", amountRUB)
}

func md5Hex(raw string) string {
	sum := md5.Sum([]byte(raw))
	return strings.ToLower(hex.EncodeToString(sum[:]))
}
