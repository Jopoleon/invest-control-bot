# MAX IMPLEMENTATION PLAN

Последнее обновление: 2026-03-26

## Текущий статус

Проект находится на этапе предварительного исследования интеграции с мессенджером MAX как второго клиентского канала наряду с Telegram.

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
- Для локального MAX development зафиксирован preferred-flow: long polling через `GET /updates` как первый transport adapter, без обязательного `ngrok` и webhook subscriptions.
- Добавлен начальный пакет `internal/max`:
  - HTTP client для `GET /updates`, `POST /subscriptions`, `POST /messages`;
  - polling-loop для local dev;
  - unit-тесты на transport boundary.
- Первый живой local polling прогон подтвержден на реальном MAX-боте: токен валиден, webhook subscriptions отсутствуют, updates приходят в `cmd/max-poller`.
- Mapper `message_created` выровнен под documented payload MAX:
  - пользователь читается из `message.sender`;
  - чат/диалог резолвится через `message.recipient.chat_id` или `message.recipient.user_id`;
  - текст сообщения читается из `message.body.text`;
  - идентификатор сообщения берется из `message.body.mid`.
- На случай новых несовпадений shape update adapter теперь пишет raw JSON update в debug-лог при `failed to map`, чтобы следующие итерации дорабатывать по фактическому payload, а не по предположениям.
- MAX private-dialog transport исправлен: outbound сообщения для текущего bot-DM flow теперь отправляются через `user_id`, а пустые callback-ack больше не отправляются в `/answers`, чтобы не получать `proto.payload` на harmless callbacks.

## Дорожная карта

### Инициация MAX-канала
Статус: in_progress
- Собрать официальные ограничения и возможности MAX Bot API.
- Зафиксировать минимальный MVP parity относительно Telegram.
- Выявить расхождения по deep links, callback-кнопкам, webhook security и группам/каналам.

### Архитектурная декомпозиция мессенджерного слоя
Статус: in_progress
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
Статус: in_progress
- Подключить официальный Go SDK MAX либо тонкий HTTP client поверх platform-api.max.ru.
- Реализовать webhook endpoint MAX с проверкой секрета `X-Max-Bot-Api-Secret`.
- Реализовать приём update types, базовый dispatcher и отправку сообщений.

### Parity пользовательских сценариев
Статус: pending
- Продублировать базовые входные сценарии: старт, регистрация, меню, мои подписки, платежи.
- Определить, какие inline/callback сценарии можно перенести без потерь.
- Определить, какие сценарии потребуют web fallback вместо нативных UI-компонентов MAX.

### Платежи и recurring
Статус: pending
- Переиспользовать существующую payment/core-логику.
- Подготовить messenger-neutral точки входа в recurring checkout/cancel flow.
- Проверить, какие deep links и возвраты из web в MAX реально поддерживаются.

### Тестирование и rollout
Статус: pending
- Локальные контрактные тесты transport-слоя.
- Sandbox / development bot на MAX.
- Ограниченный rollout без отключения Telegram-канала.

## Открытые вопросы
- Можно ли в MAX воспроизвести весь текущий callback-heavy UX без заметной деградации.
- Как именно удобнее связывать пользователя MAX и существующего пользователя системы.
- Нужен ли отдельный бот MAX на каждый бренд/коннектор или достаточно одного.
- Как вести legal/payment flow и возврат пользователя из web checkout обратно в MAX.
