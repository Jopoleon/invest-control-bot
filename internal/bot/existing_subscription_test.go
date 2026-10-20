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

// staleRebillFixture seeds a MAX user with a completed profile, an active
// autopay subscription and one pending rebill created pendingAge ago.
type staleRebillFixture struct {
	st          *memory.Store
	sender      *fakeSender
	h           *Handler
	userID      int64
	maxUserID   int64
	connectorID int64
}

func newStaleRebillFixture(t *testing.T, pendingAge time.Duration) staleRebillFixture {
	t.Helper()

	ctx := context.Background()
	st := memory.New()
	sender := &fakeSender{}
	h := NewHandler(st, sender, payment.NewMockService("http://localhost:8080"), true, "http://localhost:8080", "test-encryption-key-123456789012345")

	const maxUserID int64 = 193465790
	connectorID := seedBotConnector(t, ctx, st, "in-stale-rebill")
	seedRecurringLegalDocs(t, ctx, st)

	parentPaymentID := seedMAXPayment(t, ctx, st, maxUserID, connectorID, true)
	subID := seedMAXSubscription(t, ctx, st, maxUserID, connectorID, parentPaymentID, true)

	user, _, err := st.GetOrCreateUserByMessenger(ctx, domain.MessengerKindMAX, int64ToString(maxUserID), "")
	if err != nil {
		t.Fatalf("get MAX user: %v", err)
	}
	user.FullName = "Мария Тестова"
	user.Phone = "+79990001122"
	user.Email = "maria@example.com"
	user.UpdatedAt = time.Now().UTC()
	if err := st.SaveUser(ctx, user); err != nil {
		t.Fatalf("save user: %v", err)
	}

	pendingAt := time.Now().UTC().Add(-pendingAge)
	if err := st.CreatePayment(ctx, domain.Payment{
		Provider:        "robokassa",
		Status:          domain.PaymentStatusPending,
		Token:           "rebill-pending-token",
		UserID:          user.ID,
		ConnectorID:     connectorID,
		SubscriptionID:  subID,
		ParentPaymentID: parentPaymentID,
		AutoPayEnabled:  true,
		AmountRUB:       4444,
		CreatedAt:       pendingAt,
		UpdatedAt:       pendingAt,
	}); err != nil {
		t.Fatalf("create pending rebill: %v", err)
	}

	return staleRebillFixture{st: st, sender: sender, h: h, userID: user.ID, maxUserID: maxUserID, connectorID: connectorID}
}

func (f staleRebillFixture) checkoutPayments(t *testing.T) []domain.Payment {
	t.Helper()
	payments, err := f.st.ListPayments(context.Background(), domain.PaymentListQuery{UserID: f.userID, Limit: 20})
	if err != nil {
		t.Fatalf("list payments: %v", err)
	}
	var out []domain.Payment
	for _, p := range payments {
		if p.SubscriptionID == 0 && p.Status == domain.PaymentStatusPending {
			out = append(out, p)
		}
	}
	return out
}

func TestAcceptTerms_StaleRebillOffersManualRenewal(t *testing.T) {
	f := newStaleRebillFixture(t, 2*time.Hour)

	f.h.handleCallback(context.Background(), maxAction("cb-stale-terms", f.maxUserID, "masha", "accept_terms:"+int64ToString(f.connectorID)))

	if len(f.sender.sent) != 1 {
		t.Fatalf("sent messages = %d, want 1", len(f.sender.sent))
	}
	msg := f.sender.sent[0].msg
	if !strings.Contains(msg.Text, "Автоплатеж не прошел") {
		t.Fatalf("text = %q, want failed autopay notice", msg.Text)
	}
	if strings.Contains(msg.Text, "уже включен") {
		t.Fatalf("text = %q, must not claim autopay is fine", msg.Text)
	}
	if !hasPayAction(msg.Buttons, f.connectorID) {
		t.Fatalf("buttons = %+v, want pay action", msg.Buttons)
	}
}

func TestHandlePay_StaleRebillCreatesManualPayment(t *testing.T) {
	f := newStaleRebillFixture(t, 2*time.Hour)

	f.h.handlePay(context.Background(), maxAction("cb-stale-pay", f.maxUserID, "masha", "pay:"+int64ToString(f.connectorID)+":0"))

	if got := len(f.checkoutPayments(t)); got != 1 {
		t.Fatalf("checkout payments = %d, want 1", got)
	}
}

func TestHandlePay_FreshRebillStillBlocksCheckout(t *testing.T) {
	f := newStaleRebillFixture(t, 5*time.Minute)

	f.h.handlePay(context.Background(), maxAction("cb-fresh-pay", f.maxUserID, "masha", "pay:"+int64ToString(f.connectorID)+":0"))

	if got := len(f.checkoutPayments(t)); got != 0 {
		t.Fatalf("checkout payments = %d, want 0 while rebill may still succeed", got)
	}
	if len(f.sender.sent) != 1 || !strings.Contains(f.sender.sent[0].msg.Text, "уже включен") {
		t.Fatalf("sent = %+v, want existing subscription message", f.sender.sent)
	}
}

func hasPayAction(rows [][]messenger.ActionButton, connectorID int64) bool {
	prefix := "pay:" + int64ToString(connectorID) + ":"
	for _, row := range rows {
		for _, b := range row {
			if strings.HasPrefix(b.Action, prefix) {
				return true
			}
		}
	}
	return false
}
