package admin

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
	"github.com/Jopoleon/invest-control-bot/internal/store/memory"
)

func TestParseLegalDocumentForm(t *testing.T) {
	h := NewHandler(memory.New(), "token", "test_bot", "max_test_bot", "http://localhost:8080", "test-encryption-key-123456789012345", nil, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/admin/legal-documents", strings.NewReader("doc_type=offer&title=Offer&content=Body&is_active=true"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if err := req.ParseForm(); err != nil {
		t.Fatalf("ParseForm: %v", err)
	}

	doc, err := h.parseLegalDocumentForm(req)
	if err != nil {
		t.Fatalf("parseLegalDocumentForm: %v", err)
	}
	if doc.Type != domain.LegalDocumentTypeOffer || !doc.IsActive || doc.Title != "Offer" {
		t.Fatalf("unexpected doc: %+v", doc)
	}
}

func TestParseLegalDocumentForm_RequiresTypeAndContent(t *testing.T) {
	h := NewHandler(memory.New(), "token", "test_bot", "max_test_bot", "http://localhost:8080", "test-encryption-key-123456789012345", nil, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/admin/legal-documents", strings.NewReader("doc_type=unknown&title=Offer"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if err := req.ParseForm(); err != nil {
		t.Fatalf("ParseForm: %v", err)
	}

	if _, err := h.parseLegalDocumentForm(req); err != errLegalDocumentType {
		t.Fatalf("err = %v, want %v", err, errLegalDocumentType)
	}

	req = httptest.NewRequest(http.MethodPost, "/admin/legal-documents", strings.NewReader("doc_type=offer&title=Offer"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if err := req.ParseForm(); err != nil {
		t.Fatalf("ParseForm: %v", err)
	}
	if _, err := h.parseLegalDocumentForm(req); err != errLegalDocumentContent {
		t.Fatalf("err = %v, want %v", err, errLegalDocumentContent)
	}
}

func TestLegalDocumentPublicURL_UsesForwardedProto(t *testing.T) {
	h := NewHandler(memory.New(), "token", "test_bot", "max_test_bot", "http://localhost:8080", "test-encryption-key-123456789012345", nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "http://example.com/admin/legal-documents", nil)
	req.Host = "pay.example.com"
	req.Header.Set("X-Forwarded-Proto", "https")

	got := h.legalDocumentPublicURL(req, domain.LegalDocumentTypePrivacy, 7)
	if got != "https://pay.example.com/policy/7" {
		t.Fatalf("got = %q", got)
	}
}

func TestLegalDocumentPreview_TruncatesAndNormalizesWhitespace(t *testing.T) {
	content := strings.Repeat("a", 141) + "\nnext"
	got := legalDocumentPreview(content)
	if !strings.HasSuffix(got, "...") {
		t.Fatalf("preview = %q, want ellipsis", got)
	}
	if strings.Contains(got, "\n") {
		t.Fatalf("preview still contains newline: %q", got)
	}
}
