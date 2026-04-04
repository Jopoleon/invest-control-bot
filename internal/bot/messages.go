package bot

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
	"github.com/Jopoleon/invest-control-bot/internal/messenger"
)

const (
	botMsgConnectorUnavailable                    = "Коннектор не найден или отключен."
	botMsgStartUsage                              = "Нужна ссылка вида /start <connector_payload>."
	botBtnAcceptTerms                             = "Принимаю условия"
	botMsgEmptyValue                              = "Пустое значение. Попробуйте еще раз."
	botMsgInvalidPhone                            = "⚠️ Не правильный телефон. Введите номер в международном формате."
	botMsgInvalidEmail                            = "⚠️ Неправильный e-mail"
	botPromptFullName                             = "ФИО"
	botPromptPhone                                = "Телефон"
	botPromptEmail                                = "E-mail"
	botPromptTelegramUsername                     = "Ник в мессенджере"
	botMsgPaymentProviderNotConfigured            = "Платежный провайдер пока не настроен."
	botMsgPaymentProfilePrepareFailed             = "Не удалось подготовить профиль пользователя для оплаты. Попробуйте позже."
	botMsgPaymentLinkFailed                       = "Не удалось сформировать ссылку оплаты. Попробуйте позже."
	botMsgAutopayDocsMissing                      = "Автоплатеж пока недоступен: не найдены обязательные документы для согласия."
	botMsgAutopayConsentSaveFailed                = "Не удалось сохранить согласие на автоплатеж. Попробуйте еще раз."
	botBtnGoToPayment                             = "Перейти к оплате"
	botMenuButtonSubscription                     = "📄 Моя подписка"
	botMenuButtonPayments                         = "💳 Платежи"
	botMenuButtonAutopay                          = "🔁 Автоплатеж"
	botMenuTitle                                  = "Личный кабинет:"
	botMsgUnknownMenuCommand                      = "Неизвестная команда меню."
	botMsgSubscriptionLoadFailed                  = "Не удалось загрузить подписку. Попробуйте позже."
	botMsgNoActiveSubscriptions                   = "У вас нет активных подписок."
	botMenuSubscriptionHeader                     = "📄 Моя подписка"
	botMenuPaymentsHeader                         = "💳 Последние платежи"
	botMsgPaymentsLoadFailed                      = "Не удалось загрузить платежи. Попробуйте позже."
	botMsgNoPayments                              = "У вас пока нет платежей."
	botMsgAutopayUnavailable                      = "Автоплатеж временно недоступен. Подключим его после активации recurring в Robokassa."
	botMsgAutopayEnableOnlyDuringPayment          = "Автоплатеж включается на этапе оплаты через отдельное согласие на автоматические списания. При следующей оплате выберите режим с автоплатежом."
	botMsgAutopayPreferenceUpdateFailed           = "Не удалось изменить настройку автоплатежа."
	botMsgAutopayDisabledShort                    = "Автоплатеж выключен."
	botMsgAutopayDisableConfirm                   = "🔁 Отключить автоплатеж?\n\nПосле отключения новые автоматические списания выполняться не будут. Текущий оплаченный период сохранится до даты окончания подписки."
	botBtnAutopayDisableConfirm                   = "Да, отключить"
	botBtnAutopayDisableCancel                    = "Нет, оставить"
	botMsgAutopayDisableSubscriptionsFailed       = "Не удалось отключить автоплатеж для активных подписок."
	botMsgAutopayDisabled                         = "🔁 Автоплатеж отключен.\n\nНовые автоматические списания больше не будут выполняться. Доступ сохранится до конца уже оплаченного периода."
	botMsgAutopayChooser                          = "🔁 Выберите подписку\n\nДля каждой подписки доступно свое действие: повторное включение без оплаты или оформление автоплатежа для будущих периодов."
	botMsgAutopayReactivationResolveFailed        = "Не удалось определить подписку для повторного включения автоплатежа."
	botMsgAutopaySubscriptionNotFound             = "Подписка не найдена."
	botMsgAutopayReactivationOnlyForActive        = "Автоплатеж можно включить только для активной подписки."
	botMsgAutopayReactivationUnavailable          = "Для этой подписки нельзя включить автоплатеж без новой оплаты."
	botMsgTariffNotFound                          = "Тариф не найден."
	botMsgAutopayConsentConfirmFailed             = "Не удалось подтвердить согласие на автоплатеж."
	botMsgAutopayConsentPersistFailed             = "Не удалось сохранить согласие на автоплатеж."
	botMsgAutopayReactivationFailed               = "Не удалось включить автоплатеж для подписки."
	botMsgAutopayReactivated                      = "🔁 Автоплатеж снова включен.\n\nДля этой активной подписки будущие списания снова будут выполняться автоматически. Повторная оплата не потребовалась."
	botMsgAutopayDisableResolveFailed             = "Не удалось определить подписку для отключения автоплатежа."
	botMsgAutopayDisableOnlyForActive             = "Автоплатеж можно отключить только для активной подписки."
	botMsgAutopayDisablePerSubscriptionFailed     = "Не удалось отключить автоплатеж для подписки."
	botMsgAutopayCancelPageHint                   = "На публичной странице можно отключить автоплатеж для конкретной подписки."
	botBtnAutopayCancelPage                       = "Страница отключения"
	botBtnAutopayManageSubscriptions              = "Управлять подписками"
	botMsgAutopayNoneEnabled                      = "Автоплатеж сейчас не включен ни для одной активной подписки."
	botMsgAutopayChooseSubscription               = "Выберите подписку, для которой хотите включить или настроить автоплатеж."
	botBtnAutopayChooseSubscription               = "Выбрать подписку"
	botBtnBack                                    = "Назад"
	botMsgExistingSubscriptionAutopayEnabled      = "\n\nАвтоплатеж для этого тарифа уже включен."
	botMsgExistingSubscriptionAutopayDisabledHint = "\n\nАвтоплатеж для этого тарифа сейчас выключен, но его можно включить обратно без повторной оплаты."
	botBtnAutopayEnableAgain                      = "Включить автоплатеж обратно"
	botMsgCheckoutBase                            = "✅ Спасибо! Ваша заявка оформлена успешно.\n💳 Осталось оплатить\nЧтобы произвести оплату, нажмите на кнопку «Оплатить» ниже, для переадресации на платежную страницу"
	botMsgCheckoutAutopayDisabled                 = "\n\n☐ Автоплатеж выключен.\nЕсли хотите подключить автоматические списания, подтвердите согласие кнопкой ниже."
	botBtnCheckoutAutopayOff                      = "☐ Я согласен на автоматические списания"
	botBtnCheckoutAutopayOn                       = "☑ Я согласен на автоматические списания"
	botBtnCheckoutPay                             = "Оплатить"
	botBtnCheckoutPayWithAutopay                  = "Оплатить и включить автоплатеж"
	botMsgPayConsentToggleFailed                  = "Не удалось обновить настройку автоплатежа."
)

