# План реализации сервиса (рабочий документ)

Статус: v1.16 (identity-first runtime cleanup after clean baseline)
Дата обновления: 2026-04-01
Основание: `tz.md`, `telegram-bot-flow.md`

## 1) Цель
Собрать и реализовать Telegram-бот + backend + БД + интеграцию с выбранным платежным провайдером (YooKassa/Т-Банк/др.) + админ-панель для управления платным доступом в закрытые Telegram-чаты по подписке.

Сопутствующие рабочие документы:
- `docs/MAX_IMPLEMENTATION_PLAN.md` - MAX-specific track
- `docs/APP_REFACTOR_PLAN.md` - текущий цикл рефакторинга `internal/app`
- `docs/REFACTORING_AND_TEST_PLAN.md` - отдельный backlog по unit-тестам, дедупликации и безопасным refactoring-задачам

### Обновление 2026-03-27
- Историческая цепочка additive SQL-миграций схлопнута в новый clean bootstrap.
- `migrations/0001_init.sql` теперь описывает актуальную canonical-схему проекта.
- Локальная PostgreSQL-база была полностью очищена, затем новая baseline migration успешно применена с пустого состояния.
- Fresh bootstrap verified: `schema_migrations` содержит только `0001_init.sql`, а `GOCACHE=/tmp/go-build go test ./...` остается зеленым.
- `audit_events` переведена в более общую actor/target модель:
  - admin login/logout/session events и user/app events теперь живут в одной таблице;
  - запись больше не завязана на обязательный `telegram_id`;
  - admin events page/export уже читают новый формат.
- `user_settings` окончательно убрана из runtime-кода:
  - global user-level `auto_pay_enabled` больше не используется;
  - recurring state для UI считается из активных подписок;
  - checkout recurring choice задается только явным действием пользователя в bot flow.
- Следующий identity-first срез уже внедрен в runtime:
  - `GetLatestSubscriptionByUserConnector` и `DisableAutoPayForActiveSubscriptions` переведены на `user_id` вместо `telegram_id`;
  - bot/app use-cases для ownership, latest subscription lookup и scheduler rebill checks больше не считают `telegram_id` каноническим владельцем подписки;
  - тестовые seed helper'ы обновлены так, чтобы создавать пользователей через `user_messenger_accounts`, а не через голые Telegram IDs.
- App-level helper layer тоже сдвинут в messenger-account модель:
  - `sendUserNotification`, `buildAppTargetAuditEvent` и выбор preferred messenger теперь принимают строковый `preferredMessengerUserID` и резолвят фактический target через `user_messenger_accounts`;
  - lifecycle revoke-from-chat теперь берет Telegram account из linked identities пользователя, а не из `subscriptions.telegram_id` как обязательного источника истины;
  - это уменьшает blast radius для следующего шага, где `Payment.TelegramID` / `Subscription.TelegramID` будут удаляться из runtime-моделей полностью.

### Обновление 2026-03-29
- Зафиксирован отдельный follow-up для тестирования на реальных деньгах:
  - для smoke/E2E проверки живого recurring и автосписаний понадобится non-production-only режим с очень коротким сроком подписки (например минуты/секунды);
  - это не должно становиться production default, поэтому идея пока оставлена как явный `TODO` в коде и рабочем контексте, а не как скрытая договоренность.
- Добавлена локальная dev-утилита `cmd/robokassa-callback`:
  - умеет эмулировать Robokassa `result`, `success` и `fail` сигналы на локальный backend;
  - если сумма не передана явно, подтягивает ее из `payments` по `payments.token`;
  - это снимает зависимость от постоянного доступа к LK Robokassa для smoke/E2E проверки checkout flow.
- `internal/bootstrap` удален:
  - открытие store и применение миграций теперь инкапсулированы в `internal/app/store_open.go`;
  - `cmd/server`, `cmd/max-poller` и `pkg/vercelapp` больше не зависят от отдельного bootstrap-пакета.
- `domain.Payment` и `domain.Subscription` больше не содержат `TelegramID`:
  - runtime-модели этих бизнес-сущностей теперь `user_id`-first;
  - PostgreSQL store читает/пишет их по clean baseline без несуществующих `payments.telegram_id` / `subscriptions.telegram_id`.
- Оставшиеся app/admin/bot пути, которые раньше читали `Payment.TelegramID` / `Subscription.TelegramID`, переведены на messenger identity resolution:
  - app payment handlers, payment flow, recurring rebill, lifecycle jobs и public recurring cancel page теперь резолвят target messenger через `user_messenger_accounts`;
  - admin billing/detail/churn/export больше не считают `payment/subscription` носителями Telegram identity;
  - bot menu/payment-history flows перешли на `user_id`-based запросы.
- Public recurring cancel page стала честнее в mixed-mode:
  - token по-прежнему несет legacy messenger user id;
  - но страница и POST-path теперь сначала пытаются резолвить пользователя через linked identities (`telegram` -> `max`), а уже затем используют fallback bridge.
- Bot menu и related helpers больше не считают Telegram обязательным transport context:
  - `sendSubscriptionOverview`, `sendPaymentHistory`, `sendAutopayInfo`, `sendExistingSubscriptionMessage` и финальный registration step теперь принимают `messenger.UserIdentity`, а не raw Telegram ID;
  - active subscription lookup и menu audit events резолвят пользователя через общий messenger identity path;
  - log keys в `internal/bot` / `internal/app` выровнены под `messenger_user_id` там, где речь идет уже не о Telegram-specific transport field.
- Admin navigation и form actions стали user-first:
  - detail/revoke/paylink/rebill URLs теперь строятся от `user_id`, а не тащат `telegram_id` как обязательный route/query context;
  - hidden form fields в detail/churn screen больше не прокидывают Telegram ID, если сервер уже может восстановить transport identity через `user_id`;
  - Telegram остается только в тех админских действиях, где он реально нужен как transport-specific projection (direct send, revoke from chat, paylink to Telegram user).
- Startup transport health checks добавлены в long-lived runtime:
  - `cmd/server` теперь явно пингует Telegram `getMe` и MAX `GET /me` до webhook/setup шага;
  - `cmd/max-poller` тоже валидирует MAX token через `GET /me` перед запуском polling loop;
  - это дает ранний fail-fast на битых токенах и более ясные startup logs с identity бота.
