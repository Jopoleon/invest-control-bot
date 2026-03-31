# Прод: debug и план исправления

Дата: 2026-04-01
Статус: active

## Контекст

На проде был проведен live-money smoke test с:
- реальными платежами;
- короткими тестовыми подписками (`2 min`);
- Telegram и MAX flow;
- recurring/autopay сценарием.

Ниже зафиксированы фактические баги и продуктовые хвосты, подтвержденные логами и текущим кодом.

## Краткий вывод

Система в проде уже умеет:
- принимать реальные платежи;
- активировать подписки;
- отправлять success-уведомления в Telegram и MAX;
- обрабатывать recurring consent;
- создавать rebill payment path.

Но текущий прод smoke показал 4 реальные проблемы:
1. короткие тестовые подписки ломают scheduler semantics и создают повторные recurring-списания слишком рано;
2. админка все еще сильно Telegram-first в таблицах и фильтрах;
3. admin messaging/payment-link actions реализованы только для Telegram, не для MAX;
4. startup слишком чувствителен к временным timeout'ам Telegram API.

## Finding 1: повторные подписки / повторные платежи на коротких периодах

Приоритет: P0

### Симптом

Во время теста с `2 min` подпиской:
- `payment_id=1` помечен paid в `22:36:09`;
- уже в `22:37:15` помечен paid `payment_id=2`.

Повторяется тот же паттерн и в MAX flow:
- `payment_id=6` paid в `22:52:36`;
- `payment_id=7` paid в `22:53:16`.

Одновременно вокруг этих подписок почти сразу начали приходить:
- expiry notice;
- reminder;
- потом expired notification.

### Root cause

Для коротких тестовых подписок scheduler все еще живет в day-based окнах.

