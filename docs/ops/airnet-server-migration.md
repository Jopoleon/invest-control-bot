# Переезд production на `airnet-server`

Статус на 2026-08-12: production cutover завершён. Canonical runtime —
`airnet-server`, PostgreSQL 18 и `https://investcontrol.org`. Старый backend
inactive/disabled; старый Nginx и PostgreSQL сохранены для compatibility и
rollback. После короткой явной паузы новый backend снова active/enabled и
является единственным writer/callback handler.

Варианты регистрации нового домена или переноса зоны к другому DNS-провайдеру,
проверенные текущие записи и связанные изменения webhook/TLS вынесены в
`docs/ops/domain-dns-cutover.md`.

Этот документ не содержит паролей, токенов, приватных ключей или дампов БД.
Секреты переносятся только через защищённый канал и не коммитятся в репозиторий.

## Фактический результат cutover 2026-08-12

- Airnet authoritative DNS выдаёт `investcontrol.org A 46.8.195.244`, `www`
  является CNAME на apex; authoritative TTL — 4 часа.
- Let’s Encrypt сертификат для apex/`www` действует до 2026-11-10;
  `certbot renew --dry-run` прошёл.
- Production env передан напрямую SSH→SSH без локального plaintext-файла.
  После ownership consolidation он установлен как `ubuntu:ubuntu 0600` в
  `/home/ubuntu/apps/invest-control-bot/shared`. Сохранены provider/bot secrets и
  `APP_ENCRYPTION_KEY`; заменены target DB credentials, public base/MAX URL и
  абсолютный writable `LOG_FILE_PATH`.
- Старый writer был остановлен через `systemctl stop`; после stop подтверждены
  `MainPID=0` и отсутствие его DB sessions.
- Final PostgreSQL 14 custom dump имеет SHA-256
  `619fd73e8a6319893ecb16f2e6723656a47534064ea360091ce8fcc6982e4606`.
- Архив восстановлен PostgreSQL 18 одной транзакцией в свежую
  `investcontrol_prod`; checksum target совпал, source/target manifests совпали
  по всем таблицам/counts, migrations, sequences, owners, indexes и сравнимым
  constraints.
- Cutover manifest: 18 users, 144 payments, 115 subscriptions, 1595 audit
  events и 9 migrations. DB owner/app role — `investcontrol_app`, timezone —
  `Europe/Moscow`.
- Новый backend revision `2ed7bd2` успешно прошёл activation с `NRestarts=0`;
  startup подтвердил migrations, Telegram API/webhook, MAX API и новый MAX
  webhook. После короткой паузы он повторно запущен как `ubuntu:ubuntu` и снова
  `active/enabled` с `NRestarts=0`.
- Старый backend inactive/disabled. Старый HTTPS hostname проксирует все
  application paths на новый origin; его exact `/mcp` остаётся локальным
  маршрутом отдельного `ad-check-bot`. На новом hostname `/mcp` возвращает 404.
- Public health, landing, legal pages, admin login и invalid ResultURL response
  проверены на новом hostname; legacy hostname возвращает те же application
  ответы через proxy.
- Безопасный synthetic Telegram update `{}` с действующим webhook secret прошёл
  через Cloudflare Worker, legacy proxy и новый backend с HTTP 200, подтвердив
  текущую inbound compatibility chain без бизнес-действий.
- 10 доступных system packages на новом VPS обновлены до cutover, reboot не
  требуется; три GRUB package updates остались deferred из-за Ubuntu phasing.

Два recovery-path реально сработали во время окна:

1. Первая cross-version manifest-проверка использовала удалённый в PG18 GUC
   `lc_collate`; сценарий автоматически вернул старый writer. Проверка была
   переведена на `pg_database`, после чего manifests совпали.
2. Первый target startup обнаружил унаследованный относительный
   `LOG_FILE_PATH=logs/` до подключения к БД/provider setup; rollback снова
   вернул старый writer. Env исправлен на `shared/logs/app.log`, source заново
   заморожен и final dump/restore повторён.

Оба случая произошли до успешной target activation; старый PostgreSQL не
изменялся миграционным сценарием, а финальный snapshot всегда снимался заново
после каждого временного возврата writer.

## Исторический снимок старого production до cutover

Проверен доступный по `investcontrol-server` сервер `investcontrol`:

- Ubuntu 22.04.5, 1 GiB RAM, swap 1 GiB, корневой диск 40 GiB (занято 18 GiB);
- `invest-control-bot.service` активен под пользователем `investcontrol`;
- backend слушает только `127.0.0.1:8080`, PostgreSQL 14 — только
  `127.0.0.1:5432`; наружу HTTPS обслуживает Nginx на 80/443;
