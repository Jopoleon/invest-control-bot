package memory

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Jopoleon/telega-bot-fedor/internal/domain"
	storepkg "github.com/Jopoleon/telega-bot-fedor/internal/store"
)

// Store is a thread-safe in-memory implementation used for local development.
type Store struct {
	mu sync.RWMutex

	connectors      map[int64]domain.Connector
	payloadIndex    map[string]int64
	nextConnectorID int64
	legalDocs       map[int64]domain.LegalDocument
	nextLegalDocID  int64
	adminSessions   map[int64]domain.AdminSession
	adminByHash     map[string]int64
	nextAdminSessID int64
	users           map[int64]domain.User
	userAutoPay     map[int64]bool
	consents        map[string]domain.Consent
	states          map[int64]domain.RegistrationState
	events          []domain.AuditEvent
	nextEventID     int64
	payments        map[int64]domain.Payment
	paymentToken    map[string]int64
	nextPaymentID   int64
	subsByPayID     map[int64]domain.Subscription
	nextSubscrID    int64
}

// New creates empty in-memory store.
func New() *Store {
	return &Store{
		connectors:      make(map[int64]domain.Connector),
		payloadIndex:    make(map[string]int64),
		nextConnectorID: 1,
		legalDocs:       make(map[int64]domain.LegalDocument),
		nextLegalDocID:  1,
		adminSessions:   make(map[int64]domain.AdminSession),
		adminByHash:     make(map[string]int64),
		nextAdminSessID: 1,
		users:           make(map[int64]domain.User),
		userAutoPay:     make(map[int64]bool),
		consents:        make(map[string]domain.Consent),
		states:          make(map[int64]domain.RegistrationState),
		events:          make([]domain.AuditEvent, 0, 128),
		nextEventID:     1,
		payments:        make(map[int64]domain.Payment),
		paymentToken:    make(map[string]int64),
		nextPaymentID:   1,
		subsByPayID:     make(map[int64]domain.Subscription),
		nextSubscrID:    1,
	}
}

// CreateAdminSession stores browser admin session.
func (s *Store) CreateAdminSession(_ context.Context, session domain.AdminSession) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	session.ID = s.nextAdminSessID
	s.nextAdminSessID++
	s.adminSessions[session.ID] = session
	s.adminByHash[session.TokenHash] = session.ID
	return nil
}

// GetAdminSessionByTokenHash finds admin session by hashed token.
func (s *Store) GetAdminSessionByTokenHash(_ context.Context, tokenHash string) (domain.AdminSession, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	id, ok := s.adminByHash[tokenHash]
	if !ok {
		return domain.AdminSession{}, false, nil
	}
	session, ok := s.adminSessions[id]
	return session, ok, nil
}

// TouchAdminSession updates last activity timestamp.
func (s *Store) TouchAdminSession(_ context.Context, sessionID int64, lastSeenAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.adminSessions[sessionID]
	if !ok {
		return errors.New("admin session not found")
	}
	session.LastSeenAt = lastSeenAt
	s.adminSessions[sessionID] = session
	return nil
}

// RotateAdminSession swaps token hash for existing session.
func (s *Store) RotateAdminSession(_ context.Context, sessionID int64, newTokenHash string, rotatedAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.adminSessions[sessionID]
	if !ok {
		return errors.New("admin session not found")
	}
	delete(s.adminByHash, session.TokenHash)
	session.ReplacedByHash = newTokenHash
	session.TokenHash = newTokenHash
	session.RotatedAt = &rotatedAt
	session.LastSeenAt = rotatedAt
	s.adminSessions[sessionID] = session
	s.adminByHash[newTokenHash] = sessionID
	return nil
}

// RevokeAdminSession invalidates session immediately.
func (s *Store) RevokeAdminSession(_ context.Context, sessionID int64, revokedAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.adminSessions[sessionID]
	if !ok {
		return errors.New("admin session not found")
	}
	session.RevokedAt = &revokedAt
	s.adminSessions[sessionID] = session
	return nil
}

