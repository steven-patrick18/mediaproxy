# mediaproxy

VoIP SBC / media-relay platform. See [ARCHITECTURE.md](./ARCHITECTURE.md) for the full design.

Phase 1 (current): base app (control plane) — Go API + PostgreSQL + Redis, single VPS.

## Fresh install

On a clean Ubuntu 24.04 VPS:

```bash
git clone git@github.com:steven-patrick18/mediaproxy.git /opt/mediaproxy
cd /opt/mediaproxy
sudo ./scripts/install.sh --domain mediaproxy.example.com --admin-email you@example.com
```

Full install + DR runbook in [INSTALL.md](./INSTALL.md). Adding sip_proxy / media nodes is one-click from the panel — **Infrastructure → Nodes → Add node → Provision via SSH**.

## Local dev (on VPS)

```bash
cp .env.example .env       # then fill in DB password
make migrate-up            # apply DB schema
make dev                   # run baseapp on :8080
```

Health: `curl http://127.0.0.1:8080/healthz`
Readiness (DB+Redis): `curl http://127.0.0.1:8080/readyz`

## Layout

| Path             | Purpose                                            |
|------------------|----------------------------------------------------|
| `cmd/baseapp`    | Control-plane HTTP server entrypoint (Go + Gin)    |
| `internal/api`   | HTTP handlers and router                           |
| `internal/config`| Env-driven config loader                           |
| `internal/db`    | Postgres (pgx) + Redis (go-redis) clients          |
| `migrations`     | SQL migrations (golang-migrate)                    |
| `web`            | Admin UI — React + Vite (added later)              |

## Stack

Go 1.26 · PostgreSQL 16 · Redis 7 · Gin · pgx/v5 · go-redis/v9 · golang-migrate · Ubuntu 24.04

## Make targets

| Target           | What it does                              |
|------------------|-------------------------------------------|
| `make build`     | Compile to `bin/baseapp`                  |
| `make dev`       | `go run` the server (with current env)    |
| `make run`       | Build then run the binary                 |
| `make tidy`      | `go mod tidy`                             |
| `make fmt`       | `go fmt ./...`                            |
| `make vet`       | `go vet ./...`                            |
| `make test`      | `go test ./...`                           |
| `make migrate-up`| Apply pending migrations                  |
| `make migrate-down` | Roll back last migration               |
