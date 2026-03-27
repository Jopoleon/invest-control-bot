package app

import (
	"fmt"
	"strings"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/messenger"
)

const (
	appLegalTitleOffer                = "Публичная оферта"
	appLegalTitlePrivacy              = "Политика обработки персональных данных"
	appLegalTitleAgreement            = "Пользовательское соглашение"
	appPaymentPageTitleSuccess        = "Оплата успешно завершена"
	appPaymentPageMessageSuccess      = "Платеж подтвержден. Подписка активируется автоматически, а в боте придет сообщение с деталями."
	appPaymentPageTitleFail           = "Оплата не завершена"
	appPaymentPageMessageFail         = "Платеж был отменен или не прошел. Вернитесь в бота и попробуйте снова."
	appPaymentActionOpenBot           = "Открыть бота"
	appPaymentActionReturnToBot       = "Вернуться в бота"
	appPaymentActionOpenChannel       = "Открыть канал"
	appPaymentActionOpenTelegram      = "Открыть Telegram"
	appPaymentActionOpenMAX           = "Открыть MAX Web"
	appPaymentActionOpenMAXChannel    = "Открыть канал MAX"
	appPaymentActionReturnMAXChannel  = "Вернуться в канал MAX"
	appPaymentSuccessChannelHint      = "\n\nНажмите кнопку ниже, чтобы перейти в канал и открыть кабинет."
	appPaymentActionMySubscription    = "Моя подписка"
	appPaymentFailedRecurringText     = "⚠️ Автоматическое списание не прошло. Чтобы не потерять доступ, оплатите подписку вручную по кнопке ниже."
	appPaymentFailedRecurringButton   = "Оплатить вручную"
	appSubscriptionRenewButton        = "Продлить подписку"
	appSubscriptionReminderCommandFmt = "\n\nДля продления отправьте команду:\n/start %s"
	appSubscriptionExpiredText        = "⏰ Срок подписки истек. Чтобы восстановить доступ, оформите продление."
	appRecurringCheckoutTitle         = "Оформление подписки"
	appRecurringCheckoutHelperNote    = "Автоплатеж подключается во время новой оплаты этого тарифа. Уже действующая подписка не переводится на автосписания задним числом."
	appRecurringCancelTitle           = "Отключение автоплатежа"
	appRecurringCancelInvalidLink     = "Некорректная ссылка отключения автоплатежа."
	appRecurringCancelExpiredLink     = "Ссылка отключения автоплатежа истекла. Откройте новую ссылку из бота."
	appRecurringCancelInvalidRequest  = "Некорректный запрос отключения автоплатежа."
	appRecurringCancelNoSubscription  = "Не выбрана подписка для отключения автоплатежа."
	appRecurringCancelMissingSub      = "Подписка для отключения автоплатежа не найдена."
	appRecurringCancelAlreadyOff      = "Для этой подписки автоплатеж уже выключен."
	appRecurringCancelPersistFailed   = "Не удалось отключить автоплатеж для выбранной подписки. Попробуйте еще раз позже."
	appRecurringCancelSuccess         = "Автоплатеж отключен. Уже оплаченный период сохранится до конца срока подписки."
	appRecurringCancelStatusLoadFail  = "Не удалось загрузить статус автоплатежа."
	appRecurringCancelSubsLoadFail    = "Не удалось загрузить подписки."
)

func appPaymentSuccessMessage(endsAt time.Time) string {
	return fmt.Sprintf("✅ Оплата прошла успешно. Подписка активирована до %s.", endsAt.In(time.Local).Format("02.01.2006 15:04"))
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
		actions := make([]paymentPageAction, 0, 2)
		if strings.TrimSpace(channelURL) != "" {
			label := appPaymentActionOpenMAXChannel
			if !success {
				label = appPaymentActionReturnMAXChannel
			}
			actions = append(actions, paymentPageAction{Label: label, URL: channelURL})
		}
		actions = append(actions, paymentPageAction{Label: appPaymentActionOpenMAX, URL: "https://web.max.ru/", Secondary: len(actions) > 0})
		return actions
	default:
		primaryLabel := appPaymentActionOpenBot
		if !success {
			primaryLabel = appPaymentActionReturnToBot
		}
		return []paymentPageAction{
			{Label: primaryLabel, URL: botURL},
			{Label: appPaymentActionOpenChannel, URL: channelURL, Secondary: true},
			{Label: appPaymentActionOpenTelegram, URL: "https://t.me"},
		}
	}
}
