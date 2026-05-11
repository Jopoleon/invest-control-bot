# AI Project Memory Setup Prompt

Use this prompt in a new Codex/AI session when a project needs a durable
documentation and project-memory system.

```text
Ты запущен в новом проекте.

Задача: создать или привести в порядок проектную память и документацию для разработки через Codex/AI.

Цель не “написать красивую документацию”, а построить рабочую систему контекста, чтобы будущие AI-сессии быстро понимали проект, не теряли решения, не ломали инварианты и фиксировали результаты своей работы.

Работай аккуратно.

## Главные Правила

- Сначала изучи репозиторий read-only.
- Не меняй production code, tests, migrations, configs, templates или бизнес-логику, если задача только документационная.
- Не коммить без отдельной команды пользователя.
- Если worktree dirty, считай существующие изменения пользовательскими и не откатывай их.
- Не делай `git reset --hard`, `git checkout --`, destructive cleanup или удаление legacy-файлов.
- Не выдумывай факты. Если что-то не подтверждено кодом или существующими docs, помечай как `требует проверки`.
- Документация должна быть инженерной, конкретной, короткой и полезной.
- Durable facts, current status, decisions, runbooks и session chronology должны быть разделены.
- Не создавай много файлов ради “структуры”. Создавай только то, что реально поможет будущим сессиям.

## Сначала Изучи

1. `git status --short`
2. `AGENTS.md`, если есть
3. `README.md`, если есть
4. существующие `docs/`, если есть
5. planning/roadmap/backlog файлы, если есть
6. основные top-level директории
7. package/module files:
   - `go.mod`
   - `package.json`
   - `pyproject.toml`
   - `Cargo.toml`
   - или аналоги
8. тестовую структуру и команды
9. deployment/ops files, если есть
10. migrations/schema files, если есть

После чтения коротко сформулируй для себя:

- что это за проект;
- какие основные runtime/product flows;
- какие зоны production-sensitive;
- какие инварианты нельзя ломать;
- где source of truth;
- как запускать тесты;
- какие документы уже есть и что в них устарело.

## Нужно Создать Или Обновить

### 1. `AGENTS.md`

Главный контракт для будущих AI/Codex-сессий.

Если файл уже есть и он сильный, не переписывай его с нуля. Аккуратно дополни.

Включи:

- Purpose проекта;
- Mandatory Read Order For Agents;
- Working Rules;
- Repository Zones;
- Critical Invariants;
- Source Of Truth;
- Testing Policy;
- Documentation Discipline;
- Session Memory Discipline;
- что обновлять после meaningful work;
- запрет коммитить без отдельной команды пользователя;
- запрет откатывать пользовательские изменения;
- особенности проекта: payments, migrations, identity, security, data integrity, frontend, ops, если применимо.

Mandatory read order должен выглядеть примерно так:

1. `docs/context/project-context.md`
2. `docs/context/progress.md`
3. `docs/context/session-log.md`
4. latest file under `docs/context/session-log/`
5. `docs/planning/implementation-plan.md`
6. relevant `OVERVIEW.md` for touched code zone
7. relevant ADR/runbook/domain docs for the task

### 2. `docs/README.md`

Навигационная карта документации.

Она должна объяснять:

- что читать новому человеку;
- что читать Codex/AI-сессии;
- где durable context;
- где current progress;
- где session logs;
- где planning;
- где ADR;
- где runbooks;
- где architecture/domain docs;
- где backlog;
- какие docs обновлять при каких изменениях.

Не превращай `docs/README.md` в большой narrative-документ.

### 3. `docs/context/project-context.md`

Durable context. Не changelog.

Включи:

- что это за проект;
- главные цели;
- основные user/product/runtime flows;
- architecture overview;
- repository zones;
- source of truth;
- key domain concepts;
- critical invariants;
- security/privacy/data rules;
- legacy areas and compatibility rules;
- что нельзя делать без явной причины;
- ссылки на важные docs;
- раздел `Needs Verification` для неподтвержденных фактов.

### 4. `docs/context/progress.md`

Current checkpoint.

Включи:

- Status Summary;
- Stable / Known Done;
- Active Work;
- Sensitive Areas;
- Current Risks;
- Next Concrete Tasks;
- Open Questions;
- Verification Notes.

Правило: `progress.md` должен отвечать “где мы сейчас и что делать дальше”, а не быть огромным changelog.

### 5. `docs/context/session-log.md`

Индекс session logs.

Включи:

- зачем существуют session logs;
- куда писать новые записи;
- current latest session file;
- правило ротации файлов;
- что обязательно фиксировать после meaningful work.

Обязательно зафиксируй принцип:

Session log должен писать не только “что сделали”, но и:

- что поняли;
- какие решения приняли;
- какие предположения сделали;
- что проверили;
- что осталось непроверенным.

### 6. `docs/context/session-log/session-log-01-current.md`

Первая запись текущей документационной сессии.

Структура:

- Goal;
- Files Read;
- Actions Performed;
- Main Findings;
- Decisions;
- Unverified / Needs Follow-up;
- Verification.

Не выдумывай историю старых сессий. Это стартовая точка новой хронологии.

### 7. `docs/planning/implementation-plan.md`

Structured planning entrypoint.

Если в проекте уже есть root roadmap или `IMPLEMENTATION_PLAN.md`, не уничтожай его. Сошлись на него и аккуратно синхронизируй.

Включи:

- current priorities;
- phased roadmap;
- next safe steps;
- risks and checks;
- testing strategy;
- documentation update rules;
- links to existing plans/backlog.

### 8. `docs/adr/`

Создай ADR-систему, если в проекте есть архитектурные или продуктовые решения, которые важно не забыть.

Создай:

- `docs/adr/README.md`
- `docs/templates/adr.md`
- 2-4 первых ADR только для действительно важных решений, которые подтверждены кодом/docs.

Формат ADR:

```text
# ADR NNNN: Title

