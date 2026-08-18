# Nginx VPS Configuration

These files are the checked-in source for the `invest-control-bot` Nginx site.

- `00-default-deny.conf` rejects requests with an unknown Host/SNI.
- `invest-control.conf` redirects HTTP to HTTPS and proxies the application to
  `127.0.0.1:8080`.
- `investcontrol-org.conf` is the canonical production HTTPS proxy for
  `investcontrol.org`; it redirects `www` to the apex hostname.
- `/mcp` is outside this project's migration scope. The new production hostname
  returns 404; the legacy hostname can continue serving its separate old-host
  exact-match route during the compatibility period.

The TLS site requires the existing certificate paths under `/etc/letsencrypt`.
After installation, always run:

```bash
sudo nginx -t
sudo systemctl reload nginx
```

Test the canonical origin against the expected public IP:

```bash
curl --resolve 'investcontrol.org:443:46.8.195.244' \
  'https://investcontrol.org/healthz'
```