- Полный regression pass после этих правок:
  - `GOCACHE=/tmp/go-build go test ./...` проходит.

### Обновление 2026-04-01
- Для short-period live recurring smoke tests внедрена отдельная scheduler semantics:
  - recurring rebill для коннекторов с `test_period_seconds > 0` больше не использует боевые окна `72h/48h/24h`;
  - reminder и pre-expiry notice для таких тестовых коннекторов отключены, чтобы не шуметь и не искажать smoke-тест;
  - scheduler cadence уменьшен до `10s`, чтобы тестовые периоды `60s/120s` реально могли попадать в rebill windows.
- На уровне тестов добавлены прямые regression checks для:
  - short-period rebill eligibility;
  - отсутствия pre-expiry reminders/notices для short-period subscriptions;
  - сохранения зеленого `GOCACHE=/tmp/go-build go test ./...`.

## 2) Зафиксированные решения
- Админка на первом этапе: встроенные server-rendered HTML-страницы на Go.
- Тексты оферты и политики ПДн: хранение/редактирование в админке + возможность указать внешние ссылки.
- Подтверждение email/телефона: в MVP без OTP, но архитектурно оставить расширяемость.
- Напоминание о продлении: за 3 дня до окончания подписки.
- Валюта: только RUB.
- Интеграция с онлайн-кассой (54-ФЗ): не включать в MVP.
- Доступ в админку: один администратор.
- Локализация админки: русский/английский переключаемые UI-тексты.
- Импорт из Ainox + Robokassa: предусмотреть как отдельную задачу (скорее всего потребуется).
- Удаление из чата после окончания подписки: сразу по наступлению `expires_at`.
- Аудит: хранить историю интеракций пользователя (сообщения, действия, платежные события).
- Автопродление: реализовать через механизм recurring выбранного провайдера (если поддерживается).
- Для Robokassa recurring используем self-integration сценарий, а не LK-решение `Подписки`: наш backend и бот остаются источником истины по доступам, статусам и уведомлениям.
- Бизнес-цель recurring: не только автосписание, но и активная конверсия пользователей в автоплатеж внутри bot flow и admin follow-up.
- MAX рассматриваем как второй мессенджерный канал поверх общей предметной логики, а не как отдельный продукт с дублированным backend.
- Коннектор = тариф/условие оплаты (как товар), а не пользовательская сущность.
- Один и тот же чат может иметь несколько коннекторов (разные цены/условия); разные пользователи могут платить по разным коннекторам.
- Ссылка запуска должна поддерживать произвольный `start`-payload коннектора, например: `https://t.me/AinoxSubscriptionBot?start=in-94db7d6813507bc`.
- До выбора финального платежного шлюза используем `mock` провайдер оплаты.

## 3) Границы MVP
В MVP включаем:
- Коннекторы (`/start <start_payload>`), регистрация, акцепт оферты/ПДн.
- Сбор данных пользователя (ФИО, телефон, email, username).
- Создание платежа через выбранный провайдер и обработка webhook.
- Выдача доступа в чат после успешной оплаты.
- Хранение подписок/платежей в PostgreSQL.
- Проверка окончаний подписки и удаление из чата без blacklist.
- Напоминание о продлении за 3 дня.
- Повторная оплата и восстановление доступа.
- Минимальная админ-панель (коннекторы, пользователи, ручные операции).
- Логирование и хранение пользовательских интеракций.
- Воспроизведение текущего UX-флоу бота (раздел 13).

В post-MVP:
- Полная аналитика/дашборды.
- Расширенные роли админов.
- OTP-подтверждение контактов.
- Продвинутые ретраи автосписаний.
- Отдельный модуль импорта данных (если подтверждено по объему и срокам).

## 4) Предлагаемая архитектура
- Язык/платформа: Go.
- Telegram Bot API: webhook-режим на нашем Go-сервере (обработчик апдейтов внутри backend).
- Библиотека Telegram: `github.com/go-telegram/bot`.
- Backend API: HTTP (внутренний API + webhook endpoints).
- БД: PostgreSQL.
- Фоновые задачи: внутренний job-runner/cron.
- Админка: встроенные HTML-страницы на Go.
- Интеграции: Telegram Bot API + Payment Provider API + webhook.
- Деплой: VPS, reverse proxy (Nginx/Caddy), TLS, systemd/docker compose.

## 5) Доменная модель (уточненный черновик)
- `connectors`
  - `id` (BIGINT identity), `start_payload` (unique, например `in-94db7d6813507bc`)
  - `name`, `description`, `telegram_chat_id`, `channel_url`
  - `price_amount`, `price_currency`, `period_days`
  - `offer_url`, `privacy_url`
  - `is_active`, timestamps
- `users`
  - `id`, `full_name`, `phone`, `email`, timestamps
- `user_messenger_accounts`
  - `user_id`, `messenger_kind`, `messenger_user_id`, `username`, link/update timestamps
- `subscriptions`
  - `id`, `user_id`, `connector_id`, `status` (`pending|active|expired|canceled`), `started_at`, `expires_at`, `auto_renew_enabled`, timestamps
- `payments`
  - `id`, `user_id`, `subscription_id`, `connector_id`, `provider` (`mock|yookassa|tbank|...`), `provider_payment_id`, `amount`, `currency`, `status`, `paid_at`, `raw_payload`, timestamps
- `user_consents`
  - `id`, `user_id`, `connector_id`, `offer_accepted_at`, `privacy_accepted_at`, `offer_document_id`, `offer_document_version`, `privacy_document_id`, `privacy_document_version`, `accept_source`, timestamps
- `chat_memberships`
  - `id`, `user_id`, `connector_id`, `telegram_chat_id`, `status` (`invited|joined|removed`), `last_action_at`, `last_error`
- `user_interactions`
  - `id`, `user_id`, `channel` (`bot|admin|system`), `direction` (`in|out`), `event_type`, `payload`, `created_at`
- `audit_log`
  - `id`, `actor_type` (`system|admin`), `actor_id`, `action`, `entity_type`, `entity_id`, `payload`, `created_at`

