package admin

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
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
		h.renderLegalDocumentsPage(r.Context(), w, r, lang, strings.TrimSpace(r.URL.Query().Get("notice")))
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
		if idRaw := strings.TrimSpace(r.FormValue("id")); idRaw != "" {
			if err := h.updateLegalDocument(r.Context(), r); err != nil {
				h.renderLegalDocumentsPage(r.Context(), w, r, lang, h.localizeLegalDocumentError(lang, err))
				return
			}
			h.redirectLegalDocuments(w, r, lang, t(lang, "legal.updated"))
			return
		}
		if err := h.createLegalDocument(r.Context(), r); err != nil {
			h.renderLegalDocumentsPage(r.Context(), w, r, lang, h.localizeLegalDocumentError(lang, err))
			return
		}
		h.redirectLegalDocuments(w, r, lang, t(lang, "legal.created"))
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// toggleLegalDocument switches published state without affecting other versions.
func (h *Handler) toggleLegalDocument(w http.ResponseWriter, r *http.Request) {
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
	active := r.FormValue("active") == "true"
	if err := h.store.SetLegalDocumentActive(r.Context(), id, active); err != nil {
		h.renderLegalDocumentsPage(r.Context(), w, r, lang, err.Error())
		return
	}
	notice := t(lang, "legal.enabled")
	if !active {
		notice = t(lang, "legal.disabled")
	}
	h.redirectLegalDocuments(w, r, lang, notice)
}

// deleteLegalDocument removes document version permanently.
func (h *Handler) deleteLegalDocument(w http.ResponseWriter, r *http.Request) {
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
	if err := h.store.DeleteLegalDocument(r.Context(), id); err != nil {
		h.renderLegalDocumentsPage(r.Context(), w, r, lang, err.Error())
		return
	}
	h.redirectLegalDocuments(w, r, lang, t(lang, "legal.deleted"))
}

func (h *Handler) createLegalDocument(ctx context.Context, r *http.Request) error {
	doc, err := h.parseLegalDocumentForm(r)
	if err != nil {
		return err
	}
	doc.CreatedAt = time.Now().UTC()
	return h.store.CreateLegalDocument(ctx, doc)
}

func (h *Handler) updateLegalDocument(ctx context.Context, r *http.Request) error {
	doc, err := h.parseLegalDocumentForm(r)
	if err != nil {
		return err
	}
	docID, err := parseDocumentID(r.FormValue("id"))
	if err != nil {
		return err
	}
	current, found, err := h.store.GetLegalDocument(ctx, docID)
	if err != nil {
		return err
	}
	if !found {
		return errors.New("legal document not found")
	}
	doc.ID = docID
	doc.Type = current.Type
	doc.Version = current.Version
	doc.CreatedAt = current.CreatedAt
	return h.store.UpdateLegalDocument(ctx, doc)
}

func (h *Handler) parseLegalDocumentForm(r *http.Request) (domain.LegalDocument, error) {
	docType := domain.LegalDocumentType(strings.TrimSpace(r.FormValue("doc_type")))
	title := strings.TrimSpace(r.FormValue("title"))
	content := strings.TrimSpace(r.FormValue("content"))
	externalURL := strings.TrimSpace(r.FormValue("external_url"))
	isActive := r.FormValue("is_active") == "true"

	switch docType {
	case domain.LegalDocumentTypeOffer, domain.LegalDocumentTypePrivacy, domain.LegalDocumentTypeUserAgreement:
	default:
		return domain.LegalDocument{}, errLegalDocumentType
	}
	if title == "" {
		return domain.LegalDocument{}, errLegalDocumentTitle
	}
	if content == "" && externalURL == "" {
		return domain.LegalDocument{}, errLegalDocumentContent
	}

	return domain.LegalDocument{
		Type:        docType,
		Title:       title,
		Content:     content,
		ExternalURL: externalURL,
		IsActive:    isActive,
	}, nil
}

