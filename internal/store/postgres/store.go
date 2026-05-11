package postgres

import (
	"database/sql"
	"encoding/json"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
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
		&payment.UserID,
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
		&item.UserID,
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

func scanTelegramInviteLink(scanner rowScanner) (domain.TelegramInviteLink, error) {
	var item domain.TelegramInviteLink
	err := scanner.Scan(
		&item.ID,
		&item.UserID,
		&item.ConnectorID,
		&item.SubscriptionID,
		&item.ChatRef,
		&item.InviteLink,
		&item.ExpiresAt,
		&item.RevokedAt,
		&item.CreatedAt,
	)
	if err != nil {
		return domain.TelegramInviteLink{}, err
	}
	return item, nil
}

func scanTelegramChat(scanner rowScanner) (domain.TelegramChat, error) {
	var item domain.TelegramChat
	err := scanner.Scan(
		&item.ChatID,
		&item.Title,
		&item.Username,
		&item.Type,
		&item.BotStatus,
		&item.CanInviteUsers,
		&item.CanRestrictMembers,
		&item.CanManageChat,
		&item.LastSeenAt,
		&item.LastVerifiedAt,
		&item.UpdatedAt,
	)
	if err != nil {
		return domain.TelegramChat{}, err
	}
	return item, nil
}

func scanMessengerChatUser(scanner rowScanner) (domain.MessengerChatUser, error) {
	var (
		item         domain.MessengerChatUser
		kind         string
		status       string
		permissions  []byte
		lastJoinedAt sql.NullTime
		lastLeftAt   sql.NullTime
		lastKickedAt sql.NullTime
		lastUnbanned sql.NullTime
		userID       sql.NullInt64
	)
	err := scanner.Scan(
		&kind,
		&item.ChatRef,
		&item.MessengerUserID,
		&userID,
		&status,
		&item.IsMember,
		&item.IsBanned,
		&item.IsRestricted,
		&item.IsAdmin,
		&item.IsOwner,
		&permissions,
		&item.CheckedAt,
		&lastJoinedAt,
		&lastLeftAt,
		&lastKickedAt,
		&lastUnbanned,
		&item.LastError,
		&item.UpdatedAt,
	)
	if err != nil {
		return domain.MessengerChatUser{}, err
	}
	item.MessengerKind = domain.MessengerKind(kind)
	item.Status = domain.MessengerChatUserStatus(status)
	item.UserID = userID.Int64
	if len(permissions) > 0 {
		if err := json.Unmarshal(permissions, &item.Permissions); err != nil {
			return domain.MessengerChatUser{}, err
		}
	}
	if lastJoinedAt.Valid {
		item.LastJoinedAt = &lastJoinedAt.Time
	}
	if lastLeftAt.Valid {
		item.LastLeftAt = &lastLeftAt.Time
	}
	if lastKickedAt.Valid {
		item.LastKickedAt = &lastKickedAt.Time
	}
	if lastUnbanned.Valid {
		item.LastUnbannedAt = &lastUnbanned.Time
	}
	return item, nil
}

func scanMessengerChatUserCheck(scanner rowScanner) (domain.MessengerChatUserCheck, error) {
	var (
		item           domain.MessengerChatUserCheck
		kind           string
		status         string
		userID         sql.NullInt64
		connectorID    sql.NullInt64
		subscriptionID sql.NullInt64
		rawJSON        []byte
	)
	err := scanner.Scan(
		&item.ID,
		&kind,
		&item.ChatRef,
		&item.MessengerUserID,
		&userID,
		&connectorID,
		&subscriptionID,
		&item.Source,
		&status,
		&item.OK,
		&rawJSON,
		&item.RawError,
		&item.CheckedAt,
	)
	if err != nil {
		return domain.MessengerChatUserCheck{}, err
	}
	item.MessengerKind = domain.MessengerKind(kind)
	item.Status = domain.MessengerChatUserStatus(status)
	item.UserID = userID.Int64
	item.ConnectorID = connectorID.Int64
	item.SubscriptionID = subscriptionID.Int64
	item.RawJSON = append(item.RawJSON[:0], rawJSON...)
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

func nullableInt64(value int64) sql.NullInt64 {
	if value <= 0 {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: value, Valid: true}
}
