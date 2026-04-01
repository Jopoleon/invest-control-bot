package app

import (
	"context"
	"time"

	apprecurring "github.com/Jopoleon/invest-control-bot/internal/app/recurring"
	"github.com/Jopoleon/invest-control-bot/internal/domain"
)

var errRebillRequestFailed = apprecurring.ErrRebillRequestFailed

func (a *application) triggerRebill(ctx context.Context, subscriptionID int64, source string) (rebillResponse, error) {
	result, err := a.recurringService().TriggerRebill(ctx, subscriptionID, source)
	return rebillResponse{
		OK:        result.OK,
		InvoiceID: result.InvoiceID,
		Existing:  result.Existing,
	}, err
}

func (a *application) shouldTriggerScheduledRebill(ctx context.Context, sub domain.Subscription, now time.Time) (bool, error) {
	return a.recurringService().ShouldTriggerScheduledRebill(ctx, sub, now)
}

func (a *application) evaluateScheduledRebill(ctx context.Context, sub domain.Subscription, now time.Time) (apprecurring.ScheduledRebillDecision, error) {
	return a.recurringService().EvaluateScheduledRebill(ctx, sub, now)
}

func (a *application) recurringService() *apprecurring.Service {
	return &apprecurring.Service{
		Store:                                 a.store,
		RobokassaService:                      a.robokassaService,
		GenerateInvoiceID:                     generateInvoiceID,
		BuildTargetAuditEvent:                 a.buildAppTargetAuditEvent,
		SendUserNotification:                  a.sendUserNotification,
		ResolveUserByMessengerUserID:          a.resolveUserByMessengerUserID,
		ResolveTelegramMessengerUserID:        a.resolveTelegramMessengerUserID,
		ResolveConnectorChannel:               resolveConnectorChannelURL,
		ConnectorPeriodLabel:                  appConnectorPeriodLabel,
		RecurringCancelTitle:                  appRecurringCancelTitle,
		RecurringCancelSubsLoadFail:           appRecurringCancelSubsLoadFail,
		RecurringCancelMissingSub:             appRecurringCancelMissingSub,
		RecurringCancelAlreadyOff:             appRecurringCancelAlreadyOff,
		RecurringCancelPersistFailed:          appRecurringCancelPersistFailed,
		RecurringCancelNotification:           appRecurringCancelNotification,
		RecurringCancelSuccessForSubscription: appRecurringCancelSuccessForSubscription,
	}
}

func (a *application) recurringRebillService() *apprecurring.Service {
	return a.recurringService()
}
