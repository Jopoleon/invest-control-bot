package payments

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
	"github.com/Jopoleon/invest-control-bot/internal/max"
	"github.com/Jopoleon/invest-control-bot/internal/messenger"
	"github.com/Jopoleon/invest-control-bot/internal/store"
)

func preferredMessengerKindToDomain(kind messenger.Kind) domain.MessengerKind {
	switch kind {
	case messenger.KindMAX:
		return domain.MessengerKindMAX
	default:
		return domain.MessengerKindTelegram
	}
}

// Service owns payment-side business rules that do not need to live on HTTP
// handlers directly: payment activation and recurring failure notifications.
//
// The root app package injects the few cross-cutting dependencies that still
// connect this logic to messenger delivery, audit creation, and channel URL
// resolution, so the service stays testable without the full application
// object.
type Service struct {
	Store                   store.Store
	TelegramBotUsername     string
	SuccessChannelHint      string
	OpenChannelActionLabel  string
	MySubscriptionAction    string
	FailedRecurringText     string
	FailedRecurringButton   string
	PaymentSuccessMessage   func(domain.Payment, domain.Connector, time.Time) string
	BuildTelegramAccessLink func(context.Context, int64, domain.Connector, domain.Subscription) (string, error)
	ResolveMAXAccount       func(context.Context, int64) (domain.UserMessengerAccount, bool, error)
	AddMAXChatMembers       func(context.Context, int64, []int64) error
	ResolvePreferredKind    func(context.Context, int64, string) messenger.Kind
	SendUserNotification    func(context.Context, int64, string, messenger.OutgoingMessage) error
	BuildTargetAuditEvent   func(context.Context, int64, string, int64, string, string, time.Time) domain.AuditEvent
}

