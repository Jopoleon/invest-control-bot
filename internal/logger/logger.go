package logger

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var (
	mu          sync.Mutex
	currentFile *os.File
)

// Init configures global slog logger and returns normalized effective level.
func Init(level, filePath string) (string, error) {
	mu.Lock()
	defer mu.Unlock()

	normalized := normalizeLevel(level)
	writer := io.Writer(os.Stdout)
	if currentFile != nil {
		_ = currentFile.Close()
		currentFile = nil
	}

	if strings.TrimSpace(filePath) != "" {
		if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
			return normalized, err
		}
		f, err := os.OpenFile(filePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return normalized, err
		}
		currentFile = f
		writer = io.MultiWriter(os.Stdout, f)
	}

	handler := slog.NewJSONHandler(writer, &slog.HandlerOptions{Level: parseLevel(normalized)})
	slog.SetDefault(slog.New(handler))
	return normalized, nil
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