// CreateLegalDocument stores new versioned legal document.
func (s *Store) CreateLegalDocument(_ context.Context, doc domain.LegalDocument) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	maxVersion := 0
	for _, item := range s.legalDocs {
		if item.Type == doc.Type && item.Version > maxVersion {
			maxVersion = item.Version
		}
	}
	if doc.Version <= 0 {
		doc.Version = maxVersion + 1
	}
	if doc.CreatedAt.IsZero() {
		doc.CreatedAt = time.Now().UTC()
	}
	doc.ID = s.nextLegalDocID
	s.nextLegalDocID++
	s.legalDocs[doc.ID] = doc
	return nil
}

// UpdateLegalDocument updates existing legal document in place.
func (s *Store) UpdateLegalDocument(_ context.Context, doc domain.LegalDocument) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	current, ok := s.legalDocs[doc.ID]
	if !ok {
		return errors.New("legal document not found")
	}
	doc.Type = current.Type
	doc.Version = current.Version
	doc.CreatedAt = current.CreatedAt
	s.legalDocs[doc.ID] = doc
	return nil
}

// ListLegalDocuments returns legal documents ordered by type and version desc.
func (s *Store) ListLegalDocuments(_ context.Context, docType domain.LegalDocumentType) ([]domain.LegalDocument, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	items := make([]domain.LegalDocument, 0, len(s.legalDocs))
	for _, item := range s.legalDocs {
		if docType != "" && item.Type != docType {
			continue
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Type != items[j].Type {
			return items[i].Type < items[j].Type
		}
		if items[i].Version != items[j].Version {
			return items[i].Version > items[j].Version
		}
		return items[i].CreatedAt.After(items[j].CreatedAt)
	})
	return items, nil
}

// GetLegalDocument fetches document by ID.
func (s *Store) GetLegalDocument(_ context.Context, documentID int64) (domain.LegalDocument, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	item, ok := s.legalDocs[documentID]
	return item, ok, nil
}

// GetActiveLegalDocument returns active document for type.
func (s *Store) GetActiveLegalDocument(_ context.Context, docType domain.LegalDocumentType) (domain.LegalDocument, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var (
		best  domain.LegalDocument
		found bool
	)
	for _, item := range s.legalDocs {
		if item.Type != docType || !item.IsActive {
			continue
		}
		if !found || item.Version > best.Version || (item.Version == best.Version && item.ID > best.ID) {
			best = item
			found = true
		}
	}
	return best, found, nil
}

// SetLegalDocumentActive toggles published state for one legal document.
func (s *Store) SetLegalDocumentActive(_ context.Context, documentID int64, active bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	doc, ok := s.legalDocs[documentID]
	if !ok {
		return errors.New("legal document not found")
	}
	doc.IsActive = active
	s.legalDocs[documentID] = doc
	return nil
}

// DeleteLegalDocument removes legal document by ID.
func (s *Store) DeleteLegalDocument(_ context.Context, documentID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.legalDocs[documentID]; !ok {
		return errors.New("legal document not found")
	}
	delete(s.legalDocs, documentID)
	return nil
}

// CreateConnector inserts new connector and maintains start_payload index.
func (s *Store) CreateConnector(_ context.Context, c domain.Connector) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if c.StartPayload == "" {
		return errors.New("start payload is required")
	}
	payloadKey := strings.TrimSpace(c.StartPayload)
	if payloadKey == "" {
		return errors.New("start payload is required")
	}
	if _, exists := s.payloadIndex[payloadKey]; exists {
		return errors.New("start payload already exists")
	}

	if c.CreatedAt.IsZero() {
		c.CreatedAt = time.Now().UTC()
	}
	c.ID = s.nextConnectorID
	s.nextConnectorID++
	s.connectors[c.ID] = c
	s.payloadIndex[payloadKey] = c.ID
	return nil
}

// ListConnectors returns connectors ordered by creation time.
func (s *Store) ListConnectors(_ context.Context) ([]domain.Connector, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]domain.Connector, 0, len(s.connectors))
	for _, c := range s.connectors {
		result = append(result, c)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.Before(result[j].CreatedAt)
	})
	return result, nil
}

// GetConnector fetches connector by internal ID.
func (s *Store) GetConnector(_ context.Context, connectorID int64) (domain.Connector, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	c, ok := s.connectors[connectorID]
	return c, ok, nil
}

