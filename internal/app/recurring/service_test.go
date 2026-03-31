package recurring

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
	"github.com/Jopoleon/invest-control-bot/internal/messenger"
	"github.com/Jopoleon/invest-control-bot/internal/store/memory"
)

func TestAttemptOrdinal(t *testing.T) {
	now := time.Now().UTC()
	cases := []struct {
		name string
		ends time.Time
		want int
	}{
		{name: "outside window", ends: now.Add(80 * time.Hour), want: 0},
		{name: "first window", ends: now.Add(70 * time.Hour), want: 1},
		{name: "second window", ends: now.Add(40 * time.Hour), want: 2},
		{name: "third window", ends: now.Add(10 * time.Hour), want: 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := AttemptOrdinal(now, tc.ends); got != tc.want {
				t.Fatalf("AttemptOrdinal()=%d want=%d", got, tc.want)
			}
		})
	}
}

func TestAttemptOrdinalForPeriod_UsesShortTestWindows(t *testing.T) {
	now := time.Now().UTC()

	if got := attemptOrdinalForPeriod(now, now.Add(90*time.Second), 120); got != 0 {
		t.Fatalf("attemptOrdinalForPeriod(90s remaining)=%d want=0", got)
	}
	if got := attemptOrdinalForPeriod(now, now.Add(25*time.Second), 120); got != 1 {
		t.Fatalf("attemptOrdinalForPeriod(25s remaining)=%d want=1", got)
	}
	if got := attemptOrdinalForPeriod(now, now.Add(8*time.Second), 120); got != 2 {
		t.Fatalf("attemptOrdinalForPeriod(8s remaining)=%d want=2", got)
	}
}

func TestShouldTriggerScheduledRebill_SkipsWhenPendingExists(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	userID := int64(42)
	subscriptionID := int64(7)
	now := time.Now().UTC()

	if err := st.CreateConnector(ctx, domain.Connector{
		StartPayload: "in-pending-rebill",
		Name:         "pending-rebill",
		PriceRUB:     500,
		PeriodDays:   30,
		IsActive:     true,
		CreatedAt:    now,
	}); err != nil {
		t.Fatalf("CreateConnector err=%v", err)
	}
	connector, found, err := st.GetConnectorByStartPayload(ctx, "in-pending-rebill")
	if err != nil || !found {
		t.Fatalf("GetConnectorByStartPayload found=%v err=%v", found, err)
	}

	if err := st.CreatePayment(ctx, domain.Payment{
		Provider:        "robokassa",
		Status:          domain.PaymentStatusPending,
		Token:           "pending-rebill-1",
		UserID:          userID,
		ConnectorID:     connector.ID,
		SubscriptionID:  subscriptionID,
		ParentPaymentID: 10,
		CreatedAt:       now,
		UpdatedAt:       now,
	}); err != nil {
		t.Fatalf("CreatePayment err=%v", err)
	}
	service := &Service{Store: st}

	ok, err := service.ShouldTriggerScheduledRebill(ctx, domain.Subscription{ID: subscriptionID, UserID: userID, ConnectorID: connector.ID, EndsAt: now.Add(10 * time.Hour)}, now)
	if err != nil {
		t.Fatalf("ShouldTriggerScheduledRebill err=%v", err)
	}
	if ok {
		t.Fatalf("ShouldTriggerScheduledRebill=true want false")
	}
}