// ActivateSuccessfulPayment is the write-side heart of payment success
// handling.
//
// The function intentionally performs several phases in order:
//  1. mark the payment as paid, idempotently;
//  2. compute the subscription period and upsert the subscription row;
//  3. write subscription activation audit;
//  4. stop early on duplicate callbacks before any user-facing side effects;
//  5. resolve access delivery and notify the user.
//
// Keeping duplicate-callback protection between phases 3 and 4 is important:
// we still want activation state in storage to converge correctly, but we must
// not send duplicate success notifications or shift already-created periods.
//
// TODO: Wrap payment status update + subscription upsert + activation audit in
// a dedicated transactional store path. Today this orchestration is correct but
// spans several store calls, which makes the flow harder to reason about under
// partial DB failures.
func (s *Service) ActivateSuccessfulPayment(ctx context.Context, paymentRow domain.Payment, providerPaymentID string, now time.Time) {
	// Phase 1: converge payment state to paid and determine the effective paid
	// timestamp that should anchor the subscription period.
	paymentMarkedNow := false
	effectivePaidAt := now
	if paymentRow.Status != domain.PaymentStatusPaid {
		updated, err := s.Store.UpdatePaymentPaid(ctx, paymentRow.ID, providerPaymentID, now)
		if err != nil {
			slog.Error("update payment status failed", "error", err, "payment_id", paymentRow.ID)
			return
		}
		if updated {
			slog.Info("payment marked as paid", "payment_id", paymentRow.ID, "provider_payment_id", providerPaymentID)
			effectivePaidAt = now
			paymentMarkedNow = true
		} else {
			latestPayment, found, loadErr := s.Store.GetPaymentByToken(ctx, paymentRow.Token)
			if loadErr != nil {
				slog.Error("reload payment failed", "error", loadErr, "payment_id", paymentRow.ID)
			} else if found && latestPayment.PaidAt != nil {
				effectivePaidAt = *latestPayment.PaidAt
				paymentRow = latestPayment
			}
		}
	} else if paymentRow.PaidAt != nil {
		effectivePaidAt = *paymentRow.PaidAt
	}

	if !paymentMarkedNow {
		alreadyActivated, err := s.hasSubscriptionForPayment(ctx, paymentRow.UserID, paymentRow.ConnectorID, paymentRow.ID)
		if err != nil {
			slog.Error("check subscription activation state failed", "error", err, "payment_id", paymentRow.ID, "user_id", paymentRow.UserID, "connector_id", paymentRow.ConnectorID)
		} else if alreadyActivated {
			slog.Info("payment already activated, skip duplicate callback side effects", "payment_id", paymentRow.ID, "provider_payment_id", providerPaymentID)
			return
		}
	}

	// Phase 2: derive the subscription window. Renewal periods extend from the
	// later of "payment succeeded now" and "current active period end".
	endsAt := effectivePaidAt.AddDate(0, 0, 30)
	connector, connectorExists, err := s.Store.GetConnector(ctx, paymentRow.ConnectorID)
	if err != nil {
		slog.Error("load connector for subscription failed", "error", err, "connector_id", paymentRow.ConnectorID)
	} else if connectorExists {
		endsAt = connector.SubscriptionEndsAt(effectivePaidAt)
	}

	startAt := effectivePaidAt
	if latestSub, found, err := s.Store.GetLatestSubscriptionByUserConnector(ctx, paymentRow.UserID, paymentRow.ConnectorID); err != nil {
		slog.Error("load latest subscription failed", "error", err, "user_id", paymentRow.UserID, "connector_id", paymentRow.ConnectorID)
	} else if found && latestSub.Status == domain.SubscriptionStatusActive && latestSub.EndsAt.After(startAt) {
		startAt = latestSub.EndsAt
	}
	if connectorExists {
		endsAt = connector.SubscriptionEndsAt(startAt)
	} else {
		endsAt = startAt.AddDate(0, 0, 30)
	}
	if err := s.Store.UpsertSubscriptionByPayment(ctx, domain.Subscription{
		UserID:         paymentRow.UserID,
		ConnectorID:    paymentRow.ConnectorID,
		PaymentID:      paymentRow.ID,
		Status:         domain.SubscriptionStatusActive,
		AutoPayEnabled: paymentRow.AutoPayEnabled,
		StartsAt:       startAt,
		EndsAt:         endsAt,
		CreatedAt:      startAt,
		UpdatedAt:      now,
	}); err != nil {
		slog.Error("upsert subscription failed", "error", err, "payment_id", paymentRow.ID)
		return
	}
	activatedSub, found, err := s.loadSubscriptionForPayment(ctx, paymentRow.UserID, paymentRow.ConnectorID, paymentRow.ID)
	if err != nil {
		slog.Error("load activated subscription failed", "error", err, "payment_id", paymentRow.ID, "user_id", paymentRow.UserID, "connector_id", paymentRow.ConnectorID)
	}
	if !found {
		activatedSub = domain.Subscription{
			UserID:         paymentRow.UserID,
			ConnectorID:    paymentRow.ConnectorID,
			PaymentID:      paymentRow.ID,
			Status:         domain.SubscriptionStatusActive,
			AutoPayEnabled: paymentRow.AutoPayEnabled,
			StartsAt:       startAt,
			EndsAt:         endsAt,
			CreatedAt:      startAt,
			UpdatedAt:      now,
		}
	}

	// Phase 3: subscription activation is an auditable state transition even if
	// the user-facing notification later fails.
	if err := s.Store.SaveAuditEvent(ctx, s.BuildTargetAuditEvent(
		ctx,
		paymentRow.UserID,
		"",
		paymentRow.ConnectorID,
		domain.AuditActionSubscriptionActivated,
		"payment_id="+strconv.FormatInt(paymentRow.ID, 10),
		now,
	)); err != nil {
		slog.Error("save audit event failed", "error", err, "action", domain.AuditActionSubscriptionActivated)
	}

	// Phase 4: duplicate provider callbacks must stop here. By this point the
	// paid payment and its subscription row already converge to the right state,
	// but no delivery side effects should happen twice.
	if !paymentMarkedNow {
		return
	}

	// Phase 5: resolve destination delivery. This phase is intentionally
	// messenger-aware and connector-aware: single-destination connectors must not
	// depend on a user's globally preferred account.
	channelURL := ""
	accessSource := ""
	failureReason := "missing_access_destination"
	deliveryFailureReason := ""
	if connectorExists {
		preferredKind := messenger.KindTelegram
		if s.ResolvePreferredKind != nil {
			preferredKind = s.ResolvePreferredKind(ctx, paymentRow.UserID, "")
		}
		deliveryKind := connector.DeliveryMessengerKind(preferredMessengerKindToDomain(preferredKind))
		if deliveryKind == domain.MessengerKindTelegram && s.BuildTelegramAccessLink != nil {
			accessLink, err := s.BuildTelegramAccessLink(ctx, paymentRow.UserID, connector, activatedSub)
			if err != nil {
				slog.Error("build telegram access link failed", "error", err, "user_id", paymentRow.UserID, "connector_id", connector.ID, "payment_id", paymentRow.ID)
				if saveErr := s.Store.SaveAuditEvent(ctx, s.BuildTargetAuditEvent(
					ctx,
					paymentRow.UserID,
					"",
					paymentRow.ConnectorID,
					domain.AuditActionInviteLinkDeliveryFailed,
					"payment_id="+strconv.FormatInt(paymentRow.ID, 10)+";reason=invite_link_build_failed",
					now,
				)); saveErr != nil {
					slog.Error("save audit event failed", "error", saveErr, "action", domain.AuditActionInviteLinkDeliveryFailed)
				}
			} else if strings.TrimSpace(accessLink) != "" {
				channelURL = accessLink
				accessSource = "telegram_invite_link"
			}
		}
		if deliveryKind == domain.MessengerKindMAX {
			if chatID, ok := connector.ResolvedMAXChatID(); ok && s.ResolveMAXAccount != nil && s.AddMAXChatMembers != nil {
				account, found, err := s.ResolveMAXAccount(ctx, paymentRow.UserID)
				switch {
				case err != nil:
					slog.Error("resolve max account for access grant failed", "error", err, "user_id", paymentRow.UserID, "connector_id", connector.ID, "payment_id", paymentRow.ID)
					failureReason = "resolve_max_account_error"
					deliveryFailureReason = failureReason
				case !found:
					slog.Warn("max account missing for access grant", "user_id", paymentRow.UserID, "connector_id", connector.ID, "payment_id", paymentRow.ID)
					failureReason = "max_account_not_found"
					deliveryFailureReason = failureReason
				default:
					maxUserID, parseErr := strconv.ParseInt(strings.TrimSpace(account.MessengerUserID), 10, 64)
					if parseErr != nil || maxUserID <= 0 {
						slog.Error("invalid max account id for access grant", "error", parseErr, "user_id", paymentRow.UserID, "connector_id", connector.ID, "payment_id", paymentRow.ID, "messenger_user_id", account.MessengerUserID)
						failureReason = "invalid_max_account_id"
						deliveryFailureReason = failureReason
					} else if err := s.AddMAXChatMembers(ctx, chatID, []int64{maxUserID}); err != nil {
						deliveryFailureReason = appendAccessFailureDiagnostics("max_add_member_failed", err)
						logArgs := []any{
							"error", err,
							"user_id", paymentRow.UserID,
							"connector_id", connector.ID,
							"payment_id", paymentRow.ID,
							"chat_id", chatID,
							"messenger_user_id", maxUserID,
						}
						if mutationErr := maxMutationError(err); mutationErr != nil {
							logArgs = append(logArgs,
								"max_message", mutationErr.Message,
								"failed_user_ids", mutationErr.FailedUserIDs,
								"failed_user_details", mutationErr.FailedUserDetails,
							)
						}
						slog.Error("add max chat member failed", logArgs...)
						failureReason = "max_add_member_failed"
					} else {
						accessSource = "max_chat_member_added"
					}
				}
			} else if chatID, ok := connector.ResolvedMAXChatID(); ok && (s.ResolveMAXAccount == nil || s.AddMAXChatMembers == nil) {
				_ = chatID
				failureReason = "max_client_not_configured"
				deliveryFailureReason = failureReason
			} else if channelURL == "" && connector.HasAccessFor(domain.MessengerKindMAX) {
				failureReason = "missing_max_chat_id"
				deliveryFailureReason = failureReason
			}
		}
		// Delivery prefers the transport-native path first (Telegram invite link,
		// MAX add-member), then falls back to a connector-level access URL only if
		// that still represents a valid destination for the chosen messenger.
		if channelURL == "" {
			fallbackURL := connector.AccessURL(deliveryKind)
			if strings.TrimSpace(fallbackURL) != "" {
				channelURL = fallbackURL
				if strings.TrimSpace(accessSource) == "" {
					switch deliveryKind {
					case domain.MessengerKindMAX:
						accessSource = "max_channel_url"
					default:
						accessSource = "telegram_channel_url"
					}
				}
			} else if connector.HasAnyAccessDestination() {
				failureReason = "incompatible_access_destination"
			}
		}
		if channelURL != "" {
			slog.Info("payment access destination resolved",
				"user_id", paymentRow.UserID,
				"payment_id", paymentRow.ID,
				"connector_id", connector.ID,
				"source", accessSource,
			)
		} else {
			slog.Warn("payment access destination missing",
				"user_id", paymentRow.UserID,
				"payment_id", paymentRow.ID,
				"connector_id", connector.ID,
				"chat_id_configured", strings.TrimSpace(connector.ChatID) != "",
				"telegram_channel_url_configured", strings.TrimSpace(connector.ChannelURL) != "",
				"max_channel_url_configured", strings.TrimSpace(connector.MAXChannelURL) != "",
				"reason", failureReason,
			)
		}
	}
	successText := s.PaymentSuccessMessage(paymentRow, connector, endsAt)
	if channelURL == "" && connectorExists && failureReason == "incompatible_access_destination" {
		switch {
		case connector.HasAccessFor(domain.MessengerKindTelegram):
			successText += "\n\n⚠️ Доступ по этому тарифу выдается в Telegram. В текущем мессенджере кнопка входа недоступна."
		case connector.HasAccessFor(domain.MessengerKindMAX):
			successText += "\n\n⚠️ Доступ по этому тарифу выдается в MAX. В текущем мессенджере кнопка входа недоступна."
		}
	}
	message := messenger.OutgoingMessage{Text: successText}
	if channelURL != "" {
		message.Text += s.SuccessChannelHint
		message.Buttons = [][]messenger.ActionButton{
			{{Text: s.OpenChannelActionLabel, URL: channelURL}},
			{{Text: s.MySubscriptionAction, Action: "menu:subscription"}},
		}
	} else {
		message.Buttons = [][]messenger.ActionButton{
			{{Text: s.MySubscriptionAction, Action: "menu:subscription"}},
		}
	}
	if err := s.SendUserNotification(ctx, paymentRow.UserID, "", message); err != nil {
		slog.Error("send payment success message failed", "error", err, "user_id", paymentRow.UserID, "payment_id", paymentRow.ID)
		details := "payment_id=" + strconv.FormatInt(paymentRow.ID, 10) + ";reason=notification_send_failed"
		if channelURL != "" {
			details += ";source=" + accessSource
		}
		if saveErr := s.Store.SaveAuditEvent(ctx, s.BuildTargetAuditEvent(
			ctx,
			paymentRow.UserID,
			"",
			paymentRow.ConnectorID,
			domain.AuditActionAccessDeliveryFailed,
			details,
			now,
		)); saveErr != nil {
			slog.Error("save audit event failed", "error", saveErr, "action", domain.AuditActionAccessDeliveryFailed)
		}
		return
	}
	if channelURL != "" {
		if err := s.Store.SaveAuditEvent(ctx, s.BuildTargetAuditEvent(
			ctx,
			paymentRow.UserID,
			"",
			paymentRow.ConnectorID,
			domain.AuditActionPaymentAccessReady,
			"payment_id="+strconv.FormatInt(paymentRow.ID, 10)+";source="+accessSource,
			now,
		)); err != nil {
			slog.Error("save audit event failed", "error", err, "action", domain.AuditActionPaymentAccessReady)
		}
		if deliveryFailureReason != "" {
			if err := s.Store.SaveAuditEvent(ctx, s.BuildTargetAuditEvent(
				ctx,
				paymentRow.UserID,
				"",
				paymentRow.ConnectorID,
				domain.AuditActionAccessDeliveryFailed,
				"payment_id="+strconv.FormatInt(paymentRow.ID, 10)+";reason="+deliveryFailureReason,
				now,
			)); err != nil {
				slog.Error("save audit event failed", "error", err, "action", domain.AuditActionAccessDeliveryFailed)
			}
		}
	} else {
		if err := s.Store.SaveAuditEvent(ctx, s.BuildTargetAuditEvent(
			ctx,
			paymentRow.UserID,
			"",
			paymentRow.ConnectorID,
			domain.AuditActionAccessDeliveryFailed,
			"payment_id="+strconv.FormatInt(paymentRow.ID, 10)+";reason="+failureReason,
			now,
		)); err != nil {
			slog.Error("save audit event failed", "error", err, "action", domain.AuditActionAccessDeliveryFailed)
		}
	}
	if err := s.Store.SaveAuditEvent(ctx, s.BuildTargetAuditEvent(
		ctx,
		paymentRow.UserID,
		"",
		paymentRow.ConnectorID,
		domain.AuditActionPaymentSuccessNotified,
		"payment_id="+strconv.FormatInt(paymentRow.ID, 10),
		now,
	)); err != nil {
		slog.Error("save audit event failed", "error", err, "action", domain.AuditActionPaymentSuccessNotified)
	}
}

