package logger

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
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
		nowFunc = time.Now
		if currentRotator != nil {
			_ = currentRotator.Close()
			currentRotator = nil
		}
		if currentFile != nil {
			_ = currentFile.Close()
			currentFile = nil
		}
	})

	baseTime := time.Date(2026, 4, 8, 10, 30, 0, 0, time.Local)
	nowFunc = func() time.Time { return baseTime }

	logPath := filepath.Join(t.TempDir(), "logs", "app.jsonl")
	level, err := Init(" DEBUG ", logPath)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if level != "debug" {
		t.Fatalf("level=%q want debug", level)
	}
	wantPath := filepath.Join(filepath.Dir(logPath), "app-2026-04-08.jsonl")
	if _, err := os.Stat(wantPath); err != nil {
		t.Fatalf("Stat(%s): %v", wantPath, err)
	}
	if currentFile == nil {
		t.Fatalf("currentFile=nil want open log file")
	}
}

func TestDatedLogPath_AppendsCurrentDate(t *testing.T) {
	basePath := filepath.Join("logs", "app.log")
	got := datedLogPath(basePath, time.Date(2026, 4, 9, 1, 2, 3, 0, time.Local))
	want := filepath.Join("logs", "app-2026-04-09.log")
	if got != want {
		t.Fatalf("datedLogPath=%q want %q", got, want)
	}
}

func TestDailyRotatingWriter_RotatesWhenDayChanges(t *testing.T) {
	t.Cleanup(func() {
		nowFunc = time.Now
		if currentRotator != nil {
			_ = currentRotator.Close()
			currentRotator = nil
		}
		if currentFile != nil {
			_ = currentFile.Close()
			currentFile = nil
		}
	})

	dir := t.TempDir()
	basePath := filepath.Join(dir, "app.log")
	current := time.Date(2026, 4, 8, 23, 59, 0, 0, time.Local)
	nowFunc = func() time.Time { return current }

	w, err := newDailyRotatingWriter(basePath)
	if err != nil {
		t.Fatalf("newDailyRotatingWriter: %v", err)
	}
	if _, err := w.Write([]byte("first\n")); err != nil {
		t.Fatalf("Write first: %v", err)
	}

	current = time.Date(2026, 4, 9, 0, 1, 0, 0, time.Local)
	if _, err := w.Write([]byte("second\n")); err != nil {
		t.Fatalf("Write second: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	dayOnePath := filepath.Join(dir, "app-2026-04-08.log")
	dayTwoPath := filepath.Join(dir, "app-2026-04-09.log")
	dayOne, err := os.ReadFile(dayOnePath)
	if err != nil {
		t.Fatalf("ReadFile day1: %v", err)
	}
	dayTwo, err := os.ReadFile(dayTwoPath)
	if err != nil {
		t.Fatalf("ReadFile day2: %v", err)
	}
	if string(dayOne) != "first\n" {
		t.Fatalf("day1 contents=%q want first", string(dayOne))
	}
	if string(dayTwo) != "second\n" {
		t.Fatalf("day2 contents=%q want second", string(dayTwo))
	}
}
