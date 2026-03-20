package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Jopoleon/telega-bot-fedor/internal/domain"
	storepkg "github.com/Jopoleon/telega-bot-fedor/internal/store"
	"github.com/jmoiron/sqlx"
)

// Store is a PostgreSQL-backed implementation of store.Store.
type Store struct {
	db *sqlx.DB
}

type rowScanner interface {
	Scan(dest ...any) error
}

// New creates PostgreSQL store from opened sqlx.DB connection.
func New(db *sqlx.DB) *Store {
	return &Store{db: db}
}

func scanPayment(scanner rowScanner) (domain.Payment, error) {
	var (
		payment domain.Payment
		status  string
	)
	err := scanner.Scan(
		&payment.ID,
		&payment.Provider,
		&payment.ProviderPaymentID,
		&status,
		&payment.Token,
		&payment.TelegramID,
		&payment.ConnectorID,
		&payment.SubscriptionID,
		&payment.ParentPaymentID,
		&payment.AutoPayEnabled,
		&payment.AmountRUB,
		&payment.CheckoutURL,
		&payment.CreatedAt,
		&payment.PaidAt,
		&payment.UpdatedAt,
	)
	if err != nil {
		return domain.Payment{}, err
	}
	payment.Status = domain.PaymentStatus(status)
	return payment, nil
}