func TestShouldTriggerScheduledRebill_UsesShortTestPeriodWindows(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	now := time.Now().UTC()

	if err := st.CreateConnector(ctx, domain.Connector{
		StartPayload:      "in-short-rebill-window",
		Name:              "short-window",
		PriceRUB:          500,
		PeriodDays:        30,
		TestPeriodSeconds: 120,
		IsActive:          true,
		CreatedAt:         now,
	}); err != nil {
		t.Fatalf("CreateConnector err=%v", err)
	}
	connector, found, err := st.GetConnectorByStartPayload(ctx, "in-short-rebill-window")
	if err != nil || !found {
		t.Fatalf("GetConnectorByStartPayload found=%v err=%v", found, err)
	}

	if err := st.CreatePayment(ctx, domain.Payment{
		Provider:       "robokassa",
		Status:         domain.PaymentStatusPaid,
		Token:          "short-parent",
		UserID:         42,
		ConnectorID:    connector.ID,
		AmountRUB:      500,
		AutoPayEnabled: true,
		CreatedAt:      now.Add(-time.Minute),
		UpdatedAt:      now.Add(-time.Minute),
	}); err != nil {
		t.Fatalf("CreatePayment err=%v", err)
	}
	parentPayment, found, err := st.GetPaymentByToken(ctx, "short-parent")
	if err != nil || !found {
		t.Fatalf("GetPaymentByToken found=%v err=%v", found, err)
	}

	if err := st.UpsertSubscriptionByPayment(ctx, domain.Subscription{
		UserID:         parentPayment.UserID,
		ConnectorID:    connector.ID,
		PaymentID:      parentPayment.ID,
		Status:         domain.SubscriptionStatusActive,
		AutoPayEnabled: true,
		StartsAt:       now,
		EndsAt:         now.Add(2 * time.Minute),
		CreatedAt:      now,
		UpdatedAt:      now,
	}); err != nil {
		t.Fatalf("UpsertSubscriptionByPayment err=%v", err)
	}
	sub, found, err := st.GetLatestSubscriptionByUserConnector(ctx, parentPayment.UserID, connector.ID)
	if err != nil || !found {
		t.Fatalf("GetLatestSubscriptionByUserConnector found=%v err=%v", found, err)
	}

	service := &Service{Store: st}

	ok, err := service.ShouldTriggerScheduledRebill(ctx, sub, now.Add(30*time.Second))
	if err != nil {
		t.Fatalf("ShouldTriggerScheduledRebill early err=%v", err)
	}
	if ok {
		t.Fatalf("ShouldTriggerScheduledRebill early=true want false")
	}

	ok, err = service.ShouldTriggerScheduledRebill(ctx, sub, now.Add(95*time.Second))
	if err != nil {
		t.Fatalf("ShouldTriggerScheduledRebill near end err=%v", err)
	}
	if !ok {
		t.Fatalf("ShouldTriggerScheduledRebill near end=false want true")
	}
}

