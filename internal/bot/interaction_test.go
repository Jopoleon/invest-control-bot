package bot

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
	"github.com/Jopoleon/invest-control-bot/internal/messenger"
	"github.com/Jopoleon/invest-control-bot/internal/store/memory"
)

func TestHandleMessage_MenuSendsRecurringCabinet(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	sender := &fakeSender{}
	h := NewHandler(st, sender, nil, true, "http://localhost:8080", "test-encryption-key-123456789012345")

	h.handleMessage(ctx, messengerMessage(42, "egor", 42, "/menu"))

	if len(sender.sent) != 1 {
		t.Fatalf("sent messages = %d, want 1", len(sender.sent))
	}
	msg := sender.sent[0].msg
	if msg.Text != "Личный кабинет:" {
		t.Fatalf("menu text = %q", msg.Text)
	}
	if len(msg.Buttons) != 3 {
		t.Fatalf("menu rows = %d, want 3", len(msg.Buttons))
	}
	if msg.Buttons[2][0].Action != menuCallbackAutopay {
		t.Fatalf("third button action = %q, want %q", msg.Buttons[2][0].Action, menuCallbackAutopay)
	}
}

func TestReactivateAutopayForSubscription_ReenablesWithoutNewPayment(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	sender := &fakeSender{}
	h := NewHandler(st, sender, nil, true, "https://investcontrol.example", "test-encryption-key-123456789012345")

	connectorID := seedAutopayConnector(t, ctx, st, "reactivate-plan", "reactivate-plan")
	seedRecurringDocs(t, ctx, st)
	paymentID := seedPayment(t, ctx, st, 7001, connectorID, true)
	subID := seedSubscription(t, ctx, st, 7001, connectorID, paymentID, false)

	h.handleMenuCallback(ctx, testAction("reactivate", 7001, "egor", menuCallbackAutopayOnSub+int64ToString(subID)))

	sub, found, err := st.GetSubscriptionByID(ctx, subID)
	if err != nil {
		t.Fatalf("get subscription: %v", err)
	}
	if !found {
		t.Fatalf("subscription not found")
	}
	if !sub.AutoPayEnabled {
		t.Fatalf("subscription autopay = false, want true")
	}

	user, foundUser, err := st.GetUser(ctx, 7001)
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	if !foundUser {
		t.Fatalf("user not found")
	}
	consents, err := st.ListRecurringConsentsByUser(ctx, user.ID)
	if err != nil {
		t.Fatalf("list recurring consents: %v", err)
	}
	if len(consents) != 1 {
		t.Fatalf("recurring consents = %d, want 1", len(consents))
	}

	if len(sender.edited) != 1 {
		t.Fatalf("edited messages = %d, want 1", len(sender.edited))
	}
	if !strings.Contains(sender.edited[0].msg.Text, "снова включен") {
		t.Fatalf("edited text = %q", sender.edited[0].msg.Text)
	}
	payments, err := st.ListPayments(ctx, domain.PaymentListQuery{UserID: user.ID, Limit: 10})
	if err != nil {
		t.Fatalf("list payments: %v", err)
	}
	if len(payments) != 1 {
		t.Fatalf("payments = %d, want 1", len(payments))
	}
}

func TestDisableAutopayForSubscription_OnlyTouchesTargetSubscription(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	sender := &fakeSender{}
	h := NewHandler(st, sender, nil, true, "https://investcontrol.example", "test-encryption-key-123456789012345")

	connectorOne := seedAutopayConnector(t, ctx, st, "plan-a", "plan-a")
	connectorTwo := seedAutopayConnector(t, ctx, st, "plan-b", "plan-b")
	paymentOne := seedPayment(t, ctx, st, 8001, connectorOne, true)
	paymentTwo := seedPayment(t, ctx, st, 8001, connectorTwo, true)
	subOne := seedSubscription(t, ctx, st, 8001, connectorOne, paymentOne, true)
	subTwo := seedSubscription(t, ctx, st, 8001, connectorTwo, paymentTwo, true)

	h.handleMenuCallback(ctx, testAction("disable-one", 8001, "egor", menuCallbackAutopayOffSub+int64ToString(subOne)))

	gotOne, found, err := st.GetSubscriptionByID(ctx, subOne)
	if err != nil {
		t.Fatalf("get first subscription: %v", err)
	}
	if !found {
		t.Fatalf("first subscription not found")
	}
	if gotOne.AutoPayEnabled {
		t.Fatalf("first subscription autopay = true, want false")
	}

	gotTwo, found, err := st.GetSubscriptionByID(ctx, subTwo)
	if err != nil {
		t.Fatalf("get second subscription: %v", err)
	}
	if !found {
		t.Fatalf("second subscription not found")
	}
	if !gotTwo.AutoPayEnabled {
		t.Fatalf("second subscription autopay = false, want true")
	}

	if len(sender.edited) != 1 {
		t.Fatalf("edited messages = %d, want 1", len(sender.edited))
	}
	if !strings.Contains(sender.edited[0].msg.Text, "отключен для подписки") {
		t.Fatalf("edited text = %q", sender.edited[0].msg.Text)
	}
}

func messengerMessage(userID int64, username string, chatID int64, text string) messenger.IncomingMessage {
	return messenger.IncomingMessage{
		User:   userIdentity(userID, username),
		ChatID: chatID,
		Text:   text,
	}
}

