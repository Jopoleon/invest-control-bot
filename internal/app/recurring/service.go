package recurring

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/app/periodpolicy"
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

type ScheduledRebillDecision struct {
	Trigger        bool
	Reason         string
	ShortDuration  bool
	Remaining      time.Duration
	TargetAttempt  int
	FailedAttempts int
	PendingPayment *domain.Payment
	Connector      domain.Connector
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
	TelegramBotChatURL                    string
	MAXBotChatURL                         string
	RecurringCancelTitle                  string
	RecurringCancelSubsLoadFail           string
	RecurringCancelMissingSub             string
	RecurringCancelAlreadyOff             string
	RecurringCancelStaleSubmit            string
	RecurringCancelPersistFailed          string
	RecurringCancelNotification           func(string) string
	RecurringCancelSuccessForSubscription func(string) string
	RecurringCancelOpenTelegramLabel      string
	RecurringCancelOpenMAXLabel           string
}

type CancelPageData struct {
	PageState           string
	Title               string
	Token               string
	MessengerUserID     int64
	UserName            string
	UserAccountLabel    string
	AutoPayEnabled      bool
	SuccessMessage      string
	ErrorMessage        string
	ReturnURL           string
	ReturnLabel         string
	ExpiresAt           string
	AutoPayCount        int
	ActiveAccessCount   int
	ActiveSubscriptions []CancelSubscriptionView
	FutureSubscriptions []CancelSubscriptionView
	OtherSubscriptions  int
}

type CancelSubscriptionView struct {
	SubscriptionID int64
	Name           string
	PriceRUB       int64
	PeriodLabel    string
	StartsAtLabel  string
	EndsAtLabel    string
	ChannelURL     string
}

const (
	cancelPageStateDefault      = "default"
	cancelPageStateSuccess      = "success"
	cancelPageStateStaleSuccess = "stale_success"
	cancelPageStateAlreadyOff   = "already_off"
	cancelPageStateStaleSubmit  = "stale_submit"
	cancelPageStateExpiredLink  = "expired_link"
	cancelPageStateInvalidLink  = "invalid_link"
	cancelPageStateError        = "error"
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
	rootRecurringPayment, err := s.resolveRecurringRootPayment(ctx, parentPayment)
	if err != nil {
		return RebillResult{}, err
	}
	if strings.TrimSpace(rootRecurringPayment.Token) == "" {
		return RebillResult{}, errors.New("root recurring payment is missing token")
	}

	connector, found, err := s.Store.GetConnector(ctx, subscription.ConnectorID)
	if err != nil {
		return RebillResult{}, err
	}
	if !found {
		return RebillResult{}, errors.New("connector not found")
	}
	if !connector.SupportsRecurring() {
		return RebillResult{}, errors.New("connector does not support recurring")
	}

	invoiceID := s.GenerateInvoiceID()
	now := time.Now().UTC()
	pendingPayment := domain.Payment{
		Provider:          "robokassa",
		ProviderPaymentID: "rebill_parent:" + rootRecurringPayment.Token,
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
		PreviousInvoiceID: rootRecurringPayment.Token,
		AmountRUB:         connector.PriceRUB,
		Description:       connector.Name,
	}); err != nil {
		if _, markErr := s.Store.UpdatePaymentFailed(ctx, pendingPayment.ID, "rebill_request_failed:"+rootRecurringPayment.Token, time.Now().UTC()); markErr != nil {
			return RebillResult{}, errors.Join(ErrRebillRequestFailed, markErr)
		}
		s.saveAuditEvent(ctx,
			subscription.UserID,
			"",
			subscription.ConnectorID,
			domain.AuditActionRebillRequestFailed,
			"subscription_id="+strconv.FormatInt(subscription.ID, 10)+";invoice_id="+invoiceID+";source="+source+";error="+trimRecurringAuditValue(err.Error()),
			time.Now().UTC(),
		)
		return RebillResult{}, ErrRebillRequestFailed
	}

	s.saveAuditEvent(ctx,
		subscription.UserID,
		"",
		subscription.ConnectorID,
		domain.AuditActionRebillRequested,
		"subscription_id="+strconv.FormatInt(subscription.ID, 10)+";invoice_id="+invoiceID+";parent="+rootRecurringPayment.Token+";source="+source,
		now,
	)

	return RebillResult{OK: true, InvoiceID: invoiceID}, nil
}

