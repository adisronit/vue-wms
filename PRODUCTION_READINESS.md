# WMS Production Readiness Review (Static / Offline)

Performed as part of Phase 26 offline preparation. Docker Desktop was
unavailable for the entire review, so **everything in this document is
a static (source-code and configuration) review** — nothing here was
confirmed against a running system. Anything that needs a live system
to actually verify is called out explicitly and cross-referenced to
`UAT_CHECKLIST.md`.

No business logic was changed while writing this document — every item
below is either a confirmed-good finding, a documented and accepted
tradeoff already present in the code, or a recommendation for a future
explicit decision. Nothing here is a "verified defect that must be
fixed before Phase 26 offline prep can complete."

---

## 1. Migrations (0001–0038)

Reviewed every `.up.sql`/`.down.sql` pair. Findings:

- **Every table has an explicit named `PRIMARY KEY`** (`PK_*`) — no
  implicit/unnamed keys anywhere.
- **Every real cross-table reference is a named `FOREIGN KEY`** (`FK_*`)
  — no orphan-able child rows possible (e.g. `order_items.order_id` →
  `orders.id`, `orders.picking_task_id` → `picking_tasks.id`).
- **Uniqueness is enforced at the database, not just the application**,
  everywhere it matters: `warehouses.code`, `zones(warehouse_id, code)`,
  `locations(zone_id, code)`, `products.sku`, `products.barcode`
  (partial index, `WHERE barcode IS NOT NULL` — correctly allows
  multiple NULLs), `inventory(product_id, location_id, lot_no)`,
  `receipts.receipt_no`, `putaway_tasks.putaway_no`,
  `picking_tasks.pick_no`, `shipments.shipment_no`,
  `shipments.picking_task_id` (one shipment per picking task),
  `stock_adjustments.adjustment_no`, `stock_counts.count_no`,
  `notifications(message_id, user_id)`, `export_jobs.message_id`,
  `replenishment_tasks.task_no`, `orders.order_no`,
  `orders.external_reference`, `orders.picking_task_id`.
- **Every idempotency-guard table's PK is the idempotency key itself**
  (`receipt_confirmations`, `putaway_confirmations`,
  `picking_confirmations`, `shipment_confirmations`,
  `stock_adjustment_confirmations`, `stock_count_confirmations`,
  `replenishment_confirmations` — all `PK (idempotency_key)`), so a
  replayed key hits the PK violation as the actual enforcement
  mechanism, not just application-level checking.
- **Quantity/status invariants are CHECK-constrained**, not just
  validated in Go: `inventory.reserved_quantity >= 0 AND <= quantity`,
  every `*_qty > 0` line-quantity column, every status column's
  allowed-value set. `0029`/`0030` show the project already knows how
  to safely `ALTER`/replace a CHECK constraint when a new status value
  is added later (shipments CANCELLED, stock_counts
  ADJUSTMENT_CREATED) — a real, working precedent for future schema
  evolution.
- **No cascading deletes anywhere** (every FK is the SQL Server default
  `NO ACTION`) — consistent with this codebase's "nothing is ever hard-
  deleted out from under history" convention (cancel, don't delete).
  This is intentional, not an oversight.
- Migration 0038 (orders/order_items) matches this pattern exactly —
  reviewed above in Phase 25's own prior write-up, unchanged since.

**Minor observations (not defects, no action taken)**:
- `order_items` has an index on `order_id` but not on `product_id`
  (unlike `receipt_items`/`stock_adjustment_items`, which also only
  index their own parent FK). Given `order_items` is expected to be a
  low-cardinality-per-order table queried almost exclusively by
  `order_id`, this matches the existing precedent and is not flagged as
  a gap.
- `stock_adjustment_reminders`' PK is `stock_adjustment_id` itself
  (one-row-per-adjustment cooldown tracker) — correct for its stated
  purpose (Phase 21), just noted here since its shape differs from
  every other "confirmations" table.

**Action item**: `cmd/migrate up` has not been run against a live
database with migration 0038 present — `UAT_CHECKLIST.md` AUTH-07 and
the live test sequence below cover confirming it applies cleanly.

---

## 2. Foreign Keys / Unique Constraints / Indexes