func scanSubscription(scanner rowScanner) (domain.Subscription, error) {
	var (
		item   domain.Subscription
		status string
	)
	err := scanner.Scan(
		&item.ID,
		&item.TelegramID,
		&item.ConnectorID,
		&item.PaymentID,
		&status,
		&item.AutoPayEnabled,
		&item.StartsAt,
		&item.EndsAt,
		&item.ReminderSentAt,
		&item.ExpiryNoticeSentAt,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if err != nil {
		return domain.Subscription{}, err
	}
	item.Status = domain.SubscriptionStatus(status)
	return item, nil
}

func scanLegalDocument(scanner rowScanner) (domain.LegalDocument, error) {
	var (
		item    domain.LegalDocument
		docType string
	)
	err := scanner.Scan(
		&item.ID,
		&docType,
		&item.Title,
		&item.Content,
		&item.ExternalURL,
		&item.Version,
		&item.IsActive,
		&item.CreatedAt,
	)
	if err != nil {
		return domain.LegalDocument{}, err
	}
	item.Type = domain.LegalDocumentType(docType)
	return item, nil
}

func scanAdminSession(scanner rowScanner) (domain.AdminSession, error) {
	var session domain.AdminSession
	err := scanner.Scan(
		&session.ID,
		&session.TokenHash,
		&session.Subject,
		&session.CreatedAt,
		&session.ExpiresAt,
		&session.LastSeenAt,
		&session.RevokedAt,
		&session.IP,
		&session.UserAgent,
		&session.RotatedAt,
		&session.ReplacedByHash,
	)
	if err != nil {
		return domain.AdminSession{}, err
	}
	return session, nil
}

// CreateConnector inserts connector row.
func (s *Store) CreateConnector(ctx context.Context, c domain.Connector) error {
	if c.StartPayload == "" {
		return errors.New("start payload is required")
	}
	if c.CreatedAt.IsZero() {
		c.CreatedAt = time.Now().UTC()
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO connectors (
			start_payload, name, description, chat_id, channel_url, price_rub, period_days,
			offer_url, privacy_url, is_active, created_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
	`,
		c.StartPayload,
		c.Name,
		c.Description,
		c.ChatID,
		c.ChannelURL,
		c.PriceRUB,
		c.PeriodDays,
		c.OfferURL,
		c.PrivacyURL,
		c.IsActive,
		c.CreatedAt,
	)
	return err
}

// ListConnectors returns connectors ordered by created_at.
func (s *Store) ListConnectors(ctx context.Context) ([]domain.Connector, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, start_payload, name, description, chat_id, channel_url, price_rub, period_days,
		       offer_url, privacy_url, is_active, created_at
		FROM connectors
		ORDER BY created_at ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]domain.Connector, 0)
	for rows.Next() {
		var c domain.Connector
		if err := rows.Scan(
			&c.ID,
			&c.StartPayload,
			&c.Name,
			&c.Description,
			&c.ChatID,
			&c.ChannelURL,
			&c.PriceRUB,
			&c.PeriodDays,
			&c.OfferURL,
			&c.PrivacyURL,
			&c.IsActive,
			&c.CreatedAt,
		); err != nil {
			return nil, err
		}
		result = append(result, c)
	}
	return result, rows.Err()
}

// GetConnector fetches connector by ID.
func (s *Store) GetConnector(ctx context.Context, connectorID int64) (domain.Connector, bool, error) {
	var c domain.Connector
	err := s.db.QueryRowContext(ctx, `
		SELECT id, start_payload, name, description, chat_id, channel_url, price_rub, period_days,
		       offer_url, privacy_url, is_active, created_at
		FROM connectors
		WHERE id = $1
	`, connectorID).Scan(
		&c.ID,
		&c.StartPayload,
		&c.Name,
		&c.Description,
		&c.ChatID,
		&c.ChannelURL,
		&c.PriceRUB,
		&c.PeriodDays,
		&c.OfferURL,
		&c.PrivacyURL,
		&c.IsActive,
		&c.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return domain.Connector{}, false, nil
	}
	if err != nil {
		return domain.Connector{}, false, err
	}
	return c, true, nil
}

// GetConnectorByStartPayload fetches connector by deeplink payload.
func (s *Store) GetConnectorByStartPayload(ctx context.Context, payload string) (domain.Connector, bool, error) {
	var c domain.Connector
	err := s.db.QueryRowContext(ctx, `
		SELECT id, start_payload, name, description, chat_id, channel_url, price_rub, period_days,
		       offer_url, privacy_url, is_active, created_at
		FROM connectors
		WHERE start_payload = $1
	`, payload).Scan(
		&c.ID,
		&c.StartPayload,
		&c.Name,
		&c.Description,
		&c.ChatID,
		&c.ChannelURL,
		&c.PriceRUB,
		&c.PeriodDays,
		&c.OfferURL,
		&c.PrivacyURL,
		&c.IsActive,
		&c.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return domain.Connector{}, false, nil
	}
	if err != nil {
		return domain.Connector{}, false, err
	}
	return c, true, nil
}

// SetConnectorActive toggles connector active status.
func (s *Store) SetConnectorActive(ctx context.Context, connectorID int64, active bool) error {
	res, err := s.db.ExecContext(ctx, `UPDATE connectors SET is_active = $2 WHERE id = $1`, connectorID, active)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return storepkg.ErrConnectorNotFound
	}
	return nil
}

// DeleteConnector hard-deletes connector only when it has no dependent business history.
func (s *Store) DeleteConnector(ctx context.Context, connectorID int64) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	var exists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM connectors WHERE id = $1)`, connectorID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return storepkg.ErrConnectorNotFound
	}

	queries := []string{
		`SELECT EXISTS(SELECT 1 FROM payments WHERE connector_id = $1)`,
		`SELECT EXISTS(SELECT 1 FROM subscriptions WHERE connector_id = $1)`,
		`SELECT EXISTS(SELECT 1 FROM user_consents WHERE connector_id = $1)`,
		`SELECT EXISTS(SELECT 1 FROM registration_states WHERE connector_id = $1)`,
	}
	for _, query := range queries {
		var inUse bool
		if err := tx.QueryRowContext(ctx, query, connectorID).Scan(&inUse); err != nil {
			return err
		}
		if inUse {
			return storepkg.ErrConnectorInUse
		}
	}

	res, err := tx.ExecContext(ctx, `DELETE FROM connectors WHERE id = $1`, connectorID)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return storepkg.ErrConnectorNotFound
	}
	return tx.Commit()
}

// CreateLegalDocument inserts new legal document version and optionally marks it active.
func (s *Store) CreateLegalDocument(ctx context.Context, doc domain.LegalDocument) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	if doc.CreatedAt.IsZero() {
		doc.CreatedAt = time.Now().UTC()
	}
	if doc.Version <= 0 {
		if err := tx.QueryRowContext(ctx, `
			SELECT COALESCE(MAX(version), 0) + 1
			FROM legal_documents
			WHERE doc_type = $1
		`, string(doc.Type)).Scan(&doc.Version); err != nil {
			return err
		}
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO legal_documents (
			doc_type, title, content, external_url, version, is_active, created_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7)
	`,
		string(doc.Type),
		doc.Title,
		doc.Content,
		doc.ExternalURL,
		doc.Version,
		doc.IsActive,
		doc.CreatedAt,
	)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// UpdateLegalDocument updates existing legal document fields and active flag.
func (s *Store) UpdateLegalDocument(ctx context.Context, doc domain.LegalDocument) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	var docType string
	if err := tx.QueryRowContext(ctx, `
		SELECT doc_type
		FROM legal_documents
		WHERE id = $1
	`, doc.ID).Scan(&docType); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE legal_documents
		SET title = $2,
		    content = $3,
		    external_url = $4,
		    is_active = $5
		WHERE id = $1
	`, doc.ID, doc.Title, doc.Content, doc.ExternalURL, doc.IsActive); err != nil {
		return err
	}
	return tx.Commit()
}

