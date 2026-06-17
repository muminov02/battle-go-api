# Battle Go — one-command helpers.
#   make go/up           build + run api & worker (systemd if installed, else background)
#   make go/down         stop them
#   make go/logs         last 100 log lines (journald or local files)
#   make go-docker/up    build + start everything in Docker (postgres + api + worker)
#   make go-docker/down  stop the Docker stack
#   make go-docker/logs  last 100 Docker log lines
#   make build / test    build binaries / run unit tests

.PHONY: build test go/up go/down go/logs go-docker/up go-docker/down go-docker/logs

build:
	@bash build.sh

test:
	@go test ./...

# ---- local / server (binaries) ----
go/up: build
	@if systemctl list-units --type=service 2>/dev/null | grep -q battle-api; then \
		sudo systemctl restart battle-api battle-worker && echo "up (systemd: battle-api, battle-worker)"; \
	else \
		mkdir -p logs run; \
		nohup ./bin/api    >logs/api.log    2>&1 & echo $$! >run/api.pid; \
		nohup ./bin/worker >logs/worker.log 2>&1 & echo $$! >run/worker.pid; \
		echo "up (background) — logs/, run/"; \
	fi

go/down:
	@if systemctl list-units --type=service 2>/dev/null | grep -q battle-api; then \
		sudo systemctl stop battle-api battle-worker && echo "stopped (systemd)"; \
	else \
		-kill $$(cat run/api.pid run/worker.pid 2>/dev/null) 2>/dev/null; rm -f run/*.pid; echo "stopped"; \
	fi

go/logs:
	@if systemctl list-units --type=service 2>/dev/null | grep -q battle-api; then \
		journalctl -u battle-api -u battle-worker -n 100 --no-pager; \
	else \
		echo "===== api (last 100) ====="; tail -n 100 logs/api.log 2>/dev/null; \
		echo "===== worker (last 100) ====="; tail -n 100 logs/worker.log 2>/dev/null; \
	fi

# ---- docker ----
go-docker/up:
	docker compose up -d --build

go-docker/down:
	docker compose down

go-docker/logs:
	docker compose logs --tail 100
