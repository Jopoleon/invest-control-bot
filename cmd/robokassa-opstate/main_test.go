package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Jopoleon/invest-control-bot/internal/config"
	"github.com/Jopoleon/invest-control-bot/internal/payment"
)

func TestRunOpStateMain_RequiresInvoiceID(t *testing.T) {
	var stdout, stderr bytes.Buffer

	exitCode := runOpStateMain(nil, opStateDeps{
		loadConfig: func() (config.Config, error) { return config.Config{}, nil },
		newService: func(config.Config) opStateLookupService {
			return opStateLookupServiceFunc(func(context.Context, string) (payment.RobokassaOpState, error) {
				return payment.RobokassaOpState{}, nil
			})
		},
		stdout: &stdout,
		stderr: &stderr,
	})

	if exitCode != 1 {
		t.Fatalf("exitCode = %d, want 1", exitCode)
	}
	if !strings.Contains(stderr.String(), "invoice-id is required") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestRunOpStateMain_Success(t *testing.T) {
	var stdout, stderr bytes.Buffer

	exitCode := runOpStateMain([]string{"-invoice-id", "100500"}, opStateDeps{
		loadConfig: func() (config.Config, error) { return config.Config{}, nil },
		newService: func(config.Config) opStateLookupService {
			return opStateLookupServiceFunc(func(_ context.Context, invoiceID string) (payment.RobokassaOpState, error) {
				if invoiceID != "100500" {
					t.Fatalf("invoiceID = %q", invoiceID)
				}
				return payment.RobokassaOpState{ResultCode: 0, StateCode: 100, InvoiceID: invoiceID, OutSum: "1.00"}, nil
			})
		},
		stdout: &stdout,
		stderr: &stderr,
	})

	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0, stderr=%q", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), "\"InvoiceID\": \"100500\"") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestRunOpStateMain_LookupError(t *testing.T) {
	var stdout, stderr bytes.Buffer

	exitCode := runOpStateMain([]string{"-invoice-id", "100500"}, opStateDeps{
		loadConfig: func() (config.Config, error) { return config.Config{}, nil },
		newService: func(config.Config) opStateLookupService {
			return opStateLookupServiceFunc(func(context.Context, string) (payment.RobokassaOpState, error) {
				return payment.RobokassaOpState{}, errors.New("provider unavailable")
			})
		},
		stdout: &stdout,
		stderr: &stderr,
	})

	if exitCode != 1 {
		t.Fatalf("exitCode = %d, want 1", exitCode)
	}
	if !strings.Contains(stderr.String(), "lookup opstate: provider unavailable") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

type opStateLookupServiceFunc func(context.Context, string) (payment.RobokassaOpState, error)

func (fn opStateLookupServiceFunc) LookupOperationState(ctx context.Context, invoiceID string) (payment.RobokassaOpState, error) {
	return fn(ctx, invoiceID)
}
