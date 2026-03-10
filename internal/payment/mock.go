package payment

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"
)

// MockService generates local checkout links without external payment gateway.
type MockService struct {
	baseURL string
}

// NewMockService constructs mock provider.
func NewMockService(baseURL string) *MockService {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}
	return &MockService{baseURL: baseURL}
}

// ProviderName returns provider identifier used in user messages/logs.
func (m *MockService) ProviderName() string { return "mock" }

// CreateCheckoutURL builds test checkout URL with request payload in query params.
func (m *MockService) CreateCheckoutURL(_ context.Context, req Request) (string, error) {
	token, err := randomToken(8)
	if err != nil {
		return "", err
	}
	q := url.Values{}
	q.Set("token", token)
	q.Set("connector_id", req.ConnectorID)
	q.Set("user_id", fmt.Sprintf("%d", req.UserTelegramID))
	q.Set("amount_rub", fmt.Sprintf("%d", req.AmountRUB))
	return m.baseURL + "/mock/pay?" + q.Encode(), nil
}

// randomToken generates pseudo payment token for mock checkout URL.
func randomToken(size int) (string, error) {
	if size <= 0 {
		size = 8
	}
	b := make([]byte, size)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
