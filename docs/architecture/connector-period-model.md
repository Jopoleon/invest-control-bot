# Модель Периодов Коннектора

Статус: актуально  
Дата актуализации: 2026-04-28

## Текущее Состояние

Период доступа у коннектора задается канонической моделью:

- `period_mode`
- `period_seconds`
- `period_months`
- `fixed_ends_at`

Legacy-поля `period_days` и `test_period_seconds` удалены из runtime-модели и не должны возвращаться в код, админку или новые документы.

Короткие live-money smoke-тесты recurring теперь являются обычными `duration`-коннекторами с малым `period_seconds`, а не отдельным test-period override.

## Режимы

### `duration`

Используется для фиксированной длительности:

- `120s`
- `15m`
- `30d`

Данные:

- `period_mode = duration`
- `period_seconds > 0`
- `period_months = 0`
- `fixed_ends_at = null`

Семантика:

- `ends_at = start + period_seconds`

Recurring разрешен.

Для коротких duration-периодов timing policy берется из `internal/app/periodpolicy`, а не из обычных production окон `72h/48h/24h`.

### `calendar_months`

Используется для календарных месяцев:

- `1 month`
- `3 months`
- `12 months`

Данные:

- `period_mode = calendar_months`
- `period_months > 0`
- `period_seconds = 0`
- `fixed_ends_at = null`

Семантика:

- `ends_at = start.AddDate(0, period_months, 0)`

Recurring разрешен.

Этот режим нужен, потому что `30 days` и `1 calendar month` не являются одинаковым продуктовым обещанием.

### `fixed_deadline`

Используется для доступа до конкретной даты:

- сезонный доступ
- кампания до дедлайна
- доступ до конца курса или набора

Данные:

- `period_mode = fixed_deadline`
- `fixed_ends_at != null`
- `period_seconds = 0`
- `period_months = 0`

Семантика:

- `ends_at = fixed_ends_at`

Recurring намеренно запрещен. Автосписание для доступа до фиксированной даты должно появляться только как отдельный осознанный продуктовый сценарий, а не как побочный эффект.

## Где Реализовано

- `internal/domain/models.go` - доменная модель и расчет `SubscriptionEndsAt`.
- `internal/admin/connectors.go` - парсинг и валидация формы коннектора.
- `internal/app/periodpolicy` - timing policy для recurring/lifecycle, включая короткие duration-периоды.
- `internal/store/postgres/connectors.go` - чтение и запись новых полей.
- `migrations/0001_init.sql` - canonical bootstrap schema.
- `migrations/0004_drop_legacy_connector_period_fields.sql` - cleanup старых колонок в существующих БД.

## Инварианты

1. `fixed_deadline` не участвует в recurring.
2. `duration` и `calendar_months` могут участвовать в recurring, если включен соответствующий payment/provider flow.
3. Short-period smoke periods задаются как `duration`, например `period_seconds = 180`.
4. Subscription period хранится как точные `starts_at` / `ends_at`; connector period model нужна только для вычисления нового периода.
5. При продлении подписки действует правило проекта: продлевать от `max(now, current period end)`, где это применимо.

## Admin UX

Форма коннектора должна показывать оператору предметные варианты:

- фиксированный срок доступа;
- ежемесячный автоплатеж / календарные месяцы;
- доступ до конкретной даты.

Короткие duration-периоды для smoke tests не должны выглядеть как обычный production тариф. Если UI дает оператору создать минуты/секунды, это должно быть явно оформлено как test/debug сценарий.

## Проверки

Основная проверка после изменений period model:

```bash
GOCACHE=/tmp/go-build go test ./...
```

При recurring-изменениях дополнительно смотреть:

- `internal/app/periodpolicy/policy_test.go`
- recurring service tests
- lifecycle tests
- Robokassa callback/rebill tests