func botStartCardText(connector domain.Connector, offerURL, privacyURL string) string {
	lines := []string{
		connector.Name,
		connector.Description,
		fmt.Sprintf("⚡️ Подписка: %d ₽", connector.PriceRUB),
		fmt.Sprintf("Период оплаты: %s", botConnectorPeriodLabel(connector)),
		"Чтобы продолжить, вам необходимо принять условия публичной оферты и политики обработки персональных данных.",
	}
	if strings.TrimSpace(offerURL) != "" {
		lines = append(lines, "Оферта: "+strings.TrimSpace(offerURL))
	}
	if strings.TrimSpace(privacyURL) != "" {
		lines = append(lines, "Политика ПДн: "+strings.TrimSpace(privacyURL))
	}
	return strings.Join(lines, "\n")
}

func botConnectorAccessMismatchWarning(connector domain.Connector, kind messenger.Kind) string {
	currentKind := messengerKindFromIdentity(kind)
	if connector.HasAccessFor(currentKind) || !connector.HasAnyAccessDestination() {
		return ""
	}

	switch currentKind {
	case domain.MessengerKindMAX:
		if connector.HasAccessFor(domain.MessengerKindTelegram) {
			return "⚠️ Этот тариф выдает доступ только в Telegram.\nОформление в MAX для него недоступно. Откройте тот же тариф в Telegram или попросите администратора прислать Telegram-ссылку."
		}
	default:
		if connector.HasAccessFor(domain.MessengerKindMAX) {
			return "⚠️ Этот тариф выдает доступ только в MAX.\nОформление в Telegram для него недоступно. Откройте тот же тариф в MAX или попросите администратора прислать MAX-ссылку."
		}
	}
	return "⚠️ Для этого тарифа не настроен доступ в текущем мессенджере. Попросите администратора прислать правильную ссылку."
}

