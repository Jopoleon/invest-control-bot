# План: новая period model для коннекторов

Статус: completed
Дата: 2026-04-01

## Статус внедрения на 2026-04-01

Результат:
- additive migration `migrations/0003_connector_period_model.sql`;
- новые поля в `domain.Connector`:
  - `period_mode`
  - `period_seconds`
  - `period_months`
  - `fixed_ends_at`;
- canonical resolver в `SubscriptionEndsAt(...)`;
- runtime support в recurring/lifecycle:
  - short duration считается напрямую через `period_mode = duration`;
  - `fixed_deadline` исключен из recurring/autopay path;
- admin create flow переведен на новую period form:
  - `duration`
  - `calendar_months`
  - `fixed_deadline`.
- baseline schema `0001_init.sql` переписана под final shape;
- `0002` и `0003` переведены в historical no-op;
- `0004_drop_legacy_connector_period_fields.sql` удаляет `period_days` и `test_period_seconds` из существующих БД.

Закрыто:
- legacy read/write path через `period_days` / `test_period_seconds`;
- legacy runtime branching в bot/app/admin code;
- зависимость short-period smoke tests от специальных test-period полей.

## Зачем

Текущая модель периода у коннектора исторически слишком узкая:
- основной business путь завязан на `period_days`;
- для коротких smoke-тестов recurring добавлен отдельный `test_period_seconds` override;
- этого достаточно как временное решение, но это не финальная чистая модель.

Нам нужна period model, которая одинаково естественно покрывает:
1. обычные подписки по длительности;
2. тарифы на календарный месяц;
3. доступ до конкретной даты;
4. короткие тестовые периоды без special-case логики в данных.

## Цели

1. Уйти от implicit приоритета `period_days` как единственной формы периода.
2. Сохранить точную модель подписки через `starts_at` / `ends_at`.
3. Поддержать три предметных сценария period policy:
- relative duration;
- calendar months;
- fixed deadline.
4. Упростить recurring/lifecycle rules так, чтобы они считались от реальной period policy, а не от исторического day-based proxy.

## Важное разделение

### Что уже правильно сейчас

`subscription` уже хранит точные границы периода:
- `starts_at`
- `ends_at`

Это правильная модель.

### Что нужно поменять

Менять надо не `subscription`, а `connector period policy`.

То есть вопрос не в том, как хранить активную подписку пользователя.
Вопрос в том, как из коннектора вычислять `ends_at` для нового периода.

## Предлагаемая period model

Добавить у коннектора следующие поля:
- `period_mode`
- `period_seconds`
- `period_months`
- `fixed_ends_at`

### `period_mode`

Разрешенные значения:
- `duration`
- `calendar_months`
- `fixed_deadline`

### `duration`

Используется для периодов вида:
- `120s`
- `30m`
- `7d`
- `30d`

В этом режиме:
- используется `period_seconds > 0`
- `period_months = 0`
- `fixed_ends_at = null`

Семантика:
- `ends_at = start + period_seconds`

Подходит для:
- тестовых коротких подписок;
- тарифов на фиксированную длительность;
- спецпериодов в часах/днях.

### `calendar_months`

Используется для периодов вида:
- `1 month`
- `3 months`
- `12 months`

В этом режиме:
- используется `period_months > 0`
- `period_seconds = 0`
- `fixed_ends_at = null`

Семантика:
- `ends_at = start.AddDate(0, period_months, 0)`

Это важно, потому что:
- `30 days` и `1 calendar month` не одно и то же;
- если продукт продает именно месяц, это должна быть first-class business semantics, а не approximation.

### `fixed_deadline`

Используется для сценария:
- доступ до конкретной даты и времени

В этом режиме:
- используется `fixed_ends_at != null`
- `period_seconds = 0`
- `period_months = 0`

Семантика:
- `ends_at = fixed_ends_at`
- `starts_at = payment confirmed at`

Подходит для:
- сезонных кампаний;
- тарифов “до конца месяца”;
- доступов “до даты запуска / до экзамена / до дедлайна”.

## Что делать с recurring

Recurring не должен быть одинаково разрешен для всех режимов.

### Разрешить recurring для
- `duration`
- `calendar_months`

### По умолчанию запретить recurring для
- `fixed_deadline`

Причина:
- автосписание для доступа “до конкретной даты” обычно не имеет бизнес-смысла;
- такие тарифы чаще всего одноразовые и кампанийные.

Если когда-нибудь понадобится recurring + fixed deadline, это должен быть отдельный осознанный сценарий, а не implicit разрешение.

## Валидация period model

### Для `duration`

Обязательно:
- `period_seconds > 0`

Запрещено:
- `period_months > 0`
- `fixed_ends_at != null`

