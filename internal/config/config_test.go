package config

import (
	"strings"
	"testing"
	"time"
)

func TestValidate_LocalAcceptsMinimalConfig(t *testing.T) {
	cfg := Config{
		Environment: EnvLocal,
		Runtime:     RuntimeServer,
		HTTP:        HTTPConfig{Address: ":8080"},
		Payment:     PaymentConfig{Provider: "mock"},
		Logging:     LoggingConfig{Level: "info"},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestValidate_ProdRequiresCriticalFields(t *testing.T) {
	cfg := Config{
		Environment: EnvProd,
		Runtime:     RuntimeVercel,
		HTTP:        HTTPConfig{},
		Postgres:    PostgresConfig{Driver: "postgres"},
		Payment: PaymentConfig{
			Provider: "robokassa",
			Robokassa: RobokassaPaymentConfig{
				MerchantLogin: "",
			},
		},
		Logging:  LoggingConfig{Level: "verbose"},
		Security: SecurityConfig{EncryptionKey: "short"},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatalf("Validate err=nil want aggregated error")
	}
	msg := err.Error()
	for _, want := range []string{
		"HTTP_ADDR is required",
		"APP_ENCRYPTION_KEY must be at least 32 chars",
		"ROBOKASSA_MERCHANT_LOGIN is required",
		"LOG_LEVEL must be one of",
		"TELEGRAM_BOT_TOKEN is required",
		"ADMIN_AUTH_TOKEN is required",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("Validate error=%q missing %q", msg, want)
		}
	}
}

func TestEnvHelpersAndBuildPostgresDSN(t *testing.T) {
	t.Setenv("CFG_TEST_STR", " value ")
	t.Setenv("CFG_TEST_DUR", "90s")
	t.Setenv("CFG_TEST_INT", "17")
	t.Setenv("CFG_TEST_BOOL", "true")
	t.Setenv("CFG_TEST_CSV", " one, two ,, three ")

	if got := getEnv("CFG_TEST_STR", "fallback"); got != "value" {
		t.Fatalf("getEnv=%q want value", got)
	}
	if got := getDurationEnv("CFG_TEST_DUR", time.Second); got != 90*time.Second {
		t.Fatalf("getDurationEnv=%s want 90s", got)
	}
	if got := getIntEnv("CFG_TEST_INT", 1); got != 17 {
		t.Fatalf("getIntEnv=%d want 17", got)
	}
	if got := getBoolEnv("CFG_TEST_BOOL", false); !got {
		t.Fatalf("getBoolEnv=false want true")
	}
	if got := getCSVEnv("CFG_TEST_CSV", []string{"fallback"}); strings.Join(got, ",") != "one,two,three" {
		t.Fatalf("getCSVEnv=%v want [one two three]", got)
	}

	dsn := buildPostgresDSN(PostgresConfig{
		Driver:   "postgres",
		Host:     "db.internal",
		Port:     6543,
		Username: "invest",
		Password: "secret",
		Database: "billing",
		SSLMode:  "require",
	})
	want := "postgres://invest:secret@db.internal:6543/billing?sslmode=require"
	if dsn != want {
		t.Fatalf("buildPostgresDSN=%q want %q", dsn, want)
	}
}

func TestLoad_RobokassaReceiptEnvDefaultsToZeroVAT(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("PAYMENT_PROVIDER", "robokassa")
	t.Setenv("ROBOKASSA_MERCHANT_LOGIN", "merchant")
	t.Setenv("ROBOKASSA_PASS1", "pass1")
	t.Setenv("ROBOKASSA_PASS2", "pass2")
	t.Setenv("APP_ENCRYPTION_KEY", strings.Repeat("x", 32))
	t.Setenv("TELEGRAM_BOT_TOKEN", "telegram-token")
	t.Setenv("TELEGRAM_WEBHOOK_PUBLIC_URL", "https://example.com/telegram/webhook")
	t.Setenv("ADMIN_AUTH_TOKEN", "admin-token")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Payment.Robokassa.ReceiptTax != "none" {
		t.Fatalf("ReceiptTax=%q want none", cfg.Payment.Robokassa.ReceiptTax)
	}
	if cfg.Payment.Robokassa.ReceiptMethod != "full_payment" {
		t.Fatalf("ReceiptMethod=%q want full_payment", cfg.Payment.Robokassa.ReceiptMethod)
	}
	if cfg.Payment.Robokassa.ReceiptObject != "service" {
		t.Fatalf("ReceiptObject=%q want service", cfg.Payment.Robokassa.ReceiptObject)
	}
}

func TestLoad_RobokassaReceiptEnvOverrides(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("PAYMENT_PROVIDER", "robokassa")
	t.Setenv("ROBOKASSA_MERCHANT_LOGIN", "merchant")
	t.Setenv("ROBOKASSA_PASS1", "pass1")
	t.Setenv("ROBOKASSA_PASS2", "pass2")
	t.Setenv("ROBOKASSA_RECEIPT_TAX", "vat0")
	t.Setenv("ROBOKASSA_RECEIPT_PAYMENT_METHOD", "full_prepayment")
	t.Setenv("ROBOKASSA_RECEIPT_PAYMENT_OBJECT", "service")
	t.Setenv("ROBOKASSA_RECEIPT_SNO", "usn_income")
	t.Setenv("ROBOKASSA_RECEIPT_ITEM_NAME", "Абонемент")
	t.Setenv("APP_ENCRYPTION_KEY", strings.Repeat("x", 32))
	t.Setenv("TELEGRAM_BOT_TOKEN", "telegram-token")
	t.Setenv("TELEGRAM_WEBHOOK_PUBLIC_URL", "https://example.com/telegram/webhook")
	t.Setenv("ADMIN_AUTH_TOKEN", "admin-token")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Payment.Robokassa.ReceiptTax != "vat0" {
		t.Fatalf("ReceiptTax=%q want vat0", cfg.Payment.Robokassa.ReceiptTax)
	}
	if cfg.Payment.Robokassa.ReceiptMethod != "full_prepayment" {
		t.Fatalf("ReceiptMethod=%q want full_prepayment", cfg.Payment.Robokassa.ReceiptMethod)
	}
	if cfg.Payment.Robokassa.ReceiptObject != "service" {
		t.Fatalf("ReceiptObject=%q want service", cfg.Payment.Robokassa.ReceiptObject)
	}
	if cfg.Payment.Robokassa.ReceiptSNO != "usn_income" {
		t.Fatalf("ReceiptSNO=%q want usn_income", cfg.Payment.Robokassa.ReceiptSNO)
	}
	if cfg.Payment.Robokassa.ReceiptItemName != "Абонемент" {
		t.Fatalf("ReceiptItemName=%q want Абонемент", cfg.Payment.Robokassa.ReceiptItemName)
	}
}

func TestValidate_AcceptsOptionalTelegramRelayURLs(t *testing.T) {
	cfg := Config{
		Environment: EnvLocal,
		Runtime:     RuntimeServer,
		HTTP:        HTTPConfig{Address: ":8080"},
		Payment:     PaymentConfig{Provider: "mock"},
		Logging:     LoggingConfig{Level: "info"},
		Telegram: TelegramConfig{
			APIBaseURL:   "https://telegram-relay.example.com",
			HTTPProxyURL: "http://proxy.example.com:8080",
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestValidate_RejectsInvalidTelegramRelayURLs(t *testing.T) {
	cfg := Config{
		Environment: EnvLocal,
		Runtime:     RuntimeServer,
		HTTP:        HTTPConfig{Address: ":8080"},
		Payment:     PaymentConfig{Provider: "mock"},
		Logging:     LoggingConfig{Level: "info"},
		Telegram: TelegramConfig{
			APIBaseURL:   "://bad-base",
			HTTPProxyURL: "bad-proxy",
		},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected invalid relay url validation error")
	}
	msg := err.Error()
	for _, want := range []string{
		"TELEGRAM_API_BASE_URL must be a valid absolute URL",
		"TELEGRAM_HTTP_PROXY_URL must be a valid absolute proxy URL",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("Validate error=%q missing %q", msg, want)
		}
	}
}
