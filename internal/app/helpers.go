package app

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/channelurl"
)

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		lw := &loggingResponseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(lw, r)
		slog.Info("http request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", lw.status,
			"duration_ms", time.Since(start).Milliseconds(),
			"remote", r.RemoteAddr,
			"user_agent", trimLogValue(r.UserAgent(), 256),
			"request_id", strings.TrimSpace(r.Header.Get("X-Request-Id")),
		)
	})
}

type loggingResponseWriter struct {
	http.ResponseWriter
	status int
}

func (w *loggingResponseWriter) WriteHeader(statusCode int) {
	w.status = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *loggingResponseWriter) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(data)
}

func resolveConnectorChannelURL(channelURL, chatID string) string {
	return channelurl.Resolve(channelURL, chatID)
}

func buildBotChatURL(botUsername string) string {
	raw := strings.TrimSpace(strings.TrimPrefix(botUsername, "@"))
	if raw == "" {
		return ""
	}
	return "https://t.me/" + raw
}

func buildMAXBotChatURL(botUsername string) string {
	raw := strings.TrimSpace(strings.TrimPrefix(botUsername, "@"))
	if raw == "" {
		return ""
	}
	return "https://max.ru/" + raw
}

func buildMAXBotStartURL(botUsername, startPayload string) string {
	raw := strings.TrimSpace(strings.TrimPrefix(botUsername, "@"))
	payload := strings.TrimSpace(startPayload)
	if raw == "" || payload == "" {
		return ""
	}
	return "https://max.ru/" + raw + "?start=" + payload
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if trimmed := strings.TrimSpace(v); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func authorizedAdminRequest(r *http.Request, adminToken string) bool {
	adminToken = strings.TrimSpace(adminToken)
	if adminToken == "" {
		return true
	}
	if strings.TrimSpace(r.Header.Get("X-Admin-Token")) == adminToken {
		return true
	}
	authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
	if strings.HasPrefix(authHeader, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer ")) == adminToken
	}
	return false
}

func generateInvoiceID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 10)
	}
	v := int64(binary.BigEndian.Uint64(b[:]) & 0x7fffffffffffffff)
	if v < 1_000_000_000 {
		v += 1_000_000_000
	}
	return strconv.FormatInt(v, 10)
}

func parseRobokassaAmountToKopeks(raw string) (int64, error) {
	value := strings.TrimSpace(strings.ReplaceAll(raw, ",", "."))
	if value == "" {
		return 0, fmt.Errorf("amount is empty")
	}
	if strings.HasPrefix(value, "-") {
		return 0, fmt.Errorf("negative amount")
	}
	parts := strings.SplitN(value, ".", 2)
	rubles, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || rubles < 0 {
		return 0, fmt.Errorf("invalid rubles part")
	}
	var kopeks int64
	if len(parts) == 2 {
		fraction := parts[1]
		if fraction == "" {
			return 0, fmt.Errorf("invalid fractional part")
		}
		if len(fraction) > 2 {
			if strings.Trim(fraction[2:], "0") != "" {
				return 0, fmt.Errorf("too many fractional digits")
			}
			fraction = fraction[:2]
		}
		if len(fraction) == 1 {
			fraction += "0"
		}
		kopeks, err = strconv.ParseInt(fraction, 10, 64)
		if err != nil || kopeks < 0 {
			return 0, fmt.Errorf("invalid kopeks part")
		}
	}
	return rubles*100 + kopeks, nil
}

func logStoreError(msg string, err error, args ...any) {
	slog.Error(msg, append([]any{"error", err}, args...)...)
}

func logAuditError(action string, err error) {
	slog.Error("save audit event failed", "error", err, "action", action)
}

func logWarn(msg string, args ...any) {
	slog.Warn(msg, args...)
}

func logDebug(msg string, args ...any) {
	slog.Debug(msg, args...)
}

func trimLogValue(raw string, limit int) string {
	value := strings.TrimSpace(raw)
	if limit <= 0 || len(value) <= limit {
		return value
	}
	if limit <= 3 {
		return value[:limit]
	}
	return value[:limit-3] + "..."
}