func maxMutationError(err error) *max.MutationError {
	var mutationErr *max.MutationError
	if errors.As(err, &mutationErr) {
		return mutationErr
	}
	return nil
}

func appendAccessFailureDiagnostics(reason string, err error) string {
	details := "reason=" + sanitizeAuditDetailValue(reason)
	if mutationErr := maxMutationError(err); mutationErr != nil {
		if strings.TrimSpace(mutationErr.Message) != "" {
			details += ";max_message=" + sanitizeAuditDetailValue(mutationErr.Message)
		}
		if len(mutationErr.FailedUserIDs) > 0 {
			ids := make([]string, 0, len(mutationErr.FailedUserIDs))
			for _, id := range mutationErr.FailedUserIDs {
				ids = append(ids, strconv.FormatInt(id, 10))
			}
			details += ";failed_user_ids=" + sanitizeAuditDetailValue(strings.Join(ids, ","))
		}
		if len(mutationErr.FailedUserDetails) > 0 {
			parts := make([]string, 0, len(mutationErr.FailedUserDetails))
			for _, detail := range mutationErr.FailedUserDetails {
				chunks := make([]string, 0, 3)
				if detail.UserID > 0 {
					chunks = append(chunks, "user_id:"+strconv.FormatInt(detail.UserID, 10))
				}
				if strings.TrimSpace(detail.Code) != "" {
					chunks = append(chunks, "code:"+strings.TrimSpace(detail.Code))
				}
				if strings.TrimSpace(detail.Message) != "" {
					chunks = append(chunks, "message:"+strings.TrimSpace(detail.Message))
				}
				if len(chunks) > 0 {
					parts = append(parts, "{"+strings.Join(chunks, ", ")+"}")
				}
			}
			if len(parts) > 0 {
				details += ";failed_user_details=" + sanitizeAuditDetailValue(strings.Join(parts, ","))
			}
		}
		return details
	}
	if err != nil {
		details += ";error=" + sanitizeAuditDetailValue(err.Error())
	}
	return details
}