// GetConnectorByStartPayload fetches connector by start payload token.
func (s *Store) GetConnectorByStartPayload(_ context.Context, payload string) (domain.Connector, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	connectorID, ok := s.payloadIndex[strings.TrimSpace(payload)]
	if !ok {
		return domain.Connector{}, false, nil
	}
	c, exists := s.connectors[connectorID]
	if !exists {
		return domain.Connector{}, false, nil
	}
	return c, true, nil
}

// SetConnectorActive toggles connector active state.
func (s *Store) SetConnectorActive(_ context.Context, connectorID int64, active bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	c, ok := s.connectors[connectorID]
	if !ok {
		return storepkg.ErrConnectorNotFound
	}
	c.IsActive = active
	s.connectors[connectorID] = c
	return nil
}

// DeleteConnector removes connector only when there is no dependent state/history.
func (s *Store) DeleteConnector(_ context.Context, connectorID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	c, ok := s.connectors[connectorID]
	if !ok {
		return storepkg.ErrConnectorNotFound
	}
	for _, payment := range s.payments {
		if payment.ConnectorID == connectorID {
			return storepkg.ErrConnectorInUse
		}
	}
	for _, sub := range s.subsByPayID {
		if sub.ConnectorID == connectorID {
			return storepkg.ErrConnectorInUse
		}
	}
	for _, consent := range s.consents {
		if consent.ConnectorID == connectorID {
			return storepkg.ErrConnectorInUse
		}
	}
	for _, state := range s.states {
		if state.ConnectorID == connectorID {
			return storepkg.ErrConnectorInUse
		}
	}

	delete(s.payloadIndex, c.StartPayload)
	delete(s.connectors, connectorID)
	return nil
}

// SaveConsent stores user legal acceptance for connector.
func (s *Store) SaveConsent(_ context.Context, consent domain.Consent) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.consents[consentKey(consent.TelegramID, consent.ConnectorID)] = consent
	return nil
}

// GetConsent returns stored consent by user and connector.
func (s *Store) GetConsent(_ context.Context, telegramID int64, connectorID int64) (domain.Consent, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	consent, ok := s.consents[consentKey(telegramID, connectorID)]
	return consent, ok, nil
}

// SaveUser upserts user profile.
func (s *Store) SaveUser(_ context.Context, user domain.User) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	user.UpdatedAt = time.Now().UTC()
	s.users[user.TelegramID] = user
	return nil
}

// GetUser fetches user profile by Telegram ID.
func (s *Store) GetUser(_ context.Context, telegramID int64) (domain.User, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	u, ok := s.users[telegramID]
	return u, ok, nil
}

// ListUsers returns filtered users for admin screens.
func (s *Store) ListUsers(_ context.Context, query domain.UserListQuery) ([]domain.UserListItem, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if query.Limit <= 0 {
		query.Limit = 200
	}
	search := strings.ToLower(strings.TrimSpace(query.Search))

	items := make([]domain.UserListItem, 0, len(s.users))
	for _, user := range s.users {
		if query.TelegramID > 0 && user.TelegramID != query.TelegramID {
			continue
		}
		if search != "" {
			haystack := strings.ToLower(strings.Join([]string{
				strconv.FormatInt(user.TelegramID, 10),
				user.TelegramUsername,
				user.FullName,
				user.Phone,
				user.Email,
			}, " "))
			if !strings.Contains(haystack, search) {
				continue
			}
		}
		autoPay, hasAutoPay := s.userAutoPay[user.TelegramID]
		items = append(items, domain.UserListItem{
			TelegramID:         user.TelegramID,
			TelegramUsername:   user.TelegramUsername,
			FullName:           user.FullName,
			Phone:              user.Phone,
			Email:              user.Email,
			AutoPayEnabled:     autoPay,
			HasAutoPaySettings: hasAutoPay,
			UpdatedAt:          user.UpdatedAt,
		})
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].UpdatedAt.Equal(items[j].UpdatedAt) {
			return items[i].TelegramID > items[j].TelegramID
		}
		return items[i].UpdatedAt.After(items[j].UpdatedAt)
	})

	if len(items) > query.Limit {
		items = items[:query.Limit]
	}
	return items, nil
}

// SetUserAutoPayEnabled stores user recurring preference used for next payment link creation.
func (s *Store) SetUserAutoPayEnabled(_ context.Context, telegramID int64, enabled bool, _ time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.userAutoPay[telegramID] = enabled
	return nil
}

