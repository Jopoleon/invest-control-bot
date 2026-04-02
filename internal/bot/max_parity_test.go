package bot

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
	"github.com/Jopoleon/invest-control-bot/internal/messenger"
	"github.com/Jopoleon/invest-control-bot/internal/payment"
	"github.com/Jopoleon/invest-control-bot/internal/store/memory"
)

func TestHandleCallback_CreatesInternalUserAndMAXAccount(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	sender := &fakeSender{}
	h := NewHandler(st, sender, payment.NewMockService("http://localhost:8080"), false, "http://localhost:8080", "test-encryption-key-123456789012345")

	connectorID := seedBotConnector(t, ctx, st, "in-max-user-account")

	h.handleCallback(ctx, maxAction("cb-max-user-account", 193465776, "fedor", "accept_terms:"+int64ToString(connectorID)))

	user, found, err := st.GetUserByMessenger(ctx, domain.MessengerKindMAX, "193465776")
	if err != nil {
		t.Fatalf("get user by MAX messenger: %v", err)
	}
	if !found {
		t.Fatalf("user not found")
	}
	accounts, err := st.ListUserMessengerAccounts(ctx, user.ID)
	if err != nil {
		t.Fatalf("ListUserMessengerAccounts: %v", err)
	}
	if len(accounts) != 1 {
		t.Fatalf("accounts len = %d, want 1", len(accounts))
	}
	if accounts[0].MessengerKind != domain.MessengerKindMAX || accounts[0].MessengerUserID != "193465776" {
		t.Fatalf("unexpected messenger account: %+v", accounts[0])
	}
	if accounts[0].Username != "fedor" {
		t.Fatalf("username = %q, want fedor", accounts[0].Username)
	}
}

func TestHandleCallback_MAXMissingUsernameUsesGenericPrompt(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	sender := &fakeSender{}
	h := NewHandler(st, sender, payment.NewMockService("http://localhost:8080"), false, "http://localhost:8080", "test-encryption-key-123456789012345")

	connectorID := seedBotConnector(t, ctx, st, "in-max-registration-prompt")
	user, _, err := st.GetOrCreateUserByMessenger(ctx, domain.MessengerKindMAX, "193465777", "")
	if err != nil {
		t.Fatalf("GetOrCreateUserByMessenger: %v", err)
	}
	user.FullName = "Федор Николаевич"
	user.Phone = "+79990001122"
	user.Email = "fedor@example.com"
	user.UpdatedAt = time.Now().UTC()
	if err := st.SaveUser(ctx, user); err != nil {
		t.Fatalf("SaveUser: %v", err)
	}

	h.handleCallback(ctx, maxAction("cb-max-registration-prompt", 193465777, "", "accept_terms:"+int64ToString(connectorID)))

	state, found, err := st.GetRegistrationState(ctx, domain.MessengerKindMAX, "193465777")
	if err != nil {
		t.Fatalf("get registration state: %v", err)
	}
	if !found {
		t.Fatalf("registration state not found")
	}
	if state.Step != domain.StepUsername {
		t.Fatalf("registration step = %s, want %s", state.Step, domain.StepUsername)
	}
	if len(sender.sent) != 1 {
		t.Fatalf("sent messages = %d, want 1", len(sender.sent))
	}
	if got := sender.sent[0].msg.Text; got != "Ник в мессенджере" {
		t.Fatalf("prompt = %q, want generic messenger username prompt", got)
	}
	if sender.sent[0].user.Kind != messenger.KindMAX || sender.sent[0].user.UserID != 193465777 {
		t.Fatalf("unexpected MAX recipient: %+v", sender.sent[0].user)
	}
}

func TestMAXMenuParity_SubscriptionAndPayments(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	sender := &fakeSender{}
	h := NewHandler(st, sender, nil, true, "https://investcontrol.example", "test-encryption-key-123456789012345")

	connectorID := seedAutopayConnector(t, ctx, st, "max-menu-plan", "MAX plan")
	paymentID := seedMAXPayment(t, ctx, st, 193465778, connectorID, true)
	_ = seedMAXSubscription(t, ctx, st, 193465778, connectorID, paymentID, true)

	h.handleMessage(ctx, messenger.IncomingMessage{
		User:   maxIdentity(193465778, "fedor"),
		ChatID: 193465778,
		Text:   "/menu",
	})
	h.handleMenuCallback(ctx, maxAction("max-menu-subscription", 193465778, "fedor", menuCallbackSubscription))
	h.handleMenuCallback(ctx, maxAction("max-menu-payments", 193465778, "fedor", menuCallbackPayments))

	if len(sender.sent) != 3 {
		t.Fatalf("sent messages = %d, want 3", len(sender.sent))
	}
	if sender.sent[0].msg.Text != botMenuTitle {
		t.Fatalf("menu text = %q", sender.sent[0].msg.Text)
	}
	if sender.sent[0].user.Kind != messenger.KindMAX || sender.sent[0].user.UserID != 193465778 {
		t.Fatalf("menu recipient = %+v, want MAX user", sender.sent[0].user)
	}
	if got := sender.sent[1].msg.Text; !strings.Contains(got, "📄 Моя подписка") || !strings.Contains(got, "MAX plan") {
		t.Fatalf("subscription overview = %q", got)
	}
	if got := sender.sent[2].msg.Text; !strings.Contains(got, "💳 Последние платежи") || !strings.Contains(got, "PAID") {
		t.Fatalf("payment history = %q", got)
	}
}

