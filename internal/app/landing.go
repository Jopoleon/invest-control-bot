package app

import (
	"net/http"
	"strings"
)

type landingPageData struct {
	Title        string
	Subtitle     string
	AdminURL     string
	HealthURL    string
	OfferURL     string
	PrivacyURL   string
	AgreementURL string
}

func (a *application) handleLanding(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	title := strings.TrimSpace(a.config.AppName)
	if title == "" {
		title = "invest-control-bot"
	}

	renderAppTemplate(w, "landing.html", landingPageData{
		Title:        title,
		Subtitle:     "Сервис управления платным доступом и подписками.",
		AdminURL:     "/admin",
		HealthURL:    "/healthz",
		OfferURL:     "/legal/offer",
		PrivacyURL:   "/legal/privacy",
		AgreementURL: "/legal/agreement",
	})
}
