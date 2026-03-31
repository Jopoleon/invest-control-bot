package recurring

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
	"github.com/Jopoleon/invest-control-bot/internal/messenger"
	"github.com/Jopoleon/invest-control-bot/internal/payment"
	"github.com/Jopoleon/invest-control-bot/internal/store"
)

var ErrRebillRequestFailed = errors.New("rebill request failed")

type RebillResult struct {
	OK        bool
	InvoiceID string
	Existing  bool
}

// Service owns recurring rebill orchestration and eligibility checks.
//
// The root app package injects store access, provider access, audit-event
// construction, and invoice-id generation so the recurring payment path can be
// tested independently from HTTP handlers and the application composition root.
type Service struct {
	Store                                 store.Store
	RobokassaService                      *payment.RobokassaService
	GenerateInvoiceID                     func() string
	BuildTargetAuditEvent                 func(context.Context, int64, string, int64, string, string, time.Time) domain.AuditEvent
	SendUserNotification                  func(context.Context, int64, string, messenger.OutgoingMessage) error
	ResolveUserByMessengerUserID          func(context.Context, int64) (domain.User, bool, error)
	ResolveTelegramMessengerUserID        func(context.Context, int64) (int64, bool, error)
	ResolveConnectorChannel               func(string, string) string
	ConnectorPeriodLabel                  func(domain.Connector) string
	RecurringCancelTitle                  string
	RecurringCancelSubsLoadFail           string
	RecurringCancelMissingSub             string
	RecurringCancelAlreadyOff             string
	RecurringCancelPersistFailed          string
	RecurringCancelNotification           func(string) string
	RecurringCancelSuccessForSubscription func(string) string
}

type CancelPageData struct {
	Title               string
	Token               string
	MessengerUserID     int64
	UserName            string
	AutoPayEnabled      bool
	SuccessMessage      string
	ErrorMessage        string
	ExpiresAt           string
	ActiveSubscriptions []CancelSubscriptionView
	OtherSubscriptions  int
}

type CancelSubscriptionView struct {
	SubscriptionID int64
	Name           string
	PriceRUB       int64
	PeriodLabel    string
	EndsAtLabel    string
	ChannelURL     string
}

const (
	// Short-period connectors exist only for live recurring smoke tests. They
	// cannot reuse monthly 72h/48h/24h windows because that would trigger
	// rebill almost immediately after the first payment.
	shortPeriodFirstAttemptLead   = 30 * time.Second
	shortPeriodSecondAttemptLead  = 10 * time.Second
	shortPeriodFirstAttemptFloor  = 15 * time.Second
	shortPeriodSecondAttemptFloor = 5 * time.Second
)

