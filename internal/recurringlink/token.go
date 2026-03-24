package recurringlink

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const tokenVersion = "v1"

var (
	ErrInvalidToken = errors.New("invalid recurring token")
	ErrExpiredToken = errors.New("expired recurring token")
)

type cancelTokenPayload struct {
	TelegramID int64 `json:"t"`
	ExpiresAt  int64 `json:"e"`
}

func BuildCancelToken(secret string, telegramID int64, expiresAt time.Time) (string, error) {
	secret = strings.TrimSpace(secret)
	if secret == "" || telegramID <= 0 || expiresAt.IsZero() {
		return "", ErrInvalidToken
	}
	payload, err := json.Marshal(cancelTokenPayload{
		TelegramID: telegramID,
		ExpiresAt:  expiresAt.UTC().Unix(),
	})
	if err != nil {
		return "", fmt.Errorf("marshal cancel token payload: %w", err)
	}
	encodedPayload := base64.RawURLEncoding.EncodeToString(payload)
	signature := sign(secret, tokenVersion+"."+encodedPayload)
	return tokenVersion + "." + encodedPayload + "." + signature, nil
}

func ParseCancelToken(secret, token string, now time.Time) (int64, time.Time, error) {
	secret = strings.TrimSpace(secret)
	token = strings.TrimSpace(token)
	if secret == "" || token == "" {
		return 0, time.Time{}, ErrInvalidToken
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[0] != tokenVersion {
		return 0, time.Time{}, ErrInvalidToken
	}
	expected := sign(secret, parts[0]+"."+parts[1])
	if !hmac.Equal([]byte(expected), []byte(parts[2])) {
		return 0, time.Time{}, ErrInvalidToken
	}
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return 0, time.Time{}, ErrInvalidToken
	}
	var payload cancelTokenPayload
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return 0, time.Time{}, ErrInvalidToken
	}
	if payload.TelegramID <= 0 || payload.ExpiresAt <= 0 {
		return 0, time.Time{}, ErrInvalidToken
	}
	expiresAt := time.Unix(payload.ExpiresAt, 0).UTC()
	if !now.IsZero() && now.UTC().After(expiresAt) {
		return 0, expiresAt, ErrExpiredToken
	}
	return payload.TelegramID, expiresAt, nil
}

func sign(secret, value string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(value))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