func botPaymentLinkCreated(provider string, testMode bool) string {
	if testMode {
		return fmt.Sprintf("Сформирована ссылка оплаты через %s в тестовом режиме.", provider)
	}
	return fmt.Sprintf("Сформирована ссылка оплаты через %s.", provider)
}

func botSubscriptionOverviewLines(sub domain.Subscription, connector domain.Connector, channel string) []string {
	lines := []string{
		fmt.Sprintf("• %s", connector.Name),
		fmt.Sprintf("  Сумма: %d ₽", connector.PriceRUB),
		fmt.Sprintf("  Период: %s", botConnectorPeriodLabel(connector)),
		fmt.Sprintf("  Действует до: %s", sub.EndsAt.In(time.Local).Format("02.01.2006 15:04")),
	}
	if sub.AutoPayEnabled {
		lines = append(lines, "  Автоплатеж: включен")
	} else {
		lines = append(lines, "  Автоплатеж: выключен")
	}
	if channel != "" {
		lines = append(lines, fmt.Sprintf("  Канал: %s", channel))
	}
	return lines
}

func botSubscriptionAccessLines(connector domain.Connector, kind messenger.Kind) []string {
	currentKind := messengerKindFromIdentity(kind)
	if channel := connector.AccessURL(currentKind); channel != "" {
		return []string{fmt.Sprintf("  Канал: %s", channel)}
	}

	switch currentKind {
	case domain.MessengerKindMAX:
		if telegramURL := connector.TelegramAccessURL(); telegramURL != "" {
			return []string{
				"  Доступ: этот тариф выдается в Telegram",
				fmt.Sprintf("  Telegram: %s", telegramURL),
			}
		}
	default:
		if maxURL := connector.MAXAccessURL(); maxURL != "" {
			return []string{
				"  Доступ: этот тариф выдается в MAX",
				fmt.Sprintf("  MAX: %s", maxURL),
			}
		}
	}
	return nil
}

func botConnectorPeriodLabel(connector domain.Connector) string {
	if fixedEndsAt, ok := connector.FixedDeadline(); ok {
		return "до " + fixedEndsAt.In(time.Local).Format("02.01.2006 15:04")
	}
	if months, ok := connector.CalendarMonthsPeriod(); ok {
		return fmt.Sprintf("%d мес.", months)
	}
	if duration, ok := connector.DurationPeriod(); ok {
		return formatBotDurationLabel(duration)
	}
	return "30 дн."
}

func formatBotDurationLabel(duration time.Duration) string {
	seconds := int64(duration / time.Second)
	switch {
	case seconds%(24*60*60) == 0:
		return fmt.Sprintf("%d дн.", seconds/(24*60*60))
	case seconds%(60*60) == 0:
		return fmt.Sprintf("%d ч.", seconds/(60*60))
	case seconds%60 == 0:
		return fmt.Sprintf("%d мин.", seconds/60)
	default:
		return fmt.Sprintf("%d сек.", seconds)
	}
}

func botPaymentHistoryLines(payment domain.Payment) []string {
	paid := "-"
	if payment.PaidAt != nil {
		paid = payment.PaidAt.In(time.Local).Format("02.01.2006 15:04")
	}
	return []string{
		fmt.Sprintf("• #%d — %d ₽ — %s", payment.ID, payment.AmountRUB, strings.ToUpper(string(payment.Status))),
		fmt.Sprintf("  Создан: %s", payment.CreatedAt.In(time.Local).Format("02.01.2006 15:04")),
		fmt.Sprintf("  Оплачен: %s", paid),
	}
}