func (s *Service) TriggerRebill(ctx context.Context, subscriptionID int64, source string) (RebillResult, error) {
	if s.RobokassaService == nil {
		return RebillResult{}, errors.New("rebill provider is not configured")
	}

	subscription, found, err := s.Store.GetSubscriptionByID(ctx, subscriptionID)
	if err != nil {
		return RebillResult{}, err
	}
	if !found {
		return RebillResult{}, errors.New("subscription not found")
	}
	if subscription.Status != domain.SubscriptionStatusActive {
		return RebillResult{}, errors.New("subscription is not active")
	}
	if !subscription.AutoPayEnabled {
		return RebillResult{}, errors.New("autopay is disabled for subscription")
	}
	if pending, ok, err := s.Store.GetPendingRebillPaymentBySubscription(ctx, subscription.ID); err != nil {
		return RebillResult{}, err
	} else if ok {
		return RebillResult{OK: true, InvoiceID: pending.Token, Existing: true}, nil
	}

	parentPayment, found, err := s.Store.GetPaymentByID(ctx, subscription.PaymentID)
	if err != nil {
		return RebillResult{}, err
	}
	if !found || strings.TrimSpace(parentPayment.Token) == "" {
		return RebillResult{}, errors.New("parent payment is missing token")
	}

	connector, found, err := s.Store.GetConnector(ctx, subscription.ConnectorID)
	if err != nil {
		return RebillResult{}, err
	}
	if !found {
		return RebillResult{}, errors.New("connector not found")
	}

	invoiceID := s.GenerateInvoiceID()
	now := time.Now().UTC()
	pendingPayment := domain.Payment{
		Provider:          "robokassa",
		ProviderPaymentID: "rebill_parent:" + parentPayment.Token,
		Status:            domain.PaymentStatusPending,
		Token:             invoiceID,
		UserID:            subscription.UserID,
		ConnectorID:       subscription.ConnectorID,
		SubscriptionID:    subscription.ID,
		ParentPaymentID:   parentPayment.ID,
		AmountRUB:         connector.PriceRUB,
		AutoPayEnabled:    true,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if err := s.Store.CreatePayment(ctx, pendingPayment); err != nil {
		if existing, ok, lookupErr := s.Store.GetPendingRebillPaymentBySubscription(ctx, subscription.ID); lookupErr == nil && ok {
			return RebillResult{OK: true, InvoiceID: existing.Token, Existing: true}, nil
		}
		return RebillResult{}, err
	}

	createdPayment, found, err := s.Store.GetPaymentByToken(ctx, invoiceID)
	if err != nil {
		return RebillResult{}, err
	}
	if !found {
		return RebillResult{}, errors.New("created rebill payment not found")
	}
	pendingPayment = createdPayment

	if err := s.RobokassaService.CreateRebill(ctx, payment.RebillRequest{
		InvoiceID:         invoiceID,
		PreviousInvoiceID: parentPayment.Token,
		AmountRUB:         connector.PriceRUB,
		Description:       connector.Name,
	}); err != nil {
		if _, markErr := s.Store.UpdatePaymentFailed(ctx, pendingPayment.ID, "rebill_request_failed:"+parentPayment.Token, time.Now().UTC()); markErr != nil {
			return RebillResult{}, errors.Join(ErrRebillRequestFailed, markErr)
		}
		_ = s.Store.SaveAuditEvent(ctx, s.BuildTargetAuditEvent(
			ctx,
			subscription.UserID,
			"",
			subscription.ConnectorID,
			domain.AuditActionRebillRequestFailed,
			"subscription_id="+strconv.FormatInt(subscription.ID, 10)+";invoice_id="+invoiceID+";source="+source+";error="+err.Error(),
			time.Now().UTC(),
		))
		return RebillResult{}, ErrRebillRequestFailed
	}

	_ = s.Store.SaveAuditEvent(ctx, s.BuildTargetAuditEvent(
		ctx,
		subscription.UserID,
		"",
		subscription.ConnectorID,
		domain.AuditActionRebillRequested,
		"subscription_id="+strconv.FormatInt(subscription.ID, 10)+";invoice_id="+invoiceID+";parent="+parentPayment.Token+";source="+source,
		now,
	))

	return RebillResult{OK: true, InvoiceID: invoiceID}, nil
}

func (s *Service) ShouldTriggerScheduledRebill(ctx context.Context, sub domain.Subscription, now time.Time) (bool, error) {
	connector, found, err := s.Store.GetConnector(ctx, sub.ConnectorID)
	if err != nil {
		return false, err
	}
	if !found {
		return false, errors.New("connector not found")
	}

	targetAttempt := attemptOrdinalForConnector(now, sub.EndsAt, connector)
	if targetAttempt == 0 {
		return false, nil
	}

	payments, err := s.Store.ListPayments(ctx, domain.PaymentListQuery{
		UserID: sub.UserID,
		Limit:  500,
	})
	if err != nil {
		return false, err
	}

	attempts := 0
	for _, paymentRow := range payments {
		if paymentRow.SubscriptionID != sub.ID || paymentRow.ParentPaymentID <= 0 {
			continue
		}
		switch paymentRow.Status {
		case domain.PaymentStatusPending, domain.PaymentStatusPaid:
			return false, nil
		case domain.PaymentStatusFailed:
			attempts++
		default:
			attempts++
		}
	}

	return attempts < targetAttempt, nil
}

// BuildCancelPageData materializes the current cancel-page state for the
// validated messenger identity behind the signed public token.
func (s *Service) BuildCancelPageData(ctx context.Context, token string, messengerUserID int64, expiresAt time.Time, done string) (CancelPageData, int) {
	data := CancelPageData{
		Title:           s.RecurringCancelTitle,
		Token:           token,
		MessengerUserID: messengerUserID,
		ExpiresAt:       expiresAt.In(time.Local).Format("02.01.2006 15:04"),
	}
	if strings.TrimSpace(done) != "" && s.RecurringCancelSuccessForSubscription != nil {
		data.SuccessMessage = s.RecurringCancelSuccessForSubscription(done)
	}

	query := domain.SubscriptionListQuery{
		Status: domain.SubscriptionStatusActive,
		Limit:  20,
	}
	if user, found, err := s.ResolveUserByMessengerUserID(ctx, messengerUserID); err == nil && found {
		query.UserID = user.ID
	} else if err != nil {
		slog.Error("resolve user for public cancel page failed", "error", err, "messenger_user_id", messengerUserID)
		data.ErrorMessage = s.RecurringCancelSubsLoadFail
		return data, 500
	}

	subs, err := s.Store.ListSubscriptions(ctx, query)
	if err != nil {
		slog.Error("list subscriptions for public cancel page failed", "error", err, "messenger_user_id", messengerUserID)
		data.ErrorMessage = s.RecurringCancelSubsLoadFail
		return data, 500
	}

	if userName := s.resolveRecurringCancelUserName(ctx, subs, messengerUserID); userName != "" {
		data.UserName = userName
	}

	data.ActiveSubscriptions = make([]CancelSubscriptionView, 0, len(subs))
	for _, sub := range subs {
		connector, ok, err := s.Store.GetConnector(ctx, sub.ConnectorID)
		if err != nil || !ok {
			continue
		}
		if !sub.AutoPayEnabled {
			data.OtherSubscriptions++
			continue
		}
		data.ActiveSubscriptions = append(data.ActiveSubscriptions, CancelSubscriptionView{
			SubscriptionID: sub.ID,
			Name:           connector.Name,
			PriceRUB:       connector.PriceRUB,
			PeriodLabel:    s.ConnectorPeriodLabel(connector),
			EndsAtLabel:    sub.EndsAt.In(time.Local).Format("02.01.2006 15:04"),
			ChannelURL:     s.ResolveConnectorChannel(connector.ChannelURL, connector.ChatID),
		})
	}
	data.AutoPayEnabled = len(data.ActiveSubscriptions) > 0
	return data, 200
}

// ProcessCancelRequest applies the actual subscription mutation behind the
// public recurring-cancel page after the HTTP layer has already validated the
// signed token and selected subscription id.
func (s *Service) ProcessCancelRequest(ctx context.Context, token string, messengerUserID, subscriptionID int64, expiresAt, now time.Time) (string, CancelPageData, int) {
	sub, found, err := s.Store.GetSubscriptionByID(ctx, subscriptionID)
	if err != nil || !found || !s.subscriptionMatchesMessengerUserID(ctx, sub, messengerUserID) {
		data, status := s.BuildCancelPageData(ctx, token, messengerUserID, expiresAt, "")
		return "", withCancelError(data, s.RecurringCancelMissingSub), status
	}
	if sub.Status != domain.SubscriptionStatusActive || !sub.AutoPayEnabled {
		data, status := s.BuildCancelPageData(ctx, token, messengerUserID, expiresAt, "")
		return "", withCancelError(data, s.RecurringCancelAlreadyOff), status
	}
	if err := s.Store.SetSubscriptionAutoPayEnabled(ctx, sub.ID, false, now); err != nil {
		slog.Error("disable subscription autopay via public page failed", "error", err, "messenger_user_id", messengerUserID, "subscription_id", sub.ID)
		data, status := s.BuildCancelPageData(ctx, token, messengerUserID, expiresAt, "")
		return "", withCancelError(data, s.RecurringCancelPersistFailed), status
	}

	connectorName := ""
	if connector, found, err := s.Store.GetConnector(ctx, sub.ConnectorID); err == nil && found {
		connectorName = connector.Name
	}
	if err := s.Store.SaveAuditEvent(ctx, s.BuildTargetAuditEvent(
		ctx,
		sub.UserID,
		formatPreferredMessengerUserID(messengerUserID),
		sub.ConnectorID,
		domain.AuditActionAutopayDisabled,
		"source=web_cancel_page;subscription_id="+strconv.FormatInt(sub.ID, 10),
		now,
	)); err != nil {
		slog.Error("save audit event failed", "error", err, "action", domain.AuditActionAutopayDisabled)
	}
	if s.SendUserNotification != nil && s.RecurringCancelNotification != nil {
		msg := messenger.OutgoingMessage{Text: s.RecurringCancelNotification(connectorName)}
		if err := s.SendUserNotification(ctx, sub.UserID, formatPreferredMessengerUserID(messengerUserID), msg); err != nil {
			slog.Error("send public cancel confirmation failed", "error", err, "user_id", sub.UserID, "preferred_messenger_user_id", messengerUserID)
		}
	}
	return connectorName, CancelPageData{}, 0
}

func AttemptOrdinal(now, endsAt time.Time) int {
	return attemptOrdinalForPeriod(now, endsAt, 0)
}

func attemptOrdinalForConnector(now, endsAt time.Time, connector domain.Connector) int {
	return attemptOrdinalForPeriod(now, endsAt, connector.TestPeriodSeconds)
}

func attemptOrdinalForPeriod(now, endsAt time.Time, testPeriodSeconds int) int {
	remaining := endsAt.Sub(now)
	if testPeriodSeconds > 0 {
		return shortPeriodAttemptOrdinal(remaining, time.Duration(testPeriodSeconds)*time.Second)
	}
	switch {
	case remaining <= 0:
		return 0
	case remaining <= 24*time.Hour:
		return 3
	case remaining <= 48*time.Hour:
		return 2
	case remaining <= 72*time.Hour:
		return 1
	default:
		return 0
	}
}

func shortPeriodAttemptOrdinal(remaining, period time.Duration) int {
	if remaining <= 0 || period <= 0 {
		return 0
	}

	// Short smoke-test periods use explicit second-based windows. The scheduler
	// runs frequently enough to hit these windows without creating a rebill
	// almost immediately after the first payment.
	firstLead := minDuration(shortPeriodFirstAttemptLead, maxDuration(shortPeriodFirstAttemptFloor, period/2))
	secondLead := minDuration(shortPeriodSecondAttemptLead, maxDuration(shortPeriodSecondAttemptFloor, period/6))

	switch {
	case remaining <= secondLead:
		return 2
	case remaining <= firstLead:
		return 1
	default:
		return 0
	}
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}

func maxDuration(a, b time.Duration) time.Duration {
	if a > b {
		return a
	}
	return b
}

func (s *Service) subscriptionMatchesMessengerUserID(ctx context.Context, sub domain.Subscription, messengerUserID int64) bool {
	if sub.UserID <= 0 || messengerUserID <= 0 {
		return false
	}
	user, found, err := s.ResolveUserByMessengerUserID(ctx, messengerUserID)
	if err == nil && found {
		return user.ID == sub.UserID
	}
	telegramID, found, err := s.ResolveTelegramMessengerUserID(ctx, sub.UserID)
	if err != nil || !found {
		return false
	}
	return telegramID == messengerUserID
}

func (s *Service) resolveRecurringCancelUserName(ctx context.Context, subs []domain.Subscription, messengerUserID int64) string {
	for _, sub := range subs {
		if sub.UserID <= 0 {
			continue
		}
		user, found, err := s.Store.GetUserByID(ctx, sub.UserID)
		if err != nil {
			slog.Error("load user for public cancel page failed", "error", err, "user_id", sub.UserID, "messenger_user_id", messengerUserID)
			break
		}
		if found {
			if accounts, err := s.Store.ListUserMessengerAccounts(ctx, user.ID); err == nil {
				for _, account := range accounts {
					if account.MessengerKind == domain.MessengerKindTelegram {
						return firstNonEmpty(strings.TrimSpace(user.FullName), strings.TrimSpace(account.Username))
					}
				}
			}
			return strings.TrimSpace(user.FullName)
		}
	}
	user, found, err := s.ResolveUserByMessengerUserID(ctx, messengerUserID)
	if err != nil {
		slog.Error("load messenger user bridge for public cancel page failed", "error", err, "messenger_user_id", messengerUserID)
		return ""
	}
	if found {
		if accounts, err := s.Store.ListUserMessengerAccounts(ctx, user.ID); err == nil {
			for _, account := range accounts {
				if account.MessengerUserID == strconv.FormatInt(messengerUserID, 10) {
					return firstNonEmpty(strings.TrimSpace(user.FullName), strings.TrimSpace(account.Username))
				}
			}
		}
		return strings.TrimSpace(user.FullName)
	}
	return ""
}

func withCancelError(data CancelPageData, msg string) CancelPageData {
	data.ErrorMessage = msg
	return data
}

func formatPreferredMessengerUserID(raw int64) string {
	if raw <= 0 {
		return ""
	}
	return strconv.FormatInt(raw, 10)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
