package config

import (
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

const (
	// Supported runtime environments.
	EnvLocal = "local"
	EnvStage = "stage"
	EnvProd  = "prod"
)

// Config is the root runtime configuration object.
type Config struct {
	AppName     string
	Environment string

	HTTP HTTPConfig

	Postgres PostgresConfig
	Telegram TelegramConfig
	Yookassa YookassaConfig
	Payment  PaymentConfig

	Security SecurityConfig
}

// HTTPConfig stores HTTP server-level options.
type HTTPConfig struct {
	Address      string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

// PostgresConfig stores database connectivity settings.
type PostgresConfig struct {
	DSN string
}

// TelegramConfig stores bot credentials and webhook settings.
type TelegramConfig struct {
	BotToken    string
	BotUsername string
	Webhook     WebhookConfig
}

// WebhookConfig stores Telegram webhook metadata.
type WebhookConfig struct {
	SecretToken string
	PublicURL   string
}

// YookassaConfig stores YooKassa credentials.
type YookassaConfig struct {
	ShopID        string
	SecretKey     string
	WebhookSecret string
}

// PaymentConfig controls currently selected payment provider mode.
type PaymentConfig struct {
	Provider    string
	MockBaseURL string
}

// SecurityConfig stores application-level secrets.
type SecurityConfig struct {
	EncryptionKey string
	AdminToken    string
}

// Load reads .env (if present), applies environment variables and validates final config.
func Load() (Config, error) {
	if _, err := os.Stat(".env"); err == nil {
		if err := godotenv.Load(".env"); err != nil {
			log.Printf("config: .env found but failed to load: %v", err)
		} else {
			log.Printf("config: .env found and loaded")
		}
	} else if errors.Is(err, os.ErrNotExist) {
		log.Printf("config: .env not found, using process environment only")
	} else {
		log.Printf("config: failed to stat .env: %v", err)
	}

	cfg := Config{
		AppName:     getEnv("APP_NAME", "telega-bot-fedor"),
		Environment: strings.ToLower(getEnv("APP_ENV", EnvLocal)),
		HTTP: HTTPConfig{
			Address:      getEnv("HTTP_ADDR", ":8080"),
			ReadTimeout:  getDurationEnv("HTTP_READ_TIMEOUT", 10*time.Second),
			WriteTimeout: getDurationEnv("HTTP_WRITE_TIMEOUT", 15*time.Second),
		},
		Postgres: PostgresConfig{DSN: os.Getenv("POSTGRES_DSN")},
		Telegram: TelegramConfig{
			BotToken:    os.Getenv("TELEGRAM_BOT_TOKEN"),
			BotUsername: os.Getenv("TELEGRAM_BOT_USERNAME"),
			Webhook: WebhookConfig{
				SecretToken: os.Getenv("TELEGRAM_WEBHOOK_SECRET"),
				PublicURL:   os.Getenv("TELEGRAM_WEBHOOK_PUBLIC_URL"),
			},
		},
		Yookassa: YookassaConfig{
			ShopID:        os.Getenv("YOOKASSA_SHOP_ID"),
			SecretKey:     os.Getenv("YOOKASSA_SECRET_KEY"),
			WebhookSecret: os.Getenv("YOOKASSA_WEBHOOK_SECRET"),
		},
		Payment: PaymentConfig{
			Provider:    strings.ToLower(getEnv("PAYMENT_PROVIDER", "mock")),
			MockBaseURL: strings.TrimSpace(os.Getenv("PAYMENT_MOCK_BASE_URL")),
		},
		Security: SecurityConfig{
			EncryptionKey: os.Getenv("APP_ENCRYPTION_KEY"),
			AdminToken:    os.Getenv("ADMIN_AUTH_TOKEN"),
		},
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

// Validate performs minimal semantic checks before application startup.
func (c Config) Validate() error {
	var errs []string

	switch c.Environment {
	case EnvLocal, EnvStage, EnvProd:
	default:
		errs = append(errs, "APP_ENV must be one of: local, stage, prod")
	}

	if c.HTTP.Address == "" {
		errs = append(errs, "HTTP_ADDR is required")
	}

	if c.Security.EncryptionKey != "" && len(c.Security.EncryptionKey) < 32 {
		errs = append(errs, "APP_ENCRYPTION_KEY must be at least 32 chars")
	}
	if c.Payment.Provider == "" {
		errs = append(errs, "PAYMENT_PROVIDER is required")
	}

	if c.Environment != EnvLocal {
		if c.Postgres.DSN == "" {
			errs = append(errs, "POSTGRES_DSN is required for non-local environments")
		}
		if c.Telegram.BotToken == "" {
			errs = append(errs, "TELEGRAM_BOT_TOKEN is required for non-local environments")
		}
		if c.Yookassa.ShopID == "" || c.Yookassa.SecretKey == "" {
			errs = append(errs, "YOOKASSA_SHOP_ID and YOOKASSA_SECRET_KEY are required for non-local environments")
		}
		if c.Security.EncryptionKey == "" {
			errs = append(errs, "APP_ENCRYPTION_KEY is required for non-local environments")
		}
		if c.Security.AdminToken == "" {
			errs = append(errs, "ADMIN_AUTH_TOKEN is required for non-local environments")
		}
	}

	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}

	return nil
}

// getEnv reads a key or returns fallback when key is empty.
func getEnv(key, fallback string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	return v
}

// getDurationEnv parses duration keys and falls back on parse errors.
func getDurationEnv(key string, fallback time.Duration) time.Duration {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		fmt.Printf("invalid duration for %s=%q, fallback to %s\n", key, v, fallback)
		return fallback
	}
	return d
}