### Для `calendar_months`

Обязательно:
- `period_months > 0`

Запрещено:
- `period_seconds > 0`
- `fixed_ends_at != null`

### Для `fixed_deadline`

Обязательно:
- `fixed_ends_at != null`

Запрещено:
- `period_seconds > 0`
- `period_months > 0`

## Переход с текущей модели

Сейчас в модели уже есть:
- `period_days`
- `test_period_seconds`

Это временно рабочая комбинация, но не финальная.

### Правильный migration path

1. Добавить новые поля additive migration'ом.
2. Оставить runtime backward compatibility.
3. Перевести вычисление `SubscriptionEndsAt(...)` на новую policy.
4. Постепенно перевести admin UI и создание/редактирование коннектора.
5. После стабилизации удалить legacy path:
- `period_days`
- `test_period_seconds`

## Совместимость на переходный период

На переходном этапе resolver должен работать так:

1. Если заполнена новая period model, использовать ее.
2. Иначе fallback:
- если `test_period_seconds > 0`, трактовать как `duration`
- иначе `period_days > 0` трактовать как legacy duration в днях
- иначе default на `30d`

Это даст безопасный migration path без массового одномоментного переписывания данных.

## Влияние на runtime

### Domain model

Нужен новый resolver period policy у `Connector`.

Например:
- `SubscriptionEndsAt(start time.Time) time.Time`
- но уже на новой period model

### Recurring

Recurring windows должны считаться не от исторического `days`, а от реальной period length.

Для этого нужна функция уровня policy, которая умеет возвращать:
- `effective period duration`, если она вычислима;
- или признак, что period fixed/non-recurring.

### Lifecycle

Reminder / expiry / rebill strategy должны строиться от `period_mode`, а не от legacy day assumptions.

Примеры:
- `duration 120s` -> short windows;
- `duration 30d` -> day-based windows;
- `calendar_months 1` -> обычные production windows;
- `fixed_deadline` -> чаще всего без recurring.

## Влияние на админку

Форма коннектора должна перестать быть “period_days + hidden test field”.

Нужен явный UX:
- `Period type`
- поля в зависимости от режима

### Пример UI

1. `Duration`
- поле длительности: `120s`, `15m`, `30d`

2. `Calendar months`
- поле количества месяцев: `1`, `3`, `12`

3. `Fixed deadline`
- поле даты/времени окончания

На переходном этапе это может жить как advanced settings refactor, но конечная форма должна быть именно такой.

## Что делать с текущими short-period smoke tests

После перехода на новую period model:
- короткий recurring smoke перестанет быть special-case data hack;
- это будет обычный `period_mode = duration` с маленьким `period_seconds`.

Тогда текущий short-period scheduler path уже можно будет переосмыслить как часть общей duration strategy, а не как отдельную тестовую ветку.

## Предлагаемая схема БД

Минимально:
- `period_mode text`
- `period_seconds bigint not null default 0`
- `period_months integer not null default 0`
- `fixed_ends_at timestamptz null`

Legacy временно оставить:
- `period_days integer`
- `test_period_seconds integer`

## Примеры данных

### Обычный короткий тест
- `period_mode = duration`
- `period_seconds = 120`

### Подписка на 30 дней
- `period_mode = duration`
- `period_seconds = 2592000`

### Подписка на 1 календарный месяц
- `period_mode = calendar_months`
- `period_months = 1`

### Доступ до конца года
- `period_mode = fixed_deadline`
- `fixed_ends_at = 2026-12-31T23:59:59Z`

## Риски

1. Если сразу заменить `period_days`, можно сломать существующие коннекторы.
2. Если не ограничить recurring на `fixed_deadline`, можно получить странные бизнес-сценарии.
3. Если админка не покажет period mode явно, оператор будет путаться сильнее, чем сейчас.

## Рекомендуемый порядок внедрения

### Этап 1
- schema proposal утверждение
- additive migration
- domain fallback resolver

### Этап 2
- runtime `SubscriptionEndsAt(...)` на новой policy
- recurring/lifecycle adaptation
- unit tests

### Этап 3
- admin connectors form redesign под new period model
- data backfill / migration script if needed

### Этап 4
- cleanup legacy fields
- rewrite clean bootstrap schema when transition is proven

## Итог

Правильная целевая модель не должна быть просто “по дням” и не должна быть только “до конкретной даты”.

Нужна трехрежимная period policy:
- `duration`
- `calendar_months`
- `fixed_deadline`

Это даст:
- честную поддержку обычных подписок;
- честную поддержку календарных месяцев;
- честную поддержку campaign/deadline access;
- и уберет временный разрыв между тестовыми короткими периодами и нормальной предметной моделью.
