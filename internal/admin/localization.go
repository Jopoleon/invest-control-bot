package admin

import (
	"net/http"
	"strings"
)

const (
	langRU = "ru"
	langEN = "en"
)

var translations = map[string]map[string]string{
	langRU: {
		"nav.connectors": "Коннекторы",
		"nav.events":     "События",
		"nav.guide":      "Гайд",
		"lang.ru":        "Рус",
		"lang.en":        "Eng",

		"connectors.title":            "Коннекторы",
		"connectors.subtitle":         "Коннектор - это тариф/условие оплаты, по которому пользователь приходит в бот.",
		"connectors.create":           "Создать коннектор",
		"connectors.hint_title":       "Подсказка",
		"connectors.table_title":      "Список коннекторов",
		"connectors.required":         "Обязательные: Name, Chat ID, Price RUB. Остальные поля опциональны.",
		"connectors.created":          "коннектор создан",
		"connectors.updated":          "статус коннектора обновлен",
		"connectors.bad_form":         "не удалось разобрать форму",
		"connector.validation.price":  "цена должна быть положительным числом",
		"connector.validation.period": "период должен быть положительным числом",

		"connectors.form.name":           "Название *",
		"connectors.form.chat_id":        "Chat ID *",
		"connectors.form.price_rub":      "Цена RUB *",
		"connectors.form.period_days":    "Период (дней)",
		"connectors.form.start_payload":  "Start payload",
		"connectors.form.internal_id":    "Внутренний ID",
		"connectors.form.offer_url":      "Ссылка оферты",
		"connectors.form.privacy_url":    "Ссылка политики",
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
		"events.filter.connector_id": "Connector ID",
		"events.filter.action":       "Action",
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

		"guide.title": "Гайд по админке",
		"guide.s1":    "1. Как создать коннектор",
		"guide.s1.p":  "Заполните Name, Chat ID и Price RUB. Нажмите кнопку создания.",
		"guide.s2":    "2. Поля формы",
		"guide.s2.p1": "Name: название тарифа, которое увидит пользователь.",
		"guide.s2.p2": "Chat ID: Telegram ID чата/канала в формате без минуса, например 1003626584986.",
		"guide.s2.p3": "Price RUB: сумма оплаты в рублях.",
		"guide.s2.p4": "Period days: длительность подписки в днях (по умолчанию 30).",
		"guide.s2.p5": "Start payload: токен deeplink (/start ...). Если пусто, генерируется автоматически.",
		"guide.s2.p6": "Internal ID: внутренний ID коннектора. Если пусто, генерируется автоматически.",
		"guide.s2.p7": "Offer URL и Privacy URL: ссылки на оферту и политику ПДн.",
		"guide.s2.p8": "Description: дополнительное описание тарифа.",
		"guide.s3":    "3. Как тестировать",
		"guide.s3.p1": "Скопируйте Bot Link из таблицы и откройте его в Telegram. Пройдите диалог и нажмите «Оплатить».",
		"guide.s3.p2": "Пока активен mock-провайдер, откроется тестовая страница оплаты.",
	},
	langEN: {
		"nav.connectors": "Connectors",
		"nav.events":     "Events",
		"nav.guide":      "Guide",
		"lang.ru":        "Rus",
		"lang.en":        "Eng",

		"connectors.title":            "Connectors",
		"connectors.subtitle":         "Connector defines tariff/payment conditions used in bot deep links.",
		"connectors.create":           "Create connector",
		"connectors.hint_title":       "Hint",
		"connectors.table_title":      "Connectors list",
		"connectors.required":         "Required: Name, Chat ID, Price RUB. Other fields are optional.",
		"connectors.created":          "connector created",
		"connectors.updated":          "connector status updated",
		"connectors.bad_form":         "failed to parse form",
		"connector.validation.price":  "price must be a positive number",
		"connector.validation.period": "period must be a positive number",

		"connectors.form.name":           "Name *",
		"connectors.form.chat_id":        "Chat ID *",
		"connectors.form.price_rub":      "Price RUB *",
		"connectors.form.period_days":    "Period days",
		"connectors.form.start_payload":  "Start payload",
		"connectors.form.internal_id":    "Internal ID",
		"connectors.form.offer_url":      "Offer URL",
		"connectors.form.privacy_url":    "Privacy URL",
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

		"guide.title": "Admin guide",
		"guide.s1":    "1. How to create connector",
		"guide.s1.p":  "Fill Name, Chat ID and Price RUB. Click create button.",
		"guide.s2":    "2. Form fields",
		"guide.s2.p1": "Name: tariff name visible to users.",
		"guide.s2.p2": "Chat ID: chat/channel Telegram ID without minus, e.g. 1003626584986.",
		"guide.s2.p3": "Price RUB: payment amount in RUB.",
		"guide.s2.p4": "Period days: subscription duration in days (default 30).",
		"guide.s2.p5": "Start payload: deeplink token (/start ...). Auto-generated when empty.",
		"guide.s2.p6": "Internal ID: connector internal ID. Auto-generated when empty.",
		"guide.s2.p7": "Offer URL and Privacy URL: links to legal documents.",
		"guide.s2.p8": "Description: additional tariff description.",
		"guide.s3":    "3. How to test",
		"guide.s3.p1": "Copy Bot Link from table, open in Telegram, complete flow and click Pay.",
		"guide.s3.p2": "While mock provider is enabled, test checkout page will be opened.",
	},
}

func (h *Handler) resolveLang(w http.ResponseWriter, r *http.Request) string {
	lang := normalizeLang(r.URL.Query().Get("lang"))
	if lang != "" {
		http.SetCookie(w, &http.Cookie{
			Name:     "admin_lang",
			Value:    lang,
			Path:     "/",
			HttpOnly: false,
			SameSite: http.SameSiteLaxMode,
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
