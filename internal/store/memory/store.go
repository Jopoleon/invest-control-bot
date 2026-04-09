package memory

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
	storepkg "github.com/Jopoleon/invest-control-bot/internal/store"
)

// Store is a thread-safe in-memory implementation used for local development.
type Store struct {
	mu sync.RWMutex

	connectors             map[int64]domain.Connector
	payloadIndex           map[string]int64
	nextConnectorID        int64
	legalDocs              map[int64]domain.LegalDocument
	nextLegalDocID         int64
	adminSessions          map[int64]domain.AdminSession
	adminByHash            map[string]int64
	nextAdminSessID        int64
	users                  map[int64]domain.User
	userIDByTelegram       map[int64]int64
	messengerAccounts      map[string]domain.UserMessengerAccount
	nextUserID             int64
	consents               map[string]domain.Consent
	recurringConsents      []domain.RecurringConsent
	states                 map[string]domain.RegistrationState
	events                 []domain.AuditEvent
	nextEventID            int64
	payments               map[int64]domain.Payment
	paymentToken           map[string]int64
	nextPaymentID          int64
	subsByPayID            map[int64]domain.Subscription
	nextSubscrID           int64
	telegramInviteLinks    map[int64]domain.TelegramInviteLink
	nextTelegramInviteID   int64
	nextRecurringConsentID int64
}

// New creates empty in-memory store.
func New() *Store {
	return &Store{
		connectors:             make(map[int64]domain.Connector),
		payloadIndex:           make(map[string]int64),
		nextConnectorID:        1,
		legalDocs:              make(map[int64]domain.LegalDocument),
		nextLegalDocID:         1,
		adminSessions:          make(map[int64]domain.AdminSession),
		adminByHash:            make(map[string]int64),
		nextAdminSessID:        1,
		users:                  make(map[int64]domain.User),
		userIDByTelegram:       make(map[int64]int64),
		messengerAccounts:      make(map[string]domain.UserMessengerAccount),
		nextUserID:             1,
		consents:               make(map[string]domain.Consent),
		recurringConsents:      make([]domain.RecurringConsent, 0),
		states:                 make(map[string]domain.RegistrationState),
		events:                 make([]domain.AuditEvent, 0, 128),
		nextEventID:            1,
		payments:               make(map[int64]domain.Payment),
		paymentToken:           make(map[string]int64),
		nextPaymentID:          1,
		subsByPayID:            make(map[int64]domain.Subscription),
		nextSubscrID:           1,
		telegramInviteLinks:    make(map[int64]domain.TelegramInviteLink),
		nextTelegramInviteID:   1,
		nextRecurringConsentID: 1,
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

// ListAdminSessions returns admin sessions ordered by newest first.
func (s *Store) ListAdminSessions(_ context.Context, limit int) ([]domain.AdminSession, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 {
		limit = 100
	}
	items := make([]domain.AdminSession, 0, len(s.adminSessions))
	for _, session := range s.adminSessions {
		items = append(items, session)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].ID > items[j].ID
		}
		return items[i].CreatedAt.After(items[j].CreatedAt)
	})
	if len(items) > limit {
		items = items[:limit]
	}
	return items, nil
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

// CleanupAdminSessions removes revoked and absolute-expired sessions.
func (s *Store) CleanupAdminSessions(_ context.Context, expiredBefore time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for id, session := range s.adminSessions {
		if session.RevokedAt != nil || session.ExpiresAt.Before(expiredBefore) {
			delete(s.adminByHash, session.TokenHash)
			delete(s.adminSessions, id)
		}
	}
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

	s.consents[consentKey(consent.UserID, consent.ConnectorID)] = consent
	return nil
}

// GetConsent returns stored consent by user and connector.
func (s *Store) GetConsent(_ context.Context, userID int64, connectorID int64) (domain.Consent, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	consent, ok := s.consents[consentKey(userID, connectorID)]
	return consent, ok, nil
}

