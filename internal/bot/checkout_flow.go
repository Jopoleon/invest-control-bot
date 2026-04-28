package bot

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
	"github.com/Jopoleon/invest-control-bot/internal/messenger"
)

const payConsentCallbackPrefix = "payconsent:"

// buildFinalPaymentStep returns the last onboarding message before checkout.
// It decides whether recurring opt-in can be offered for the selected connector
// based on current product settings and available legal documents.
func (h *Handler) buildFinalPaymentStep(ctx context.Context, connectorID int64, currentKind messenger.Kind, recurringOptIn bool) (string, [][]messenger.ActionButton) {
	baseText := botMsgCheckoutBase

	if !h.recurringEnabled {
		return baseText, paymentKeyboard(connectorID, false, false)
	}

	connector, ok, err := h.store.GetConnector(ctx, connectorID)
	if err != nil || !ok || !connector.IsActive {
		return baseText, paymentKeyboard(connectorID, false, false)
	}
	if warning := botConnectorAccessMismatchWarning(connector, currentKind); warning != "" {
		return baseText + "\n\n" + warning, nil
	}
	if !connector.SupportsRecurring() {
		return baseText, paymentKeyboard(connectorID, false, false)
	}

	offerURL, offerDoc, offerDocFound := h.resolveRecurringOfferLink(ctx, connector)
	agreementURL, _, agreementDocFound := h.resolveLegalDocumentURL(ctx, domain.LegalDocumentTypeUserAgreement)
	if strings.TrimSpace(offerURL) == "" || strings.TrimSpace(agreementURL) == "" || !agreementDocFound {
		return baseText, paymentKeyboard(connectorID, false, false)
	}

	if recurringOptIn {
		baseText += botCheckoutAutopayEnabled(offerURL)
		return baseText, paymentKeyboard(connectorID, true, true)
	}

	_ = offerDoc
	_ = offerDocFound
	baseText += botMsgCheckoutAutopayDisabled
	return baseText, paymentKeyboard(connectorID, false, true)
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

func (h *Handler) buildRecurringConsent(ctx context.Context, userID int64, connector domain.Connector) (domain.RecurringConsent, error) {
	if !connector.SupportsRecurring() {
		return domain.RecurringConsent{}, errors.New("connector does not support recurring")
	}
	_, offerDoc, offerDocFound := h.resolveRecurringOfferLink(ctx, connector)
	_, agreementDoc, agreementFound := h.resolveLegalDocumentURL(ctx, domain.LegalDocumentTypeUserAgreement)
	if !agreementFound {
		return domain.RecurringConsent{}, errors.New("active user agreement is required for recurring consent")
	}

	consent := domain.RecurringConsent{
		UserID:      userID,
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

// parsePayCallbackData decodes checkout callback payloads built by paymentKeyboard.
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

// parsePayConsentCallbackData decodes recurring opt-in toggle actions.
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
