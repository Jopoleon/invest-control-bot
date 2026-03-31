# План рефакторинга `internal/app`

Статус: active  
Дата: 2026-03-31

## Зачем

Текущий пакет `internal/app` уже выполняет слишком много ролей одновременно:
- composition root
- HTTP routing
- payment callbacks и payment result pages
- recurring public pages
- recurring rebill orchestration
- subscription lifecycle jobs
- app-level notification delivery
- app-level audit helpers
- landing / legal / static pages

Из-за этого сейчас страдают сразу четыре цели:
1. код избыточен и дублируется;
2. структура пакета тяжело читается;
3. тестирование требует слишком много скрытого контекста;
4. расширение под новые use-case зоны и transport/payment paths становится дороже.

## Цели текущего цикла

1. Уменьшить объем кода за счет явных use-case boundaries и выноса повторяющейся логики.
2. Сделать `internal/app` организованным по предметным зонам, а не по случайным file groups.
3. Повысить тестируемость за счет concrete services с узкими зависимостями.
4. Подготовить пакет к безопасному расширению без жирного `*application`.

## Чего НЕ делаем

1. Не вводим интерфейсы на все подряд.
2. Не создаем пакеты `utils`, `helpers`, `misc`, `common`.
3. Не режем пакет только ради красивого дерева файлов.
4. Не переписываем одновременно все HTTP/admin/bot/store слои.

## Базовые правила

1. `application` должен остаться composition root, а не глобальным receiver для всей бизнес-логики.
2. Новые зоны логики выносятся в concrete service structs с явными зависимостями.
3. Интерфейсы добавляем только на реальных внешних границах или там, где это реально упрощает тест.
4. Если кусок логики можно выразить как pure function, он должен быть pure function.
5. Каждый шаг рефакторинга должен оставлять `go test ./...` зеленым.

## Правила именования будущих пакетов

Разрешены только предметные названия по зоне ответственности:
- `payments`
- `recurring`
- `subscriptions`
- `delivery` или `notifications` — только если это станет самостоятельной областью

Не используем:
- `utils`
- `helpers`
- `misc`
- `common`

Если пакет нельзя назвать предметно, код пока не созрел для выноса.

## Целевая форма `internal/app`

### Корень `internal/app`

Оставляем:
- `application.go`
- `server.go`
- `routes.go`
- `store_open.go`
- HTTP/root wiring

Роль:
- собрать зависимости
- собрать router
- поднять runtime
- связать сервисы между собой

### Будущие зоны

1. `internal/app/payments`
- payment result/fail/success handlers
- payment activation flow
- provider callback orchestration
- rebill result handling, если это payment-centered logic

2. `internal/app/recurring`
- public recurring checkout/cancel pages
- recurring-specific page data builders
- recurring rebill orchestration, если не захотим держать его в `payments`

3. `internal/app/subscriptions`
- reminder / expiry / revoke lifecycle
- subscription state transitions после окончания периода

## Промежуточный шаг перед физическим распилом

Сначала не подпакеты, а services внутри `internal/app`.

## Текущий прогресс

### 2026-03-31

Сделан первый рабочий срез `paymentRuntime` и затем физический перенос payment business logic в `internal/app/payments`:
- payment-specific orchestration сначала вынесен с `*application` в `paymentRuntime`;
- mock checkout / mock success handlers тоже перенесены в `paymentRuntime`;
- затем активация успешной оплаты и уведомление о проваленном recurring-платеже физически вынесены в пакет [payments](/home/egor/Work/src/github.com/Jopoleon/invest-control-bot/internal/app/payments);
- у нового пакета есть `doc.go`, package-level tests и реальное runtime wiring из корневого `internal/app`;
- `payment_handlers.go`, `payment_flow.go` и `mock_handlers.go` оставлены как thin wrappers для совместимости текущих call sites и тестов;
- `go test ./...` после этого шага остается зеленым.

Что пока осознанно НЕ сделано:
- не убраны все compatibility wrappers с `*application`;
- payment status pages и HTTP-only DTO пока остаются в корневом `internal/app`;
- payment page action rendering пока еще не вынесен из корневого пакета.