// GetUserAutoPayEnabled returns user recurring preference if explicitly set.
func (s *Store) GetUserAutoPayEnabled(_ context.Context, telegramID int64) (bool, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	enabled, ok := s.userAutoPay[telegramID]
	return enabled, ok, nil
}

// SaveRegistrationState stores FSM progress for user.
func (s *Store) SaveRegistrationState(_ context.Context, state domain.RegistrationState) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	state.UpdatedAt = time.Now().UTC()
	s.states[state.TelegramID] = state
	return nil
}

// GetRegistrationState fetches FSM progress for user.
func (s *Store) GetRegistrationState(_ context.Context, telegramID int64) (domain.RegistrationState, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	state, ok := s.states[telegramID]
	return state, ok, nil
}

// DeleteRegistrationState clears FSM state after completion/cancel.
func (s *Store) DeleteRegistrationState(_ context.Context, telegramID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.states, telegramID)
	return nil
}

// SaveAuditEvent appends immutable action event to in-memory list.
func (s *Store) SaveAuditEvent(_ context.Context, event domain.AuditEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}
	event.ID = s.nextEventID
	s.nextEventID++
	s.events = append(s.events, event)
	return nil
}

// ListAuditEvents returns most recent events first.
func (s *Store) ListAuditEvents(_ context.Context, query domain.AuditEventListQuery) ([]domain.AuditEvent, int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if query.Page <= 0 {
		query.Page = 1
	}
	if query.PageSize <= 0 {
		query.PageSize = 50
	}

	filtered := make([]domain.AuditEvent, 0, len(s.events))
	for _, event := range s.events {
		if query.TelegramID > 0 && event.TelegramID != query.TelegramID {
			continue
		}
		if query.ConnectorID > 0 && event.ConnectorID != query.ConnectorID {
			continue
		}
		if query.Action != "" && event.Action != query.Action {
			continue
		}
		if query.Search != "" && !strings.Contains(strings.ToLower(event.Details), strings.ToLower(query.Search)) {
			continue
		}
		if query.CreatedFrom != nil && event.CreatedAt.Before(*query.CreatedFrom) {
			continue
		}
		if query.CreatedToExclude != nil && !event.CreatedAt.Before(*query.CreatedToExclude) {
			continue
		}
		filtered = append(filtered, event)
	}

	total := len(filtered)
	sortBy := query.SortBy
	if sortBy == "" {
		sortBy = "created_at"
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		left := filtered[i]
		right := filtered[j]

		var cmp int
		switch sortBy {
		case "telegram_id":
			switch {
			case left.TelegramID < right.TelegramID:
				cmp = -1
			case left.TelegramID > right.TelegramID:
				cmp = 1
			}
		case "connector_id":
			switch {
			case left.ConnectorID < right.ConnectorID:
				cmp = -1
			case left.ConnectorID > right.ConnectorID:
				cmp = 1
			}
		case "action":
			cmp = strings.Compare(left.Action, right.Action)
		default:
			switch {
			case left.CreatedAt.Before(right.CreatedAt):
				cmp = -1
			case left.CreatedAt.After(right.CreatedAt):
				cmp = 1
			}
		}
		if cmp == 0 {
			switch {
			case left.ID < right.ID:
				cmp = -1
			case left.ID > right.ID:
				cmp = 1
			}
		}
		if query.SortDesc {
			return cmp > 0
		}
		return cmp < 0
	})

	offset := (query.Page - 1) * query.PageSize
	if offset >= total {
		return []domain.AuditEvent{}, total, nil
	}
	end := offset + query.PageSize
	if end > total {
		end = total
	}
	return filtered[offset:end], total, nil
}

// CreatePayment stores pending payment transaction by ID and token.
func (s *Store) CreatePayment(_ context.Context, payment domain.Payment) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if payment.Token == "" {
		return errors.New("payment token is required")
	}
	if _, exists := s.paymentToken[payment.Token]; exists {
		return errors.New("payment token already exists")
	}
	if payment.SubscriptionID > 0 && payment.Status == domain.PaymentStatusPending {
		for _, existing := range s.payments {
			if existing.SubscriptionID == payment.SubscriptionID && existing.Status == domain.PaymentStatusPending {
				return errors.New("pending rebill already exists for subscription")
			}
		}
	}
	now := time.Now().UTC()
	if payment.CreatedAt.IsZero() {
		payment.CreatedAt = now
	}
	payment.ID = s.nextPaymentID
	s.nextPaymentID++
	payment.UpdatedAt = now
	s.payments[payment.ID] = payment
	s.paymentToken[payment.Token] = payment.ID
	return nil
}