func sanitizeAuditDetailValue(value string) string {
	return strings.NewReplacer(";", ",", "=", ":", "\n", " ", "\r", " ").Replace(strings.TrimSpace(value))
}

func (s *Service) hasSubscriptionForPayment(ctx context.Context, userID, connectorID, paymentID int64) (bool, error) {
	sub, found, err := s.loadSubscriptionForPayment(ctx, userID, connectorID, paymentID)
	if err != nil {
		return false, err
	}
	return found && sub.PaymentID == paymentID, nil
}

func (s *Service) loadSubscriptionForPayment(ctx context.Context, userID, connectorID, paymentID int64) (domain.Subscription, bool, error) {
	// The store interface does not yet expose a direct "get subscription by
	// payment id" method, so payment success currently scans the user's
	// connector-local subscription rows.
	//
	// TODO: Add a direct store method keyed by payment_id. This will simplify
	// payment activation, avoid broad list scans, and make the duplicate-callback
	// path cheaper and easier to read.
	subs, err := s.Store.ListSubscriptions(ctx, domain.SubscriptionListQuery{
		UserID:      userID,
		ConnectorID: connectorID,
		Limit:       200,
	})
	if err != nil {
		return domain.Subscription{}, false, err
	}
	for _, sub := range subs {
		if sub.PaymentID == paymentID {
			return sub, true, nil
		}
	}
	return domain.Subscription{}, false, nil
}

