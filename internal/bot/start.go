package bot

import (
	"context"
	"log/slog"
	"strconv"
	"strings"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
	"github.com/Jopoleon/invest-control-bot/internal/messenger"
)

// handleStart resolves connector by payload and shows tariff card with consent button.
func (h *Handler) handleStart(ctx context.Context, msg messenger.IncomingMessage) {
	parts := strings.Fields(strings.TrimSpace(msg.Text))
	if len(parts) < 2 {
		h.sendTo(ctx, msg.ChatID, msg.User, botMsgStartUsage)
		return
	}

	payload := strings.TrimSpace(parts[1])
	connector, ok, err := h.store.GetConnectorByStartPayload(ctx, payload)
	if err != nil {
		slog.Error("load connector by payload failed", "error", err, "payload", payload)
		return
	}
	if !ok {
		if id, parseErr := strconv.ParseInt(payload, 10, 64); parseErr == nil && id > 0 {
			connector, ok, err = h.store.GetConnector(ctx, id)
			if err != nil {
				slog.Error("fallback load connector by id failed", "error", err, "connector_id", payload)
				return
			}
		}
	}
	if !ok || !connector.IsActive {
		h.sendTo(ctx, msg.ChatID, msg.User, botMsgConnectorUnavailable)
		return
	}
	h.logAuditEvent(ctx, msg.User, connector.ID, domain.AuditActionStartOpened, "payload="+payload)

	offerURL := connector.OfferURL
	if offerURL == "" {
		offerURL = h.resolveLegalURL(ctx, domain.LegalDocumentTypeOffer)
	}
	privacyURL := connector.PrivacyURL
	if privacyURL == "" {
		privacyURL = h.resolveLegalURL(ctx, domain.LegalDocumentTypePrivacy)
	}

	out := messenger.OutgoingMessage{
		Text: botStartCardText(connector, offerURL, privacyURL),
	}
	if warning := botConnectorAccessMismatchWarning(connector, msg.User.Kind); warning != "" {
		out.Text += "\n\n" + warning
	} else {
		out.Buttons = [][]messenger.ActionButton{{
			buttonAction(botBtnAcceptTerms, "accept_terms:"+strconv.FormatInt(connector.ID, 10)),
		}}
	}

	if err := h.sender.Send(ctx, recipientRef(msg.ChatID, msg.User), out); err != nil {
		slog.Error("send start message failed", "error", err, "chat_id", msg.ChatID, "connector_id", connector.ID)
	}
}

func (h *Handler) resolveLegalURL(ctx context.Context, docType domain.LegalDocumentType) string {
	if url, _, found := h.resolveLegalDocumentURL(ctx, docType); found {
		return url
	}

	switch docType {
	case domain.LegalDocumentTypeOffer:
		return "https://example.com/contract"
	case domain.LegalDocumentTypePrivacy:
		return "https://example.com/policy"
	case domain.LegalDocumentTypeUserAgreement:
		return ""
	default:
		return "https://example.com"
	}
}

func (h *Handler) resolveLegalDocument(ctx context.Context, docType domain.LegalDocumentType) (domain.LegalDocument, bool) {
	doc, found, err := h.store.GetActiveLegalDocument(ctx, docType)
	if err != nil {
		slog.Error("load active legal document failed", "error", err, "doc_type", docType)
		return domain.LegalDocument{}, false
	}
	return doc, found
}
