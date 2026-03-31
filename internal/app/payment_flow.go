package app

import (
	"context"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
)

func (a *application) activateSuccessfulPayment(ctx context.Context, paymentRow domain.Payment, providerPaymentID string, now time.Time) {
	a.payments().activateSuccessfulPayment(ctx, paymentRow, providerPaymentID, now)
}

func (a *application) notifyFailedRecurringPayment(ctx context.Context, paymentRow domain.Payment) {
	a.payments().notifyFailedRecurringPayment(ctx, paymentRow)
}

func (a *application) buildPaymentPageActions(ctx context.Context, paymentRow domain.Payment, channelURL string, success bool) []paymentPageAction {
	return a.payments().buildPaymentPageActions(ctx, paymentRow, channelURL, success)
}