func (s *Service) NotifyFailedRecurringPayment(ctx context.Context, paymentRow domain.Payment) {
	if !paymentRow.AutoPayEnabled || paymentRow.SubscriptionID <= 0 {
		return
	}
	connector, found, err := s.Store.GetConnector(ctx, paymentRow.ConnectorID)
	if err != nil {
		slog.Error("load connector for failed recurring payment notification failed", "error", err, "connector_id", paymentRow.ConnectorID, "payment_id", paymentRow.ID)
		return
	}
	if !found {
		return
	}

	renewURL := buildBotStartURL(s.TelegramBotUsername, connector.StartPayload)
	message := messenger.OutgoingMessage{Text: s.FailedRecurringText}
	if renewURL != "" {
		message.Buttons = [][]messenger.ActionButton{{
			{Text: s.FailedRecurringButton, URL: renewURL},
		}}
	}

	if err := s.SendUserNotification(ctx, paymentRow.UserID, "", message); err != nil {
		slog.Warn("failed recurring payment notify failed", "payment_id", paymentRow.ID, "user_id", paymentRow.UserID, "error", err)
		return
	}
	if err := s.Store.SaveAuditEvent(ctx, s.BuildTargetAuditEvent(
		ctx,
		paymentRow.UserID,
		"",
		paymentRow.ConnectorID,
		domain.AuditActionRecurringPaymentFailedNotice,
		"payment_id="+strconv.FormatInt(paymentRow.ID, 10),
		time.Now().UTC(),
	)); err != nil {
		slog.Error("save audit event failed", "error", err, "action", domain.AuditActionRecurringPaymentFailedNotice)
	}
}

func buildBotStartURL(botUsername, startPayload string) string {
	username := strings.TrimSpace(strings.TrimPrefix(botUsername, "@"))
	payload := strings.TrimSpace(startPayload)
	if username == "" || payload == "" {
		return ""
	}
	return "https://t.me/" + username + "?start=" + payload
}
