# Декомпозиция интеграции MAX

Последнее обновление: 2026-03-26

## Цель

Нужно добавить второй мессенджерный канал MAX без копирования всей логики Telegram-бота.

Ключевое требование: платежи, подписки, recurring, legal flow и audit должны оставаться едиными для всех мессенджеров.

## Главный принцип

Нельзя строить архитектуру по схеме:
- `telegram_bot`
- `max_bot`
- внутри каждого дублированная логика подписок и платежей.

Такая схема быстро приведёт к расхождению поведения, двойным багам и дорогой поддержке.

Нужна схема с разделением на слои:
- общий предметный слой;
- мессенджерный transport слой;
- web/payment слой;
- persistence слой.

## Предлагаемая структура слоёв

### 1. Domain / Core
Это слой предметных правил, который не должен знать ничего о Telegram или MAX.

Что здесь должно жить:
- пользователь;
- подписка;
- платёж;
- recurring consent;
- автоплатёж;
- legal documents;
- audit events;
- registration state machine;
- правила активации и продления подписки.

Что здесь не должно жить:
- `telegram_id` как единственный идентификатор пользователя;
- callback payload конкретного мессенджера;
- формат кнопок Telegram/MAX;
- HTTP/webhook детали.

### 2. Use Cases / Application
Это слой сценариев, который управляет бизнес-операциями.

Примеры use cases:
- старт регистрации;
- принятие условий;
- выбор коннектора;
- показ активных подписок;
- запуск оплаты;
- включение recurring;
- отключение recurring;
- ручной rebill;
- показ истории платежей.

Именно этот слой должен вызываться и из Telegram, и из MAX, и из web endpoints.

### 3. Messenger Abstraction
Это тонкий слой нормализации событий мессенджеров.

Он нужен, чтобы Telegram и MAX не тянули в use-case слой свои transport-specific детали.

Примерный набор сущностей:
- `MessengerKind`
- `UserIdentity`
- `IncomingEvent`
- `IncomingAction`
- `OutgoingMessage`
- `ActionButton`
- `MessageEditor`

Задача слоя:
- принять update из конкретного мессенджера;
- преобразовать его в внутренний формат;
- вызвать use case;
- преобразовать ответ обратно в формат нужного мессенджера.

### 4. Transport Adapters
Это конкретные реализации под каналы доставки.

Отдельные адаптеры:
- Telegram adapter;
- MAX adapter;
- возможно позже web widget / email / push, если понадобится.

Что делает adapter:
- принимает webhook/update;
- валидирует transport-specific security;
- парсит payload;
- вызывает внутренний router/use case;
- отправляет ответ через SDK/API конкретного канала.

Что adapter не должен делать:
- принимать решения о бизнес-правилах;
- самостоятельно менять статус подписки;
- содержать payment/recurring логику.

### 5. Web / Payment Layer
Этот слой остаётся общим и не должен зависеть от конкретного мессенджера.

Что сюда входит:
- checkout page;
- recurring checkout page;
- cancel recurring page;
- payment callbacks;
- legal pages.

Смысл слоя:
- мессенджеры только подводят пользователя к этим entrypoints;
- деньги и согласия живут в общей модели.

### 6. Persistence Layer
Хранилище должно оставаться единым.

Что нужно хранить общим образом:
- internal user id;
- внешние идентификаторы пользователей по каналам;
- subscriptions;
- payments;
- recurring consents;
- legal accepts;
- audit.

## Что нужно изменить в модели идентификации пользователя

Сейчас, если проект исторически строился вокруг `telegram_id`, это нужно разжать.

Правильнее ввести:
- внутренний `user_id`;
- таблицу внешних идентификаторов каналов, например `user_messenger_accounts`.

Пример полей:
- `user_id`
- `messenger_kind` (`telegram`, `max`)
- `external_user_id`
- `username`
- `display_name`
- `created_at`
- `updated_at`

Это позволит:
- одному пользователю иметь аккаунт и в Telegram, и в MAX;
- не дублировать подписки и платежи на каждый канал;
- отдельно решать, как связывать учётки пользователя между мессенджерами.

## Какие интерфейсы стоит ввести