func (h *Handler) renderLegalDocumentsPage(ctx context.Context, w http.ResponseWriter, r *http.Request, lang, notice string) {
	docs, err := h.store.ListLegalDocuments(ctx, "")
	if err != nil {
		notice = t(lang, "legal.load_error")
	}

	editID, _ := parseOptionalDocumentID(r.URL.Query().Get("edit_id"))
	editDoc, editing := h.loadLegalDocumentForEdit(ctx, editID)

	activeOffer, _, _ := h.store.GetActiveLegalDocument(ctx, domain.LegalDocumentTypeOffer)
	activePrivacy, _, _ := h.store.GetActiveLegalDocument(ctx, domain.LegalDocumentTypePrivacy)
	activeAgreement, _, _ := h.store.GetActiveLegalDocument(ctx, domain.LegalDocumentTypeUserAgreement)

	rows := make([]legalDocumentView, 0, len(docs))
	for _, doc := range docs {
		activeLabel, activeClass := connectorActiveBadge(lang, doc.IsActive)
		toggleLabel := t(lang, "legal.table.enable")
		toggleTo := true
		if doc.IsActive {
			toggleLabel = t(lang, "legal.table.disable")
			toggleTo = false
		}
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
			PublicURL:      h.legalDocumentPublicURL(r, doc.Type, doc.ID),
			ToggleTo:       toggleTo,
			ToggleLabel:    toggleLabel,
			DeleteURL:      "/admin/legal-documents/delete?lang=" + lang,
		})
	}

	formType := string(domain.LegalDocumentTypeOffer)
	formSubmit := t(lang, "legal.create_button")
	if editing {
		formType = string(editDoc.Type)
		formSubmit = t(lang, "legal.update_button")
	}

	h.renderer.render(w, "legal_documents.html", legalDocumentsPageData{
		basePageData: basePageData{
			Lang:       lang,
			I18N:       dictForLang(lang),
			CSRFToken:  h.ensureCSRFToken(w, r),
			TopbarPath: "/admin/legal-documents",
			ActiveNav:  "legal",
		},
		Notice:             notice,
		ExportURL:          buildExportURL("/admin/legal-documents/export.csv", r.URL.Query(), lang),
		OfferPublicURL:     h.activeLegalDocumentURL(r, activeOffer),
		PrivacyPublicURL:   h.activeLegalDocumentURL(r, activePrivacy),
		AgreementPublicURL: h.activeLegalDocumentURL(r, activeAgreement),
		Documents:          rows,
		EditingID:          editDoc.ID,
		Editing:            editing,
		FormAction:         "/admin/legal-documents",
		FormSubmitLabel:    formSubmit,
		FormType:           formType,
		FormTitle:          editDoc.Title,
		FormExternalURL:    editDoc.ExternalURL,
		FormContent:        editDoc.Content,
		FormIsActive:       !editing || editDoc.IsActive,
	})
}

func (h *Handler) redirectLegalDocuments(w http.ResponseWriter, r *http.Request, lang, notice string) {
	query := url.Values{}
	query.Set("lang", lang)
	if notice != "" {
		query.Set("notice", notice)
	}
	http.Redirect(w, r, "/admin/legal-documents?"+query.Encode(), http.StatusSeeOther)
}

func (h *Handler) activeLegalDocumentURL(r *http.Request, doc domain.LegalDocument) string {
	if doc.ID <= 0 {
		return ""
	}
	return h.legalDocumentPublicURL(r, doc.Type, doc.ID)
}

func (h *Handler) legalDocumentPublicURL(r *http.Request, docType domain.LegalDocumentType, id int64) string {
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
	switch docType {
	case domain.LegalDocumentTypeOffer:
		return fmt.Sprintf("%s://%s/oferta/%d", scheme, host, id)
	case domain.LegalDocumentTypePrivacy:
		return fmt.Sprintf("%s://%s/policy/%d", scheme, host, id)
	case domain.LegalDocumentTypeUserAgreement:
		return fmt.Sprintf("%s://%s/agreement/%d", scheme, host, id)
	default:
		return ""
	}
}

func (h *Handler) legalDocumentTypeLabel(lang string, docType domain.LegalDocumentType) string {
	switch docType {
	case domain.LegalDocumentTypeOffer:
		return t(lang, "legal.type.offer")
	case domain.LegalDocumentTypePrivacy:
		return t(lang, "legal.type.privacy")
	case domain.LegalDocumentTypeUserAgreement:
		return t(lang, "legal.type.user_agreement")
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

func (h *Handler) loadLegalDocumentForEdit(ctx context.Context, documentID int64) (domain.LegalDocument, bool) {
	if documentID <= 0 {
		return domain.LegalDocument{}, false
	}
	doc, found, err := h.store.GetLegalDocument(ctx, documentID)
	if err != nil || !found {
		return domain.LegalDocument{}, false
	}
	return doc, true
}

func parseDocumentID(raw string) (int64, error) {
	id, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || id <= 0 {
		return 0, errors.New("invalid document id")
	}
	return id, nil
}

func parseOptionalDocumentID(raw string) (int64, bool) {
	id, err := parseDocumentID(raw)
	if err != nil {
		return 0, false
	}
	return id, true
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