// GetPaymentByToken finds payment by externally visible token.
func (s *Store) GetPaymentByToken(_ context.Context, token string) (domain.Payment, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	paymentID, ok := s.paymentToken[token]
	if !ok {
		return domain.Payment{}, false, nil
	}
	payment, exists := s.payments[paymentID]
	if !exists {
		return domain.Payment{}, false, nil
	}
	return payment, true, nil
}

// GetPaymentByID fetches payment by internal DB identifier.
func (s *Store) GetPaymentByID(_ context.Context, paymentID int64) (domain.Payment, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.payments[paymentID]
	return p, ok, nil
}

// GetPendingRebillPaymentBySubscription returns outstanding recurring attempt for subscription.
func (s *Store) GetPendingRebillPaymentBySubscription(_ context.Context, subscriptionID int64) (domain.Payment, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var (
		best  domain.Payment
		found bool
	)
	for _, payment := range s.payments {
		if payment.SubscriptionID != subscriptionID || payment.Status != domain.PaymentStatusPending {
			continue
		}
		if !found || payment.CreatedAt.After(best.CreatedAt) || (payment.CreatedAt.Equal(best.CreatedAt) && payment.ID > best.ID) {
			best = payment
			found = true
		}
	}
	return best, found, nil
}

// UpdatePaymentPaid moves payment into paid state and stores provider reference.
// Returns true only when state changed from non-paid to paid.
func (s *Store) UpdatePaymentPaid(_ context.Context, paymentID int64, providerPaymentID string, paidAt time.Time) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	payment, ok := s.payments[paymentID]
	if !ok {
		return false, errors.New("payment not found")
	}
	if payment.Status == domain.PaymentStatusPaid {
		return false, nil
	}
	if paidAt.IsZero() {
		paidAt = time.Now().UTC()
	}
	payment.Status = domain.PaymentStatusPaid
	payment.ProviderPaymentID = providerPaymentID
	payment.PaidAt = &paidAt
	payment.UpdatedAt = time.Now().UTC()
	s.payments[paymentID] = payment
	return true, nil
}

// UpdatePaymentFailed marks payment as failed (idempotent for already-failed rows).
func (s *Store) UpdatePaymentFailed(_ context.Context, paymentID int64, providerPaymentID string, updatedAt time.Time) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	payment, ok := s.payments[paymentID]
	if !ok {
		return false, errors.New("payment not found")
	}
	if payment.Status == domain.PaymentStatusPaid {
		return false, nil
	}
	if payment.Status == domain.PaymentStatusFailed {
		return false, nil
	}
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}
	payment.Status = domain.PaymentStatusFailed
	payment.ProviderPaymentID = providerPaymentID
	payment.UpdatedAt = updatedAt
	s.payments[paymentID] = payment
	return true, nil
}

// UpsertSubscriptionByPayment creates or updates subscription by unique payment ID.
func (s *Store) UpsertSubscriptionByPayment(_ context.Context, sub domain.Subscription) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if sub.PaymentID <= 0 {
		return errors.New("payment ID is required")
	}
	now := time.Now().UTC()
	if sub.CreatedAt.IsZero() {
		sub.CreatedAt = now
	}
	// Re-activation means reminder should be sent again for the new period.
	sub.ReminderSentAt = nil
	sub.ExpiryNoticeSentAt = nil
	if sub.ID <= 0 {
		sub.ID = s.nextSubscrID
		s.nextSubscrID++
	}
	sub.UpdatedAt = now
	s.subsByPayID[sub.PaymentID] = sub
	return nil
}

// GetSubscriptionByID fetches subscription by internal identifier.
func (s *Store) GetSubscriptionByID(_ context.Context, subscriptionID int64) (domain.Subscription, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, sub := range s.subsByPayID {
		if sub.ID == subscriptionID {
			return sub, true, nil
		}
	}
	return domain.Subscription{}, false, nil
}

