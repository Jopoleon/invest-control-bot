package logger

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizeAndParseLevel(t *testing.T) {
	if got := normalizeLevel(" DEBUG "); got != "debug" {
		t.Fatalf("normalizeLevel=%q want debug", got)
	}
	if got := normalizeLevel("loud"); got != "info" {
		t.Fatalf("normalizeLevel=%q want info", got)
	}
	if got := parseLevel("warn"); got != slog.LevelWarn {
		t.Fatalf("parseLevel=%v want warn", got)
	}
	if got := parseLevel("error"); got != slog.LevelError {
		t.Fatalf("parseLevel=%v want error", got)
	}
}

func TestInit_CreatesLogFileAndReturnsNormalizedLevel(t *testing.T) {
	t.Cleanup(func() {
		if currentFile != nil {
			_ = currentFile.Close()
			currentFile = nil
		}
	})

	logPath := filepath.Join(t.TempDir(), "logs", "app.jsonl")
	level, err := Init(" DEBUG ", logPath)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if level != "debug" {
		t.Fatalf("level=%q want debug", level)
	}
	if _, err := os.Stat(logPath); err != nil {
		t.Fatalf("Stat(%s): %v", logPath, err)
	}
	if currentFile == nil {
		t.Fatalf("currentFile=nil want open log file")
	}
}
