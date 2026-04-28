# Документация Проекта

Документация разложена по назначению. Актуальное состояние проекта сначала смотреть в [README.md](/home/egor/Work/src/github.com/Jopoleon/invest-control-bot/README.md) и [AGENTS.md](/home/egor/Work/src/github.com/Jopoleon/invest-control-bot/AGENTS.md).

## Product / Payments

- [payments/flow-ru.md](/home/egor/Work/src/github.com/Jopoleon/invest-control-bot/docs/payments/flow-ru.md) - простое описание платежей, подписок и автоплатежей.
- [payments/robokassa-recurring.md](/home/egor/Work/src/github.com/Jopoleon/invest-control-bot/docs/payments/robokassa-recurring.md) - Robokassa recurring checklist и текущие требования.

## Ops

- [ops/admin-guide.md](/home/egor/Work/src/github.com/Jopoleon/invest-control-bot/docs/ops/admin-guide.md) - операторский гайд по админке.
- [ops/migrations.md](/home/egor/Work/src/github.com/Jopoleon/invest-control-bot/docs/ops/migrations.md) - как устроены миграции.
- [ops/vercel.md](/home/egor/Work/src/github.com/Jopoleon/invest-control-bot/docs/ops/vercel.md) - заметки по Vercel runtime.
- [ops/prod-postgres-mcp.md](/home/egor/Work/src/github.com/Jopoleon/invest-control-bot/docs/ops/prod-postgres-mcp.md) - Codex MCP доступ к prod PostgreSQL через SSH tunnel.

## Architecture

- [architecture/connector-period-model.md](/home/egor/Work/src/github.com/Jopoleon/invest-control-bot/docs/architecture/connector-period-model.md) - актуальная модель периодов коннектора.
- [architecture/app-refactor.md](/home/egor/Work/src/github.com/Jopoleon/invest-control-bot/docs/architecture/app-refactor.md) - refactor plan для `internal/app`.
- [architecture/refactoring-and-tests.md](/home/egor/Work/src/github.com/Jopoleon/invest-control-bot/docs/architecture/refactoring-and-tests.md) - инженерный backlog по тестам и cleanup.
- [architecture/max-decomposition.md](/home/egor/Work/src/github.com/Jopoleon/invest-control-bot/docs/architecture/max-decomposition.md) - архитектурное разложение MAX/messenger-neutral слоя.

## MAX

- [max/implementation.md](/home/egor/Work/src/github.com/Jopoleon/invest-control-bot/docs/max/implementation.md) - текущий MAX track.
- [max/research.md](/home/egor/Work/src/github.com/Jopoleon/invest-control-bot/docs/max/research.md) - исследование MAX Bot API и исторические выводы.

## Compliance

- [compliance/data-russia.md](/home/egor/Work/src/github.com/Jopoleon/invest-control-bot/docs/compliance/data-russia.md) - рабочая памятка по ПДн РФ.

## Backlog

- [backlog/todo.md](/home/egor/Work/src/github.com/Jopoleon/invest-control-bot/docs/backlog/todo.md) - актуальный рабочий TODO.

## Archive

Архив не является текущей инструкцией. Он нужен только для восстановления контекста расследований.

- [archive/incidents/prod-recurring-2026-04-01.md](/home/egor/Work/src/github.com/Jopoleon/invest-control-bot/docs/archive/incidents/prod-recurring-2026-04-01.md)
- [archive/handoffs/2026-04-02-recurring.md](/home/egor/Work/src/github.com/Jopoleon/invest-control-bot/docs/archive/handoffs/2026-04-02-recurring.md)

Удалены как неактуальные дубли:
- `docs/PROD_DEBUG_PLAN_2026-04-01.md` - поглощен archived incident/handoff.
- `docs/ITERATION_0.md` и `docs/ITERATION_2.md` - ранние исторические итерации, которые уже не описывали текущий продукт.
codex --sandbox workspace-write