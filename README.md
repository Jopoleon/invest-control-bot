# invest-control-bot

Go backend для платного доступа к Telegram/MAX чатам с:
- ботом и onboarding flow
- админкой
- PostgreSQL
- Robokassa recurring/autopay
- публичными страницами оформления и отключения автоплатежа

## Что Сейчас Есть
- Telegram и MAX работают через единый backend, без отдельной MAX-бизнес-ветки.
- Основной production storage: PostgreSQL.
- Реализованы:
  - коннекторы
  - регистрация и профиль пользователя
  - платежи `mock` и `robokassa`
  - подписки, recurring consent, audit events
  - recurring lifecycle jobs
  - admin panel
  - public pages `/subscribe/{start_payload}` и `/unsubscribe/{token}`
- Текущий production приоритет: не ломать Telegram и удерживать recurring-логику стабильной.

## Основные Пакеты
- `cmd/server` - основной backend entrypoint
- `internal/app` - HTTP app, payment callbacks, recurring lifecycle
- `internal/admin` - server-rendered админка
- `internal/bot` - пользовательский flow
- `internal/telegram` - Telegram client
- `internal/max` - MAX client/sender/adapter
- `internal/payment` - payment providers
- `internal/store/postgres` - основной store
- `migrations` - схема БД
- `docs` - подробная документация

## Быстрый Старт
1. Скопировать `.env.example` в `.env`
2. Заполнить env
3. Запустить:

```bash
go run ./cmd/server
```

Тесты:
```bash
GOCACHE=/tmp/go-build GOTMPDIR=/tmp go test ./...
```

## Обязательные И Опциональные Env

Обязательные для `stage/prod`:

```env
APP_ENV=prod
APP_RUNTIME=server
HTTP_ADDR=127.0.0.1:8080

DB_HOST=127.0.0.1
DB_PORT=5432
DB_USERNAME=...
DB_PASSWORD=...
DB_DATABASE=investcontrol_prod
DB_SSL=disable

TELEGRAM_BOT_TOKEN=...
TELEGRAM_WEBHOOK_PUBLIC_URL=https://your-domain.example/telegram/webhook

APP_ENCRYPTION_KEY=replace-with-32-or-more-char-secret
ADMIN_AUTH_TOKEN=replace-with-strong-admin-token
```

Обязательные дополнительно, если включён Robokassa:

```env
PAYMENT_PROVIDER=robokassa
ROBOKASSA_MERCHANT_LOGIN=...
ROBOKASSA_PASS1=...
ROBOKASSA_PASS2=...
```

Опциональные, но practically useful:

```env
LOG_LEVEL=info
LOG_FILE_PATH=logs/app.log

TELEGRAM_BOT_USERNAME=...
TELEGRAM_WEBHOOK_SECRET=...

MAX_BOT_TOKEN=...
MAX_BOT_NAME=...
MAX_BOT_USERNAME=...
MAX_WEBHOOK_SECRET=...
MAX_WEBHOOK_PUBLIC_URL=https://your-domain.example/max/webhook

PAYMENT_MOCK_BASE_URL=https://your-domain.example
ROBOKASSA_IS_TEST_MODE=false
ROBOKASSA_RECURRING_ENABLED=true
ROBOKASSA_CHECKOUT_URL=https://auth.robokassa.ru/Merchant/Index.aspx
ROBOKASSA_REBILL_URL=https://auth.robokassa.ru/Merchant/Recurring
```

Опциональные аварийные network overrides для Telegram:

```env
TELEGRAM_API_BASE_URL=https://telegram-relay.example.com
TELEGRAM_HTTP_PROXY_URL=http://proxy.example.com:8080
```

Правило выбора:
- в обычном режиме оба поля пустые
- `TELEGRAM_API_BASE_URL` использовать для relay/custom Bot API endpoint
- `TELEGRAM_HTTP_PROXY_URL` использовать, если уже есть доверенный outbound proxy
- обычно нужен только один из двух обходов

## Telegram Relay Через Cloudflare
Некоторые RU-hosting/VPS сети могут нормально обслуживать backend, БД и обычный интернет, но выборочно не достукиваться до `api.telegram.org`. Для такого случая в проекте уже предусмотрен relay path через `TELEGRAM_API_BASE_URL`.

Текущий рабочий вариант:
- Cloudflare Worker выступает как HTTPS relay до `https://api.telegram.org` для outbound Bot API запросов
- тот же Worker может принимать Telegram webhook и пересылать его на origin приложения, если Telegram не достукивается до RU-домена напрямую
- приложение продолжает работать через обычный Telegram client, бизнес-логика при этом не меняется

