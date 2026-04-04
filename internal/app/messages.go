package app

import (
	"fmt"
	"strings"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
	"github.com/Jopoleon/invest-control-bot/internal/messenger"
)

const (
	appLegalTitleOffer                 = "Публичная оферта"
	appLegalTitlePrivacy               = "Политика обработки персональных данных"
	appLegalTitleAgreement             = "Пользовательское соглашение"
	appPaymentPageTitleSuccess         = "Оплата успешно завершена"
	appPaymentPageMessageSuccess       = "Платеж подтвержден. Доступ в канал активирован, а в боте придет сообщение с деталями."
	appPaymentPageTitleFail            = "Оплата не завершена"
	appPaymentPageMessageFail          = "Платеж был отменен или не прошел. Вернитесь в бота и попробуйте снова."
	appPaymentActionOpenBot            = "Открыть бота"
	appPaymentActionReturnToBot        = "Вернуться в бота"
	appPaymentActionOpenChannel        = "Открыть канал"
	appPaymentActionOpenTelegram       = "Открыть Telegram"
	appPaymentActionOpenMAXBot         = "Открыть бота в MAX"
	appPaymentActionReturnToMAXBot     = "Вернуться к боту в MAX"
	appPaymentActionOpenMAX            = "Открыть MAX"
	appPaymentActionOpenMAXChannel     = "Открыть канал в MAX"
	appPaymentActionReturnMAXChannel   = "Вернуться в канал в MAX"
	appPaymentSuccessChannelHint       = "\n\nНажмите кнопку ниже, чтобы перейти в канал."
	appPaymentActionMySubscription     = "Моя подписка"
	appPaymentFailedRecurringText      = "⚠️ Автоматическое списание не прошло. Чтобы не потерять доступ, оплатите подписку вручную по кнопке ниже."
	appPaymentFailedRecurringButton    = "Оплатить вручную"
	appSubscriptionRenewButton         = "Продлить подписку"
	appSubscriptionReminderCommandFmt  = "\n\nДля продления отправьте команду:\n/start %s"
	appSubscriptionExpiredText         = "⏰ Срок подписки истек. Чтобы восстановить доступ, оформите продление."
	appRecurringCheckoutTitle          = "Оформление подписки"
	appRecurringCheckoutHelperNote     = "Автоплатеж подключается во время новой оплаты этого тарифа. Уже действующая подписка не переводится на автосписания задним числом."
	appRecurringCheckoutTelegramCTA    = "Открыть бота в Telegram"
	appRecurringCheckoutMAXCTA         = "Открыть бота в MAX"
	appRecurringCheckoutMAXFallbackCTA = "Открыть MAX"
	appRecurringCheckoutBotCTA         = "Продолжить оформление в боте"
	appRecurringCheckoutMAXTitle       = "Продолжить в MAX"
	appRecurringCheckoutMAXHint        = "Если оформляете подписку в MAX, откройте бота по кнопке ниже. Если кнопка не сработает, откройте чат бота и отправьте команду:"
	appRecurringCheckoutConsentNote    = "Чекбокс не отмечен заранее. Автоплатеж включится только после вашего явного согласия на следующем шаге."
	appRecurringCancelTitle            = "Отключение автоплатежа"
	appRecurringCancelInvalidLink      = "Некорректная ссылка отключения автоплатежа."
	appRecurringCancelExpiredLink      = "Ссылка отключения автоплатежа истекла. Откройте новую ссылку из бота."
	appRecurringCancelInvalidRequest   = "Некорректный запрос отключения автоплатежа."
	appRecurringCancelNoSubscription   = "Не выбрана подписка для отключения автоплатежа."
	appRecurringCancelMissingSub       = "Подписка для отключения автоплатежа не найдена."
	appRecurringCancelAlreadyOff       = "Для этой подписки автоплатеж уже выключен."
	appRecurringCancelStaleSubmit      = "Эта страница была открыта на уже неактуальном состоянии подписки. Обновите страницу из бота, чтобы увидеть текущий статус автоплатежа."
	appRecurringCancelPersistFailed    = "Не удалось отключить автоплатеж для выбранной подписки. Попробуйте еще раз позже."
	appRecurringCancelSuccess          = "Автоплатеж отключен. Уже оплаченный период сохранится до конца срока подписки."
	appRecurringCancelStatusLoadFail   = "Не удалось загрузить статус автоплатежа."
	appRecurringCancelSubsLoadFail     = "Не удалось загрузить подписки."
	appRecurringCancelOpenTelegramBot  = "Открыть бота в Telegram"
	appRecurringCancelOpenMAXBot       = "Открыть бота в MAX"
)

