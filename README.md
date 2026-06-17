# battle-go-api

Go rewrite of the Battle module — a competitive quiz where students battle each other
(P2P / GROUP) or a scripted AI. Replaces the old PHP/Firebase implementation.

- **PostgreSQL** holds active battle state during play.
- **MySQL** (existing app DB) is read for questions/students and written for final results.
- **Realtime** is pluggable: **Ably** or native **WebSocket** (`REALTIME_DRIVER`).
- Auth: RS256 JWT issued by the PHP app (`get-jwt`), validated here.

## Layout
```
cmd/api        HTTP service (Gin)         :8080
cmd/worker     background daemons (kick / idle / end-battle)
internal/      auth, battle logic, db (postgres/mysql), service, realtime, handler, models, config
deploy/        systemd units + nginx config
docs/          ADR.md, openapi.yaml, BATTLE_API.md, BATTLE_PRESENTATION.md
smoke_test.go  end-to-end smoke test (build tag: smoke)
```

## Run

```bash
cp .env.example .env      # fill in DSNs, ABLY_KEY, JWT_PUBLIC_KEY
```

One-command helpers (Makefile):

| Command | Does |
|---|---|
| `make go/up` | build + run api & worker (systemd if installed, else background) |
| `make go/down` | stop them |
| `make go/logs` | last 100 log lines (journald or `logs/*.log`) |
| `make go-docker/up` | build + start everything in Docker (postgres + api + worker) |
| `make go-docker/down` | stop the Docker stack |
| `make go-docker/logs` | last 100 Docker log lines |
| `make build` / `make test` | build binaries / run unit tests |

**Docker note:** `docker compose` brings up its own PostgreSQL; MySQL is your existing DB —
in `.env` set `MYSQL_DSN` host to `host.docker.internal` (mapped in compose) or the real host.

Manual:
```bash
go run ./cmd/api      # HTTP API
go run ./cmd/worker   # daemons
go test ./...         # unit tests
```

## Docs
- `docs/openapi.yaml` — API spec (import to Postman)
- `docs/ADR.md` — architecture decisions for client devs
- `docs/BATTLE_API.md` — request/response examples + realtime
- `docs/BATTLE_PRESENTATION.md` — overview