- production-домен: `xn--b1aghkfidhbthmd7l.xn--p1ai`; на момент проверки его
  A-запись — `192.144.13.87` (старый сервер);
- Let’s Encrypt сертификат этого домена действителен до 2026-10-26;
- БД `investcontrol_prod` занимает 11 MB и на 2026-08-10 содержит 18
  пользователей, 141 платёж и 115 подписок;
- применены миграции `0001_init.sql` … `0009_messenger_chat_users.sql`;
- обычных crontab-задач для root или `investcontrol` нет: lifecycle/rebill jobs
  работают внутри backend; явный backup/dump скрипт на сервере при базовой
  проверке не найден.

В production включены PostgreSQL, Robokassa в production-режиме с recurring,
Telegram relay через Cloudflare Worker и MAX webhook. Это означает, что перенос
не должен запускать вторую активную копию приложения с production-секретами и
скопированной БД: она способна отправлять сообщения, менять webhook-и или
создавать повторные списания.

На текущем сервере тот же production-домен маршрутизирует exact path `/mcp` в
API контейнера другого проекта `ad-check-bot` на `127.0.0.1:18081`, а все
остальные пути — в `invest-control-bot` на `127.0.0.1:8080`. У `ad-check-bot`
есть второй bot-контейнер, отдельная БД `ad_check_bot_prod` и отдельный
`MCP_ACCESS_TOKEN`. Этот stack не является частью данного репозитория, но его
публичный `/mcp` **входит в DNS cutover**. До смены A-записи нужно либо перенести
stack вместе с БД/env, либо временно проксировать exact path на старый сервер.
Остальные Nginx sites не входят в этот переезд.

Повторная проверка 2026-08-10 обнаружила ожидаемую дельту production: 18 users,
141 payment и 115 subscriptions. Из платежей 115 paid, 6 failed и 20 pending;
из подписок 12 active, 78 expired и 25 revoked; у шести активных включён
autopay. Перед cutover эти агрегаты надо снять заново: они не являются
замороженной контрольной суммой.

### Важные внешние точки

- Robokassa подтверждает оплату серверным `POST /payment/result`. Redirect
  страницы не являются подтверждением платежа.
- Telegram отправляет webhook в Cloudflare Worker, а Worker пересылает запрос
  на secret `TELEGRAM_WEBHOOK_ORIGIN_URL`. При сохранении того же production
  домена этот secret должен оставаться URL
  `https://<production-domain>/telegram/webhook`; перед cutover его надо
  сверить, но не заменять на worker URL или IP-адрес.
- MAX использует прямой URL
  `https://<production-domain>/max/webhook` и backend актуализирует его при
  старте. На 2026-08-10 у MAX подтверждена ровно одна subscription на текущий
  production URL.
- Публичные `/subscribe/*`, `/unsubscribe/*` и legal pages также должны
  работать на новом origin до переключения DNS.

### Обнаруженный риск layout

Systemd ожидает `.../current/invest-control-bot`. На старом сервере `current`
остался symlink на release от апреля, но бинарник и `REVISION` позже были
перезаписаны default `simple` deploy. На новом сервере нужно выбрать одну
схему. Для текущего `scripts/deploy_vps.sh` выбран **simple layout**:
`current/` является обычным каталогом, а не symlink; releases не используются.

## Доступ и административная модель

Новый сервер отвечает на:

```bash
ssh -o BatchMode=yes airnet-server 'hostname; whoami'
```

Локальная SSH-конфигурация указывает на пользователя `ubuntu`, IP
`46.8.195.244` и выделенный ed25519-key. На сервере подтверждён именно этот
public-key fingerprint. По решению владельца `ubuntu` имеет постоянный
`NOPASSWD` sudo через root-owned `0440` sudoers drop-in. Это удобная, но широкая
административная модель: компрометация SSH key пользователя равна компрометации
root. Поэтому password/root SSH отключены, UFW и fail2ban включены.

## Исторический снимок нового сервера до cutover

- hostname `airnet-server`, Ubuntu 26.04 LTS, kernel `7.0.0-29`, amd64, VMware
  VM;
- 2 vCPU, 3.3 GiB RAM, swap 3.8 GiB, XFS disk 50 GiB / около 42 GiB свободно;
- внутренний адрес `10.10.32.202/30`, внешний `46.8.195.244` через NAT;
- время синхронизировано через chrony, timezone `Europe/Moscow`;
- bootstrap updates установлены и новый kernel загружен; на повторной проверке
  накопилось 22 package updates, reboot requirement отсутствует;
