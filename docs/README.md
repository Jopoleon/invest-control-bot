# Документация Проекта

Документация разложена по назначению. Для обычного входа в проект сначала
смотреть [README.md](/home/egor/Work/src/github.com/Jopoleon/invest-control-bot/README.md).
Для Codex/AI-сессий главным контрактом остается
[AGENTS.md](/home/egor/Work/src/github.com/Jopoleon/invest-control-bot/AGENTS.md).

## Быстрый Порядок Чтения Для AI/Codex

1. [context/project-context.md](/home/egor/Work/src/github.com/Jopoleon/invest-control-bot/docs/context/project-context.md)
2. [context/progress.md](/home/egor/Work/src/github.com/Jopoleon/invest-control-bot/docs/context/progress.md)
3. [context/session-log.md](/home/egor/Work/src/github.com/Jopoleon/invest-control-bot/docs/context/session-log.md)
4. Latest session log under [context/session-log/](/home/egor/Work/src/github.com/Jopoleon/invest-control-bot/docs/context/session-log)
5. [planning/implementation-plan.md](/home/egor/Work/src/github.com/Jopoleon/invest-control-bot/docs/planning/implementation-plan.md)
6. Relevant `OVERVIEW.md` for the touched code zone
7. Payment/recurring/messenger/ops docs relevant to the task

## Context / Project Memory

- [context/project-context.md](/home/egor/Work/src/github.com/Jopoleon/invest-control-bot/docs/context/project-context.md) - durable facts: what the system is, architecture, invariants, source-of-truth rules.
- [context/progress.md](/home/egor/Work/src/github.com/Jopoleon/invest-control-bot/docs/context/progress.md) - current checkpoint, active risks, next concrete tasks.
- [context/session-log.md](/home/egor/Work/src/github.com/Jopoleon/invest-control-bot/docs/context/session-log.md) - chronological session-log index and rules.
- [context/session-log/session-log-01-current.md](/home/egor/Work/src/github.com/Jopoleon/invest-control-bot/docs/context/session-log/session-log-01-current.md) - current session chronology file.

## Planning

- [planning/implementation-plan.md](/home/egor/Work/src/github.com/Jopoleon/invest-control-bot/docs/planning/implementation-plan.md) - current structured planning entrypoint.
- [../IMPLEMENTATION_PLAN.md](/home/egor/Work/src/github.com/Jopoleon/invest-control-bot/IMPLEMENTATION_PLAN.md) - historical/root implementation roadmap and older decisions.

## Product / Payments

- [payments/flow-ru.md](/home/egor/Work/src/github.com/Jopoleon/invest-control-bot/docs/payments/flow-ru.md) - простое описание платежей, подписок и автоплатежей.
- [payments/robokassa-recurring.md](/home/egor/Work/src/github.com/Jopoleon/invest-control-bot/docs/payments/robokassa-recurring.md) - Robokassa recurring checklist и текущие требования.

## Ops

- [ops/admin-guide.md](/home/egor/Work/src/github.com/Jopoleon/invest-control-bot/docs/ops/admin-guide.md) - операторский гайд по админке.
- [ops/migrations.md](/home/egor/Work/src/github.com/Jopoleon/invest-control-bot/docs/ops/migrations.md) - как устроены миграции.
- [ops/vercel.md](/home/egor/Work/src/github.com/Jopoleon/invest-control-bot/docs/ops/vercel.md) - заметки по Vercel runtime.
- [ops/prod-postgres-mcp.md](/home/egor/Work/src/github.com/Jopoleon/invest-control-bot/docs/ops/prod-postgres-mcp.md) - Codex MCP доступ к prod PostgreSQL через SSH tunnel.
- [ops/airnet-server-migration.md](/home/egor/Work/src/github.com/Jopoleon/invest-control-bot/docs/ops/airnet-server-migration.md) - исследование и runbook переезда production на новый VPS.
- [ops/domain-dns-cutover.md](/home/egor/Work/src/github.com/Jopoleon/invest-control-bot/docs/ops/domain-dns-cutover.md) - DNS, новый домен и связанные webhook/TLS настройки для `airnet-server`.

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
- [backlog/telegram-chat-id-problem.md](/home/egor/Work/src/github.com/Jopoleon/invest-control-bot/docs/backlog/telegram-chat-id-problem.md) - Telegram chat-id, native `t.me` access links, implemented safety boundary and remaining `chat_shared` UX.

## Zone Overviews

Короткие `OVERVIEW.md` помогают быстро понять ownership и invariants зоны кода:

- [../internal/app/OVERVIEW.md](/home/egor/Work/src/github.com/Jopoleon/invest-control-bot/internal/app/OVERVIEW.md)
- [../internal/bot/OVERVIEW.md](/home/egor/Work/src/github.com/Jopoleon/invest-control-bot/internal/bot/OVERVIEW.md)
- [../internal/payment/OVERVIEW.md](/home/egor/Work/src/github.com/Jopoleon/invest-control-bot/internal/payment/OVERVIEW.md)
- [../internal/store/OVERVIEW.md](/home/egor/Work/src/github.com/Jopoleon/invest-control-bot/internal/store/OVERVIEW.md)

## Templates

- [templates/ai-project-memory-prompt.md](/home/egor/Work/src/github.com/Jopoleon/invest-control-bot/docs/templates/ai-project-memory-prompt.md) - reusable prompt for setting up this style of AI/Codex project memory in another repository.

## Что Обновлять После Работы

- Meaningful code/product changes: append the latest session log.
- Current status or next tasks changed: update `docs/context/progress.md`.
- Durable project fact changed: update `docs/context/project-context.md`.
- Recurring/payment behavior changed: update `docs/payments/*`.
- Messenger identity/transport boundaries changed: update architecture docs and context.
- Migration behavior changed: update `docs/ops/migrations.md` and relevant context.

## Archive

Архив не является текущей инструкцией. Он нужен только для восстановления контекста расследований.

- [archive/incidents/prod-recurring-2026-04-01.md](/home/egor/Work/src/github.com/Jopoleon/invest-control-bot/docs/archive/incidents/prod-recurring-2026-04-01.md)
- [archive/handoffs/2026-04-02-recurring.md](/home/egor/Work/src/github.com/Jopoleon/invest-control-bot/docs/archive/handoffs/2026-04-02-recurring.md)

Удалены как неактуальные дубли:
- `docs/PROD_DEBUG_PLAN_2026-04-01.md` - поглощен archived incident/handoff.
- `docs/ITERATION_0.md` и `docs/ITERATION_2.md` - ранние исторические итерации, которые уже не описывали текущий продукт.
codex --sandbox workspace-write
