package recurringlink

import (
	"errors"
	"testing"
	"time"
)

func TestBuildAndParseCancelToken(t *testing.T) {
	expiresAt := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
	token, err := BuildCancelToken("test-secret-123456789012345678901234", 42, expiresAt)
	if err != nil {
		t.Fatalf("build token: %v", err)
	}
	telegramID, gotExpiresAt, err := ParseCancelToken("test-secret-123456789012345678901234", token, expiresAt.Add(-time.Minute))
	if err != nil {
		t.Fatalf("parse token: %v", err)
	}
	if telegramID != 42 {
		t.Fatalf("telegram id = %d, want 42", telegramID)
	}
	if !gotExpiresAt.Equal(expiresAt) {
		t.Fatalf("expiresAt = %s, want %s", gotExpiresAt, expiresAt)
	}
}

func TestParseCancelToken_Expired(t *testing.T) {
	expiresAt := time.Now().UTC().Add(-time.Minute)
	token, err := BuildCancelToken("test-secret-123456789012345678901234", 77, expiresAt)
	if err != nil {
		t.Fatalf("build token: %v", err)
	}
	_, _, err = ParseCancelToken("test-secret-123456789012345678901234", token, time.Now().UTC())
	if !errors.Is(err, ErrExpiredToken) {
		t.Fatalf("err = %v, want ErrExpiredToken", err)
	}
}

func TestParseCancelToken_InvalidSignature(t *testing.T) {
	expiresAt := time.Now().UTC().Add(time.Hour)
	token, err := BuildCancelToken("test-secret-123456789012345678901234", 77, expiresAt)
	if err != nil {
		t.Fatalf("build token: %v", err)
	}
	_, _, err = ParseCancelToken("wrong-secret-123456789012345678901234", token, time.Now().UTC())
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("err = %v, want ErrInvalidToken", err)
	}
}
