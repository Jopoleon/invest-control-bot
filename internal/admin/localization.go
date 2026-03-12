package admin

import (
	"net/http"
	"strings"
	"time"
)

const (
	langRU = "ru"
	langEN = "en"
)

var translations = map[string]map[string]string{
	langRU: {
		"nav.connectors": "Коннекторы",
		"nav.billing":    "Биллинг",
		"nav.events":     "События",
		"nav.guide":      "Гайд",
		"nav.logout":     "Выйти",
		"lang.ru":        "Рус",
		"lang.en":        "Eng",

		"login.title":        "Вход в админку",
		"login.subtitle":     "Введите ADMIN_AUTH_TOKEN для доступа к панели.",
		"login.token":        "Токен",
		"login.submit":       "Войти",
		"login.hint":         "Для локальной разработки используйте значение ADMIN_AUTH_TOKEN из .env.",
		"login.bad_form":     "не удалось разобрать форму",
		"login.bad_token":    "неверный токен",
		"login.rate_limited": "слишком много неудачных попыток, попробуйте позже",
		"csrf.invalid":       "сессия формы устарела, обновите страницу и попробуйте снова",

		"connectors.title":                 "Коннекторы",
		"connectors.subtitle":              "Коннектор - это тариф/условие оплаты, по которому пользователь приходит в бот.",
		"connectors.create":                "Создать коннектор",
		"connectors.hint_title":            "Подсказка",
		"connectors.table_title":           "Список коннекторов",
		"connectors.required":              "Обязательные: Name, Price RUB. Из пары Chat ID / Channel URL заполните хотя бы одно.",
		"connectors.created":               "коннектор создан",
		"connectors.updated":               "статус коннектора обновлен",
		"connectors.bad_form":              "не удалось разобрать форму",
		"connector.validation.price":       "цена должна быть положительным числом",
		"connector.validation.period":      "период должен быть положительным числом",
		"connector.validation.chat_or_url": "нужно заполнить Chat ID или Channel URL",

		"connectors.form.name":           "Название *",
		"connectors.form.chat_id":        "Chat ID *",
		"connectors.form.price_rub":      "Цена RUB *",
		"connectors.form.period_days":    "Период (дней)",
		"connectors.form.start_payload":  "Start payload",
		"connectors.form.internal_id":    "Внутренний ID",
		"connectors.form.offer_url":      "Ссылка оферты",
		"connectors.form.privacy_url":    "Ссылка политики",
		"connectors.form.channel_url":    "Ссылка канала",
		"connectors.form.description":    "Описание",
		"connectors.form.description_ph": "Краткое описание тарифа",
		"connectors.form.price_ph":       "3335",
		"connectors.form.payload_hint":   "Если пусто, будет сгенерирован автоматически.",
		"connectors.form.id_hint":        "Если пусто, будет сгенерирован автоматически.",

		"connectors.table.id":       "ID",
		"connectors.table.payload":  "Payload",
		"connectors.table.name":     "Название",
		"connectors.table.chat":     "Chat",
		"connectors.table.price":    "Цена",
		"connectors.table.period":   "Период",
		"connectors.table.offer":    "Оферта",
		"connectors.table.privacy":  "Политика",
		"connectors.table.channel":  "Канал",
		"connectors.table.bot_link": "Ссылка бота",
		"connectors.table.active":   "Активен",
		"connectors.table.action":   "Действие",
		"connectors.table.enable":   "Включить",
		"connectors.table.disable":  "Выключить",

		"events.title":      "События аудита",
		"events.subtitle":   "Фильтрация, сортировка и пагинация действий пользователей.",
		"events.load_error": "не удалось загрузить события",
		"events.apply":      "Применить",
		"events.reset":      "Сброс",
		"events.total":      "Всего",
		"events.page":       "Страница",
		"events.first":      "Первая",
		"events.prev":       "Назад",
		"events.next":       "Вперед",
		"events.last":       "Последняя",

		"events.filter.telegram_id":  "Telegram ID",
		"events.filter.connector_id": "ID коннектора",
		"events.filter.action":       "Действие",
		"events.filter.search":       "Поиск в details",
		"events.filter.date_from":    "Дата от",
		"events.filter.date_to":      "Дата до",
		"events.filter.sort_by":      "Сортировка по",
		"events.filter.sort_dir":     "Направление",
		"events.filter.page_size":    "Размер страницы",
		"events.sort.asc":            "по возрастанию",
		"events.sort.desc":           "по убыванию",

		"events.table.time":        "Время",
		"events.table.telegram_id": "Telegram ID",
		"events.table.connector":   "Коннектор",
		"events.table.action":      "Действие",
		"events.table.details":     "Детали",

		"billing.title":                        "Платежи и подписки",
		"billing.subtitle":                     "Операционный срез по оплатам и выданным подпискам.",
		"billing.load_error":                   "не удалось загрузить платежи/подписки",
		"billing.apply":                        "Применить",
		"billing.reset":                        "Сброс",
		"billing.filter.telegram_id":           "Telegram ID",
		"billing.filter.connector_id":          "ID коннектора",
		"billing.filter.payment_status":        "Статус платежа",
		"billing.filter.subscription_status":   "Статус подписки",
		"billing.filter.date_from":             "Дата от",
		"billing.filter.date_to":               "Дата до",
		"billing.payment_status.any":           "Любой",
		"billing.payment_status.pending":       "pending",
		"billing.payment_status.paid":          "paid",
		"billing.payment_status.failed":        "failed",
		"billing.sub_status.any":               "Любой",
		"billing.sub_status.active":            "active",
		"billing.sub_status.expired":           "expired",
		"billing.sub_status.revoked":           "revoked",
		"billing.payments.title":               "Платежи",
		"billing.subscriptions.title":          "Подписки",
		"billing.table.payment.id":             "ID",
		"billing.table.payment.provider":       "Провайдер",
		"billing.table.payment.status":         "Статус",
		"billing.table.payment.autopay":        "Автоплатеж",
		"billing.table.payment.telegram":       "Telegram",
		"billing.table.payment.connector":      "Коннектор",
		"billing.table.payment.amount":         "Сумма",
		"billing.table.payment.created":        "Создан",
		"billing.table.payment.paid_at":        "Оплачен",
		"billing.table.subscription.id":        "ID",
		"billing.table.subscription.status":    "Статус",
		"billing.table.subscription.autopay":   "Автоплатеж",
		"billing.table.subscription.telegram":  "Telegram",
		"billing.table.subscription.connector": "Коннектор",
		"billing.table.subscription.payment":   "Платеж",
		"billing.table.subscription.starts":    "Начало",
		"billing.table.subscription.ends":      "Окончание",
		"billing.table.subscription.created":   "Создана",

		"guide.title": "Гайд по админке",
		"guide.s1":    "1. Как создать коннектор",
		"guide.s1.p":  "Заполните Name, Chat ID и Price RUB. Нажмите кнопку создания.",
		"guide.s2":    "2. Поля формы",
		"guide.s2.p1": "Name: название тарифа, которое увидит пользователь.",
		"guide.s2.p2": "Chat ID: Telegram ID чата/канала в формате без минуса, например 1003626584986.",
		"guide.s2.p3": "Price RUB: сумма оплаты в рублях.",
		"guide.s2.p4": "Period days: длительность подписки в днях (по умолчанию 30).",
		"guide.s2.p5": "Start payload: токен deeplink (/start ...). Если пусто, генерируется автоматически.",
		"guide.s2.p6": "ID коннектора: автоинкрементный числовой ID в БД, заполнять вручную не нужно.",
		"guide.s2.p7": "Offer URL, Privacy URL и Channel URL: ссылки на оферту, политику ПДн и канал для перехода после оплаты.",
		"guide.s2.p8": "Description: дополнительное описание тарифа.",
		"guide.s3":    "3. Как тестировать",
		"guide.s3.p1": "Скопируйте Bot Link из таблицы и откройте его в Telegram. Пройдите диалог и нажмите «Оплатить».",
		"guide.s3.p2": "Пока активен mock-провайдер, откроется тестовая страница оплаты.",
	},
	langEN: {
		"nav.connectors": "Connectors",
		"nav.billing":    "Billing",
		"nav.events":     "Events",
		"nav.guide":      "Guide",
		"nav.logout":     "Logout",
		"lang.ru":        "Rus",
		"lang.en":        "Eng",

		"login.title":        "Admin login",
		"login.subtitle":     "Enter ADMIN_AUTH_TOKEN to access admin panel.",
		"login.token":        "Token",
		"login.submit":       "Sign in",
		"login.hint":         "For local development use ADMIN_AUTH_TOKEN value from .env.",
		"login.bad_form":     "failed to parse form",
		"login.bad_token":    "invalid token",
		"login.rate_limited": "too many failed attempts, try again later",
		"csrf.invalid":       "form session expired, refresh page and try again",

		"connectors.title":                 "Connectors",
		"connectors.subtitle":              "Connector defines tariff/payment conditions used in bot deep links.",
		"connectors.create":                "Create connector",
		"connectors.hint_title":            "Hint",
		"connectors.table_title":           "Connectors list",
		"connectors.required":              "Required: Name, Price RUB. Fill at least one of Chat ID / Channel URL.",
		"connectors.created":               "connector created",
		"connectors.updated":               "connector status updated",
		"connectors.bad_form":              "failed to parse form",
		"connector.validation.price":       "price must be a positive number",
		"connector.validation.period":      "period must be a positive number",
		"connector.validation.chat_or_url": "either Chat ID or Channel URL is required",

		"connectors.form.name":           "Name *",
		"connectors.form.chat_id":        "Chat ID *",
		"connectors.form.price_rub":      "Price RUB *",
		"connectors.form.period_days":    "Period days",
		"connectors.form.start_payload":  "Start payload",
		"connectors.form.internal_id":    "Internal ID",
		"connectors.form.offer_url":      "Offer URL",
		"connectors.form.privacy_url":    "Privacy URL",
		"connectors.form.channel_url":    "Channel URL",
		"connectors.form.description":    "Description",
		"connectors.form.description_ph": "Short tariff description",
		"connectors.form.price_ph":       "3335",
		"connectors.form.payload_hint":   "Auto-generated if empty.",
		"connectors.form.id_hint":        "Auto-generated if empty.",

		"connectors.table.id":       "ID",
		"connectors.table.payload":  "Payload",
		"connectors.table.name":     "Name",
		"connectors.table.chat":     "Chat",
		"connectors.table.price":    "Price",
		"connectors.table.period":   "Period",
		"connectors.table.offer":    "Offer",
		"connectors.table.privacy":  "Privacy",
		"connectors.table.channel":  "Channel",
		"connectors.table.bot_link": "Bot link",
		"connectors.table.active":   "Active",
		"connectors.table.action":   "Action",
		"connectors.table.enable":   "Enable",
		"connectors.table.disable":  "Disable",

		"events.title":      "Audit events",
		"events.subtitle":   "Filter, sort and paginate user actions.",
		"events.load_error": "failed to load events",
		"events.apply":      "Apply",
		"events.reset":      "Reset",
		"events.total":      "Total",
		"events.page":       "Page",
		"events.first":      "First",
		"events.prev":       "Prev",
		"events.next":       "Next",
		"events.last":       "Last",

		"events.filter.telegram_id":  "Telegram ID",
		"events.filter.connector_id": "Connector ID",
		"events.filter.action":       "Action",
		"events.filter.search":       "Search in details",
		"events.filter.date_from":    "Date from",
		"events.filter.date_to":      "Date to",
		"events.filter.sort_by":      "Sort by",
		"events.filter.sort_dir":     "Sort dir",
		"events.filter.page_size":    "Page size",
		"events.sort.asc":            "ascending",
		"events.sort.desc":           "descending",

		"events.table.time":        "Time",
		"events.table.telegram_id": "Telegram ID",
		"events.table.connector":   "Connector",
		"events.table.action":      "Action",
		"events.table.details":     "Details",

		"billing.title":                        "Payments and subscriptions",
		"billing.subtitle":                     "Operational view of payment attempts and granted subscriptions.",
		"billing.load_error":                   "failed to load payments/subscriptions",
		"billing.apply":                        "Apply",
		"billing.reset":                        "Reset",
		"billing.filter.telegram_id":           "Telegram ID",
		"billing.filter.connector_id":          "Connector ID",
		"billing.filter.payment_status":        "Payment status",
		"billing.filter.subscription_status":   "Subscription status",
		"billing.filter.date_from":             "Date from",
		"billing.filter.date_to":               "Date to",
		"billing.payment_status.any":           "Any",
		"billing.payment_status.pending":       "pending",
		"billing.payment_status.paid":          "paid",
		"billing.payment_status.failed":        "failed",
		"billing.sub_status.any":               "Any",
		"billing.sub_status.active":            "active",
		"billing.sub_status.expired":           "expired",
		"billing.sub_status.revoked":           "revoked",
		"billing.payments.title":               "Payments",
		"billing.subscriptions.title":          "Subscriptions",
		"billing.table.payment.id":             "ID",
		"billing.table.payment.provider":       "Provider",
		"billing.table.payment.status":         "Status",
		"billing.table.payment.autopay":        "Autopay",
		"billing.table.payment.telegram":       "Telegram",
		"billing.table.payment.connector":      "Connector",
		"billing.table.payment.amount":         "Amount",
		"billing.table.payment.created":        "Created",
		"billing.table.payment.paid_at":        "Paid at",
		"billing.table.subscription.id":        "ID",
		"billing.table.subscription.status":    "Status",
		"billing.table.subscription.autopay":   "Autopay",
		"billing.table.subscription.telegram":  "Telegram",
		"billing.table.subscription.connector": "Connector",
		"billing.table.subscription.payment":   "Payment",
		"billing.table.subscription.starts":    "Starts",
		"billing.table.subscription.ends":      "Ends",
		"billing.table.subscription.created":   "Created",

		"guide.title": "Admin guide",
		"guide.s1":    "1. How to create connector",
		"guide.s1.p":  "Fill Name, Chat ID and Price RUB. Click create button.",
		"guide.s2":    "2. Form fields",
		"guide.s2.p1": "Name: tariff name visible to users.",
		"guide.s2.p2": "Chat ID: chat/channel Telegram ID without minus, e.g. 1003626584986.",
		"guide.s2.p3": "Price RUB: payment amount in RUB.",
		"guide.s2.p4": "Period days: subscription duration in days (default 30).",
		"guide.s2.p5": "Start payload: deeplink token (/start ...). Auto-generated when empty.",
		"guide.s2.p6": "Connector ID: auto-increment numeric ID in DB, no manual input needed.",
		"guide.s2.p7": "Offer URL, Privacy URL and Channel URL: links to legal docs and destination channel after payment.",
		"guide.s2.p8": "Description: additional tariff description.",
		"guide.s3":    "3. How to test",
		"guide.s3.p1": "Copy Bot Link from table, open in Telegram, complete flow and click Pay.",
		"guide.s3.p2": "While mock provider is enabled, test checkout page will be opened.",
	},
}

func (h *Handler) resolveLang(w http.ResponseWriter, r *http.Request) string {
	lang := normalizeLang(r.URL.Query().Get("lang"))
	if lang != "" {
		h.setCookie(w, r, http.Cookie{
			Name:     "admin_lang",
			Value:    lang,
			Path:     "/",
			HttpOnly: false,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   int((30 * 24 * time.Hour).Seconds()),
		})
		return lang
	}
	if c, err := r.Cookie("admin_lang"); err == nil {
		if normalized := normalizeLang(c.Value); normalized != "" {
			return normalized
		}
	}
	return langRU
}

func normalizeLang(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case langEN:
		return langEN
	case langRU:
		return langRU
	default:
		return ""
	}
}

func dictForLang(lang string) map[string]string {
	if d, ok := translations[lang]; ok {
		return d
	}
	return translations[langRU]
}

func t(lang, key string) string {
	d := dictForLang(lang)
	if v, ok := d[key]; ok {
		return v
	}
	if v, ok := translations[langRU][key]; ok {
		return v
	}
	return key
}