func TestHandlePay_MAXCreatesPaymentAndSendsLink(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	sender := &fakeSender{}
	robokassa := payment.NewRobokassaService(payment.RobokassaConfig{
		MerchantLogin: "test-merchant",
		Password1:     "test-pass1",
		Password2:     "test-pass2",
		IsTest:        true,
		BaseURL:       "https://auth.robokassa.ru/Merchant/Index.aspx",
	})
	h := NewHandler(st, sender, robokassa, true, "http://localhost:8080", "test-encryption-key-123456789012345")

	connectorID := seedBotConnector(t, ctx, st, "in-max-pay")
	seedRecurringLegalDocs(t, ctx, st)

	h.handlePay(ctx, maxAction("cb-max-pay", 193465779, "fedor", "pay:"+int64ToString(connectorID)+":1"))

	user, found, err := st.GetUserByMessenger(ctx, domain.MessengerKindMAX, "193465779")
	if err != nil {
		t.Fatalf("get user by MAX messenger: %v", err)
	}
	if !found {
		t.Fatalf("user not found")
	}
	payments, err := st.ListPayments(ctx, domain.PaymentListQuery{UserID: user.ID, Limit: 10})
	if err != nil {
		t.Fatalf("list payments: %v", err)
	}
	if len(payments) != 1 {
		t.Fatalf("payments len = %d, want 1", len(payments))
	}
	if !payments[0].AutoPayEnabled {
		t.Fatalf("payment AutoPayEnabled = false, want true")
	}
	if len(sender.sent) != 1 {
		t.Fatalf("sent messages = %d, want 1", len(sender.sent))
	}
	if sender.sent[0].user.Kind != messenger.KindMAX || sender.sent[0].user.UserID != 193465779 {
		t.Fatalf("payment recipient = %+v, want MAX user", sender.sent[0].user)
	}
	if got := sender.sent[0].msg.Text; !strings.Contains(strings.ToLower(got), "robokassa") {
		t.Fatalf("payment text = %q, want provider info", got)
	}
	if len(sender.sent[0].msg.Buttons) != 1 || len(sender.sent[0].msg.Buttons[0]) != 1 {
		t.Fatalf("payment buttons = %+v, want checkout button", sender.sent[0].msg.Buttons)
	}
	if !strings.Contains(sender.sent[0].msg.Buttons[0][0].URL, "Recurring=true") {
		t.Fatalf("checkout url = %q, want recurring flag", sender.sent[0].msg.Buttons[0][0].URL)
	}
}

func maxIdentity(userID int64, username string) messenger.UserIdentity {
	return messenger.UserIdentity{
		Kind:     messenger.KindMAX,
		ID:       userID,
		Username: username,
	}
}

func maxAction(id string, userID int64, username, data string) messenger.IncomingAction {
	return messenger.IncomingAction{
		Ref: messenger.ActionRef{
			Kind: messenger.KindMAX,
			ID:   id,
		},
		User:      maxIdentity(userID, username),
		ChatID:    userID,
		MessageID: 0,
		Data:      data,
	}
}

func seedMAXPayment(t *testing.T, ctx context.Context, st *memory.Store, maxUserID, connectorID int64, autopay bool) int64 {
	t.Helper()

	user, _, err := st.GetOrCreateUserByMessenger(ctx, domain.MessengerKindMAX, int64ToString(maxUserID), "")
	if err != nil {
		t.Fatalf("get or create MAX user: %v", err)
	}
	err = st.CreatePayment(ctx, domain.Payment{
		Provider:       "robokassa",
		Status:         domain.PaymentStatusPaid,
		Token:          "max-token-" + int64ToString(maxUserID) + "-" + int64ToString(connectorID),
		UserID:         user.ID,
		ConnectorID:    connectorID,
		AmountRUB:      2300,
		AutoPayEnabled: autopay,
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("create payment: %v", err)
	}
	payments, err := st.ListPayments(ctx, domain.PaymentListQuery{UserID: user.ID, ConnectorID: connectorID, Limit: 10})
	if err != nil {
		t.Fatalf("list payments: %v", err)
	}
	if len(payments) == 0 {
		t.Fatalf("payment not found after create")
	}
	return payments[0].ID
}

func seedMAXSubscription(t *testing.T, ctx context.Context, st *memory.Store, maxUserID, connectorID, paymentID int64, autopay bool) int64 {
	t.Helper()

	user, _, err := st.GetOrCreateUserByMessenger(ctx, domain.MessengerKindMAX, int64ToString(maxUserID), "")
	if err != nil {
		t.Fatalf("get or create MAX user: %v", err)
	}
	err = st.UpsertSubscriptionByPayment(ctx, domain.Subscription{
		UserID:         user.ID,
		ConnectorID:    connectorID,
		PaymentID:      paymentID,
		Status:         domain.SubscriptionStatusActive,
		AutoPayEnabled: autopay,
		StartsAt:       time.Now().UTC(),
		EndsAt:         time.Now().UTC().Add(30 * 24 * time.Hour),
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("upsert subscription: %v", err)
	}
	sub, found, err := st.GetLatestSubscriptionByUserConnector(ctx, user.ID, connectorID)
	if err != nil {
		t.Fatalf("get latest subscription: %v", err)
	}
	if !found {
		t.Fatalf("subscription not found")
	}
	return sub.ID
}