## 6) Ключевые бизнес-процессы
1. Вход по коннектору
- Пользователь открывает `https://t.me/<bot>?start=<start_payload>`.
- Бот валидирует активный коннектор по `start_payload`.
- Бот показывает: название, описание, сумму, период, ссылки оферты/ПДн.
- Нажатие «Принимаю условия» фиксируется в `user_consents`.

2. Регистрация
- FSM: ФИО -> телефон -> email -> username.
- Валидация телефона и email по правилам референсного flow.
- Сохранение/обновление профиля пользователя.

3. Оплата
- Кнопка «Оплатить» создаёт платеж через выбранный провайдер с idempotence key.
- Параметры платежа берутся из выбранного коннектора.
- Пользователь уходит на checkout URL.
- Webhook обновляет статусы платежа и подписки.

3.1. Автоплатеж / recurring для Robokassa
- Первый платеж пользователя создается с признаком `Recurring=true` только при явном opt-in пользователя.
- После успешного первого recurring-friendly платежа сохраняем возможность повторного списания и привязку к активной подписке.
- Повторное списание инициирует наш backend через `Merchant/Recurring` с `PreviousInvoiceID` и новым `InvoiceID`.
- Подтверждением успешного повторного списания считается только callback `ResultURL`/`ResultUrl2`, а не ответ `OK+InvoiceID` на запрос `Merchant/Recurring`.
- При успешном rebill продлеваем текущую подписку от `max(now, current_expires_at)`.
- При неуспешном rebill:
- фиксируем неудачную попытку и причину;
- отправляем пользователю сообщение о проблеме со списанием;
- прикладываем ссылку на ручную оплату;
- при наступлении дедлайна продления удаляем из чата и уведомляем об исключении.

4. Выдача доступа
- При `payment.succeeded`: попытка выдать доступ в чат.
- Fallback: отправка invite-link.
- Фиксация результата в `chat_memberships`.

5. Продление и окончание
- Напоминание за 3 дня до `expires_at`.
- Удаление из чата в момент наступления `expires_at` (без blacklist).
- Повторная оплата снова активирует доступ.

## 7) План реализации (итерации)
### Итерация 0: Подготовка
- Статус: `выполнено`.
- Результат: каркас проекта, env-конфиг, стратегия секретов.

### Итерация 1: Базовый каркас
- Статус: `выполнено`.
- Сделано: HTTP-сервер, базовые handlers, in-memory store, PostgreSQL store, миграции и переключение `DB_DRIVER`.
- Осталось: углубление схемы БД под подписки/платежи следующих итераций.

### Итерация 2: Коннекторы и онбординг
- Статус: `выполнено (MVP-часть)`.
- Сделано: CRUD коннекторов в админке, `/start <start_payload>`, акцепт условий, FSM регистрации, референсные тексты, E.164 валидация телефона.
- Осталось для полного паритета: привязка кнопки `Оплатить` к реальному checkout URL выбранного провайдера.

### Итерация 3: Интеграция платежного провайдера
Статус: `выполнено частично`.
- Создание платежа + checkout URL.
- Webhook endpoint + идемпотентность.
- Обновление `payments/subscriptions`.
Примечание: работает `mock` provider, финальный провайдер (YooKassa/T-Банк) ожидает выбора.

### Итерация 4: Доступ в чат
- Выдача доступа после оплаты.
- Fallback invite-link.
- Повторная активация после повторной оплаты.

### Итерация 5: Жизненный цикл подписки
- Напоминания за 3 дня.
- Удаление по `expires_at`.
- Автопролонгация через recurring выбранного провайдера (при наличии).

### Итерация 6: Админка
Статус: `выполнено частично`.
- Авторизация одного администратора.
- Экраны: коннекторы, пользователи, подписки, ручные действия.
- Список коннекторов (условия оплаты, сумма, start-ссылка, чат, статус).
- Список платящих пользователей и их статусы.
- История платежей и история активности в боте.
Примечание: реализованы страницы `connectors`, `users`, `billing`, `events`, `help`; есть карточка пользователя и ручное отключение подписки. В работе остаются остальные manual actions, экспорт и углубление user operations.

### Итерация 7: Импорт, стабилизация и запуск
- Миграционный скрипт импорта из Ainox/Robokassa (при подтверждении источника и формата).
- Интеграционные тесты критического пути.
- Логи, метрики, деплой и runbook.

### Итерация 8: Recurring / автоплатежи Robokassa
- Статус: `выполнено частично`.
- Первый recurring-friendly платеж: opt-in пользователя в боте, правильная маркировка платежа, сохранение признаков recurring.
- Checkout/cancel compliance pages добавлены и используются в продукте.
- Магазин Robokassa активирован для recurring; E2E flow подтвержден на тестовом сценарии.
- Для production readiness продолжаем держать в актуальном состоянии checklist из `docs/robokassa-recurring-checklist.md`.
- Compliance-блок recurring:
- дописать оферту под recurring-условия Robokassa;
- добавить пользовательское соглашение;
- добавить отдельный opt-in чекбокс на автосписания (не pre-checked);
- хранить историю согласий на автосписания отдельно от обычного акцепта оферты/ПДн;
- сделать понятный сценарий отмены автоплатежа в боте/админке.
- UX-конверсия в автоплатеж:
- агитационный блок перед оплатой и после успешного первого платежа;
- подсветка выгод/непрерывности доступа;
- отдельная кнопка включения автоплатежа в кабинете бота.
- Повторные списания через self-integration API Robokassa (`Merchant/Recurring`).
- Хранение цепочки родительского и дочерних recurring-платежей.
- Автоматический джоб повторных списаний перед окончанием подписки.
- Обработка неуспешного списания: retry policy, фиксация причины, перевод подписки в `past_due`.
- Reminder / churn policy:
- за 3 дня до окончания подписки отправляем предупреждение о скором окончании;
- в день окончания отправляем финальное сообщение;
- по наступлению `expires_at` удаляем из чата.
- Сообщения пользователю:
- уведомление об успешном продлении;
- уведомление о неудачном списании со ссылкой на ручную оплату;
- уведомление об исключении из чата при окончательном неплатеже.
- Кнопки в боте:
- включить автоплатеж;
- выключить автоплатеж;
- сменить карту;
- отменить подписку.
- Public compliance UX для согласования с Robokassa:
- публичная страница оформления recurring-подписки c checkbox и legal links;
- публичная страница отмены автоплатежа без авторизации по подписанной ссылке;
- возможность приложить support-ready скриншоты checkout/cancel страниц в переписку с Robokassa.
Примечание: recurring flow уже активирован и работает; в доработке остаются дальнейшее юридическое выравнивание текстов, эксплуатационные проверки и расширение UX под новый мессенджерный слой.

