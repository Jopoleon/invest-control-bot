# Итерация 2: коннекторы + онбординг (актуализировано)

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

## Текущие ограничения
- Финальный платежный шлюз пока не выбран (работает `mock` provider).
- Автодобавление/удаление в чат по Telegram API и recurring-платежи еще не реализованы.

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

## Авторизация админки
- `?token=<ADMIN_AUTH_TOKEN>`
- или заголовок `Authorization: Bearer <ADMIN_AUTH_TOKEN>`
- если `ADMIN_AUTH_TOKEN` пустой, в локальной среде доступ открыт.
