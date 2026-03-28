package app

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
	"github.com/Jopoleon/invest-control-bot/internal/messenger"
)

// buildAppTargetAuditEvent records app-initiated actions such as provider callbacks,
// scheduler jobs and public-page side effects while preserving the affected user context.
func (a *application) buildAppTargetAuditEvent(ctx context.Context, userID, legacyExternalID, connectorID int64, action, details string, createdAt time.Time) domain.AuditEvent {
	event := domain.AuditEvent{
		ActorType:    domain.AuditActorTypeApp,
		ActorSubject: "app",
		TargetUserID: userID,
		ConnectorID:  connectorID,
		Action:       action,
		Details:      details,
		CreatedAt:    createdAt,
	}
	if legacyExternalID > 0 {
		event.TargetMessengerKind = messengerKindToDomain(a.resolvePreferredMessengerKind(ctx, userID, legacyExternalID))
		event.TargetMessengerUserID = strconv.FormatInt(legacyExternalID, 10)
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
