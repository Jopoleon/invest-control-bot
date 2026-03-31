# Прод: трек текущих багов и план исправления

Дата: 2026-04-01
Статус: active

## Контекст

На проде был проведен live-money smoke test:
- Telegram flow;
- MAX flow;
- реальные платежи;
- короткие тестовые подписки `2 min`;
- recurring/autopay сценарий.

Ниже зафиксированы выводы по логам и по текущему коду.

## Короткое резюме

Сейчас прод уже умеет:
- принимать реальные платежи через Robokassa;
- активировать подписку после `ResultURL` callback;
- отправлять success-notify в Telegram и MAX;
- проводить MAX onboarding и оплату;
- создавать rebill path для recurring.

Но smoke test подтвердил 4 реальные продуктовые проблемы:
1. short-period тест подписки ломает текущую recurring/lifecycle стратегию и провоцирует слишком ранние повторные rebill-платежи;
2. админка все еще визуально и структурно Telegram-first;
3. admin actions `message` и `payment link` сейчас Telegram-only и не работают для MAX-only пользователя;
4. startup слишком чувствителен к единичным timeout'ам Telegram API.

Дополнительно:
- публичный endpoint активно сканируют внешние боты;
- это не текущий business bug, но это нужно учесть на уровне proxy/rate-limit.

## Finding 1. «Подписки создаются 2 раза»

Приоритет: P0

### Что видно в логах

Telegram flow:
- `22:36:09` — `payment_id=1` помечен `paid`;
- `22:37:15` — `payment_id=2` помечен `paid`.

MAX flow:
- `22:52:36` — `payment_id=6` помечен `paid`;
- `22:53:16` — `payment_id=7` помечен `paid`.

Это не похоже на двойную обработку одного и того же callback:
- в логах нет повторного `paid` для одного и того же `payment_id`;
- идут именно разные payment rows.

### Что делает код сейчас

1. Каждая успешная оплата вызывает активацию через:
- [service.go](/home/egor/Work/src/github.com/Jopoleon/invest-control-bot/internal/app/payments/service.go)

2. Там вызывается:
- `UpsertSubscriptionByPayment(...)`

3. На уровне PostgreSQL конфликт только по `payment_id`:
- [subscriptions.go](/home/egor/Work/src/github.com/Jopoleon/invest-control-bot/internal/store/postgres/subscriptions.go)

Это означает:
- один и тот же payment не создаст подписку дважды;
- но каждый новый payment, включая rebill, создаст новую subscription row для нового периода.

### Вывод

Нужно четко разделить:

1. Ожидаемое поведение модели:
- новая успешная оплата = новая subscription row на новый период.

2. Реальный баг из smoke test:
- новый rebill payment создается слишком рано для тестовых коротких подписок.

Именно из-за этого у тебя визуально возникает ощущение, что «подписка создалась два раза».
По факту система успевает почти сразу перейти к следующему recurring payment и следующему subscription period.

### Корневая причина

Recurring scheduler использует day-based окна, которые подходят для обычных месячных периодов, но не подходят для `2 min`.

Код:
- [service.go](/home/egor/Work/src/github.com/Jopoleon/invest-control-bot/internal/app/recurring/service.go)
- [subscription_lifecycle.go](/home/egor/Work/src/github.com/Jopoleon/invest-control-bot/internal/app/subscription_lifecycle.go)

Ключевой дефект:
- `AttemptOrdinal(...)` считает окна `72h / 48h / 24h`;
- для подписки длиной `2 min` остаток времени практически сразу попадает в `<= 24h`;
- scheduler начинает считать, что пора инициировать rebill почти сразу.

Параллельно lifecycle logic тоже day-based:
- reminder = `3 days before end`;
- expiry notice = `24h before end`.

Поэтому short-period smoke на проде сейчас не тестирует «месячный recurring в миниатюре», а ломает production windows и попадает в неподходящую зону поведения.

### Что исправлять

Нужен явный `short-period strategy` для коннекторов с `TestPeriodSeconds > 0`.

Сделать:
1. Отдельные rebill windows для short periods.
2. Отдельные reminder/expiry windows для short periods.
3. Не использовать day-based `72h/48h/24h` и `3 days/24h` для short test connectors.
4. Добавить прямые тесты именно на `60s/120s` сценарии.

### Рабочая стратегия для тестовых периодов

Если `TestPeriodSeconds <= 10m`:
- rebill attempt #1: за `30s` до `ends_at`;
- rebill attempt #2: за `10s` до `ends_at`;
- если уже есть `pending` rebill payment, scheduler больше не дергает повторно;
- pre-expiry notice отключить;
- reminder либо отключить, либо оставить один soft notice за `45s`;
- expired notification оставить.

### Acceptance criteria

