# MAX IMPLEMENTATION PLAN

Последнее обновление: 2026-04-03

## Текущий статус

MAX уже интегрирован как второй transport внутри общего backend. Текущий поддерживаемый runtime path - webhook/server mode через `POST /max/webhook`; отдельный polling runner не считается живым runtime.

## Прогресс

### 2026-03-26
- Завершено первичное исследование официальной документации MAX для разработки ботов.
- Подтверждено, что для production MAX рекомендует Webhook как основной механизм доставки событий; Long Polling допустим для разработки и тестирования, когда бот не подписан на Webhook.
- Подтверждено наличие официальной Go-библиотеки MAX Bot API.
- Подтверждено, что доступ к платформе MAX для партнёров открыт для юрлиц и ИП — резидентов РФ, после верификации профиля организации.
- Подтверждено, что Webhook MAX должен быть доступен по HTTPS на порту `443`.
- Подготовлен предварительный технический план адаптации продукта под MAX.
- Описана целевая архитектурная декомпозиция для messenger-neutral core и отдельных transport adapters.
- Реализован первый безопасный слой messenger-neutral outbound abstractions: добавлены `internal/messenger` и Telegram adapter на уровне отправки/редактирования сообщений и answer callbacks.
- Реализован следующий безопасный слой inbound abstractions: внутри `internal/bot` входящие события переведены на transport-neutral `IncomingMessage` / `IncomingAction`, а Telegram DTO остались только в adapter-level маршрутизации update.
- В persistence foundation подготовлен переход от единственного `telegram_id` к внутреннему `user_id` и отдельным messenger accounts, чтобы MAX можно было подключать без форка user model.
- Read-only user resolution вынесен в messenger-identity слой: recurring public pages и admin user detail начали использовать lookup по messenger account, а не только прямой `GetUser(telegram_id)`.
- Admin operational flows больше не опираются только на legacy detail route: user-facing admin actions и user-oriented CSV exports начали передавать и сохранять `user_id`, что уменьшает сцепление с Telegram-only identity.
- Admin read paths тоже начали жить в mixed mode: filters и exports для `users`, `billing` и `churn` уже принимают `user_id`, но пока резолвят его в legacy `telegram_id` до полного перехода payment/subscription слоя на internal user model.
- Payment/subscription persistence переведены на следующий additive шаг: `payments` и `subscriptions` уже умеют хранить `user_id` параллельно с legacy `telegram_id`, а write paths платежей и подписок начали реально его заполнять.
- Для раннего local MAX development был зафиксирован промежуточный long-polling flow через `GET /updates` как первый transport adapter; после стабилизации webhook path этот режим оставлен только как исторический этап, а не как текущий runtime.
- Добавлен начальный пакет `internal/max`:
  - HTTP client для `GET /updates`, `POST /subscriptions`, `POST /messages`;
  - polling-loop для local dev;
  - unit-тесты на transport boundary.
- Первый живой local polling прогон на реальном MAX-боте исторически подтвердил transport adapter: токен валиден, webhook subscriptions отсутствуют, updates приходили через временный debug runner.
- Mapper `message_created` выровнен под documented payload MAX:
  - пользователь читается из `message.sender`;
  - чат/диалог резолвится через `message.recipient.chat_id` или `message.recipient.user_id`;
  - текст сообщения читается из `message.body.text`;
  - идентификатор сообщения берется из `message.body.mid`.
- На случай новых несовпадений shape update adapter теперь пишет raw JSON update в debug-лог при `failed to map`, чтобы следующие итерации дорабатывать по фактическому payload, а не по предположениям.
- MAX private-dialog transport исправлен: outbound сообщения для текущего bot-DM flow теперь отправляются через `user_id`, а пустые callback-ack больше не отправляются в `/answers`, чтобы не получать `proto.payload` на harmless callbacks.
- На живом local E2E прогоне подтверждены `/menu`, `/start <payload>`, регистрация, `accept_terms`, `payconsent` и генерация checkout-ссылки через Robokassa.
- App-level post-payment notification path больше не Telegram-only: success/failure уведомления по платежам переведены на messenger-aware notifier с выбором linked messenger account и fallback-роутингом для mixed-mode записей.
- Основной backend теперь поддерживает MAX webhook mode: `cmd/server` синхронизирует webhook subscription, удаляет stale subscriptions и принимает update delivery на `POST /max/webhook`. Это и есть текущий поддерживаемый runtime path.

