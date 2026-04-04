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