1. Подписка `2 min` не создает новый recurring payment почти сразу после первой оплаты.
2. До истечения периода есть максимум один `pending` rebill payment.
3. Не приходят сразу и `reminder`, и `expiry notice` после первой оплаты.
4. После успешного rebill появляется ровно один следующий payment и ровно один следующий subscription period.

## Finding 2. Админка все еще Telegram-first

Приоритет: P1

### Симптом

В интерфейсе слишком много Telegram-first presentation:
- `telegram_id` filters;
- `telegram_username` columns;
- таблицы, где Telegram projection виден явно, а MAX почти не виден.

Это уже мешает не только визуально, но и операционно:
- MAX user существует в системе;
- но в админке его representation неравноправен.

### Подтверждение в коде

Тексты и labels:
- [localization.go](/home/egor/Work/src/github.com/Jopoleon/invest-control-bot/internal/admin/localization.go)

Telegram-first filters/exports:
- [billing.go](/home/egor/Work/src/github.com/Jopoleon/invest-control-bot/internal/admin/billing.go)
- [users_page.go](/home/egor/Work/src/github.com/Jopoleon/invest-control-bot/internal/admin/users_page.go)
- [churn.go](/home/egor/Work/src/github.com/Jopoleon/invest-control-bot/internal/admin/churn.go)
- [export.go](/home/egor/Work/src/github.com/Jopoleon/invest-control-bot/internal/admin/export.go)

Шаблоны:
- [users.html](/home/egor/Work/src/github.com/Jopoleon/invest-control-bot/internal/admin/templates/users.html)
- [billing.html](/home/egor/Work/src/github.com/Jopoleon/invest-control-bot/internal/admin/templates/billing.html)
- [churn.html](/home/egor/Work/src/github.com/Jopoleon/invest-control-bot/internal/admin/templates/churn.html)

### Что делать

1. Основной identity axis сделать `user_id`.
2. Telegram ID перевести в secondary/legacy filter.
3. В users/billing/churn/events добавить messenger-neutral projection:
- primary messenger;
- linked identities;
- preferred delivery channel.
4. Убрать визуальное ощущение, что MAX — это «неполный пользователь».

### Acceptance criteria

1. MAX-only пользователь читается в админке так же явно, как Telegram user.
2. Telegram больше не выглядит как единственный канонический transport.
3. Оператор может понять, в какой transport писать пользователю, не залезая в БД.

## Finding 3. Сообщения из интерфейса уходят только в Telegram

Приоритет: P1

### Симптом

Из админки:
- сообщение пользователю можно отправить в Telegram;
- MAX-only пользователю нельзя;
- payment link из user detail тоже фактически Telegram-only.

### Подтверждение в коде

Код admin actions жестко зависит от Telegram identity и Telegram sender:
- [user_actions.go](/home/egor/Work/src/github.com/Jopoleon/invest-control-bot/internal/admin/user_actions.go)

Что именно там сейчас происходит:
- вызывается `resolveTelegramIdentity(...)`;
- при отсутствии Telegram identity action считается невалидным;
- сообщение уходит через `h.tg.SendMessage(...)`;
- payment link строится как Telegram deeplink.

### Root cause

Admin layer еще не переведен на messenger-neutral delivery boundary.
Он опирается на старую модель «админ пишет только в Telegram».

### Что делать

Перевести admin actions на transport-neutral отправку:

1. `sendUserMessage`
- выбрать preferred linked messenger account пользователя;
- отправить через общий sender boundary;
- записать audit с messenger context.

2. `sendUserPaymentLink`
- Telegram: оставить deeplink button;
- MAX: слать MAX deeplink и fallback-команду `/start <payload>`.

3. В user detail явно показать доступные actions по transport:
- `Send Telegram message` только если есть Telegram identity;
- `Send MAX message` или unified `Send message`, если preferred kind = MAX.

### Acceptance criteria

1. MAX-only пользователю можно отправить обычное сообщение из админки.
2. MAX-only пользователю можно отправить payment link из админки.
3. Audit event фиксирует transport доставки.

## Finding 4. Startup слишком чувствителен к временному Telegram timeout

Приоритет: P2

### Что видно в логах

На старте есть:
- `create app server failed`;
- `getMe ... context deadline exceeded`.

Следующий старт уже успешен.

### Вывод

Сейчас startup fail-fast даже на одиночный временный сетевой timeout.
Для битого токена это правильно.
Для временного сетевого дребезга — слишком хрупко.

### Что делать

1. Добавить retry/backoff на startup transport health check:
- Telegram `getMe`;
- MAX `/me`.
2. Развести ошибки:
- invalid token / auth error -> fail hard сразу;
- timeout / temporary network error -> 2-3 retries.

### Acceptance criteria