func botAutopayInfoMessage(enabledCount, totalCount int, cancelURL string) (string, [][]messenger.ActionButton) {
	text := "🔁 Автоплатеж\n\n"
	if enabledCount > 0 {
		text += fmt.Sprintf("Автоплатеж включен для %d из %d активных подписок.", enabledCount, totalCount)
		rows := make([][]messenger.ActionButton, 0, 2)
		if strings.TrimSpace(cancelURL) != "" {
			text += "\n\n" + botMsgAutopayCancelPageHint
			rows = append(rows, []messenger.ActionButton{buttonURL(botBtnAutopayCancelPage, cancelURL)})
		}
		rows = append(rows, []messenger.ActionButton{buttonAction(botBtnAutopayManageSubscriptions, menuCallbackAutopayPick)})
		return text, rows
	}
	text += botMsgAutopayNoneEnabled
	if totalCount == 0 {
		return text, nil
	}
	text += "\n\n" + botMsgAutopayChooseSubscription
	return text, [][]messenger.ActionButton{{buttonAction(botBtnAutopayChooseSubscription, menuCallbackAutopayPick)}}
}

func autopayInfoMessage(enabledCount, totalCount int, cancelURL string) (string, [][]messenger.ActionButton) {
	return botAutopayInfoMessage(enabledCount, totalCount, cancelURL)
}

func botAutopayChooserButtonDisable(name string) string {
	return "Отключить: " + name
}

func botAutopayChooserButtonReenable(name string) string {
	return "Включить обратно: " + name
}

func botAutopayChooserButtonCheckout(name string) string {
	return "Оформить автоплатеж: " + name
}

func botAutopayDisabledForSubscription(connectorName string) string {
	text := "🔁 Автоплатеж отключен."
	if strings.TrimSpace(connectorName) != "" {
		text = "🔁 Автоплатеж отключен для подписки «" + connectorName + "»."
	}
	return text + "\n\nНовые автоматические списания для этого тарифа больше не будут выполняться. Доступ сохранится до конца уже оплаченного периода."
}

func botExistingSubscriptionText(connectorName string, endsAt time.Time) string {
	return fmt.Sprintf("У вас уже есть активная подписка «%s» до %s.", connectorName, endsAt.In(time.Local).Format("02.01.2006 15:04"))
}

func botCheckoutAutopayEnabled(offerURL, agreementURL string) string {
	lines := []string{
		"",
		"☑️ Автоплатеж будет включен для следующих списаний.",
		"Согласие действует по оферте и пользовательскому соглашению.",
	}
	if strings.TrimSpace(offerURL) != "" {
		lines = append(lines, "Оферта: "+strings.TrimSpace(offerURL))
	}
	if strings.TrimSpace(agreementURL) != "" {
		lines = append(lines, "Пользовательское соглашение: "+strings.TrimSpace(agreementURL))
	}
	return strings.Join(lines, "\n")
}

func registrationPrompt(step domain.RegistrationStep) string {
	switch step {
	case domain.StepFullName:
		return botPromptFullName
	case domain.StepPhone:
		return botPromptPhone
	case domain.StepEmail:
		return botPromptEmail
	case domain.StepUsername:
		return botPromptTelegramUsername
	default:
		return ""
	}
}

func paymentKeyboard(connectorID int64, recurringOptIn, canOfferRecurring bool) [][]messenger.ActionButton {
	rows := make([][]messenger.ActionButton, 0, 2)
	if canOfferRecurring {
		toggleText := botBtnCheckoutAutopayOff
		toggleState := "on"
		if recurringOptIn {
			toggleText = botBtnCheckoutAutopayOn
			toggleState = "off"
		}
		rows = append(rows, []messenger.ActionButton{
			buttonAction(toggleText, payConsentCallbackPrefix+strconv.FormatInt(connectorID, 10)+":"+toggleState),
		})
	}

	payText := botBtnCheckoutPay
	payMode := "0"
	if recurringOptIn && canOfferRecurring {
		payText = botBtnCheckoutPayWithAutopay
		payMode = "1"
	}
	rows = append(rows, []messenger.ActionButton{{
		Text:   payText,
		Action: "pay:" + strconv.FormatInt(connectorID, 10) + ":" + payMode,
	}})

	return rows
}