Status: Proposed | Accepted | Superseded
Date: YYYY-MM-DD

## Context
...

## Decision
...

## Consequences
...

## Alternatives Considered
...

## Related Docs
...
```

Не создавай ADR для мелких задач. ADR нужен для решений, которые будущая сессия может случайно переоткрыть или сломать.

### 9. `docs/templates/`

Создай полезные шаблоны:

- `docs/templates/session-log-entry.md`
- `docs/templates/adr.md`
- `docs/templates/incident.md`, если проект production/ops-sensitive
- `docs/templates/runbook.md`, если есть ops/deploy/debug flows

Шаблоны должны быть короткими и пригодными к копированию.

### 10. `docs/runbooks/`

Создавай runbooks только если в проекте есть реальные повторяемые операции:

- deploy;
- production investigation;
- incident response;
- local setup;
- migrations;
- data import/export;
- payment/provider debugging;
- recurring jobs;
- queue/worker debugging.

Runbook должен быть практическим:

- prerequisites;
- commands;
- expected outputs;
- rollback/safety notes;
- what not to do;
- links to related docs.

### 11. `OVERVIEW.md` В Крупных Зонах

Создай `OVERVIEW.md` только в 2-6 наиболее важных директориях.

Не создавай overview ради количества.

Кандидаты:

- `src/OVERVIEW.md`
- `backend/OVERVIEW.md`
- `frontend/OVERVIEW.md`
- `internal/app/OVERVIEW.md`
- `internal/domain/OVERVIEW.md`
- `internal/store/OVERVIEW.md`
- `migrations/OVERVIEW.md`
- `services/OVERVIEW.md`
- `tools/OVERVIEW.md`
- `scripts/OVERVIEW.md`

Каждый overview должен быть коротким:

- responsibility;
- important files;
- invariants;
- common entrypoints;
- relevant tests/checks;
- related docs.

### 12. `README.md`

Если README есть, проверь, что он согласован с новой документацией.

README должен быть входной дверью:

- что за проект;
- быстрый запуск;
- тесты;
- ссылка на `docs/README.md`;
- ссылка на `AGENTS.md` для AI/Codex-сессий.

Не превращай README в огромный context dump.

## Документационная Модель

Используй разделение:

- `AGENTS.md` — контракт для AI-сессий.
- `docs/context/project-context.md` — durable facts.
- `docs/context/progress.md` — current status.
- `docs/context/session-log/*` — chronology and handoffs.
- `docs/planning/implementation-plan.md` — roadmap and next steps.
- `docs/adr/*` — important decisions and why.
- `docs/runbooks/*` — repeatable operational procedures.
- `docs/templates/*` — reusable writing templates.
- `OVERVIEW.md` — quick map of important code zones.
- `README.md` — human entrypoint.
- `docs/README.md` — documentation map.

## Качество Документации

Пиши:

- конкретно;
- проверяемо;
- без маркетинга;
- без воды;
- без длинных философских разделов;
- с явным разделением stable facts и current work;
- с пометками `требует проверки`, если факт не подтвержден;
- с ссылками на реальные файлы проекта.

Не пиши:

- выдуманные возможности;
- “будет сделано” как будто уже есть;
- boilerplate ради структуры;
- слишком длинные overview;
- абстрактные правила без связи с проектом.

## Финальная Проверка

Перед финальным ответом проверь:

```bash
git status --short
```

Если менялась только документация, тесты можно не запускать, но явно скажи это.

Если случайно затронут production code/tests/configs/migrations, запусти релевантные тесты или объясни, почему не смог.

Проверь:

- mandatory read order указывает на существующие файлы;
- `docs/README.md` согласован с `AGENTS.md`;
- session log создан;
- progress создан;
- durable context создан;
- ADR/template/runbook структура не избыточна;
- нет случайных secrets;
- не изменены пользовательские файлы без необходимости.

## Финальный Ответ

В финальном ответе дай:

1. Created files.
2. Updated files.
3. Mandatory read order.
4. Key invariants captured.
5. ADRs created, если были.
6. Runbooks/templates created, если были.
7. Unverified items.
8. Were code changes made? Expected answer: no, unless explicitly needed.
9. Were tests run or skipped?
10. `git status --short`.

Не делай коммит.
```

## Usage Notes

- For a small project, add: "Do not create ADR/runbooks/OVERVIEW files unless
  they are clearly useful."
- For a production-heavy project, add: "ADR and runbooks are required for
  payment, security, data, migration, deploy, or incident-response decisions."
- For an existing mature documentation tree, add: "Do not rewrite strong docs
  from scratch; integrate the memory model around them."