Для Worker:
- в приложении задаётся
```env
TELEGRAM_API_BASE_URL=https://telegram-bot-relay.egortictac3.workers.dev
```
- если входящий webhook тоже нужно вести через Worker, в приложении задаётся:
```env
TELEGRAM_WEBHOOK_PUBLIC_URL=https://telegram-bot-relay.egortictac3.workers.dev/telegram/webhook
```
- в Worker задаются secrets:
```bash
wrangler secret put TELEGRAM_BOT_TOKEN
wrangler secret put TELEGRAM_WEBHOOK_SECRET
wrangler secret put TELEGRAM_WEBHOOK_ORIGIN_URL
```
- в `TELEGRAM_BOT_TOKEN` кладётся тот же токен, что и в приложении
- в `TELEGRAM_WEBHOOK_SECRET` кладётся тот же secret, что и в приложении
- в `TELEGRAM_WEBHOOK_ORIGIN_URL` кладётся прямой origin приложения, например `https://xn--b1aghkfidhbthmd7l.xn--p1ai/telegram/webhook`
- secrets нужны, чтобы relay обслуживал только нашего бота и принимал только webhook-запросы с правильным Telegram secret

Важно:
- в `TELEGRAM_API_BASE_URL` указывается только base URL relay
- без `/bot`
- без токена
- `TELEGRAM_WEBHOOK_PUBLIC_URL` может указывать на Worker route `/telegram/webhook`
- `TELEGRAM_WEBHOOK_ORIGIN_URL` в Worker должен указывать на настоящий webhook приложения, а не обратно на Worker

## Deploy И Ops

Простой VPS deploy:
```bash
bash scripts/deploy_vps.sh
```

Полезные варианты:
```bash
DEPLOY_LAYOUT=releases bash scripts/deploy_vps.sh
SKIP_RESTART=1 bash scripts/deploy_vps.sh
SHOW_SERVICE_STATUS=1 SHOW_SERVICE_LOGS=1 bash scripts/deploy_vps.sh
REMOTE_SERVICE_NAME=invest-control-bot bash scripts/deploy_vps.sh
```

Установка `systemd`-сервиса на сервере:
```bash
sudo cp /path/to/repo/deploy/systemd/invest-control-bot.service /etc/systemd/system/invest-control-bot.service
sudo mkdir -p /home/investcontrol/apps/invest-control-bot/releases
sudo mkdir -p /home/investcontrol/apps/invest-control-bot/shared
sudo chown -R investcontrol:investcontrol /home/investcontrol/apps/invest-control-bot
sudo -u investcontrol editor /home/investcontrol/apps/invest-control-bot/shared/invest-control-bot.env
sudo systemctl daemon-reload
sudo systemctl enable invest-control-bot
sudo systemctl start invest-control-bot
```

Systemd:
- unit template: `deploy/systemd/invest-control-bot.service`
- production env file:
  `/home/investcontrol/apps/invest-control-bot/shared/invest-control-bot.env`

Основные команды:
```bash
sudo systemctl status invest-control-bot
sudo journalctl -u invest-control-bot -n 100 --no-pager
```

## Production Notes
- PostgreSQL - основной source of truth
- `internal/store/memory` нужен только для local/dev и тестовых сценариев
- при старте backend применяет миграции автоматически, если `DB_WITH_MIGRATION=true`
- file logging опционален; если задан `LOG_FILE_PATH`, файлы ротируются по суткам
- при сетевых проблемах до Telegram backend теперь может переживать degraded startup и не падать целиком только из-за transport timeout

## MAX
MAX интегрирован как второй transport внутри того же backend.

Минимальные env для MAX:
```env
MAX_BOT_TOKEN=...
MAX_BOT_USERNAME=id9718272494_bot
MAX_WEBHOOK_PUBLIC_URL=https://your-domain.example/max/webhook
MAX_WEBHOOK_SECRET=your-max-webhook-secret
```

При старте backend:
- синхронизирует MAX webhook
- удаляет устаревшие subscriptions
- принимает события на `POST /max/webhook`

## Платежи И Recurring
- `mock` - локальный checkout flow
- `robokassa` - production payment flow
- источник истины по успешной оплате: provider callback, а не success page
- recurring уже включает:
  - explicit opt-in
  - cancel/re-enable flow
  - retry automation
  - audit trail
  - operator tooling в админке

Полезные файлы:
- `docs/payments/flow-ru.md`
- `docs/payments/robokassa-recurring.md`
- `docs/architecture/connector-period-model.md`

## Админка
В админке сейчас есть:
- `connectors`
- `users`
- `billing`
- `events`
- `issues`
- `legal documents`
- `sessions`
- `help`

Admin auth:
- browser-admin через server-side session и `HttpOnly` cookie
- machine-to-machine через `Authorization: Bearer <ADMIN_AUTH_TOKEN>`

## Документация
- `docs/README.md`
- `docs/ops/admin-guide.md`
- `docs/payments/flow-ru.md`
- `docs/payments/robokassa-recurring.md`
- `docs/max/implementation.md`
- `docs/architecture/app-refactor.md`
- `docs/architecture/refactoring-and-tests.md`
- `docs/backlog/todo.md`
- `IMPLEMENTATION_PLAN.md`
