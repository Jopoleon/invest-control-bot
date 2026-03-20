package bot

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/Jopoleon/telega-bot-fedor/internal/domain"
	"github.com/go-telegram/bot/models"
)

const payConsentCallbackPrefix = "payconsent:"

func (h *Handler) buildFinalPaymentStep(ctx context.Context, connectorID int64, recurringOptIn bool) (string, *models.InlineKeyboardMarkup) {
	baseText := "✅ Спасибо! Ваша заявка оформлена успешно.\n💳 Осталось оплатить\nЧтобы произвести оплату, нажмите на кнопку «Оплатить» ниже, для переадресации на платежную страницу"

	if !h.recurringEnabled {
		return baseText, paymentKeyboard(connectorID, false, false)
	}

	connector, ok, err := h.store.GetConnector(ctx, connectorID)
	if err != nil || !ok || !connector.IsActive {
		return baseText, paymentKeyboard(connectorID, false, false)
	}

	offerURL, offerDoc, offerDocFound := h.resolveRecurringOfferLink(ctx, connector)
	agreementURL, _, agreementDocFound := h.resolveLegalDocumentURL(ctx, domain.LegalDocumentTypeUserAgreement)
	if strings.TrimSpace(offerURL) == "" || strings.TrimSpace(agreementURL) == "" || !agreementDocFound {
		return baseText, paymentKeyboard(connectorID, false, false)
	}

	if recurringOptIn {
		baseText += "\n\n☑️ Автоплатеж будет включен для следующих списаний.\nСогласие действует по оферте (" + offerURL + ") и пользовательскому соглашению (" + agreementURL + ")."
		return baseText, paymentKeyboard(connectorID, true, true)
	}

	_ = offerDoc
	_ = offerDocFound
	baseText += "\n\n☐ Автоплатеж выключен.\nЕсли хотите подключить автоматические списания, подтвердите согласие кнопкой ниже."
	return baseText, paymentKeyboard(connectorID, false, true)
}

func paymentKeyboard(connectorID int64, recurringOptIn, canOfferRecurring bool) *models.InlineKeyboardMarkup {
	rows := make([][]models.InlineKeyboardButton, 0, 2)
	if canOfferRecurring {
		toggleText := "☐ Я согласен на автоматические списания"
		toggleState := "on"
		if recurringOptIn {
			toggleText = "☑ Я согласен на автоматические списания"
			toggleState = "off"
		}
		rows = append(rows, []models.InlineKeyboardButton{
			{Text: toggleText, CallbackData: payConsentCallbackPrefix + strconv.FormatInt(connectorID, 10) + ":" + toggleState},
		})
	}

	payText := "Оплатить"
	payMode := "0"
	if recurringOptIn && canOfferRecurring {
		payText = "Оплатить и включить автоплатеж"
		payMode = "1"
	}
	rows = append(rows, []models.InlineKeyboardButton{{
		Text:         payText,
		CallbackData: "pay:" + strconv.FormatInt(connectorID, 10) + ":" + payMode,
	}})

	return &models.InlineKeyboardMarkup{InlineKeyboard: rows}
}

func (h *Handler) resolveLegalDocumentURL(ctx context.Context, docType domain.LegalDocumentType) (string, domain.LegalDocument, bool) {
	doc, found := h.resolveLegalDocument(ctx, docType)
	if !found {
		return "", domain.LegalDocument{}, false
	}
	if strings.TrimSpace(doc.ExternalURL) != "" {
		return doc.ExternalURL, doc, true
	}
	if h.publicBaseURL == "" || doc.ID <= 0 {
		return "", doc, false
	}
	switch docType {
	case domain.LegalDocumentTypeOffer:
		return h.publicBaseURL + "/oferta/" + strconv.FormatInt(doc.ID, 10), doc, true
	case domain.LegalDocumentTypePrivacy:
		return h.publicBaseURL + "/policy/" + strconv.FormatInt(doc.ID, 10), doc, true
	case domain.LegalDocumentTypeUserAgreement:
		return h.publicBaseURL + "/agreement/" + strconv.FormatInt(doc.ID, 10), doc, true
	default:
		return "", doc, false
	}
}

func (h *Handler) resolveRecurringOfferLink(ctx context.Context, connector domain.Connector) (string, domain.LegalDocument, bool) {
	if explicit := strings.TrimSpace(connector.OfferURL); explicit != "" {
		return explicit, domain.LegalDocument{}, false
	}
	return h.resolveLegalDocumentURL(ctx, domain.LegalDocumentTypeOffer)
}

func (h *Handler) buildRecurringConsent(ctx context.Context, telegramID int64, connector domain.Connector) (domain.RecurringConsent, error) {
	_, offerDoc, offerDocFound := h.resolveRecurringOfferLink(ctx, connector)
	_, agreementDoc, agreementFound := h.resolveLegalDocumentURL(ctx, domain.LegalDocumentTypeUserAgreement)
	if !agreementFound {
		return domain.RecurringConsent{}, errors.New("active user agreement is required for recurring consent")
	}

	consent := domain.RecurringConsent{
		TelegramID:  telegramID,
		ConnectorID: connector.ID,
		AcceptedAt:  time.Now().UTC(),
	}
	if offerDocFound {
		consent.OfferDocumentID = offerDoc.ID
		consent.OfferDocumentVersion = offerDoc.Version
	}
	consent.UserAgreementDocumentID = agreementDoc.ID
	consent.UserAgreementDocumentVersion = agreementDoc.Version
	return consent, nil
}

func parsePayCallbackData(raw string) (connectorID int64, explicitRecurring bool, hasExplicitRecurring bool, ok bool) {
	parts := strings.Split(strings.TrimSpace(raw), ":")
	if len(parts) < 2 || parts[0] != "pay" {
		return 0, false, false, false
	}
	connectorID, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || connectorID <= 0 {
		return 0, false, false, false
	}
	if len(parts) >= 3 {
		switch parts[2] {
		case "1":
			return connectorID, true, true, true
		case "0":
			return connectorID, false, true, true
		}
	}
	return connectorID, false, false, true
}

func parsePayConsentCallbackData(raw string) (connectorID int64, enable bool, ok bool) {
	parts := strings.Split(strings.TrimSpace(strings.TrimPrefix(raw, payConsentCallbackPrefix)), ":")
	if len(parts) != 2 {
		return 0, false, false
	}
	connectorID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || connectorID <= 0 {
		return 0, false, false
	}
	switch parts[1] {
	case "on":
		return connectorID, true, true
	case "off":
		return connectorID, false, true
	default:
		return 0, false, false
	}
}