### Итерация 9: Пользователи и churn-management в админке
- Статус: `выполнено частично`.
- Экран `Пользователи`: таблица + карточка клиента.
- Поля карточки:
- Telegram ID / username;
- ФИО;
- телефон / email;
- активные и исторические подписки;
- статус автоплатежа;
- история платежей;
- история уведомлений / ручных действий администратора.
- Фильтры по пользователям:
- активные;
- просроченные;
- перестали платить;
- не прошел rebill;
- отключили автоплатеж;
- удалены из чата.
- Ручные действия из админки:
- отправить сообщение пользователю;
- вручную запустить напоминание / ссылку на оплату;
- вручную удалить/отключить подписку пользователя из админки с уведомлением в боте;
- вручную исключить из чата;
- вручную восстановить доступ после оплаты.
- Отдельный operational flow: приостановка подписки и удаление из чата для тех, кто перестал платить.
- На первом этапе outreach только точечный: через карточку пользователя, без массовых рассылок по фильтру.
Примечание: реализованы `users`, карточка пользователя, ручное отключение подписки, точечное сообщение, отправка ссылки на оплату и отдельный экран `churn`/`проблемные оплаты`.

### Итерация 10: Отчеты, экспорт и операционные экраны
Статус: `выполнено частично`.
- CSV-экспорт по текущим фильтрам из всех admin-таблиц/экранов, где есть табличные списки (`billing`, `users`, `events`, проблемные оплаты и последующие реестры).
- Финансовые агрегаты:
- сумма по группе/коннектору;
- сумма по фильтру статусов;
- сумма по периодам.
- Экран/раздел оферт:
- текущая версия оферты;
- история изменений;
- публикация текста и внешней ссылки;
- привязка версии оферты к акцептам пользователей.
- Экран по проблемным оплатам:
- список клиентов с неуспешным списанием;
- причина / дата последней попытки;
- быстрые действия для ручного follow-up.
Примечание: CSV-экспорт уже реализован для `connectors`, `legal_documents`, `users`, `events`, `billing/payments`, `billing/subscriptions`, `churn`; экран оферт реализован в базовом виде (версии, активация, внешняя ссылка/текст, публичные страницы `/legal/offer` и `/legal/privacy`), остаются привязка версии документа к акцептам и дальнейшие операционные срезы.

### Итерация 11: Auth / session hardening для админки
Статус: `выполнено частично`.
- Убрать модель "вечный ADMIN_AUTH_TOKEN в cookie" как основной runtime-механизм.
- Ввести единый auth middleware для всех `/admin/*` маршрутов, кроме `/admin/login`.
- Целевая схема для browser-admin:
- короткоживущая signed session cookie (`HttpOnly`, `SameSite=Lax`, `Secure` на HTTPS);
- явный `expires_at` у сессии;
- sliding refresh / rotation при активном использовании;
- server-side проверка сессии на каждый запрос.
- Отдельно для programmatic/admin API:
- bearer token или service token для non-browser сценариев;
- без смешивания browser session и machine token в один механизм.
- Хранилище сессий:
- отдельная таблица `admin_sessions`;
- `id`, `session_token_hash`, `subject`, `created_at`, `expires_at`, `last_seen_at`, `revoked_at`, `ip`, `user_agent`;
- хранить только hash токена, не raw value.
- Login flow:
- `/admin/login` создает новую сессию;
- опционально ограничение "одна активная сессия на браузер" не требуется в MVP;
- logout = явный revoke текущей сессии.
- Middleware / security:
- единая middleware-цепочка: request logging -> recover -> auth -> CSRF (для state-changing routes);
- idle timeout и absolute timeout;
- rate limit на login сохранить и усилить;
- audit events на `admin_login_success`, `admin_login_failed`, `admin_logout`, `admin_session_revoked`.
- Cookie policy:
- `HttpOnly`;
- `Secure` при HTTPS;
- `SameSite=Lax`;
- `MaxAge` <= absolute session TTL;
- rotation token после login и периодически в активной сессии.
- Дополнительно:
- страница/секция "Активные сессии" в админке не обязательна для MVP, но заложить модель данных;
- совместимость с текущим server-rendered HTML без SPA/JWT client storage.
- Принципиальное решение:
- для браузерной админки не использовать "JWT в localStorage" как основной механизм;
- если JWT использовать, то только как подписанный session artifact внутри `HttpOnly` cookie, без client-side хранения.
- Технические подэтапы реализации:
- `11.1` Схема данных и storage:
- миграция `admin_sessions`;
- store-методы: `CreateAdminSession`, `GetAdminSessionByTokenHash`, `TouchAdminSession`, `RevokeAdminSession`;
- доменные модели и TTL-константы.
- `11.2` Auth core:
- генерация random session token;
- hash токена перед записью в БД;
- cookie encode/decode helpers;
- единая middleware `requireAdminSession`.
- `11.3` Login/logout refactor:
- `/admin/login` больше не пишет в cookie raw admin token;
- login проверяет `ADMIN_AUTH_TOKEN`, затем создает session record;
- `/admin/logout` ревокает текущую session.
- `11.4` Session lifecycle:
- absolute TTL;
- idle TTL;
- sliding refresh / periodic rotation;
- cleanup expired/revoked sessions.
- `11.5` Route protection:
- единый wrapper на все `/admin/*`;
- whitelist только для `/admin/login` и static assets;
- CSRF проверка остается отдельным слоем поверх authenticated POST.
- `11.6` Observability / audit:
- audit events на login success/fail, logout, revoke;
- логирование auth ошибок без утечки токенов;
- rate limit на login остается и переносится в новый flow.
- `11.7` Optional follow-up:
- экран активных админ-сессий;
- ручной revoke сессии;
- разделение browser session и machine token routes.
- Порядок внедрения:
- сначала storage + middleware;
- затем login/logout;
- затем замена текущего `requireAuth`;
- затем cleanup и audit;
- потом optional UI.
- Критерии готовности:
- cookie истекает и реально перестает работать на сервере;
- logout делает сессию недействительной немедленно;
- на каждый `/admin/*` идет server-side session lookup;
- browser admin работает без bearer token в URL/JS storage;
- текущие CSRF/login-rate-limit проверки не деградируют.
Примечание: уже реализованы `admin_sessions`, hash-only хранение session token, browser session cookie, server-side lookup на каждом `requireAuth`, absolute/idle expiry, logout revoke и audit events по login/logout/session revoke; остаются единая route middleware и optional screen для активных сессий.