// ListConsentsByUser returns all consent records for one internal user.
func (s *Store) ListConsentsByUser(_ context.Context, userID int64) ([]domain.Consent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	items := make([]domain.Consent, 0)
	for _, consent := range s.consents {
		if consent.UserID != userID {
			continue
		}
		items = append(items, consent)
	}
	sort.Slice(items, func(i, j int) bool {
		left := items[i].OfferAcceptedAt
		if items[i].PrivacyAcceptedAt.After(left) {
			left = items[i].PrivacyAcceptedAt
		}
		right := items[j].OfferAcceptedAt
		if items[j].PrivacyAcceptedAt.After(right) {
			right = items[j].PrivacyAcceptedAt
		}
		if !left.Equal(right) {
			return left.After(right)
		}
		return items[i].ConnectorID > items[j].ConnectorID
	})
	return items, nil
}

// CreateRecurringConsent stores explicit recurring/autopay consent event.
func (s *Store) CreateRecurringConsent(_ context.Context, consent domain.RecurringConsent) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	consent.ID = s.nextRecurringConsentID
	s.nextRecurringConsentID++
	s.recurringConsents = append(s.recurringConsents, consent)
	return nil
}

// ListRecurringConsentsByUser returns recurring consent history for one user.
func (s *Store) ListRecurringConsentsByUser(_ context.Context, userID int64) ([]domain.RecurringConsent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	items := make([]domain.RecurringConsent, 0)
	for _, consent := range s.recurringConsents {
		if consent.UserID != userID {
			continue
		}
		items = append(items, consent)
	}
	sort.Slice(items, func(i, j int) bool {
		if !items[i].AcceptedAt.Equal(items[j].AcceptedAt) {
			return items[i].AcceptedAt.After(items[j].AcceptedAt)
		}
		return items[i].ID > items[j].ID
	})
	return items, nil
}

// SaveUser upserts user profile.
func (s *Store) SaveUser(_ context.Context, user domain.User) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	user.UpdatedAt = time.Now().UTC()
	if user.ID <= 0 {
		user.ID = s.nextUserID
		s.nextUserID++
	}
	if existing, ok := s.users[user.ID]; ok && !existing.CreatedAt.IsZero() {
		user.CreatedAt = existing.CreatedAt
	}
	if user.CreatedAt.IsZero() {
		user.CreatedAt = user.UpdatedAt
	}

	s.users[user.ID] = user
	return nil
}

// GetUserByID fetches user profile by internal user ID.
func (s *Store) GetUserByID(_ context.Context, userID int64) (domain.User, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	u, ok := s.users[userID]
	return u, ok, nil
}

// GetUser fetches user profile by Telegram ID.
func (s *Store) GetUser(_ context.Context, telegramID int64) (domain.User, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	userID, ok := s.userIDByTelegram[telegramID]
	if !ok {
		return domain.User{}, false, nil
	}
	u, ok := s.users[userID]
	return u, ok, nil
}

// GetUserByMessenger resolves an existing user by external messenger identity without creating one.
func (s *Store) GetUserByMessenger(_ context.Context, kind domain.MessengerKind, messengerUserID string) (domain.User, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if account, ok := s.messengerAccounts[messengerAccountKey(kind, messengerUserID)]; ok {
		user, found := s.users[account.UserID]
		return user, found, nil
	}
	return domain.User{}, false, nil
}

