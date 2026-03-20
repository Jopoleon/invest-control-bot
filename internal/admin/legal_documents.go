package admin

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Jopoleon/telega-bot-fedor/internal/domain"
)

var (
	errLegalDocumentType    = errors.New("legal_document_type_required")
	errLegalDocumentTitle   = errors.New("legal_document_title_required")
	errLegalDocumentContent = errors.New("legal_document_content_required")
)

// legalDocumentsPage renders and mutates legal document versions.
func (h *Handler) legalDocumentsPage(w http.ResponseWriter, r *http.Request) {
	if !h.requireAuth(w, r) {
		return
	}
	lang := h.resolveLang(w, r)

	switch r.Method {
	case http.MethodGet:
		h.renderLegalDocumentsPage(r.Context(), w, r, lang, "")
	case http.MethodPost:
		if err := r.ParseForm(); err != nil {
			h.renderLegalDocumentsPage(r.Context(), w, r, lang, t(lang, "legal.bad_form"))
			return
		}
		if !h.verifyCSRF(r) {
			w.WriteHeader(http.StatusForbidden)
			h.renderLegalDocumentsPage(r.Context(), w, r, lang, t(lang, "csrf.invalid"))
			return
		}
		if err := h.createLegalDocument(r.Context(), r); err != nil {
			h.renderLegalDocumentsPage(r.Context(), w, r, lang, h.localizeLegalDocumentError(lang, err))
			return
		}
		h.renderLegalDocumentsPage(r.Context(), w, r, lang, t(lang, "legal.created"))
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// activateLegalDocument marks selected version active for its type.
func (h *Handler) activateLegalDocument(w http.ResponseWriter, r *http.Request) {
	if !h.requireAuth(w, r) {
		return
	}
	lang := h.resolveLang(w, r)
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		h.renderLegalDocumentsPage(r.Context(), w, r, lang, t(lang, "legal.bad_form"))
		return
	}
	if !h.verifyCSRF(r) {
		w.WriteHeader(http.StatusForbidden)
		h.renderLegalDocumentsPage(r.Context(), w, r, lang, t(lang, "csrf.invalid"))
		return
	}
	id, err := parseDocumentID(r.FormValue("id"))
	if err != nil {
		h.renderLegalDocumentsPage(r.Context(), w, r, lang, t(lang, "legal.invalid_id"))
		return
	}
	if err := h.store.SetLegalDocumentActive(r.Context(), id); err != nil {
		h.renderLegalDocumentsPage(r.Context(), w, r, lang, err.Error())
		return
	}
	h.renderLegalDocumentsPage(r.Context(), w, r, lang, t(lang, "legal.activated"))
}

func (h *Handler) createLegalDocument(ctx context.Context, r *http.Request) error {
	docType := domain.LegalDocumentType(strings.TrimSpace(r.FormValue("doc_type")))
	title := strings.TrimSpace(r.FormValue("title"))
	content := strings.TrimSpace(r.FormValue("content"))
	externalURL := strings.TrimSpace(r.FormValue("external_url"))
	isActive := r.FormValue("is_active") == "true"

	switch docType {
	case domain.LegalDocumentTypeOffer, domain.LegalDocumentTypePrivacy:
	default:
		return errLegalDocumentType
	}
	if title == "" {
		return errLegalDocumentTitle
	}
	if content == "" && externalURL == "" {
		return errLegalDocumentContent
	}

	return h.store.CreateLegalDocument(ctx, domain.LegalDocument{
		Type:        docType,
		Title:       title,
		Content:     content,
		ExternalURL: externalURL,
		IsActive:    isActive,
		CreatedAt:   time.Now().UTC(),
	})
}

func (h *Handler) renderLegalDocumentsPage(ctx context.Context, w http.ResponseWriter, r *http.Request, lang, notice string) {
	docs, err := h.store.ListLegalDocuments(ctx, "")
	if err != nil {
		notice = t(lang, "legal.load_error")
	}

	rows := make([]legalDocumentView, 0, len(docs))
	for _, doc := range docs {
		activeLabel, activeClass := connectorActiveBadge(lang, doc.IsActive)
		rows = append(rows, legalDocumentView{
			ID:             doc.ID,
			Type:           string(doc.Type),
			TypeLabel:      h.legalDocumentTypeLabel(lang, doc.Type),
			Title:          doc.Title,
			ContentPreview: legalDocumentPreview(doc.Content),
			ExternalURL:    doc.ExternalURL,
			Version:        doc.Version,
			IsActive:       doc.IsActive,
			ActiveLabel:    activeLabel,
			ActiveClass:    activeClass,
			CreatedAt:      doc.CreatedAt.Local().Format("02.01.2006 15:04"),
			PublicURL:      h.legalDocumentPublicURL(r, doc.Type),
			CanActivate:    !doc.IsActive,
		})
	}

	h.renderer.render(w, "legal_documents.html", legalDocumentsPageData{
		basePageData: basePageData{
			Lang:       lang,
			I18N:       dictForLang(lang),
			CSRFToken:  h.ensureCSRFToken(w, r),
			TopbarPath: "/admin/legal-documents",
			ActiveNav:  "legal",
		},
		Notice:           notice,
		ExportURL:        buildExportURL("/admin/legal-documents/export.csv", r.URL.Query(), lang),
		OfferPublicURL:   h.legalDocumentPublicURL(r, domain.LegalDocumentTypeOffer),
		PrivacyPublicURL: h.legalDocumentPublicURL(r, domain.LegalDocumentTypePrivacy),
		Documents:        rows,
	})
}

func (h *Handler) legalDocumentPublicURL(r *http.Request, docType domain.LegalDocumentType) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")); forwarded != "" {
		scheme = forwarded
	}
	host := strings.TrimSpace(r.Host)
	if host == "" {
		return ""
	}
	return fmt.Sprintf("%s://%s/legal/%s", scheme, host, docType)
}

func (h *Handler) legalDocumentTypeLabel(lang string, docType domain.LegalDocumentType) string {
	switch docType {
	case domain.LegalDocumentTypeOffer:
		return t(lang, "legal.type.offer")
	case domain.LegalDocumentTypePrivacy:
		return t(lang, "legal.type.privacy")
	default:
		return string(docType)
	}
}

func (h *Handler) localizeLegalDocumentError(lang string, err error) string {
	switch {
	case errors.Is(err, errLegalDocumentType):
		return t(lang, "legal.validation.type")
	case errors.Is(err, errLegalDocumentTitle):
		return t(lang, "legal.validation.title")
	case errors.Is(err, errLegalDocumentContent):
		return t(lang, "legal.validation.content")
	default:
		return err.Error()
	}
}

func parseDocumentID(raw string) (int64, error) {
	id, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || id <= 0 {
		return 0, errors.New("invalid document id")
	}
	return id, nil
}

func legalDocumentPreview(content string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}
	content = strings.ReplaceAll(content, "\n", " ")
	if len(content) <= 140 {
		return content
	}
	return content[:140] + "..."
}
