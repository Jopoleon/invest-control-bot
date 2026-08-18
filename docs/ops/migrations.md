# Миграции БД

В проекте используется `github.com/rubenv/sql-migrate`.

## Как работает сейчас
- SQL-файлы миграций лежат в `migrations/*.sql`.
- Формат файлов: блоки `-- +migrate Up` и `-- +migrate Down`.
- При старте сервиса, если `DB_WITH_MIGRATION=true`, вызывается `migrations.ApplyUp(...)`.
- История примененных миграций хранится в таблице `schema_migrations` в PostgreSQL.

## Up/Down паттерн
- `ApplyUp(db)` применяет все pending migration `Up`.
- `ApplyDown(db, max)` откатывает `max` миграций (если `max<=0`, откатывается 1).

## Примечания
- `Up` должен быть идемпотентным в рамках проектного стандарта (`IF NOT EXISTS` где применимо).
- `Down` должен безопасно откатывать изменения текущего файла миграции.
- Для production рекомендуется делать бэкап перед крупными `Up/Down` миграциями.

## Перенос между PostgreSQL major versions

При переезде VPS с PostgreSQL 14 на PostgreSQL 18 нельзя копировать data-dir.
Используется logical custom dump/restore:

```bash
pg_dump --format=custom --no-owner --no-privileges
pg_restore --exit-on-error --single-transaction \
  --no-owner --no-privileges --role=investcontrol_app
```

Перед cutover обязательна disposable rehearsal. Проверяются checksum,
`pg_restore --list`, encoding/locale/extensions, migration IDs, counts всех
таблиц, sequences, indexes, constraints и ownership. PostgreSQL 18 отдельно
представляет `NOT NULL` как constraint type `n`, поэтому сырой общий count
constraints между 14 и 18 сравнивать нельзя.

Production cutover PostgreSQL 14.23→18.4 выполнен 2026-08-12 этим способом.
Final archive checksum и source/target manifests совпали; target DB явно
сохраняет source timezone `Europe/Moscow`. Подробности и rollback state — в
`docs/ops/airnet-server-migration.md`.
