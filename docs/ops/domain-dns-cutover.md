# Домен и DNS для `airnet-server`

Актуально на 2026-08-12. DNS и application cutover на новый production origin
`46.8.195.244` выполнены. Документ сохраняет проверенную схему, post-cutover
состояние и правила для оставшихся provider URL changes.

Новый домен зарегистрирован как `investcontrol.org`; его DNS обслуживает
Airnet (`dns1.airnet.uz`, `dns2.airnet.uz`). Provider default
`A @ = 176.96.243.100` заменён одной записью `A @ = 46.8.195.244`.

## Проверенная текущая схема

- canonical production: `investcontrol.org A 46.8.195.244`;
- `www.investcontrol.org` — CNAME на apex и HTTPS redirect на apex;
- authoritative TTL Airnet — `14400` секунд (4 часа), несмотря на
  предполагавшийся меньший migration TTL;
- сертификат Let’s Encrypt покрывает apex и `www`, действует до 2026-11-10;
  `certbot renew --dry-run` прошёл;
- `AAAA`, `MX`, `TXT`, `CAA` и DNSSEC для нового домена отсутствуют;
- legacy `инвестконтроль.рф` остаётся на `192.144.13.87` и проксирует
  application paths на новый HTTPS origin;
- в первые минуты после cutover Cloudflare/Google видели новый IP, а Quad9 ещё
  держал старый provider IP по четырёхчасовому TTL.

DNS направляет hostname, а не URL path. На новом origin application paths идут
в `invest-control-bot`, а `/mcp` намеренно возвращает 404 как out-of-scope.
Legacy exact `/mcp` остаётся локальным маршрутом отдельного старого stack.

## Сценарий A: новый домен `investcontrol.org`

Минимальная зона у нового DNS-провайдера:

| Имя | Тип | Значение | TTL | Нужно сейчас |
| --- | --- | --- | --- | --- |
| `investcontrol.org.` | `A` | `46.8.195.244` | provider-managed | да |
| `www.investcontrol.org.` | `CNAME` | `investcontrol.org.` | provider-managed | рекомендуется с HTTPS redirect |

В интерфейсе Airnet первое поле — полное имя, второе — тип, третье — приоритет,
четвёртое — значение. Для A-записи приоритет остаётся пустым. Сначала удалить
`A investcontrol.org. = 176.96.243.100`, затем добавить
`A investcontrol.org. = 46.8.195.244`. Две NS-записи Airnet не удалять.

Airnet автоматически создал также `MX @ -> investcontrol.org`, `mail`/`ftp`
CNAME и `www` CNAME. Для web-only зоны MX, `mail` и `ftp` не нужны; если
планируется почта, их следует заменить точным набором выбранного mail provider,
а не направлять SMTP на web origin. `www` можно оставить, если одновременно
добавить его в Nginx и TLS certificate.

Не добавлять без отдельной причины:

- `AAAA`: на сервере не подтверждён рабочий global IPv6;
- `MX`, SPF/DKIM/DMARC и другие `TXT`: их выдаёт выбранный почтовый сервис;
- произвольные `NS`/`SOA`: их создаёт DNS-провайдер;
- `DS`/`DNSKEY`: DNSSEC включается по инструкции нового провайдера, а DS
  публикуется у регистратора только после готовности подписанной зоны;
- wildcard `*`: приложению он не нужен;
- `CAA`: можно позже разрешить `letsencrypt.org`, но до проверки всех
  используемых CA безопаснее не вводить ограничение.

`www` следует создавать только вместе с `server_name`, redirect/proxy policy и
TLS-сертификатом, который содержит `www.<новый-домен>`. Простого CNAME
недостаточно.

Эти шаги выполнены: checked-in Nginx обслуживает apex/`www`, новый сертификат
выпущен отдельно от legacy-сертификата, HTTP и `www` перенаправляются на
canonical HTTPS apex.

## Сценарий B: тот же домен, но новый DNS-провайдер

Смену NS и смену origin лучше разделить:

1. Создать у нового DNS-провайдера зону, пока оставив `A @ = 192.144.13.87`.
2. Перенести только реально используемые записи из export старой зоны. AXFR у
   NIC.RU запрещён, поэтому один публичный DNS-аудит не доказывает отсутствие
   всех произвольных поддоменов.
3. Сверить ответы всех новых авторитетных NS до изменения делегации.
4. У регистратора заменить NS на выданные новым провайдером.
5. Держать старую и новую зоны идентичными и доступными минимум 4 суток, лучше
   7 суток.
6. Лишь после стабилизации делегации снизить apex TTL до `300`, дождаться
   полного старого TTL `3600` и проводить отдельный application/DB cutover.

Нельзя копировать старые или чужие `DS`: неверный DS сделает домен
неразрешимым для validating resolver-ов.

## Состояние URL вне DNS

Выполнено:

- Nginx/TLS для apex и `www`;
- `MAX_WEBHOOK_PUBLIC_URL=https://investcontrol.org/max/webhook`; provider
  подтверждает только эту subscription;
- `PAYMENT_MOCK_BASE_URL=https://investcontrol.org` и public base приложения;
- production backend/DB cutover.

Остаётся заменить после доступа к provider settings:

- Cloudflare Worker secret
  `TELEGRAM_WEBHOOK_ORIGIN_URL=https://investcontrol.org/telegram/webhook`;
- Robokassa ResultURL/SuccessURL/FailURL на `/payment/result`,
  `/payment/success`, `/payment/fail` нового HTTPS hostname.

До этих изменений legacy hostname proxy сохраняет работоспособность текущих
Worker/Robokassa URL. Локальная Wrangler authentication была недоступна при
cutover, поэтому Worker secret не менялся автоматически.

`TELEGRAM_WEBHOOK_PUBLIC_URL` сейчас указывает на Cloudflare Worker. Его не
нужно заменять прямым origin URL, если relay сохраняется; меняется именно secret
origin внутри Worker. MAX, напротив, использует прямой HTTPS webhook.

Старое и новое production-приложение нельзя одновременно запускать с одной
копией production secrets/данных: startup регистрирует webhook-и, а recurring
и lifecycle jobs выполняются внутри процесса.

## Post-cutover проверки

Проверять ответы авторитетных серверов и публичных resolver-ов:

```bash
dig +short NS investcontrol.org
dig +short A investcontrol.org @1.1.1.1
dig +short AAAA investcontrol.org @1.1.1.1
```

Проверять HTTPS:

```bash
curl -I http://investcontrol.org/healthz
curl -I https://investcontrol.org/healthz
```

В observation window отдельно проверяются Telegram relay, MAX webhook,
Robokassa callbacks, public legal/recurring pages и отсутствие второго writer.
Фактический stop/dump/restore/start/rollback описан в
`docs/ops/airnet-server-migration.md`.