Сделан первый рабочий срез `recurringRuntime`:
- public recurring checkout/cancel flow вынесен с `*application` в `recurringRuntime`;
- page data builders и recurring cancel page orchestration теперь живут в одном concrete runtime;
- rebill orchestration и scheduled rebill eligibility тоже перенесены в `recurringRuntime`;
- `recurring_pages.go` оставлен как thin wrapper для маршрутов;
- `recurring_rebill.go` оставлен как thin wrapper для текущих call sites и scheduler integration;
- `go test ./...` после этого шага остается зеленым.

Что пока осознанно НЕ сделано:
- lifecycle scheduler еще не отделен от `*application`;
- checkout page и template rendering все еще остаются в корневом `internal/app`.

Сделан первый физический перенос в `internal/app/recurring`:
- rebill orchestration и scheduled rebill eligibility вынесены в [recurring] package;
- business state публичной страницы отключения автоплатежа тоже вынесен в [recurring] package;
- в новом пакете есть `doc.go` и прямые package-level tests;
- корневой `internal/app` теперь использует этот пакет через thin wrappers;
- HTTP routing, token parsing и template rendering пока временно остаются в корневом `internal/app`, чтобы не смешивать page/template refactor с business-service split.

Сделан первый рабочий срез `subscriptionLifecycleRuntime`:
- reminder / expiry notice / expiration processing вынесены с `*application` в `subscriptionLifecycleRuntime`;
- renewal notification builder теперь тоже живет в lifecycle runtime;
- `subscription_lifecycle.go` оставлен как thin слой для scheduler wiring, wrappers и package-level helper functions;
- `go test ./...` после этого шага остается зеленым.

Что пока осознанно НЕ сделано:
- scheduler orchestration еще не вынесен из корневого `internal/app`;
- HTTP/job wiring все еще остается в корневом `internal/app`, хотя бизнес-логика уже вынесена в `internal/app/subscriptions`.

### Шаг 1. `paymentRuntime`

Перенести с `*application`:
- `handlePaymentResult`
- `handlePaymentSuccess`
- `handlePaymentFail`
- `handlePaymentRebill`
- `activateSuccessfulPayment`

Зависимости:
- `store.Store`
- `payment.Service` / `*payment.RobokassaService`
- messenger delivery path
- config fragments, которые реально нужны payment flow

### Шаг 2. `recurringRuntime`

Перенести:
- `handleRecurringCheckout`
- `handleRecurringCancel`
- recurring page builders
- rebill trigger orchestration, если оно логически ближе к recurring UX

Зависимости:
- `store.Store`
- public URL / encryption bits
- bot launch / messenger launch context

### Шаг 3. `subscriptionLifecycleRuntime`

Перенести:
- reminder job
- expiry notice job
- expired subscription processing
- transport-specific revoke path

Зависимости:
- `store.Store`
- `telegram.Client`
- `messenger.Sender`
- notification and audit helpers

## Критерии успеха

### Этап A
- `application` перестает быть владельцем payment business logic.
- payment-related tests не требуют сборки всего app world.

### Этап B
- recurring public pages и rebill path живут в отдельной зоне ответственности.
- recurring tests покрывают business branches без лишнего HTTP/runtime шума.

### Этап C
- lifecycle scheduler отделен от page/payment logic.
- reminder/expiry/revoke тестируются как отдельный сервис.

### Этап D
- после стабилизации services физически переносим их в подпакеты.
- package boundaries остаются предметными и не требуют `utils/helpers/misc`.

## Точки контроля

На каждом этапе проверяем:
1. стало ли меньше зависимостей у одного receiver/service;
2. уменьшилось ли количество скрытых полей/связей;
3. выросла ли простота unit-тестов;
4. исчез ли реальный повтор, а не просто переместился в другой файл.

## Что трекаем отдельно

1. Не потерять current business invariants:
- payment success only by callback
- recurring rebill success only by result callback
- extend from `max(now, current ends_at)`

2. Не сломать transport-neutral path:
- никакого второго MAX-only business flow

3. Не скрыть спорные участки:
- оставлять `TODO:` там, где short test periods или lifecycle windows еще не доведены

## Порядок работ

1. `paymentRuntime`
2. `recurringRuntime`
3. `subscriptionLifecycleRuntime`
4. после этого — физический перенос в подпакеты

Это и есть текущий рекомендуемый путь для следующего цикла рефакторинга `internal/app`.