### 2026-03-27
- Выполнен clean-schema pass для всего репозитория:
  - историческая цепочка `0001..0014` схлопнута в новый baseline `migrations/0001_init.sql`;
  - локальная PostgreSQL-база очищена и проверена новой чистой накаткой;
  - fresh bootstrap подтвержден отдельным прогоном `migrations.ApplyUp`.
- Это важно для MAX-track, потому что дальнейшие multi-messenger изменения теперь опираются на clean bootstrap schema, а не на историческую цепочку совместимых миграций.

### 2026-04-03
- Для MAX зафиксирован production-shaped return path после web checkout:
  - на `/payment/success` страница возвращает пользователя не в абстрактный `web.max.ru`, а в конкретного бота через direct deeplink `https://max.ru/<bot>?start=<connector_start_payload>`;
  - на `/payment/fail` страница так же ведет обратно в конкретного MAX-бота, но без лишнего return-to-channel action;
  - `MAX Web` остается только явным fallback action, если пользователь открыл flow вне нативного клиента или диплинк не сработал.
- Это доводит MAX до минимального parity по пользовательскому payment loop: старт, регистрация, меню, мои подписки, платежи и возврат из web checkout обратно в бот.

## Дорожная карта

### Инициация MAX-канала
Статус: mostly_done
- Официальные ограничения и возможности MAX Bot API зафиксированы.
- Минимальный MVP parity относительно Telegram подтвержден для стартового пользовательского flow.
- Открытым остается только накопление production наблюдений по deep links после web checkout.

### Архитектурная декомпозиция мессенджерного слоя
Статус: implemented_incrementally
- Выделить messenger-agnostic core для сценариев регистрации, подписок, платежей и автоплатежей.
- Ввести абстракции transport/update sender, callback handling, identity mapping.
- Отвязать предметную логику от Telegram-специфичных DTO и callback payloads.
- Уже сделано:
  - transport-neutral outbound message model;
  - Telegram sender adapter поверх этой модели;
  - перевод `internal/bot` с прямого `InlineKeyboardMarkup` на внутренние action buttons.
  - transport-neutral inbound events;
  - локализация Telegram update mapping в одном месте (`update_router`).
  - внутренний `user_id` и foundation для multiple messenger identities в store/domain/migrations.

### Реализация MAX adapter
Статус: implemented
- Реализован тонкий HTTP client/sender поверх MAX Bot API.
- Реализован webhook endpoint MAX с проверкой секрета `X-Max-Bot-Api-Secret`.
- Реализованы прием update types, mapper в internal messenger events и отправка сообщений.

### Parity пользовательских сценариев
Статус: mostly_done
- Базовые входные сценарии реализованы: старт, регистрация, меню, мои подписки, платежи.
- Определить, какие inline/callback сценарии можно перенести без потерь.
- Для recurring checkout/cancel зафиксирован messenger-neutral web fallback:
  - `/subscribe/{start_payload}` остается web entry page;
  - для MAX страница должна вести обратно в конкретного бота через direct deeplink `max.ru/<bot>?start=<payload>`;
  - `/unsubscribe/{token}` остается публичной web page отключения автоплатежа и должна давать явный return path обратно в MAX-бота.

### Платежи и recurring
Статус: implemented_with_live_validation_pending
- Используется существующая payment/core-логика.
- Messenger-neutral точки входа в recurring checkout/cancel flow подготовлены.
- Уже зафиксировано целевое поведение для MAX:
  - `/payment/success` и `/payment/fail` должны возвращать пользователя в конкретного бота через direct deeplink `max.ru/<bot>?start=<connector_start_payload>`;
  - `MAX Web` остается fallback-кнопкой, а не основным return path;
  - recurring checkout/cancel сохраняют тот же принцип: web page как compliance layer, бот как основной маршрут продолжения сценария.
- Остается подтвердить это поведение на production MAX-клиенте после живого прогона.

### Тестирование и rollout
Статус: ongoing
- Локальные контрактные тесты transport-слоя.
- Sandbox / development bot на MAX.
- Ограниченный rollout без отключения Telegram-канала.

## Открытые вопросы
- Можно ли в MAX воспроизвести весь текущий callback-heavy UX без заметной деградации.
- Как именно удобнее связывать пользователя MAX и существующего пользователя системы.
- Нужен ли отдельный бот MAX на каждый бренд/коннектор или достаточно одного.
- Насколько стабильно MAX-клиент обрабатывает `start` deeplink после возврата из внешнего web checkout на реальных устройствах.