// GetOrCreateUserByMessenger resolves a user by external messenger identity or creates one if absent.
func (s *Store) GetOrCreateUserByMessenger(_ context.Context, kind domain.MessengerKind, messengerUserID, username string) (domain.User, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := messengerAccountKey(kind, messengerUserID)
	if account, ok := s.messengerAccounts[key]; ok {
		user := s.users[account.UserID]
		if strings.TrimSpace(username) != "" && account.Username != username {
			account.Username = username
			account.UpdatedAt = time.Now().UTC()
			s.messengerAccounts[key] = account
		}
		return user, false, nil
	}

	user := domain.User{
		ID:        s.nextUserID,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	s.nextUserID++
	s.users[user.ID] = user
	s.upsertMessengerAccountLocked(domain.UserMessengerAccount{
		UserID:          user.ID,
		MessengerKind:   kind,
		MessengerUserID: messengerUserID,
		Username:        username,
		LinkedAt:        user.UpdatedAt,
		UpdatedAt:       user.UpdatedAt,
	})
	return user, true, nil
}

// ListUserMessengerAccounts returns linked messenger identities for one internal user.
func (s *Store) ListUserMessengerAccounts(_ context.Context, userID int64) ([]domain.UserMessengerAccount, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	items := make([]domain.UserMessengerAccount, 0)
	for _, account := range s.messengerAccounts {
		if account.UserID != userID {
			continue
		}
		items = append(items, account)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].MessengerKind != items[j].MessengerKind {
			return items[i].MessengerKind < items[j].MessengerKind
		}
		return items[i].MessengerUserID < items[j].MessengerUserID
	})
	return items, nil
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
		telegramID, telegramUsername := s.telegramIdentityLocked(user.ID)
		if query.UserID > 0 && user.ID != query.UserID {
			continue
		}
		if query.TelegramID > 0 && telegramID != query.TelegramID {
			continue
		}
		if search != "" {
			haystack := strings.ToLower(strings.Join([]string{
				strconv.FormatInt(telegramID, 10),
				telegramUsername,
				user.FullName,
				user.Phone,
				user.Email,
			}, " "))
			if !strings.Contains(haystack, search) {
				continue
			}
		}
		autoPay, hasAutoPay := s.userAutopaySummaryLocked(user.ID)
		items = append(items, domain.UserListItem{
			UserID:             user.ID,
			TelegramID:         telegramID,
			TelegramUsername:   telegramUsername,
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
			return items[i].UserID > items[j].UserID
		}
		return items[i].UpdatedAt.After(items[j].UpdatedAt)
	})

	if len(items) > query.Limit {
		items = items[:query.Limit]
	}
	return items, nil
}

func (s *Store) userAutopaySummaryLocked(userID int64) (bool, bool) {
	hasActive := false
	enabled := false
	for _, sub := range s.subsByPayID {
		if sub.UserID != userID || sub.Status != domain.SubscriptionStatusActive {
			continue
		}
		hasActive = true
		if sub.AutoPayEnabled {
			enabled = true
		}
	}
	return enabled, hasActive
}

// SaveRegistrationState stores FSM progress for user.
func (s *Store) SaveRegistrationState(_ context.Context, state domain.RegistrationState) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	state.UpdatedAt = time.Now().UTC()
	s.states[registrationStateKey(state.MessengerKind, state.MessengerUserID)] = state
	return nil
}

// GetRegistrationState fetches FSM progress for user.
func (s *Store) GetRegistrationState(_ context.Context, kind domain.MessengerKind, messengerUserID string) (domain.RegistrationState, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	state, ok := s.states[registrationStateKey(kind, messengerUserID)]
	return state, ok, nil
}

