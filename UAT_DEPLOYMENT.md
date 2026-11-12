# WMS UAT Deployment Package

This document prepares the project for deployment to a **separate UAT
machine** — the development machine's RAM has proven insufficient for
reliable live verification (see `PROJECT_STATUS.md`'s Phase 26 notes),
so Phase 25/26 live verification is being moved to dedicated hardware
instead of continuing to fight resource contention on the dev box.

**Topology**: this UAT deployment runs SQL Server **outside Docker**,
using an instance already installed on the UAT Windows Server itself —
`docker-compose.yml` has no `sqlserver` service in this configuration.
Backend, worker, and migrate connect out to it via
`DB_HOST=host.docker.internal`. RabbitMQ still runs in Docker,
unchanged. Frontend is published on host port **9098**, backend on
**9099**.

**No business logic was changed to produce this document or its
configuration updates.** This pass touched `docker-compose.yml`
(removed the `sqlserver` service and every `depends_on: sqlserver:`
reference, added `extra_hosts` for `host.docker.internal`, changed the
frontend/backend host port defaults to 9098/9099),
`.env.uat.example` (external SQL Server variables, new ports, CORS
origin), and this document — no Go, Vue, or SQL migration source was
touched. Phase 25 remains `CODE COMPLETE — LIVE VERIFICATION PENDING`
and Phase 26 remains `OFFLINE UAT PREPARATION COMPLETE — LIVE UAT
PENDING` until the sequences in sections 14–15 below are actually run
on the UAT machine.

---

## 1. Required project files

Copy the following to the UAT machine, preserving directory structure:

```
backend/                   (entire directory)
fontend/                   (entire directory — see below for exclusions)
docker-compose.yml
.env.example
.env.uat.example           (new — this package's UAT template)
README.md
Claude.md
PROJECT_STATUS.md
UAT_CHECKLIST.md
PRODUCTION_READINESS.md
UAT_DEPLOYMENT.md          (this file)
.gitignore
```

**Do NOT copy:**

| Path | Why |
|---|---|
| `.env` (root) | Contains this dev machine's actual passwords/secrets — never copy real credentials to another environment. UAT gets its own `.env` generated from `.env.uat.example` (section 3). |
| `backend/.env`, `backend/.env.example` | Only relevant for running the Go binaries directly outside Docker on a dev machine; the UAT deployment path is Docker-only (`docker compose`), which reads the root `.env` — these files aren't needed and `backend/.env` has real local values. |
| `fontend/node_modules/` | ~hundreds of MB of installed dependencies; the frontend Docker build runs its own `npm install` from `package.json`/`package-lock.json` inside the image — copying this directory is unnecessary and risks a stale/platform-mismatched install being picked up. |
| `fontend/dist/` | A stale local build output; the frontend Docker build produces its own. |
| `backend/docs/docs.go`, `backend/docs/swagger.json`, `backend/docs/swagger.yaml` | Generated output (gitignored on purpose — see `.gitignore`). The backend Dockerfile's builder stage runs `swag init` fresh on every `docker compose build`, so these are always regenerated from source; copying stale ones is unnecessary and they'd be overwritten anyway. |
| `backend/wms-server`, `backend/wms-worker`, any `*.exe`/`*.test`/`*.out` | Local build artifacts from this dev machine's OS/architecture — irrelevant and potentially incompatible with the UAT machine. |
| Any `.vscode/`, `.idea/`, OS files (`.DS_Store`, `Thumbs.db`) | Editor/OS cruft, not part of the deployable project. |
| SQL Server / RabbitMQ Docker volumes from this dev machine | There are none to copy in the first place — this dev machine's Docker Desktop was never successfully brought up alongside a completed live-data session in a way that produced UAT-relevant data, and even if it had, UAT must start from a clean, migration-built schema (section 12) rather than an inherited dev database. |

If you have git available, the cleanest transfer is `git archive` (or a
plain `rsync`/`robocopy` with the excludes above) rather than a raw
directory copy, specifically to avoid dragging `node_modules`/`dist`
along.

---

## 2. Docker Compose requirements

- **Docker Engine** with the **Compose v2 plugin** (`docker compose`,
  not the legacy standalone `docker-compose` v1 binary) — this project's
  `docker-compose.yml` uses the `name:` top-level key, which requires a
  reasonably current Compose v2 (any release from the last couple of
  years is fine; this dev machine's own `docker-compose` plugin is
  v5.0.0-desktop.1, well past the minimum).
- No Kubernetes/Swarm — this is a single-host Compose deployment.
- **Recommended minimum host resources for Docker** in this topology
  (SQL Server no longer counts against Docker's own budget, since it
  runs natively on the Windows Server): **at least 4GB RAM available
  to Docker** (RabbitMQ, backend, worker, frontend, and overhead — all
  modest, see the container memory figures already observed on the dev
  machine in `PROJECT_STATUS.md`'s Phase 26 notes), 2 vCPUs, ~5GB free
  disk for images. SQL Server's own resource needs are whatever the
  Windows Server's existing SQL Server installation is already sized
  for — outside this document's scope.
- Internet access during the **first** `docker compose build` (to pull
  base images `golang:1.23-bookworm`, `alpine:3.20`,
  `node:*`/whatever the frontend Dockerfile's base is,
  `rabbitmq:3.13-management` — `mcr.microsoft.com/mssql/server` is no
  longer pulled at all in this topology) — not required afterward for
  routine start/stop.
- **Network reachability**: the Docker host (the UAT Windows Server
  itself, since Docker Desktop for Windows runs containers in a local
  WSL2 VM on the same machine) must be able to reach its own SQL
  Server instance on TCP 1433 via `host.docker.internal` — see
  section 5 for the exact SQL Server-side settings this depends on
  (TCP/IP enabled, static port, Windows Firewall rule).

---

## 3. UAT `.env` template

Use the new `.env.uat.example` (repo root, created alongside this
document) instead of the dev-oriented `.env.example`. On the UAT
machine:

```sh
cp .env.uat.example .env
```

Then edit `.env` and replace every value marked `UAT_CHANGE_ME_*` /
`REQUIRED CHANGE` in that file's own comments:

| Variable | Why it must change |
|---|---|
| `JWT_SECRET` | Signs every issued token — must be unique to UAT so a UAT-issued token can never be replayed elsewhere and vice versa. Generate with `openssl rand -base64 48` or equivalent. |
| `DB_USER` / `DB_PASSWORD` | Must be a **real, already-existing login on the UAT Windows Server's own SQL Server instance** — this project doesn't create that login, only the `wms_uat` database once it can connect. Must have `CREATE DATABASE` permission for the first run (section 5/7). |
| `RABBITMQ_PASSWORD` | Same reasoning — don't reuse the dev credential. |
| `SEED_ADMIN_PASSWORD` | Becomes the first real login testers use — pick something UAT-specific, not the dev placeholder. |
| `CORS_ALLOWED_ORIGINS` | `*` is a development convenience (see `PRODUCTION_READINESS.md` section 9); set to `http://<UAT_SERVER_IP>:9098` once the UAT host's real address is known — see section 4's frontend-connectivity note for why this matters less than it sounds (the main app flow doesn't depend on it). |

Everything else in `.env.uat.example` (ports, `DB_NAME=wms_uat`,
`RABBITMQ_USER=wms_uat`, `DB_HOST=host.docker.internal`, timeouts,
feature-tuning values) is a reasonable UAT default and does not need
to change unless the UAT host's own constraints require it (e.g. a
port collision — see section 4).

**No real secret is packaged anywhere in this deliverable** — every
value in `.env.uat.example` is either an obvious placeholder
(`UAT_CHANGE_ME_...`) or a non-secret default (hostnames, ports,
timeouts) already safe to publish.

---

## 4. Required ports

| Port | Service | Purpose | Env override |
|---|---|---|---|
| **9098** | frontend | Web UI | `FRONTEND_PORT` |
| **9099** | backend | REST API + Swagger UI | `BACKEND_PORT` |
| 1433 | SQL Server (external, on the Windows host — not a Docker port mapping) | SQL Server TCP, reached via `host.docker.internal:1433` | `DB_PORT` |
| 5673 | rabbitmq | AMQP (remapped from RabbitMQ's own default 5672 — see below) | `RABBITMQ_AMQP_PORT` |
| 15673 | rabbitmq | Management UI (remapped from 15672) | `RABBITMQ_MGMT_PORT` |

`docker-compose.yml`'s frontend/backend port mappings now default to
9098/80 and 9099/8080 respectively (`${FRONTEND_PORT:-9098}:80`,
`${BACKEND_PORT:-9099}:8080`) — `.env.uat.example` sets the same
values explicitly. `.env.uat.example` also remaps the two RabbitMQ
ports to 5673/15673 by default: this exact dev machine had a stray,
unrelated `rabbitmq:3-management` container from a different project
squatting on the *default* 5672/15672, so the remapped defaults avoid
silently binding to/conflicting with anything similar on the UAT host.
1433 is **not** a Compose port mapping in this topology at all — it's
the external SQL Server instance's own listening port on the Windows
host, reached directly via `host.docker.internal`, not published
through Docker.

Before first startup, confirm nothing else is already listening on
9098/9099/5673/15673 (`netstat -ano | findstr :<port>` on Windows) —
a bound port fails the container's own startup, not silently. Also
confirm SQL Server's own port 1433 is actually listening on the
Windows host (`netstat -ano | findstr :1433`) before starting the
stack — see section 5.

### Frontend API connectivity / CORS — what actually needs to change

**Inspected before making any change** (per this task's own
instruction not to change this blindly): `fontend/nginx.conf` already
reverse-proxies `/api/` and `/swagger/` to `http://backend:8080/...`
using Compose's internal service-name DNS (`backend`), and the
frontend's own `VITE_API_BASE_URL` is the **relative** path `/api/v1`
(consumed by `fontend/src/services/api.js`). This means:

- The browser only ever talks to the frontend's own origin
  (`http://<UAT_SERVER_IP>:9098`) — its `/api/v1/...` calls are same-origin
  relative requests, not cross-origin ones.
- Nginx then proxies those internally to `backend:8080` — the
  container's **fixed internal port**, which never changes regardless
  of what host port the backend is published on (9099). Changing
  `BACKEND_PORT` from 8080 to 9099 only affects how something *outside*
  Docker (a browser hitting Swagger directly, `curl` health checks)
  reaches the backend — it has no effect on the frontend's own
  internal proxy target.

**Conclusion: `VITE_API_BASE_URL` stays `/api/v1` (relative) — it was
deliberately left unchanged.** Setting it to an absolute
`http://<UAT_SERVER_IP>:9099/api/v1` would be both unnecessary and a
regression: it would make the frontend's own API calls genuinely
cross-origin (reintroducing a real CORS dependency for the main app
flow, not just the edge case below) for zero benefit, and would
diverge from the reverse-proxy architecture CLAUDE.md itself prefers
("prefer reverse proxying... this avoids unnecessary production CORS
complexity"). `nginx.conf`'s proxy target (`backend:8080`) also did not
need to change, for the same reason.

`CORS_ALLOWED_ORIGINS` was still updated to
`http://<UAT_SERVER_IP>:9098` per this task's explicit instruction
(section 3) — it isn't needed for the main frontend-through-Nginx flow
described above, but does matter for any direct cross-origin API
access (a local `vite dev` instance pointed at the UAT backend, a
script/tool calling the API from a different origin, etc.), so setting
it correctly is still the right default.

---

## 5. SQL Server configuration

**This UAT deployment does not run SQL Server in Docker.** It uses an
instance already installed directly on the UAT Windows Server —
`docker-compose.yml` has no `sqlserver` service, no `sqlserver_data`
volume, and no `depends_on: sqlserver:` anywhere (verified — see
section 14 of the original request / the validation note at the end
of this document).

**What the UAT Windows Server's own SQL Server instance needs, before
first deployment:**

1. **TCP/IP protocol enabled** (SQL Server Configuration Manager →
   SQL Server Network Configuration → Protocols) on a **static port
   1433** (or whatever `DB_PORT` is set to) — SQL Server's default
   install often has TCP/IP disabled or dynamic-port until explicitly
   configured otherwise.
2. **Windows Firewall** allows inbound TCP on that port from the
   Docker host's internal network (on Docker Desktop for Windows,
   containers reach the host through a local network the firewall
   sees as coming from the machine itself, but a restrictive server
   firewall policy may still need an explicit inbound rule for 1433).
3. **SQL Authentication mode enabled** (not Windows-Authentication-only)
   — this project's config (`DB_USER`/`DB_PASSWORD`) always connects
   with a SQL login, never Windows integrated auth.
4. **A SQL login with `CREATE DATABASE` permission** (e.g. `sa`, or a
   dedicated login granted the `dbcreator` server role) — `wms-migrate`
   connects to `master` first and runs `CREATE DATABASE [wms_uat]`
   itself if that database doesn't exist yet (see section 7), exactly
   as it would against a fresh Docker-managed instance. If the login
   you provide lacks this permission, either grant it or have a DBA
   pre-create an empty `wms_uat` database and grant that login
   `db_owner` on it instead.
5. Confirm connectivity **before** running `docker compose up` — from
   the Windows Server itself:
   `sqlcmd -S localhost,1433 -U <DB_USER> -P <DB_PASSWORD> -Q "SELECT 1"`
   should succeed. If that fails, nothing Docker-side will work either.

**Container-side**: `DB_HOST=host.docker.internal` /
`DB_PORT=1433` in `.env` (section 3), reached with the `extra_hosts:
host.docker.internal:host-gateway` entry now present on the `backend`,
`worker`, and `migrate` services in `docker-compose.yml` — this makes
the alias resolve reliably regardless of the underlying Docker engine,
not just on Docker Desktop's own built-in magic DNS.

No SQL Server configuration file, image, or `.bak` needs to be copied
from the dev machine — the schema is built entirely from
`backend/migrations/*.up.sql` by the `migrate` service (section 7)
against whatever empty (or already-`wms_uat`-populated, on a re-run)
database it finds.

`MSSQL_PID=Developer`/licensing considerations from a Docker-managed
instance no longer apply here — licensing is whatever the UAT Windows
Server's own existing SQL Server installation already has, outside
this project's control.

---

## 6. RabbitMQ configuration

- Image: `rabbitmq:3.13-management` (pulled fresh).
- Credentials via `RABBITMQ_USER`/`RABBITMQ_PASSWORD`/`RABBITMQ_VHOST`
  in `.env`.
- Management UI reachable at `http://<UAT_SERVER_IP>:15673` (or whatever
  `RABBITMQ_MGMT_PORT` is set to) — useful during UAT to watch the
  `wms.system.health` dev heartbeat, `wms.stock_adjustment.pending_reminder`,
  and `wms.report.export` queues process messages live.
- Persistent volume: `rabbitmq_data` — see section 12 for UAT-scoping
  guidance (this is now the only Docker-managed data volume in this
  topology, alongside `export_files`).
- Healthcheck: `rabbitmq-diagnostics -q ping`.
- No exchange/queue setup is manual — `backend`'s own startup declares
  the topology (`internal/messaging/rabbitmq.DeclareTopology`) against
  a fresh RabbitMQ instance automatically; nothing to import or
  configure by hand.

---

## 7. Migration procedure

Migrations are already wired as their own one-shot Compose service
(`migrate`), which runs `wms-migrate up && wms-migrate seed` and must
complete successfully before `backend`/`worker` are allowed to start
(`depends_on: migrate: condition: service_completed_successfully` in
`docker-compose.yml`) — this happens **automatically** as part of
`docker compose up` (section 9).

To run it standalone first and inspect its output before starting
anything else (recommended for the first UAT deployment, so migration
errors are visible on their own rather than buried under backend/
worker startup logs):

```sh
docker compose run --rm migrate
```

Exact command executed inside that container:
`wms-migrate up && wms-migrate seed`

`wms-migrate up` is idempotent and safe to re-run at any time: it
tracks applied versions in a `schema_migrations` table
(`backend/cmd/migrate/main.go`) and only applies migrations not yet
recorded there — re-running it after a partial or repeated deployment
never re-applies an already-applied migration. Expect it to apply
migrations **0001 through 0038** on a fresh UAT database (0038 is
Order Management's `orders`/`order_items` tables — confirm this is the
highest version number in `backend/migrations/` before running, in
case newer migrations have been added since this document was
written).

Before this runs for the first time, `wms-migrate` also connects to
the external SQL Server's `master` database and runs `CREATE DATABASE
[wms_uat]` if that database doesn't exist yet
(`ensureDatabaseExists` in `backend/cmd/migrate/main.go`) — this is
the same behavior it would have against a fresh Docker-managed
instance, just now pointed at the Windows Server's own instance. This
is why `DB_USER` needs `CREATE DATABASE` permission there — see
section 5.

---

## 8. Initial admin / RBAC seed procedure

Part of the same `migrate` service command (`wms-migrate seed`, chained
after `up` — see section 7), so it runs automatically with no separate
step required. It:

1. Seeds the full permission catalog from
   `backend/internal/rbac/catalog.go` (55 entries as of the `order`
   module's addition in Phase 25 — verify this count in the `migrate`
   container's own log line `"permissions seeded" count=...` against
   the current catalog if it may have grown further).
2. Seeds the 7-role catalog (`ADMIN`, `WAREHOUSE_MANAGER`, `RECEIVING`,
   `QC`, `PUTAWAY`, `PICKING`, `SHIPPING`) and each role's default
   permission grants.
3. Creates one development/UAT admin user from `SEED_ADMIN_USERNAME`/
   `SEED_ADMIN_PASSWORD` (section 3) — password bcrypt-hashed before
   storage, never logged.

Also idempotent — insert-if-missing for permissions and role grants
(`cmd/migrate/main.go`'s `seedPermissions`/`seedRolePermissions`), and
the admin user is only created if a user with that username doesn't
already exist. Safe to re-run.

**Verify**: after this step, log in as the seeded admin (section 14,
AUTH-01) and confirm `GET /api/v1/permissions` includes `order.view`,
`order.create`, `order.update`, `order.approve` (AUTH-07).

---

## 9. Backend / frontend / worker startup procedure

First deployment (builds images, since none exist yet on the UAT
machine):

```sh
docker compose up -d --build
```

This single command, in order (enforced by `depends_on` conditions in
`docker-compose.yml`, not manual sequencing):

1. Starts `rabbitmq`, waits for its healthcheck. (No `sqlserver` step
   in this topology — the external SQL Server on the Windows host is
   assumed already running and reachable; see section 5 for the
   preflight checks to do *before* this command, since Compose has no
   way to healthcheck a service it doesn't manage.)
2. Runs `migrate` to completion (section 7–8) — it has no `depends_on`
   at all in this topology (it never touched RabbitMQ, and its former
   `sqlserver: condition: service_healthy` gate was removed along with
   the service itself); it starts as soon as its image is built and
   connects straight to the external SQL Server.
3. Starts `backend`, `worker` (both wait for `rabbitmq` healthy and
   `migrate` completed-successfully), then `frontend`.

Subsequent starts (images already built, no source changes):

```sh
docker compose up -d
```

After a source change (e.g. a future code update deployed to this same
UAT box), rebuild only the images that actually changed rather than
the whole stack — per this project's own established resource-safety
practice:

```sh
docker compose up -d --build backend frontend worker
```

(Never rebuild `rabbitmq` — it's a pulled base image,
not built from this project's source, and rebuilding it is
unnecessary and risks disturbing their running state.)

---

## 10. Health-check procedure

```sh
docker compose ps
```

Confirm all 4 Docker-managed services (`rabbitmq`, `backend`,
`worker`, `frontend`) show `healthy` (or, for `worker`, simply `Up` —
it has no HTTP healthcheck of its own since it exposes no port) and
`migrate` shows `Exited (0)`. There is no `sqlserver` row to check
here — confirm the external SQL Server is up via section 5's own
preflight `sqlcmd` check instead, done before `docker compose up`.

Then:

```sh
curl http://<UAT_SERVER_IP>:9099/api/v1/health
```

Expected response:

```json
{
  "success": true,
  "data": {
    "status": "ok",
    "database": "connected",
    "rabbitmq": "connected"
  }
}
```

`"status": "degraded"` with `"database": "disconnected"` means the
backend can't reach the external SQL Server — recheck section 5's
preflight steps (TCP/IP enabled, firewall rule, correct
`DB_HOST`/`DB_PORT`/`DB_USER`/`DB_PASSWORD`) rather than a container
log, since there's no `sqlserver` container to inspect here.
`"rabbitmq": "disconnected"` — check `docker compose logs rabbitmq` /
`docker compose logs backend` as usual.

---

## 11. Swagger verification

```
http://<UAT_SERVER_IP>:9099/swagger/index.html
```

Checklist:

1. Page loads (confirms `SWAGGER_ENABLED=true` and the backend is up).
2. Search/scroll for the `Orders` tag — confirm all 6 endpoints appear
   (`GET /orders`, `GET /orders/{id}`, `POST /orders`,
   `PUT /orders/{id}`, `POST /orders/{id}/create-picking-task`,
   `POST /orders/{id}/cancel}`), each with request/response schemas.
3. `POST /api/v1/auth/login` via the UI (or `curl`) with the seeded
   admin credentials, copy the returned `access_token`.
4. Click **Authorize**, enter `Bearer <token>`.
5. Execute `GET /orders` through the UI — confirm a real 200 response,
   not a 401/403 — proves the Bearer-auth wiring works end to end
   through Swagger, not just via direct `curl` calls.

---

## 12. Database isolation requirements

Unlike the RabbitMQ/backend/worker/frontend containers, **SQL Server
isolation is no longer Docker's job** in this topology — it runs on
the UAT Windows Server's own instance, which may already host other
databases (other projects, or even a future production database on
the same physical SQL Server). Isolation here is a SQL Server-level
concern, not a container-level one:

- **Use a UAT-distinct database name.** `.env.uat.example` sets
  `DB_NAME=wms_uat` (not `wms`, the dev default, and not whatever a
  future production deployment might use) — makes it unmistakable in
  `sqlcmd` output, backend logs, and SSMS-style tooling which database
  belongs to this UAT environment. `wms-migrate` only ever creates and
  touches this one named database (section 5/7) — it has no way to
  reach any other database on that same SQL Server instance.
- **Use a UAT-distinct SQL login if possible.** If the DBA can grant a
  dedicated login (rather than reusing `sa`) scoped to just the
  `wms_uat` database, that further limits blast radius — this project
  only ever needs `CREATE DATABASE` once (first run) and normal DML/DDL
  against `wms_uat` afterward, never access to any other database on
  that instance.
- **Never point `DB_HOST`/`DB_NAME` at a dev or production database.**
  There is no code-level guard against this (the app connects to
  whatever `.env` says) — it is entirely an operational discipline of
  keeping UAT's `.env` pointed at `wms_uat` and nothing else.
- **RabbitMQ isolation is still Docker-level**, unchanged from before —
  use a UAT-distinct Compose project name so its container/volume/
  network are namespaced away from any other stack that might ever
  share this host:

  ```sh
  export COMPOSE_PROJECT_NAME=wms-uat
  ```

  (or pass `-p wms-uat` on every `docker compose` command instead).
  With this set, the volumes from section 6 become
  `wms-uat_rabbitmq_data` / `wms-uat_export_files` instead of `wms_*`
  — trivially distinguishable from a dev deployment of the same
  `docker-compose.yml` running elsewhere, and from any future
  production deployment which should use its own project name (e.g.
  `wms-prod`) in turn.
- **If UAT and dev/prod ever run on the same physical host** (not
  recommended, but possible for a small team), the project-name
  namespacing above (for RabbitMQ/containers) plus the distinct
  `DB_NAME`/login (for SQL Server) plus the port remapping in section 4
  together guarantee no collision between them.

---

## 13. UAT backup/restore guidance

UAT data is expected to be **disposable and reproducible from
migrations + seed** — there is no requirement to preserve it
indefinitely. Backup/restore is now a SQL-Server-on-Windows operation,
not a `docker compose exec sqlserver` one (there is no `sqlserver`
container in this topology) — two levels, depending on need:

**Level 1 — routine persistence (already automatic, for RabbitMQ/
exports only).** As long as you never run `docker compose down -v`
(see section 16), the `rabbitmq_data`/`export_files` volumes survive
`docker compose down`/`docker compose up` cycles, container restarts,
and host reboots. The `wms_uat` **database's own persistence is
whatever the Windows Server's SQL Server installation already does**
— outside Docker's control entirely, governed by that instance's own
normal operational practices (this is actually simpler than the
dev/Docker-managed case: a real SQL Server instance a DBA already
operates typically already has its own backup/maintenance plan).

**Level 2 — snapshot a specific UAT state to return to** (e.g. before
running a destructive test case, so you can reset to "known good"
without re-running the entire UAT matrix from scratch). Run this
directly on the UAT Windows Server (via `sqlcmd`, SSMS, or a scheduled
maintenance plan — whichever the DBA prefers), not through Docker:

```powershell
sqlcmd -S localhost,1433 -U <DB_USER> -P <DB_PASSWORD> -Q ^
  "BACKUP DATABASE wms_uat TO DISK = 'C:\SQLBackups\wms_uat.bak'"
```

To restore that snapshot later:

```powershell
sqlcmd -S localhost,1433 -U <DB_USER> -P <DB_PASSWORD> -Q ^
  "RESTORE DATABASE wms_uat FROM DISK = 'C:\SQLBackups\wms_uat.bak' WITH REPLACE"
```

(Adjust the backup path to wherever the SQL Server service account has
write access — `C:\SQLBackups\` is illustrative, not a requirement of
this project.)

**Do not restore a dev-machine backup into UAT, and do not restore a
UAT backup into anything else** — keep the environments' data
completely separate, per section 12. Since dev's own database has
historically run inside a Docker-managed `sqlserver` container (a
different topology from this UAT deployment), a dev backup wouldn't be
a meaningful restore target here anyway.

---

## 14. Phase 25 live verification sequence

Run this **first**, before the broader Phase 26 matrix — it is the
newest, least-proven module. Full detail and rationale in
`PRODUCTION_READINESS.md` section 17 and `UAT_CHECKLIST.md` section 15
(cases `ORD-01` through `ORD-13`, plus `AUTH-07` through `AUTH-10` for
the RBAC side); summarized order to execute:

1. `AUTH-07` — confirm `order.view`/`order.create`/`order.update`/
   `order.approve` exist via `GET /permissions`.
2. `AUTH-08`/`AUTH-09`/`AUTH-10` — ADMIN and WAREHOUSE_MANAGER have
   full Order Management access; an unauthorized role (e.g. PICKING)
   gets 403. **Use freshly issued tokens** (login after seeding) —
   permissions are embedded in the JWT at login, not re-queried per
   request (documented tradeoff, see `PRODUCTION_READINESS.md`
   section 7).
3. `ORD-01` — create an order, confirm zero inventory effect.
4. `ORD-02`/`ORD-03` — update while PENDING; rejected once not PENDING.
5. `ORD-04` — duplicate `external_reference` rejected cleanly, no
   partial rows left behind.
6. **`ORD-07` — insufficient-stock rollback.** The single most
   important Phase 25 test: attempt `create-picking-task` for a
   quantity exceeding available stock, confirm the order is left
   exactly PENDING/unlinked, no picking task, no reservation, no
   ledger row, and the order is immediately retryable.
7. `ORD-05` — the success path: sufficient stock, confirm exactly one
   picking task created, reservation visible via `GET /inventory`, no
   ledger row written by this call.
8. `ORD-06` — duplicate `create-picking-task` on the same order
   returns 409, no second task, no second reservation.
9. `ORD-12` — concurrent `create-picking-task` requests (if
   practical): exactly one winner, exactly one task, exactly one
   reservation.
10. `ORD-08`/`ORD-09` — fulfillment status tracks real Picking/Shipping
    progress correctly, both via the single-order and list endpoints.
11. `ORD-10`/`ORD-11` — cancellation allowed while PENDING, rejected
    once a picking task exists.
12. `ORD-13` — full browser flow through the actual frontend.
13. Swagger check (section 11 above).

**If every case above passes**, update `PROJECT_STATUS.md`'s Phase 25
heading to `VERIFIED COMPLETE` and record the results (this document
does not do that update itself — it is prepared but not executed,
since no live testing has happened yet in this session).

---

## 15. Phase 26 — 85-case UAT sequence

Once Phase 25's own subset (section 14) is clean, work through the
**entire** `UAT_CHECKLIST.md` — all 85 cases across all 16 module
areas (Auth/RBAC, Warehouse/Zone/Location, Product, Receiving,
Putaway, Replenishment, Inventory, Picking, Shipping, Stock
Adjustment, Stock Count, Notifications, Reports/Export, Dashboard,
Order Management, Cancellation/Audit). For each case:

1. Execute the steps against the UAT deployment.
2. Fill in **Actual result** with what really happened.
3. Flip **Status** from `PENDING` to `PASS` or `FAIL`.
4. On `FAIL`, record the exact error/response in **Notes** and stop
   propagating to dependent cases until the defect is triaged (see
   section 16's "fix only verified defects" note).
5. Update the summary tally table at the bottom of
   `UAT_CHECKLIST.md` as you go.

Phase 26 itself is only `VERIFIED COMPLETE` once all 85 cases have a
real, non-`PENDING` status and no unresolved `FAIL` remains.

---

## 16. Rollback / cleanup procedure

**Between test runs, keeping data** (most common — e.g. pausing UAT
for the day):

```sh
docker compose down
```

Stops and removes containers, **keeps** the named volumes
(`rabbitmq_data`, `export_files` — or their `wms-uat_`-prefixed
equivalents per section 12). Restart later with `docker compose up -d`
and everything picks up exactly where it left off. **This has no
effect on the `wms_uat` database at all** — it lives on the external
SQL Server, entirely outside anything `docker compose down` touches.

**Resetting RabbitMQ/export-file state** (deliberate, destructive —
only when you specifically want a clean slate for those two, e.g.
stuck/undeliverable messages during testing):

```sh
docker compose down -v
docker compose up -d --build
```

⚠️ `-v` removes the `rabbitmq_data`/`export_files` volumes — any queued
messages and generated export files are gone. **It does NOT touch the
`wms_uat` database** — this is a meaningful difference from the old
Docker-managed-SQL-Server topology, where the same command used to
wipe the database too. Run this only as an explicit, intentional
action — never as a troubleshooting reflex. Do **not** run
`docker system prune`, `docker volume prune`, or `docker system
prune -a` — these are untargeted and can affect containers/volumes/
images belonging to other projects on a shared host; this project
never needs them.

**Resetting the UAT database itself** (separate, SQL-Server-side
action — not a Docker command at all):

```powershell
sqlcmd -S localhost,1433 -U <DB_USER> -P <DB_PASSWORD> -Q "DROP DATABASE wms_uat"
```

Then re-run `docker compose run --rm migrate` (section 7) — it
recreates `wms_uat` from scratch and reapplies every migration plus
the RBAC/admin seed, exactly like a first deployment. This is the
"start UAT data over completely" action; do it deliberately, and only
after confirming (with the DBA, if it's not your own instance) that
`wms_uat` genuinely isn't needed anymore.

**Rolling back a specific migration** (rare — only if a migration
itself needs to be undone, not for routine data reset):

```sh
docker compose run --rm migrate wms-migrate down
```

Rolls back exactly the most recently applied migration one step at a
time (per `backend/cmd/migrate/main.go`'s `migrateDown`) — run it
repeatedly to roll back further. Prefer the full reset above for a
routine "start UAT over" — this is for the specific case of a bad
migration needing surgical removal.

**Removing built images** (e.g. before rebuilding from a clean state,
or decommissioning this UAT box):

```sh
docker compose down --rmi local
```

Removes only the images this project's own `docker-compose.yml` built
(`wms-backend`, `wms-frontend`, `wms-worker`, `wms-migrate`) — leaves
the pulled base images (`rabbitmq:3.13-management`, `golang`,
`alpine`, etc.) alone, so a subsequent rebuild doesn't need to re-pull
them from the internet. (`mcr.microsoft.com/mssql/server` is not
pulled at all in this topology — see section 5.)

---

## Summary

- **Files to copy**: everything in section 1's first list.
- **Files NOT to copy**: everything in section 1's table (dev `.env`,
  `node_modules`, `dist`, generated Swagger output, local build
  binaries, editor/OS cruft).
- **UAT prerequisites**: Docker Engine + Compose v2 plugin on the UAT
  Windows Server, ~4GB RAM available to Docker (SQL Server no longer
  runs in Docker — see section 2), ~5GB free disk, one-time internet
  access to pull base images, and a SQL Server instance already
  installed and reachable on that same Windows Server (TCP/IP enabled,
  static port 1433, firewall rule, a login with `CREATE DATABASE`
  permission — see section 5).
- **Exact deployment commands**:
  ```sh
  cp .env.uat.example .env
  # edit .env: DB_USER/DB_PASSWORD (real external SQL Server login),
  # JWT_SECRET, RABBITMQ_PASSWORD, SEED_ADMIN_PASSWORD, CORS_ALLOWED_ORIGINS
  export COMPOSE_PROJECT_NAME=wms-uat
  docker compose up -d --build
  ```
- **Exact migration command**: `docker compose run --rm migrate` (or
  let it run automatically as part of the command above) — creates
  `wms_uat` on the external SQL Server if it doesn't exist, applies
  migrations 0001–0038, seeds RBAC + the admin user.
- **Exact verification URL/endpoints**:
  `http://<UAT_SERVER_IP>:9099/api/v1/health`,
  `http://<UAT_SERVER_IP>:9099/swagger/index.html`,
  `http://<UAT_SERVER_IP>:9098/` (frontend), `http://<UAT_SERVER_IP>:15673`
  (RabbitMQ Management).
- **SQL Server**: external, `host.docker.internal:1433`, database
  `wms_uat` — no `sqlserver` service, volume, or `depends_on` reference
  remains anywhere in `docker-compose.yml` (verified — see this
  document's own validation note and the final report for this task).
