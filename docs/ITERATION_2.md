# Итерация 2: коннекторы + онбординг (актуализировано)

Важно: это исторический документ о состоянии проекта на момент ранней итерации.  
Он больше не описывает текущую production-like систему полностью.

За актуальным состоянием смотреть:

- [README.md](/home/egor/Work/src/github.com/Jopoleon/invest-control-bot/README.md)
- [ADMIN_GUIDE.md](/home/egor/Work/src/github.com/Jopoleon/invest-control-bot/docs/ADMIN_GUIDE.md)
- [IMPLEMENTATION_PLAN.md](/home/egor/Work/src/github.com/Jopoleon/invest-control-bot/IMPLEMENTATION_PLAN.md)

Дата: 2026-03-12

## Реализовано
- Минимальный CRUD коннекторов в админке.
- Telegram webhook endpoint для приема апдейтов.
- Обработка `/start <start_payload>`.
- Экран принятия условий через inline-кнопку `Принимаю условия`.
- FSM регистрации:
  - ФИО -> телефон -> email -> username.
- Сохранение `consent`, профиля пользователя и состояния регистрации.
- Сообщение о завершении регистрации с кнопкой `Оплатить`.
- Mock checkout для временного режима без финального платежного шлюза.
- Отдельные HTML-шаблоны админки вынесены в `internal/admin/templates/`.

## Telegram stack
- Используется библиотека `github.com/go-telegram/bot`.
- Бот работает webhook-режимом на нашем Go-сервере (`POST /telegram/webhook`).

## Актуальные тексты flow
- Поля анкеты: `ФИО`, `Телефон`, `E-mail`, `Ник телеграм`.
- Ошибка телефона: `⚠️ Не правильный телефон. Введите номер в международном формате.`
- Ошибка email: `⚠️ Неправильный e-mail`.
- Валидация телефона: E.164 (с нормализацией пробелов/скобок/дефисов).

## Текущие ограничения на момент этой итерации
- Финальный платежный шлюз тогда еще не был выбран, работал только `mock`.
- Recurring и расширенный operator flow тогда еще не были реализованы.

## Маршруты
- `GET /healthz`
- `POST /telegram/webhook`
- `GET /mock/pay`
- `GET /mock/pay/success`
- `GET /admin/connectors`
- `GET /admin/billing`
- `GET /admin/events`
- `POST /admin/connectors`
- `POST /admin/connectors/toggle`
- `GET /admin/help`

## Авторизация админки на тот момент
- В ранней версии использовался простой token-based доступ.
- Сейчас админка переведена на login page + server-side sessions.
