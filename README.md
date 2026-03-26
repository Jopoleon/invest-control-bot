# invest-control-bot

Сервис управления подпиской и доступом в Telegram-чаты с админкой, платежами и recurring-логикой.

## Текущий статус
- Telegram-бот работает через `github.com/go-telegram/bot` и webhook на нашем Go-сервере.
- Основное хранилище: PostgreSQL (`sqlx` + `sql-migrate`), для локальных тестов доступен `memory` store.
- Реализованы:
  - коннекторы и ссылки оплаты `/start <payload>`
  - регистрация пользователя и повторное использование уже сохраненного профиля
  - платежный flow с `mock` и `robokassa`
  - хранение платежей, подписок, согласий и audit events
  - recurring flow для Robokassa: explicit opt-in, cancel/re-enable flow, retry automation, operator tools
  - публичные compliance-страницы оформления и отключения автоплатежа
- В админке уже есть экраны:
  - `connectors`
  - `users` и карточка пользователя
  - `billing`
  - `events`
  - `issues`
  - `legal documents`
  - `sessions`
  - `help`
- Browser-admin переведен на server-side sessions c `HttpOnly` cookie и валидацией на каждый запрос.
- Начато исследование и проектирование второго мессенджерного канала через MAX.

## Структура
- `cmd/server` - точка входа backend сервиса.
- `cmd/max-poller` - локальный MAX long-polling worker для dev/test.
- `internal/config` - загрузка и валидация конфигурации.
- `internal/app` - HTTP-сервер, маршрутизация, lifecycle jobs, payment callbacks.
- `internal/admin` - server-rendered админка, auth/session middleware, экраны операторов.
- `internal/bot` - Telegram flow, FSM регистрации, кабинет пользователя, checkout flow.
- `internal/max` - MAX transport client, sender, adapter и polling loop.
- `internal/payment` - интеграции платежных провайдеров.
- `internal/telegram` - клиент Telegram API.
- `internal/domain` - доменные модели и audit action constants.
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
4. Для `stage/prod` обязательны секреты и параметры БД.
5. Уровень логов задается через `LOG_LEVEL` (`debug|info|warn|error`).

## Запуск
```bash
go run ./cmd/server
```

### Локальный MAX development
Для MAX локальный dev-flow рекомендуем запускать через long polling, а не через webhook tunnel.

Минимальные env:
```env
APP_RUNTIME=server
APP_ENV=local
LOG_LEVEL=debug

MAX_BOT_TOKEN=...
MAX_BOT_NAME=...
MAX_POLLING_TYPES=bot_started,message_created,message_callback
MAX_POLLING_TIMEOUT_SEC=30
MAX_POLLING_LIMIT=100

PAYMENT_PROVIDER=mock
PAYMENT_MOCK_BASE_URL=https://your-ngrok-domain.ngrok-free.app

APP_ENCRYPTION_KEY=replace-with-32-or-more-char-secret
ADMIN_AUTH_TOKEN=replace-with-strong-admin-token
```

Запуск:
```bash
go run ./cmd/server
go run ./cmd/max-poller
```

Важно:
- для long polling у MAX не нужен `ngrok`;
- `ngrok` нужен только для web/payment ссылок, если ты открываешь их с телефона;
- перед polling у MAX-бота не должно быть активной webhook subscription.

### MAX webhook mode
Для production MAX теперь поддерживается и webhook flow внутри основного HTTP-сервера.

Минимальные env:
```env
MAX_BOT_TOKEN=...
MAX_WEBHOOK_PUBLIC_URL=https://your-domain.example/max/webhook
MAX_WEBHOOK_SECRET=your-max-webhook-secret
MAX_POLLING_TYPES=bot_started,message_created,message_callback
```

При старте `cmd/server` backend:
- синхронизирует MAX webhook subscription через API;
- удаляет устаревшие webhook subscriptions;
- принимает события на `POST /max/webhook`.

## Тесты
```bash
go test ./...
```

Для изолированного запуска в проблемном локальном окружении удобно:
```bash
GOCACHE=/tmp/go-build go test ./...
```

## База данных
- `DB_DRIVER=postgres` - использовать PostgreSQL.
- `DB_DRIVER=memory` - использовать in-memory store для локальных тестов.
- `DB_WITH_MIGRATION=true` - автоматически применять миграции при старте.
- Доступ к PostgreSQL реализован через `github.com/jmoiron/sqlx`.
- Миграции выполняются через `github.com/rubenv/sql-migrate`.

## Основные маршруты
- `GET /healthz`
- `POST /telegram/webhook`
- `POST /max/webhook`
- `GET /mock/pay`
- `GET /mock/pay/success`
- `POST /payment/result`
- `POST /payment/success`
- `POST /payment/fail`
- `POST /payment/rebill` - internal/admin trigger повторного списания
- `GET /subscribe/{start_payload}` - публичная страница оформления recurring-подписки
- `GET /unsubscribe/{token}` - публичная страница отключения автоплатежа
- `GET /oferta/{id}`
- `GET /policy/{id}`
- `GET /agreement/{id}`
- `GET /legal/offer`
- `GET /legal/privacy`
- `GET /legal/agreement`
- `GET /admin/login`
- `GET /admin/logout`
- `GET /admin/connectors`
- `GET /admin/users`
- `GET /admin/users/view`
- `GET /admin/billing`
- `GET /admin/events`
- `GET /admin/churn`
- `GET /admin/legal-documents`
- `GET /admin/sessions`
- `GET /admin/help`
- `GET /admin/assets/*`

