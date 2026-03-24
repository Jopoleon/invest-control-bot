package postgres

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
)

func (s *Store) SaveAuditEvent(ctx context.Context, event domain.AuditEvent) error {
	now := time.Now().UTC()
	if !event.CreatedAt.IsZero() {
		now = event.CreatedAt
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO audit_events (
			telegram_id, connector_id, action, details, created_at
		) VALUES ($1,$2,$3,$4,$5)
	`, event.TelegramID, event.ConnectorID, event.Action, event.Details, now)
	return err
}

// ListAuditEvents returns last N events ordered by newest first.
func (s *Store) ListAuditEvents(ctx context.Context, query domain.AuditEventListQuery) ([]domain.AuditEvent, int, error) {
	if query.Page <= 0 {
		query.Page = 1
	}
	if query.PageSize <= 0 {
		query.PageSize = 50
	}
	if query.PageSize > 500 {
		query.PageSize = 500
	}

	sortColumn := "created_at"
	switch query.SortBy {
	case "telegram_id":
		sortColumn = "telegram_id"
	case "connector_id":
		sortColumn = "connector_id"
	case "action":
		sortColumn = "action"
	case "created_at", "":
		sortColumn = "created_at"
	default:
		sortColumn = "created_at"
	}
	sortDir := "ASC"
	if query.SortDesc {
		sortDir = "DESC"
	}

	where := make([]string, 0, 8)
	args := make([]any, 0, 10)
	where = append(where, "1=1")
	addArg := func(value any) string {
		args = append(args, value)
		return fmt.Sprintf("$%d", len(args))
	}

	if query.TelegramID > 0 {
		where = append(where, "telegram_id = "+addArg(query.TelegramID))
	}
	if query.ConnectorID > 0 {
		where = append(where, "connector_id = "+addArg(query.ConnectorID))
	}
	if query.Action != "" {
		where = append(where, "action = "+addArg(query.Action))
	}
	if query.Search != "" {
		where = append(where, "details ILIKE "+addArg("%"+query.Search+"%"))
	}
	if query.CreatedFrom != nil {
		where = append(where, "created_at >= "+addArg(*query.CreatedFrom))
	}
	if query.CreatedToExclude != nil {
		where = append(where, "created_at < "+addArg(*query.CreatedToExclude))
	}

	whereClause := strings.Join(where, " AND ")

	var total int
	countSQL := "SELECT COUNT(*) FROM audit_events WHERE " + whereClause
	if err := s.db.QueryRowContext(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	offset := (query.Page - 1) * query.PageSize
	dataSQL := fmt.Sprintf(`
		SELECT id, telegram_id, connector_id, action, details, created_at
		FROM audit_events
		WHERE %s
		ORDER BY %s %s, id %s
		LIMIT %s OFFSET %s
	`, whereClause, sortColumn, sortDir, sortDir, addArg(query.PageSize), addArg(offset))

	rows, err := s.db.QueryContext(ctx, dataSQL, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	result := make([]domain.AuditEvent, 0, query.PageSize)
	for rows.Next() {
		var event domain.AuditEvent
		if err := rows.Scan(
			&event.ID,
			&event.TelegramID,
			&event.ConnectorID,
			&event.Action,
			&event.Details,
			&event.CreatedAt,
		); err != nil {
			return nil, 0, err
		}
		result = append(result, event)
	}
	return result, total, rows.Err()
}
