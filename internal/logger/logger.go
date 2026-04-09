package logger

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var (
	mu             sync.Mutex
	currentFile    *os.File
	currentRotator *dailyRotatingWriter
	nowFunc        = time.Now
)

// Init configures global slog logger and returns normalized effective level.
func Init(level, filePath string) (string, error) {
	mu.Lock()
	defer mu.Unlock()

	normalized := normalizeLevel(level)
	writer := io.Writer(os.Stdout)
	if currentRotator != nil {
		_ = currentRotator.Close()
		currentRotator = nil
	}
	if currentFile != nil {
		_ = currentFile.Close()
		currentFile = nil
	}

	if strings.TrimSpace(filePath) != "" {
		rotator, err := newDailyRotatingWriter(filePath)
		if err != nil {
			return normalized, err
		}
		currentRotator = rotator
		writer = io.MultiWriter(os.Stdout, rotator)
	}

	handler := slog.NewJSONHandler(writer, &slog.HandlerOptions{Level: parseLevel(normalized)})
	slog.SetDefault(slog.New(handler))
	return normalized, nil
}

type dailyRotatingWriter struct {
	mu         sync.Mutex
	basePath   string
	currentDay string
	file       *os.File
}

func newDailyRotatingWriter(basePath string) (*dailyRotatingWriter, error) {
	w := &dailyRotatingWriter{basePath: basePath}
	if err := w.rotateIfNeeded(nowFunc()); err != nil {
		return nil, err
	}
	return w, nil
}

func (w *dailyRotatingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if err := w.rotateIfNeeded(nowFunc()); err != nil {
		return 0, err
	}
	return w.file.Write(p)
}

func (w *dailyRotatingWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.file == nil {
		return nil
	}
	err := w.file.Close()
	w.file = nil
	w.currentDay = ""
	currentFile = nil
	return err
}

func (w *dailyRotatingWriter) rotateIfNeeded(ts time.Time) error {
	day := ts.In(time.Local).Format("2006-01-02")
	if w.file != nil && w.currentDay == day {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(w.basePath), 0o755); err != nil {
		return err
	}
	nextPath := datedLogPath(w.basePath, ts)
	f, err := os.OpenFile(nextPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if w.file != nil {
		_ = w.file.Close()
	}
	w.file = f
	w.currentDay = day
	currentFile = f
	return nil
}

func datedLogPath(basePath string, ts time.Time) string {
	dir := filepath.Dir(basePath)
	name := filepath.Base(basePath)
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	if stem == "" {
		stem = "app"
	}
	if ext == "" {
		ext = ".log"
	}
	day := ts.In(time.Local).Format("2006-01-02")
	return filepath.Join(dir, fmt.Sprintf("%s-%s%s", stem, day, ext))
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
