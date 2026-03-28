package bot

import (
	"context"
	"log/slog"
	"strconv"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
	"github.com/Jopoleon/invest-control-bot/internal/messenger"
)

// logAuditEvent persists business-significant user actions for support and compliance.
func (h *Handler) logAuditEvent(ctx context.Context, identity messenger.UserIdentity, connectorID int64, action, details string) {
	if action == "" {
		return
	}
	userID := int64(0)
	if user, ok := h.resolveMessengerUser(ctx, identity); ok {
		userID = user.ID
	}
	event := domain.AuditEvent{
		ActorType:             domain.AuditActorTypeUser,
		ActorUserID:           userID,
		ActorMessengerKind:    messengerKindFromIdentity(identity.Kind),
		ActorMessengerUserID:  strconv.FormatInt(identity.ID, 10),
		TargetUserID:          userID,
		TargetMessengerKind:   messengerKindFromIdentity(identity.Kind),
		TargetMessengerUserID: strconv.FormatInt(identity.ID, 10),
		ConnectorID:           connectorID,
		Action:                action,
		Details:               details,
		CreatedAt:             time.Now().UTC(),
	}
	if err := h.store.SaveAuditEvent(ctx, event); err != nil {
		slog.Error("save audit event failed", "error", err, "messenger_kind", identity.Kind, "messenger_user_id", identity.ID, "connector_id", connectorID, "action", action)
	}
}