- установлены PostgreSQL 18.4, Nginx 1.28.3, Certbot 4.0 и fail2ban;
- UFW разрешает только inbound TCP 22/80/443, PostgreSQL слушает только
  `127.0.0.1:5432`;
- SSH принимает public key, но не password/root login; X11 forwarding отключён;
- `unattended-upgrades` и persistent journald включены;
- outbound HTTPS до Telegram API, Cloudflare relay, Robokassa и MAX доступен.

Новый сервер имеет один системный диск. Локальный dump на нём не является
независимым backup; проверенная зашифрованная копия должна храниться off-host.

## Подготовка, завершённая до cutover

- создан service user `investcontrol` без интерактивного login;
- создан simple layout; `ubuntu` владеет deploy-каталогами `current/.deploy`,
  а runtime logs принадлежат `investcontrol`;
- root-only PostgreSQL credential создан на сервере, app role использует
  SCRAM-SHA-256; production DB пока отсутствует;
- unit установлен, но disabled/inactive; production env намеренно отсутствует;
- бинарник revision `2ed7bd2` загружен с `SKIP_RESTART=1`;
- memory/mock smoke прошёл как `investcontrol`: `/healthz=ok`, landing `200`,
  graceful shutdown;
- Nginx configs синхронизированы с `deploy/nginx`, неизвестный Host/SNI
  отклоняется, HTTP перенаправляется на HTTPS;
- действующий Let’s Encrypt state перенесён напрямую старый→новый по SSH;
  public-key fingerprint совпал, сертификат действует до 2026-10-26;
- `curl --resolve` через публичный IP `46.8.195.244` подтвердил HTTPS
  `/healthz=ok`, landing `200` и HTTP→HTTPS redirect;
- `/mcp` на новом origin намеренно возвращает 503 до решения по отдельному MCP
  процессу;
- PostgreSQL 14→18 rehearsal прошла через custom dump: checksum и archive list
  проверены, совпали 16 таблиц, все counts, 10 sequences, 53 indexes и 42
  сопоставимых constraints. PostgreSQL 18 дополнительно моделирует 141
  `NOT NULL` constraint новым типом; это ожидаемая version delta;
- все rehearsal dump-файлы и disposable DB после проверки удалены.

Это исторический pre-cutover layout. После cutover он заменён единым
`ubuntu:ubuntu` layout в `/home/ubuntu/apps/invest-control-bot`; отдельный Linux
service user больше не используется приложением. После проверки отсутствия
процессов и service data legacy Linux account/home `investcontrol` удалён;
одноимённая PostgreSQL роль `investcontrol_app` сохранена.

Повторная read-only проверка 2026-08-10 подтвердила это состояние. Failed units
и reboot requirement отсутствуют; SSH, UFW, fail2ban, Nginx, PostgreSQL и
Certbot исправны. Накопилось 22 пакетных обновления, которые нужно установить и
перепроверить до migration window. Приложение по-прежнему disabled/inactive,
production env и `investcontrol_prod` отсутствуют.

На старом сервере production env имеет исторические права `0664` и принадлежит
`investcontrol`; это не переносится как образец. На новом сервере env должен
оставаться `ubuntu:ubuntu 0600`. `DATABASE_URL` и `DB_DSN` в старом env
отсутствуют.

## План миграции

### 1. Bootstrap нового сервера — после восстановления SSH

1. Выполнить доступные Ubuntu updates, перезагрузиться после kernel update и
   повторно проверить SSH, NTP, диск, RAM, listeners и failed units.
2. Создать `/home/ubuntu/apps/invest-control-bot/{current,shared,.deploy}` и
   `shared/logs` с owner/group `ubuntu:ubuntu`. `current` должен быть обычным
   каталогом. Deploy и runtime выполняются только как Linux user `ubuntu`;
   отдельный OS service user не используется.
3. Использовать SSH Host `airnet-server` для deploy через `ubuntu`. Постоянный
   passwordless sudo оставлен по явному решению владельца сервера.
4. Установить PostgreSQL 18/client, Nginx и Certbot nginx plugin. PostgreSQL
   должен слушать loopback, а роль/БД приложения иметь только нужные права.
   Перенос с PostgreSQL 14 делается только через logical
   `pg_dump`/`pg_restore`, не через копирование data-dir.
5. Установить unit из `deploy/systemd/invest-control-bot.service`, адаптировав
   только пути при необходимости. Создать env-файл с правами `0600`, владельцем
   `ubuntu:ubuntu`. Переносить
   значения через защищённый канал, не через Git, scp в repo или терминальный
   лог. `APP_ENCRYPTION_KEY` нельзя менять: он нужен для уже выданных cancel
   tokens. Отдельно проверить, что `DB_DSN`/`DATABASE_URL` не переопределяют
   новые `DB_*` и не ведут обратно в старую БД.
