package app

import (
	"context"
	"strings"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
	"github.com/Jopoleon/invest-control-bot/internal/messenger"
)

// buildAppTargetAuditEvent records app-initiated actions such as provider callbacks,
// scheduler jobs and public-page side effects while preserving the affected user context.
func (a *application) buildAppTargetAuditEvent(ctx context.Context, userID int64, preferredMessengerUserID string, connectorID int64, action, details string, createdAt time.Time) domain.AuditEvent {
	event := domain.AuditEvent{
		ActorType:    domain.AuditActorTypeApp,
		ActorSubject: "app",
		TargetUserID: userID,
		ConnectorID:  connectorID,
		Action:       action,
		Details:      details,
		CreatedAt:    createdAt,
	}
	if account, found, err := a.resolvePreferredMessengerAccount(ctx, userID, preferredMessengerUserID); err == nil && found {
		event.TargetMessengerKind = account.MessengerKind
		event.TargetMessengerUserID = account.MessengerUserID
	}
	return event
}

func buildAppMessengerTargetAuditEvent(kind domain.MessengerKind, messengerUserID string, connectorID int64, action, details string, createdAt time.Time) domain.AuditEvent {
	return domain.AuditEvent{
		ActorType:             domain.AuditActorTypeApp,
		ActorSubject:          "app",
		TargetMessengerKind:   kind,
		TargetMessengerUserID: strings.TrimSpace(messengerUserID),
		ConnectorID:           connectorID,
		Action:                action,
		Details:               details,
		CreatedAt:             createdAt,
	}
}

func messengerKindToDomain(kind messenger.Kind) domain.MessengerKind {
	switch kind {
	case messenger.KindMAX:
		return domain.MessengerKindMAX
	default:
		return domain.MessengerKindTelegram
	}
}