func (s *Service) resolveRecurringRootPayment(ctx context.Context, paymentRow domain.Payment) (domain.Payment, error) {
	current := paymentRow
	for hops := 0; hops < 32; hops++ {
		if current.ParentPaymentID <= 0 {
			return current, nil
		}
		parent, found, err := s.Store.GetPaymentByID(ctx, current.ParentPaymentID)
		if err != nil {
			return domain.Payment{}, err
		}
		if !found {
			return domain.Payment{}, errors.New("root recurring payment not found")
		}
		current = parent
	}
	return domain.Payment{}, errors.New("recurring payment chain is too deep")
}

func (s *Service) ShouldTriggerScheduledRebill(ctx context.Context, sub domain.Subscription, now time.Time) (bool, error) {
	decision, err := s.EvaluateScheduledRebill(ctx, sub, now)
	if err != nil {
		return false, err
	}
	return decision.Trigger, nil
}

func (s *Service) EvaluateScheduledRebill(ctx context.Context, sub domain.Subscription, now time.Time) (ScheduledRebillDecision, error) {
	connector, found, err := s.Store.GetConnector(ctx, sub.ConnectorID)
	if err != nil {
		return ScheduledRebillDecision{}, err
	}
	if !found {
		return ScheduledRebillDecision{}, errors.New("connector not found")
	}
	decision := ScheduledRebillDecision{
		Connector:     connector,
		ShortDuration: periodpolicy.Resolve(connector).ShortDuration,
		Remaining:     sub.EndsAt.Sub(now),
	}
	if sub.StartsAt.After(now) {
		decision.Reason = "subscription_not_started"
		return decision, nil
	}
	if !connector.SupportsRecurring() {
		decision.Reason = "connector_not_recurring"
		return decision, nil
	}

	payments, err := s.Store.ListPayments(ctx, domain.PaymentListQuery{
		UserID: sub.UserID,
		Limit:  500,
	})
	if err != nil {
		return ScheduledRebillDecision{}, err
	}

	attempts := 0
	for _, paymentRow := range payments {
		if paymentRow.SubscriptionID != sub.ID || paymentRow.ParentPaymentID <= 0 {
			continue
		}
		switch paymentRow.Status {
		case domain.PaymentStatusPending, domain.PaymentStatusPaid:
			decision.PendingPayment = &paymentRow
			if paymentRow.Status == domain.PaymentStatusPending {
				decision.Reason = "pending_rebill_exists"
			} else {
				decision.Reason = "rebill_already_paid"
			}
			decision.FailedAttempts = attempts
			return decision, nil
		case domain.PaymentStatusFailed:
			attempts++
		default:
			attempts++
		}
	}

	decision.FailedAttempts = attempts
	decision.TargetAttempt = attemptOrdinalForConnector(now, sub.EndsAt, connector)
	if decision.TargetAttempt == 0 {
		decision.Reason = "outside_rebill_window"
		return decision, nil
	}
	if attempts >= decision.TargetAttempt {
		decision.Reason = "attempt_budget_exhausted"
		return decision, nil
	}
	decision.Trigger = true
	decision.Reason = "rebill_due"
	return decision, nil
}

