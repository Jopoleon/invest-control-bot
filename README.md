# telega-bot-fedor

Сервис управления подпиской и доступом в Telegram-чаты.

## Текущий статус
- Выполнена итерация 0: базовая структура проекта, стратегия конфигурации и секретов, фиксация окружений.
- Выполнена итерация 2 (MVP-часть): коннекторы в админке, `/start payload`, акцепт условий и FSM регистрации.
- Telegram stack переведен на `github.com/go-telegram/bot` (webhook на нашем Go-сервере).
- Реализованы PostgreSQL-миграции и хранение платежей/подписок.
- Добавлены админ-экраны `billing` и `events`.

## Структура
- `cmd/server` - точка входа backend сервиса.
- `internal/config` - загрузка и валидация конфигурации.
- `internal/app` - HTTP-сервер и маршрутизация.
- `internal/admin` - минимальная админка (коннекторы).
- `internal/bot` - обработчик Telegram flow.
- `internal/store/memory` - in-memory хранилище для локальных тестов.
- `internal/store/postgres` - PostgreSQL хранилище (основной режим).
- `migrations` - SQL-миграции БД.
- `docs` - техническая документация.

## Окружения
- `local` - разработка на локальной машине.
- `stage` - тестовый VPS/предпрод.
- `prod` - боевое окружение.

## Конфигурация
1. Скопируйте `.env.example` в `.env`.
2. Заполните переменные окружения.
3. Приложение автоматически читает `.env` при старте.
4. Для `stage/prod` обязательны все секреты (`TELEGRAM_BOT_TOKEN`, `YOOKASSA_*`, `APP_ENCRYPTION_KEY`, `ADMIN_AUTH_TOKEN`) и DB-параметры (`DB_DRIVER`, `DB_HOST`, `DB_PORT`, `DB_USERNAME`, `DB_PASSWORD`, `DB_DATABASE`, `DB_SSL`).
5. Уровень логов задается через `LOG_LEVEL` (`debug|info|warn|error`).

## Запуск
```bash
go run ./cmd/server
```

## База данных
- `DB_DRIVER=postgres` - использовать PostgreSQL (по умолчанию).
- `DB_DRIVER=memory` - использовать in-memory store (только для локальных тестов).
- `DB_WITH_MIGRATION=true` - автоматически применять миграции при старте.
- Доступ к PostgreSQL реализован через `github.com/jmoiron/sqlx`.
- Миграции выполняются через `github.com/rubenv/sql-migrate`.

## Доступные маршруты
- `GET /healthz`
- `POST /telegram/webhook`
- `GET /mock/pay`
- `GET /mock/pay/success`
- `POST /payment/result`
- `POST /payment/success`
- `POST /payment/fail`
- `POST /payment/rebill` (admin/internal endpoint for recurring charge trigger)
- `GET /admin/connectors`
- `GET /admin/billing`
- `GET /admin/events`
- `GET /admin/login`
- `GET /admin/logout`
- `POST /admin/connectors`
- `POST /admin/connectors/toggle`
- `GET /admin/help`
- `GET /admin/assets/*`

## Авторизация админки
- Откройте `/admin/login`, введите `ADMIN_AUTH_TOKEN`, после этого токен сохранится в HTTP-only cookie.
- Для API/скриптов доступен `Authorization: Bearer <ADMIN_AUTH_TOKEN>`.
- Если `ADMIN_AUTH_TOKEN` пустой, в `local` окружении доступ открыт.

## Коннекторы
Коннектор описывает условия оплаты (тариф):
- `start_payload` (например `in-94db7d6813507bc`)
- стоимость
- период
- `channel_url` и/или `chat_id` (хотя бы одно поле должно быть заполнено)
- ссылки оферты/политики

Ссылка для пользователя:
`https://t.me/<TELEGRAM_BOT_USERNAME>?start=<start_payload>`

## Временный режим оплаты
- Пока платежный шлюз не выбран, используется `PAYMENT_PROVIDER=mock`.
- Кнопка `Оплатить` ведет на тестовую страницу checkout (`/mock/pay`).
- Для публичного теста укажи `PAYMENT_MOCK_BASE_URL` (обычно URL ngrok).

## Robokassa
- Для включения: `PAYMENT_PROVIDER=robokassa`.
- Обязательные env:
  - `ROBOKASSA_MERCHANT_LOGIN`
  - `ROBOKASSA_PASS1`
  - `ROBOKASSA_PASS2`
  - `ROBOKASSA_IS_TEST_MODE=true` (для тестов)
  - `ROBOKASSA_REBILL_URL` (по умолчанию `https://auth.robokassa.ru/Merchant/Recurring`)
- URL callbacks:
  - `POST /payment/result` (подтверждение платежа, источник истины)
  - `POST /payment/success`
  - `POST /payment/fail`
  - `POST /payment/rebill` (внутренний endpoint для запуска повторного списания по `subscription_id`)

## Логирование
- Используется `log/slog` (structured logs).
- В `debug` режиме включаются подробные Telegram webhook payload логи.

## Документация
- Гайд по админке: `docs/ADMIN_GUIDE.md`
- Выжимка по регуляторике ПДн (РФ): `docs/DATA_COMPLIANCE_RU.md`
- Миграции БД (sql-migrate): `docs/MIGRATIONS.md`

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