// ListLegalDocuments returns all legal document versions.
func (s *Store) ListLegalDocuments(ctx context.Context, docType domain.LegalDocumentType) ([]domain.LegalDocument, error) {
	query := `
		SELECT id, doc_type, title, content, external_url, version, is_active, created_at
		FROM legal_documents
	`
	args := []any{}
	if docType != "" {
		query += ` WHERE doc_type = $1`
		args = append(args, string(docType))
	}
	query += ` ORDER BY doc_type ASC, version DESC, created_at DESC`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]domain.LegalDocument, 0)
	for rows.Next() {
		item, err := scanLegalDocument(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// GetLegalDocument fetches legal document by ID.
func (s *Store) GetLegalDocument(ctx context.Context, documentID int64) (domain.LegalDocument, bool, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, doc_type, title, content, external_url, version, is_active, created_at
		FROM legal_documents
		WHERE id = $1
	`, documentID)
	item, err := scanLegalDocument(row)
	if err == sql.ErrNoRows {
		return domain.LegalDocument{}, false, nil
	}
	if err != nil {
		return domain.LegalDocument{}, false, err
	}
	return item, true, nil
}

// GetActiveLegalDocument fetches active document version by type.
func (s *Store) GetActiveLegalDocument(ctx context.Context, docType domain.LegalDocumentType) (domain.LegalDocument, bool, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, doc_type, title, content, external_url, version, is_active, created_at
		FROM legal_documents
		WHERE doc_type = $1 AND is_active = TRUE
		ORDER BY version DESC, id DESC
		LIMIT 1
	`, string(docType))
	item, err := scanLegalDocument(row)
	if err == sql.ErrNoRows {
		return domain.LegalDocument{}, false, nil
	}
	if err != nil {
		return domain.LegalDocument{}, false, err
	}
	return item, true, nil
}

// SetLegalDocumentActive toggles published state for a single legal document.
func (s *Store) SetLegalDocumentActive(ctx context.Context, documentID int64, active bool) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE legal_documents
		SET is_active = $2
		WHERE id = $1
	`, documentID, active)
	return err
}

// DeleteLegalDocument removes legal document by ID.
func (s *Store) DeleteLegalDocument(ctx context.Context, documentID int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM legal_documents WHERE id = $1`, documentID)
	return err
}

// CreateAdminSession inserts admin session row.
func (s *Store) CreateAdminSession(ctx context.Context, session domain.AdminSession) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO admin_sessions (
			session_token_hash, subject, created_at, expires_at, last_seen_at,
			revoked_at, ip, user_agent, rotated_at, replaced_by_hash
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
	`,
		session.TokenHash,
		session.Subject,
		session.CreatedAt,
		session.ExpiresAt,
		session.LastSeenAt,
		session.RevokedAt,
		session.IP,
		session.UserAgent,
		session.RotatedAt,
		session.ReplacedByHash,
	)
	return err
}

// GetAdminSessionByTokenHash fetches admin session by token hash.
func (s *Store) GetAdminSessionByTokenHash(ctx context.Context, tokenHash string) (domain.AdminSession, bool, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, session_token_hash, subject, created_at, expires_at, last_seen_at,
		       revoked_at, ip, user_agent, rotated_at, replaced_by_hash
		FROM admin_sessions
		WHERE session_token_hash = $1
		LIMIT 1
	`, tokenHash)
	session, err := scanAdminSession(row)
	if err == sql.ErrNoRows {
		return domain.AdminSession{}, false, nil
	}
	if err != nil {
		return domain.AdminSession{}, false, err
	}
	return session, true, nil
}

// TouchAdminSession updates last access timestamp.
func (s *Store) TouchAdminSession(ctx context.Context, sessionID int64, lastSeenAt time.Time) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE admin_sessions
		SET last_seen_at = $2
		WHERE id = $1
	`, sessionID, lastSeenAt)
	return err
}

// RotateAdminSession replaces token hash for active session.
func (s *Store) RotateAdminSession(ctx context.Context, sessionID int64, newTokenHash string, rotatedAt time.Time) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE admin_sessions
		SET replaced_by_hash = $2,
		    session_token_hash = $2,
		    rotated_at = $3,
		    last_seen_at = $3
		WHERE id = $1
	`, sessionID, newTokenHash, rotatedAt)
	return err
}

// RevokeAdminSession marks session revoked.
func (s *Store) RevokeAdminSession(ctx context.Context, sessionID int64, revokedAt time.Time) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE admin_sessions
		SET revoked_at = $2
		WHERE id = $1
	`, sessionID, revokedAt)
	return err
}

// SaveConsent upserts user consent for connector.
func (s *Store) SaveConsent(ctx context.Context, consent domain.Consent) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO user_consents (
			telegram_id, connector_id, offer_accepted_at, privacy_accepted_at,
			offer_document_id, offer_document_version, privacy_document_id, privacy_document_version
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		ON CONFLICT (telegram_id, connector_id)
		DO UPDATE SET
			offer_accepted_at = EXCLUDED.offer_accepted_at,
			privacy_accepted_at = EXCLUDED.privacy_accepted_at,
			offer_document_id = EXCLUDED.offer_document_id,
			offer_document_version = EXCLUDED.offer_document_version,
			privacy_document_id = EXCLUDED.privacy_document_id,
			privacy_document_version = EXCLUDED.privacy_document_version
	`,
		consent.TelegramID,
		consent.ConnectorID,
		consent.OfferAcceptedAt,
		consent.PrivacyAcceptedAt,
		consent.OfferDocumentID,
		consent.OfferDocumentVersion,
		consent.PrivacyDocumentID,
		consent.PrivacyDocumentVersion,
	)
	return err
}