func (s *Service) ReportStalePendingRebill(ctx context.Context, sub domain.Subscription, decision ScheduledRebillDecision, now time.Time) {
	if decision.PendingPayment == nil || !decision.ShortDuration {
		return
	}
	pending := *decision.PendingPayment
	if pending.Status != domain.PaymentStatusPending {
		return
	}
	timing := periodpolicy.Resolve(decision.Connector)
	if !timing.ShouldDeferExpiration(now, sub.EndsAt) {
		return
	}
	if now.Before(sub.EndsAt) {
		return
	}
	details := "subscription_id=" + strconv.FormatInt(sub.ID, 10) +
		";payment_id=" + strconv.FormatInt(pending.ID, 10) +
		";invoice_id=" + pending.Token +
		";pending_since=" + pending.CreatedAt.UTC().Format(time.RFC3339) +
		";ends_at=" + sub.EndsAt.UTC().Format(time.RFC3339)
	alreadyLogged, err := s.hasStalePendingAudit(ctx, sub, pending.ID)
	if err != nil {
		slog.Error("lookup stale rebill audit failed", "error", err, "subscription_id", sub.ID, "payment_id", pending.ID)
	}
	if alreadyLogged {
		return
	}
	slog.Warn("stale pending rebill without callback",
		"subscription_id", sub.ID,
		"payment_id", pending.ID,
		"invoice_id", pending.Token,
		"connector_id", sub.ConnectorID,
		"user_id", sub.UserID,
		"pending_age", now.Sub(pending.CreatedAt),
		"ended_ago", now.Sub(sub.EndsAt),
	)
	s.saveAuditEvent(ctx, sub.UserID, "", sub.ConnectorID, domain.AuditActionRebillPendingStale, details, now)
}

func (s *Service) saveAuditEvent(ctx context.Context, userID int64, preferredMessengerUserID string, connectorID int64, action, details string, createdAt time.Time) {
	if err := s.Store.SaveAuditEvent(ctx, s.BuildTargetAuditEvent(ctx, userID, preferredMessengerUserID, connectorID, action, details, createdAt)); err != nil {
		slog.Error("save audit event failed", "error", err, "action", action, "user_id", userID, "connector_id", connectorID)
	}
}

func trimRecurringAuditValue(raw string) string {
	value := strings.TrimSpace(raw)
	value = strings.NewReplacer(";", ",", "=", ":").Replace(value)
	if len(value) <= 240 {
		return value
	}
	return value[:237] + "..."
}

func (s *Service) hasStalePendingAudit(ctx context.Context, sub domain.Subscription, paymentID int64) (bool, error) {
	events, _, err := s.Store.ListAuditEvents(ctx, domain.AuditEventListQuery{
		TargetUserID: sub.UserID,
		ConnectorID:  sub.ConnectorID,
		Action:       domain.AuditActionRebillPendingStale,
		Search:       "payment_id=" + strconv.FormatInt(paymentID, 10),
		Page:         1,
		PageSize:     1,
		SortBy:       "created_at",
		SortDesc:     true,
	})
	if err != nil {
		return false, err
	}
	return len(events) > 0, nil
}