func appPaymentSuccessMessage(paymentRow domain.Payment, connector domain.Connector, endsAt time.Time) string {
	lines := []string{"✅ Оплата прошла успешно."}

	if name := strings.TrimSpace(connector.Name); name != "" {
		lines = append(lines, "Подписка: "+name)
	}
	if paymentRow.AmountRUB > 0 {
		lines = append(lines, fmt.Sprintf("Сумма: %d ₽", paymentRow.AmountRUB))
	} else if connector.PriceRUB > 0 {
		lines = append(lines, fmt.Sprintf("Сумма: %d ₽", connector.PriceRUB))
	}
	if connector.PeriodMode != "" || connector.PeriodSeconds > 0 || connector.PeriodMonths > 0 || connector.FixedEndsAt != nil {
		periodLabel := strings.TrimSpace(appConnectorPeriodLabel(connector))
		if periodLabel != "" {
			lines = append(lines, "Период: "+periodLabel)
		}
	}
	lines = append(lines, "Доступ в канал активирован до "+endsAt.In(time.Local).Format("02.01.2006 15:04")+".")
	if paymentRow.AutoPayEnabled {
		lines = append(lines, "Автоплатеж для следующих списаний включен.")
	}
	return strings.Join(lines, "\n")
}

func appSubscriptionReminderMessage(endsAt time.Time) string {
	return fmt.Sprintf("🔔 Напоминание: подписка закончится %s. Чтобы продлить доступ, нажмите кнопку ниже.", endsAt.In(time.Local).Format("02.01.2006 15:04"))
}

func appSubscriptionExpiryNoticeMessage(endsAt time.Time) string {
	return fmt.Sprintf("⏰ Сегодня заканчивается подписка. Доступ будет отключен %s, если продление не поступит.", endsAt.In(time.Local).Format("02.01.2006 15:04"))
}

func appRecurringCancelNotification(connectorName string) string {
	if strings.TrimSpace(connectorName) == "" {
		return "🔁 Автоплатеж отключен через страницу управления подпиской. Новые автоматические списания больше не будут выполняться, а доступ сохранится до конца уже оплаченного периода."
	}
	return "🔁 Автоплатеж для подписки \"" + connectorName + "\" отключен через страницу управления. Новые автоматические списания для этого тарифа больше не будут выполняться, а доступ сохранится до конца уже оплаченного периода."
}

func appRecurringCancelSuccessForSubscription(connectorName string) string {
	if strings.TrimSpace(connectorName) == "" || connectorName == "1" {
		return appRecurringCancelSuccess
	}
	return "Автоплатеж для подписки \"" + connectorName + "\" отключен. Уже оплаченный период сохранится до конца срока подписки."
}

func appPaymentPageActions(kind messenger.Kind, success bool, channelURL, botURL string) []paymentPageAction {
	switch kind {
	case messenger.KindMAX:
		actions := make([]paymentPageAction, 0, 3)
		if success && strings.TrimSpace(channelURL) != "" {
			label := appPaymentActionOpenMAXChannel
			if !success {
				label = appPaymentActionReturnMAXChannel
			}
			actions = append(actions, paymentPageAction{Label: label, URL: channelURL})
		}
		if strings.TrimSpace(botURL) != "" {
			label := appPaymentActionOpenMAXBot
			if !success {
				label = appPaymentActionReturnToMAXBot
			}
			actions = append(actions, paymentPageAction{Label: label, URL: botURL, Secondary: len(actions) > 0})
		}
		if strings.TrimSpace(botURL) != "https://web.max.ru/" {
			actions = append(actions, paymentPageAction{Label: appPaymentActionOpenMAX, URL: "https://web.max.ru/", Secondary: len(actions) > 0})
		}
		return actions
	default:
		primaryLabel := appPaymentActionOpenBot
		if !success {
			primaryLabel = appPaymentActionReturnToBot
		}
		actions := []paymentPageAction{{Label: primaryLabel, URL: botURL}}
		if strings.TrimSpace(channelURL) != "" {
			actions = append(actions, paymentPageAction{Label: appPaymentActionOpenChannel, URL: channelURL, Secondary: true})
		}
		actions = append(actions, paymentPageAction{Label: appPaymentActionOpenTelegram, URL: "https://t.me", Secondary: len(actions) > 0})
		return actions
	}
}