// GetConsent returns stored consent.
func (s *Store) GetConsent(ctx context.Context, telegramID int64, connectorID int64) (domain.Consent, bool, error) {
	var c domain.Consent
	err := s.db.QueryRowContext(ctx, `
		SELECT telegram_id, connector_id, offer_accepted_at, privacy_accepted_at,
		       offer_document_id, offer_document_version, privacy_document_id, privacy_document_version
		FROM user_consents
		WHERE telegram_id = $1 AND connector_id = $2
	`, telegramID, connectorID).Scan(
		&c.TelegramID,
		&c.ConnectorID,
		&c.OfferAcceptedAt,
		&c.PrivacyAcceptedAt,
		&c.OfferDocumentID,
		&c.OfferDocumentVersion,
		&c.PrivacyDocumentID,
		&c.PrivacyDocumentVersion,
	)
	if err == sql.ErrNoRows {
		return domain.Consent{}, false, nil
	}
	if err != nil {
		return domain.Consent{}, false, err
	}
	return c, true, nil
}

// ListConsentsByTelegram returns all consents for a specific Telegram user.
func (s *Store) ListConsentsByTelegram(ctx context.Context, telegramID int64) ([]domain.Consent, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT telegram_id, connector_id, offer_accepted_at, privacy_accepted_at,
		       offer_document_id, offer_document_version, privacy_document_id, privacy_document_version
		FROM user_consents
		WHERE telegram_id = $1
		ORDER BY GREATEST(offer_accepted_at, privacy_accepted_at) DESC, connector_id DESC
	`, telegramID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]domain.Consent, 0)
	for rows.Next() {
		var c domain.Consent
		if err := rows.Scan(
			&c.TelegramID,
			&c.ConnectorID,
			&c.OfferAcceptedAt,
			&c.PrivacyAcceptedAt,
			&c.OfferDocumentID,
			&c.OfferDocumentVersion,
			&c.PrivacyDocumentID,
			&c.PrivacyDocumentVersion,
		); err != nil {
			return nil, err
		}
		items = append(items, c)
	}
	return items, rows.Err()
}

// CreateRecurringConsent stores explicit recurring/autopay opt-in history.
func (s *Store) CreateRecurringConsent(ctx context.Context, consent domain.RecurringConsent) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO recurring_consents (
			telegram_id, connector_id, accepted_at,
			offer_document_id, offer_document_version,
			user_agreement_document_id, user_agreement_document_version
		) VALUES ($1,$2,$3,$4,$5,$6,$7)
	`,
		consent.TelegramID,
		consent.ConnectorID,
		consent.AcceptedAt,
		consent.OfferDocumentID,
		consent.OfferDocumentVersion,
		consent.UserAgreementDocumentID,
		consent.UserAgreementDocumentVersion,
	)
	return err
}

// ListRecurringConsentsByTelegram returns recurring consent history for one Telegram user.
func (s *Store) ListRecurringConsentsByTelegram(ctx context.Context, telegramID int64) ([]domain.RecurringConsent, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, telegram_id, connector_id, accepted_at,
		       offer_document_id, offer_document_version,
		       user_agreement_document_id, user_agreement_document_version
		FROM recurring_consents
		WHERE telegram_id = $1
		ORDER BY accepted_at DESC, id DESC
	`, telegramID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]domain.RecurringConsent, 0)
	for rows.Next() {
		var consent domain.RecurringConsent
		if err := rows.Scan(
			&consent.ID,
			&consent.TelegramID,
			&consent.ConnectorID,
			&consent.AcceptedAt,
			&consent.OfferDocumentID,
			&consent.OfferDocumentVersion,
			&consent.UserAgreementDocumentID,
			&consent.UserAgreementDocumentVersion,
		); err != nil {
			return nil, err
		}
		items = append(items, consent)
	}
	return items, rows.Err()
}

// SaveUser upserts user profile.
func (s *Store) SaveUser(ctx context.Context, user domain.User) error {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO users (
			telegram_id, telegram_username, full_name, phone, email, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6)
		ON CONFLICT (telegram_id)
		DO UPDATE SET
			telegram_username = EXCLUDED.telegram_username,
			full_name = EXCLUDED.full_name,
			phone = EXCLUDED.phone,
			email = EXCLUDED.email,
			updated_at = EXCLUDED.updated_at
	`, user.TelegramID, user.TelegramUsername, user.FullName, user.Phone, user.Email, now)
	return err
}

// GetUser fetches user by Telegram ID.
func (s *Store) GetUser(ctx context.Context, telegramID int64) (domain.User, bool, error) {
	var u domain.User
	err := s.db.QueryRowContext(ctx, `
		SELECT telegram_id, telegram_username, full_name, phone, email, updated_at
		FROM users
		WHERE telegram_id = $1
	`, telegramID).Scan(&u.TelegramID, &u.TelegramUsername, &u.FullName, &u.Phone, &u.Email, &u.UpdatedAt)
	if err == sql.ErrNoRows {
		return domain.User{}, false, nil
	}
	if err != nil {
		return domain.User{}, false, err
	}
	return u, true, nil
}

