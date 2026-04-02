# TODO

Рабочий список для текущего цикла. Это не исторический план и не changelog.
Закрытые пункты лучше удалять или переносить в профильные документы.

## Current Focus

- [ ] Прогнать новый live-money short-period recurring smoke test после правки `PreviousInvoiceID -> root recurring payment`.
- [ ] Проверить в прод-логах:
  - `short-period rebill scheduler decision`
  - `robokassa rebill request`
  - `robokassa rebill response`
  - `robokassa rebill opstate`
  - `stale pending rebill without callback`
- [ ] Сверить по prod БД:
  - parent payment
  - child rebill payment
  - `rebill_requested`
  - `rebill_request_failed`
  - `robokassa_result_received`

## Recurring / Robokassa

- [ ] Подтвердить повторным прод-тестом, что Robokassa принимает rebill от root invoice серии, а не только первый child rebill.
- [ ] Если provider-side ошибка повторится, снять `OpStateExt` через `go run ./cmd/robokassa-opstate --invoice-id <InvId>`.
- [ ] При необходимости вынести provider-state lookup в отдельный admin/debug flow без ручного запуска из shell.
- [ ] Держать short-period recurring `TODO:`-маркеры в коде до повторного live-money подтверждения.

## Access Delivery

- [ ] Задеплоить правку с одноразовой Telegram invite link после успешной оплаты.
- [ ] Проверить на проде, что success message действительно содержит private single-use invite link, а не только публичный `channel_url`.
- [ ] Если Telegram invite link не создается, снять точную ошибку прав бота в канале (`CanInviteUsers` / channel admin rights).
- [ ] Лучше оформить success message после оплаты подписки:
  - показывать название подписки / тарифа
  - явно писать, что именно было оплачено
  - сохранять дату окончания доступа
  - не ограничиваться общим текстом `Оплата прошла успешно`
- [ ] Актуализировать UI экрана отмены автоплатежа `/unsubscribe/{token}`:
  - текущий экран выглядит бедно и даёт мало уверенности пользователю
  - показывать больше данных о пользователе и выбранной подписке
  - яснее объяснять, что отключается именно автоплатёж, а не уже оплаченный доступ
  - улучшить карточку тарифа / подписки и общий визуальный ритм страницы
  - отдельно продумать post-submit состояние и подтверждение успешного отключения

## Operational Notes

- [ ] Не делать выводы по recurring без проверки текущей прод-ревизии в `current/REVISION`.
- [ ] Пока `LOG_FILE_PATH` в проде не используется, считать `journald` основным источником runtime-логов.
- [ ] Для prod DB investigation использовать repo-local MCP `investcontrol_prod_postgres` через SSH tunnel.
