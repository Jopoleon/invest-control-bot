# Исследование: интеграция чат-бота с MAX

Последнее обновление: 2026-03-26

## Что удалось подтвердить по официальной документации

MAX предоставляет официальную партнёрскую платформу для разработки чат-ботов, мини-приложений и каналов. Для разработки ботов используется отдельная документация `dev.max.ru` и HTTP API на `platform-api.max.ru`.

Ключевые факты из официальной документации:
- Подключение к платформе MAX для партнёров доступно для юрлиц и ИП, которые являются резидентами РФ.
- Для работы с ботом нужен верифицированный профиль организации и сам бот должен пройти модерацию.
- На одну организацию доступно до 5 ботов.
- Для production MAX рекомендует Webhook как основной механизм доставки событий; Long Polling допускается для разработки и тестирования, если бот не подписан на Webhook.
- MAX поддерживает официальный Go SDK: `github.com/max-messenger/max-bot-api-client-go`.
- Для Webhook-подписки можно указать secret; он приходит в заголовке `X-Max-Bot-Api-Secret`.
- Webhook endpoint бота должен быть доступен по HTTPS на порту `443`.
- MAX API поддерживает отправку и редактирование сообщений, inline keyboard, callback answers, работу с групповыми чатами и загрузку файлов.

## Официальные ссылки

Базовые документы:
- Обзор платформы: https://dev.max.ru/docs
- Создание бота: https://dev.max.ru/docs/chatbots/bots-create
- Подготовка и настройка бота: https://dev.max.ru/docs/chatbots/bots-coding/prepare
- Help по чат-ботам: https://dev.max.ru/help/chatbots

SDK и примеры:
- Go-библиотека: https://dev.max.ru/docs/chatbots/bots-coding/library/go
- Hello Bot на Go: https://dev.max.ru/docs/chatbots/bots-coding/hellobot/go

API:
- Обзор API: https://dev.max.ru/docs-api
- Подписка на Webhook: https://dev.max.ru/docs-api/methods/POST/subscriptions
- Список webhook subscriptions: https://dev.max.ru/docs-api/methods/GET/subscriptions
- Long Polling updates: https://dev.max.ru/docs-api/methods/GET/updates
- Отправка сообщения: https://dev.max.ru/docs-api/methods/POST/messages
- Ответ на callback: https://dev.max.ru/docs-api/methods/POST/answers
- Групповые чаты: https://dev.max.ru/docs-api/methods/GET/chats

## Что это значит для нашего проекта

Нам не нужен отдельный новый продукт. Нам нужен второй messenger adapter поверх существующей логики.

Практически это означает следующее:
- платежи, подписки, recurring, legal documents, consent storage и audit должны остаться общими;
- Telegram-специфичный входящий transport нужно перестать считать единственным;
- поверх общей логики нужен отдельный MAX transport layer.

Если сделать это напрямую, копируя весь Telegram-код под MAX, получится дорогая и хрупкая поддержка двух разных ботов. Это плохой путь.

Правильная схема:
- выделить общую предметную логику;
- оставить Telegram adapter;
- добавить MAX adapter;
- унифицировать входящие команды, callback actions, menu actions и web entrypoints.

## Предварительная карта соответствия Telegram -> MAX

### Почти наверняка переносимо напрямую
- команды `/start`-типа и базовые текстовые сообщения;
- отправка текстов;
- кнопки-ссылки;
- callback interaction через inline keyboard + `POST /answers`;
- webhook delivery;
- редактирование сообщений;
- работа с чатами и участниками на уровне группового API.

### Требует проверки и осторожного дизайна
- полный parity текущих deep link и start payload flow;
- насколько удобно воспроизводится текущий callback-heavy Telegram UX;
- сценарии, где Telegram логика завязана на конкретный формат `callback_data`;
- способы возврата пользователя из web checkout обратно в MAX;
- особенности поиска, discoverability и bot moderation в MAX.

### Скорее всего надо делать messenger-neutral web fallback
- checkout / recurring opt-in page;
- recurring cancel page;
- любые чувствительные legal/payment confirmations;
- часть flows, где лучше не зависеть от ограничений клиентского UI мессенджера.

## Предлагаемый план реализации

### Шаг 1. Зафиксировать минимальный MAX MVP
Нужно не пытаться сразу перенести весь Telegram UX 1:1.

Минимальный MAX MVP:
- старт бота;
- вход/регистрация пользователя;
- просмотр активных подписок;
- запуск оплаты;
- ссылки на web checkout / recurring pages;
- базовое меню;
- уведомления об успешной оплате и статусе подписки.

Это даст рабочий канал без неоправданной переписки архитектуры за один проход.

### Шаг 2. Выделить messenger-neutral core
Нужно выделить общий слой:
- user identity linking;
- registration state machine;
- connector selection;
- payment initiation;
- recurring enable/disable;
- audit logging;
- consent/legal handling.

Messenger adapters должны только:
- принимать updates;
- нормализовать события;
- вызывать общий use-case слой;
- отправлять ответ пользователю.