// ListUsers returns admin-oriented user list with recurring preference metadata.
func (s *Store) ListUsers(ctx context.Context, query domain.UserListQuery) ([]domain.UserListItem, error) {
	if query.Limit <= 0 {
		query.Limit = 200
	}
	if query.Limit > 1000 {
		query.Limit = 1000
	}

	where := make([]string, 0, 2)
	args := make([]any, 0, 4)
	argPos := 1

	if query.TelegramID > 0 {
		where = append(where, fmt.Sprintf("u.telegram_id = $%d", argPos))
		args = append(args, query.TelegramID)
		argPos++
	}
	if search := strings.TrimSpace(query.Search); search != "" {
		like := "%" + search + "%"
		where = append(where, fmt.Sprintf("(CAST(u.telegram_id AS TEXT) ILIKE $%d OR u.telegram_username ILIKE $%d OR u.full_name ILIKE $%d OR u.phone ILIKE $%d OR u.email ILIKE $%d)", argPos, argPos, argPos, argPos, argPos))
		args = append(args, like)
		argPos++
	}

	stmt := `
		SELECT
			u.telegram_id,
			u.telegram_username,
			u.full_name,
			u.phone,
			u.email,
			COALESCE(us.auto_pay_enabled, false) AS auto_pay_enabled,
			us.telegram_id IS NOT NULL AS has_auto_pay_settings,
			u.updated_at
		FROM users u
		LEFT JOIN user_settings us ON us.telegram_id = u.telegram_id
	`
	if len(where) > 0 {
		stmt += " WHERE " + strings.Join(where, " AND ")
	}
	stmt += fmt.Sprintf(" ORDER BY u.updated_at DESC, u.telegram_id DESC LIMIT $%d", argPos)
	args = append(args, query.Limit)

	rows, err := s.db.QueryContext(ctx, stmt, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]domain.UserListItem, 0)
	for rows.Next() {
		var item domain.UserListItem
		if err := rows.Scan(
			&item.TelegramID,
			&item.TelegramUsername,
			&item.FullName,
			&item.Phone,
			&item.Email,
			&item.AutoPayEnabled,
			&item.HasAutoPaySettings,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// SetUserAutoPayEnabled stores user recurring preference used for future payment creation.
func (s *Store) SetUserAutoPayEnabled(ctx context.Context, telegramID int64, enabled bool, updatedAt time.Time) error {
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO user_settings (telegram_id, auto_pay_enabled, updated_at)
		VALUES ($1,$2,$3)
		ON CONFLICT (telegram_id)
		DO UPDATE SET
			auto_pay_enabled = EXCLUDED.auto_pay_enabled,
			updated_at = EXCLUDED.updated_at
	`, telegramID, enabled, updatedAt)
	return err
}

// GetUserAutoPayEnabled returns user recurring preference if record exists.
func (s *Store) GetUserAutoPayEnabled(ctx context.Context, telegramID int64) (bool, bool, error) {
	var enabled bool
	err := s.db.QueryRowContext(ctx, `
		SELECT auto_pay_enabled
		FROM user_settings
		WHERE telegram_id = $1
	`, telegramID).Scan(&enabled)
	if err == sql.ErrNoRows {
		return false, false, nil
	}
	if err != nil {
		return false, false, err
	}
	return enabled, true, nil
}

// SaveRegistrationState upserts registration FSM state.
func (s *Store) SaveRegistrationState(ctx context.Context, state domain.RegistrationState) error {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO registration_states (
			telegram_id, connector_id, step, telegram_username, updated_at
		) VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (telegram_id)
		DO UPDATE SET
			connector_id = EXCLUDED.connector_id,
			step = EXCLUDED.step,
			telegram_username = EXCLUDED.telegram_username,
			updated_at = EXCLUDED.updated_at
	`, state.TelegramID, state.ConnectorID, string(state.Step), state.TelegramUsername, now)
	return err
}

// GetRegistrationState fetches registration FSM state.
func (s *Store) GetRegistrationState(ctx context.Context, telegramID int64) (domain.RegistrationState, bool, error) {
	var state domain.RegistrationState
	var step string
	err := s.db.QueryRowContext(ctx, `
		SELECT telegram_id, connector_id, step, telegram_username, updated_at
		FROM registration_states
		WHERE telegram_id = $1
	`, telegramID).Scan(&state.TelegramID, &state.ConnectorID, &step, &state.TelegramUsername, &state.UpdatedAt)
	if err == sql.ErrNoRows {
		return domain.RegistrationState{}, false, nil
	}
	if err != nil {
		return domain.RegistrationState{}, false, err
	}
	state.Step = domain.RegistrationStep(step)
	return state, true, nil
}

// DeleteRegistrationState removes registration FSM state.
func (s *Store) DeleteRegistrationState(ctx context.Context, telegramID int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM registration_states WHERE telegram_id = $1`, telegramID)
	return err
}

// SaveAuditEvent writes immutable user action to audit_events table.
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

// CreatePayment inserts pending payment transaction.
func (s *Store) CreatePayment(ctx context.Context, payment domain.Payment) error {
	now := time.Now().UTC()
	if payment.CreatedAt.IsZero() {
		payment.CreatedAt = now
	}
	if payment.UpdatedAt.IsZero() {
		payment.UpdatedAt = now
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO payments (
			provider, provider_payment_id, status, token, telegram_id, connector_id, subscription_id, parent_payment_id,
			auto_pay_enabled, amount_rub, checkout_url, created_at, paid_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
	`,
		payment.Provider,
		payment.ProviderPaymentID,
		string(payment.Status),
		payment.Token,
		payment.TelegramID,
		payment.ConnectorID,
		payment.SubscriptionID,
		payment.ParentPaymentID,
		payment.AutoPayEnabled,
		payment.AmountRUB,
		payment.CheckoutURL,
		payment.CreatedAt,
		payment.PaidAt,
		payment.UpdatedAt,
	)
	return err
}

