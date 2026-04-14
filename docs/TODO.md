# TODO

Рабочий backlog по текущему состоянию кода и docs.
Сверху быстрые и короткие задачи с высоким прикладным эффектом.
Ниже более тяжелые, длинные и архитектурно дорогие задачи.

## Quick Wins

- [ ] Задеплоить текущий набор recurring-фиксов и повторно проверить short-period recurring на live money.
- [ ] На проде проверить `/unsubscribe/{token}` после stale-page фикса: отключение должно применяться к текущей active subscription того же коннектора, даже если страница была открыта на старой подписке.
- [x] На проде подтвердить audit для web cancel path: должен писаться `autopay_disabled` с `source=web_cancel_page`, а при stale submit еще и `requested_subscription_id`.
- [x] На проде подтвердить, что после отключения автоплатежа новые rebill payments больше не создаются.
- [x] На проде подтвердить, что после успешной оплаты Telegram-пользователь получает одноразовую invite link именно в канал коннектора, а не только fallback на публичный `channel_url`.
- [ ] На проде проверить, что у бота есть нужные права в каждом платном Telegram-канале: создание invite links и удаление участников.
- [x] На проде подтвердить end-to-end сценарий: истечение подписки без replacement period приводит к удалению пользователя из канала и audit `subscription_revoked_from_chat`.
- [ ] Обновить recurring docs после следующего production подтверждения, чтобы убрать уже устаревшие формулировки про callback visibility как главный неподтвержденный риск.
- [ ] Синхронизировать roadmap/docs, где еще остались historical формулировки про старую схему, старые recurring-гипотезы или старые Telegram-only ограничения.

## Small Product / Ops Tasks

- [x] Улучшить success message после оплаты: явно показывать название подписки, что именно оплачено, срок доступа и что делать дальше.
- [x] Добавить более явную observability вокруг неудачной выдачи доступа: ошибка создания invite link, отсутствие `chat_id`, недостаточные права бота.
- [x] Проверить, нужен ли дополнительный audit/event для invite-link delivery failures по recurring/success flow, а не только runtime log.
- [x] Продумать fallback-поведение, если revoke из канала не удался: добавить retry/backoff, финальный `needs manual check` и явный audit trail.
- [x] Показывать revoke/access-delivery failures в admin UI явно, а не только через audit/events.
- [x] Ослабить startup fail-fast для временных Telegram API timeout'ов: одиночный `getMe`/setup timeout не должен валить весь boot без retry/backoff.
- [x] Проверить, нужен ли такой же retry/backoff слой для MAX startup health checks и webhook setup.
- [x] Решить, нужно ли для unsupported payment providers продолжать fallback-to-mock, или лучше fail-fast вместо предупреждения `payment provider is not implemented yet`.
- [x] Удалить отдельный MAX polling runner и зачистить docs, чтобы MAX оставался только в webhook/server runtime.
- [ ] Добавить в админке и боте удобную фильтрацию по коннекторам там, где оператору или пользователю приходится разбирать несколько похожих подписок/платежей вручную.
- [ ] Системно убрать ложное отображение двойной подписки в recurring-цепочках: future renewal не должен выглядеть как вторая текущая подписка ни в боте, ни в админке, ни в учетных таблицах; отдельно перепроверить, что статус автоплатежа везде показывается по актуальному текущему периоду.
- [ ] Пересобрать Telegram access flow вокруг чатов, а не абстрактных "каналов": проверить, не лучше ли завести отдельный тестовый чат для production-like revoke/regrant сценариев, потому что membership lifecycle и права бота важнее, чем публичная витрина канала.
- [ ] Добавить в админке управляемый flow удаления пользователей из платных Telegram/MAX чатов: продумать безопасную реализацию, права доступа, audit trail и защиту от случайного массового revoke.
- [ ] Сделать в админке более явное отображение связи `коннектор -> подписка -> пользователь`: в таблице коннекторов или соседнем operational view нужно быстрее понимать, кто сейчас подключен, какие подписки активны, где включен автоплатеж и как это сортировать без ручного расследования.

