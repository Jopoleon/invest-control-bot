package bot

import (
	"context"
	"log/slog"
	"time"

	"github.com/Jopoleon/telega-bot-fedor/internal/domain"
)

// logAuditEvent persists business-significant user actions for support and compliance.
func (h *Handler) logAuditEvent(ctx context.Context, telegramID int64, connectorID, action, details string) {
	if action == "" {
		return
	}
	event := domain.AuditEvent{
		TelegramID:  telegramID,
		ConnectorID: connectorID,
		Action:      action,
		Details:     details,
		CreatedAt:   time.Now().UTC(),
	}
	if err := h.store.SaveAuditEvent(ctx, event); err != nil {
		slog.Error("save audit event failed", "error", err, "telegram_id", telegramID, "connector_id", connectorID, "action", action)
	}
}