6. Настроить Nginx как reverse proxy к `127.0.0.1:8080`, с теми же
   `Host`, `X-Real-IP`, `X-Forwarded-For`, `X-Forwarded-Proto`, что в текущем
   site. Открыть только 22, 80 и 443; перед включением UFW сначала разрешить
   SSH. После проверки key-based входа выключить password SSH и X11 forwarding.
   Настроить автоматическое продление сертификата. Текущий certificate state
   уже перенесён, но `certbot renew --dry-run` выполнять после DNS cutover:
   HTTP challenge до него всё ещё приходит на старый сервер.

Готовность этапа: `nginx -t`, `systemctl is-enabled`/`is-active` для Nginx и
PostgreSQL, PostgreSQL недоступен извне, SSH по ключу проверен во второй сессии,
а Nginx возвращает ожидаемый ответ на тестовом hostname без production-трафика.

### 2. Подготовка приложения без production side effects

1. Собрать Linux amd64 бинарник из фиксированного коммита и передать его через
   `SSH_HOST=airnet-server REMOTE_APP_DIR=/home/ubuntu/apps/invest-control-bot`
   в **simple** layout.
2. Сверить SHA бинарника/`REVISION`, unit, владельцев и режимы файлов.
3. Проверить бинарник и HTTP только через отдельный side-effect-free
   memory/mock env без production bot/provider tokens. `/healthz` проверяет
   только HTTP-процесс и **не проверяет БД**. Не запускать cloned production БД
   с production Robokassa/Telegram/MAX secrets: startup синхронизирует webhook-и
   и сразу запускает lifecycle/rebill pass, затем повторяет его каждые 10 секунд.
4. Сверить перенесённый TLS через `curl --resolve`; после DNS cutover проверить
   обычный HTTPS и `certbot renew --dry-run`.

5. Проверить, что Vercel `/api/cron/lifecycle`, внешний cron или другой
   scheduler не обращается к production DB. Во время cutover должен быть только
   один lifecycle executor.

Готовность этапа: новый origin принимает HTTPS и имеет валидный сертификат,
но provider callbacks, Telegram и MAX всё ещё обслуживает старый production.
Binary smoke выполнен только в безопасном mock окружении.

### 3. Репетиция восстановления БД

1. На новом PostgreSQL создать пустую БД и роль приложения с теми же логическими
   именами/правами, но новым паролем.
2. Снять PostgreSQL 14 logical custom-format dump старой БД, используя env
   текущего сервера только внутри защищённой shell-сессии; применять
   `pg_dump --format=custom --no-owner --no-privileges`.
3. Проверить SHA-256 и `pg_restore --list`, затем восстановить dump PostgreSQL
   18 client в заново созданную пустую БД с
   `--exit-on-error --single-transaction --no-owner --no-privileges`.
4. Сверить encoding/locale/extensions/owner, все migration IDs, counts всех
   таблиц, sequences/identities и constraints. Затем выполнить `SELECT 1`,
   DB-backed legal/admin read и проверить startup migration logs в изолированном
   окружении. Одного `/healthz` недостаточно.
5. Удалить временные dump-файлы с локальной машины и серверов после проверки
   либо сохранить зашифрованную резервную копию в согласованном хранилище.

Готовность этапа: restore воспроизводим, migration IDs и контрольные counts
совпадают, а сведения о секретах нигде не попали в Git/логи.

### Финальный DB preflight 2026-08-10

Повторная read-only проверка непосредственно перед обсуждением final migration
подтвердила:

- source PostgreSQL `14.23`, target PostgreSQL `18.4`;
- source DB имеет размер `11,529,563` bytes, `UTF8`, libc locale
  `en_US.UTF-8`, owner `investcontrol_app`, только `plpgsql`;
- все 16 public tables и 10 sequences принадлежат `investcontrol_app`;
- применены ровно миграции `0001`…`0009`;
- invalid/not-ready indexes, unvalidated constraints, blocking/long
  transactions и replication отсутствуют;
- на target роль `investcontrol_app` имеет только `LOGIN` с SCRAM password,
  production DB ещё отсутствует, свободно 41 GB;
- source использует `TimeZone=Europe/Moscow`, target cluster — `Etc/UTC`.
  После создания target DB требуется
  `ALTER DATABASE investcontrol_prod SET TimeZone TO 'Europe/Moscow'` и
  отдельная проверка через app connection;