### Шаг 3. Ввести интерфейсы мессенджера
Примерный набор интерфейсов:
- `MessengerUpdate`
- `MessengerSender`
- `MessengerRouter`
- `MessengerUserRef`
- `InteractiveMessage` / `ActionButton`

Идея в том, чтобы Telegram callback data и MAX callback payload не тянуть в бизнес-логику напрямую.

### Шаг 4. Реализовать MAX webhook adapter
Нужно добавить:
- новый HTTP endpoint для MAX webhook;
- проверку `X-Max-Bot-Api-Secret`;
- десериализацию update payloads;
- dispatcher в общий use-case слой;
- sender для сообщений, edits и callback answers.

На development-этапе можно поддержать Long Polling, но production ориентировать только на Webhook.

## Локальная разработка и тестирование

Для MAX локальный dev-flow отличается от Telegram:
- в Telegram удобно жить через `ngrok` и webhook;
- в MAX официальный dev-friendly путь проще: long polling через `GET /updates`, если бот не подписан на webhook.

Практический вывод для нашего проекта:
- для первого локального MAX теста нам не нужен `ngrok`;
- нужен локальный polling-loop, который:
  - ходит в MAX `GET /updates`;
  - нормализует update в наш messenger-neutral inbound event;
  - передает его в общий handler;
  - отправляет ответы обратно через MAX API.

Когда tunnel все-таки нужен:
- если хотим тестировать production-like webhook delivery;
- если хотим проверить валидацию `X-Max-Bot-Api-Secret`;
- если хотим пройти полный deployment path с `POST /subscriptions`.

Минимальный milestone, после которого MAX можно будет реально гонять локально:
1. MAX client с `GET /updates`;
2. mapper `MAX Update -> internal/messenger.Incoming*`;
3. sender для текстовых сообщений и простых кнопок;
4. локальный runner, который запускает polling-loop с access token.

После этого уже можно:
- поднять сервер локально;
- запустить MAX poller рядом с ним;
- писать в реального MAX-бота без webhook и без `ngrok`.

Минимальный набор env для такого сценария:
```env
APP_RUNTIME=server
APP_ENV=local
LOG_LEVEL=debug

MAX_BOT_TOKEN=...
MAX_BOT_NAME=...
MAX_POLLING_TYPES=bot_started,message_created,message_callback
MAX_POLLING_TIMEOUT_SEC=30
MAX_POLLING_LIMIT=100

PAYMENT_PROVIDER=mock
PAYMENT_MOCK_BASE_URL=https://your-ngrok-domain.ngrok-free.app

APP_ENCRYPTION_KEY=replace-with-32-or-more-char-secret
ADMIN_AUTH_TOKEN=replace-with-strong-admin-token
```

Запуск локально:
```bash
go run ./cmd/server
go run ./cmd/max-poller
```

### Шаг 5. Перенести сценарии по приоритету
Приоритет переноса:
1. `/start` и базовая навигация;
2. регистрация/выбор коннектора;
3. экран подписок;
4. запуск оплаты;
5. recurring status / cancel / re-enable;
6. admin/ops уведомления, если они вообще нужны внутри MAX.

### Шаг 6. Не переносить лишнее до подтверждения API parity
Пока не подтверждены все ограничения MAX, не надо сразу переносить:
- сложные вложенные callback menu trees;
- Telegram-specific formatting assumptions;
- узкоспециальные chat-member flows.

Сначала нужно добиться стабильного пользовательского пути оплаты и подписки.

## Основные риски

### Продуктовый риск
MAX может поддерживать похожие primitive UI, но не гарантирует удобство полного Telegram parity. Поэтому прямое копирование интерфейса может дать плохой UX.

### Архитектурный риск
Если просто дублировать Telegram код веткой `max_*`, проект быстро станет трудно поддерживать. Поэтому сначала нужна декомпозиция transport и use-case слоя.

### Операционный риск
Создание и изменение бота в MAX связано с модерацией. Это значит, что iteration loop может быть медленнее, чем в Telegram.

### Интеграционный риск
Нужно отдельно проверить:
- как MAX идентифицирует пользователя;
- какие поля пользователя доступны стабильно;
- как лучше связывать MAX identity с существующими пользователями;
- как работает deeplink / re-entry после web checkout.

## Что рекомендую делать следующим шагом

1. Зафиксировать минимальный MAX MVP без попытки полного parity.
2. Выделить messenger-neutral интерфейсы и use-case слой.
3. Сделать proof-of-concept MAX webhook bot с командами:
   - start
   - menu
   - subscriptions
   - pay
4. После этого решать, какие Telegram-сценарии действительно стоит переносить один в один.

## Что нужно уточнить дополнительно

Это вопросы, которые уже надо проверять на этапе реализации и тестового бота:
- как именно выглядит payload входящих callback updates;
- есть ли удобный аналог deep link start payload для маркетинговых ссылок;
- как лучше маршрутизировать пользователя из web checkout назад в MAX;
- нужны ли дополнительные требования MAX к аватару, описанию, moderation copy и legal copy;
- нужно ли отдельное брендирование и отдельный бот MAX под разные коннекторы.