Код:
- [service.go](/home/egor/Work/src/github.com/Jopoleon/invest-control-bot/internal/app/recurring/service.go#L177)
- [service.go](/home/egor/Work/src/github.com/Jopoleon/invest-control-bot/internal/app/recurring/service.go#L316)
- [subscription_lifecycle.go](/home/egor/Work/src/github.com/Jopoleon/invest-control-bot/internal/app/subscription_lifecycle.go#L17)
- [subscription_lifecycle.go](/home/egor/Work/src/github.com/Jopoleon/invest-control-bot/internal/app/subscription_lifecycle.go#L94)

Что происходит:
- `AttemptOrdinal(now, sub.EndsAt)` считает попытки по окнам `72h / 48h / 24h`.
- Для подписки длиной `2 min` остаток времени всегда `<= 24h`, значит scheduler почти сразу считает, что надо делать rebill.
- Reminder и expiry notice тоже не адаптированы под short period:
  - reminder = `3 days before end`;
  - expiry notice = `24h before end`.

Итог:
- мы получаем не “нормальный тестовый recurring через 2 минуты”, а немедленное попадание в production windows, рассчитанные на дневные/месячные периоды.

### Вывод

Это не общий баг “подписки всегда создаются два раза”.
Это баг именно short-period prod testing mode поверх существующей day-based recurring/lifecycle стратегии.

### Исправление

Нужен отдельный `test-only short period strategy`.

Сделать:
1. В rebill scheduler ввести отдельную стратегию для `connector.TestPeriodSeconds > 0`.
2. В reminder/expiry logic ввести отдельные окна для short periods.
3. Запретить использовать day-based windows для test-period connectors.
4. Явно покрыть это тестами.

### Предлагаемая стратегия для short periods

Для `TestPeriodSeconds <= 10m`:
- rebill try #1: за `30s` до конца;
- rebill try #2: за `10s` до конца;
- больше не дергать до прихода callback по pending payment;
- reminder notice: либо отключить совсем, либо слать только один soft notice за `45s`;
- expiry notice: отключить отдельный pre-expiry notice для test mode;
- expired notification оставить.

### Acceptance criteria

1. Подписка `2 min` не создает rebill сразу после первой успешной оплаты.
2. До конца периода возникает максимум один pending rebill payment.
3. Не приходят одновременно и reminder, и expiry notice сразу после оплаты.
4. После callback по rebill появляется ровно одно продление периода.

## Finding 2: админка все еще Telegram-first

Приоритет: P1

### Симптом

В интерфейсе слишком много:
- `telegram_id` фильтров;
- `telegram_username` колонок;
- Telegram-only названий в billing/users/churn/events.

При этом для MAX в этих же местах нет симметричного представления identity.

### Подтверждение в коде

Шаблоны:
- [users.html](/home/egor/Work/src/github.com/Jopoleon/invest-control-bot/internal/admin/templates/users.html#L41)
- [billing.html](/home/egor/Work/src/github.com/Jopoleon/invest-control-bot/internal/admin/templates/billing.html#L40)
- [billing.html](/home/egor/Work/src/github.com/Jopoleon/invest-control-bot/internal/admin/templates/billing.html#L163)
- [billing.html](/home/egor/Work/src/github.com/Jopoleon/invest-control-bot/internal/admin/templates/billing.html#L208)
- [churn.html](/home/egor/Work/src/github.com/Jopoleon/invest-control-bot/internal/admin/templates/churn.html#L48)
- [churn.html](/home/egor/Work/src/github.com/Jopoleon/invest-control-bot/internal/admin/templates/churn.html#L130)

Экспорты и query params:
- [export.go](/home/egor/Work/src/github.com/Jopoleon/invest-control-bot/internal/admin/export.go)

### Root cause

UI уже частично переведен на `user_id`, но presentation layer все еще строится вокруг Telegram projection как будто это canonical user identity.

### Исправление

Сделать единый UI vocabulary:
- основной идентификатор: `user_id`;
- отдельный блок/channel projection:
  - Telegram ID
  - Telegram username
  - MAX user id / MAX display name

### Конкретные шаги

1. Во всех filters первым оставить `user_id`.
2. Telegram filter спрятать под “Дополнительные фильтры” или переименовать в `Telegram ID (legacy filter)`.
3. В `users`, `billing`, `churn`, `events` добавить messenger-neutral колонку:
   - `Primary Messenger`
   - или `Identities`.
4. MAX identity выводить рядом, а не только в скрытом store-level смысле.

### Acceptance criteria

1. В users/billing/churn можно визуально понять, что пользователь — MAX-only.
2. Таблицы не создают впечатление, что без Telegram пользователя “как бы нет”.
3. В UI нет критичных business-action blockers для MAX-only user.

## Finding 3: admin message и payment link сейчас Telegram-only

Приоритет: P1

### Симптом

Из админки:
- сообщение пользователю уходит в Telegram;
- в MAX аналогичного рабочего path нет.

### Подтверждение в коде

Код admin actions жестко зависит от Telegram identity и Telegram client:
- [user_actions.go](/home/egor/Work/src/github.com/Jopoleon/invest-control-bot/internal/admin/user_actions.go#L14)
- [user_actions.go](/home/egor/Work/src/github.com/Jopoleon/invest-control-bot/internal/admin/user_actions.go#L41)
- [user_actions.go](/home/egor/Work/src/github.com/Jopoleon/invest-control-bot/internal/admin/user_actions.go#L57)
- [user_actions.go](/home/egor/Work/src/github.com/Jopoleon/invest-control-bot/internal/admin/user_actions.go#L68)
- [user_actions.go](/home/egor/Work/src/github.com/Jopoleon/invest-control-bot/internal/admin/user_actions.go#L97)
- [user_actions.go](/home/egor/Work/src/github.com/Jopoleon/invest-control-bot/internal/admin/user_actions.go#L143)

Что именно там не так:
- перед отправкой вызывается `resolveTelegramIdentity(...)`;
- если Telegram identity нет, action считается невалидным;
- отправка делается через `h.tg.SendMessage(...)`;
- payment link собирается только как Telegram deeplink `https://t.me/...`.

### Root cause

Admin actions еще не переведены на messenger-neutral delivery boundary. Они остались на старой модели “админ пишет только в Telegram”.

### Исправление

Перевести admin actions на единый messenger delivery слой.

Нужны 2 отдельные задачи:
1. `sendUserMessage`
   - выбирать preferred linked messenger account пользователя;
   - отправлять через transport-specific sender;
   - писать честный audit с messenger context.
2. `sendUserPaymentLink`
   - для Telegram слать deeplink button;
   - для MAX слать либо MAX deeplink, либо `/start payload` как текст + button на `max.ru/<bot>?start=...`, если deeplink стабилен.

### Acceptance criteria

1. Из user detail можно отправить обычное сообщение MAX-only пользователю.
2. Из user detail можно отправить payment link MAX-only пользователю.
3. Audit events фиксируют, в какой messenger ушло сообщение.

## Finding 4: startup слишком хрупок к временному timeout Telegram API

Приоритет: P2

### Симптом

В начале лога есть:
- `create app server failed`
- `getMe ... context deadline exceeded`

А потом при следующем старте все прошло нормально.

### Root cause

Startup health check сейчас fail-fast на единичный сетевой timeout Telegram API.
Это правильно для hard failure, но слишком чувствительно для временного сетевого дребезга.

### Исправление

1. Добавить небольшой retry/backoff на startup ping:
   - Telegram `getMe`
   - MAX `/me`
2. Развести типы ошибок:
   - invalid token -> fail hard сразу;
   - temporary timeout / network error -> 2-3 retries before abort.

### Acceptance criteria

1. Единичный Telegram timeout не валит cold start мгновенно.
2. Битый токен по-прежнему валит startup сразу и явно.

## Finding 5: прод торчит наружу и активно сканируется

Приоритет: P2

### Симптом

В логах много внешнего мусора:
- `/wp-admin/setup-config.php`
- `/vendor/phpunit/.../eval-stdin.php`
- `PROPFIND /`
- `/ReportServer`
- `/containers/json`

### Вывод

Это не баг приложения, но это нормальный сигнал, что публичный endpoint активно сканируют.

### Что сделать

1. На reverse proxy/Nginx добавить rate limiting.
2. Поставить deny/cheap reject rules для типичных scan paths.
3. Добавить отдельный access-log filter или metrics counter по suspicious paths.
4. Проверить, что `/admin` не светится лишними заголовками и не доступен без auth beyond login page.

## Приоритетный план работ

### Этап 1. Срочно

1. Исправить short-period recurring strategy.
2. Добавить тесты на `2 min` connector:
   - первая оплата;
   - scheduler pass до конца периода;
   - один rebill, а не cascade.
3. Повторить live-money smoke после фикса.

### Этап 2. Следом

1. Перевести admin `send message` на messenger-neutral path.
2. Перевести admin `send payment link` на messenger-neutral path.
3. Обновить user detail actions для MAX-only users.

### Этап 3. UI cleanup

1. Почистить Telegram-first filters/columns в:
   - users
   - billing
   - churn
   - events
2. Показать MAX identity так же явно, как Telegram.

### Этап 4. Ops hardening

1. Retry/backoff на transport startup pings.
2. Reverse-proxy hardening против сканеров.

## Нужные проверочные сценарии после фиксов

### A. Telegram short-period recurring
- connector: `2 min`, `autopay on`
- первая оплата
- ждать scheduler pass
- убедиться, что rebill не создается мгновенно
- убедиться, что нет immediate reminder + expiry notice пары
- дождаться rebill window
- получить ровно один recurring payment

### B. MAX short-period recurring
- тот же сценарий в MAX
- проверить delivery:
  - consent
  - success
  - reminder/expiry
  - expired

### C. Admin actions on MAX-only user
- send message
- send payment link
- проверить audit trail

## Решение по продукту

До фикса short-period scheduler logic:
- **не считать текущий recurring smoke на `1-2 min` валидным продуктовым тестом**;
- **не использовать результат “вторая подписка создалась” как сигнал поломки базовой месячной прод логики**.

Это именно конфликт тестового короткого периода с production day-based recurring windows.
