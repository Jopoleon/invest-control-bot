package payments

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
	"github.com/Jopoleon/invest-control-bot/internal/max"
	"github.com/Jopoleon/invest-control-bot/internal/messenger"
	"github.com/Jopoleon/invest-control-bot/internal/store/memory"
)

func TestNotifyFailedRecurringPayment_SkipsWhenAutopayDisabled(t *testing.T) {
	st := memory.New()
	sent := 0
	service := &Service{
		Store:                 st,
		TelegramBotUsername:   "test_bot",
		FailedRecurringText:   "fail",
		FailedRecurringButton: "pay",
		SendUserNotification:  func(context.Context, int64, string, messenger.OutgoingMessage) error { sent++; return nil },
		BuildTargetAuditEvent: func(context.Context, int64, string, int64, string, string, time.Time) domain.AuditEvent {
			return domain.AuditEvent{}
		},
	}

	service.NotifyFailedRecurringPayment(context.Background(), domain.Payment{ID: 1, UserID: 10, ConnectorID: 1, SubscriptionID: 2, AutoPayEnabled: false})
	if sent != 0 {
		t.Fatalf("sent=%d want=0", sent)
	}
}

func TestActivateSuccessfulPayment_ExtendsFromCurrentSubscriptionEnd(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	now := time.Date(2026, 3, 31, 12, 0, 0, 0, time.UTC)
	connector := domain.Connector{ID: 1, StartPayload: "in-payments-pkg", Name: "tariff", PriceRUB: 1000, PeriodMode: domain.ConnectorPeriodModeDuration, PeriodSeconds: 30 * 24 * 60 * 60, IsActive: true, CreatedAt: now}
	if err := st.CreateConnector(ctx, connector); err != nil {
		t.Fatalf("CreateConnector err=%v", err)
	}
	paymentRow := domain.Payment{ID: 1, Provider: "robokassa", Status: domain.PaymentStatusPending, Token: "p1", UserID: 42, ConnectorID: connector.ID, AmountRUB: 1000, CreatedAt: now, UpdatedAt: now}
	if err := st.CreatePayment(ctx, paymentRow); err != nil {
		t.Fatalf("CreatePayment err=%v", err)
	}
	if err := st.UpsertSubscriptionByPayment(ctx, domain.Subscription{UserID: 42, ConnectorID: connector.ID, PaymentID: 999, Status: domain.SubscriptionStatusActive, StartsAt: now.Add(-24 * time.Hour), EndsAt: now.Add(10 * 24 * time.Hour), CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("UpsertSubscriptionByPayment err=%v", err)
	}

	service := &Service{
		Store:                 st,
		PaymentSuccessMessage: func(domain.Payment, domain.Connector, time.Time) string { return "ok" },
		SendUserNotification:  func(context.Context, int64, string, messenger.OutgoingMessage) error { return nil },
		BuildTargetAuditEvent: func(context.Context, int64, string, int64, string, string, time.Time) domain.AuditEvent {
			return domain.AuditEvent{}
		},
		SuccessChannelHint:     "",
		OpenChannelActionLabel: "open",
		MySubscriptionAction:   "sub",
	}

	service.ActivateSuccessfulPayment(ctx, paymentRow, "robokassa:p1", now)

	sub, found, err := st.GetLatestSubscriptionByUserConnector(ctx, 42, connector.ID)
	if err != nil || !found {
		t.Fatalf("GetLatestSubscriptionByUserConnector found=%v err=%v", found, err)
	}
	wantStart := now.Add(10 * 24 * time.Hour)
	if !sub.StartsAt.Equal(wantStart) {
		t.Fatalf("starts_at=%s want=%s", sub.StartsAt, wantStart)
	}
}

func TestActivateSuccessfulPayment_UsesTelegramAccessLinkWhenAvailable(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	now := time.Date(2026, 4, 2, 10, 0, 0, 0, time.UTC)
	connector := domain.Connector{
		ID:            1,
		StartPayload:  "in-payments-link",
		Name:          "private tg",
		PriceRUB:      1000,
		ChatID:        "1001234567890",
		ChannelURL:    "https://t.me/public_fallback",
		PeriodMode:    domain.ConnectorPeriodModeDuration,
		PeriodSeconds: 30 * 24 * 60 * 60,
		IsActive:      true,
		CreatedAt:     now,
	}
	if err := st.CreateConnector(ctx, connector); err != nil {
		t.Fatalf("CreateConnector err=%v", err)
	}
	paymentRow := domain.Payment{
		ID:          1,
		Provider:    "robokassa",
		Status:      domain.PaymentStatusPending,
		Token:       "p-access",
		UserID:      42,
		ConnectorID: connector.ID,
		AmountRUB:   1000,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := st.CreatePayment(ctx, paymentRow); err != nil {
		t.Fatalf("CreatePayment err=%v", err)
	}

	var sent messenger.OutgoingMessage
	var linkUserID int64
	var linkConnector domain.Connector
	service := &Service{
		Store:                 st,
		PaymentSuccessMessage: func(domain.Payment, domain.Connector, time.Time) string { return "ok" },
		BuildTelegramAccessLink: func(_ context.Context, userID int64, accessConnector domain.Connector) (string, error) {
			linkUserID = userID
			linkConnector = accessConnector
			return "https://t.me/+private_one_time_link", nil
		},
		SendUserNotification: func(_ context.Context, _ int64, _ string, msg messenger.OutgoingMessage) error {
			sent = msg
			return nil
		},
		BuildTargetAuditEvent: func(context.Context, int64, string, int64, string, string, time.Time) domain.AuditEvent {
			return domain.AuditEvent{}
		},
		SuccessChannelHint:     "",
		OpenChannelActionLabel: "open",
		MySubscriptionAction:   "sub",
	}

	service.ActivateSuccessfulPayment(ctx, paymentRow, "robokassa:p-access", now)

	if len(sent.Buttons) == 0 || len(sent.Buttons[0]) == 0 {
		t.Fatalf("buttons are empty")
	}
	if linkUserID != paymentRow.UserID {
		t.Fatalf("link user id=%d want=%d", linkUserID, paymentRow.UserID)
	}
	if linkConnector.ID != connector.ID || linkConnector.ChatID != connector.ChatID {
		t.Fatalf("connector passed to access link builder = %+v want id=%d chat_id=%q", linkConnector, connector.ID, connector.ChatID)
	}
	if got := sent.Buttons[0][0].URL; got != "https://t.me/+private_one_time_link" {
		t.Fatalf("access link url=%q want telegram invite link", got)
	}
}

func TestActivateSuccessfulPayment_WritesAccessDeliveryFailedAuditWhenDestinationMissing(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	now := time.Date(2026, 4, 2, 10, 5, 0, 0, time.UTC)
	connector := domain.Connector{
		ID:            1,
		StartPayload:  "in-payments-missing-access",
		Name:          "private tg",
		PriceRUB:      1000,
		PeriodMode:    domain.ConnectorPeriodModeDuration,
		PeriodSeconds: 30 * 24 * 60 * 60,
		IsActive:      true,
		CreatedAt:     now,
	}
	if err := st.CreateConnector(ctx, connector); err != nil {
		t.Fatalf("CreateConnector err=%v", err)
	}
	paymentRow := domain.Payment{
		ID:          1,
		Provider:    "robokassa",
		Status:      domain.PaymentStatusPending,
		Token:       "p-no-access",
		UserID:      42,
		ConnectorID: connector.ID,
		AmountRUB:   1000,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := st.CreatePayment(ctx, paymentRow); err != nil {
		t.Fatalf("CreatePayment err=%v", err)
	}
	createdPayment, found, err := st.GetPaymentByToken(ctx, paymentRow.Token)
	if err != nil || !found {
		t.Fatalf("GetPaymentByToken found=%v err=%v", found, err)
	}
	paymentRow = createdPayment

	service := &Service{
		Store:                 st,
		PaymentSuccessMessage: func(domain.Payment, domain.Connector, time.Time) string { return "ok" },
		SendUserNotification:  func(context.Context, int64, string, messenger.OutgoingMessage) error { return nil },
		BuildTargetAuditEvent: func(_ context.Context, userID int64, _ string, connectorID int64, action, details string, createdAt time.Time) domain.AuditEvent {
			return domain.AuditEvent{TargetUserID: userID, ConnectorID: connectorID, Action: action, Details: details, CreatedAt: createdAt}
		},
		OpenChannelActionLabel: "open",
		MySubscriptionAction:   "sub",
	}

	service.ActivateSuccessfulPayment(ctx, paymentRow, "robokassa:p-no-access", now)

	events, _, err := st.ListAuditEvents(ctx, domain.AuditEventListQuery{Page: 1, PageSize: 50})
	if err != nil {
		t.Fatalf("ListAuditEvents err=%v", err)
	}
	if got := countPaymentAuditEvents(events, domain.AuditActionAccessDeliveryFailed); got != 1 {
		t.Fatalf("access_delivery_failed count=%d want=1", got)
	}
	if !strings.Contains(findPaymentAuditDetails(events, domain.AuditActionAccessDeliveryFailed), "reason=missing_access_destination") {
		t.Fatalf("access_delivery_failed details=%q want missing_access_destination", findPaymentAuditDetails(events, domain.AuditActionAccessDeliveryFailed))
	}
}

func TestActivateSuccessfulPayment_WritesPaymentAccessReadyAuditWhenDeliverySucceeds(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	now := time.Date(2026, 4, 2, 10, 10, 0, 0, time.UTC)
	connector := domain.Connector{
		ID:            1,
		StartPayload:  "in-payments-access-ready",
		Name:          "private tg",
		PriceRUB:      1000,
		ChannelURL:    "https://t.me/private_channel",
		PeriodMode:    domain.ConnectorPeriodModeDuration,
		PeriodSeconds: 30 * 24 * 60 * 60,
		IsActive:      true,
		CreatedAt:     now,
	}
	if err := st.CreateConnector(ctx, connector); err != nil {
		t.Fatalf("CreateConnector err=%v", err)
	}
	paymentRow := domain.Payment{
		ID:          1,
		Provider:    "robokassa",
		Status:      domain.PaymentStatusPending,
		Token:       "p-access-ready",
		UserID:      42,
		ConnectorID: connector.ID,
		AmountRUB:   1000,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := st.CreatePayment(ctx, paymentRow); err != nil {
		t.Fatalf("CreatePayment err=%v", err)
	}
	createdPayment, found, err := st.GetPaymentByToken(ctx, paymentRow.Token)
	if err != nil || !found {
		t.Fatalf("GetPaymentByToken found=%v err=%v", found, err)
	}
	paymentRow = createdPayment

	service := &Service{
		Store:                 st,
		PaymentSuccessMessage: func(domain.Payment, domain.Connector, time.Time) string { return "ok" },
		SendUserNotification:  func(context.Context, int64, string, messenger.OutgoingMessage) error { return nil },
		BuildTargetAuditEvent: func(_ context.Context, userID int64, _ string, connectorID int64, action, details string, createdAt time.Time) domain.AuditEvent {
			return domain.AuditEvent{TargetUserID: userID, ConnectorID: connectorID, Action: action, Details: details, CreatedAt: createdAt}
		},
		OpenChannelActionLabel: "open",
		MySubscriptionAction:   "sub",
	}

	service.ActivateSuccessfulPayment(ctx, paymentRow, "robokassa:p-access-ready", now)

	events, _, err := st.ListAuditEvents(ctx, domain.AuditEventListQuery{Page: 1, PageSize: 50})
	if err != nil {
		t.Fatalf("ListAuditEvents err=%v", err)
	}
	if got := countPaymentAuditEvents(events, domain.AuditActionPaymentAccessReady); got != 1 {
		t.Fatalf("payment_access_ready count=%d want=1", got)
	}
	if !strings.Contains(findPaymentAuditDetails(events, domain.AuditActionPaymentAccessReady), "source=telegram_channel_url") {
		t.Fatalf("payment_access_ready details=%q want telegram_channel_url", findPaymentAuditDetails(events, domain.AuditActionPaymentAccessReady))
	}
}

func TestActivateSuccessfulPayment_AddsMAXMemberAndKeepsChannelButton(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	now := time.Date(2026, 4, 4, 12, 0, 0, 0, time.UTC)
	connector := domain.Connector{
		ID:            1,
		StartPayload:  "in-max-access-ready",
		Name:          "private max",
		PriceRUB:      1000,
		MAXChatID:     "-72598909498032",
		MAXChannelURL: "https://web.max.ru/-72598909498032",
		PeriodMode:    domain.ConnectorPeriodModeDuration,
		PeriodSeconds: 30 * 24 * 60 * 60,
		IsActive:      true,
		CreatedAt:     now,
	}
	if err := st.CreateConnector(ctx, connector); err != nil {
		t.Fatalf("CreateConnector err=%v", err)
	}
	maxUser, _, err := st.GetOrCreateUserByMessenger(ctx, domain.MessengerKindMAX, "193465776", "fedor")
	if err != nil {
		t.Fatalf("GetOrCreateUserByMessenger err=%v", err)
	}
	paymentRow := domain.Payment{
		ID:          1,
		Provider:    "robokassa",
		Status:      domain.PaymentStatusPending,
		Token:       "p-max-access-ready",
		UserID:      maxUser.ID,
		ConnectorID: connector.ID,
		AmountRUB:   1000,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := st.CreatePayment(ctx, paymentRow); err != nil {
		t.Fatalf("CreatePayment err=%v", err)
	}

	var addedChatID int64
	var addedUserIDs []int64
	var sent messenger.OutgoingMessage
	service := &Service{
		Store:                 st,
		PaymentSuccessMessage: func(domain.Payment, domain.Connector, time.Time) string { return "ok" },
		ResolveMAXAccount: func(context.Context, int64) (domain.UserMessengerAccount, bool, error) {
			return domain.UserMessengerAccount{MessengerKind: domain.MessengerKindMAX, MessengerUserID: "193465776"}, true, nil
		},
		AddMAXChatMembers: func(_ context.Context, chatID int64, userIDs []int64) error {
			addedChatID = chatID
			addedUserIDs = append([]int64(nil), userIDs...)
			return nil
		},
		ResolvePreferredKind: func(context.Context, int64, string) messenger.Kind { return messenger.KindMAX },
		SendUserNotification: func(_ context.Context, _ int64, _ string, msg messenger.OutgoingMessage) error {
			sent = msg
			return nil
		},
		BuildTargetAuditEvent: func(_ context.Context, userID int64, _ string, connectorID int64, action, details string, createdAt time.Time) domain.AuditEvent {
			return domain.AuditEvent{TargetUserID: userID, ConnectorID: connectorID, Action: action, Details: details, CreatedAt: createdAt}
		},
		OpenChannelActionLabel: "open",
		MySubscriptionAction:   "sub",
	}

	service.ActivateSuccessfulPayment(ctx, paymentRow, "robokassa:p-max-access-ready", now)

	if addedChatID != -72598909498032 {
		t.Fatalf("added chat id=%d want=-72598909498032", addedChatID)
	}
	if len(addedUserIDs) != 1 || addedUserIDs[0] != 193465776 {
		t.Fatalf("added user ids=%v want [193465776]", addedUserIDs)
	}
	if len(sent.Buttons) == 0 || len(sent.Buttons[0]) == 0 || sent.Buttons[0][0].URL != "https://web.max.ru/-72598909498032" {
		t.Fatalf("sent buttons=%+v want MAX channel URL button", sent.Buttons)
	}

	events, _, err := st.ListAuditEvents(ctx, domain.AuditEventListQuery{Page: 1, PageSize: 50})
	if err != nil {
		t.Fatalf("ListAuditEvents err=%v", err)
	}
	if details := findPaymentAuditDetails(events, domain.AuditActionPaymentAccessReady); !strings.Contains(details, "source=max_chat_member_added") {
		t.Fatalf("payment_access_ready details=%q want source=max_chat_member_added", details)
	}
}

func TestActivateSuccessfulPayment_DualDestinationMAXFlowDoesNotBuildTelegramInvite(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	now := time.Date(2026, 4, 4, 12, 0, 0, 0, time.UTC)
	connector := domain.Connector{
		ID:            1,
		StartPayload:  "in-max-dual-access",
		Name:          "dual access",
		ChatID:        "1001234567890",
		ChannelURL:    "https://t.me/private_tg_channel",
		MAXChatID:     "-72598909498032",
		MAXChannelURL: "https://web.max.ru/-72598909498032",
		PriceRUB:      1000,
		PeriodMode:    domain.ConnectorPeriodModeDuration,
		PeriodSeconds: 30 * 24 * 60 * 60,
		IsActive:      true,
		CreatedAt:     now,
	}
	if err := st.CreateConnector(ctx, connector); err != nil {
		t.Fatalf("CreateConnector err=%v", err)
	}
	maxUser, _, err := st.GetOrCreateUserByMessenger(ctx, domain.MessengerKindMAX, "193465776", "fedor")
	if err != nil {
		t.Fatalf("GetOrCreateUserByMessenger err=%v", err)
	}
	paymentRow := domain.Payment{
		ID:          1,
		Provider:    "robokassa",
		Status:      domain.PaymentStatusPending,
		Token:       "p-max-dual-access",
		UserID:      maxUser.ID,
		ConnectorID: connector.ID,
		AmountRUB:   1000,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := st.CreatePayment(ctx, paymentRow); err != nil {
		t.Fatalf("CreatePayment err=%v", err)
	}

	telegramInviteCalls := 0
	var sent messenger.OutgoingMessage
	service := &Service{
		Store:                 st,
		PaymentSuccessMessage: func(domain.Payment, domain.Connector, time.Time) string { return "ok" },
		BuildTelegramAccessLink: func(context.Context, int64, domain.Connector) (string, error) {
			telegramInviteCalls++
			return "https://t.me/+invite-link", nil
		},
		ResolveMAXAccount: func(context.Context, int64) (domain.UserMessengerAccount, bool, error) {
			return domain.UserMessengerAccount{MessengerKind: domain.MessengerKindMAX, MessengerUserID: "193465776"}, true, nil
		},
		AddMAXChatMembers: func(_ context.Context, _ int64, _ []int64) error { return nil },
		ResolvePreferredKind: func(context.Context, int64, string) messenger.Kind {
			return messenger.KindMAX
		},
		SendUserNotification: func(_ context.Context, _ int64, _ string, msg messenger.OutgoingMessage) error {
			sent = msg
			return nil
		},
		BuildTargetAuditEvent: func(_ context.Context, userID int64, _ string, connectorID int64, action, details string, createdAt time.Time) domain.AuditEvent {
			return domain.AuditEvent{TargetUserID: userID, ConnectorID: connectorID, Action: action, Details: details, CreatedAt: createdAt}
		},
		OpenChannelActionLabel: "open",
		MySubscriptionAction:   "sub",
	}

	service.ActivateSuccessfulPayment(ctx, paymentRow, "robokassa:p-max-dual-access", now)

	if telegramInviteCalls != 0 {
		t.Fatalf("telegram invite calls=%d want 0 for MAX flow", telegramInviteCalls)
	}
	if len(sent.Buttons) == 0 || len(sent.Buttons[0]) == 0 || sent.Buttons[0][0].URL != "https://web.max.ru/-72598909498032" {
		t.Fatalf("sent buttons=%+v want MAX channel URL button", sent.Buttons)
	}
}

func TestActivateSuccessfulPayment_MAXMissingChatIDAuditsFailureAndFallsBackToURL(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	now := time.Date(2026, 4, 4, 12, 0, 0, 0, time.UTC)
	connector := domain.Connector{
		ID:            1,
		StartPayload:  "in-max-missing-chat-id",
		Name:          "max access",
		MAXChannelURL: "https://web.max.ru/channel-without-id",
		PriceRUB:      1000,
		PeriodMode:    domain.ConnectorPeriodModeDuration,
		PeriodSeconds: 30 * 24 * 60 * 60,
		IsActive:      true,
		CreatedAt:     now,
	}
	if err := st.CreateConnector(ctx, connector); err != nil {
		t.Fatalf("CreateConnector err=%v", err)
	}
	maxUser, _, err := st.GetOrCreateUserByMessenger(ctx, domain.MessengerKindMAX, "193465776", "fedor")
	if err != nil {
		t.Fatalf("GetOrCreateUserByMessenger err=%v", err)
	}
	paymentRow := domain.Payment{
		ID:          1,
		Provider:    "robokassa",
		Status:      domain.PaymentStatusPending,
		Token:       "p-max-missing-chat-id",
		UserID:      maxUser.ID,
		ConnectorID: connector.ID,
		AmountRUB:   1000,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := st.CreatePayment(ctx, paymentRow); err != nil {
		t.Fatalf("CreatePayment err=%v", err)
	}
	createdPayment, found, err := st.GetPaymentByToken(ctx, paymentRow.Token)
	if err != nil || !found {
		t.Fatalf("GetPaymentByToken found=%v err=%v", found, err)
	}
	paymentRow = createdPayment

	service := &Service{
		Store:                 st,
		PaymentSuccessMessage: func(domain.Payment, domain.Connector, time.Time) string { return "ok" },
		ResolvePreferredKind:  func(context.Context, int64, string) messenger.Kind { return messenger.KindMAX },
		SendUserNotification:  func(context.Context, int64, string, messenger.OutgoingMessage) error { return nil },
		BuildTargetAuditEvent: func(_ context.Context, userID int64, _ string, connectorID int64, action, details string, createdAt time.Time) domain.AuditEvent {
			return domain.AuditEvent{TargetUserID: userID, ConnectorID: connectorID, Action: action, Details: details, CreatedAt: createdAt}
		},
		OpenChannelActionLabel: "open",
		MySubscriptionAction:   "sub",
	}

	service.ActivateSuccessfulPayment(ctx, paymentRow, "robokassa:p-max-missing-chat-id", now)

	events, _, err := st.ListAuditEvents(ctx, domain.AuditEventListQuery{Page: 1, PageSize: 50})
	if err != nil {
		t.Fatalf("ListAuditEvents err=%v", err)
	}
	if !strings.Contains(findPaymentAuditDetails(events, domain.AuditActionPaymentAccessReady), "source=max_channel_url") {
		t.Fatalf("payment_access_ready details=%q want source=max_channel_url", findPaymentAuditDetails(events, domain.AuditActionPaymentAccessReady))
	}
	if !strings.Contains(findPaymentAuditDetails(events, domain.AuditActionAccessDeliveryFailed), "reason=missing_max_chat_id") {
		t.Fatalf("access_delivery_failed details=%q want missing_max_chat_id", findPaymentAuditDetails(events, domain.AuditActionAccessDeliveryFailed))
	}
}

func TestActivateSuccessfulPayment_MAXClientMissingAuditsFailureAndFallsBackToURL(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	now := time.Date(2026, 4, 4, 12, 0, 0, 0, time.UTC)
	connector := domain.Connector{
		ID:            1,
		StartPayload:  "in-max-client-missing",
		Name:          "max access",
		MAXChatID:     "-72598909498032",
		MAXChannelURL: "https://web.max.ru/-72598909498032",
		PriceRUB:      1000,
		PeriodMode:    domain.ConnectorPeriodModeDuration,
		PeriodSeconds: 30 * 24 * 60 * 60,
		IsActive:      true,
		CreatedAt:     now,
	}
	if err := st.CreateConnector(ctx, connector); err != nil {
		t.Fatalf("CreateConnector err=%v", err)
	}
	maxUser, _, err := st.GetOrCreateUserByMessenger(ctx, domain.MessengerKindMAX, "193465776", "fedor")
	if err != nil {
		t.Fatalf("GetOrCreateUserByMessenger err=%v", err)
	}
	paymentRow := domain.Payment{
		ID:          1,
		Provider:    "robokassa",
		Status:      domain.PaymentStatusPending,
		Token:       "p-max-client-missing",
		UserID:      maxUser.ID,
		ConnectorID: connector.ID,
		AmountRUB:   1000,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := st.CreatePayment(ctx, paymentRow); err != nil {
		t.Fatalf("CreatePayment err=%v", err)
	}
	createdPayment, found, err := st.GetPaymentByToken(ctx, paymentRow.Token)
	if err != nil || !found {
		t.Fatalf("GetPaymentByToken found=%v err=%v", found, err)
	}
	paymentRow = createdPayment

	service := &Service{
		Store:                 st,
		PaymentSuccessMessage: func(domain.Payment, domain.Connector, time.Time) string { return "ok" },
		ResolvePreferredKind:  func(context.Context, int64, string) messenger.Kind { return messenger.KindMAX },
		SendUserNotification:  func(context.Context, int64, string, messenger.OutgoingMessage) error { return nil },
		BuildTargetAuditEvent: func(_ context.Context, userID int64, _ string, connectorID int64, action, details string, createdAt time.Time) domain.AuditEvent {
			return domain.AuditEvent{TargetUserID: userID, ConnectorID: connectorID, Action: action, Details: details, CreatedAt: createdAt}
		},
		OpenChannelActionLabel: "open",
		MySubscriptionAction:   "sub",
	}

	service.ActivateSuccessfulPayment(ctx, paymentRow, "robokassa:p-max-client-missing", now)

	events, _, err := st.ListAuditEvents(ctx, domain.AuditEventListQuery{Page: 1, PageSize: 50})
	if err != nil {
		t.Fatalf("ListAuditEvents err=%v", err)
	}
	if !strings.Contains(findPaymentAuditDetails(events, domain.AuditActionPaymentAccessReady), "source=max_channel_url") {
		t.Fatalf("payment_access_ready details=%q want source=max_channel_url", findPaymentAuditDetails(events, domain.AuditActionPaymentAccessReady))
	}
	if !strings.Contains(findPaymentAuditDetails(events, domain.AuditActionAccessDeliveryFailed), "reason=max_client_not_configured") {
		t.Fatalf("access_delivery_failed details=%q want max_client_not_configured", findPaymentAuditDetails(events, domain.AuditActionAccessDeliveryFailed))
	}
}

func TestActivateSuccessfulPayment_MAXAddMemberFailureAuditsVerboseDetailsAndFallsBackToURL(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	now := time.Date(2026, 4, 5, 2, 26, 53, 0, time.UTC)
	connector := domain.Connector{
		ID:            1,
		StartPayload:  "in-max-add-member-failed",
		Name:          "max access",
		MAXChatID:     "-72598909498032",
		MAXChannelURL: "https://web.max.ru/-72598909498032",
		PriceRUB:      1000,
		PeriodMode:    domain.ConnectorPeriodModeDuration,
		PeriodSeconds: 6 * 60 * 60,
		IsActive:      true,
		CreatedAt:     now,
	}
	if err := st.CreateConnector(ctx, connector); err != nil {
		t.Fatalf("CreateConnector err=%v", err)
	}
	maxUser, _, err := st.GetOrCreateUserByMessenger(ctx, domain.MessengerKindMAX, "193465776", "fedor")
	if err != nil {
		t.Fatalf("GetOrCreateUserByMessenger err=%v", err)
	}
	paymentRow := domain.Payment{
		ID:          56,
		Provider:    "robokassa",
		Status:      domain.PaymentStatusPending,
		Token:       "p-max-add-member-failed",
		UserID:      maxUser.ID,
		ConnectorID: connector.ID,
		AmountRUB:   1000,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := st.CreatePayment(ctx, paymentRow); err != nil {
		t.Fatalf("CreatePayment err=%v", err)
	}
	createdPayment, found, err := st.GetPaymentByToken(ctx, "p-max-add-member-failed")
	if err != nil || !found {
		t.Fatalf("GetPaymentByToken found=%v err=%v", found, err)
	}
	paymentRow = createdPayment

	service := &Service{
		Store:                 st,
		PaymentSuccessMessage: func(domain.Payment, domain.Connector, time.Time) string { return "ok" },
		ResolveMAXAccount: func(context.Context, int64) (domain.UserMessengerAccount, bool, error) {
			return domain.UserMessengerAccount{MessengerKind: domain.MessengerKindMAX, MessengerUserID: "193465776"}, true, nil
		},
		AddMAXChatMembers: func(context.Context, int64, []int64) error {
			return &max.MutationError{
				Operation:     "add chat members returned success=false",
				Message:       "cannot add member",
				FailedUserIDs: []int64{193465776},
				FailedUserDetails: []max.FailedUserDetail{{
					UserID:  193465776,
					Code:    "already_member",
					Message: "user already in chat",
				}},
			}
		},
		ResolvePreferredKind: func(context.Context, int64, string) messenger.Kind { return messenger.KindMAX },
		SendUserNotification: func(context.Context, int64, string, messenger.OutgoingMessage) error { return nil },
		BuildTargetAuditEvent: func(_ context.Context, userID int64, _ string, connectorID int64, action, details string, createdAt time.Time) domain.AuditEvent {
			return domain.AuditEvent{TargetUserID: userID, ConnectorID: connectorID, Action: action, Details: details, CreatedAt: createdAt}
		},
		OpenChannelActionLabel: "open",
		MySubscriptionAction:   "sub",
	}

	service.ActivateSuccessfulPayment(ctx, paymentRow, "robokassa:p-max-add-member-failed", now)

	events, _, err := st.ListAuditEvents(ctx, domain.AuditEventListQuery{Page: 1, PageSize: 50})
	if err != nil {
		t.Fatalf("ListAuditEvents err=%v", err)
	}
	readyDetails := findPaymentAuditDetails(events, domain.AuditActionPaymentAccessReady)
	if !strings.Contains(readyDetails, "source=max_channel_url") {
		t.Fatalf("payment_access_ready details=%q want source=max_channel_url", readyDetails)
	}
	failureDetails := findPaymentAuditDetails(events, domain.AuditActionAccessDeliveryFailed)
	if !strings.Contains(failureDetails, "reason=max_add_member_failed") {
		t.Fatalf("access_delivery_failed details=%q want reason=max_add_member_failed", failureDetails)
	}
	if !strings.Contains(failureDetails, "max_message=cannot add member") {
		t.Fatalf("access_delivery_failed details=%q want max_message", failureDetails)
	}
	if !strings.Contains(failureDetails, "failed_user_ids=193465776") {
		t.Fatalf("access_delivery_failed details=%q want failed_user_ids", failureDetails)
	}
	if !strings.Contains(failureDetails, "failed_user_details={user_id:193465776, code:already_member, message:user already in chat}") {
		t.Fatalf("access_delivery_failed details=%q want failed_user_details", failureDetails)
	}
}

func TestActivateSuccessfulPayment_DuplicatePaidCallbackDoesNotShiftExistingSubscription(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	now := time.Date(2026, 4, 4, 7, 43, 46, 0, time.UTC)
	connector := domain.Connector{
		ID:            1,
		StartPayload:  "in-duplicate-paid",
		Name:          "3h recurring",
		PriceRUB:      100,
		PeriodMode:    domain.ConnectorPeriodModeDuration,
		PeriodSeconds: int64((3 * 60 * 60)),
		IsActive:      true,
		CreatedAt:     now,
	}
	if err := st.CreateConnector(ctx, connector); err != nil {
		t.Fatalf("CreateConnector err=%v", err)
	}

	paidAt := now
	paymentRow := domain.Payment{
		ID:                41,
		Provider:          "robokassa",
		ProviderPaymentID: "robokassa:41",
		Status:            domain.PaymentStatusPaid,
		Token:             "paid-41",
		UserID:            42,
		ConnectorID:       connector.ID,
		AmountRUB:         100,
		CreatedAt:         now.Add(-10 * time.Second),
		PaidAt:            &paidAt,
		UpdatedAt:         now,
	}
	if err := st.CreatePayment(ctx, paymentRow); err != nil {
		t.Fatalf("CreatePayment err=%v", err)
	}

	originalStart := now
	originalEnd := now.Add(3 * time.Hour)
	if err := st.UpsertSubscriptionByPayment(ctx, domain.Subscription{
		UserID:         paymentRow.UserID,
		ConnectorID:    connector.ID,
		PaymentID:      paymentRow.ID,
		Status:         domain.SubscriptionStatusActive,
		AutoPayEnabled: true,
		StartsAt:       originalStart,
		EndsAt:         originalEnd,
		CreatedAt:      originalStart,
		UpdatedAt:      now,
	}); err != nil {
		t.Fatalf("UpsertSubscriptionByPayment err=%v", err)
	}

	service := &Service{
		Store:                 st,
		PaymentSuccessMessage: func(domain.Payment, domain.Connector, time.Time) string { return "ok" },
		SendUserNotification:  func(context.Context, int64, string, messenger.OutgoingMessage) error { return nil },
		BuildTargetAuditEvent: func(_ context.Context, userID int64, _ string, connectorID int64, action, details string, createdAt time.Time) domain.AuditEvent {
			return domain.AuditEvent{TargetUserID: userID, ConnectorID: connectorID, Action: action, Details: details, CreatedAt: createdAt}
		},
		OpenChannelActionLabel: "open",
		MySubscriptionAction:   "sub",
	}

	service.ActivateSuccessfulPayment(ctx, paymentRow, "robokassa:41", now.Add(time.Minute))

	sub, found, err := st.GetLatestSubscriptionByUserConnector(ctx, paymentRow.UserID, connector.ID)
	if err != nil || !found {
		t.Fatalf("GetLatestSubscriptionByUserConnector found=%v err=%v", found, err)
	}
	if !sub.StartsAt.Equal(originalStart) {
		t.Fatalf("starts_at=%s want=%s", sub.StartsAt, originalStart)
	}
	if !sub.EndsAt.Equal(originalEnd) {
		t.Fatalf("ends_at=%s want=%s", sub.EndsAt, originalEnd)
	}

	events, _, err := st.ListAuditEvents(ctx, domain.AuditEventListQuery{Page: 1, PageSize: 50})
	if err != nil {
		t.Fatalf("ListAuditEvents err=%v", err)
	}
	if got := countPaymentAuditEvents(events, domain.AuditActionSubscriptionActivated); got != 0 {
		t.Fatalf("subscription_activated count=%d want=0 on duplicate callback", got)
	}
	if got := countPaymentAuditEvents(events, domain.AuditActionPaymentSuccessNotified); got != 0 {
		t.Fatalf("payment_success_notified count=%d want=0 on duplicate callback", got)
	}
}

func TestActivateSuccessfulPayment_WritesInviteLinkFailureAuditWhenFallbackSucceeds(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	now := time.Date(2026, 4, 2, 10, 12, 0, 0, time.UTC)
	connector := domain.Connector{
		ID:            1,
		StartPayload:  "in-payments-invite-fallback",
		Name:          "private tg",
		PriceRUB:      1000,
		ChatID:        "1001234567890",
		ChannelURL:    "https://t.me/private_channel",
		PeriodMode:    domain.ConnectorPeriodModeDuration,
		PeriodSeconds: 30 * 24 * 60 * 60,
		IsActive:      true,
		CreatedAt:     now,
	}
	if err := st.CreateConnector(ctx, connector); err != nil {
		t.Fatalf("CreateConnector err=%v", err)
	}
	paymentRow := domain.Payment{
		ID:          1,
		Provider:    "robokassa",
		Status:      domain.PaymentStatusPending,
		Token:       "p-invite-fallback",
		UserID:      42,
		ConnectorID: connector.ID,
		AmountRUB:   1000,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := st.CreatePayment(ctx, paymentRow); err != nil {
		t.Fatalf("CreatePayment err=%v", err)
	}

	service := &Service{
		Store:                 st,
		PaymentSuccessMessage: func(domain.Payment, domain.Connector, time.Time) string { return "ok" },
		BuildTelegramAccessLink: func(context.Context, int64, domain.Connector) (string, error) {
			return "", errors.New("telegram api failed")
		},
		SendUserNotification: func(context.Context, int64, string, messenger.OutgoingMessage) error { return nil },
		BuildTargetAuditEvent: func(_ context.Context, userID int64, _ string, connectorID int64, action, details string, createdAt time.Time) domain.AuditEvent {
			return domain.AuditEvent{TargetUserID: userID, ConnectorID: connectorID, Action: action, Details: details, CreatedAt: createdAt}
		},
		OpenChannelActionLabel: "open",
		MySubscriptionAction:   "sub",
	}

	service.ActivateSuccessfulPayment(ctx, paymentRow, "robokassa:p-invite-fallback", now)

	events, _, err := st.ListAuditEvents(ctx, domain.AuditEventListQuery{Page: 1, PageSize: 50})
	if err != nil {
		t.Fatalf("ListAuditEvents err=%v", err)
	}
	if got := countPaymentAuditEvents(events, domain.AuditActionInviteLinkDeliveryFailed); got != 1 {
		t.Fatalf("invite_link_delivery_failed count=%d want=1", got)
	}
	if !strings.Contains(findPaymentAuditDetails(events, domain.AuditActionInviteLinkDeliveryFailed), "reason=invite_link_build_failed") {
		t.Fatalf("invite_link_delivery_failed details=%q want invite_link_build_failed", findPaymentAuditDetails(events, domain.AuditActionInviteLinkDeliveryFailed))
	}
	if got := countPaymentAuditEvents(events, domain.AuditActionPaymentAccessReady); got != 1 {
		t.Fatalf("payment_access_ready count=%d want=1", got)
	}
	if !strings.Contains(findPaymentAuditDetails(events, domain.AuditActionPaymentAccessReady), "source=telegram_channel_url") {
		t.Fatalf("payment_access_ready details=%q want telegram_channel_url", findPaymentAuditDetails(events, domain.AuditActionPaymentAccessReady))
	}
}

func TestActivateSuccessfulPayment_TelegramOnlyConnectorUsesTelegramAccessEvenWithMAXFallback(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	now := time.Date(2026, 4, 2, 10, 15, 0, 0, time.UTC)
	connector := domain.Connector{
		ID:            1,
		StartPayload:  "in-payments-max-incompatible",
		Name:          "telegram only",
		ChannelURL:    "https://t.me/private_channel",
		PriceRUB:      1000,
		PeriodMode:    domain.ConnectorPeriodModeDuration,
		PeriodSeconds: 30 * 24 * 60 * 60,
		IsActive:      true,
		CreatedAt:     now,
	}
	if err := st.CreateConnector(ctx, connector); err != nil {
		t.Fatalf("CreateConnector err=%v", err)
	}
	maxUser, _, err := st.GetOrCreateUserByMessenger(ctx, domain.MessengerKindMAX, "193465776", "fedor")
	if err != nil {
		t.Fatalf("GetOrCreateUserByMessenger err=%v", err)
	}
	paymentRow := domain.Payment{
		ID:          1,
		Provider:    "robokassa",
		Status:      domain.PaymentStatusPending,
		Token:       "p-max-incompatible",
		UserID:      maxUser.ID,
		ConnectorID: connector.ID,
		AmountRUB:   1000,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := st.CreatePayment(ctx, paymentRow); err != nil {
		t.Fatalf("CreatePayment err=%v", err)
	}

	var sent messenger.OutgoingMessage
	service := &Service{
		Store:                 st,
		PaymentSuccessMessage: func(domain.Payment, domain.Connector, time.Time) string { return "ok" },
		ResolvePreferredKind: func(context.Context, int64, string) messenger.Kind {
			return messenger.KindMAX
		},
		SendUserNotification: func(_ context.Context, _ int64, _ string, msg messenger.OutgoingMessage) error {
			sent = msg
			return nil
		},
		BuildTargetAuditEvent: func(_ context.Context, userID int64, _ string, connectorID int64, action, details string, createdAt time.Time) domain.AuditEvent {
			return domain.AuditEvent{TargetUserID: userID, ConnectorID: connectorID, Action: action, Details: details, CreatedAt: createdAt}
		},
		OpenChannelActionLabel: "open",
		MySubscriptionAction:   "sub",
	}

	service.ActivateSuccessfulPayment(ctx, paymentRow, "robokassa:p-max-incompatible", now)

	if len(sent.Buttons) == 0 || len(sent.Buttons[0]) == 0 || sent.Buttons[0][0].URL != "https://t.me/private_channel" {
		t.Fatalf("buttons=%+v want telegram access button", sent.Buttons)
	}
	events, _, err := st.ListAuditEvents(ctx, domain.AuditEventListQuery{Page: 1, PageSize: 50})
	if err != nil {
		t.Fatalf("ListAuditEvents err=%v", err)
	}
	if got := countPaymentAuditEvents(events, domain.AuditActionAccessDeliveryFailed); got != 0 {
		t.Fatalf("access_delivery_failed count=%d want 0", got)
	}
	if !strings.Contains(findPaymentAuditDetails(events, domain.AuditActionPaymentAccessReady), "source=telegram_channel_url") {
		t.Fatalf("payment_access_ready details=%q want telegram_channel_url", findPaymentAuditDetails(events, domain.AuditActionPaymentAccessReady))
	}
}

func countPaymentAuditEvents(events []domain.AuditEvent, action string) int {
	count := 0
	for _, event := range events {
		if event.Action == action {
			count++
		}
	}
	return count
}

func findPaymentAuditDetails(events []domain.AuditEvent, action string) string {
	for _, event := range events {
		if event.Action == action {
			return event.Details
		}
	}
	return ""
}