// GetPaymentByToken returns payment by externally visible checkout token.
func (s *Store) GetPaymentByToken(ctx context.Context, token string) (domain.Payment, bool, error) {
	payment, err := scanPayment(s.db.QueryRowContext(ctx, `
		SELECT id, provider, provider_payment_id, status, token, telegram_id, connector_id, subscription_id, parent_payment_id,
		       auto_pay_enabled, amount_rub, checkout_url, created_at, paid_at, updated_at
		FROM payments
		WHERE token = $1
	`, token))
	if err == sql.ErrNoRows {
		return domain.Payment{}, false, nil
	}
	if err != nil {
		return domain.Payment{}, false, err
	}
	return payment, true, nil
}

// GetPaymentByID returns payment by internal identifier.
func (s *Store) GetPaymentByID(ctx context.Context, paymentID int64) (domain.Payment, bool, error) {
	payment, err := scanPayment(s.db.QueryRowContext(ctx, `
		SELECT id, provider, provider_payment_id, status, token, telegram_id, connector_id, subscription_id, parent_payment_id,
		       auto_pay_enabled, amount_rub, checkout_url, created_at, paid_at, updated_at
		FROM payments
		WHERE id = $1
	`, paymentID))
	if err == sql.ErrNoRows {
		return domain.Payment{}, false, nil
	}
	if err != nil {
		return domain.Payment{}, false, err
	}
	return payment, true, nil
}

// GetPendingRebillPaymentBySubscription returns outstanding recurring attempt for subscription.
func (s *Store) GetPendingRebillPaymentBySubscription(ctx context.Context, subscriptionID int64) (domain.Payment, bool, error) {
	if subscriptionID <= 0 {
		return domain.Payment{}, false, nil
	}
	payment, err := scanPayment(s.db.QueryRowContext(ctx, `
		SELECT id, provider, provider_payment_id, status, token, telegram_id, connector_id, subscription_id, parent_payment_id,
		       auto_pay_enabled, amount_rub, checkout_url, created_at, paid_at, updated_at
		FROM payments
		WHERE subscription_id = $1
		  AND status = $2
		ORDER BY created_at DESC, id DESC
		LIMIT 1
	`, subscriptionID, string(domain.PaymentStatusPending)))
	if err == sql.ErrNoRows {
		return domain.Payment{}, false, nil
	}
	if err != nil {
		return domain.Payment{}, false, err
	}
	return payment, true, nil
}

// UpdatePaymentPaid marks payment as paid and stores provider payment reference.
// Returns true only when row changed from non-paid to paid.
func (s *Store) UpdatePaymentPaid(ctx context.Context, paymentID int64, providerPaymentID string, paidAt time.Time) (bool, error) {
	if paidAt.IsZero() {
		paidAt = time.Now().UTC()
	}
	res, err := s.db.ExecContext(ctx, `
		UPDATE payments
		SET status = $2, provider_payment_id = $3, paid_at = $4, updated_at = $5
		WHERE id = $1
		  AND status <> $2
	`, paymentID, string(domain.PaymentStatusPaid), providerPaymentID, paidAt, time.Now().UTC())
	if err != nil {
		return false, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	if affected > 0 {
		return true, nil
	}
	var exists bool
	if err := s.db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM payments WHERE id = $1)`, paymentID).Scan(&exists); err != nil {
		return false, err
	}
	if !exists {
		return false, errors.New("payment not found")
	}
	return false, nil
}

// UpdatePaymentFailed marks payment as failed and stores provider reference.
// Returns true only when row changed from non-failed to failed.
func (s *Store) UpdatePaymentFailed(ctx context.Context, paymentID int64, providerPaymentID string, updatedAt time.Time) (bool, error) {
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}
	res, err := s.db.ExecContext(ctx, `
		UPDATE payments
		SET status = $2, provider_payment_id = $3, updated_at = $4
		WHERE id = $1
		  AND status = $5
	`, paymentID, string(domain.PaymentStatusFailed), providerPaymentID, updatedAt, string(domain.PaymentStatusPending))
	if err != nil {
		return false, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	if affected > 0 {
		return true, nil
	}
	var (
		exists bool
		status string
	)
	if err := s.db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM payments WHERE id = $1)`, paymentID).Scan(&exists); err != nil {
		return false, err
	}
	if !exists {
		return false, errors.New("payment not found")
	}
	if err := s.db.QueryRowContext(ctx, `SELECT status FROM payments WHERE id = $1`, paymentID).Scan(&status); err == nil && status == string(domain.PaymentStatusPaid) {
		return false, nil
	}
	return false, nil
}

