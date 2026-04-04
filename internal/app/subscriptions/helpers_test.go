package subscriptions

import (
	"testing"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
)

func TestHelperFunctions(t *testing.T) {
	if got := buildBotStartURL("@test_bot", "payload-1"); got != "https://t.me/test_bot?start=payload-1" {
		t.Fatalf("buildBotStartURL=%q", got)
	}
	if _, ok := normalizeTelegramChatID("bad"); ok {
		t.Fatalf("normalizeTelegramChatID should reject invalid input")
	}
	if got, ok := normalizeTelegramChatID("100777"); !ok || got != -100777 {
		t.Fatalf("normalizeTelegramChatID positive=%d ok=%v want -100777,true", got, ok)
	}
	if got, ok := normalizeTelegramChatID("-100888"); !ok || got != -100888 {
		t.Fatalf("normalizeTelegramChatID negative=%d ok=%v want -100888,true", got, ok)
	}
	if got := subscriptionRevokeRetryDelay(1); got != 5*time.Minute {
		t.Fatalf("retryDelay(1)=%s want 5m", got)
	}
	if got := subscriptionRevokeRetryDelay(2); got != 30*time.Minute {
		t.Fatalf("retryDelay(2)=%s want 30m", got)
	}
}

func TestBuildSubscriptionRevokeStateAndAuditHelpers(t *testing.T) {
	now := time.Now().UTC()
	events := []domain.AuditEvent{
		{
			Action:    domain.AuditActionSubscriptionRevokeFailed,
			Details:   "subscription_id=17;attempt=1;reason=telegram_failed",
			CreatedAt: now.Add(-2 * time.Minute),
		},
		{
			Action:    domain.AuditActionSubscriptionRevokeFailed,
			Details:   "subscription_id=17;attempt=2;reason=telegram_client_not_configured",
			CreatedAt: now.Add(-time.Minute),
		},
	}
	state := buildSubscriptionRevokeState(17, events)
	if state.failureCount != 2 {
		t.Fatalf("failureCount=%d want 2", state.failureCount)
	}
	if state.lastFailureReason != "telegram_client_not_configured" {
		t.Fatalf("lastFailureReason=%q", state.lastFailureReason)
	}
	if auditDetailInt64(events[0].Details, "subscription_id") != 17 {
		t.Fatalf("auditDetailInt64 failed")
	}
	if auditDetailValue(events[1].Details, "reason") != "telegram_client_not_configured" {
		t.Fatalf("auditDetailValue failed")
	}
	if details := buildSubscriptionRevokeManualCheckDetails(17, 3, "remove_chat_member_failed"); details != "subscription_id=17;failed_attempts=3;reason=remove_chat_member_failed" {
		t.Fatalf("manualCheckDetails=%q", details)
	}
	if got := prependAuditEvent(events, domain.AuditEvent{Action: domain.AuditActionSubscriptionRevokedFromChat}); len(got) != 3 || got[0].Action != domain.AuditActionSubscriptionRevokedFromChat {
		t.Fatalf("prependAuditEvent=%+v", got)
	}
}
