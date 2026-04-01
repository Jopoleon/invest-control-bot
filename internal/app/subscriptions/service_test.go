package subscriptions

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
	"github.com/Jopoleon/invest-control-bot/internal/messenger"
)

func TestBuildRenewalNotification_MAXUsesCommandText(t *testing.T) {
	svc := &Service{
		TelegramBotUsername:  "test_bot",
		RenewalButtonLabel:   "Продлить подписку",
		RenewalCommandFormat: "\n\nДля продления отправьте команду:\n/start %s",
		ResolvePreferredKind: func(context.Context, int64, string) messenger.Kind { return messenger.KindMAX },
	}

	msg := svc.BuildRenewalNotification(context.Background(), 1, "193465776", "in-max-renew", "Напоминание")
	if !strings.Contains(msg.Text, "/start in-max-renew") {
		t.Fatalf("text=%q want max command", msg.Text)
	}
	if len(msg.Buttons) != 0 {
		t.Fatalf("button rows=%d want=0", len(msg.Buttons))
	}
}

func TestBuildRenewalNotification_TelegramUsesButton(t *testing.T) {
	svc := &Service{
		TelegramBotUsername:  "test_bot",
		RenewalButtonLabel:   "Продлить подписку",
		RenewalCommandFormat: "\n\nДля продления отправьте команду:\n/start %s",
		ResolvePreferredKind: func(context.Context, int64, string) messenger.Kind { return messenger.KindTelegram },
	}

	msg := svc.BuildRenewalNotification(context.Background(), 1, "777", "in-tg-renew", "Напоминание")
	if len(msg.Buttons) != 1 || len(msg.Buttons[0]) != 1 {
		t.Fatalf("buttons=%v want one telegram renewal button", msg.Buttons)
	}
	if msg.Buttons[0][0].URL != "https://t.me/test_bot?start=in-tg-renew" {
		t.Fatalf("url=%q want telegram deeplink", msg.Buttons[0][0].URL)
	}
}

func TestShouldSendReminder_SkipsShortTestPeriods(t *testing.T) {
	now := time.Now().UTC()
	connector := domain.Connector{PeriodMode: domain.ConnectorPeriodModeDuration, PeriodSeconds: 120}

	if shouldSendReminder(now, now.Add(20*time.Second), connector) {
		t.Fatalf("shouldSendReminder(short period)=true want false")
	}
}

func TestShouldSendExpiryNotice_SkipsShortTestPeriods(t *testing.T) {
	if shouldSendExpiryNotice(domain.Connector{PeriodMode: domain.ConnectorPeriodModeDuration, PeriodSeconds: 120}) {
		t.Fatalf("shouldSendExpiryNotice(short period)=true want false")
	}
	if !shouldSendExpiryNotice(domain.Connector{PeriodMode: domain.ConnectorPeriodModeDuration, PeriodSeconds: 30 * 24 * 60 * 60}) {
		t.Fatalf("shouldSendExpiryNotice(normal period)=false want true")
	}
}
