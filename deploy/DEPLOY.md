# Deploy — Battle Go API

Two processes: **api** (HTTP :8080) and **worker** (kick/end/idle daemons). Both read
`./.env`. PostgreSQL is Go-owned (active battle state); MySQL is the existing app DB.

## 0. Prerequisites on the server
- Go ≥ 1.25 (only to build; the binaries are static, no Go needed at runtime)
- PostgreSQL running, a database created (e.g. `battle_go`)
- Network access to the existing MySQL
- nginx (optional, for TLS + domain)

## 1. Get the code + config
```bash
cd /var/www/battle-go-api/go-api
cp .env .env.bak              # keep a backup
# edit .env for production values (see §2)
```

## 2. `.env` (production)
```ini
PORT=8080
PG_DSN=postgres://USER:PASS@127.0.0.1:5432/battle_go?sslmode=disable
MYSQL_DSN=USER:PASS@tcp(MYSQL_HOST:3306)/eng-erp?parseTime=true
ABLY_KEY=<ably api key>
JWT_PUBLIC_KEY=-----BEGIN PUBLIC KEY-----\n…\n-----END PUBLIC KEY-----
```
- Keep `JWT_PUBLIC_KEY` on one line with literal `\n` (the app converts them).
- `clientFoundRows=true` is appended to MYSQL_DSN automatically by the app.

## 3. Build
```bash
bash build.sh        # produces bin/api and bin/worker (static, stripped)
```

## 4. Install systemd services
```bash
sudo cp deploy/battle-api.service    /etc/systemd/system/
sudo cp deploy/battle-worker.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now battle-api battle-worker
```
Check:
```bash
systemctl status battle-api battle-worker
journalctl -u battle-api -f      # live logs
journalctl -u battle-worker -f
```
The api runs DB migration on startup (`postgres.Migrate`). First boot creates tables.

## 5. nginx + TLS (optional but recommended)
```bash
sudo cp deploy/nginx-battle.conf /etc/nginx/sites-available/battle-api
sudo ln -s /etc/nginx/sites-available/battle-api /etc/nginx/sites-enabled/
# edit server_name to your domain
sudo nginx -t && sudo systemctl reload nginx
sudo certbot --nginx -d battle.yourdomain.com    # TLS
```

## 6. Verify
```bash
curl -s -o /dev/null -w "%{http_code}\n" http://127.0.0.1:8080/student/v1/battle/find   # 401 (no token) = up
```

## Redeploy (after code change)
```bash
cd /var/www/battle-go-api/go-api
git pull            # or copy new code
bash build.sh
sudo systemctl restart battle-api battle-worker
```

## Production hardening checklist
- [ ] `GIN_MODE=release` — set in the unit files (done).
- [ ] **CORS** is currently `*` (dev). For prod, restrict to your frontend origin in
      `internal/handler/middleware.go` `CORSMiddleware` (or make it an env var).
- [ ] PostgreSQL: strong password, listen only on localhost (or firewall 5432).
- [ ] `.env` perms: `chmod 600 .env` (contains secrets).
- [ ] Firewall: expose only 80/443 publicly; keep 8080 internal (nginx proxies it).
- [ ] JWT issuance: wire the PHP `jwt-access` route so the frontend can mint `go-api` tokens.
- [ ] Backups: PostgreSQL holds only transient battle state; MySQL holds final results — back up MySQL as usual.

## Rollback
```bash
sudo systemctl stop battle-api battle-worker
# restore previous bin/ or git checkout previous tag, rebuild, start
```
