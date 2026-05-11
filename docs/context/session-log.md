# Session Log Index

Session logs preserve chronological context for future AI/Codex sessions.
They are not a replacement for durable context or current progress:

- durable facts go in `docs/context/project-context.md`;
- current state and next tasks go in `docs/context/progress.md`;
- session chronology goes in `docs/context/session-log/`.

## Latest Session File

- `docs/context/session-log/session-log-01-current.md`

Append new entries to the latest `*-current.md` file unless it becomes too large.
When rotating, create the next numbered file, update this index, and keep older
logs for archaeology.

## What To Record

Every meaningful session should record:

- goal;
- files and systems inspected;
- actions performed;
- main findings;
- decisions made;
- assumptions and unverified items;
- verification commands and results;
- follow-up tasks.

Write what was learned, not only what was changed. This is especially important
for payment, recurring, identity, messenger delivery, migrations, and production
operations.

## Reading Rule

For normal work, read:

1. `docs/context/project-context.md`
2. `docs/context/progress.md`
3. this index
4. the latest session file
5. docs relevant to the touched area

For archaeology, search older session files and archived incident/handoff docs.
