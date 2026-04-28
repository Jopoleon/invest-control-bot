# REFACTORING AND TEST PLAN

Последнее обновление: 2026-03-27

## Назначение

Этот документ фиксирует ближайший технический план по:
- наращиванию unit-test покрытия;
- упрощению сложных use-case слоев;
- выносу повторяющегося кода в общие helper/service abstractions;
- последовательному cleanup без массового risky rewrite.

Документ не заменяет:
- `IMPLEMENTATION_PLAN.md` как основной roadmap проекта;
- `docs/max/implementation.md` как канал-специфичный план по MAX.

Он нужен как отдельный рабочий backlog для инженерной доработки кода.

## Принципы

1. Рефакторинг и тесты идут вместе.
2. Telegram-продукт не ломаем ради абстракций.
3. Multi-messenger путь развиваем incrementally, без массовой замены всех `telegram_id` за один проход.
4. Выносить только тот код, который реально убирает дублирование или снижает сложность.
5. Не строить общий framework ради абстракции без явного payoff.

## Приоритеты

### High

1. `internal/app/payment_handlers.go`
- покрыть success/fail pages для Telegram и MAX;
- покрыть `buildPaymentPageActions`;
- покрыть `notifyFailedRecurringPayment`;
- проверить fallback cases:
  - нет `channel_url`;
  - нет messenger account;
  - fallback на legacy external id.

2. `internal/app/payment_flow.go`
- покрыть повторный callback на уже оплаченный payment;
- покрыть правило продления подписки от `max(now, current ends_at)`;
- покрыть сценарии с `channel_url` и без него;
- покрыть messenger-aware success notification.

3. `internal/app/recurring_pages.go`
- покрыть истекший cancel token;
- покрыть POST с чужой subscription;
- покрыть already-disabled subscription;
- покрыть mixed-mode user name resolution;
- покрыть success redirect/banner cases.

4. `internal/app/subscription_lifecycle.go`
- reminder / expiry / expired сценарии для Telegram и MAX;
- поведение без connector;
- поведение без start payload;
- verify: revoke from chat only for Telegram;
- verify: mark fields not causing duplicate sends.

5. `internal/bot`
- покрыть `handleCallback` как routing table;
- покрыть `handlePay` для recurring on/off и missing-docs scenarios;
- покрыть `sendSubscriptionOverview`;
- покрыть `sendPaymentHistory`;
- покрыть `sendExistingSubscriptionMessage` отрицательными кейсами.

### Medium

1. `internal/max`
- integration-like tests для webhook handler;
- callback payload odd-shape cases;
- `bot_started` / deeplink payload parsing после финального MAX parity.

2. `internal/app/payment_handlers.go`
- вынести parsing Robokassa form fields в helper:
  - `parseRobokassaResultForm`
  - `parseRobokassaSuccessForm`
  - `parseRobokassaFailForm`

3. `internal/app`
- собрать user-facing notification builders в отдельный слой:
  - payment success;
  - failed recurring;
  - reminder;
  - expiry notice;
  - expired notice.

4. `internal/app`
- вынести messenger-aware delivery logic в более явный service/helper слой:
  - preferred messenger resolution;
  - fallback to legacy external id;
  - send path selection.

### Low

1. MAX runtime/docs cleanup
- MAX runtime должен оставаться webhook/server path;
- если снова понадобится transport-level debug runner, заводить его уже как явный `internal/devtools` артефакт, а не как отдельный `cmd/*`.

2. `internal/bot/menu.go`
- можно разрезать на smaller files:
  - `menu_subscription.go`
  - `menu_payments.go`
  - `menu_autopay.go`
- делать только если файл продолжит разрастаться после ближайших покрытий тестами.

## Кандидаты на дедупликацию

### 1. Connector + legal context builders

Повторяющаяся логика встречается в:
- `internal/bot/start.go`
- `internal/app/recurring_pages.go`
- `internal/app/payment_flow.go`

Кандидат:
- helper, который собирает:
  - connector;
  - offer URL;
  - privacy URL;
  - agreement URL;
  - resolved channel URL.

Цель:
- не дублировать legal/channel resolution в нескольких use-case слоях.

### 2. Notification template builders

Повторяющиеся шаблоны уже частично централизованы, но логика сборки still spread.

Кандидат:
- `internal/app/notification_templates.go`

Цель:
- общий формат для payment/lifecycle/public-page notifications;
- меньше дублирования при расширении MAX/Telegram parity.

### 3. Public recurring cancel page assembly

`buildRecurringCancelPageData` уже крупный и делает сразу:
- auth/context state;
- load subscriptions;
- resolve display user;
- map subscriptions to view;
- success/error banner state.

Кандидат:
- разрезать на 2-4 smaller helpers.

Цель:
- улучшить читаемость;
- сделать тесты на pure mapping проще.

### 4. Payment callback HTTP parsing vs side effects

Сейчас в `payment_handlers.go` HTTP parsing и business side effects связаны довольно плотно.

Кандидат:
- отделить:
  - request parsing;
  - payment verification;
  - post-payment side effects.

Цель:
- сделать handler тоньше;
- упростить тесты без лишнего HTTP boilerplate.

## Архитектурные рефакторинги после ближайшего покрытия

### 1. `subscription_lifecycle` как service

Текущие функции уже рабочие, но service-объект упростит:
- dependency injection;
- isolated tests;
- future multi-messenger extensions.

Рекомендуемый timing:
- после усиления тестов на текущий lifecycle behavior.

### 2. `internal/bot` onboarding/payment orchestration

Регистрация, consent и payment preparation сейчас размазаны по handler methods.

Кандидат:
- один use-case service для onboarding flow.

Рекомендуемый timing:
- только после покрытия основных callback/message branches.

### 3. Unified messenger delivery service

Сейчас mixed-mode delivery logic уже работает, но still spread between app/bot helpers.

Кандидат:
- единый слой delivery resolution.

Рекомендуемый timing:
- после стабилизации recurring pages + lifecycle + MAX webhook path.

## Что не делать сейчас

1. Не делать mass schema rewrite `telegram_id -> user_id`.
2. Не совмещать backend refactoring с frontend/template overhaul.
3. Не выносить каждый маленький helper в отдельный пакет.
4. Не переписывать admin глубже текущих multi-messenger нужд.
5. Не трогать clean-schema/migration reset до завершения behavioral stabilization.

## Рекомендуемый порядок работ

1. Усилить тесты в:
- `internal/app/payment_handlers.go`
- `internal/app/payment_flow.go`
- `internal/app/recurring_pages.go`

2. Потом усилить тесты в:
- `internal/app/subscription_lifecycle.go`
- `internal/bot` critical menu/payment branches

3. После покрытия:
- вынести connector/legal context helpers;
- вынести notification builders.

4. Затем:
- решить, нужен ли service-refactor для lifecycle;
- решить, нужен ли deeper split `internal/bot/menu.go`.

5. После ближайшего milestone:
- оценить, что осталось до clean-schema этапа;
- проверить, не осталось ли в runtime скрытых допущений про отдельный MAX polling runner.