### Итерация 12: MAX как второй мессенджерный канал
Статус: `исследование и декомпозиция в процессе`.
- Собрать и зафиксировать официальные ограничения и возможности MAX Bot API.
- Подготовить messenger-neutral декомпозицию текущего Telegram-слоя.
- Выделить transport adapters и общий use-case слой для подписок, платежей и recurring.
- Уже подготовлено:
  - messenger-neutral inbound/outbound слой внутри `internal/bot`;
  - внутренний `user_id` и `user_messenger_accounts`;
  - mixed-mode resolution в `bot`, `app` и `admin`;
  - additive `user_id` слой в `payments` и `subscriptions` без отказа от legacy `telegram_id`.
- Стартовый MAX transport foundation добавлен:
  - `internal/max` client;
  - long polling через `GET /updates`;
  - базовый outbound `POST /messages`;
  - unit-тесты на polling и отправку сообщений.
- Определить минимальный MAX MVP:
  - старт;
  - меню;
  - просмотр подписок;
  - запуск оплаты;
  - recurring status / cancel / re-enable через общие web entrypoints.
- Для локальной разработки MAX первым этапом идем через long polling (`GET /updates`), а не через webhook tunnel.
- После этого реализовать proof-of-concept MAX webhook adapter для production-контура.
Примечание: детали вынесены в `docs/MAX_BOT_RESEARCH.md`, `docs/MAX_DECOMPOSITION_PLAN.md` и `docs/MAX_IMPLEMENTATION_PLAN.md`.

## 8) Нефункциональные требования
- Надежность webhook: идемпотентность, ретраи, таблица необработанных событий.
- Безопасность ПДн: шифрование `phone/email` at-rest, masking в логах.
- Наблюдаемость: structured logs + базовые метрики по платежам/подпискам.
- Производительность: >100 коннекторов и >300 пользователей на одной инстанции с запасом.
- Операционный экспорт: CSV-выгрузка должна уважать активные фильтры и быть доступна без ручного SQL.
- Для каждого табличного admin-экрана должна быть кнопка экспорта CSV по текущему набору фильтров.

## 9) Критичные риски
- Ограничения Telegram на сценарий автоматического добавления в закрытые чаты.
- Юридические требования к оферте/ПДн и формату хранения согласий.
- Несовпадение UX/текстов/валидаций с текущим рабочим ботом при миграции.
- Риск хранения ПДн в debug-логах webhook (нужен режим redaction для stage/prod).
- Recurring у Robokassa требует точного разделения: ответ `Merchant/Recurring` != подтвержденная оплата; подтверждение только через callback/result flow.
- Для сценария `смена карты` потребуется уточнение UX и технический механизм перепривязки карты в Robokassa.

## 10) Вопросы и статусы
1. `Закрыт` Какой стек админки? -> Встроенные HTML-страницы на Go.
2. `Закрыт` Где хранить оферту/ПДн? -> В админке + поддержка внешних ссылок.
3. `Закрыт` Нужен ли OTP? -> В MVP нет, оставить расширяемость.
4. `Закрыт (предварительно)` Для recurring Robokassa используем ли self-integration или LK `Подписки`? -> Идем в self-integration, чтобы доступами и жизненным циклом управлял наш backend/бот.
5. `Закрыт` Напоминания: когда и сколько? -> За 3 дня, один reminder в MVP.
6. `Закрыт` Валюта/НДС? -> Только RUB, без доп. требований в текущем объеме.
7. `Закрыт` Нужна отдельная онлайн-касса? -> Пока нет в MVP.
8. `Закрыт` Модель админки? -> Один админ.
9. `Закрыт` Мультиязычность? -> Только русский.
10. `Закрыт (условно)` Нужен импорт из Ainox/Robokassa? -> Да, вероятно потребуется; объем и формат уточнить позже.
11. `Закрыт` SLA удаления из чата? -> Сразу при наступлении `expires_at`.
12. `Закрыт` Хранить историю уведомлений/действий? -> Да, хранить все интеракции.
13. `Закрыт` Формат валидации телефона? -> Используем E.164 в MVP, с возможностью ужесточить правила позже.
14. `Закрыт` Тексты сообщений/кнопок? -> Делаем 1:1 как в `telegram-bot-flow.md`, с возможностью последующей правки.
15. `Закрыт` Какую Go-библиотеку берем для Telegram API? -> Используем `github.com/go-telegram/bot`.
16. `Закрыт` Какой policy по reminder/churn для неуспешных recurring-списаний? -> За 3 дня до окончания подписки пишем клиенту предупреждение; в день окончания пишем повторно; по `expires_at` удаляем из чата.
17. `Переоткрыт` Как именно реализуем `смену карты` в UX? -> Нужно отдельно изучить API/продуктовые возможности Robokassa; вероятно есть встроенный сценарий, который лучше нашего кастома.
18. `Закрыт` Нужен ли массовый ручной outreach из админки или достаточно точечного? -> Пока только точечный outreach через карточку пользователя.
19. `Закрыт` Какой auth/session подход берем для админки? -> Для browser-admin идем в signed server-validated session cookie с TTL/rotation; bearer/JWT оставляем только для machine-to-machine сценариев.
20. `В работе` Как добавлять MAX без дублирования Telegram-логики? -> Через messenger-neutral core и отдельный MAX adapter; детали вынесены в отдельные docs-файлы.