// GetLatestSubscriptionByUserConnector returns latest subscription by ends_at for pair.
func (s *Store) GetLatestSubscriptionByUserConnector(_ context.Context, telegramID, connectorID int64) (domain.Subscription, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var (
		best  domain.Subscription
		found bool
	)
	for _, sub := range s.subsByPayID {
		if sub.TelegramID != telegramID || sub.ConnectorID != connectorID {
			continue
		}
		if !found || sub.EndsAt.After(best.EndsAt) {
			best = sub
			found = true
		}
	}
	if !found {
		return domain.Subscription{}, false, nil
	}
	return best, true, nil
}

// ListPayments returns recent payments using admin filters.
func (s *Store) ListPayments(_ context.Context, query domain.PaymentListQuery) ([]domain.Payment, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	limit := query.Limit
	if limit <= 0 {
		limit = 200
	}
	if limit > 1000 {
		limit = 1000
	}

	filtered := make([]domain.Payment, 0, len(s.payments))
	for _, item := range s.payments {
		if query.TelegramID > 0 && item.TelegramID != query.TelegramID {
			continue
		}
		if query.ConnectorID > 0 && item.ConnectorID != query.ConnectorID {
			continue
		}
		if query.Status != "" && item.Status != query.Status {
			continue
		}
		if query.CreatedFrom != nil && item.CreatedAt.Before(*query.CreatedFrom) {
			continue
		}
		if query.CreatedToExclude != nil && !item.CreatedAt.Before(*query.CreatedToExclude) {
			continue
		}
		filtered = append(filtered, item)
	}
	sort.Slice(filtered, func(i, j int) bool {
		if filtered[i].CreatedAt.Equal(filtered[j].CreatedAt) {
			return filtered[i].ID > filtered[j].ID
		}
		return filtered[i].CreatedAt.After(filtered[j].CreatedAt)
	})
	if len(filtered) > limit {
		filtered = filtered[:limit]
	}
	return filtered, nil
}

// ListSubscriptions returns recent subscriptions using admin filters.
func (s *Store) ListSubscriptions(_ context.Context, query domain.SubscriptionListQuery) ([]domain.Subscription, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	limit := query.Limit
	if limit <= 0 {
		limit = 200
	}
	if limit > 1000 {
		limit = 1000
	}

	filtered := make([]domain.Subscription, 0, len(s.subsByPayID))
	for _, item := range s.subsByPayID {
		if query.TelegramID > 0 && item.TelegramID != query.TelegramID {
			continue
		}
		if query.ConnectorID > 0 && item.ConnectorID != query.ConnectorID {
			continue
		}
		if query.Status != "" && item.Status != query.Status {
			continue
		}
		if query.CreatedFrom != nil && item.CreatedAt.Before(*query.CreatedFrom) {
			continue
		}
		if query.CreatedToExclude != nil && !item.CreatedAt.Before(*query.CreatedToExclude) {
			continue
		}
		filtered = append(filtered, item)
	}
	sort.Slice(filtered, func(i, j int) bool {
		if filtered[i].CreatedAt.Equal(filtered[j].CreatedAt) {
			return filtered[i].ID > filtered[j].ID
		}
		return filtered[i].CreatedAt.After(filtered[j].CreatedAt)
	})
	if len(filtered) > limit {
		filtered = filtered[:limit]
	}
	return filtered, nil
}

// ListSubscriptionsForReminder returns active subscriptions that are ending soon and not yet notified.
func (s *Store) ListSubscriptionsForReminder(_ context.Context, remindBefore time.Time, limit int) ([]domain.Subscription, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 {
		limit = 200
	}
	if limit > 1000 {
		limit = 1000
	}
	now := time.Now().UTC()
	filtered := make([]domain.Subscription, 0, len(s.subsByPayID))
	for _, item := range s.subsByPayID {
		if item.Status != domain.SubscriptionStatusActive {
			continue
		}
		if item.ReminderSentAt != nil {
			continue
		}
		if !item.EndsAt.After(now) {
			continue
		}
		if item.EndsAt.After(remindBefore) {
			continue
		}
		filtered = append(filtered, item)
	}
	sort.Slice(filtered, func(i, j int) bool {
		if filtered[i].EndsAt.Equal(filtered[j].EndsAt) {
			return filtered[i].ID < filtered[j].ID
		}
		return filtered[i].EndsAt.Before(filtered[j].EndsAt)
	})
	if len(filtered) > limit {
		filtered = filtered[:limit]
	}
	return filtered, nil
}