// DeleteRegistrationState clears FSM state after completion/cancel.
func (s *Store) DeleteRegistrationState(_ context.Context, kind domain.MessengerKind, messengerUserID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.states, registrationStateKey(kind, messengerUserID))
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
		if query.ActorType != "" && event.ActorType != query.ActorType {
			continue
		}
		if query.TargetUserID > 0 && event.TargetUserID != query.TargetUserID {
			continue
		}
		if query.TargetMessengerKind != "" && event.TargetMessengerKind != query.TargetMessengerKind {
			continue
		}
		if query.TargetMessengerUserID != "" && event.TargetMessengerUserID != query.TargetMessengerUserID {
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
		case "target_messenger_user_id":
			switch {
			case left.TargetMessengerUserID < right.TargetMessengerUserID:
				cmp = -1
			case left.TargetMessengerUserID > right.TargetMessengerUserID:
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
		case "actor_type":
			cmp = strings.Compare(string(left.ActorType), string(right.ActorType))
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
	payment.UserID, _ = s.resolveUserIdentityLocked(payment.UserID, 0)
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
	sub.UserID, _ = s.resolveUserIdentityLocked(sub.UserID, 0)
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
func (s *Store) GetLatestSubscriptionByUserConnector(_ context.Context, userID, connectorID int64) (domain.Subscription, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var (
		best  domain.Subscription
		found bool
	)
	for _, sub := range s.subsByPayID {
		if sub.UserID != userID || sub.ConnectorID != connectorID {
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
		if query.UserID > 0 && item.UserID != query.UserID {
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
		if query.UserID > 0 && item.UserID != query.UserID {
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
		if item.StartsAt.After(now) {
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
		if item.StartsAt.After(now) {
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

// DisableAutoPayForActiveSubscriptions clears recurring flag for all active subscriptions of one user.
func (s *Store) DisableAutoPayForActiveSubscriptions(_ context.Context, userID int64, updatedAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}
	for paymentID, sub := range s.subsByPayID {
		if sub.UserID != userID || sub.Status != domain.SubscriptionStatusActive || !sub.AutoPayEnabled {
			continue
		}
		sub.AutoPayEnabled = false
		sub.UpdatedAt = updatedAt
		s.subsByPayID[paymentID] = sub
	}
	return nil
}

// SetSubscriptionAutoPayEnabled updates recurring flag for a single subscription.
func (s *Store) SetSubscriptionAutoPayEnabled(_ context.Context, subscriptionID int64, enabled bool, updatedAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}
	for paymentID, sub := range s.subsByPayID {
		if sub.ID != subscriptionID {
			continue
		}
		sub.AutoPayEnabled = enabled
		sub.UpdatedAt = updatedAt
		s.subsByPayID[paymentID] = sub
		return nil
	}
	return errors.New("subscription not found")
}

// consentKey builds deterministic compound key for consent map.
func consentKey(userID, connectorID int64) string {
	return int64ToString(connectorID) + ":" + int64ToString(userID)
}

// int64ToString converts int64 values for map key composition.
func int64ToString(v int64) string {
	return strconv.FormatInt(v, 10)
}

func registrationStateKey(kind domain.MessengerKind, messengerUserID string) string {
	return string(kind) + ":" + strings.TrimSpace(messengerUserID)
}

func messengerAccountKey(kind domain.MessengerKind, messengerUserID string) string {
	return string(kind) + ":" + strings.TrimSpace(messengerUserID)
}

func (s *Store) resolveUserIdentityLocked(userID, telegramID int64) (int64, int64) {
	if userID > 0 {
		if _, ok := s.users[userID]; ok {
			if telegramID <= 0 {
				telegramID, _ = s.telegramIdentityLocked(userID)
			}
			return userID, telegramID
		}
	}
	if telegramID > 0 {
		if resolvedUserID, ok := s.userIDByTelegram[telegramID]; ok {
			return resolvedUserID, telegramID
		}
	}
	return userID, telegramID
}

func (s *Store) upsertMessengerAccountLocked(account domain.UserMessengerAccount) {
	key := messengerAccountKey(account.MessengerKind, account.MessengerUserID)
	existing, ok := s.messengerAccounts[key]
	if ok {
		account.LinkedAt = existing.LinkedAt
		if account.UpdatedAt.IsZero() {
			account.UpdatedAt = time.Now().UTC()
		}
	} else if account.LinkedAt.IsZero() {
		account.LinkedAt = time.Now().UTC()
	}
	if account.UpdatedAt.IsZero() {
		account.UpdatedAt = account.LinkedAt
	}
	s.messengerAccounts[key] = account
	if account.MessengerKind == domain.MessengerKindTelegram {
		if telegramID, err := strconv.ParseInt(account.MessengerUserID, 10, 64); err == nil && telegramID > 0 {
			s.userIDByTelegram[telegramID] = account.UserID
		}
	}
}

func (s *Store) telegramIdentityLocked(userID int64) (int64, string) {
	for _, account := range s.messengerAccounts {
		if account.UserID != userID || account.MessengerKind != domain.MessengerKindTelegram {
			continue
		}
		telegramID, err := strconv.ParseInt(account.MessengerUserID, 10, 64)
		if err != nil || telegramID <= 0 {
			return 0, account.Username
		}
		return telegramID, account.Username
	}
	return 0, ""
}
