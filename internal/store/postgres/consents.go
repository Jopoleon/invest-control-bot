package postgres

import (
	"context"
	"database/sql"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
)

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
