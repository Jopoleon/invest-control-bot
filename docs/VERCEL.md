# Vercel Deployment

## Почему текущий деплой падал
Vercel пытался поднять долгоживущий Go-сервер (`go run ./cmd/server`), который делает `ListenAndServe()` и ждет входящих соединений. Для Vercel это неверная модель. На Vercel нужно деплоить request-based Functions.

## Runtime mode
- `APP_RUNTIME=server` - локальный/VPS режим через `go run ./cmd/server`.
- `APP_RUNTIME=vercel` - request-based режим через `api/*.go`.

## Что сделано
- Весь текущий mux отдается через `api/index.go` как одна Go Function.
- Lifecycle/scheduler вынесен в `api/cron/lifecycle.go` и запускается через Vercel Cron.
- `vercel.json` переписывает все пользовательские маршруты на `/api/index`.

## Что настроить в Vercel
### Production Environment
Задать production env vars:
- `APP_ENV=prod`
- `APP_RUNTIME=vercel`
- `HTTP_ADDR=:8080` не нужен для Vercel Function, но не мешает
- `DB_DRIVER=postgres`
- `DB_HOST`
- `DB_PORT`
- `DB_USERNAME`
- `DB_PASSWORD`
- `DB_DATABASE`
- `DB_SSL=require` или фактический режим вашего провайдера
- `DB_WITH_MIGRATION=true`
- `TELEGRAM_BOT_TOKEN`
- `TELEGRAM_BOT_USERNAME`
- `TELEGRAM_WEBHOOK_SECRET`
- `TELEGRAM_WEBHOOK_PUBLIC_URL=https://<production-domain>/telegram/webhook`
- `PAYMENT_PROVIDER`
- Robokassa env vars при необходимости
- `APP_ENCRYPTION_KEY`
- `ADMIN_AUTH_TOKEN`
- `LOG_LEVEL=info`
- `LOG_FILE_PATH=` пустой

### Preview Environment
Preview deployment не должен переписывать production webhook Telegram и callback URLs Robokassa.

Безопасный вариант:
- `APP_ENV=local`
- `APP_RUNTIME=vercel`
- `DB_DRIVER=memory` или отдельная preview database
- `TELEGRAM_BOT_TOKEN=` пусто
- `TELEGRAM_WEBHOOK_PUBLIC_URL=` пусто
- `PAYMENT_PROVIDER=mock`
- `LOG_FILE_PATH=` пусто

Если нужен полноценный preview c Telegram, используй отдельного preview-бота и отдельные preview callback URLs.

## Build & Runtime Settings
Если в проекте были вручную выставлены `Build Command` / `Output Directory` / `Install Command` / `Development Command`, убери их. Для этого проекта Vercel должен использовать `vercel.json` и Go Functions из `api/`.

## Cron
Lifecycle теперь вызывается Vercel Cron каждые 5 минут:
- `/api/cron/lifecycle`

При желании можно добавить `VERCEL_CRON_SECRET` и использовать query token в path вручную, но базово уже есть проверка `User-Agent: vercel-cron/1.0`.

## CLI
Установить CLI:
```bash
npm i -g vercel
```

Полезные команды:
```bash
vercel pull --environment=production
vercel pull --environment=preview
vercel env ls
vercel deploy
vercel deploy --prod
vercel logs <deployment-url>
```
