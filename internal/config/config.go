package config

import (
	"errors"
	"log/slog"
	"net/url"
	"os"
	"strconv"
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
	Logging  LoggingConfig

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
	Driver        string
	Host          string
	Port          int
	Username      string
	Password      string
	Database      string
	Charset       string
	Collation     string
	SSLMode       string
	WithMigration bool
	DSN           string
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
	Robokassa   RobokassaPaymentConfig
}

// RobokassaPaymentConfig stores Robokassa credentials and mode flags.
type RobokassaPaymentConfig struct {
	MerchantLogin string
	Password1     string
	Password2     string
	IsTestMode    bool
	CheckoutURL   string
}

// LoggingConfig controls verbosity of structured logs.
type LoggingConfig struct {
	Level    string
	FilePath string
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
			slog.Warn("config .env found but failed to load", "error", err)
		} else {
			slog.Info("config .env found and loaded")
		}
	} else if errors.Is(err, os.ErrNotExist) {
		slog.Info("config .env not found, using process environment only")
	} else {
		slog.Warn("config failed to stat .env", "error", err)
	}

	cfg := Config{
		AppName:     getEnv("APP_NAME", "telega-bot-fedor"),
		Environment: strings.ToLower(getEnv("APP_ENV", EnvLocal)),
		HTTP: HTTPConfig{
			Address:      getEnv("HTTP_ADDR", ":8080"),
			ReadTimeout:  getDurationEnv("HTTP_READ_TIMEOUT", 10*time.Second),
			WriteTimeout: getDurationEnv("HTTP_WRITE_TIMEOUT", 15*time.Second),
		},
		Postgres: PostgresConfig{
			Driver:        strings.ToLower(getEnv("DB_DRIVER", "postgres")),
			Host:          getEnv("DB_HOST", "localhost"),
			Port:          getIntEnv("DB_PORT", 5432),
			Username:      getEnv("DB_USERNAME", "postgres"),
			Password:      os.Getenv("DB_PASSWORD"),
			Database:      getEnv("DB_DATABASE", "telega_bot_fedor"),
			Charset:       getEnv("DB_CHARSET", "utf8"),
			Collation:     getEnv("DB_COLLATION", "utf8_unicode_ci"),
			SSLMode:       getEnv("DB_SSL", "disable"),
			WithMigration: getBoolEnv("DB_WITH_MIGRATION", true),
		},
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
			Robokassa: RobokassaPaymentConfig{
				MerchantLogin: strings.TrimSpace(os.Getenv("ROBOKASSA_MERCHANT_LOGIN")),
				Password1:     strings.TrimSpace(os.Getenv("ROBOKASSA_PASS1")),
				Password2:     strings.TrimSpace(os.Getenv("ROBOKASSA_PASS2")),
				IsTestMode:    getBoolEnv("ROBOKASSA_IS_TEST_MODE", true),
				CheckoutURL:   strings.TrimSpace(os.Getenv("ROBOKASSA_CHECKOUT_URL")),
			},
		},
		Logging: LoggingConfig{
			Level:    strings.ToLower(getEnv("LOG_LEVEL", "info")),
			FilePath: strings.TrimSpace(getEnv("LOG_FILE_PATH", "logs/app.log")),
		},
		Security: SecurityConfig{
			EncryptionKey: os.Getenv("APP_ENCRYPTION_KEY"),
			AdminToken:    os.Getenv("ADMIN_AUTH_TOKEN"),
		},
	}
	cfg.Postgres.DSN = buildPostgresDSN(cfg.Postgres)

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
	if c.Payment.Provider == "robokassa" {
		if c.Payment.Robokassa.MerchantLogin == "" {
			errs = append(errs, "ROBOKASSA_MERCHANT_LOGIN is required when PAYMENT_PROVIDER=robokassa")
		}
		if c.Payment.Robokassa.Password1 == "" {
			errs = append(errs, "ROBOKASSA_PASS1 is required when PAYMENT_PROVIDER=robokassa")
		}
		if c.Payment.Robokassa.Password2 == "" {
			errs = append(errs, "ROBOKASSA_PASS2 is required when PAYMENT_PROVIDER=robokassa")
		}
	}
	switch c.Logging.Level {
	case "debug", "info", "warn", "error":
	default:
		errs = append(errs, "LOG_LEVEL must be one of: debug, info, warn, error")
	}

	if c.Environment != EnvLocal {
		if c.Postgres.Driver == "" {
			errs = append(errs, "DB_DRIVER is required for non-local environments")
		}
		if c.Postgres.Host == "" {
			errs = append(errs, "DB_HOST is required for non-local environments")
		}
		if c.Postgres.Port <= 0 {
			errs = append(errs, "DB_PORT must be > 0 for non-local environments")
		}
		if c.Postgres.Username == "" {
			errs = append(errs, "DB_USERNAME is required for non-local environments")
		}
		if c.Postgres.Database == "" {
			errs = append(errs, "DB_DATABASE is required for non-local environments")
		}
		if c.Postgres.Driver == "postgres" && c.Postgres.SSLMode == "" {
			errs = append(errs, "DB_SSL is required for postgres non-local environments")
		}
		if c.Postgres.DSN == "" {
			errs = append(errs, "constructed postgres DSN is empty")
		}
		if c.Telegram.BotToken == "" {
			errs = append(errs, "TELEGRAM_BOT_TOKEN is required for non-local environments")
		}
		if c.Payment.Provider == "yookassa" && (c.Yookassa.ShopID == "" || c.Yookassa.SecretKey == "") {
			errs = append(errs, "YOOKASSA_SHOP_ID and YOOKASSA_SECRET_KEY are required when PAYMENT_PROVIDER=yookassa")
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
		slog.Warn("invalid duration env, fallback will be used", "key", key, "value", v, "fallback", fallback.String())
		return fallback
	}
	return d
}

func getIntEnv(key string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		slog.Warn("invalid int env, fallback will be used", "key", key, "value", v, "fallback", fallback)
		return fallback
	}
	return n
}

func getBoolEnv(key string, fallback bool) bool {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		slog.Warn("invalid bool env, fallback will be used", "key", key, "value", v, "fallback", fallback)
		return fallback
	}
	return b
}

func buildPostgresDSN(pg PostgresConfig) string {
	if pg.Driver == "" {
		return ""
	}
	if pg.Driver != "postgres" {
		// For now we only build postgres DSN, but keep DB_DRIVER in config for future extensibility.
		return ""
	}

	q := url.Values{}
	if pg.SSLMode != "" {
		q.Set("sslmode", pg.SSLMode)
	}
	// charset/collation are intentionally not injected into postgres DSN:
	// these params are MySQL-oriented and break PostgreSQL connections.

	u := &url.URL{
		Scheme: "postgres",
		Host:   pg.Host + ":" + strconv.Itoa(pg.Port),
		Path:   "/" + pg.Database,
	}
	if pg.Username != "" {
		u.User = url.UserPassword(pg.Username, pg.Password)
	}
	u.RawQuery = q.Encode()
	return u.String()
}