// MarkSubscriptionReminderSent stores reminder timestamp.
func (s *Store) MarkSubscriptionReminderSent(_ context.Context, subscriptionID int64, sentAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if sentAt.IsZero() {
		sentAt = time.Now().UTC()
	}
	for paymentID, sub := range s.subsByPayID {
		if sub.ID != subscriptionID {
			continue
		}
		sub.ReminderSentAt = &sentAt
		sub.UpdatedAt = time.Now().UTC()
		s.subsByPayID[paymentID] = sub
		return nil
	}
	return errors.New("subscription not found")
}

// ListSubscriptionsForExpiryNotice returns active subscriptions that are ending within the next day.
func (s *Store) ListSubscriptionsForExpiryNotice(_ context.Context, noticeBefore time.Time, limit int) ([]domain.Subscription, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 {
		limit = 200
	}
	if limit > 1000 {
		limit = 1000
	}
	now := time.Now().UTC()
	filtered := make([]domain.Subscription, 0, len(s.subsByPayID))
	for _, item := range s.subsByPayID {
		if item.Status != domain.SubscriptionStatusActive {
			continue
		}
		if item.ExpiryNoticeSentAt != nil {
			continue
		}
		if !item.EndsAt.After(now) {
			continue
		}
		if item.EndsAt.After(noticeBefore) {
			continue
		}
		filtered = append(filtered, item)
	}
	sort.Slice(filtered, func(i, j int) bool {
		if filtered[i].EndsAt.Equal(filtered[j].EndsAt) {
			return filtered[i].ID < filtered[j].ID
		}
		return filtered[i].EndsAt.Before(filtered[j].EndsAt)
	})
	if len(filtered) > limit {
		filtered = filtered[:limit]
	}
	return filtered, nil
}

// MarkSubscriptionExpiryNoticeSent stores same-day notice timestamp.
func (s *Store) MarkSubscriptionExpiryNoticeSent(_ context.Context, subscriptionID int64, sentAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if sentAt.IsZero() {
		sentAt = time.Now().UTC()
	}
	for paymentID, sub := range s.subsByPayID {
		if sub.ID != subscriptionID {
			continue
		}
		sub.ExpiryNoticeSentAt = &sentAt
		sub.UpdatedAt = time.Now().UTC()
		s.subsByPayID[paymentID] = sub
		return nil
	}
	return errors.New("subscription not found")
}

// ListExpiredActiveSubscriptions returns active subscriptions whose end time is in the past.
func (s *Store) ListExpiredActiveSubscriptions(_ context.Context, now time.Time, limit int) ([]domain.Subscription, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 {
		limit = 200
	}
	if limit > 1000 {
		limit = 1000
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	filtered := make([]domain.Subscription, 0, len(s.subsByPayID))
	for _, item := range s.subsByPayID {
		if item.Status != domain.SubscriptionStatusActive {
			continue
		}
		if item.EndsAt.After(now) {
			continue
		}
		filtered = append(filtered, item)
	}
	sort.Slice(filtered, func(i, j int) bool {
		if filtered[i].EndsAt.Equal(filtered[j].EndsAt) {
			return filtered[i].ID < filtered[j].ID
		}
		return filtered[i].EndsAt.Before(filtered[j].EndsAt)
	})
	if len(filtered) > limit {
		filtered = filtered[:limit]
	}
	return filtered, nil
}

// UpdateSubscriptionStatus updates status by subscription ID.
func (s *Store) UpdateSubscriptionStatus(_ context.Context, subscriptionID int64, status domain.SubscriptionStatus, updatedAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}
	for paymentID, sub := range s.subsByPayID {
		if sub.ID != subscriptionID {
			continue
		}
		sub.Status = status
		sub.UpdatedAt = updatedAt
		s.subsByPayID[paymentID] = sub
		return nil
	}
	return errors.New("subscription not found")
}

// consentKey builds deterministic compound key for consent map.
func consentKey(telegramID, connectorID int64) string {
	return int64ToString(connectorID) + ":" + int64ToString(telegramID)
}

// int64ToString converts int64 values for map key composition.
func int64ToString(v int64) string {
	return strconv.FormatInt(v, 10)
}