## UI / UX Improvements

- [x] Актуализировать UI экрана отмены автоплатежа `/unsubscribe/{token}`: больше данных о пользователе, тарифе и текущем состоянии подписки.
- [x] Яснее объяснить на `/unsubscribe/{token}`, что отключается именно автоплатеж, а уже оплаченный доступ сохраняется до `ends_at`.
- [x] Улучшить post-submit состояние `/unsubscribe/{token}`: сделать подтверждение отключения заметнее и понятнее.
- [x] Проверить, нужны ли отдельные UI-состояния для stale submit, already-off и expired link, чтобы пользователь не видел их как одинаковую ошибку.
- [x] Облагородить web-экран успешной оплаты `/payment/success`: показывать не только общий success state, но и понятные детали платежа/подписки, включая payment reference (`InvId` / token / номер платежа), если он уже известен по callback и payment row.
- [ ] Довести public recurring pages `/subscribe/{start_payload}` и `/unsubscribe/{token}` до полной messenger-aware подачи destinations и warnings, чтобы web copy не оставалась более общей, чем bot/payment flow.
- [ ] Переписать copy и названия полей на экране коннекторов в более операционный язык: убрать неочевидные формулировки вроде абстрактных "ссылок на источник"/"длительности" там, где админ ожидает видеть явные платежные и access-смыслы.
- [ ] Упростить форму коннектора под реальный операторский сценарий: скрыть или свернуть редко используемые поля, убрать визуальный шум и сократить "нейросеточный" интерфейсный оверхед, оставив только обсужденные и реально используемые действия.
- [ ] Переработать UX выбора периода коннектора под более понятную продуктовую модель: отдельно решить, что показывать в основном интерфейсе для ежемесячных автосписаний, а что оставлять в advanced/debug path для test connectors и специальных сроков.
- [ ] Ограничить short-duration опции в основном UI коннекторов: минуты и произвольные короткие duration не должны выглядеть как обычный production path; если они нужны только для smoke tests, оформить это как явный test-only сценарий вроде `test 5 min`.
- [ ] Отдельно спроектировать bounded recurring / "платить N месяцев" как самостоятельный сценарий, а не перегружать им обычные evergreen-подписки: это может быть полезно для когортных кейсов вроде цессий, но не должно запутывать основной поток существующих ежемесячных подписок.

## Validation / Follow-up

- [x] На проде подтвердить, что `PreviousInvoiceID` стабильно идет от root recurring payment и не ломает второй/третий rebill.
- [ ] Продолжать снимать recurring-диагностику через `journald` и prod DB, пока short-period сценарий не будет подтвержден несколькими повторами подряд.
- [ ] Пересмотреть short-period windows в `internal/app/periodpolicy/policy.go` после повторных live-money smoke tests и реальных замеров provider latency.
- [ ] Не убирать `TODO:` из `internal/app/periodpolicy/policy.go`, пока не будет повторного production подтверждения, что текущие окна действительно корректны.
- [ ] Убедиться, что для short-period сценариев revoke не срабатывает ложно при `pending` rebill в grace window.

## Medium Engineering Work