## 11) Формат дальнейшего обсуждения
- Весь флоу обсуждения и планирования ведем в этом файле.
- Каждое изменение отмечаем датой и коротким описанием решения.
- Новые вопросы добавляем в раздел 10 со статусом.

## 12) Журнал изменений
- `2026-03-07` Итерация 0 выполнена: подготовлены структура проекта, env-конфиг с валидацией, `.env.example`, документация по окружениям и стратегии secret management.
- `2026-03-10` Итерация 2 (MVP-часть) выполнена: добавлены webhook для Telegram, flow `/start <start_payload>`, акцепт условий, FSM регистрации и минимальный CRUD коннекторов в админке на in-memory store.
- `2026-03-10` Учтен референсный flow из `telegram-bot-flow.md`; уточнена модель коннекторов как тарифов с разными условиями и `start_payload`.
- `2026-03-10` Закрыты вопросы по валидации телефона и текстам сообщений; подтвержден webhook-подход: бот работает на нашем Go-сервере и слушает события там же.
- `2026-03-10` Зафиксирован Telegram stack: библиотека `github.com/go-telegram/bot`.
- `2026-03-10` Реализация bot/webhook слоя переведена на `github.com/go-telegram/bot`; в flow добавлены `start_payload`, E.164 валидация телефона и кнопка `Оплатить`.
- `2026-03-10` В коде обновлены admin/bot/store: коннекторы расширены (`start_payload`, offer/privacy URL, описание), генерация deeplink в админке, удален самописный Telegram types слой.
- `2026-03-10` Добавлен временный `mock` checkout режим до выбора провайдера оплаты; добавлены `/mock/pay` и `/mock/pay/success`.
- `2026-03-10` Добавлен документ по регуляторике ПДн РФ: `docs/DATA_COMPLIANCE_RU.md`.
- `2026-03-11` Обновлены docs под текущее состояние кода (mock checkout, шаблоны админки, маршруты) и добавлены требования по обезличиванию (Приказ РКН №140 от 19.06.2025).
- `2026-03-11` План платежей переведен на provider-agnostic модель (до выбора финальной кассы).
- `2026-03-11` Внедрен единый structured logger (`log/slog`) с конфигурируемым уровнем `LOG_LEVEL`.
- `2026-03-11` Реализовано подключение PostgreSQL: DB-конфиг через `DB_*`, store на Postgres, автоприменение миграций, fallback `DB_DRIVER=memory`.
- `2026-03-11` Стек БД уточнен: SQL-доступ через `sqlx`, миграции через `sql-migrate`.
- `2026-03-11` Перевод идентификаторов на автоинкрементные BIGINT (`connectors/payments/subscriptions`) и обновление доменной модели/сторов.
- `2026-03-11` Добавлены страницы админки `billing` и `events`, локальные assets (Pico + GridJS), фильтрация/поиск/пагинация в таблицах.
- `2026-03-11` Реализовано сохранение платежей/подписок в PostgreSQL и активация подписки после `mock`-оплаты.
- `2026-03-11` После успешной оплаты бот отправляет подтверждение пользователю.
- `2026-03-12` Добавлен `channel_url` в коннектор (миграция + админка + использование в кнопке перехода после оплаты).
- `2026-03-12` Обновлена валидация коннектора: обязательны `Name` и `Price RUB`, из пары `Chat ID/Channel URL` требуется хотя бы одно поле (frontend+backend).
- `2026-03-19` Изучен recurring flow Robokassa: зафиксирован курс на self-integration (`Recurring=true` + `Merchant/Recurring` + подтверждение через `ResultURL`).
- `2026-03-19` Roadmap расширен новыми этапами: recurring/autopay, users/churn-management, CSV/export, экран оферт, ручной outreach и обработка неуспешных списаний.
- `2026-03-19` Уточнен churn policy: reminder за 3 дня до окончания подписки, повторное сообщение в день окончания, затем удаление из чата по `expires_at`.
- `2026-03-19` Уточнен scope admin outreach: на первом этапе только точечная работа через карточку пользователя; массовые рассылки отложены.
- `2026-03-19` Зафиксировано сквозное требование: во всех admin-таблицах нужен CSV-экспорт по активным фильтрам.
- `2026-03-20` Реализован экран `Пользователи`, карточка пользователя, ручное отключение подписки из админки с уведомлением в боте и базовый operational UI для работы по пользователю.
- `2026-03-20` Админка переведена на общий визуальный слой (`admin.css`): страницы `connectors`, `users`, `billing`, `events`, `help`, `login` и карточка пользователя выровнены по единой дизайн-системе.
- `2026-03-20` Реализован CSV-экспорт по активным фильтрам для основных admin-реестров: `connectors`, `users`, `events`, `billing/payments`, `billing/subscriptions`.
- `2026-03-20` В карточку пользователя добавлены операционные действия: отправка произвольного сообщения и отправка ссылки на повторную оплату по конкретной подписке/коннектору.
- `2026-03-20` Панели фильтров на admin-экранах (`users`, `billing`, `events`) приведены к единому компактному reusable-pattern через `admin.css`; восстановлен UI фильтров на экране событий.
- `2026-03-20` Экран `billing` расширен операционной сводкой по текущей выборке: KPI-карточки и разбивка итогов по коннекторам (суммы, успешные/ожидающие/ошибочные платежи, активные подписки).
- `2026-03-20` Исправлена пагинация на экране `events`: убрана конфликтующая client-side pagination GridJS, экран переведен на явную server-side пагинацию с pager bar под таблицей.
- `2026-03-20` Добавлен отдельный operational-экран `churn` / `проблемные оплаты`: проблемные кейсы по пользователю+коннектору, фильтры, CSV-экспорт, быстрый переход в карточку пользователя и отправка ссылки на оплату.
- `2026-03-20` Audit action strings вынесены в доменные константы; admin users-layer разрезан на `page/detail/actions/helpers`, чтобы остановить разрастание `internal/admin`.
- `2026-03-20` UI admin-форм подчищен: подсказка по созданию коннектора перенесена внутрь формы, а action/filter buttons приведены к более компактному общему стилю через `admin.css`.
- `2026-03-20` Добавлен экран `Оферты и ПДн`: хранение версий юридических документов в БД (`legal_documents`), создание новых версий, редактирование существующих, активация нужной версии, CSV-экспорт и публичные страницы конкретных версий (`/oferta/{id}`, `/policy/{id}`); бот использует активные документы как fallback, если у коннектора нет собственных ссылок.
- `2026-03-20` Поведение экрана оферт скорректировано: создание/редактирование переведено на `POST/Redirect/Get`, а статус документов стал обычным toggle `включить/выключить` без автоотключения старых версий при создании новой.
- `2026-03-20` Action-buttons в admin-таблицах приведены к единой semantic-схеме: destructive = красный, edit = синий, open/view = нейтрально-синий, send/enable = зеленый, disable = amber/muted; подсказки переводятся в tooltip-паттерн с иконкой `?`.
- `2026-03-20` Начата `Итерация 11` по auth/session hardening: добавлена таблица `admin_sessions`, браузерная админка переведена с raw admin token cookie на server-side sessions с hash токена, absolute/idle expiry, revoke/logout и audit events; добавлены автотесты на login/session/logout flow.
- `2026-03-20` Добавлена отдельная итерация по усилению admin auth/session: фиксируем переход к signed session cookie + server-side session validation, вместо опоры на долгоживущий статический токен в браузере.
- `2026-03-20` Реализовано versioning согласий: в `user_consents` теперь сохраняются `offer_document_id/version` и `privacy_document_id/version` для fallback-документов из реестра, а карточка пользователя показывает историю акцепта по коннекторам и версиям документов.
- `2026-03-20` Зафиксирован отдельный recurring-compliance checklist под требования Robokassa: юридические документы, отдельный opt-in на автосписания, история consent, cancel flow и порядок включения recurring только после активации магазина со стороны Robokassa.
- `2026-03-20` Для recurring readiness расширен legal registry: добавлен тип документа `user_agreement`, публичные URL `/legal/agreement` и `/agreement/{id}`, а также отдельный data-layer `recurring_consents` и отображение recurring-consent истории в карточке пользователя.
- `2026-03-20` В bot checkout flow добавлен явный opt-in на автосписания: stateless-toggle перед оплатой, запись `recurring_consents` при выборе режима с автоплатежом, безопасный override `pay:...:0/1`, и перевод menu-autopay в compliance-safe режим без тихого включения recurring.
- `2026-03-20` Для cancel flow автоплатежа добавлено отдельное подтверждение отключения в боте: запрос на отмену, явное подтверждение/отмена и понятный текст о последствиях отключения без потери уже оплаченного периода.
- `2026-03-24` Добавлены публичные recurring-compliance страницы: `/subscribe/{start_payload}` для оформления подписки с recurring opt-in текстом и legal links, и `/unsubscribe/{token}` для отключения автоплатежа без авторизации по подписанной ссылке; cancel-link встроен в bot-autopay flow.
- `2026-03-24` Recurring у Robokassa активирован со стороны магазина; подтвержден тестовый E2E flow первого recurring-friendly платежа и управления автоплатежом по подписке.
- `2026-03-20` Карточка пользователя расширена operational recurring-summary: явный статус автоплатежа, последний opt-in, последний коннектор и диагностический health-статус (`нет consent` / `enabled without consent` / `consistent` / `disabled by user`).
- `2026-03-20` Для recurring backend automation добавлены scheduler-aware retry windows T-3/T-2/T-1, общий `triggerRebill` для admin/scheduler, уведомление пользователю о неуспешном автосписании со ссылкой на ручную оплату и автотесты на retry-окна и scheduled rebill flow.
- `2026-03-20` Auth/session слой админки доведен до route-level middleware: публичными оставлены только `/admin/login` и `/admin/assets/*`, а все остальные `/admin/*` маршруты защищаются единым server-side session middleware без повторных lookup внутри handlers.
- `2026-03-20` Экран `churn` усилен recurring-диагностикой: по каждой проблемной записи показываются статус автоплатежа, retry-состояние (`нет попыток` / `pending` / `ошибок N/3`) и время последней попытки автосписания.
- `2026-03-20` На экран `churn` добавлены server-side фильтры по recurring state: `автоплатеж on/off/unset` и `retry state` (`нет попыток` / `pending` / `ошибки 1-2` / `ошибки 3/3`), чтобы оператор мог быстро выделять нужные кейсы без ручного просмотра всей таблицы.
- `2026-03-20` Auth/session слой дополнен cleanup механикой: при создании новой admin session backend best-effort удаляет revoked и absolute-expired записи из `admin_sessions`, чтобы таблица сессий не разрасталась бесконтрольно.
- `2026-03-20` Для админки добавлен operational экран активных browser-сессий: список `admin_sessions`, индикация текущей сессии, ручной revoke и аудит ручного завершения сессии.
- `2026-03-26` Для второго мессенджерного канала начато исследование MAX: собраны официальные документы, подготовлен отдельный research-doc, архитектурная декомпозиция и рабочий MAX-план внедрения.
- `2026-03-26` Начат безопасный кодовый рефакторинг под multi-messenger: добавлен пакет `internal/messenger` с transport-neutral outbound model (`OutgoingMessage`, `ActionButton`, `Sender`), Telegram client завернут в этот контракт, а `internal/bot` переведен с прямого построения `InlineKeyboardMarkup` на внутреннюю модель сообщений без изменения поведения.
- `2026-03-26` Продолжен безопасный multi-messenger рефакторинг inbound path: внутри `internal/bot` убраны прямые зависимости от Telegram `models.Message` / `models.CallbackQuery`, добавлены transport-neutral `IncomingMessage` и `IncomingAction`, а mapping из Telegram update локализован в `update_router`.
- `2026-03-26` Параллельно с multi-messenger рефакторингом усилено unit-покрытие bot/use-case слоя: добавлен `fake sender` и сценарные тесты на `/menu`, повторное включение автоплатежа без новой оплаты и отключение автоплатежа только для одной подписки с корректной агрегацией user autopay state.
- `2026-03-26` В persistence foundation введен внутренний `user_id` и подготовлена поддержка внешних messenger identities: добавлены доменные типы `MessengerKind` / `UserMessengerAccount`, миграция `0013_user_messenger_accounts.sql`, новые store-методы lookup/create user by messenger, а также unit-тесты для memory/postgres реализаций.
- `2026-03-26` Новый identity foundation начал использоваться в рабочем bot flow: `accept_terms` и шаги регистрации теперь разрешают/создают пользователя через `GetOrCreateUserByMessenger`, а unit-тесты дополнительно проверяют создание внутреннего пользователя и Telegram account link.
- `2026-03-26` Продолжен identity-refactor за пределами bot-layer: добавлен read-only lookup `GetUserByMessenger`, public recurring cancel page переведена на messenger-identity resolution, а admin user detail теперь открывается как по новому `user_id`, так и по legacy `telegram_id`; добавлены unit-тесты на новый lookup и admin detail route.
- `2026-03-26` Admin action/export layer подтянут к dual-identity модели: user detail forms и action URLs теперь прокидывают `user_id`, POST handlers (`message`, `paylink`, `revoke`, `rebill`) сначала резолвят пользователя по `user_id/telegram_id`, а CSV-экспорт пользователей и churn-выгрузка получили колонку `user_id`; добавлен action-level тест на отправку сообщения через `user_id`.
- `2026-03-26` Read-only admin filters начали принимать `user_id` без переписывания query-моделей store: `users`, `billing`, `churn` и связанные CSV exports резолвят `user_id` в текущий `telegram_id` через unified identity helper; добавлены unit-тесты на filter-resolution и users-page filtering по `user_id`.
- `2026-03-26` Payment/subscription слой переведен на следующий additive шаг multi-messenger migration: добавлена миграция `0014_payments_subscriptions_user_id.sql`, `payments` и `subscriptions` теперь сохраняют `user_id` параллельно с legacy `telegram_id`, а write/read paths и store-фильтры начали реально использовать обе модели.
- `2026-03-28` `users` runtime/store выровнен с clean baseline: доменная модель пользователя теперь хранит только канонический профиль (`id/full_name/phone/email/timestamps`), Telegram username/id читаются через `user_messenger_accounts`, а bot registration/admin user resolution/public recurring cancel перестали зависеть от колонок `users.telegram_id` и `users.telegram_username`.
- `2026-03-28` Postgres store для `payments` и `subscriptions` выровнен с clean baseline схемой: SQL write paths больше не пишут в несуществующие `telegram_id` колонки, а read/filter paths продолжают отдавать legacy `TelegramID` как derived runtime projection через join к `user_messenger_accounts` для Telegram identity. Это сохранило текущие bot/app/admin сценарии без отдельного compatibility столбца в БД.
- `2026-03-29` `PaymentListQuery` и `SubscriptionListQuery` переведены на `user_id`-first контракт: admin billing/exports и public recurring cancel page больше не фильтруют платежи и подписки по `telegram_id`, а Postgres/memory list methods больше не тащат derived Telegram column в SQL/runtime для этих списков.
- `2026-03-29` `UserListQuery` и admin screens `users/churn` начали принимать `user_id` как основной filter key, а recurring cancel token model получил messenger-neutral naming (`messenger_user_id` вместо Telegram-specific payload field), чтобы убрать еще один misleading identity слой из runtime.
- `2026-03-29` Дочищены Telegram-biased runtime хвосты вокруг checkout/cancel: `internal/payment.Request` больше не тащит лишний `UserTelegramID`, mock checkout flow опирается на payment token вместо декоративного user query param, а recurring cancel page внутри app-layer переименована в messenger-neutral термины без misleading `legacyExternalID`/`subscriptionMatchesLegacyTelegramID`.
- `2026-03-31` В connectors добавлен короткий test-only override периода подписки через duration field (`90s`, `15m`), который хранится как `test_period_seconds` и используется в payment activation для быстрых smoke-тестов recurring/autopay без подмены обычного `period_days`.
- `2026-03-26` Для MAX добавлен первый рабочий local-dev transport: `cmd/max-poller` поднимает long polling через `GET /updates`, пакет `internal/max` покрыт client/poller/adapter unit-тестами, а mapper `message_created` выровнен под documented payload (`message.sender`, `message.recipient`, `message.body.mid`) и теперь логирует raw update при очередном несовпадении формы события.
- `2026-03-27` На живом MAX E2E подтверждены `/menu`, `/start <payload>`, регистрация, `accept_terms`, `payconsent` и генерация Robokassa checkout link. App-level post-payment notification path переведен с Telegram-only отправки на messenger-aware notifier, чтобы успешная оплата и ошибки recurring могли уведомлять пользователя в MAX-чате.
- `2026-03-27` Отдельно зафиксирован следующий инфраструктурный этап по БД: после стабилизации multi-messenger behavior нужен clean-schema pass с полной пересборкой миграций под чистую накатку, удобным финальным порядком полей и удалением временных compatibility-слоев там, где они больше не нужны.
- `2026-03-27` MAX переведен на production-shaped webhook contour: основной HTTP-сервер теперь поднимает `POST /max/webhook`, при старте синхронизирует webhook subscription через MAX API и держит polling как dev-only fallback.

## 13) Референсный flow текущего бота (для воспроизведения)
Источник: `telegram-bot-flow.md`.

Целевая последовательность в боте:
1. Старт по коннектору (`/start <payload>`).
2. Сообщение с названием подписки, описанием, суммой, периодом, ссылками на оферту/ПДн.
3. Шаги анкеты:
- `ФИО`
- `Телефон`
- при неверном: `⚠️ Не правильный телефон. Введите номер в международном формате.`
- `E-mail`
- при неверном: `⚠️ Неправильный e-mail`
- `Ник телеграм`
4. Успешное завершение:
- `✅ Спасибо! Ваша заявка оформлена успешно.`
- `💳 Осталось оплатить`
- `Чтобы произвести оплату, нажмите на кнопку «Оплатить» ниже, для переадресации на платежную страницу`

Требование паритета:
- На этапе интеграции платежей привести тексты и шаги к максимально близкому виду к текущей реализации.