func TestProcessCancelRequest_DisablesAutopayAndNotifiesUser(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	now := time.Date(2026, 3, 31, 12, 0, 0, 0, time.UTC)

	user, _, err := st.GetOrCreateUserByMessenger(ctx, domain.MessengerKindMAX, "193465776", "max_user")
	if err != nil {
		t.Fatalf("GetOrCreateUserByMessenger err=%v", err)
	}
	if err := st.CreateConnector(ctx, domain.Connector{
		StartPayload: "in-cancel-test",
		Name:         "Tariff",
		PriceRUB:     3200,
		PeriodDays:   30,
		IsActive:     true,
		CreatedAt:    now,
	}); err != nil {
		t.Fatalf("CreateConnector err=%v", err)
	}
	connector, found, err := st.GetConnectorByStartPayload(ctx, "in-cancel-test")
	if err != nil || !found {
		t.Fatalf("GetConnectorByStartPayload found=%v err=%v", found, err)
	}
	if err := st.CreatePayment(ctx, domain.Payment{
		Provider:       "robokassa",
		Status:         domain.PaymentStatusPaid,
		Token:          "paid-1",
		UserID:         user.ID,
		ConnectorID:    connector.ID,
		SubscriptionID: 1,
		AmountRUB:      3200,
		AutoPayEnabled: true,
		CreatedAt:      now,
		UpdatedAt:      now,
	}); err != nil {
		t.Fatalf("CreatePayment err=%v", err)
	}
	if err := st.UpsertSubscriptionByPayment(ctx, domain.Subscription{
		UserID:         user.ID,
		ConnectorID:    connector.ID,
		PaymentID:      1,
		Status:         domain.SubscriptionStatusActive,
		AutoPayEnabled: true,
		StartsAt:       now.Add(-time.Hour),
		EndsAt:         now.Add(24 * time.Hour),
		CreatedAt:      now,
		UpdatedAt:      now,
	}); err != nil {
		t.Fatalf("UpsertSubscriptionByPayment err=%v", err)
	}
	sub, found, err := st.GetLatestSubscriptionByUserConnector(ctx, user.ID, connector.ID)
	if err != nil || !found {
		t.Fatalf("GetLatestSubscriptionByUserConnector found=%v err=%v", found, err)
	}

	sent := 0
	service := &Service{
		Store: st,
		SendUserNotification: func(_ context.Context, userID int64, preferredMessengerUserID string, msg messenger.OutgoingMessage) error {
			sent++
			if userID != user.ID {
				t.Fatalf("userID=%d want=%d", userID, user.ID)
			}
			if preferredMessengerUserID != "193465776" {
				t.Fatalf("preferredMessengerUserID=%q want=193465776", preferredMessengerUserID)
			}
			if msg.Text == "" {
				t.Fatalf("empty notification text")
			}
			return nil
		},
		BuildTargetAuditEvent: func(_ context.Context, userID int64, preferredMessengerUserID string, connectorID int64, action, details string, createdAt time.Time) domain.AuditEvent {
			return domain.AuditEvent{TargetUserID: userID, TargetMessengerUserID: preferredMessengerUserID, ConnectorID: connectorID, Action: action, Details: details, CreatedAt: createdAt}
		},
		ResolveUserByMessengerUserID: func(ctx context.Context, messengerUserID int64) (domain.User, bool, error) {
			return st.GetUserByMessenger(ctx, domain.MessengerKindMAX, strconv.FormatInt(messengerUserID, 10))
		},
		ResolveTelegramMessengerUserID: func(context.Context, int64) (int64, bool, error) {
			return 0, false, nil
		},
		ResolveConnectorChannel:               func(channelURL, chatID string) string { return channelURL + chatID },
		ConnectorPeriodLabel:                  func(domain.Connector) string { return "30 дн." },
		RecurringCancelTitle:                  "Отключение автоплатежа",
		RecurringCancelSubsLoadFail:           "load failed",
		RecurringCancelMissingSub:             "missing sub",
		RecurringCancelAlreadyOff:             "already off",
		RecurringCancelPersistFailed:          "persist failed",
		RecurringCancelNotification:           func(string) string { return "cancel ok" },
		RecurringCancelSuccessForSubscription: func(name string) string { return "done " + name },
	}

	connectorName, pageData, status := service.ProcessCancelRequest(ctx, "token-1", 193465776, sub.ID, now.Add(time.Hour), now)
	if status != 0 {
		t.Fatalf("status=%d pageData=%+v want success", status, pageData)
	}
	if connectorName != "Tariff" {
		t.Fatalf("connectorName=%q want Tariff", connectorName)
	}
	if sent != 1 {
		t.Fatalf("sent=%d want=1", sent)
	}

	updatedSub, found, err := st.GetSubscriptionByID(ctx, sub.ID)
	if err != nil || !found {
		t.Fatalf("GetSubscriptionByID found=%v err=%v", found, err)
	}
	if updatedSub.AutoPayEnabled {
		t.Fatalf("AutoPayEnabled=true want false")
	}
}

func TestBuildCancelPageData_ShowsSuccessBanner(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	now := time.Date(2026, 3, 31, 12, 0, 0, 0, time.UTC)
	service := &Service{
		Store: st,
		ResolveUserByMessengerUserID: func(context.Context, int64) (domain.User, bool, error) {
			return domain.User{}, false, nil
		},
		ResolveTelegramMessengerUserID: func(context.Context, int64) (int64, bool, error) {
			return 0, false, nil
		},
		ResolveConnectorChannel:               func(channelURL, chatID string) string { return channelURL + chatID },
		ConnectorPeriodLabel:                  func(domain.Connector) string { return "30 дн." },
		RecurringCancelTitle:                  "Отключение автоплатежа",
		RecurringCancelSubsLoadFail:           "load failed",
		RecurringCancelMissingSub:             "missing sub",
		RecurringCancelAlreadyOff:             "already off",
		RecurringCancelPersistFailed:          "persist failed",
		RecurringCancelNotification:           func(string) string { return "cancel ok" },
		RecurringCancelSuccessForSubscription: func(name string) string { return "done " + name },
	}

	data, status := service.BuildCancelPageData(ctx, "token-1", 193465776, now.Add(time.Hour), "Tariff")
	if status != 200 {
		t.Fatalf("status=%d want=200", status)
	}
	if data.SuccessMessage != "done Tariff" {
		t.Fatalf("SuccessMessage=%q want=%q", data.SuccessMessage, "done Tariff")
	}
}