- DB-only freeze/dump/transfer/restore/manifest verification оценивается в
  2–5 минут. База слишком мала, чтобы оправдать настройку logical replication.

Снимок source на момент preflight: 18 users, 141 payments, 115 subscriptions,
1568 audit events. Эти числа не являются финальным manifest: его нужно снять
после остановки writer.

Final DB migration нельзя выполнить заранее отдельно от application cutover.
Пока старый backend работает, восстановленная target DB немедленно устаревает.
Допустим только один boundary: подготовленный target/env/app → graceful stop
старого backend → отсутствие app DB sessions → final dump/restore/verify →
запуск единственного нового writer.

У старого deployed unit всё ещё `TimeoutStopSec=15`, тогда как checked-in и
новый unit используют `60`. Перед final stop старый timeout следует привести к
60 секундам и выполнить daemon-reload либо явно принять риск прерывания
in-flight lifecycle/HTTP работы. Использовать именно `systemctl stop`, а не
kill: unit настроен с `Restart=always`.

### 4. Cutover — выполнено 2026-08-12

Перед окном подтвердить в личном кабинете Robokassa ResultURL, Cloudflare
Worker origin, Telegram webhook, MAX subscription и отсутствие второго
lifecycle scheduler. При сохранении прежнего production домена URL не должны
измениться; меняется только origin за DNS. Заранее снизить DNS TTL и дождаться
старого TTL; проверить `A`, отсутствие/содержимое `AAAA` и `CAA`.

1. Выбрать окно вне short-period recurring smoke-test и без незакрытого
   provider incident. Зафиксировать timestamp, revision, counts и список
   pending rebill до остановки старого процесса.
2. Остановить старый backend на минимальное время и подтвердить отсутствие его
   процесса/DB sessions. Немедленно снять финальный logical dump, проверить его
   checksum и восстановить в **заново созданную** целевую БД. Не считать
   redirect или `OK+InvoiceID` подтверждённой оплатой — подтверждение только
   `/payment/result` callback.
3. Запустить backend на новом сервере, проверить `systemctl`, `/healthz`,
   Nginx, startup migration logs и отсутствие неожиданных rebill attempts.
4. После старта нового backend временно переключить старый Nginx `/` на новый
   origin, чтобы клиенты со старым DNS не получали 502. Старое приложение при
   этом должно оставаться остановленным. Для `/mcp` использовать отдельно
   согласованный target. Затем изменить DNS production-домена на новый IP.
   Старый сервер и его TLS/Nginx оставить до истечения TTL и проверок.
5. Проверить исходящие Telegram/MAX действия и входящие Telegram/MAX/Robokassa
   события. Для recurring сверить journald, audit events, payment rows и
   subscription rows; pending rebill не должен быть ошибочно продлён.

### 5. Наблюдение и rollback — текущий этап

В первые 24–72 часа наблюдать:

- `short-period rebill scheduler decision`;
- `robokassa rebill request` и `robokassa rebill response`;
- `stale pending rebill without callback`;
- `payment marked as paid` и обращения `/payment/result`;
- ошибки Telegram relay, MAX webhook и Nginx 4xx/5xx.

Rollback теперь требует сначала остановить новый backend, определить новые
записи/callbacks, появившиеся в PostgreSQL 18 после cutover, и только затем
осознанно вернуть данные/старый writer. Простого запуска старого service уже
недостаточно: его DB является frozen snapshot и начнёт расходиться. При полном
rollback MAX startup вернёт старую subscription, а deploy defaults потребуется
явно переопределить. Не запускать два writer одновременно.

## Post-cutover follow-ups

- наблюдать возобновлённый новый backend 24–72 часа перед удалением rollback
  assets. Старый writer не запускать;
- обновить Robokassa ResultURL/SuccessURL/FailURL в provider dashboard и
  проверить реальный callback; до этого legacy proxy обязателен;
- после повторной Cloudflare/Wrangler авторизации изменить Worker
  `TELEGRAM_WEBHOOK_ORIGIN_URL` на новый direct URL. Сейчас Telegram webhook
  имеет `pending=0` и no last error; Worker compatibility smoke прошёл, а
  legacy proxy сохраняет прежний origin;
- дождаться истечения Airnet TTL у stale resolver-ов: immediate check показал
  новый IP у Cloudflare/Google, но старый provider IP у Quad9;
- отсутствие Vercel/external lifecycle scheduler;
- создать независимый зашифрованный off-host backup. Final dump временно
  сохранён `0600` вне repo, protected target archive и старая frozen DB также
  удерживаются на observation window;
- решить срок retirement старого hostname/host. `/mcp` принят как отдельный
  out-of-scope сервис и не блокирует основной production.
