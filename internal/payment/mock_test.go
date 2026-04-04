package payment

import (
	"context"
	"net/url"
	"strings"
	"testing"
)

func TestNewMockService_DefaultsAndCheckoutURL(t *testing.T) {
	svc := NewMockService("")
	if svc.ProviderName() != "mock" {
		t.Fatalf("provider=%q want mock", svc.ProviderName())
	}
	if !svc.IsTestMode() {
		t.Fatalf("IsTestMode=false want true")
	}

	rawURL, err := svc.CreateCheckoutURL(context.Background(), Request{
		ConnectorID: 17,
		AmountRUB:   2322,
	})
	if err != nil {
		t.Fatalf("CreateCheckoutURL: %v", err)
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if parsed.Scheme != "http" || parsed.Host != "localhost:8080" {
		t.Fatalf("url=%q want localhost mock base", rawURL)
	}
	if parsed.Path != "/mock/pay" {
		t.Fatalf("path=%q want /mock/pay", parsed.Path)
	}
	if parsed.Query().Get("connector_id") != "17" {
		t.Fatalf("connector_id=%q want 17", parsed.Query().Get("connector_id"))
	}
	if parsed.Query().Get("amount_rub") != "2322" {
		t.Fatalf("amount_rub=%q want 2322", parsed.Query().Get("amount_rub"))
	}
	if token := parsed.Query().Get("token"); len(token) != 16 {
		t.Fatalf("token=%q want 16 hex chars", token)
	}
}

func TestMockService_UsesProvidedInvoiceID(t *testing.T) {
	svc := NewMockService("https://example.com/base/")
	rawURL, err := svc.CreateCheckoutURL(context.Background(), Request{
		ConnectorID: 5,
		AmountRUB:   99,
		InvoiceID:   "inv-42",
	})
	if err != nil {
		t.Fatalf("CreateCheckoutURL: %v", err)
	}
	if !strings.HasPrefix(rawURL, "https://example.com/base/mock/pay?") {
		t.Fatalf("url=%q want trimmed custom base", rawURL)
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if parsed.Query().Get("token") != "inv-42" {
		t.Fatalf("token=%q want inv-42", parsed.Query().Get("token"))
	}
}

func TestRandomToken_UsesDefaultSize(t *testing.T) {
	token, err := randomToken(0)
	if err != nil {
		t.Fatalf("randomToken: %v", err)
	}
	if len(token) != 16 {
		t.Fatalf("len(token)=%d want 16", len(token))
	}
}
