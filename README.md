# Smart WMS

A Warehouse Management System: Vue 3 frontend, Go/Gin backend, Microsoft SQL
Server, and RabbitMQ, all running under Docker Compose.

> **Status:** Phase 1 — Backend + Infrastructure Foundation. Business
> modules (Products, Inventory, Receiving, Orders, Users, ...) are not yet
> implemented; see `backend/internal/messaging/rabbitmq`'s package doc and
> the integration analysis this phase was built from for what's next.

## What exists today

- A Go API server (`backend/cmd/server`) exposing `GET /api/v1/health`
  and Swagger UI.
- A Go worker (`backend/cmd/worker`) that connects to RabbitMQ
  independently of the API server and consumes a development-only
  `wms.system.health` queue, to prove the publish → exchange → queue →
  consume pipeline works end to end.
- SQL Server and RabbitMQ connection foundations with retry/backoff on
  startup and background reconnect handling — no business tables or
  queues yet.
- The existing Vue frontend, unchanged, served through Nginx with a
  reverse proxy to the backend and Vue Router's history-mode fallback.
- Docker Compose wiring all five services together.

## Prerequisites

- Docker and Docker Compose (Docker Desktop on Windows/Mac, or Docker
  Engine + the Compose plugin on Linux).
- Optional, for local (non-Docker) development:
  - Go 1.23+ (backend)
  - Node 20.19+ or 22.12+ (frontend — see `fontend/package.json`
    `engines`)

## Configure environment variables

Copy the root example file and fill in real values (at minimum, change
`DB_PASSWORD` and `JWT_SECRET`):

```sh
cp .env.example .env
```

`.env` is read automatically by `docker compose` and is git-ignored —
never commit it. See `.env.example` for every variable and what it's
for. `DB_PASSWORD` becomes the SQL Server `sa` password and must satisfy
SQL Server's complexity policy (8+ characters, with upper case, lower
case, a digit, and a symbol) or the `sqlserver` container will fail to
initialize.

If you want to run the Go server or worker directly on your machine
(outside Docker) instead of only through Compose, also copy
`backend/.env.example` to `backend/.env` and export those variables into
your shell before `go run`.

## Start the stack

```sh
docker compose up --build
```

First startup pulls the SQL Server and RabbitMQ images and builds the
backend and frontend images, so it can take a few minutes. SQL Server in
particular can take 20–40 seconds after its container starts before it
accepts connections — the backend and worker both retry with backoff, and
Compose additionally waits for `sqlserver`/`rabbitmq`'s own healthchecks
before starting `backend`/`worker`, so this is expected and self-resolves.

Run in the background instead with:

```sh
docker compose up --build -d
```

## Stop the stack

```sh
docker compose down
```

This stops and removes the containers but **keeps** the `sqlserver_data`
and `rabbitmq_data` volumes, so your database and queue state survive a
restart.

## Reset development containers

To wipe all persisted data (SQL Server data, RabbitMQ state) and start
completely clean:

```sh
docker compose down -v
docker compose up --build
```

To rebuild images from scratch without cache (useful after a dependency
change that Docker's layer cache might otherwise skip):

```sh
docker compose build --no-cache
```

## Access the running stack

| Service              | URL                                       |
| --------------------- | ------------------------------------------ |
| Frontend              | http://localhost                          |
| Backend health check  | http://localhost:8080/api/v1/health       |
| Swagger UI            | http://localhost:8080/swagger/index.html  |
| RabbitMQ Management   | http://localhost:15672                    |
| SQL Server            | localhost:1433                            |

RabbitMQ Management and SQL Server credentials are whatever you set for
`RABBITMQ_USER`/`RABBITMQ_PASSWORD` and `DB_USER`/`DB_PASSWORD` in `.env`.

Host ports are configurable via `FRONTEND_PORT`, `BACKEND_PORT`,
`SQLSERVER_PORT`, `RABBITMQ_AMQP_PORT`, and `RABBITMQ_MGMT_PORT` in
`.env`, in case any of the defaults conflict with something else already
running on your machine.

## Regenerate Swagger documentation

Swagger docs are generated from source, not hand-edited, and are
regenerated automatically as part of every `docker compose build`. To
regenerate them locally (e.g. while editing handler annotations, before
rebuilding the image):

```sh
cd backend
go install github.com/swaggo/swag/cmd/swag@v1.16.4
swag init -g cmd/server/main.go -o docs --parseInternal --parseDependency
```

`--parseInternal` and `--parseDependency` are required because the
response envelope types (`SuccessResponse`, etc.) referenced in handler
annotations live in an internal package rather than the same package as
`main.go`.

## Run Go tests

```sh
cd backend
go fmt ./...
go vet ./...
go test ./...
```

(Or, without a local Go install, run the same commands inside a
container: `docker run --rm -v "$(pwd)/backend:/src" -w /src golang:1.23-bookworm sh -c "go fmt ./... && go vet ./... && go test ./..."`.)

## Project layout

```
backend/    Go API server + worker (see backend's own package docs)
fontend/    Vue 3 frontend (unchanged in this phase; directory name is
            the project's existing spelling, not a typo introduced here)
docker-compose.yml
.env.example
```
