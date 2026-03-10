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
)

// Store is a thread-safe in-memory implementation used for local development.
type Store struct {
	mu sync.RWMutex

	connectors   map[string]domain.Connector
	payloadIndex map[string]string
	users        map[int64]domain.User
	consents     map[string]domain.Consent
	states       map[int64]domain.RegistrationState
}

// New creates empty in-memory store.
func New() *Store {
	return &Store{
		connectors:   make(map[string]domain.Connector),
		payloadIndex: make(map[string]string),
		users:        make(map[int64]domain.User),
		consents:     make(map[string]domain.Consent),
		states:       make(map[int64]domain.RegistrationState),
	}
}

// CreateConnector inserts new connector and maintains start_payload index.
func (s *Store) CreateConnector(_ context.Context, c domain.Connector) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if c.ID == "" {
		return errors.New("connector ID is required")
	}
	if _, exists := s.connectors[c.ID]; exists {
		return errors.New("connector already exists")
	}
	if c.StartPayload == "" {
		c.StartPayload = c.ID
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
func (s *Store) GetConnector(_ context.Context, connectorID string) (domain.Connector, bool, error) {
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
func (s *Store) SetConnectorActive(_ context.Context, connectorID string, active bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	c, ok := s.connectors[connectorID]
	if !ok {
		return errors.New("connector not found")
	}
	c.IsActive = active
	s.connectors[connectorID] = c
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
func (s *Store) GetConsent(_ context.Context, telegramID int64, connectorID string) (domain.Consent, bool, error) {
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

// consentKey builds deterministic compound key for consent map.
func consentKey(telegramID int64, connectorID string) string {
	return connectorID + ":" + int64ToString(telegramID)
}

// int64ToString converts int64 values for map key composition.
func int64ToString(v int64) string {
	return strconv.FormatInt(v, 10)
}