// UpsertSubscriptionByPayment creates/updates access period bound to unique payment.
func (s *Store) UpsertSubscriptionByPayment(ctx context.Context, sub domain.Subscription) error {
	now := time.Now().UTC()
	if sub.CreatedAt.IsZero() {
		sub.CreatedAt = now
	}
	if sub.UpdatedAt.IsZero() {
		sub.UpdatedAt = now
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO subscriptions (
			telegram_id, connector_id, payment_id, status, auto_pay_enabled, starts_at, ends_at, reminder_sent_at, expiry_notice_sent_at, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		ON CONFLICT (payment_id)
		DO UPDATE SET
			status = EXCLUDED.status,
			auto_pay_enabled = EXCLUDED.auto_pay_enabled,
			starts_at = EXCLUDED.starts_at,
			ends_at = EXCLUDED.ends_at,
			reminder_sent_at = EXCLUDED.reminder_sent_at,
			expiry_notice_sent_at = EXCLUDED.expiry_notice_sent_at,
			updated_at = EXCLUDED.updated_at
	`,
		sub.TelegramID,
		sub.ConnectorID,
		sub.PaymentID,
		string(sub.Status),
		sub.AutoPayEnabled,
		sub.StartsAt,
		sub.EndsAt,
		sub.ReminderSentAt,
		sub.ExpiryNoticeSentAt,
		sub.CreatedAt,
		sub.UpdatedAt,
	)
	return err
}

// GetSubscriptionByID fetches subscription by internal identifier.
func (s *Store) GetSubscriptionByID(ctx context.Context, subscriptionID int64) (domain.Subscription, bool, error) {
	item, err := scanSubscription(s.db.QueryRowContext(ctx, `
		SELECT id, telegram_id, connector_id, payment_id, status, auto_pay_enabled, starts_at, ends_at, reminder_sent_at, expiry_notice_sent_at, created_at, updated_at
		FROM subscriptions
		WHERE id = $1
	`, subscriptionID))
	if err == sql.ErrNoRows {
		return domain.Subscription{}, false, nil
	}
	if err != nil {
		return domain.Subscription{}, false, err
	}
	return item, true, nil
}

// GetLatestSubscriptionByUserConnector returns latest subscription by ends_at for user/connector pair.
func (s *Store) GetLatestSubscriptionByUserConnector(ctx context.Context, telegramID, connectorID int64) (domain.Subscription, bool, error) {
	item, err := scanSubscription(s.db.QueryRowContext(ctx, `
		SELECT id, telegram_id, connector_id, payment_id, status, auto_pay_enabled, starts_at, ends_at, reminder_sent_at, expiry_notice_sent_at, created_at, updated_at
		FROM subscriptions
		WHERE telegram_id = $1 AND connector_id = $2
		ORDER BY ends_at DESC, id DESC
		LIMIT 1
	`, telegramID, connectorID))
	if err == sql.ErrNoRows {
		return domain.Subscription{}, false, nil
	}
	if err != nil {
		return domain.Subscription{}, false, err
	}
	return item, true, nil
}

// ListPayments returns recent payments with optional admin filters.
func (s *Store) ListPayments(ctx context.Context, query domain.PaymentListQuery) ([]domain.Payment, error) {
	limit := query.Limit
	if limit <= 0 {
		limit = 200
	}
	if limit > 1000 {
		limit = 1000
	}

	where := make([]string, 0, 6)
	args := make([]any, 0, 8)
	where = append(where, "1=1")
	addArg := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}
	if query.TelegramID > 0 {
		where = append(where, "telegram_id = "+addArg(query.TelegramID))
	}
	if query.ConnectorID > 0 {
		where = append(where, "connector_id = "+addArg(query.ConnectorID))
	}
	if query.Status != "" {
		where = append(where, "status = "+addArg(string(query.Status)))
	}
	if query.CreatedFrom != nil {
		where = append(where, "created_at >= "+addArg(*query.CreatedFrom))
	}
	if query.CreatedToExclude != nil {
		where = append(where, "created_at < "+addArg(*query.CreatedToExclude))
	}
	whereClause := strings.Join(where, " AND ")
	sqlText := fmt.Sprintf(`
		SELECT id, provider, provider_payment_id, status, token, telegram_id, connector_id, subscription_id, parent_payment_id,
		       auto_pay_enabled, amount_rub, checkout_url, created_at, paid_at, updated_at
		FROM payments
		WHERE %s
		ORDER BY created_at DESC, id DESC
		LIMIT %s
	`, whereClause, addArg(limit))

	rows, err := s.db.QueryContext(ctx, sqlText, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]domain.Payment, 0, limit)
	for rows.Next() {
		item, err := scanPayment(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

// ListSubscriptions returns recent subscriptions with optional admin filters.
func (s *Store) ListSubscriptions(ctx context.Context, query domain.SubscriptionListQuery) ([]domain.Subscription, error) {
	limit := query.Limit
	if limit <= 0 {
		limit = 200
	}
	if limit > 1000 {
		limit = 1000
	}

	where := make([]string, 0, 6)
	args := make([]any, 0, 8)
	where = append(where, "1=1")
	addArg := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}
	if query.TelegramID > 0 {
		where = append(where, "telegram_id = "+addArg(query.TelegramID))
	}
	if query.ConnectorID > 0 {
		where = append(where, "connector_id = "+addArg(query.ConnectorID))
	}
	if query.Status != "" {
		where = append(where, "status = "+addArg(string(query.Status)))
	}
	if query.CreatedFrom != nil {
		where = append(where, "created_at >= "+addArg(*query.CreatedFrom))
	}
	if query.CreatedToExclude != nil {
		where = append(where, "created_at < "+addArg(*query.CreatedToExclude))
	}
	whereClause := strings.Join(where, " AND ")
	sqlText := fmt.Sprintf(`
		SELECT id, telegram_id, connector_id, payment_id, status, auto_pay_enabled, starts_at, ends_at, reminder_sent_at, expiry_notice_sent_at, created_at, updated_at
		FROM subscriptions
		WHERE %s
		ORDER BY created_at DESC, id DESC
		LIMIT %s
	`, whereClause, addArg(limit))

	rows, err := s.db.QueryContext(ctx, sqlText, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]domain.Subscription, 0, limit)
	for rows.Next() {
		item, err := scanSubscription(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

// ListSubscriptionsForReminder returns active subscriptions that need a pre-expiration reminder.
func (s *Store) ListSubscriptionsForReminder(ctx context.Context, remindBefore time.Time, limit int) ([]domain.Subscription, error) {
	if limit <= 0 {
		limit = 200
	}
	if limit > 1000 {
		limit = 1000
	}
	now := time.Now().UTC()
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, telegram_id, connector_id, payment_id, status, auto_pay_enabled, starts_at, ends_at, reminder_sent_at, expiry_notice_sent_at, created_at, updated_at
		FROM subscriptions
		WHERE status = $1
		  AND reminder_sent_at IS NULL
		  AND ends_at > $2
		  AND ends_at <= $3
		ORDER BY ends_at ASC
		LIMIT $4
	`, string(domain.SubscriptionStatusActive), now, remindBefore, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]domain.Subscription, 0, limit)
	for rows.Next() {
		item, err := scanSubscription(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// MarkSubscriptionReminderSent stores timestamp when reminder was delivered.
func (s *Store) MarkSubscriptionReminderSent(ctx context.Context, subscriptionID int64, sentAt time.Time) error {
	if sentAt.IsZero() {
		sentAt = time.Now().UTC()
	}
	res, err := s.db.ExecContext(ctx, `
		UPDATE subscriptions
		SET reminder_sent_at = $2, updated_at = $3
		WHERE id = $1
	`, subscriptionID, sentAt, time.Now().UTC())
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return errors.New("subscription not found")
	}
	return nil
}

// ListSubscriptionsForExpiryNotice returns active subscriptions that should receive same-day notice.
func (s *Store) ListSubscriptionsForExpiryNotice(ctx context.Context, noticeBefore time.Time, limit int) ([]domain.Subscription, error) {
	if limit <= 0 {
		limit = 200
	}
	if limit > 1000 {
		limit = 1000
	}
	now := time.Now().UTC()
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, telegram_id, connector_id, payment_id, status, auto_pay_enabled, starts_at, ends_at, reminder_sent_at, expiry_notice_sent_at, created_at, updated_at
		FROM subscriptions
		WHERE status = $1
		  AND expiry_notice_sent_at IS NULL
		  AND ends_at > $2
		  AND ends_at <= $3
		ORDER BY ends_at ASC
		LIMIT $4
	`, string(domain.SubscriptionStatusActive), now, noticeBefore, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]domain.Subscription, 0, limit)
	for rows.Next() {
		item, err := scanSubscription(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// MarkSubscriptionExpiryNoticeSent stores timestamp when same-day notice was delivered.
func (s *Store) MarkSubscriptionExpiryNoticeSent(ctx context.Context, subscriptionID int64, sentAt time.Time) error {
	if sentAt.IsZero() {
		sentAt = time.Now().UTC()
	}
	res, err := s.db.ExecContext(ctx, `
		UPDATE subscriptions
		SET expiry_notice_sent_at = $2, updated_at = $3
		WHERE id = $1
	`, subscriptionID, sentAt, time.Now().UTC())
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return errors.New("subscription not found")
	}
	return nil
}

// ListExpiredActiveSubscriptions returns active subscriptions that already ended.
func (s *Store) ListExpiredActiveSubscriptions(ctx context.Context, now time.Time, limit int) ([]domain.Subscription, error) {
	if limit <= 0 {
		limit = 200
	}
	if limit > 1000 {
		limit = 1000
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, telegram_id, connector_id, payment_id, status, auto_pay_enabled, starts_at, ends_at, reminder_sent_at, expiry_notice_sent_at, created_at, updated_at
		FROM subscriptions
		WHERE status = $1
		  AND ends_at <= $2
		ORDER BY ends_at ASC
		LIMIT $3
	`, string(domain.SubscriptionStatusActive), now, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]domain.Subscription, 0, limit)
	for rows.Next() {
		item, err := scanSubscription(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// UpdateSubscriptionStatus updates subscription status and touched timestamp.
func (s *Store) UpdateSubscriptionStatus(ctx context.Context, subscriptionID int64, status domain.SubscriptionStatus, updatedAt time.Time) error {
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}
	res, err := s.db.ExecContext(ctx, `
		UPDATE subscriptions
		SET status = $2, updated_at = $3
		WHERE id = $1
	`, subscriptionID, string(status), updatedAt)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return errors.New("subscription not found")
	}
	return nil
}