func seedAutopayConnector(t *testing.T, ctx context.Context, st *memory.Store, payload, name string) int64 {
	t.Helper()

	if err := st.CreateConnector(ctx, domain.Connector{
		StartPayload: payload,
		Name:         name,
		ChatID:       "@test_channel",
		PriceRUB:     2300,
		PeriodDays:   30,
		IsActive:     true,
		CreatedAt:    time.Now().UTC(),
	}); err != nil {
		t.Fatalf("create connector: %v", err)
	}
	connector, found, err := st.GetConnectorByStartPayload(ctx, payload)
	if err != nil {
		t.Fatalf("get connector by payload: %v", err)
	}
	if !found {
		t.Fatalf("connector not found")
	}
	return connector.ID
}

func seedPayment(t *testing.T, ctx context.Context, st *memory.Store, telegramID, connectorID int64, autopay bool) int64 {
	t.Helper()

	user, _, err := st.GetOrCreateUserByMessenger(ctx, domain.MessengerKindTelegram, int64ToString(telegramID), "")
	if err != nil {
		t.Fatalf("get or create user by messenger: %v", err)
	}

	err = st.CreatePayment(ctx, domain.Payment{
		Provider:       "robokassa",
		Status:         domain.PaymentStatusPaid,
		Token:          "token-" + int64ToString(telegramID) + "-" + int64ToString(connectorID) + "-" + boolSuffix(autopay),
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

func seedSubscription(t *testing.T, ctx context.Context, st *memory.Store, telegramID, connectorID, paymentID int64, autopay bool) int64 {
	t.Helper()

	user, _, err := st.GetOrCreateUserByMessenger(ctx, domain.MessengerKindTelegram, int64ToString(telegramID), "")
	if err != nil {
		t.Fatalf("get or create user by messenger: %v", err)
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

func seedRecurringDocs(t *testing.T, ctx context.Context, st *memory.Store) {
	t.Helper()

	for _, doc := range []domain.LegalDocument{
		{
			Type:      domain.LegalDocumentTypeOffer,
			Title:     "Offer",
			Content:   "Offer content",
			Version:   1,
			IsActive:  true,
			CreatedAt: time.Now().UTC(),
		},
		{
			Type:      domain.LegalDocumentTypeUserAgreement,
			Title:     "Agreement",
			Content:   "Agreement content",
			Version:   1,
			IsActive:  true,
			CreatedAt: time.Now().UTC(),
		},
	} {
		if err := st.CreateLegalDocument(ctx, doc); err != nil {
			t.Fatalf("create legal document: %v", err)
		}
	}
}

func boolSuffix(v bool) string {
	if v {
		return "on"
	}
	return "off"
}

func TestSendSubscriptionOverview_BuildsResolvedChannelText(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	sender := &fakeSender{}
	h := NewHandler(st, sender, nil, true, "https://investcontrol.example", "test-encryption-key-123456789012345")

	connectorID := seedAutopayConnector(t, ctx, st, "sub-overview", "sub-overview")
	paymentID := seedPayment(t, ctx, st, 9106, connectorID, true)
	_ = seedSubscription(t, ctx, st, 9106, connectorID, paymentID, true)

	h.sendSubscriptionOverview(ctx, 9106, messenger.UserIdentity{Kind: messenger.KindTelegram, ID: 9106})

	if len(sender.sent) != 1 {
		t.Fatalf("sent messages = %d, want 1", len(sender.sent))
	}
	text := sender.sent[0].msg.Text
	if !strings.Contains(text, botMenuSubscriptionHeader) {
		t.Fatalf("text = %q", text)
	}
	if !strings.Contains(text, "sub-overview") {
		t.Fatalf("text = %q", text)
	}
	if !strings.Contains(text, "https://t.me/test_channel") {
		t.Fatalf("text = %q", text)
	}
}

func TestSendPaymentHistory_BuildsLatestPaymentsText(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	sender := &fakeSender{}
	h := NewHandler(st, sender, nil, true, "https://investcontrol.example", "test-encryption-key-123456789012345")

	connectorID := seedAutopayConnector(t, ctx, st, "payment-history", "payment-history")
	_ = seedPayment(t, ctx, st, 9107, connectorID, true)

	h.sendPaymentHistory(ctx, 9107, messenger.UserIdentity{Kind: messenger.KindTelegram, ID: 9107})

	if len(sender.sent) != 1 {
		t.Fatalf("sent messages = %d, want 1", len(sender.sent))
	}
	text := sender.sent[0].msg.Text
	if !strings.Contains(text, botMenuPaymentsHeader) {
		t.Fatalf("text = %q", text)
	}
	if !strings.Contains(text, "2300 ₽") {
		t.Fatalf("text = %q", text)
	}
	if !strings.Contains(text, "PAID") {
		t.Fatalf("text = %q", text)
	}
}

func TestReactivateAutopayForSubscription_RejectsForeignSubscription(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	sender := &fakeSender{}
	h := NewHandler(st, sender, nil, true, "https://investcontrol.example", "test-encryption-key-123456789012345")

	connectorID := seedAutopayConnector(t, ctx, st, "foreign-sub", "foreign-sub")
	seedRecurringDocs(t, ctx, st)
	paymentID := seedPayment(t, ctx, st, 9201, connectorID, true)
	subID := seedSubscription(t, ctx, st, 9201, connectorID, paymentID, false)

	h.handleMenuCallback(ctx, testAction("reactivate-foreign", 9202, "egor", menuCallbackAutopayOnSub+int64ToString(subID)))

	if len(sender.sent) != 1 {
		t.Fatalf("sent messages = %d, want 1", len(sender.sent))
	}
	if sender.sent[0].msg.Text != botMsgAutopaySubscriptionNotFound {
		t.Fatalf("text = %q", sender.sent[0].msg.Text)
	}
}
