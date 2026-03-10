# telega-bot-fedor

Сервис управления подпиской и доступом в Telegram-чаты.

## Текущий статус
- Выполнена итерация 0: базовая структура проекта, стратегия конфигурации и секретов, фиксация окружений.
- Выполнена итерация 2 (MVP-часть): коннекторы в админке, `/start payload`, акцепт условий и FSM регистрации.
- Telegram stack переведен на `github.com/go-telegram/bot` (webhook на нашем Go-сервере).

## Структура
- `cmd/server` - точка входа backend сервиса.
- `internal/config` - загрузка и валидация конфигурации.
- `internal/app` - HTTP-сервер и маршрутизация.
- `internal/admin` - минимальная админка (коннекторы).
- `internal/bot` - обработчик Telegram flow.
- `internal/store/memory` - временное in-memory хранилище.
- `migrations` - SQL-миграции БД.
- `web` - шаблоны и статические файлы админки.
- `docs` - техническая документация.

## Окружения
- `local` - разработка на локальной машине.
- `stage` - тестовый VPS/предпрод.
- `prod` - боевое окружение.

## Конфигурация
1. Скопируйте `.env.example` в `.env`.
2. Заполните переменные окружения.
3. Приложение автоматически читает `.env` при старте.
4. Для `stage/prod` обязательны все секреты (`TELEGRAM_BOT_TOKEN`, `YOOKASSA_*`, `APP_ENCRYPTION_KEY`, `ADMIN_AUTH_TOKEN`, `POSTGRES_DSN`).

## Запуск
```bash
go run ./cmd/server
```

## Доступные маршруты
- `GET /healthz`
- `POST /telegram/webhook`
- `GET /mock/pay`
- `GET /mock/pay/success`
- `GET /admin/connectors`
- `POST /admin/connectors`
- `POST /admin/connectors/toggle`
- `GET /admin/help`

## Авторизация админки
- `?token=<ADMIN_AUTH_TOKEN>` или `Authorization: Bearer <ADMIN_AUTH_TOKEN>`.
- Если `ADMIN_AUTH_TOKEN` пустой, в `local` окружении доступ открыт.

## Коннекторы
Коннектор описывает условия оплаты (тариф):
- `start_payload` (например `in-94db7d6813507bc`)
- стоимость
- период
- чат
- ссылки оферты/политики

Ссылка для пользователя:
`https://t.me/<TELEGRAM_BOT_USERNAME>?start=<start_payload>`

## Временный режим оплаты
- Пока платежный шлюз не выбран, используется `PAYMENT_PROVIDER=mock`.
- Кнопка `Оплатить` ведет на тестовую страницу checkout (`/mock/pay`).
- Для публичного теста укажи `PAYMENT_MOCK_BASE_URL` (обычно URL ngrok).

## Документация
- Гайд по админке: `docs/ADMIN_GUIDE.md`
- Выжимка по регуляторике ПДн (РФ): `docs/DATA_COMPLIANCE_RU.md`

## Структура bot-пакета
- `internal/bot/handler.go` - базовый `Handler` и зависимости.
- `internal/bot/update_router.go` - роутинг Telegram update.
- `internal/bot/start.go` - обработка `/start <payload>`.
- `internal/bot/callback.go` - обработка callback-кнопок.
- `internal/bot/registration.go` - FSM регистрации и финальное сообщение.
- `internal/bot/payment.go` - формирование ссылки оплаты (mock/provider).
- `internal/bot/message.go` - входная обработка обычных сообщений.
- `internal/bot/validation.go` - нормализация/валидация телефона.
- `internal/bot/send.go` - отправка сообщений в Telegram.