### Sender
```go
type MessengerSender interface {
    SendMessage(ctx context.Context, user UserIdentity, msg OutgoingMessage) error
    EditMessage(ctx context.Context, user UserIdentity, messageRef MessageRef, msg OutgoingMessage) error
    AnswerAction(ctx context.Context, actionRef ActionRef, text string) error
}
```

### Event Router
```go
type MessengerRouter interface {
    HandleEvent(ctx context.Context, event IncomingEvent) error
}
```

### Identity Resolver
```go
type IdentityResolver interface {
    ResolveOrCreateUser(ctx context.Context, identity UserIdentity) (User, error)
}
```

### Payment Entry Builder
```go
type PaymentEntryBuilder interface {
    BuildCheckoutLink(ctx context.Context, userID int64, connectorID int64, opts CheckoutOptions) (string, error)
    BuildRecurringCancelLink(ctx context.Context, userID int64, subscriptionID int64) (string, error)
}
```

## Как разложить пакеты

Ниже не единственно возможный вариант, но это прагматичная схема для Go-проекта.

### Вариант структуры
- `internal/domain`
  - доменные сущности и базовые правила
- `internal/usecase`
  - сценарии приложения
- `internal/messenger`
  - общие интерфейсы мессенджеров
- `internal/messenger/telegram`
  - Telegram adapter
- `internal/messenger/max`
  - MAX adapter
- `internal/http`
  - web endpoints, payment callbacks, legal pages
- `internal/store`
  - интерфейсы хранилища
- `internal/store/postgres`
  - postgres impl

Если проект уже сильно завязан на текущую структуру, не надо делать огромный big bang refactor. Лучше идти поэтапно.

## Поэтапный план рефакторинга

### Этап 1. Выделить абстракции без смены поведения
Сначала нужно:
- описать внутренние интерфейсы мессенджера;
- завернуть текущий Telegram flow в эти интерфейсы;
- не менять пользовательское поведение.

Цель: подготовить архитектуру, не ломая работающий Telegram канал.

### Этап 2. Убрать жёсткую завязку на `telegram_id`
Нужно:
- ввести внутренний `user_id`, если его ещё нет;
- добавить таблицу внешних идентификаторов каналов;
- адаптировать use cases так, чтобы они работали от внутреннего пользователя.

Это самый важный архитектурный шаг.

### Этап 3. Перенести сценарии из bot handler в use cases
Нужно вытащить из Telegram handler всё, что относится к:
- регистрации;
- оплате;
- подпискам;
- recurring;
- меню и действиям.

После этого Telegram adapter станет тонким.

### Этап 4. Сделать MAX proof-of-concept
Только после предыдущих шагов имеет смысл делать:
- webhook endpoint MAX;
- парсинг update;
- отправку сообщений;
- команды `start`, `menu`, `subscriptions`, `pay`.

### Этап 5. Дотянуть parity только там, где это оправдано
Не весь Telegram UX обязан переехать 1:1.

Если MAX где-то хуже поддерживает сложные callback flows, правильнее:
- упростить сценарий;
- перевести часть действий в web page;
- не тащить transport-specific хаос в core.

## Что переносить первым

### Обязательно
- старт пользователя;
- идентификация пользователя;
- выбор коннектора;
- запуск оплаты;
- просмотр активной подписки;
- recurring status;
- recurring cancel/re-enable;
- уведомление об успешной оплате.

### Можно позже
- сложные callback-деревья;
- расширенная история платежей;
- редкие административные сценарии;
- тонкие Telegram-специфичные UX-детали.

## Основные решения, которые надо принять заранее

### 1. Один пользователь на несколько мессенджеров или отдельные учётки
Рекомендация: один внутренний пользователь, несколько messenger identities.

### 2. Один бот MAX на всю систему или несколько
Рекомендация для старта: один бот MAX на систему, пока нет жёсткого брендового требования.

### 3. Полный UI parity или функциональный parity
Рекомендация: функциональный parity важнее, чем визуальное копирование Telegram UX.

## Практический следующий шаг

Следующее разумное действие уже не в документации, а в коде:
- описать интерфейсы `internal/messenger`;
- выделить внутренние `IncomingEvent` и `OutgoingMessage`;
- проверить, какой кусок текущего Telegram handler можно первым перевести на use-case слой без массового рефакторинга.