## Авторизация админки
- Browser-admin:
  - вход через `/admin/login`
  - вводится `ADMIN_AUTH_TOKEN`
  - после успешного логина создается server-side session
  - в браузер пишется `HttpOnly` cookie, сама сессия валидируется на каждый запрос
- Для операторского контроля есть экран `/admin/sessions`, где можно смотреть и отзывать активные admin-сессии.
- Для machine-to-machine сценариев сохранен `Authorization: Bearer <ADMIN_AUTH_TOKEN>`.
- Если `ADMIN_AUTH_TOKEN` пустой, в `local` окружении админка остается открытой.

## Экраны админки
- `Connectors` - создание, активация/деактивация и удаление тарифов, генерация bot-link.
- `Users` - список пользователей, карточка клиента, подписки, платежи, события, согласия, ручные действия.
- `Billing` - платежи, подписки, summary cards и breakdown по коннекторам.
- `Events` - audit trail по действиям пользователей и операторов.
- `Issues` - проблемные подписки и оплаты, recurring-state, retry-state, ручной rebill.
- `Legal documents` - реестр оферт, политик и пользовательских соглашений, публичные ссылки, версии.
- `Sessions` - активные browser-сессии админки и их revoke.

## Коннекторы
Коннектор описывает условия оплаты:
- стоимость
- период
- `channel_url` и/или `chat_id`
- ссылки оферты/политики
- `start_payload`

Ссылка для пользователя:
`https://t.me/<TELEGRAM_BOT_USERNAME>?start=<start_payload>`

Если пользователь уже есть в базе и профиль заполнен, бот повторно не спрашивает ФИО, телефон и email.

## Пользовательские и юридические данные
- Храним:
  - профиль пользователя
  - платежи
  - подписки
  - audit events
  - согласия на оферту/политику с версией документа
  - recurring consents
- Юридические документы доступны публично по ссылкам без авторизации.
- Для fallback-сценариев бот может подставлять активные документы из реестра, если у коннектора не указаны свои URL.

## Платежные провайдеры
### Mock
- `PAYMENT_PROVIDER=mock`
- Кнопка `Оплатить` ведет на локальную test checkout-страницу.
- Для локального теста с Telegram удобно указывать `PAYMENT_MOCK_BASE_URL` через `ngrok`.

### Robokassa
- `PAYMENT_PROVIDER=robokassa`
- Обязательные env:
  - `ROBOKASSA_MERCHANT_LOGIN`
  - `ROBOKASSA_PASS1`
  - `ROBOKASSA_PASS2`
  - `ROBOKASSA_IS_TEST_MODE=true` для тестов
  - `ROBOKASSA_RECURRING_ENABLED=true` для recurring flow в средах, где он разрешен
  - `ROBOKASSA_REBILL_URL` по умолчанию `https://auth.robokassa.ru/Merchant/Recurring`
- Callback endpoints:
  - `POST /payment/result` - источник истины по успешной оплате
  - `POST /payment/success`
  - `POST /payment/fail`
- Ручной и автоматический recurring trigger уже реализованы.

## Recurring / автоплатежи
- В продукте уже есть:
  - explicit opt-in consent flow
  - cancel flow в боте и через публичную страницу
  - re-enable flow по подписке
  - recurring consent history
  - scheduler для retry-окон `T-3 / T-2 / T-1`
  - уведомления о failed recurring payment с fallback на ручную оплату
  - operator tools в админке: recurring health, retry state, ручной `rebill`
- Для production readiness нужно держать в актуальном состоянии юридические тексты и checkout/cancel UX.
- Практический checklist находится в `docs/robokassa-recurring-checklist.md`.

## Логирование
- Используется `log/slog` со structured logs.
- Можно включать file logging через `LOG_FILE_PATH`.
- В `debug` режиме пишутся подробные Telegram webhook payload логи. Это удобно локально, но перед `stage/prod` нужно проверять политику по ПДн.

## Документация
- Гайд по админке: `docs/ADMIN_GUIDE.md`
- Выжимка по регуляторике ПДн (РФ): `docs/DATA_COMPLIANCE_RU.md`
- Миграции БД: `docs/MIGRATIONS.md`
- Подробный flow оплат и автоплатежей: `docs/PAYMENTS_FLOW_RU.md`
- Чеклист по recurring для Robokassa: `docs/robokassa-recurring-checklist.md`
- MAX: исследование интеграции `docs/MAX_BOT_RESEARCH.md`
- MAX: архитектурная декомпозиция `docs/MAX_DECOMPOSITION_PLAN.md`
- MAX: рабочий план внедрения `docs/MAX_IMPLEMENTATION_PLAN.md`
- Основной roadmap продукта и статус итераций: `IMPLEMENTATION_PLAN.md`

## Структура bot-пакета
- `internal/bot/handler.go` - базовый `Handler` и зависимости.
- `internal/bot/update_router.go` - роутинг Telegram update.
- `internal/bot/start.go` - обработка `/start <payload>`.
- `internal/bot/callback.go` - callback-кнопки, акцепт условий, recurring opt-in.
- `internal/bot/registration.go` - FSM регистрации и завершение заявки.
- `internal/bot/payment.go` - формирование checkout-ссылок.
- `internal/bot/menu.go` - кабинет пользователя, подписки, автоплатеж.
- `internal/bot/validation.go` - нормализация и валидация полей.
- `internal/bot/send.go` - отправка сообщений в Telegram.