- [ ] Убрать bounded N+1 lookup в `internal/admin/users_page.go` и заменить его bulk projection для messenger accounts.
- [ ] Вынести сборку payment status pages из runtime-обработчиков в отдельный page-model/helper слой: сейчас `internal/app/payment_runtime.go` смешивает HTTP, connector lookup, messenger selection и action assembly.
- [ ] Сделать более явный store path для `subscription by payment_id`, чтобы payment activation не искал нужную подписку через `ListSubscriptions(...)` и не держал этот обход в `internal/app/payments/service.go`.
- [ ] Добавить transactional store path для payment activation: `payment paid -> subscription upsert -> activation audit` сейчас правильно оркестрируется на app-layer, но все еще состоит из нескольких отдельных store calls.
- [ ] Добавить тесты на edge cases payment success/fail pages для messenger-aware actions и fallback path без `channel_url`.
- [ ] Покрыть тестами messenger-mismatch flow для коннекторов с destination только в Telegram или только в MAX: start, checkout step, pay guard и success-notification fallback.
- [ ] Добавить тесты на recurring cancel page для expired token, чужой subscription, already-disabled subscription и mixed-mode user resolution, если какие-то ветки все еще не покрыты после последних правок.
- [ ] Усилить тесты `internal/bot` для recurring on/off, missing-docs scenarios, subscription overview и payment history.
- [ ] Если provider-side сбои повторятся, вынести `OpStateExt` lookup из shell-only debug команды в более удобный admin/debug flow.
- [ ] Проверить, какие admin screens все еще используют исторические/transport-specific assumptions и требуют cleanup после последних messenger-neutral изменений.
- [ ] Продолжить identity cleanup: держать linked account resolution в одном месте и сокращать mixed-mode compatibility paths там, где это еще не доведено до `user_id`-first модели.
- [ ] Усилить recurring cancel token: кодировать не только `messenger_user_id`, но и `messenger_kind`, чтобы public `/unsubscribe/{token}` не зависел от telegram-first fallback при mixed-mode одинаковых numeric IDs.
- [ ] Решить, нужен ли отдельный persistent учет выдачи/отзыва доступа (`chat_memberships` из старого roadmap) или этот legacy-пункт надо официально убрать из docs.
- [ ] Добавить reconciliation/cleanup для Telegram invite links: stale expired rows не должны оставаться неотмеченными в БД после transient API/DB сбоев.

## MAX Track

- [x] Довести MAX до минимального parity с Telegram по пользовательским сценариям: старт, регистрация, меню, мои подписки, платежи.
- [x] Добавить для MAX окно/экран отправки сообщений, близкий по UX к Telegram compose flow, если такого parity-path еще нет.
- [x] Отдельно проверить recurring checkout/cancel UX для MAX и решить, где нужен web fallback вместо нативных UI-компонентов.
- [x] Подтвердить, как именно должен выглядеть возврат пользователя из web checkout обратно в MAX в production-потоке.

## Large Refactor / Cleanup

- [ ] Закрыть оставшиеся test gaps из `docs/REFACTORING_AND_TEST_PLAN.md` для payment pages, recurring pages и bot callback/payment branches.
- [ ] Довести cleanup `internal/app`: убрать оставшиеся compatibility wrappers, где они больше не нужны после выноса business logic в `internal/app/payments`, `internal/app/recurring`, `internal/app/subscriptions`.
- [ ] Вынести payment status pages и `buildPaymentPageActions` из корневого `internal/app`, как это уже намечено в `docs/APP_REFACTOR_PLAN.md`.
- [ ] Дорезать recurring/public-page assembly на smaller helpers, чтобы `buildRecurringCancelPageData` не продолжал расти как многозадачный mapper.
- [ ] Ввести более явный recurring-chain projection или identifier вместо текущего неявного grouping по `user + connector` в public cancel flows и related UI.
- [ ] Вынести повторяющуюся connector/legal context logic из `bot/start`, recurring pages и payment flow в один helper/service слой.
- [ ] Вынести user-facing notification builders в более явный слой, вместо дальнейшего размазывания payment/lifecycle/public-page текстов.
- [ ] Решить, нужен ли отдельный unified messenger delivery service после стабилизации текущего recurring/lifecycle/payment набора.
- [ ] Упростить wiring внутри `internal/app`: сейчас root application все еще вручную собирает несколько service runtimes с пересекающимися зависимостями.
- [ ] После следующего recurring milestone перепроверить, какие тестовые TODO из `docs/REFACTORING_AND_TEST_PLAN.md` уже можно вычеркнуть, а какие еще реально открыты.