// BuildCancelPageData materializes the current cancel-page state for the
// validated messenger identity behind the signed public token.
func (s *Service) BuildCancelPageData(ctx context.Context, token string, messengerUserID int64, expiresAt time.Time, done, pageState string) (CancelPageData, int) {
	data := CancelPageData{
		PageState:       cancelPageStateDefault,
		Title:           s.RecurringCancelTitle,
		Token:           token,
		MessengerUserID: messengerUserID,
		ExpiresAt:       expiresAt.In(time.Local).Format("02.01.2006 15:04"),
	}
	if strings.TrimSpace(pageState) != "" {
		data.PageState = pageState
	}
	if strings.TrimSpace(done) != "" && s.RecurringCancelSuccessForSubscription != nil {
		data.SuccessMessage = s.RecurringCancelSuccessForSubscription(done)
		if data.PageState == cancelPageStateDefault {
			data.PageState = cancelPageStateSuccess
		}
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

	now := time.Now().UTC()
	account := domain.UserMessengerAccount{}
	if userName, accountLabel, resolvedAccount := s.resolveRecurringCancelUserIdentity(ctx, subs, messengerUserID); userName != "" || accountLabel != "" {
		data.UserName = userName
		data.UserAccountLabel = accountLabel
		account = resolvedAccount
		data.ReturnURL, data.ReturnLabel = s.resolveRecurringCancelReturn(account)
	}

	data.ActiveSubscriptions = make([]CancelSubscriptionView, 0, len(subs))
	data.FutureSubscriptions = make([]CancelSubscriptionView, 0, len(subs))
	for _, sub := range subs {
		connector, ok, err := s.Store.GetConnector(ctx, sub.ConnectorID)
		if err != nil || !ok {
			continue
		}
		if !sub.AutoPayEnabled {
			data.OtherSubscriptions++
			continue
		}
		view := CancelSubscriptionView{
			SubscriptionID: sub.ID,
			Name:           connector.Name,
			PriceRUB:       connector.PriceRUB,
			PeriodLabel:    s.ConnectorPeriodLabel(connector),
			StartsAtLabel:  sub.StartsAt.In(time.Local).Format("02.01.2006 15:04"),
			EndsAtLabel:    sub.EndsAt.In(time.Local).Format("02.01.2006 15:04"),
			ChannelURL:     connector.AccessURL(account.MessengerKind),
		}
		switch {
		case sub.IsFutureActiveAt(now):
			data.FutureSubscriptions = append(data.FutureSubscriptions, view)
		case sub.IsCurrentActiveAt(now):
			data.ActiveSubscriptions = append(data.ActiveSubscriptions, view)
		default:
			data.OtherSubscriptions++
		}
	}
	data.ActiveAccessCount = len(data.ActiveSubscriptions)
	data.AutoPayCount = len(data.ActiveSubscriptions) + len(data.FutureSubscriptions)
	data.AutoPayEnabled = data.AutoPayCount > 0
	return data, 200
}

// ProcessCancelRequest applies the actual subscription mutation behind the
// public recurring-cancel page after the HTTP layer has already validated the
// signed token and selected subscription id.
func (s *Service) ProcessCancelRequest(ctx context.Context, token string, messengerUserID, subscriptionID int64, expiresAt, now time.Time) (string, string, CancelPageData, int) {
	sub, found, err := s.Store.GetSubscriptionByID(ctx, subscriptionID)
	if err != nil || !found || !s.subscriptionMatchesMessengerUserID(ctx, sub, messengerUserID) {
		data, status := s.BuildCancelPageData(ctx, token, messengerUserID, expiresAt, "", cancelPageStateError)
		return "", "", withCancelError(data, s.RecurringCancelMissingSub), status
	}
	cancelSub, resolution, found, err := s.resolveActiveCancelableSubscription(ctx, sub, messengerUserID)
	if err != nil {
		slog.Error("resolve current subscription for public cancel page failed", "error", err, "messenger_user_id", messengerUserID, "requested_subscription_id", sub.ID)
		data, status := s.BuildCancelPageData(ctx, token, messengerUserID, expiresAt, "", cancelPageStateError)
		return "", "", withCancelError(data, s.RecurringCancelPersistFailed), status
	}
	if !found {
		state := cancelPageStateAlreadyOff
		msg := s.RecurringCancelAlreadyOff
		if resolution == cancelResolutionStaleSubmit {
			state = cancelPageStateStaleSubmit
			msg = s.RecurringCancelStaleSubmit
		}
		data, status := s.BuildCancelPageData(ctx, token, messengerUserID, expiresAt, "", state)
		return "", "", withCancelError(data, msg), status
	}
	cancelableSubs, err := s.listCancelableChainSubscriptions(ctx, cancelSub, messengerUserID)
	if err != nil {
		slog.Error("list cancelable subscription chain via public page failed", "error", err, "messenger_user_id", messengerUserID, "subscription_id", cancelSub.ID, "connector_id", cancelSub.ConnectorID)
		data, status := s.BuildCancelPageData(ctx, token, messengerUserID, expiresAt, "", cancelPageStateError)
		return "", "", withCancelError(data, s.RecurringCancelPersistFailed), status
	}
	if len(cancelableSubs) == 0 {
		state := cancelPageStateAlreadyOff
		msg := s.RecurringCancelAlreadyOff
		if cancelSub.ID != sub.ID {
			state = cancelPageStateStaleSubmit
			msg = s.RecurringCancelStaleSubmit
		}
		data, status := s.BuildCancelPageData(ctx, token, messengerUserID, expiresAt, "", state)
		return "", "", withCancelError(data, msg), status
	}
	disabledIDs := make([]string, 0, len(cancelableSubs))
	for _, item := range cancelableSubs {
		if err := s.Store.SetSubscriptionAutoPayEnabled(ctx, item.ID, false, now); err != nil {
			slog.Error("disable subscription autopay via public page failed", "error", err, "messenger_user_id", messengerUserID, "subscription_id", item.ID)
			data, status := s.BuildCancelPageData(ctx, token, messengerUserID, expiresAt, "", cancelPageStateError)
			return "", "", withCancelError(data, s.RecurringCancelPersistFailed), status
		}
		disabledIDs = append(disabledIDs, strconv.FormatInt(item.ID, 10))
	}

	connectorName := ""
	if connector, found, err := s.Store.GetConnector(ctx, cancelSub.ConnectorID); err == nil && found {
		connectorName = connector.Name
	}
	details := "source=web_cancel_page;subscription_id=" + strconv.FormatInt(cancelSub.ID, 10)
	if cancelSub.ID != sub.ID {
		details += ";requested_subscription_id=" + strconv.FormatInt(sub.ID, 10)
	}
	details += ";disabled_subscription_ids=" + strings.Join(disabledIDs, ",")
	if err := s.Store.SaveAuditEvent(ctx, s.BuildTargetAuditEvent(
		ctx,
		cancelSub.UserID,
		formatPreferredMessengerUserID(messengerUserID),
		cancelSub.ConnectorID,
		domain.AuditActionAutopayDisabled,
		details,
		now,
	)); err != nil {
		slog.Error("save audit event failed", "error", err, "action", domain.AuditActionAutopayDisabled)
	}
	if s.SendUserNotification != nil && s.RecurringCancelNotification != nil {
		msg := messenger.OutgoingMessage{Text: s.RecurringCancelNotification(connectorName)}
		if err := s.SendUserNotification(ctx, cancelSub.UserID, formatPreferredMessengerUserID(messengerUserID), msg); err != nil {
			slog.Error("send public cancel confirmation failed", "error", err, "user_id", cancelSub.UserID, "preferred_messenger_user_id", messengerUserID)
		}
	}
	resultState := cancelPageStateSuccess
	if cancelSub.ID != sub.ID {
		resultState = cancelPageStateStaleSuccess
	}
	return connectorName, resultState, CancelPageData{}, 0
}

func (s *Service) listCancelableChainSubscriptions(ctx context.Context, activeSub domain.Subscription, messengerUserID int64) ([]domain.Subscription, error) {
	subs, err := s.Store.ListSubscriptions(ctx, domain.SubscriptionListQuery{
		UserID:      activeSub.UserID,
		ConnectorID: activeSub.ConnectorID,
		Status:      domain.SubscriptionStatusActive,
		Limit:       50,
	})
	if err != nil {
		return nil, err
	}

	out := make([]domain.Subscription, 0, len(subs))
	for _, sub := range subs {
		if !sub.AutoPayEnabled {
			continue
		}
		if !s.subscriptionMatchesMessengerUserID(ctx, sub, messengerUserID) {
			continue
		}
		out = append(out, sub)
	}
	return out, nil
}

// resolveActiveCancelableSubscription keeps public cancel links usable for
// short-period recurring flows where the active subscription may rotate between
// page render and form submit.
func (s *Service) resolveActiveCancelableSubscription(ctx context.Context, requested domain.Subscription, messengerUserID int64) (domain.Subscription, string, bool, error) {
	if requested.Status == domain.SubscriptionStatusActive && requested.AutoPayEnabled {
		return requested, cancelResolutionExact, true, nil
	}
	latest, found, err := s.Store.GetLatestSubscriptionByUserConnector(ctx, requested.UserID, requested.ConnectorID)
	if err != nil {
		return domain.Subscription{}, "", false, err
	}
	if !found || !s.subscriptionMatchesMessengerUserID(ctx, latest, messengerUserID) {
		if requested.Status != domain.SubscriptionStatusActive {
			return domain.Subscription{}, cancelResolutionStaleSubmit, false, nil
		}
		return domain.Subscription{}, cancelResolutionAlreadyOff, false, nil
	}
	if latest.Status != domain.SubscriptionStatusActive || !latest.AutoPayEnabled {
		if requested.Status != domain.SubscriptionStatusActive || latest.ID != requested.ID {
			return domain.Subscription{}, cancelResolutionStaleSubmit, false, nil
		}
		return domain.Subscription{}, cancelResolutionAlreadyOff, false, nil
	}
	return latest, cancelResolutionStaleSubmit, true, nil
}

const (
	cancelResolutionExact       = "exact"
	cancelResolutionAlreadyOff  = "already_off"
	cancelResolutionStaleSubmit = "stale_submit"
)

func AttemptOrdinal(now, endsAt time.Time) int {
	return attemptOrdinalForPeriod(now, endsAt)
}

func attemptOrdinalForConnector(now, endsAt time.Time, connector domain.Connector) int {
	if timing := periodpolicy.Resolve(connector); timing.CustomRebillTiming {
		return timing.RebillAttemptOrdinal(now, endsAt)
	}
	return attemptOrdinalForPeriod(now, endsAt)
}

func attemptOrdinalForPeriod(now, endsAt time.Time) int {
	remaining := endsAt.Sub(now)
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

func (s *Service) resolveRecurringCancelUserIdentity(ctx context.Context, subs []domain.Subscription, messengerUserID int64) (string, string, domain.UserMessengerAccount) {
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
					if account.MessengerUserID == strconv.FormatInt(messengerUserID, 10) {
						return firstNonEmpty(strings.TrimSpace(user.FullName), formatRecurringCancelAccountLabel(account)), formatRecurringCancelAccountLabel(account), account
					}
				}
				for _, account := range accounts {
					if account.MessengerKind == domain.MessengerKindTelegram {
						return firstNonEmpty(strings.TrimSpace(user.FullName), formatRecurringCancelAccountLabel(account)), formatRecurringCancelAccountLabel(account), account
					}
				}
			}
			return strings.TrimSpace(user.FullName), "", domain.UserMessengerAccount{}
		}
	}
	user, found, err := s.ResolveUserByMessengerUserID(ctx, messengerUserID)
	if err != nil {
		slog.Error("load messenger user bridge for public cancel page failed", "error", err, "messenger_user_id", messengerUserID)
		return "", "", domain.UserMessengerAccount{}
	}
	if found {
		if accounts, err := s.Store.ListUserMessengerAccounts(ctx, user.ID); err == nil {
			for _, account := range accounts {
				if account.MessengerUserID == strconv.FormatInt(messengerUserID, 10) {
					return firstNonEmpty(strings.TrimSpace(user.FullName), formatRecurringCancelAccountLabel(account)), formatRecurringCancelAccountLabel(account), account
				}
			}
		}
		return strings.TrimSpace(user.FullName), "", domain.UserMessengerAccount{}
	}
	return "", "", domain.UserMessengerAccount{}
}

func (s *Service) resolveRecurringCancelReturn(account domain.UserMessengerAccount) (string, string) {
	switch account.MessengerKind {
	case domain.MessengerKindMAX:
		if strings.TrimSpace(s.MAXBotChatURL) != "" {
			return s.MAXBotChatURL, s.RecurringCancelOpenMAXLabel
		}
	case domain.MessengerKindTelegram:
		if strings.TrimSpace(s.TelegramBotChatURL) != "" {
			return s.TelegramBotChatURL, s.RecurringCancelOpenTelegramLabel
		}
	}
	return "", ""
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

func formatRecurringCancelAccountLabel(account domain.UserMessengerAccount) string {
	username := strings.TrimSpace(strings.TrimPrefix(account.Username, "@"))
	switch {
	case username != "":
		return "@" + username
	case strings.TrimSpace(account.MessengerUserID) != "":
		return strings.TrimSpace(account.MessengerUserID)
	default:
		return ""
	}
}