1. Единичный Telegram timeout не валит cold start мгновенно.
2. Битый токен по-прежнему валит startup сразу и явно.

## Finding 5. Публичный endpoint активно сканируют

Приоритет: P2

### Что видно в логах

В логе много типового интернет-мусора:
- `/wp-admin/setup-config.php`
- `/wordpress/wp-admin/setup-config.php`
- `/vendor/phpunit/.../eval-stdin.php`
- `PROPFIND /`
- `/ReportServer`
- `/containers/json`

### Вывод

Это не указывает на компрометацию приложения.
Это нормальный внешний скан публичного домена.

### Что делать

На уровне reverse proxy / ingress:
1. rate limit;
2. cheap reject rules на типовые scan paths;
3. отдельный suspicious-path counter/metric;
4. убедиться, что `/admin` не экспонирует ничего лишнего без auth.

## План исправления

### Этап 1. Исправить short-period recurring semantics

Приоритет: P0
Статус: implemented

Файлы-кандидаты:
- [service.go](/home/egor/Work/src/github.com/Jopoleon/invest-control-bot/internal/app/recurring/service.go)
- [subscription_lifecycle.go](/home/egor/Work/src/github.com/Jopoleon/invest-control-bot/internal/app/subscription_lifecycle.go)
- соответствующие тесты в `internal/app`

Сделать:
1. ввести explicit short-period strategy;
2. покрыть тестами `60s/120s`;
3. не трогать production monthly logic для normal connectors.

Сделано:
1. Для коннекторов с `test_period_seconds > 0` rebill больше не использует production windows `72h/48h/24h`.
2. Для short-period connectors pre-expiry reminder и expiry notice отключены.
3. Scheduler cadence уменьшен до `10s`, чтобы `60s/120s` тестовые периоды реально попадали в rebill windows.
4. Добавлены regression tests на short-period rebill eligibility и на отсутствие pre-expiry сообщений.

### Этап 2. Перевести admin message/payment-link на messenger-neutral delivery

Приоритет: P1
Статус: implemented

Файлы-кандидаты:
- [user_actions.go](/home/egor/Work/src/github.com/Jopoleon/invest-control-bot/internal/admin/user_actions.go)
- [handler.go](/home/egor/Work/src/github.com/Jopoleon/invest-control-bot/internal/admin/handler.go)
- шаблоны user detail

Сделать:
1. unified delivery action;
2. MAX support;
3. audit with messenger context.

Сделано:
1. `admin.Handler` теперь получает не только Telegram client, но и MAX sender.
2. `sendUserMessage` больше не требует Telegram identity и отправляет в preferred linked messenger account пользователя.
3. `sendUserPaymentLink` теперь работает для MAX-only пользователя:
   - строит MAX deeplink;
   - добавляет fallback-команду `/start <payload>` в текст сообщения.
4. User detail page больше не скрывает direct-message action только из-за отсутствия Telegram identity.
5. Добавлены regression tests на:
   - direct admin message в MAX-only account;
   - admin payment-link send в MAX-only account.

### Этап 3. Дочистить Telegram-first presentation в админке

Приоритет: P1

Файлы-кандидаты:
- templates `users`, `billing`, `churn`, `events`;
- `localization.go`;
- export/query labels.

Сделать:
1. user-first vocabulary;
2. linked identities view;
3. Telegram legacy fields сделать secondary.

### Этап 4. Смягчить startup transport health checks

Приоритет: P2

Файлы-кандидаты:
- `internal/app/application.go`
- startup transport init/check code

Сделать:
1. retry/backoff на temporary transport errors;
2. fail-hard только на auth/config errors.

## Post-fix smoke checklist

### Telegram short-period recurring

1. Создать connector `2 min`.
2. Пройти first payment с autopay on.
3. Убедиться, что сразу после first payment не создается следующий payment.
4. Убедиться, что rebill создается только в short-period window.
5. Убедиться, что приходит максимум один success-notify на один payment.

### MAX short-period recurring

1. Создать connector `2 min`.
2. Пройти first payment с autopay on.
3. Убедиться, что admin view показывает MAX identity без Telegram assumptions.
4. Отправить admin message пользователю в MAX.
5. Отправить admin payment link пользователю в MAX.
6. Проверить rebill и next subscription period.

## Итог

Главное: продовый smoke уже показал, что платежная и messenger-интеграция в целом работают.
Но short-period recurring тест сейчас нельзя считать валидной проверкой «боевого автосписания», потому что:
- текущий scheduler живет в production day-based semantics;
- test periods минутного масштаба в эти semantics не укладываются.

Первым исправлением должен быть не косметический UI, а именно `short-period recurring strategy`.
Иначе все остальные наблюдения по duplicate subscriptions будут и дальше шуметь и маскировать реальное поведение системы.
