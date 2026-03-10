package logger

import (
	"log/slog"
	"os"
	"strings"
)

// Init configures global slog logger and returns normalized effective level.
func Init(level string) string {
	normalized := normalizeLevel(level)
	handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: parseLevel(normalized)})
	slog.SetDefault(slog.New(handler))
	return normalized
}

func normalizeLevel(level string) string {
	level = strings.ToLower(strings.TrimSpace(level))
	switch level {
	case "debug", "info", "warn", "error":
		return level
	default:
		return "info"
	}
}

func parseLevel(level string) slog.Level {
	switch level {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
