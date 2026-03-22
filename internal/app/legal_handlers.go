package app

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
)

type legalDocumentPageData struct {
	Title       string
	Subtitle    string
	Content     string
	ExternalURL string
}

func (a *application) handleLegalOffer(w http.ResponseWriter, r *http.Request) {
	a.handleActiveLegalDocument(w, r, domain.LegalDocumentTypeOffer, "Публичная оферта")
}

func (a *application) handleLegalPrivacy(w http.ResponseWriter, r *http.Request) {
	a.handleActiveLegalDocument(w, r, domain.LegalDocumentTypePrivacy, "Политика обработки персональных данных")
}

func (a *application) handleLegalAgreement(w http.ResponseWriter, r *http.Request) {
	a.handleActiveLegalDocument(w, r, domain.LegalDocumentTypeUserAgreement, "Пользовательское соглашение")
}

func (a *application) handleOfferByID(w http.ResponseWriter, r *http.Request) {
	a.handleLegalDocumentByID(w, r, "/oferta/", domain.LegalDocumentTypeOffer, "Публичная оферта")
}

func (a *application) handlePrivacyByID(w http.ResponseWriter, r *http.Request) {
	a.handleLegalDocumentByID(w, r, "/policy/", domain.LegalDocumentTypePrivacy, "Политика обработки персональных данных")
}

func (a *application) handleAgreementByID(w http.ResponseWriter, r *http.Request) {
	a.handleLegalDocumentByID(w, r, "/agreement/", domain.LegalDocumentTypeUserAgreement, "Пользовательское соглашение")
}

func (a *application) handleActiveLegalDocument(w http.ResponseWriter, r *http.Request, docType domain.LegalDocumentType, fallbackTitle string) {
	doc, found, err := a.store.GetActiveLegalDocument(r.Context(), docType)
	if err != nil {
		http.Error(w, "failed to load document", http.StatusInternalServerError)
		return
	}
	if !found {
		http.NotFound(w, r)
		return
	}
	a.renderLegalDocument(w, doc, fallbackTitle)
}

func (a *application) handleLegalDocumentByID(w http.ResponseWriter, r *http.Request, prefix string, expectedType domain.LegalDocumentType, fallbackTitle string) {
	id, err := parsePublicDocumentID(strings.TrimPrefix(r.URL.Path, prefix))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	doc, found, err := a.store.GetLegalDocument(r.Context(), id)
	if err != nil {
		http.Error(w, "failed to load document", http.StatusInternalServerError)
		return
	}
	if !found || doc.Type != expectedType {
		http.NotFound(w, r)
		return
	}
	a.renderLegalDocument(w, doc, fallbackTitle)
}

func (a *application) renderLegalDocument(w http.ResponseWriter, doc domain.LegalDocument, fallbackTitle string) {
	renderAppTemplate(w, "legal_document.html", legalDocumentPageData{
		Title:       doc.Title,
		Subtitle:    fallbackTitle,
		Content:     doc.Content,
		ExternalURL: doc.ExternalURL,
	})
}

func parsePublicDocumentID(raw string) (int64, error) {
	raw = strings.Trim(strings.TrimSpace(raw), "/")
	return strconv.ParseInt(raw, 10, 64)
}
