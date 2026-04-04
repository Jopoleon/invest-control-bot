package postgres

import (
	"context"
	"regexp"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/Jopoleon/invest-control-bot/internal/domain"
)

func TestSaveAuditEvent(t *testing.T) {
	store, mock, cleanup := newMockStore(t)
	defer cleanup()

	createdAt := time.Date(2026, 4, 4, 12, 30, 0, 0, time.UTC)
	mock.ExpectExec(regexp.QuoteMeta(`
		INSERT INTO audit_events (
			actor_type,
			actor_user_id,
			actor_messenger_kind,
			actor_messenger_user_id,
			actor_subject,
			target_user_id,
			target_messenger_kind,
			target_messenger_user_id,
			connector_id,
			action,
			details,
			created_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
	`)).
		WithArgs(
			domain.AuditActorTypeAdmin,
			sqlmock.AnyArg(),
			string(domain.MessengerKindTelegram),
			"264704572",
			"admin_panel",
			sqlmock.AnyArg(),
			string(domain.MessengerKindMAX),
			"193465776",
			sqlmock.AnyArg(),
			"admin_message_sent",
			"message=hello",
			createdAt,
		).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := store.SaveAuditEvent(context.Background(), domain.AuditEvent{
		ActorType:             domain.AuditActorTypeAdmin,
		ActorUserID:           1,
		ActorMessengerKind:    domain.MessengerKindTelegram,
		ActorMessengerUserID:  "264704572",
		ActorSubject:          "admin_panel",
		TargetUserID:          2,
		TargetMessengerKind:   domain.MessengerKindMAX,
		TargetMessengerUserID: "193465776",
		ConnectorID:           11,
		Action:                "admin_message_sent",
		Details:               "message=hello",
		CreatedAt:             createdAt,
	})
	if err != nil {
		t.Fatalf("SaveAuditEvent: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestListAuditEvents_AppliesFiltersAndPagination(t *testing.T) {
	store, mock, cleanup := newMockStore(t)
	defer cleanup()

	from := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 4, 5, 0, 0, 0, 0, time.UTC)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT COUNT(*) FROM audit_events WHERE 1=1 AND actor_type = $1 AND target_user_id = $2 AND target_messenger_kind = $3 AND target_messenger_user_id = $4 AND connector_id = $5 AND action = $6 AND details ILIKE $7 AND created_at >= $8 AND created_at < $9`)).
		WithArgs(
			domain.AuditActorTypeApp,
			int64(42),
			domain.MessengerKindMAX,
			"193465776",
			int64(11),
			"payment_success_notified",
			"%payment_id=40%",
			from,
			to,
		).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	rows := sqlmock.NewRows([]string{
		"id", "actor_type", "actor_user_id", "actor_messenger_kind", "actor_messenger_user_id", "actor_subject",
		"target_user_id", "target_messenger_kind", "target_messenger_user_id", "connector_id", "action", "details", "created_at",
	}).AddRow(7, "app", 0, "", "", "scheduler", 42, "max", "193465776", 11, "payment_success_notified", "payment_id=40", from)

	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT
			id,
			actor_type,
			COALESCE(actor_user_id, 0),
			actor_messenger_kind,
			actor_messenger_user_id,
			actor_subject,
			COALESCE(target_user_id, 0),
			target_messenger_kind,
			target_messenger_user_id,
			COALESCE(connector_id, 0),
			action,
			details,
			created_at
		FROM audit_events
		WHERE 1=1 AND actor_type = $1 AND target_user_id = $2 AND target_messenger_kind = $3 AND target_messenger_user_id = $4 AND connector_id = $5 AND action = $6 AND details ILIKE $7 AND created_at >= $8 AND created_at < $9
		ORDER BY action DESC, id DESC
		LIMIT $10 OFFSET $11
	`)).
		WithArgs(
			domain.AuditActorTypeApp,
			int64(42),
			domain.MessengerKindMAX,
			"193465776",
			int64(11),
			"payment_success_notified",
			"%payment_id=40%",
			from,
			to,
			20,
			20,
		).
		WillReturnRows(rows)

	items, total, err := store.ListAuditEvents(context.Background(), domain.AuditEventListQuery{
		ActorType:             domain.AuditActorTypeApp,
		TargetUserID:          42,
		TargetMessengerKind:   domain.MessengerKindMAX,
		TargetMessengerUserID: "193465776",
		ConnectorID:           11,
		Action:                "payment_success_notified",
		Search:                "payment_id=40",
		CreatedFrom:           &from,
		CreatedToExclude:      &to,
		SortBy:                "action",
		SortDesc:              true,
		Page:                  2,
		PageSize:              20,
	})
	if err != nil {
		t.Fatalf("ListAuditEvents: %v", err)
	}
	if total != 1 || len(items) != 1 || items[0].ID != 7 {
		t.Fatalf("unexpected result: total=%d items=%+v", total, items)
	}
}

func TestListAuditEvents_DefaultsAndCapsPageSize(t *testing.T) {
	store, mock, cleanup := newMockStore(t)
	defer cleanup()

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT COUNT(*) FROM audit_events WHERE 1=1`)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT
			id,
			actor_type,
			COALESCE(actor_user_id, 0),
			actor_messenger_kind,
			actor_messenger_user_id,
			actor_subject,
			COALESCE(target_user_id, 0),
			target_messenger_kind,
			target_messenger_user_id,
			COALESCE(connector_id, 0),
			action,
			details,
			created_at
		FROM audit_events
		WHERE 1=1
		ORDER BY created_at ASC, id ASC
		LIMIT $1 OFFSET $2
	`)).
		WithArgs(500, 0).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "actor_type", "actor_user_id", "actor_messenger_kind", "actor_messenger_user_id", "actor_subject",
			"target_user_id", "target_messenger_kind", "target_messenger_user_id", "connector_id", "action", "details", "created_at",
		}))

	items, total, err := store.ListAuditEvents(context.Background(), domain.AuditEventListQuery{
		SortBy:   "unsupported",
		SortDesc: false,
		Page:     0,
		PageSize: 999,
	})
	if err != nil {
		t.Fatalf("ListAuditEvents: %v", err)
	}
	if total != 0 || len(items) != 0 {
		t.Fatalf("unexpected result: total=%d items=%d", total, len(items))
	}
}