Covered inline in section 1 above — no gaps found beyond the two minor
observations noted. Every workflow table that needs a warehouse-scoped
or status-scoped list query has the matching index (`IX_*_warehouse_id`,
`IX_*_status` throughout).

---

## 3. Transaction Boundaries

Verified via direct grep across every `internal/*/service.go`:

```
dbctx.Detach usage (opens the durable transaction only after all
fail-fast, side-effect-free validation has already run):
  inventory, order, picking, putaway, receiving, replenishment,
  role, shipping, stockadjustment, stockcount
```

This is the exact "validate first with the caller's own cancellable
context, detach only once a transaction is genuinely about to open"
pattern Phase 8 established project-wide after a live client-disconnect
bug orphaned open transactions. `internal/order`'s three transactional
actions (`Create`, `Update`, `CreatePickingTask`, `Cancel`) all follow
it identically — confirmed by re-reading `order/service.go` fresh for
this review, not just trusting the Phase 25 write-up.

**Cross-package atomicity** (the one place this project needs a single
SQL transaction spanning two packages' own repositories): `internal/
order.CreatePickingTask` and `internal/stockcount.CreateAdjustments`
both use the identical `CreateTx(ctx, tx *gorm.DB, ...)` pattern —
a package that needs to be called from inside another package's own
transaction exposes a Tx-bound variant sharing its normal `Create`'s
core logic, rather than either duplicating that logic or attempting a
compensating/saga pattern across two separate transactions. Confirmed
both call sites open exactly one `Repository.Transaction` and pass the
same `tx` through to the far-side `CreateTx` call — a failure anywhere
in that chain rolls back everything, including the claim (see section
5 below).

**No orphaned transaction risk found** — every `Transaction(...)`
closure returns its error rather than swallowing it, and every service
constructor takes a fully-formed `*gorm.DB`/`*Repository` at startup
(no lazy/nil-checked connections created mid-request).

---

## 4. Inventory Mutation Ownership

Verified by direct grep, not by trusting each package's own doc
comments:

```
Only these 6 packages ever call AddStockTx/RemoveStockTx/MoveStockTx/
ReserveStockTx/ConsumeReservationTx/ReleaseReservationTx:
  picking, replenishment, receiving, stockcount, stockadjustment, putaway

Only internal/inventory/repository.go ever writes UPDATE/INSERT
against the `inventory` or `inventory_transactions` tables directly.

internal/order and internal/shipping do NOT import internal/inventory
at all (grepped for the import path itself — zero matches in either
package).

internal/dashboard and internal/report import internal/inventory but
only ever call its read paths (grepped for the six mutation method
names above against both packages — zero matches).
```

This is a complete, verifiable ownership boundary: exactly six
workflow packages can ever change a balance or write a ledger row, all
exclusively through `internal/inventory`'s own atomic guarded-UPDATE
primitives, and nothing else in the codebase — including the brand new
Order Management package — bypasses or duplicates that logic.

## 5. Reservation Ownership

Same six packages own reservation (`ReserveStockTx`/
`ConsumeReservationTx`/`ReleaseReservationTx`) as own mutation — there
is exactly one reservation mechanism in the codebase (Phase 12's
`ReserveRequest`/guarded UPDATE against `inventory.reserved_quantity`),
reused identically by Picking (Phase 12), Replenishment (Phase 24), and
— via `picking.Service.CreateTx`, never a second implementation — Order
Management (Phase 25). `internal/order` itself has zero reservation
calls of its own; confirmed via the same import-path grep as section 4.

**The specific atomicity property this enables** (central to Phase 25):
`order.Repository.ClaimForPickingTask` (a guarded `UPDATE orders SET
status='PICKING' WHERE status='PENDING' AND picking_task_id IS NULL`)
and `picking.Service.CreateTx`'s own reservation writes execute inside
the **same** database transaction. A picking-creation failure (e.g.
insufficient stock) rolls back both together — the order's claim is
undone along with the never-created picking task, so there is no
window where an order can be left "claimed" with no resulting
reservation, or "released" with no resulting task. This specific
guarantee is not yet confirmed live — see ORD-07 in
`UAT_CHECKLIST.md`, the single most important Phase 25 live test.

---

## 6. Idempotency Guards

Present (dedicated `*_confirmations` table, PK = idempotency key,
inserted as the first statement inside the mutating transaction) for
every workflow whose Confirm-style action can plausibly be
double-submitted by a real user (double-click, retry-after-timeout):
Receiving, Putaway, Picking, Shipping, Stock Adjustment, Stock Count,
Replenishment.

**Order Management's `CreatePickingTask` deliberately has no separate
idempotency-key table.** This was a considered design choice, not a
gap: the action is naturally at-most-once because it's gated by the
order's own status (`ClaimForPickingTask`'s guarded UPDATE — see
section 5), and `orders.picking_task_id` carries a real `UNIQUE`
constraint as the backstop. A second call — with or without a
client-retry — always finds the order no longer PENDING and returns
409, never a second task. This mirrors `internal/picking`'s own
`Cancel` action, which also has no dedicated idempotency table for the
identical reason (status-gated, naturally at-most-once). Live
confirmation: ORD-06 and ORD-12 in `UAT_CHECKLIST.md`.

---

## 7. RBAC Permissions

- Catalog (`internal/rbac/catalog.go`) is the single source of truth,
  seeded into `permissions`/`role_permissions` by `cmd/migrate` at
  startup — idempotent (insert-if-missing), confirmed by reading
  `seedPermissions`/`seedRolePermissions` in `cmd/migrate/main.go`.
- The `order` module (`view`/`create`/`update`/`approve`) was added
  this phase alongside every other module in the same catalog list —
  `ADMIN` (`allPermissionCodes()`) and `WAREHOUSE_MANAGER`
  (`allExcept("delete")`) receive it automatically through the existing
  generic computation; no operational-staff role (PICKING/SHIPPING/etc.)
  was given an explicit grant, matching the "Warehouse Setup is
  administration-tier, not floor-staff" precedent this catalog already
  established.
- `RolePermissionView.vue`'s own `MODULE_ORDER`/`MODULE_NAMES`
  constants were updated alongside the backend catalog — without this,
  new permission codes exist in the database but are silently filtered
  out of the Roles & Permissions screen's rendering. Confirmed present
  in the current source.
- **Documented tradeoff, not a defect**: permissions are embedded in
  the JWT at login time (`internal/auth`), not re-queried from SQL
  Server on every request (`internal/platform/middleware/permission.go`'s
  own doc comment). A role/permission change — including the new
  `order.*` grants themselves — only takes effect for a user the next
  time they log in. Anyone live-testing RBAC (AUTH-06 through AUTH-10)
  must obtain a **fresh token after** `cmd/migrate seed` has run with
  the current catalog, not reuse an older session.
- **Action item**: none of this has been confirmed against a live
  `permissions` table yet — see AUTH-07/08/09/10 in
  `UAT_CHECKLIST.md`.

---

## 8. JWT / Security Configuration

- Signing: HMAC-SHA256 only, explicit algorithm allow-list
  (`jwt.WithValidMethods([]string{"HS256"})`) in `RequireJWT` — prevents
  the classic "alg: none" / algorithm-confusion class of JWT
  vulnerability.
- `JWT_SECRET` is a required environment variable (`config.Load` fails
  fast if missing) — never hardcoded, never defaulted.
- Passwords: bcrypt, confirmed via direct grep (`internal/auth/service.go`,
  `internal/user/service.go`, both with matching `_test.go` coverage) —
  not just claimed in CLAUDE.md, actually implemented.
- No password, JWT secret, or DB/RabbitMQ credential appears in any
  `slog.*` call anywhere in the codebase (grepped for
  password/secret/token adjacent to logging calls — zero matches).
  `RequestLogging` middleware logs only method/path/status/duration/
  request_id, never the request body (so a login attempt's password
  is never logged even accidentally).
- **Action item (production hardening, not a code defect)**: `.env`/
  `.env.example` both use obvious `ChangeMe_*` placeholder values for
  `DB_PASSWORD`, `SEED_ADMIN_PASSWORD`, `RABBITMQ_PASSWORD`, and
  `JWT_SECRET` is `change-me-to-a-long-random-string`. These are
  correctly excluded from version control (see section 15) and are
  fine for local development, but **must be rotated to real generated
  secrets before any non-development deployment** — this is expected
  operator action, not something the codebase can self-enforce.

---

## 9. CORS

`internal/platform/middleware/cors.go`: defaults to `AllowAllOrigins:
true` when `CORS_ALLOWED_ORIGINS` is unset or `"*"` (the shipped
default in both `.env.example` files) — deliberate, documented as a
development convenience since production serves the frontend
same-origin through Nginx (`/api` reverse-proxied to the backend, per
CLAUDE.md's own architecture). `AllowCredentials: false` is set
correctly alongside the wildcard (browsers reject
`AllowCredentials: true` + wildcard origin combined, and this app
doesn't use cookies for auth anyway — the JWT travels in an
`Authorization` header, not a cookie).

**Action item**: set `CORS_ALLOWED_ORIGINS` to the real production
origin(s) explicitly once a production domain exists — currently
correct for development, not yet configured for a specific production
deployment (there isn't one yet).

---

## 10. Swagger Exposure

- Gated by `SWAGGER_ENABLED` (defaults `true`) — `cmd/server/main.go`
  only registers `/swagger/*any` when the flag is set.
- Full annotation coverage confirmed for every endpoint including all 6
  new Order Management routes (regenerated via `swag init` this phase,
  see Phase 25's own write-up — `docs/swagger.json` contains all 4
  distinct `/orders...` path templates).
- Security definition (`BearerAuth`) is declared and every protected
  endpoint's annotations include `@Security BearerAuth` — Swagger UI's
  own "Authorize" button works against real protected endpoints, no
  hardcoded token in the spec.
- **Action item**: `SWAGGER_ENABLED=true` is fine for a demo/staging
  environment but should be set to `false` for a real production
  deployment (API documentation/schema shouldn't be publicly
  browsable in prod) — the flag already exists to do this; it's an
  environment-configuration decision, not a code change.

---

## 11. File / Export Authorization

Reviewed `internal/report/service.go`'s `ResolveDownload` and
`ProcessJob`:

- Every export job lookup (`GetJob`, `ResolveDownload`) checks
  `job.RequestedBy != userID` and returns the same `ErrNotFound` a
  nonexistent job would — no existence leak, no cross-user download.
- The on-disk filename is **always server-generated**
  (`csvexport.SafeFilename(fmt.Sprintf("inventory_transactions_%d_%s.csv",
  jobID, timestamp))`) — the client never supplies a filename or path
  component, so there is no path-traversal surface (`filepath.Join`
  is combining two trusted, server-controlled strings, not user input).
- `EXPORT_DIR` is shared via a named Docker volume between `backend`
  (serves downloads) and `worker` (writes files) — no cross-container
  file transfer step to get wrong.

No issues found.

---

## 12. Environment Variables

`config.Load()` fails fast (returns an error before the server even
starts listening) if any of `APP_ENV`, `APP_PORT`, `DB_HOST`,
`DB_PORT`, `DB_USER`, `DB_PASSWORD`, `DB_NAME`, `JWT_SECRET`,
`RABBITMQ_HOST`, `RABBITMQ_PORT`, `RABBITMQ_USER`,
`RABBITMQ_PASSWORD`, or `RABBITMQ_VHOST` is missing/empty — a
misconfigured deployment fails immediately and loudly at startup
rather than serving requests in a broken state. Every optional/
tunable value (`DB_ENCRYPT`, `JWT_EXPIRES_IN`,
`DB_TRANSACTION_TIMEOUT`, the Stock Adjustment reminder knobs,
`EXPORT_DIR`, `SWAGGER_ENABLED`, `CORS_ALLOWED_ORIGINS`) has a sane,
documented fallback. Both `.env.example` files (`backend/.env.example`
and the root one used by `docker compose`) stay in sync with what
`config.Load` actually reads — spot-checked, no drift found.

---

## 13. Secrets Handling

- Root `.gitignore` excludes `.env`/`.env.*` while explicitly
  allowlisting `.env.example`/`backend/.env.example` — correct pattern,
  already in place before this review.
- `backend/.env` and root `.env` both exist locally with the same
  `ChangeMe_*`-style placeholder values as their `.example` siblings —
  confirmed no real-looking secret was accidentally committed as a
  "default." (Note: this project has no git repository initialized yet
  — `git status` is not applicable — so there is currently no risk of
  these being pushed anywhere; this becomes relevant the moment `git
  init` + a remote are set up, at which point the existing `.gitignore`
  already protects against it.)
- No secret is baked into any Docker image layer — `docker-compose.yml`
  injects everything via `env_file: .env` at container-start time, and
  the Dockerfile's build stage never receives credentials as build
  args.

---

## 14. Docker Compose Configuration

`docker compose config` was run this phase (see section 18) and
resolves cleanly with no errors. Reviewed the full file:

- Health checks present for `sqlserver` (real `sqlcmd SELECT 1`, not
  just "port open") and `rabbitmq` (`rabbitmq-diagnostics ping`) — both
  with reasonable `start_period`/`retries` for a cold start.
- `migrate` is a genuine one-shot job (`restart: "no"`,
  `depends_on: sqlserver: condition: service_healthy`) that must
  complete successfully before `backend`/`worker` start
  (`condition: service_completed_successfully`) — correct ordering,
  matches CLAUDE.md's "backend should not assume dependencies are
  ready" instruction.
- Named volumes (`sqlserver_data`, `rabbitmq_data`, `export_files`) for
  every piece of state that must survive a container restart —
  matches CLAUDE.md's persistence requirement.
- `worker` has no exposed ports (correct — it's a pure consumer, never
  receives inbound HTTP).

**Recommendations for a real production deployment** (not required for
this project's current scope, noted for completeness):
- No `mem_limit`/`cpus` resource constraints are set on any service.
  Given this development machine's own tight RAM (~8GB total, often
  under 2GB free), an unconstrained `sqlserver` container in
  particular can starve the host — worth adding conservative limits if
  this ever runs somewhere shared/constrained long-term.
- `sqlserver`'s port 1433 and `rabbitmq`'s AMQP/management ports are
  published to the host by default — fine for local development,
  should be removed/firewalled for a real production deployment where
  only `backend`/`worker` need to reach them (over the Compose-internal
  network, which already works without the host port mapping).
- Neither the `server`/`worker`/`migrate` Docker images declare a
  non-root `USER` — they currently run as root inside their
  containers. Not a functional problem for this project's scope, but a
  standard hardening step (`USER nonroot` or equivalent) worth doing
  before a real production rollout.

---

## 15. Health Endpoints

`GET /api/v1/health` (`internal/health`): pings SQL Server for real
(`db.Ping(ctx)`), reads RabbitMQ's cached live-connection state (no
extra round trip needed — the connection manager already tracks it via
`NotifyClose` + its reconnect loop), returns `status: "ok"` only when
both are connected, `"degraded"` otherwise. Response shape
(`{status, database, rabbitmq}`) never includes a connection string,
hostname, port, or credential — matches CLAUDE.md's "do not expose
credentials or internal connection strings" instruction exactly. The
backend's own Docker `HEALTHCHECK` directive calls this same endpoint.

No issues found.

---

## 16. Logging / Error Handling

- Structured logging via `log/slog` throughout, with `request_id`
  attached by `RequestID` middleware and threaded through every
  handler → service → repository call via `dbctx.WithRequestID`.
- Every durable transaction logs its own outcome via
  `dbctx.LogTransaction(ctx, "<package>.<action>", start, err)` —
  confirmed present on all of Order Management's four transactional
  actions (`order.create`, `order.update`,
  `order.create_picking_task`, `order.cancel`), matching every other
  workflow package's own naming convention exactly.
- Error responses never leak internal detail: every handler's
  `writeError`/`writeError`-equivalent switches on a known, typed set
  of sentinel errors (`errors.Is`/`errors.As`) and returns a fixed,
  safe message + error code; anything unrecognized falls through to a
  generic `500 INTERNAL_ERROR` rather than the raw Go error string
  (which could otherwise leak a SQL fragment or file path).
- No panic-recovery gap found: `middleware.Recovery(logger)` is
  registered first in the middleware chain in `cmd/server/main.go`, so
  an unexpected panic in any handler is caught, logged, and turned into
  a 500 rather than crashing the whole process.

No issues found.

---

## 17. Phase 25 (Order Management) — Live Tests Still Required

Everything below is implemented and passed every **offline** check this
project has (unit tests, `go vet`, `go build`, `gofmt`), but has never
run against a real SQL Server. In priority order:

1. **ORD-07 (insufficient-stock rollback)** — the single most important
   test. Confirms `ClaimForPickingTask` + `picking.Service.CreateTx`
   sharing one transaction actually rolls back together, leaving no
   half-claimed order.
2. **ORD-05/ORD-06/ORD-12** — the success path, the duplicate-release
   409, and (if practical) genuine concurrent requests — confirms the
   atomic claim guard is both correct and race-safe under real
   concurrent load, not just correct in the single-threaded case `go
   test` can exercise.
3. **AUTH-07 through AUTH-10** — confirms `cmd/migrate seed` actually
   inserts the four `order.*` permission rows and that ADMIN/
   WAREHOUSE_MANAGER/an unauthorized role each get the expected
   403-or-not outcome, with a **freshly issued token** (see section 7's
   JWT-embeds-permissions tradeoff).
4. **ORD-01 through ORD-04** — basic CRUD/validation correctness
   against a real SQL Server (server-generated `order_no`, the
   `external_reference` UNIQUE constraint's actual behavior, full-
   replace `Update` semantics).
5. **ORD-08/ORD-09** — fulfillment-status derivation matches real
   Picking/Shipping state, via both the single-order (service-call) and
   list (SQL-join) code paths, and that they never disagree with each
   other.
6. **ORD-10/ORD-11** — cancellation rules.
7. **ORD-13** — the frontend, end to end, in a real browser.
8. **Swagger UI** — `/swagger/index.html` reachable, `Authorize` flow
   works, and a real call can be made through the UI for at least one
   Order endpoint.

Once all of the above pass, Phase 25 may be marked `VERIFIED COMPLETE`
— not before.

---

## 18. Offline Checks Run This Phase

```
go fmt ./...      → clean, no files needed reformatting
go vet ./...      → clean
go test ./...     → all packages pass (including internal/order,
                     internal/picking's new CreateTx test)
go build ./...    → clean
npm run build     → clean (fontend/)
docker compose config → resolves cleanly, no errors
```

Exact output captured in this session's tool log; summarized results
also recorded in `PROJECT_STATUS.md`'s Phase 26 section.

---

## Exact Live Test Sequence (run once Docker is restored)

Minimum-footprint order, matching the resource-safety constraints this
project has been operating under (limited host RAM, no full-stack
rebuild, never remove volumes):

1. `docker info` / `docker ps -a` / `docker compose ps` — confirm the
   daemon is actually reachable before touching anything (this has
   repeatedly been the actual blocker, not application code).
2. Check host free memory. If dangerously low, stop before starting
   any container.
3. `docker compose up -d sqlserver rabbitmq` — infrastructure only.
   Wait for both to report `healthy` in `docker compose ps` (do **not**
   proceed on `starting`).
4. Run the project's existing migrate/seed step (`docker compose run
   --rm migrate` or equivalent — do not invent a new process; the
   `migrate` service in `docker-compose.yml` already runs `wms-migrate
   up && wms-migrate seed` as its command). Confirm in its logs:
   migration 0038 applied, permission count now 55 (was 51 as of Phase
   24), `order` module rows present.
5. `docker compose up -d --build backend frontend` (targeted rebuild —
   `worker` does not need rebuilding for Phase 25 since Order
   Management has no RabbitMQ component; only rebuild it if a
   regression check specifically needs it running).
6. `GET /api/v1/health` → confirm `database: connected`,
   `rabbitmq: connected`.
7. Log in as `admin` (fresh token, issued after step 4's seed).
8. Work through `UAT_CHECKLIST.md` sections 1 and 15 first (RBAC, then
   Order Management) — these are Phase 25's own scope. Then sections
   2–14 and 16 as the Phase 1–24 regression pass.
9. Verify the frontend in a real browser (ORD-13 plus a spot-check of
   at least Picking and Shipping, since those are Order Management's
   direct integration points).
10. Record every result directly in `UAT_CHECKLIST.md` (flip PENDING →
    PASS/FAIL, fill in Actual result).
11. If everything in section 15/17's Phase 25 list passes, update
    `PROJECT_STATUS.md`'s Phase 25 heading to `VERIFIED COMPLETE` and
    record the regression summary. Do not mark Phase 26 (UAT/
    Production Readiness) itself `VERIFIED COMPLETE` from this pass
    alone — that requires the **entire** `UAT_CHECKLIST.md` matrix to
    be run, not just Phase 25's own subset.
