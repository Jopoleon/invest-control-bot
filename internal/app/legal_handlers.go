package app

import (
	"net/http"

	"github.com/Jopoleon/telega-bot-fedor/internal/domain"
)

type legalDocumentPageData struct {
	Title       string
	Subtitle    string
	Content     string
	ExternalURL string
}

func (a *application) handleLegalOffer(w http.ResponseWriter, r *http.Request) {
	a.handleLegalDocument(w, r, domain.LegalDocumentTypeOffer, "Публичная оферта")
}

func (a *application) handleLegalPrivacy(w http.ResponseWriter, r *http.Request) {
	a.handleLegalDocument(w, r, domain.LegalDocumentTypePrivacy, "Политика обработки персональных данных")
}

func (a *application) handleLegalDocument(w http.ResponseWriter, r *http.Request, docType domain.LegalDocumentType, fallbackTitle string) {
	doc, found, err := a.store.GetActiveLegalDocument(r.Context(), docType)
	if err != nil {
		http.Error(w, "failed to load document", http.StatusInternalServerError)
		return
	}
	if !found {
		http.NotFound(w, r)
		return
	}

	renderAppTemplate(w, "legal_document.html", legalDocumentPageData{
		Title:       doc.Title,
		Subtitle:    fallbackTitle,
		Content:     doc.Content,
		ExternalURL: doc.ExternalURL,
	})
}
