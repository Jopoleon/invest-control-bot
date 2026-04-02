package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/config"
)

func TestRunStartupStepWithRetry_RetriesTimeoutsAndSucceeds(t *testing.T) {
	attempts := 0
	err := runStartupStepWithRetry(context.Background(), "test step", startupRetryPolicy{
		attempts: 3,
		timeout:  time.Second,
		backoff:  0,
	}, func(context.Context) error {
		attempts++
		if attempts < 3 {
			return context.DeadlineExceeded
		}
		return nil
	})
	if err != nil {
		t.Fatalf("runStartupStepWithRetry err=%v", err)
	}
	if attempts != 3 {
		t.Fatalf("attempts=%d want=3", attempts)
	}
}

func TestRunStartupStepWithRetry_DoesNotRetryNonRetryableErrors(t *testing.T) {
	attempts := 0
	wantErr := errors.New("bad token")
	err := runStartupStepWithRetry(context.Background(), "test step", startupRetryPolicy{
		attempts: 3,
		timeout:  time.Second,
		backoff:  0,
	}, func(context.Context) error {
		attempts++
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("err=%v want=%v", err, wantErr)
	}
	if attempts != 1 {
		t.Fatalf("attempts=%d want=1", attempts)
	}
}

func TestBuildPaymentService_UnsupportedProviderFails(t *testing.T) {
	_, _, err := buildPaymentService(config.Config{
		Payment: config.PaymentConfig{
			Provider: "stripe",
		},
	}, "https://example.com")
	if err == nil {
		t.Fatal("buildPaymentService err=nil want unsupported provider error")
	}
}
