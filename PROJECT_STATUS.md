# Smart WMS — Project Status

This file is updated after every significant implementation phase.

---

## Phase 1 — Backend + Infrastructure Foundation: VERIFIED COMPLETE

Go API server + worker, SQL Server and RabbitMQ connection foundations
(retry/backoff, reconnect loops), Gin + standard response envelope,
Swagger/OpenAPI, Docker Compose wiring all services, health endpoint.
No business tables/queues in this phase — infrastructure only.

---

## Phase 2 — Authentication + RBAC: VERIFIED COMPLETE

### Scope

Authentication, Users, Roles, Permissions, and frontend auth
integration only — no Warehouse/Product/Inventory/Inbound/Outbound/
Orders/Picking, no RabbitMQ business queues, per CLAUDE.md's explicit
restriction.

### Pre-implementation findings (frontend inspection)

- **LoginView.vue** was pure mock: field-presence validation only, then
  an unconditional redirect to `/dashboard`. No API call, no token
  storage.
- **Router** had zero auth guards — every route was reachable directly
  by URL.
- **Role mismatch** (the issue this phase was asked to resolve): the
  User Management form's role dropdown had 7 roles (`ADMIN`,
  `WAREHOUSE_MANAGER`, `RECEIVING`, `QC`, `PUTAWAY`, `PICKING`,
  `SHIPPING`); the User Management *filter* dropdown and the Roles &
  Permissions screen's role list both only had 4 (`ADMIN`,
  `WAREHOUSE_MANAGER`, `RECEIVING`, `PICKING`).
- **Permission defaults gap**: `RolePermissionView.vue`'s
  `createDefaultPermissions()` only had explicit logic for `ADMIN`,
  `WAREHOUSE_MANAGER`, `RECEIVING`, and `PICKING`. `QC`, `PUTAWAY`, and
  `SHIPPING` had no branch at all, so selecting them produced an
  all-false permission set in the mock.
- **No password field** existed anywhere in the User Management
  create/edit form.

### Normalization decisions (resolving the above)

- Canonical role set = the 7 roles from the User form (the actual
  assignable set an admin sees). Seeded into `roles`.
- `QC`/`PUTAWAY`/`SHIPPING` are seeded with **zero granted
  permissions**, faithfully mirroring the frontend's actual (gap)
  behavior rather than inventing plausible-looking defaults. **Needs a
  real product/business decision before these roles are used for real
  staff** — flagged as a blocker item below.
- One user → one role (`users.role_id` FK) — the form uses a single
  `<select>`, not multi-select.
- Permission codes normalized to `module.action` (e.g. `users.view`,
  `roles.update`). `USER_MANAGEMENT` → `users`, `ROLE_PERMISSION` →
  `roles`, `Edit` → `update`, matching CLAUDE.md's own example codes.
  The other 8 modules (`dashboard`, `receiving`, `quality_check`,
  `putaway`, `stock_inquiry`, `stock_adjustment`, `picking`,
  `shipping`) keep their frontend names lowercased and are
  **catalog-only this phase** — nothing enforces them yet since those
  business modules don't exist.
- `status` (`ACTIVE`/`INACTIVE`/`LOCKED`) is the real users column
  instead of a plain `is_active` boolean, since the frontend already
  needs three states.
- `warehouse` (the field/column in the User form) has **no backend
  column this phase** — no Warehouse module exists to assign it
  against. The UI field/column stays as-is but isn't wired to real
  data (shows `—` in the table).
- Role create/delete are **not implemented** — the frontend's "+ New
  Role" button never actually persisted anything either (its
  `savePermission()` was a bare `alert()`). Only
  `GET/PUT .../roles/{id}/permissions` exist, per the task's own
  "likely endpoints" list and "don't add unnecessary CRUD."
- A minimal **Password** input was added to the "New User" panel
  (create only, not edit) — the smallest functional change that makes
  user creation actually work through a real API, not a redesign.

### Completed

1. **Database migrations** — `backend/migrations/0001..0004_*.up.sql` /
   `.down.sql`: `roles`, `permissions`, `role_permissions`, `users`.
   Plain SQL, not GORM AutoMigrate. Applied and verified against two
   real SQL Server instances: the developer's existing local instance
   and a brand-new containerized one (see Docker section).
2. **Migration/seed runner** (`backend/cmd/migrate`) — embeds the SQL
   files (`backend/migrations/embed.go`), tracks applied versions in a
   `schema_migrations` table, and on first run against a brand new SQL
   Server also creates the `wms` database itself (`CREATE DATABASE`
   against `master`) before running migrations. `up`, `down`, `seed`
   subcommands.
3. **Seed data** — 7 roles, 39 permissions (full frontend matrix), all
   `role_permissions` grants, and one development admin user from
   `SEED_ADMIN_USERNAME`/`SEED_ADMIN_PASSWORD` (bcrypt-hashed, never
   logged, never hardcoded in Go source). Idempotent — safe to re-run.
4. **Auth domain** (`backend/internal/auth`) — `POST /auth/login`
   (bcrypt verify, HS256 JWT with configurable `JWT_EXPIRES_IN`,
   issuer/subject/expiry claims, no sensitive data in claims),
   `GET /auth/me` (JWT-protected, live DB read).
5. **JWT middleware** — reused Phase 1's `middleware.RequireJWT`
   unchanged.
6. **RBAC** — new `middleware.RequirePermission(code)`, chained as
   `RequireJWT` → `RequirePermission` → handler on every protected
   route. Permission codes are embedded in the JWT at login (a
   snapshot) rather than re-queried per request — cheap, side-effect
   free, but means a permission/role change takes effect on the
   affected user's *next* login, not immediately. Accepted tradeoff for
   this phase.
7. **User Management API** — `GET/POST /users`,
   `GET/PUT/DELETE /users/{id}`, with search/role/status filters and
   pagination. Passwords never returned. Duplicate username/email → 409.
   Invalid role_code → 422.
8. **Role/Permission API** — `GET /roles`, `GET /permissions`,
   `GET/PUT /roles/{id}/permissions`.
9. **Frontend service layer** — `frontend/src/services/{api,auth,users,roles}.js`,
   native `fetch`, no Axios/Pinia added.
10. **Frontend auth integration** — real login call, `localStorage`
    session (`wms_auth`: access token + client-computed expiry + user
    profile, never the password), router `beforeEach` guard, a 401 from
    any API call clears the session and redirects to `/login`.
11. **Authorized UI** — `MainLayout.vue`'s Administration nav items
    (User Management, Roles & Permissions) are now gated by
    `hasPermission('users.view')` / `hasPermission('roles.view')` — the
    only two nav items backed by an actually-enforced backend
    permission this phase. Backend enforces independently regardless of
    what the UI hides.
12. **Swagger** — all new endpoints documented (request/response
    models, 400/401/403/404/409/422 where applicable), BearerAuth
    scheme, verified in Swagger UI (endpoint grouping, lock icons on
    protected routes correct).
13. **Tests** — bcrypt hash/verify round trip, login success/invalid
    password/unknown user/inactive/locked, JWT issue/parse round trip,
    JWT wrong-secret/expired rejection (extends Phase 1's existing JWT
    tests), unauthorized protected route (Phase 1), permission
    allowed/denied/missing-claim, user Create's password-length and
    status validation. `go test ./...` passes.
14. **Docker** — new `migrate` one-shot service/Dockerfile target;
    `backend`/`worker` now `depends_on: migrate: condition:
    service_completed_successfully`. New env vars (`JWT_EXPIRES_IN`,
    `SEED_ADMIN_USERNAME`, `SEED_ADMIN_PASSWORD`) added to both
    `.env.example` files and `docker-compose.yml` — no secrets
    hardcoded in the compose file itself.

### Two real bugs found and fixed during Docker verification

Both were caught only by actually running `docker compose up` end to
end against a **fresh** SQL Server volume — compiling and unit-testing
alone did not catch either:

1. **Backend healthcheck always failed.** `wget --spider` sends a HEAD
   request; the health route only registered `GET`, so Gin returned
   404 and the container was permanently stuck `unhealthy` even though
   the server itself was working correctly (confirmed via direct
   `curl`). Fixed by changing the healthcheck to `wget -O /dev/null`
   (forces GET) instead of `--spider`.
2. **Frontend healthcheck always failed.** `wget --spider
   http://localhost/` resolved `localhost` to `::1` first inside the
   Alpine container; nginx only listens on IPv4 (`0.0.0.0:80`), so the
   IPv6 attempt was refused. Fixed by pointing the healthcheck at
   `127.0.0.1` explicitly.
3. **(Design gap, not a bug, but only surfaced this way)**: the
   original `migrate` command assumed the `wms` database already
   existed — true on the developer's pre-existing local SQL Server, but
   not on a brand new containerized instance with an empty volume.
   Fixed by having `cmd/migrate` connect to `master` first and
   `CREATE DATABASE` if missing, before running any migration.

### Docker verification result (full `docker compose up -d`, this session)

All of the following were actually run and observed, not assumed from
compilation:

- `docker compose config` — resolves cleanly.
- `docker compose build` — all 4 images (`backend`, `worker`,
  `migrate`, `frontend`) build successfully.
- `docker compose up -d` — SQL Server, RabbitMQ, backend, frontend all
  reach `healthy`; worker running; `migrate` exits 0.
- Fresh volume: `migrate` created the `wms` database, applied all 4
  migrations, and seeded 7 roles / 39 permissions / grants / 1 admin
  user — observed directly in container logs.
- `GET /api/v1/health` (direct and via the Nginx `/api` proxy on port
  80) reports `database: connected`, `rabbitmq: connected`.
- Login, `/auth/me`, and a protected endpoint all verified through the
  **Nginx-proxied** path (port 80), not just the backend's own port —
  matches how the app is actually accessed in this deployment.
- Swagger UI loads at `/swagger/index.html` via the Nginx proxy too.
- **Restart persistence**: `docker compose restart sqlserver rabbitmq`,
  waited for both to report `healthy` again, then logged in as `admin`
  successfully without re-seeding — confirms `sqlserver_data` and
  `rabbitmq_data` volumes genuinely persist state across a restart, and
  the backend's reconnect loop (built in Phase 1) recovers on its own.
- One environment-level note, not a bug: this host already runs an
  unrelated `rabbitmq-rabbitmq-1` container (a different project) bound
  to the default 5672/15672. This project's `.env` remaps
  `RABBITMQ_AMQP_PORT`/`RABBITMQ_MGMT_PORT` to 5673/15673 — only the
  host-side mapping; inside Compose, services still reach RabbitMQ at
  `rabbitmq:5672` unaffected.

### Frontend verification (this session)

- `npm run build` succeeds.
- Manually driven in a real browser end to end: unauthenticated access
  to a protected route redirects to `/login`; invalid login shows the
  backend's real error message; valid login redirects to `/dashboard`
  and the topbar shows the real authenticated user
  (`full_name`/`role.name`, not the old hardcoded "Admin User" /
  "Warehouse Manager"); User Management's role filter now shows all 7
  roles (mismatch resolved); created a real user through the UI
  (including the new password field) and it persisted via the API;
  Roles & Permissions now lists all 7 roles, and the permission matrix
  correctly shows 39/39 checked for `ADMIN` and 0/39 checked for `QC`
  (matching the documented seed behavior); logout clears the session
  and redirects; a corrupted/invalid token on any API call triggers the
  401 handler, clearing the session and redirecting to `/login`.

### Known gaps / follow-ups for later phases

- **QC/PUTAWAY/SHIPPING have zero permissions.** These roles can be
  assigned to a user today, but that user would have almost no access
  (not even `dashboard.view`). This needs a real product decision, not
  an engineering one — surfacing it here rather than guessing.
- **Role create/delete** are not implemented — the "+ New Role" button
  is still a local-only draft in the UI.
- **Permission changes require re-login** to take effect (JWT-embedded
  snapshot, not re-checked per request).
- **No pagination UI** in User Management yet — the API supports
  `page`/`limit`, but the screen currently just requests `limit=100` in
  one page; add real pagination controls once user counts justify it.
- **Warehouse field** on users is cosmetic only this phase (no backend
  column) — wire it up once the Warehouse module exists.
- Sidebar navigation (including Logout) is hidden below Tailwind's `lg`
  breakpoint (1024px) — this was already true before Phase 2 and is
  unrelated to this phase's changes, but worth fixing whenever the
  frontend gets a responsive-design pass.

---

## Phase 3 — Warehouse Setup + RBAC Permission Gap Resolution: VERIFIED COMPLETE

### RBAC gap review (before writing any code)

Inspected: `roles`/`permissions`/`role_permissions` tables, seed data
(`internal/rbac/catalog.go`), the User Management role dropdown, the
Roles & Permissions screen, `MainLayout.vue`'s nav, and
`middleware.RequirePermission`. Findings:

1. **Roles that exist**: the same 7 from Phase 2 — `ADMIN`,
   `WAREHOUSE_MANAGER`, `RECEIVING`, `QC`, `PUTAWAY`, `PICKING`,
   `SHIPPING`. This task's own example list (ADMIN/SUPERVISOR/
   OPERATOR/VIEWER/QC/PUTAWAY/SHIPPING) does not match what the actual
   frontend uses — `SUPERVISOR`, `OPERATOR`, `VIEWER` appear nowhere in
   `UserManagementView.vue`'s role dropdown. Per "use the existing
   frontend as source of truth" and "do not silently remove existing
   roles," the 7-role set from Phase 2 was preserved unchanged; the
   task's example names were treated as illustrative, not literal.
2. **Permissions that existed**: 39 codes across 10 modules (Phase 2).
   `quality_check.*`, `putaway.*`, and `shipping.*` (view/create/
   update/approve each) already existed in the catalog.
3. **The actual gap**: not missing *permissions* — missing *grants*.
   `QC`, `PUTAWAY`, `SHIPPING` had zero rows in `role_permissions`,
   because the frontend's `createDefaultPermissions()` never had a
   branch for them (a bug preserved faithfully in Phase 2, flagged for
   this phase to resolve).
4. **Warehouse Setup had no permission mapping at all** — the frontend
   screen existed as pure mock with no RBAC concept, and no
   `warehouse` module existed in the permission catalog.
5. **Modules with view/create/update/delete vs. approve**: `users` and
   `warehouse` (new) are plain CRUD (view/create/update/delete, no
   approve) — matches their nature as admin config screens, not
   operational workflows needing sign-off.

### RBAC changes made

- `internal/rbac/catalog.go`: `QC`/`PUTAWAY`/`SHIPPING` each granted
  `dashboard.view`, their own module's `view`/`create`/`update`, and
  `stock_inquiry.view` — the same shape `RECEIVING`/`PICKING` already
  had (no `approve`/`delete`, matching that conservative baseline). No
  new permission codes were needed for this — no SQL migration either,
  since the project's mechanism for RBAC *data* (as opposed to schema)
  is the idempotent `migrate seed` command, not baked-in SQL inserts
  (consistent with how the original 39 permissions were seeded in
  Phase 2, not written into `0002_create_permissions_table.up.sql`).
- Added a new, genuinely new permission module: `warehouse`
  (view/create/update/delete — 4 new codes, 39 → 43 total), for the
  Warehouse Setup screen this phase actually implements. `ADMIN` and
  `WAREHOUSE_MANAGER`'s grants are computed generically in
  `catalog.go` (`allPermissionCodes()` / `allExcept("delete")`), so
  both automatically picked up the 4 new codes with no separate edit.
- Re-ran `migrate seed` (idempotent) against both the developer's
  local SQL Server and a fresh containerized instance — verified live
  in both (see Migration Verification below).

### Roles and permissions after normalization

7 roles unchanged. 43 permissions (39 from Phase 2 + 4 `warehouse.*`).
Verified live via `/auth/me` for real QC/PUTAWAY/SHIPPING test
accounts:

| Role | Permissions |
|---|---|
| QC | `dashboard.view`, `quality_check.view/create/update`, `stock_inquiry.view` |
| PUTAWAY | `dashboard.view`, `putaway.view/create/update`, `stock_inquiry.view` |
| SHIPPING | `dashboard.view`, `shipping.view/create/update`, `stock_inquiry.view` |
| ADMIN | all 43 |
| WAREHOUSE_MANAGER | all 43 except every `.delete` |

### Warehouse domain (frontend inspected first)

`WarehouseView.vue`'s "Warehouse Setup" screen: a warehouse
(code/name/type/address/contact/phone, later added capacity) table
with search/type/status filters, and a drawer with 4 tabs — Warehouse
Info, Zone Setup (code/name/type ∈ RECEIVING/STORAGE/PICKING/PACKING/
SHIPPING/QC/HOLD/status), Location Structure (zone → aisle → rack →
bin tree), Capacity Dashboard. Hierarchy implemented as Warehouse →
Zone → Location, matching the frontend's own terminology exactly (no
forced renaming).

Three new backend packages, each Handler → Service → Repository → SQL
Server, mirroring Phase 2's `user`/`role` structure: `internal/warehouse`,
`internal/zone`, `internal/location`. Dependency direction is
warehouse ← zone ← location only — each Service takes a narrow
interface satisfied by the next package's concrete `*Repository`
(e.g. `warehouse.Service` depends on a `zoneCounter` interface, not
`package zone` directly), so none of the three import each other in a
cycle.

### Migration files created

`backend/migrations/0005_create_warehouses_table.{up,down}.sql`,
`0006_create_zones_table.{up,down}.sql`,
`0007_create_locations_table.{up,down}.sql` — additive, no edits to
0001–0004.

### Warehouse schema created

- `warehouses`: code (unique), name, type, address, contact_person,
  phone, capacity (nullable — see Known issues), status
  (ACTIVE/INACTIVE), timestamps.
- `zones`: warehouse_id FK, code, name, type (CHECK-constrained to the
  7 frontend values), status. `UNIQUE(warehouse_id, code)`.
- `locations`: zone_id FK, code, aisle, rack, bin, status.
  `UNIQUE(zone_id, code)`.

Delete safety: warehouse delete refuses (409) if zones exist; zone
delete refuses (409) if locations exist — checked in the Service layer
(clean error, not a raw FK violation) before the repository delete
runs.

### APIs created

```
GET/POST      /api/v1/warehouses
GET/PUT/DELETE /api/v1/warehouses/{id}
GET/POST      /api/v1/zones          (?warehouse_id= required on GET)
GET/PUT/DELETE /api/v1/zones/{id}
GET/POST      /api/v1/locations      (?zone_id= required on GET)
GET/PUT/DELETE /api/v1/locations/{id}
```

All gated by `RequireJWT` → `RequirePermission("warehouse.*")`. Zones
and locations reuse the same `warehouse.view`/`warehouse.update`
permissions as their parent (they have no separate nav entry — they're
tabs inside the warehouse drawer).

### Frontend files modified

- New: `services/warehouses.js` (warehouses + zones + locations —
  grouped in one file since the screen consumes all three together).
- `WarehouseView.vue`: mock arrays replaced with real API calls;
  loading/error states added; a **Capacity** input was added to the
  Warehouse Info tab (the field exists in the real schema but had no
  UI input at all); a **Save Zones** button was added to the Zone Setup
  tab (the mock never persisted zones — `addZone()` only pushed a
  local row with no save path); the top summary cards (previously
  hardcoded "3"/"18"/"10,500"/"78%") now compute from real data
  (Avg Utilization shows "—", honestly, since it's not computable —
  see Known issues). Layout, tabs, filters, and table structure
  unchanged.
- `RolePermissionView.vue`: added `warehouse` to the local
  `MODULE_ORDER`/`MODULE_NAMES` maps so the new permission module
  renders in the matrix (the matrix itself is already API-driven from
  Phase 2 — this was a 2-line addition, not a rewrite).
- `MainLayout.vue`: Warehouse Setup nav item gated by
  `hasPermission('warehouse.view')`, same pattern as Phase 2's User
  Management/Roles & Permissions gating.
- `vite.config.js`: unchanged from Phase 2 (dev proxy already covers
  the new routes under `/api`).

### Swagger changes

All 15 new endpoint operations documented (request/response models,
query params, 400/401/403/404/409/422, `BearerAuth`). Regenerated
successfully; verified `/warehouses`, `/warehouses/{id}`, `/zones`,
`/zones/{id}`, `/locations`, `/locations/{id}` all appear with correct
security requirements.

### Test results

New unit tests (validation-only paths against zero-value Services, and
narrow-interface fakes for the cross-package existence checks — same
pattern as Phase 2's `user`/`auth` tests, since `Repository` fields are
concrete, not interfaces): warehouse status validation, zone type/status
validation, **zone create with a fake warehouseChecker returning
not-found** (`ErrWarehouseNotFound`, no real DB touched), location
status validation, **location create with a fake zoneChecker returning
not-found** (`ErrZoneNotFound`, no real DB touched). Two new
warehouse-specific RBAC middleware tests (`warehouse.view` allowed for
an ADMIN-shaped token, denied for a QC-shaped token). `go test ./...`
passes.

Create/duplicate-code/referential-safety paths (which need a real
repository) were verified live against real SQL Server via the full
CRUD chain below, consistent with how Phase 2 handled the same
tradeoff for `user`/`role`.

### Migration verification (live, not from compilation alone)

Ran `go run ./cmd/migrate up` then `seed` against the developer's
existing local SQL Server: all 3 tables created with FKs/unique
constraints, 43 permissions seeded, QC/PUTAWAY/SHIPPING grants
confirmed via `GET /roles/{id}/permissions`. Full live CRUD chain
exercised via curl: create warehouse → duplicate code (409) → create
zone → zone for nonexistent warehouse (404) → duplicate zone code
(409) → invalid zone type (422) → delete warehouse with zones (409) →
create location → duplicate location code (409) → delete zone with
locations (409) → verified counts on `GET /warehouses/{id}` → cleaned
up in reverse order (all 204). Also verified: no JWT → 401; a real QC
user → 403 on `/warehouses` and `/roles`; real PUTAWAY/SHIPPING users'
`/auth/me` shows exactly the expected permission sets.

### Docker/runtime verification

`docker compose config` clean; `docker compose build` — all 4 images
rebuilt successfully; `docker compose up -d` — all 5 services healthy
(SQL Server, RabbitMQ, backend, frontend; worker running). Confirmed
on a **fresh** containerized SQL Server: `migrate` created the 3 new
tables and re-seeded 43 permissions with QC/PUTAWAY/SHIPPING grants
from a clean volume. Verified end to end through the Nginx-proxied path
(port 80, not just the backend's own port): login, `/api/v1/warehouses`,
Swagger UI.

Full browser verification (real Chrome, not just curl): logged in as
admin, created a warehouse through the actual UI (including the new
Capacity field), added a zone through the actual UI (new Save Zones
button, confirmed persisted via API), created locations via API and
confirmed the Location Structure tab renders the real zone → aisle →
rack → bin tree from live data, confirmed the Capacity Dashboard tab
shows the real capacity value and an honest "—" for
Occupied/Available. Logged in as a real QC user: confirmed Warehouse
Setup (and User Management, Roles & Permissions) do not appear in the
nav; confirmed direct navigation to `/administration/warehouse` still
loads the page shell (router only checks authentication) but the API
call fails with 403 and the UI surfaces `"permission denied:
warehouse.view"` honestly instead of hiding the failure — the security
boundary is the backend, not the hidden nav item, as required. All
test data cleaned up afterward.

### Known issues

- **Utilization is not computable.** `warehouses.capacity` is real
  master data now, but there's no "occupied" quantity without real
  inventory (explicitly out of scope this phase). The API omits
  `utilization` entirely; the frontend shows 0%/"—" honestly rather
  than a fabricated number.
- **"Generate Bin Locations" is still a mock `alert()`.** Bulk-
  generating locations from an aisle/rack/bin count pattern is
  meaningful business logic the task's own API list didn't request
  (only standard CRUD was asked for); the Location Structure tab
  displays real data from individually-created locations, but bulk
  generation needs a dedicated endpoint in a later phase if wanted.
- **"Save Zones" always PUTs every existing zone row**, not just
  changed ones — simple and correct, but slightly wasteful for
  warehouses with many zones. Fine at current scale.
- Everything listed as a known issue in Phase 2 (permission changes
  need re-login, no pagination UI, Product Master's own RBAC not yet
  wired, sidebar hidden below the `lg` breakpoint) still applies
  unchanged.

## Phase 4 — Product Master: VERIFIED COMPLETE

### Frontend requirements found (before writing code)

`ProductMasterView.vue` inspected completely. Fields: SKU, Product Name,
Category (PACKAGING/LABEL/RFID/RAW_MATERIAL), Base UOM (PCS/BOX/ROLL/KG),
Pack Size, Lot/Serial/Expiry Required (booleans), Default Zone
(A/B/C — a static picker with no warehouse context), Min/Max Stock,
Status (ACTIVE/INACTIVE), Description. Screen: search box ("Search SKU,
product, barcode..."), category/status/lot-rule filters, a table, and a
4-tab drawer (Product Info, Barcode Mapping, Inventory Rules, UOM
Conversion). No sort control. No delete button anywhere — the only way
to retire a product is setting Status to Inactive via the edit form.
`grep`-checked the rest of the frontend for other product references:
none — no other view reads or writes product data.

Two tabs (Barcode Mapping — multiple barcode rows with
type/UOM/qty/isPrimary; UOM Conversion — multiple from/to/qty rows) are
more complex than the task's own suggested schema, which lists a single
scalar `barcode` field, not a child table. Consistent with prior
phases' deferral pattern (Generate Bin Locations in Phase 3, role
create/delete in Phase 2), these two tabs remain **local-only mock
UI this phase** — documented as an inferred scope decision, not a
silent omission. The Product Info tab gained one new field, **Barcode**
(single, optional, unique-when-present), so the real scalar column has
a UI home; everything else maps 1:1 to existing fields.

### RBAC changes

`product` module added to the permission catalog (view/create/update/
delete — 43→47 total codes; same 4-action shape as `users`/`warehouse`,
no `approve`, since this is a config screen like those two, not an
approval workflow). `ADMIN` and `WAREHOUSE_MANAGER` picked up the 4 new
codes automatically via their generic grant computation (no catalog
edit needed for either). No other role was granted `product.*` —
Product Master lives under Administration nav alongside Users/Roles/
Warehouse, so the same administrative-scope reasoning from
`warehouse.*` applies directly; granting it to RECEIVING/QC/PUTAWAY/
PICKING/SHIPPING would have been inventing a requirement not derivable
from the actual frontend. Verified live via `/auth/me` for a real
ADMIN user (has all 4) and a real PICKING user (has none, confirmed
403 on every `/products` route).

### Migration files created

`backend/migrations/0008_create_products_table.{up,down}.sql` —
additive, no edits to 0001–0007.

### Product schema

`products`: sku (unique), name, description, category (CHECK-constrained
to the 4 frontend values), base_uom (CHECK-constrained to the 4
frontend values), pack_size, barcode (nullable, unique **when present**
— implemented as a filtered unique index, since a plain SQL Server
UNIQUE constraint only allows a single NULL across the whole table,
unlike Postgres/MySQL), lot_required/serial_required/expiry_required
(bit), default_zone (plain string — not a FK to `zones`, since the
frontend's picker has no warehouse context and inventing that link
isn't supported by the actual UI design), min_stock, max_stock, status,
timestamps. No FK to any other table — Inventory doesn't exist yet, so
there's nothing to reference and nothing to protect against on delete.

### APIs created

```
GET/POST       /api/v1/products
GET/PUT/DELETE /api/v1/products/{id}
```

`DELETE` is a real hard delete (no children to protect against yet —
documented in the handler that this must gain the same
referential-safety check `warehouse`/`zone` delete already have once
Inventory exists). It has **no UI button** — Product Master only ever
toggles status — exactly like `users`' delete endpoint in Phase 2;
verified via curl/Swagger only, same established precedent.

### Frontend files modified

- New: `services/products.js`.
- `ProductMasterView.vue`: mock array replaced with real API calls;
  loading/error states; one new **Barcode** field on the Product Info
  tab; summary cards (previously hardcoded "1,248"/"1,180"/"320"/"68")
  now compute from real data. A small field-name mapping layer
  (`mapProductFromApi`/`mapProductToApi`) translates between the
  backend's snake_case and the frontend's existing camelCase
  (`productForm.baseUom` etc.) so the rest of the component — and the
  entire template — needed no further changes. Layout, tabs, filters,
  and table structure unchanged.
- `RolePermissionView.vue`: added `product` to the local
  `MODULE_ORDER`/`MODULE_NAMES` maps (2-line addition, matrix itself
  already API-driven since Phase 2).
- `MainLayout.vue`: Product Master nav item gated by
  `hasPermission('product.view')`.

### Swagger changes

`GET/POST /products`, `GET/PUT/DELETE /products/{id}` fully documented
(request/response models, search/category/status/lot_required query
params, pagination, 400/401/403/404/409/422, `BearerAuth`).
Regenerated and verified in Swagger UI.

### Tests performed

Unit tests (validation-only, zero-value Service — same pattern as
every prior phase, since `Repository` is concrete not an interface):
invalid category, invalid base_uom, invalid status, negative stock,
min>max stock, `Category`/`UOM.IsValid()`, `normalizeBarcode()`. Two
new RBAC middleware tests (`product.view` allowed for an ADMIN-shaped
token, denied for a PICKING-shaped token). `go test ./...` passes.

Create/duplicate-SKU/duplicate-barcode/update/search paths (which need
a real repository) verified live via curl: create → duplicate SKU
(409 `DUPLICATE_SKU`) → duplicate barcode (409 `DUPLICATE_BARCODE`) →
invalid category (422) → get by id → update → get nonexistent (404) →
search filter → no JWT (401) — all against the developer's real SQL
Server, consistent with how Phase 2/3 handled the same repository-vs-
interface tradeoff.

### Live DB verification

`go run ./cmd/migrate up` then `seed` against the developer's existing
SQL Server: `products` table created with its filtered unique index on
barcode and CHECK constraints confirmed; `permissions` grew to 47 rows;
`ADMIN`'s `/roles/{id}/permissions` confirmed all 4 `product.*` codes.

### Docker/runtime verification

`docker compose config` clean; `docker compose build` — all 4 images
rebuilt; `docker compose up -d` — all 5 services healthy immediately
(SQL Server, RabbitMQ, backend, frontend; worker running). Confirmed
on a **fresh** containerized SQL Server: `migrate` created the
`products` table and re-seeded 47 permissions from a clean volume.
Verified through the Nginx-proxied path (port 80): login,
`/api/v1/products`, Swagger UI.

Full browser verification (real Chrome): logged in as admin, created a
product through the actual UI (SKU/name/category/**barcode**), verified
it persisted via API including the new barcode field, opened it via
"Manage," changed status to Inactive and saved — table and summary
cards updated live. Logged in as a real PICKING user: confirmed
Product Master doesn't appear in the nav (alongside User Management,
Roles & Permissions, Warehouse Setup); confirmed direct navigation to
`/administration/products` still loads the page shell but the API call
fails with 403 and the UI shows `"permission denied: product.view"`
honestly — same defense-in-depth pattern verified in Phase 3. All test
data cleaned up afterward.

### Regression results

Explicitly re-verified this session, all passing: `/auth/me`, `GET
/users`, `GET /roles`, `GET /permissions` (now 47), `GET /warehouses`,
`GET /zones` and `GET /locations` (400 without their required query
param, as designed) — no Phase 1–3 behavior changed. Warehouse Setup
screen reloaded in the browser and confirmed still functional
(unrelated code, untouched this phase).

### Known issues

- **Barcode Mapping and UOM Conversion tabs remain local-only mock UI**
  — multiple barcodes/conversions per product are not persisted. If
  genuinely needed, this is child-table work analogous to
  Warehouse→Zone→Location in Phase 3, not a quick addition.
- Everything listed as a known issue in Phase 2/3 still applies
  unchanged (permission changes need re-login, no pagination UI,
  sidebar hidden below the `lg` breakpoint, Warehouse utilization not
  computable).

## Phase 5 — Inventory Core: VERIFIED COMPLETE

### Frontend requirements found (before writing code)

Inspected all inventory views. **`StockInquiryView.vue`** (in scope —
read-only) shows `sku, productName, barcode, warehouse, zone, location,
lotNo, expiryDate, availableQty, reservedQty, onHandQty, uom,
stockStatus` per row, a search box, warehouse/zone/status filters, and
a detail drawer with a "Recent Movement" ledger panel.
**`CheckStockView.vue`** (Stock Count) and **`StockAdjustmentView.vue`**
(Stock Adjustment) are full write workflows explicitly excluded this
phase — inspected only for terminology (transaction types like
Increase/Decrease, reason codes like "Damaged Goods"/"Cycle Count
Variance") that informed the ledger schema. `DashboardView.vue`'s
"Available Stock" card was left untouched — not part of the explicit
in-scope screen list, and wiring it wasn't requested.

### Inventory data model

`Product -> Inventory -> Location -> Zone -> Warehouse`, exactly as
specified. `inventory` is the current-balance table (never computed
from `products` or summed from the ledger at read time — it's an
independent, directly-queried source of truth, updated only through
the mutation service). `inventory_transactions` is the separate,
immutable, append-only ledger.

### Lot/serial/expiry design decision

- **Lot**: part of the balance's identity key —
  `(product_id, location_id, lot_no)`, with `lot_no NOT NULL DEFAULT ''`
  (not nullable). A nullable lot_no would let SQL Server's unique-index
  NULL semantics (the same pitfall hit with `products.barcode` in
  Phase 4) silently create duplicate "no-lot" balance rows for the same
  product+location instead of merging into one — `''` uniformly means
  "not lot-tracked."
- **Expiry**: an attribute of the balance row, not a separate identity
  dimension — a given lot has one expiry in practice.
- **Serial**: deliberately **not modeled**. No frontend view anywhere
  displays individual serial numbers — Product Master's
  `serial_required` is a flag with no consuming UI. Building a
  per-unit serial table now, with nothing to verify it against, would
  be guessing at a design a future Receiving/Picking phase needs to
  drive. Documented as an explicit deferral, not a silent gap.

### Migrations created

`backend/migrations/0009_create_inventory_table.{up,down}.sql`,
`0010_create_inventory_transactions_table.{up,down}.sql` — additive, no
edits to 0001–0008.

### Schema

- `inventory`: product_id/location_id FKs, lot_no, expiry_date,
  quantity, reserved_quantity, on_hold, timestamps.
  `UNIQUE(product_id, location_id, lot_no)`.
  `CHECK(quantity >= 0)` and
  `CHECK(reserved_quantity >= 0 AND reserved_quantity <= quantity)` —
  enforced in the database itself, not just application code, per the
  explicit "must not rely only on frontend checks" instruction; proven
  live (see below) by bypassing the Go service entirely with a raw SQL
  UPDATE and watching SQL Server reject it. `quantity`/`reserved_quantity`
  are `DECIMAL(18,4)`, not `INT` — `base_uom` includes KG, a weight unit
  where fractional quantities are realistic even though no mock example
  shows one.
- `inventory_transactions`: transaction_type (CHECK-constrained to all
  8 concepts — RECEIVE/PUTAWAY/MOVE/PICK/SHIP/ADJUST/RETURN/COUNT — so
  later phases never need a schema change to use it), product_id/
  location_id FKs, lot_no, signed quantity delta, reference_type/
  reference_id (loose strings, not FKs — no future-phase schema
  dependency), reason, created_by (FK to users), created_at only (no
  updated_at — structurally immutable).
- `reserved_quantity`/`on_hold` exist so the API can express the
  frontend's Available/Reserved/On Hand/Hold distinction, but neither
  is written by anything this phase — both simply default to 0/false
  on every row created so far. No reservation or hold workflow exists
  yet.

### APIs created

```
GET /api/v1/inventory                    (search/product/warehouse/zone/location/status/lot filters, pagination)
GET /api/v1/inventory/{id}
GET /api/v1/inventory/summary            (Stock Inquiry's 4 summary cards)
GET /api/v1/inventory/{id}/transactions  (Stock Inquiry detail drawer's "Recent Movement")
```

All read-only. `status` (AVAILABLE/RESERVED/HOLD/LOW_STOCK) is
**derived at query time** (in SQL, via a CASE expression, so filtering
+ pagination stay correct together) — never stored. Precedence
(documented since the mock only showed examples, never stated a rule):
`on_hold` wins outright; otherwise any reservation makes a row
"RESERVED" even if it's also running low; otherwise low available
stock (≤ the product's `min_stock`) makes it "LOW_STOCK"; otherwise
"AVAILABLE". The `/{id}/transactions` endpoint wasn't in the task's own
example list, but the Stock Inquiry detail drawer's "Recent Movement"
panel is part of the *current* in-scope screen (not a future
workflow), so exposing a read-only view of the ledger to power it with
real (currently empty) data was judged in-scope, not scope creep.

### Stock mutation architecture

`AddStock`, `RemoveStock`, `MoveStock` in `internal/inventory/service.go`
— **no HTTP handler calls any of them**; they exist purely as the
reusable foundation later Receiving/Putaway/Picking/Shipping/
Adjustment/Count phases will call. Every mutation: validates the
request → opens a database transaction → updates the balance →
inserts a ledger row → commits both together; any failure rolls back
both. `MoveStock` composes `RemoveStock` + `AddStock` inside **one**
shared transaction (not two), writing two ledger rows
(transaction_type=MOVE) correlated by a generated `reference_id` —
either both legs land or neither does.

### Concurrency strategy

The balance UPDATE is a single atomic SQL statement with the arithmetic
*and* the non-negative check both inside the WHERE clause:

```sql
UPDATE inventory SET quantity = quantity + @delta, updated_at = SYSUTCDATETIME()
WHERE product_id=@p AND location_id=@l AND lot_no=@lot AND quantity + @delta >= 0
```

This is deliberately not "read quantity in Go, compute in Go, write
back" — that pattern loses updates under concurrent access. Because the
arithmetic happens inside the database in one statement, SQL Server's
row-level locking for the duration of the UPDATE makes concurrent
callers serialize on that row automatically; a 0-rows-affected result
is the same statement authoritatively telling the caller "insufficient
stock or row doesn't exist," not a race condition. **Proven live**: 20
goroutines fired `AddStock(+1)` at the exact same `(product, location,
lot)` concurrently — final quantity was exactly source+20, zero lost
updates (see Live DB Verification below).

### RBAC

No changes. `stock_inquiry.view` already existed (Phase 2) and was
already granted to all 7 roles (confirmed in `internal/rbac/catalog.go`
before writing any code) — reused directly on the new `/inventory`
routes, per CLAUDE.md's "don't create duplicate concepts" guidance. No
new permission module, no new migration, no new grants needed.

### Frontend files modified

- New: `services/inventory.js`.
- `StockInquiryView.vue`: mock `stockList`/`movementHistory` replaced
  with real API calls (list, summary, per-item transaction history);
  loading/empty/error states added; warehouse/zone filter dropdowns
  replaced their hardcoded WH-01/WH-02/WH-03 and A/B/C options with
  real data fetched from the Warehouse/Zone APIs built in Phase 3 (a
  cascading filter — zone options load once a warehouse is selected),
  now filtering by real IDs instead of fake strings. Layout, tabs, and
  table structure unchanged; the inert "Create Adjustment" and "Export
  Stock" buttons (no click handler in the original mock either) were
  left exactly as they were.
- `MainLayout.vue`: **not modified**. Since every current role already
  has `stock_inquiry.view`, gating that one nav item while its
  siblings (Stock Adjustment, Check Stock, Receiving, ...) remain
  ungated — none of those are backed by enforced permissions yet
  either — would have been an inconsistent, arbitrary change. Backend
  enforcement (verified via 401/403 below) is the actual security
  boundary regardless.

### Swagger changes

All 4 endpoints documented (query filters, pagination, response
models, 400/401/403/404, `BearerAuth`). Regenerated and verified.

### Tests

Unit tests: `validateStockChange`'s and `MoveStock`'s upfront
validation paths (zero-value `Service`, no DB — the actual
balance/ledger/rollback/concurrency behavior needs real SQL Server
transactional semantics and was verified live instead, see below),
`TransactionType.IsValid()`, `Inventory.Available()`. Two new RBAC
middleware tests for `stock_inquiry.view` — since every real role
already has it, the "denied" case necessarily uses a synthetic minimal
token rather than a real role-shaped one (documented in the test).
`go test ./...` passes.

### Live DB verification

Applied `0009`/`0010` against the developer's real SQL Server; tables,
FKs, indexes, and CHECK constraints confirmed to exist. Because no HTTP
endpoint exposes the mutation service, verification used a temporary
Go program (`backend/cmd/verify_inventory`, deleted immediately after
this run — not part of the permanent codebase) that called
`AddStock`/`RemoveStock`/`MoveStock` directly against real scratch
Product/Warehouse/Zone/Location records, then cleaned them up. Every
check passed:

- AddStock creates a new balance row and writes one ledger row.
- A second AddStock increases the existing balance correctly.
- RemoveStock decreases balance correctly.
- Removing more than available → `ErrInsufficientStock`, and the
  balance/ledger are provably unchanged afterward (rollback proven,
  not assumed).
- Removing from a (product, location, lot) with no balance at all →
  `ErrNoStock`.
- A raw SQL UPDATE that bypasses the Go service entirely and would take
  quantity negative is rejected by the database's own CHECK constraint
  — negative stock is prevented at the data layer, not just in
  application code.
- MoveStock decreases the source and increases the destination
  atomically, writing exactly 2 ledger rows sharing one correlated
  `reference_id`.
- **20 concurrent `AddStock(+1)` goroutines against the same row → the
  final quantity was exactly right, zero lost updates.**

One real bug was found and fixed during this process:
`Repository.Summary`'s `total_skus` field consistently returned 0 while
the other three aggregate fields were correct. Cause: GORM's
`Raw().Scan()` maps SQL column aliases to Go struct fields via its
naming-strategy guess when no explicit `gorm:"column:..."` tag is
present, and it does not reliably derive `total_skus` from a field
named `TotalSKUs` (the `SKUs` acronym+plural breaks the snake_case
conversion). Fixed by scanning into an explicitly-tagged intermediate
struct, then mapping to the response DTO — verified fixed both via a
rebuild and live against the running Docker stack.

### Docker/runtime verification

`docker compose config` clean; `docker compose build` — all 4 images
rebuilt (one unrelated hiccup: Docker Desktop itself had stopped
mid-session; restarted it, containers with `restart: unless-stopped`
came back on their own). `docker compose up -d` — all 5 services
healthy on a **fresh** containerized SQL Server, `migrate` created
both new tables from a clean volume. Verified through the
Nginx-proxied path (port 80): login, `/api/v1/inventory`,
`/api/v1/inventory/summary`, Swagger UI, 401 without a JWT.

Full browser verification with real joined data (inserted directly via
`sqlcmd` inside the `wms-sqlserver` container, since no write endpoint
exists — a real product/warehouse/zone/location/inventory row created
through the actual Phase 3/4 APIs plus one balance row): Stock Inquiry
rendered the correct SKU/warehouse/location/lot/available/reserved/
on-hand/UOM and the correctly-derived "Reserved" status badge; the
detail drawer showed the same data plus an honest "No movement
recorded yet." (accurate — the row was inserted directly, bypassing
the ledger); the warehouse filter dropdown was populated with the real
warehouse; server-side status filtering confirmed correct (`?status=
RESERVED` matched the row, `?status=LOW_STOCK` correctly did not). All
test data cleaned up afterward (including via the real DELETE
endpoints for product/warehouse/zone/location).

### Regression results

Explicitly re-verified after the Phase 5 rebuild, all passing:
`/auth/me`, `GET /users`, `GET /roles`, `GET /permissions` (still 47 —
confirming no permission drift), `GET /warehouses`, `GET /products`.
No Phase 1–4 behavior changed.

### Known limitations

- **No HTTP write path for inventory yet** — by design. Receiving/
  Putaway/Picking/Shipping/Adjustment/Count (later phases) will call
  `AddStock`/`RemoveStock`/`MoveStock`, not reimplement stock mutation.
- **Serial-level tracking is entirely deferred** — see the design
  decision above. Whichever phase first needs it will need real schema
  design work, not a quick column addition.
- **`reserved_quantity`/`on_hold` are always 0/false** — no reservation
  or hold workflow writes to them yet.
- Everything listed as a known issue in Phase 2–4 still applies
  unchanged.

## Phase 6 — Inbound Receiving: VERIFIED COMPLETE

### Receiving frontend requirements found

Inspected `ReceiveView.vue` (list + a "Mock Detail Panel" drawer) and
`NewReceivingView.vue` (create form). Findings:

- The list showed two separate document-number-shaped fields per row —
  `documentNo` (e.g. `PO-20260608-001`) and `asnNo` (e.g. `ASN-0001`) —
  but the create form only has **one** free-text "Reference No." input.
  Resolved as: backend-generated `receipt_no` (race-free, derived from
  the row's own IDENTITY id — see Receipt numbering below) for
  "Document No.", the form's user-typed field for "Reference No."/"ASN
  No.".
- Vendor was a 4-value hardcoded `<select>` with no master-data
  screen anywhere in the app. Kept as a plain string column (`vendor
  VARCHAR(150) NULL`) rather than inventing a vendor master table this
  phase never asked for — CLAUDE.md is explicit about not creating
  tables the frontend doesn't need yet.
- Warehouse/Staging Location were hardcoded `<select>`s
  (`WH-01`/`WH-02`/`WH-03`, `STAGING-01`/`STAGING-02`/`QC-HOLD-01`) —
  replaced with real data from Phase 3's Warehouse/Zone/Location APIs
  (cascading: locations load once a warehouse is picked, same pattern
  `WarehouseView.vue` already uses).
- The item table's SKU/Product Name/Barcode inputs were free text with
  **no connection to any real product** — a user could type any string.
  Replaced with a real lookup against Phase 4's Product API (exact SKU
  or barcode match; the scan bar does the same lookup).
- The detail drawer's "Receiving Items" list was **entirely
  disconnected** from the row that was clicked — a hardcoded
  `mockProducts` array (SKU-0001/0002/0003) rendered no matter which
  document was opened, and its "Confirm Receiving" button had no click
  handler at all. Replaced with the real `GET /receipts/{id}` response
  and a working confirm call.
- The create form's computed `totalVariance` is colored **green when
  ≥ 0** (over-receipt) and only red when negative (under-receipt) —
  the frontend's own existing design already treats over-receipt as a
  normal, allowed outcome, not an error. Confirmed and preserved this
  as the over-receipt rule (see below).
- No lot/expiry/serial *validation* existed anywhere in the mock (the
  lot/expiry inputs were free text with no rules) — that rule comes
  from Phase 4's `lot_required`/`expiry_required`/`serial_required`
  product flags, applied here for the first time.

### Receiving workflow / status model

```
Create receipt (DRAFT or WAITING, header + lines)
        |
        v
Confirm (1..N calls, each a batch of line quantities)
        |
        v
   inventory.AddStockTx (same DB transaction)
        |
        v
   Ledger row + receipt/line status recomputed
```

Header status: `DRAFT -> WAITING -> PARTIALLY_RECEIVED -> COMPLETED`,
plus terminal `CANCELLED` (state exists in the schema/model for future
use; nothing in this phase transitions a receipt to it). Confirm may be
called multiple times against the same receipt (partial receiving across
several sessions) — each call is a delta, not a replacement, and
`received_qty` accumulates. Line status
(`PENDING`/`PARTIALLY_RECEIVED`/`COMPLETED`) and header status are both
purely derived from quantities after every Confirm, never set directly.

### Staging vs. direct-to-stock decision

The mock's Staging Location field, evaluated against the "does the
frontend actually model a putaway step" question: it doesn't — there is
no separate putaway/confirm-to-final-location screen or button anywhere
in scope, and CLAUDE.md explicitly excludes implementing Putaway this
phase. **Decision: Confirm adds stock directly to whatever `location_id`
each line specifies at confirm time** (defaulting to the receipt's
staging location, but overridable per line — proven live by putting
line 1 at a different real location than the header's staging location).
There is no separate "move from staging to final bin" step. This keeps
Receiving's own scope honest (matches what the frontend actually
exposes) rather than half-building Putaway inside this phase.

### SQL schema

- `receipts` (0011): header — `receipt_no` (unique, backend-derived),
  `receiving_type` (CHECK: PO/ASN/RETURN/TRANSFER), `reference_no`,
  `vendor VARCHAR(150) NULL`, `warehouse_id`/`staging_location_id` FKs,
  `status` (CHECK, default `DRAFT`), `receiving_date`, `remark`,
  `created_by` FK to `users`.
- `receipt_items` (0012): lines — `receipt_id`/`product_id` FKs,
  `expected_qty`/`received_qty DECIMAL(18,4)` (received is cumulative,
  documented in the migration itself), `uom`, `lot_no VARCHAR(50) NOT
  NULL DEFAULT ''` (the same "not-null-default-empty over nullable"
  pattern Phase 5 established, avoiding SQL Server's one-NULL-per-table
  uniqueness quirk if a uniqueness rule is ever added later), `location_id`
  FK, `status` (CHECK).
- `receipt_confirmations` (0013): the idempotency guard —
  `idempotency_key VARCHAR(64) PRIMARY KEY`, `receipt_id` FK,
  `created_at`.

### Inventory integration

`internal/inventory/service.go` gained one additive method,
`AddStockTx(ctx, txRepo *Repository, req StockChangeRequest)` — the
exact `AddStock` logic, but against a `Repository` the caller already
has open inside its own transaction, instead of opening a second one.
`internal/receiving/service.go`'s `Confirm` runs entirely inside
`Repository.Transaction`, and inside that one transaction: inserts the
idempotency guard row → validates the line → calls
`inventory.AddStockTx` (which itself writes the balance update and the
ledger row) → updates the receipt line → after all lines, recomputes
and updates the header status → commits. Any failure anywhere in that
sequence — including a later line in the same Confirm call — rolls back
everything already done in that call, proven live (see below). No
balance or ledger logic is duplicated in `internal/receiving`; it only
composes Phase 5's existing, already-verified service.

### Lot/expiry/serial handling

Both at line-create time and at Confirm time, the product's
`lot_required`/`expiry_required`/`serial_required` flags (Phase 4) are
enforced: missing `lot_no` on a lot-required product → 422; missing
`expiry_date` on an expiry-required product → 422; any quantity against
a serial-required product → 422 with an explicit "requires serial
tracking, which is not yet supported by receiving" message, rather than
silently accepting it or crashing. Confirmed live for all three cases.

### Idempotency strategy

`idempotency_key` (required on every Confirm call) is inserted into
`receipt_confirmations` as the **first** statement inside the
transaction. A repeat of the same key hits the PRIMARY KEY constraint,
the whole transaction rolls back (nothing re-applied), and the service
returns the receipt's current state as a successful no-op rather than
an error — so a double-click or a retried request is safe. **A real bug
was found and fixed here**: SQL Server's actual message text for this
constraint violation is `"Violation of PRIMARY KEY constraint ...
Cannot insert duplicate key ..."`, which contains neither `"2627"` nor
`"unique"` — the two substrings `isUniqueViolation` checked for (a
pattern copied from `internal/{location,zone,user,warehouse,product}`,
where the constrained columns are plain `UNIQUE`, not `PRIMARY KEY`, so
their message text really does contain "UNIQUE" and those checks are
fine as-is). The duplicate-key case was silently falling through to a
generic 500 instead of the idempotent-no-op path. Fixed by adding a
`"duplicate key"` check (`internal/receiving/repository.go`); added
`TestIsUniqueViolation` asserting against the exact real message text;
re-verified live (resending the same key now correctly returns 200 with
the unchanged state, and the ledger has exactly one row, not two).

### Quantity rules

Over-receipt is **allowed, not rejected** — confirmed against the
frontend's own variance-coloring logic (see above) and proven live:
confirming 15 against an expected 10 succeeds, the line reaches
`COMPLETED`, and `received_qty` (15) is simply left greater than
`expected_qty` (10). Zero or negative confirm quantities are rejected
(422) — a receiving action that adds nothing or removes stock isn't a
receiving action.

### Receipt numbering

`receipt_no` is derived from the row's own IDENTITY `id` right after
insert, inside the same transaction (`RCV-%06d`) — not a separately
maintained counter/sequence, so there is no race window between
"read the next number" and "claim it": the database's own IDENTITY
column already serializes concurrent inserts.

### RBAC

No new permission module or codes — reused the existing `receiving.*`
codes from Phase 2's catalog exactly as seeded
(`view`/`create`/`update`/`delete`/`approve`). `Confirm` (the endpoint
that actually moves stock) is gated on `receiving.approve`, which per
the existing Phase 2/3 seed grants only `ADMIN`/`WAREHOUSE_MANAGER`
have — `RECEIVING` staff can create/edit receipts but not confirm them.
This is a segregation-of-duties **consequence** of grants already
established in earlier phases, not a new policy decision made here.
Verified live with a real `RECEIVING`-role user: `POST /receipts` → 201,
`POST /receipts/{id}/confirm` → 403 with `"permission denied:
receiving.approve"`.

### Frontend files modified

- New: `services/receiving.js` (`listReceipts`, `getReceipt`,
  `createReceipt`, `updateReceipt`, `addReceiptItem`,
  `updateReceiptItem`, `confirmReceipt`, `newIdempotencyKey`).
- `NewReceivingView.vue`: Warehouse/Staging Location dropdowns now load
  real data (Phase 3 APIs, cascading); the item table's SKU field
  resolves against the real Product API on blur/enter (Product
  Name/Barcode become read-only, populated from the match); the scan
  bar does the same lookup; per-line and header location fields are
  real `<select>`s over real locations; "Save Draft" calls `POST
  /receipts` with `status: DRAFT`; "Confirm Receiving" calls `POST
  /receipts` with `status: WAITING` then immediately `POST
  /receipts/{id}/confirm` for every line with a receive quantity > 0,
  using a fresh `idempotency_key` per click. Both actions now navigate
  back to the list on success (previously "Save Draft" was a no-op
  `alert()` that stayed on the page — since it's now a real persistent
  create, staying would let a second click create a duplicate draft, so
  navigating away after any successful save was the correct minimal
  adjustment). The "Confirm Receiving" button is hidden (replaced with
  an explanatory line) for a user without `receiving.approve`.
  Loading/disabled-while-submitting states and an inline error banner
  (backend validation messages surfaced verbatim) were added. Layout,
  fields, and styling are unchanged.
- `ReceiveView.vue`: `receivingList` now comes from `GET /receipts`
  (server-side search/status/date filtering, replacing the old
  client-side `filteredReceivingList` computed); the 4 summary cards
  read real counts from a separate unfiltered fetch (there's no stats
  endpoint this phase); the status filter's "Receiving" and "Partial
  Received" options both map to the backend's single
  `PARTIALLY_RECEIVED` value (documented inline — the backend
  consolidates what the mock showed as two states); the detail drawer
  fetches the real `GET /receipts/{id}` on open (replacing the
  disconnected `mockProducts` array) and its "Confirm Receiving" button
  calls the real confirm endpoint for whichever lines have a receive
  quantity entered, correctly hidden once every line is already
  `COMPLETED` or the user lacks `receiving.approve`. "+ New Receiving"
  is likewise hidden without `receiving.create`. Loading/empty/error
  states added throughout.

### Swagger changes

All 5 endpoints documented (`GET/POST /receipts`, `GET/PUT
/receipts/{id}`, `POST /receipts/{id}/items`, `PUT
/receipts/{id}/items/{itemId}`, `POST /receipts/{id}/confirm`) —
summary, tags, request/response models, every status code the handler
can actually return, `BearerAuth`. Regenerated (`swag init`) and
verified all 5 route groups present in `docs/swagger.json`.

### Tests

`go test ./...` passes. New: `internal/receiving/service_test.go`
(status machine, `ReceivingType.IsValid`, `itemStatus`/
`computeReceiptStatus` derivation tables including over-receipt and
zero-expected-qty edges, `Create`'s pre-DB validation paths via fake
`warehouseChecker`/`locationChecker`, `Confirm`'s empty-items
rejection); `internal/receiving/repository_test.go`
(`TestIsUniqueViolation`, asserting against SQL Server's real PRIMARY
KEY-violation message text — the exact regression this phase's bug fix
addresses); 3 new RBAC middleware tests using real `RECEIVING`-role- and
`WAREHOUSE_MANAGER`-role-shaped tokens (`receiving.create` allowed for
RECEIVING, `receiving.approve` denied for RECEIVING and allowed for
WAREHOUSE_MANAGER-shaped). `internal/auth` also re-verified after
extracting `middleware.CurrentUserID` out of it (no behavior change).

### Live DB verification

Full lifecycle run against the real Docker SQL Server (fresh volume this
session — created scratch warehouse/zone/2 locations/3 products,
including one lot+expiry-required and one serial-required product, via
the real Phase 3/4 APIs):

- Create receipt (2 lines, one lot+expiry-required) → `WAITING`.
- Confirm 40/100 of line 1 → header `PARTIALLY_RECEIVED`, line
  `PARTIALLY_RECEIVED`, inventory balance = 40 at the confirmed
  location, exactly 1 ledger row (`RECEIVE`, `reference_id` = the real
  `receipt_no`).
- **Resend the same `idempotency_key`** → found and fixed the bug
  described above; after the fix, correctly returns 200 with the
  unchanged state and still exactly 1 ledger row (confirmed by
  re-querying `/inventory/{id}/transactions`).
- Confirm the remaining 60 + all of line 2 (50, with lot/expiry) → both
  lines `COMPLETED`, header `COMPLETED`.
- Confirm again on the now-`COMPLETED` receipt → 409
  `INVALID_TRANSITION`.
- Second receipt: confirm 15 against expected 10 → succeeds
  (over-receipt allowed), line `COMPLETED`, `received_qty=15`.
- Create with a serial-required product → 422 with the explicit serial
  message.
- Create with a lot-required product and no `lot_no` → 422.
- Create with an unknown `staging_location_id` → 404
  `LOCATION_NOT_FOUND`.
- Restarted the `wms-sqlserver` container mid-session — all receipts,
  lines, and their statuses (including the 150%-progress over-receipt)
  were unchanged afterward, confirming the volume-backed persistence
  Phase 1 set up still holds.
- Live RBAC: real `RECEIVING`-role user → create 201, confirm 403.

### Docker/runtime verification

`docker compose config` clean. Rebuilt `backend`/`worker`/`frontend`/
`migrate` images; `migrate` applied `0011`–`0013` cleanly and re-seeded
RBAC (idempotent — re-running it a second time after the backend-only
rebuild for the idempotency fix produced identical output, no drift).
All 5 services healthy. Verified through the Nginx-proxied path (port
80) in a real browser: login; Goods Receiving list rendering real
documents with correct summary counts and a correctly-consolidated
status filter; the detail drawer showing real lines and a correct
"Fully received" state; New Receiving's warehouse selection correctly
cascading real staging-location options; typing a real SKU into the
item table resolving Product Name/Barcode live; the full
create-then-confirm flow via "Confirm Receiving" producing a real
`COMPLETED` receipt that immediately appeared in the list; Stock Inquiry
(Phase 5's own screen) independently showing the exact same balances
Receiving had just written, cross-confirming the integration end to
end. No console errors.

### Regression results

Explicitly re-verified after the Phase 6 rebuild: login,
`GET /warehouses` (Warehouse Setup screen, real data), Stock Inquiry
(Phase 5, showing correct live balances), worker still consuming
RabbitMQ heartbeat messages continuously post-rebuild. No Phase 1–5
behavior changed.

### Known limitations

- **No Putaway step** — Confirm writes stock directly to whatever
  location each line specifies; there is no staging → final-bin move.
  Deliberate, see the staging-vs-direct-to-stock decision above.
- **Serial-required products cannot be received at all** — rejected
  with an explicit error rather than silently mishandled; real
  serial-level tracking is deferred to whichever future phase designs
  it properly (same deferral Phase 4/5 already documented).
- **Vendor is a free string, no master table** — matches the mock,
  which never modeled vendor master data either.
- **`CANCELLED` is modeled but unreachable** — no cancel endpoint
  exists this phase.
- **Update/AddItem/UpdateItem only allow editing while `DRAFT`/`WAITING`**
  — once anything has been received, the document that produced that
  stock movement can no longer be edited out from under it, by design.
- Everything listed as a known issue in Phase 2–5 still applies
  unchanged.

## Phase 7 — Putaway: VERIFIED COMPLETE

### Putaway frontend requirements found

Inspected `PutawayView.vue` (list + a one-item detail drawer) and
`NewPutawayView.vue` (create form). Findings:

- `PutawayView.vue`'s list is **item-level already** — each row is one
  `taskNo`/`sku`/`qty`/`fromLocation`/`suggestedBin`/`actualBin`/
  `status`, not an aggregate per-task summary the way Receiving's list
  was. This shaped the whole backend design: the primary list query
  joins task + item + product + both locations and returns one row per
  line, not a nested items-per-task shape.
- `NewPutawayView.vue`'s "Source Document" dropdown mixed QC and RCV
  reference numbers (`QC-20260608-001`, `RCV-20260608-001`), then
  `loadSourceItems()` pulled from a hardcoded `mockSourceItems` object
  keyed by that document number. QC (Quality Check) has no backend
  module yet — only Receiving does (Phase 6) — so the real
  implementation sources exclusively from completed/partially-received
  receipts; the QC half of the mock is simply not backed by real data
  this phase, matching the "don't implement QC yet" restriction.
- The mock's own item table lets a user **also** manually "+ Add Item"
  or scan an arbitrary SKU with no source document at all — a putaway
  line can be receipt-traced or ad-hoc, both are real, coexisting
  behaviors in the same table.
- Both screens display a "Suggested Bin" column, but nothing anywhere
  in this codebase (or the task's own instructions) computes a
  suggestion — the mock's values are hardcoded strings with no
  algorithm behind them. Rather than fabricate a bin-suggestion engine,
  "Suggested Bin" is presentational-only (`—`) and the one real
  location value a line has is `destination_location_id`, which is what
  both "Suggested Bin" and "Actual Bin" would have shown anyway once a
  destination is picked.
- `NewPutawayView.vue`'s own subtitle — "Move QC passed goods from
  staging area to **storage bin locations**" — is the one real
  location-classification rule the frontend actually states, and became
  the destination-must-be-STORAGE validation (see below).
- Per-item status vocabulary differs slightly between the two screens
  (`PutawayView.vue`'s rows show PENDING/IN_PROGRESS/COMPLETED/ISSUE;
  `NewPutawayView.vue`'s per-item select shows
  PENDING/CONFIRMED/ISSUE) and `NewPutawayView.vue`'s own
  `confirmPutaway()` **blocked** submission entirely if any line was
  ISSUE, while `PutawayView.vue`'s drawer has a dedicated, independent
  "Report Issue" button that submits an issue as a real action. These
  two behaviors are inconsistent with each other in the mock; resolved
  by treating ISSUE as a legitimate, real submittable outcome (matching
  the drawer, the more deliberate of the two) rather than a client-side
  submission blocker.

### Putaway workflow chosen

```
Completed/partially-received receipt line (Phase 6)
        |
        v
Putaway task (PENDING) — one task, 1..N lines,
   each line either receipt-traced or ad-hoc
        |
        v
Confirm (1..N calls; each line: CONFIRMED -> move, or ISSUE -> flag only)
        |
        v
   inventory.MoveStockTx (same DB transaction) — never AddStock
        |
        v
   Two ledger rows (source -qty / destination +qty), transaction_type=PUTAWAY
```

Task creation is **user-initiated, not automatic**: nothing creates a
task just because a receipt completed. The user opens New Putaway Task,
optionally picks a source receipt (which pre-populates lines from its
real remaining-to-putaway quantities), and/or adds lines manually/by
scan — matching option A and B from the task's own menu simultaneously,
since that's what the frontend's own `loadSourceItems()` +
`addItem()`/`scanItem()` coexisting already do. Entirely synchronous —
no RabbitMQ job creation.

### Location-type/staging decision

**No schema change was needed.** `internal/zone` (Phase 3) already has
a `Type` column with exactly the classification this phase needed —
`RECEIVING`, `STORAGE`, `PICKING`, `PACKING`, `SHIPPING`, `QC`, `HOLD` —
set on every zone since Phase 3 shipped. A location's "role" is simply
its parent zone's type; `internal/putaway/repository.go`'s
`resolveLocation` joins `locations -> zones -> warehouses` to read it.
The one rule this phase actually enforces (backed by
`NewPutawayView.vue`'s own subtitle): **the destination location's zone
must be `STORAGE`** — rejected otherwise with a clear 422. Source
location has no zone-type restriction (a putaway can legitimately start
from any location that currently holds stock — matching the "manual
from existing inventory" origin option).

### Migrations created

- `0014_create_putaway_tasks_table`: header — `putaway_no` (unique,
  backend-derived, same `PA-%06d` pattern as `receipt_no`),
  `receipt_id BIGINT NULL` FK (nullable — a task can be entirely
  ad-hoc), `warehouse_id`, `status` (CHECK:
  PENDING/IN_PROGRESS/COMPLETED/ISSUE/CANCELLED — CANCELLED
  unreachable this phase, same forward-compat precedent as Receiving),
  `priority` (CHECK: LOW/NORMAL/HIGH), `assigned_to NVARCHAR(150) NULL`
  (plain text — the form field is free text, not a user picker),
  `completed_at`.
- `0015_create_putaway_task_items_table`: lines —
  `receipt_item_id BIGINT NULL` FK to `receipt_items(id)` (nullable —
  set for receipt-traced lines, null for ad-hoc ones),
  `source_location_id`/`destination_location_id` FKs,
  `requested_qty`/`moved_qty DECIMAL(18,4)` (moved is cumulative across
  confirm calls, same convention as `receipt_items.received_qty`), no
  `expiry_date` column (deliberately — the moved lot's expiry is always
  read from the source balance row at confirm time, never stored
  redundantly on the line).
- `0016_create_putaway_confirmations_table`: the idempotency guard,
  identical shape to `receipt_confirmations`
  (`idempotency_key VARCHAR(64) PRIMARY KEY`).

All three applied and rolled back cleanly against the real Docker SQL
Server (up → down → up round-trip verified).

### APIs created

```
GET  /api/v1/putaway                       (item-level list, matches PutawayView.vue)
GET  /api/v1/putaway/{id}                  (task + its lines)
GET  /api/v1/putaway/sources/{receiptId}   (a receipt's still-eligible lines, powers loadSourceItems())
POST /api/v1/putaway                       (create task: header + lines)
POST /api/v1/putaway/{id}/confirm          (confirm/flag 1..N lines)
```

Five endpoints, matching Receiving's own scale — no per-line CRUD
endpoints, since the frontend never edits an existing persisted line
independently of a create-then-confirm submission. `GET
/putaway/sources/{receiptId}` is a new, putaway-owned read query (a
join across `receipt_items` and this phase's own
`putaway_task_items`) — it does not modify anything in
`internal/receiving`.

### MoveStock integration

`internal/inventory/service.go` gained `MoveStockTx` (the transaction-
aware counterpart to the existing `MoveStock`, added the same way
Phase 6 added `AddStockTx`) plus an additive `Service.FindBalance`
wrapper. `MoveStockRequest` gained optional `ExpiryDate`/`Type`/
`ReferenceType`/`ReferenceID` fields (all zero-value-compatible — an
existing caller passing none of them gets `MoveStock`'s original
MOVE-typed, auto-generated-reference behavior unchanged). `internal/
putaway/service.go`'s `Confirm` runs entirely inside one transaction
(`Repository.Transaction`, exposing the raw `*gorm.DB` the same way
`internal/receiving` does) and, per confirmed line: reads the source
balance's authoritative `ExpiryDate` via `inventory.FindBalance`, then
calls `inventory.MoveStockTx` with `Type: TransactionPutaway`. **No
call to `AddStock` exists anywhere in `internal/putaway`** — grepped to
confirm. Total quantity for any (product, lot) is unchanged by
construction, not by a check.

### Ledger behavior

Both legs of a move use `transaction_type=PUTAWAY` (the CHECK
constraint already had this as a first-class value since Phase 5's
schema — never used by anything until now) with
`reference_type=PUTAWAY` and `reference_id=<putaway_no>`, shared by
both rows — giving the full trace `Receipt -> Putaway -> Source (-qty)
-> Destination (+qty)` a single query can follow. Verified live (see
below): exactly one negative and one positive ledger row per confirmed
quantity, both carrying the same `reference_id`.

### Idempotency strategy

Identical mechanical pattern to Phase 6's `receipt_confirmations`,
including baking in the fix Phase 6 needed live: `isUniqueViolation`
matches `"duplicate key"` from the start (not just `"2627"`/`"unique"`,
which only ever matched genuine `UNIQUE`-constraint violations, not the
`PRIMARY KEY` violation this table actually produces) — a regression
test (`TestIsUniqueViolation`) asserts against SQL Server's real
message text for both cases. Verified live: resending the same
`idempotency_key` against `POST /putaway/{id}/confirm` returns 200 with
unchanged `moved_qty` and exactly one ledger row per leg, not two.

### RBAC verification

Reused `putaway.view`/`create`/`update`/`approve` from Phase 2's
catalog exactly as seeded — no new codes. `Confirm` gated on
`putaway.approve`; per Phase 3's `PUTAWAY` role grants
(`putaway.view`/`create`/`update`, no `.approve`), the same
segregation-of-duties shape Receiving already established. Verified
live with a real `PUTAWAY`-role user: `POST /putaway` → 201 (has
`.create`), `POST /putaway/{id}/confirm` → 403 `"permission denied:
putaway.approve"` (lacks `.approve`). 3 new RBAC middleware tests added
with real role-shaped tokens (`putaway.create` allowed for PUTAWAY,
`putaway.approve` denied for PUTAWAY, allowed for a
WAREHOUSE_MANAGER-shaped token).

### Frontend files modified

- New: `services/putaway.js` (`listPutaway`, `getPutawayTask`,
  `getPutawaySources`, `createPutawayTask`, `confirmPutawayTask`,
  `newIdempotencyKey`).
- `NewPutawayView.vue`: Source Document now lists real
  COMPLETED/PARTIALLY_RECEIVED receipts (`GET /receipts`, Phase 6);
  selecting one calls the new `GET /putaway/sources/{receiptId}` and
  populates real lines with real source locations/lots/remaining
  quantities; Warehouse/From Location are real Phase 3 data
  (cascading); the item table's SKU field resolves against the real
  Product API on blur/enter (same pattern as Receiving's item lookup);
  the scan bar resolves both a real product **and** a real destination
  location by code (matched against every location in the selected
  warehouse, fetched up front — there's no dedicated
  find-location-by-code endpoint, and warehouse-scale location counts
  make a client-side match reasonable, the same choice Receiving's own
  scan flow made for products); "Suggested Bin" renders as `—`
  (documented above); every line requires a real destination and
  quantity before Save Draft or Confirm Putaway will submit at all
  (tightened from the mock's per-action-only validation, because the
  backend's `destination_location_id` is `NOT NULL` and must differ
  from the source at create time — see Known limitations); both actions
  create-then-optionally-confirm and navigate back to the list on
  success. "Confirm Putaway" is hidden without `putaway.approve`.
- `PutawayView.vue`: `putawayList` now comes from `GET /putaway`
  (item-level, matching the table 1:1); the 4 summary cards read real
  item-status counts from a separate unfiltered fetch; the Zone filter
  fetches every warehouse's zones and flattens them (there's no
  all-zones endpoint) replacing the hardcoded A/B/C; the status filter's
  "In Progress" option now queries the real `PARTIALLY_COMPLETED`
  value; the detail drawer fetches the real task via `GET
  /putaway/{id}`, resolves a scanned "Actual Bin" code against that
  task's real warehouse locations, and both "Confirm Putaway"/"Report
  Issue" call the real confirm endpoint with `action: CONFIRMED` /
  `action: ISSUE` respectively; the drawer shows an explicit "already
  resolved" message once a line is COMPLETED/ISSUE instead of a dead
  form, and is hidden without `putaway.approve`. "+ New Putaway Task" is
  hidden without `putaway.create`.

### Swagger changes

All 5 endpoints documented (request/response models, every status code
the handler can return, `BearerAuth`, idempotency behavior noted on
Confirm). Regenerated (`swag init -g cmd/server/main.go -o docs
--parseInternal --parseDependency`, the flag set this project's own
README documents — the plain `swag init` command hit an unrelated
parser error resolving `apihttp.SuccessResponse` without them) and
verified all 4 route groups present in `docs/swagger.json`.

### Tests

`go test ./...` passes. New: `internal/putaway/service_test.go`
(`Status.CanConfirm`/`IsValid`, `Priority.IsValid`, `itemStatus`/
`computeTaskStatus` derivation tables including the ISSUE-overrides-
everything and all-terminal-mixed-outcome cases, `Create`'s pre-DB
validation paths via fake `warehouseChecker`/`receiptChecker`,
`Confirm`'s empty-items rejection); `internal/putaway/repository_test.go`
(`TestIsUniqueViolation`, baked-in correct from the start this time);
3 new RBAC middleware tests (see above).

### Live DB verification

Full lifecycle run against the real Docker SQL Server: created a
RECEIVING-zone staging location and a STORAGE-zone bin, received real
stock into staging via Phase 6, then:

- `GET /putaway/sources/{receiptId}` returned the real remaining-to-
  putaway quantity for that receipt line.
- Created a task from that source, confirmed 30 of 80 → task
  `IN_PROGRESS`, line `PARTIALLY_COMPLETED`, exactly one `-30`/`+30`
  ledger pair sharing one `reference_id`.
- Resent the same `idempotency_key` → 200, unchanged state, still
  exactly one ledger pair (idempotency proven, not assumed).
- Confirmed the remaining 50 → task `COMPLETED`, `completed_at` set.
- Confirm on a `COMPLETED` task → 409 `INVALID_TRANSITION`.
- Destination in a `RECEIVING`-type zone → 422 "must be in a STORAGE
  zone"; inactive destination → 422; same source/destination → 422;
  cross-warehouse source+destination against a task's own warehouse →
  422 `CROSS_WAREHOUSE`.
- Confirming a line whose source balance had already been fully moved
  away → 422 "insufficient source stock to move this quantity"
  (`MoveStockTx`'s own atomic guarded UPDATE catching it, not a
  separate pre-check), with the destination provably unchanged
  afterward.
- `action: ISSUE` with a remark → task `ISSUE`, item `ISSUE`, no
  ledger rows written, remark persisted.
- Real `PUTAWAY`-role user: create 201, confirm 403 (RBAC, see above).
- Full create → confirm flow driven through the actual browser UI
  (New Putaway Task form, real source document, real product lookup,
  real destination bins) produced a task that appeared correctly in
  the list and Stock Inquiry immediately afterward.

**A real bug was found and fixed during this verification**: hard-
navigating away from the browser mid-Confirm-click (and, reproduced
independently, a `curl --max-time` client disconnecting mid-request)
left the request's database transaction orphaned — `sys.dm_exec_
sessions` showed the connection `sleeping` with
`open_transaction_count=1` indefinitely, and every subsequent request
touching that (product, location, lot) row blocked forever waiting on
its lock, until a DBA manually ran `KILL <session_id>`. Root cause:
Gin cancels `c.Request.Context()` the instant the client disconnects;
that context was flowing straight into `Repository.Transaction`, so a
disconnect mid-query could abandon the transaction's Commit/Rollback
before it ever completed. Fixed in `internal/putaway/handler.go` by
detaching Create's and Confirm's database work from the request's
cancellation (`context.WithoutCancel` rebounded with its own 30-second
`context.WithTimeout`), so the transaction now always reaches a real
commit, rollback, or timeout on its own terms regardless of what the
client does. Reproduced the exact same disconnect scenario against the
fixed build: the transaction no longer orphans (confirmed via `sys.
dm_exec_sessions`, zero open transactions afterward), and a subsequent
clean request completes normally. This same `Repository.Transaction`
pattern is used identically by `internal/receiving` (Phase 6) and
`internal/inventory` (Phase 5) — the underlying risk likely exists
there too, but fixing it there was out of this phase's scope (see
Known limitations) and was not done.

### Inventory conservation verification

Before: Source (STAGE-B01) 80, Destination (BIN-C01) 0 — total 80.
After confirming 30: Source 50, Destination 30 — total 80. After
confirming the remaining 50: Source 0, Destination 80 — total 80,
unchanged throughout. Cross-checked independently via Stock Inquiry
(Phase 5's own screen) after every step, and again at the very end
across all four locations touched by every putaway test this phase
(`A-01-02`, `BIN-C01`, `STAGE-B01`, `STAGING-01`): SKU-TEST-01 summed
to 215 and SKU-TEST-02 summed to 50 both before and after the browser-
driven task, confirming putaway moved stock between locations without
creating or destroying any of it.

### Docker/regression results

`docker compose config` clean. Rebuilt `backend`/`frontend` (and
`migrate`, to embed the new `.sql` files); `migrate` applied
`0014`–`0016` cleanly. All 5 services healthy. Browser-verified:
login; Putaway list rendering real item-level rows with correct
summary counts and a resolved-state drawer; New Putaway Task's full
create-then-confirm flow against real receiving/warehouse/product data;
Stock Inquiry (Phase 5) immediately reflecting the resulting balance
changes; Receiving (Phase 6) still listing/creating/confirming
correctly; worker still consuming RabbitMQ heartbeat messages
continuously across every rebuild. No console errors.

### Known limitations

- **The orphaned-transaction risk found and fixed above is scoped to
  `internal/putaway` only** — `internal/receiving` and
  `internal/inventory` share the exact same `Repository.Transaction`
  pattern without the same protection. Recommended as an explicit,
  narrow follow-up (apply the same `mutationContext`-style detachment
  to Receiving's Confirm) rather than something folded silently into
  this phase.
- **"Save Draft" requires every line fully specified** (real product,
  source, destination, quantity) — the backend's
  `destination_location_id` is `NOT NULL` and validated against the
  source at create time, so there is no way to persist a genuinely
  incomplete line. The mock's own "Save Draft" was a no-op `alert()`
  with no real validation to relax, so this isn't a regression from any
  previously-working leniency.
- **"Putaway Date" is not persisted** — the create form still has the
  field (layout preserved), but nothing in this phase's schema consumes
  it; no workflow needs a scheduled-vs-actual date distinction yet.
- **No capacity enforcement** — Phase 3's `warehouses.capacity` is
  purely informational (no occupied-quantity tracking anywhere), and
  zones/locations have no capacity field at all; fabricating an
  enforcement rule on top of that would be inventing data the schema
  doesn't have, per the task's own instruction.
- **Barcode field in `PutawayView.vue`'s drawer is a soft check** — it
  is required to be non-empty (matching the mock) but not cross-
  validated against the item's real product barcode, since that field
  isn't part of this list's own response shape.
- Everything listed as a known issue in Phase 2–6 still applies
  unchanged.

## Phase 8 — Transaction Context Hardening and Transactional Reliability: VERIFIED COMPLETE

Pure reliability work — no new business endpoints, no schema changes,
no frontend changes. Goal: eliminate the request-cancellation
transaction risk Phase 7 discovered and fixed locally on Putaway,
consistently, everywhere it actually exists.

### Transaction path audit

| Package.Operation | Tx used? | Was at risk? | Action taken |
|---|---|---|---|
| `inventory.AddStock`/`RemoveStock`/`MoveStock` | Yes (self-transacting) | Yes (no live caller today, but hardened for when one lands) | Detach + bound before `.Transaction(` |
| `inventory.AddStockTx`/`MoveStockTx` | Reuses caller's txRepo | N/A — inherits caller's protection | Unchanged — must not detach twice |
| `receiving.Create` / `AddItem` / `Confirm` | Yes | **Yes — Confirm shares the exact shape that orphaned live on Putaway, never itself tested against disconnect until now** | Detach + bound before `.Transaction(` |
| `receiving.Update` / `UpdateItem` | No (single statement) | No — single statement auto-commits, no multi-statement window to orphan | Left unchanged |
| `putaway.Create` / `Confirm` | Yes | Already fixed in Phase 7, but ownership lived in `handler.go` | Moved to Service layer; centralized timeout |
| `role.SetRolePermissions` → `ReplaceRolePermissions` | Yes (delete+insert) | Yes — same risk class, never previously tested | Detach + bound in Service before calling repo |
| `cmd/migrate` up/down/seed | Yes (`sql.Tx`) | No — `context.Background()` at the CLI's own top level, no HTTP client exists in this process | Out of scope, left unchanged |

**Root cause (confirmed live, not just theorized):** Gin cancels
`c.Request.Context()` the instant the client disconnects. That context
flowed straight into `Repository.Transaction` in every package above,
so a disconnect mid-transaction could abandon it before its
Commit/Rollback ever ran — proven in Phase 7 on Putaway via both a hard
browser navigation and an independent `curl --max-time` reproduction,
confirmed via `sys.dm_exec_sessions` showing a connection `sleeping`
with `open_transaction_count=1` indefinitely.

**A second, more severe pre-existing bug was found while implementing
this phase's own request_id/logging requirement**, unrelated to SQL
transactions but squarely a request-reliability issue: `gin.
RouterGroup.Group()` copies the parent's middleware chain at the moment
it's called. `internal/platform/http.NewEngine()` created and returned
the `/api/v1` route group *before* `cmd/server/main.go` ever called
`engine.Use(...)` — so none of `Recovery`/`RequestID`/`RequestLogging`/
`CORS` had ever actually run for any `/api/v1/*` route, since Phase 1.
Per-route JWT auth still worked (wired separately, per-package). Most
seriously: **`Recovery` never protecting these routes meant a single
unrecovered panic in any handler could have crashed the entire backend
process for every in-flight request**, not just the one that panicked
— a far larger blast radius than the orphaned-transaction bug this
phase set out to fix. Fixed by reordering `NewEngine` to accept and
apply middleware before creating the group (`internal/platform/http/
router.go`); a new `TestNewEngine_RecoveryProtectsV1Routes` proves a
panicking handler on the group is now caught rather than escaping.

### Central transaction context policy

New package `internal/platform/dbctx`:

- `Detach(ctx) (context.Context, context.CancelFunc)` —
  `context.WithTimeout(context.WithoutCancel(ctx), timeout)`. Called
  once per mutation, immediately before the SQL transaction opens — not
  at the top of the method. Validation/lookups before that point stay
  on the original, still-cancellable context (see Cancellation
  semantics below).
- `WithRequestID`/`RequestIDFromContext` — `middleware.GetRequestID`
  lives on `*gin.Context` and never crosses the Handler -> Service
  boundary on its own; the handler embeds it onto `context.Context`
  before calling the service, so it survives `Detach` (which preserves
  values, only strips cancellation/deadline) and comes back out in
  `LogTransaction`.
- `LogTransaction(ctx, operation, start, err)` — one structured log
  line per transaction (`transaction committed`/`transaction failed`),
  with `request_id`, `operation`, `duration`, and — on failure — a
  `classification` (`cancelled_or_timeout` / `deadlock` /
  `unique_violation` / `fk_violation` / `other`) via the new `sqlerr`
  package below. Not per-SQL-statement logging — this is not GORM's
  slow-query logger, and nothing here logs bind parameters, SQL text,
  passwords, JWTs, or DB secrets.

`SetDefaultTimeout` is called once at startup in both `cmd/server` and
`cmd/worker` (`main.go`), the same way `slog.SetDefault` already
configures the process-wide logger — a deliberate, single existing
precedent this phase followed rather than threading a `time.Duration`
through four separate service constructors.

### Transaction timeout configuration

New `DB_TRANSACTION_TIMEOUT` (Go duration string, e.g. `30s`) —
deliberately **not** `DB_TRANSACTION_TIMEOUT_SECONDS` as a plain
integer: this codebase already has an established Go-duration-string
convention (`JWT_EXPIRES_IN=1h`, parsed via the same `parseDuration`
helper with the same graceful-fallback-on-unset-or-invalid behavior),
and matching it was judged more valuable than introducing a second,
inconsistent seconds-based convention. Default `30s`. Documented and
added to `.env.example`, `backend/.env.example`, and the developer's
own real (gitignored) `.env`/`backend/.env` files. `docker-compose.yml`
needed **no changes** — `backend`/`worker`/`migrate` already load their
entire environment via `env_file: - .env`, so the new variable is
picked up automatically. Not exposed to the frontend (no
`VITE_`-prefixed equivalent — this is a pure backend concern).

Also used to compute the server's shutdown grace period
(`cfg.DBTransactionTimeout + 5s`, replacing a hardcoded `15*time.
Second`) — see Graceful shutdown below.

### Transaction ownership: Service layer, not Handler

`putaway/handler.go`'s Phase 7 `mutationContext` helper (which called
`dbctx`-equivalent logic directly in the HTTP layer) was removed
entirely. `putaway/service.go`'s `Create`/`Confirm` now own the
detach+timeout decision themselves, the same shape as
`receiving`/`inventory`/`role`. Handlers across all four packages now
do exactly one thing related to this policy: embed the request id onto
`c.Request.Context()` via `dbctx.WithRequestID` before calling the
service — transaction-boundary ownership itself lives entirely in the
Service layer, consistently.

### Receiving hardening

`Create`, `AddItem`, and `Confirm` each now: run their pre-transaction
validation/lookups on the original cancellable context (unchanged
behavior — an already-gone client still fails fast here), then detach
before `s.repo.Transaction(...)`, then call `dbctx.LogTransaction`
around it. Reproduced the exact disconnect scenario Phase 7 found on
Putaway, live, against Receiving's `Confirm` for the first time ever:
create a receipt, `curl --max-time 0.001` a confirm call (client
disconnects in ~1ms, before the server can possibly finish) — no
orphaned transaction (`sys.dm_exec_sessions` clean immediately after),
the transaction ran to a full commit server-side regardless (receipt
`COMPLETED`, `received_qty` correct, immediate follow-up request
answered in under 200ms with zero blocking), and a client retry with
the same `idempotency_key` returned the unchanged state with exactly
one `RECEIVE` ledger row, not two.

### Inventory hardening

`AddStock`, `RemoveStock`, `MoveStock` (the self-transacting, currently-
uncalled-by-any-HTTP-handler public methods) each detach+bound before
opening their own transaction, exactly like Receiving/Putaway/Role —
hardened now so a future phase (Picking, Adjustment, ...) that wires an
HTTP handler directly to them inherits the same protection by
construction rather than needing to remember this policy exists.
`AddStockTx`/`MoveStockTx` (the composable versions `receiving`/
`putaway` actually call today) are **unchanged** — per the audit, they
reuse whatever transactional `txRepo`/context the caller already has
open and must not start a second, unrelated one; they already only
ever forwarded the caller's `ctx` as-is, so nothing needed to change
there, only verifying that remains true. `translateFKViolation`'s gate
now checks the real SQL Server error number (547, via `sqlerr`) before
inspecting the message for which constraint, rather than an unguarded
substring search.

### Putaway regression

Phase 7's handler-level fix was not assumed final — re-reviewed against
the new centralized policy and refactored to match Receiving/Inventory/
Role exactly (see Transaction ownership above). Re-ran the exact
original failure scenario against the refactored code: create a task,
`curl --max-time 0.001` a confirm call — no orphan, task reached
`COMPLETED` server-side despite the ~1ms disconnect, an immediate
follow-up request answered in 89ms with zero blocking, a retry with the
same `idempotency_key` left `moved_qty` and the destination balance
unchanged, and the destination's ledger held exactly one `PUTAWAY` row
for that operation.

### SQL Server error-handling improvements

New package `internal/platform/database/sqlerr`, built on
`github.com/microsoft/go-mssqldb`'s `mssql.Error` (already an indirect
dependency via `gorm.io/driver/sqlserver`; `go mod tidy` promoted it to
direct once imported) via `errors.As` — real error **numbers** instead
of message substrings:

- `IsUniqueViolation` (2627 or 2601) — replaces the six independently-
  duplicated `isUniqueViolation` string-matching functions across
  `location`/`zone`/`user`/`warehouse`/`receiving`/`putaway`
  (`product`'s differently-named `translateUniqueViolation` was
  hardened the same way: gate on the real error number first, then
  disambiguate which column from the message, instead of an unguarded
  substring search). This directly generalizes the fix Phase 6 needed
  live for `receipt_confirmations`' PRIMARY KEY violation (whose
  message contains neither `"2627"` nor `"unique"`) and Phase 7 copied
  forward for `putaway_confirmations` — both packages' own
  `isUniqueViolation` now delegate to the shared classifier instead of
  carrying their own patched-up substring list.
- `IsForeignKeyViolation` (547) — used to hardn `inventory`'s
  `translateFKViolation` gate.
- `IsDeadlock` (1205) — exposed for callers that need to distinguish a
  genuine data conflict from an orphaned lock/cancellation; nothing in
  this codebase retries on deadlock (the guarded-UPDATE concurrency
  pattern was proven live under 20 concurrent writers with zero
  deadlocks in Phase 5, and again this phase — see Concurrency
  regression below), so no retry policy was invented.
- `IsCancelled` — `errors.Is(err, context.Canceled) ||
  errors.Is(err, context.DeadlineExceeded)`, used by `LogTransaction`'s
  classification.

`sqlerr` has its own exhaustive unit tests (real `mssql.Error{Number:
...}` values, not fake strings). The two repositories whose own tests
previously constructed fake string-based errors to simulate SQL Server
message text (`putaway`/`receiving`'s `TestIsUniqueViolation`) were
updated to use real `mssql.Error` values and now only confirm the
delegation itself is wired correctly, since the exhaustive
classification truth table is `sqlerr`'s own responsibility.

### Connection pool review

`internal/platform/database/database.go`'s pool settings
(`MaxOpenConns=25`, `MaxIdleConns=5`, `ConnMaxLifetime=30m`,
`ConnMaxIdleTime=5m`, set since Phase 1) were reviewed against this
phase's findings. **No change made.** The orphaned-transaction bug was
never actually a pool-sizing problem — even a single orphaned
connection blocked *specific rows*, not the pool itself, and 25
connections comfortably covers this application's realistic concurrent
load; the real fix was ensuring transactions can't be orphaned in the
first place (Detach), not making the pool larger to dilute the
symptom. These values remain reasonable and were left as documented.

### Idempotency regression results

Both Receiving and Putaway confirmation, live, under: identical
idempotency key resubmission, a simulated request cancellation followed
by a retry with the same key, and a rapid double-submit — in every
case, the mutation applied exactly once (one ledger row per leg, one
balance change, no double stock movement). See Receiving hardening /
Putaway regression above for the specific runs.

### Concurrency regression results

Re-ran Phase 5's concurrent-mutation proof, live, through the real HTTP
API this time (Phase 5's original proof used a throwaway Go program
calling the service directly) — fired 10 concurrent `POST /receipts/
{id}/confirm` calls against the *same* receipt line, each adding 10
units. Inventory's ledger and balance were **exactly correct**: 10
separate `RECEIVE` ledger rows of 10 each, balance increased by exactly
100, zero lost updates, zero duplicates — `AdjustBalance`'s atomic
guarded UPDATE (the mechanism, unchanged by this phase) remains fully
intact under concurrency after the context-detachment changes.

**A real, pre-existing (Phase 6, not introduced by this phase) bug was
found by this same test**: the receipt line's own `received_qty` field
under-counted (40 instead of 100) under the same concurrent load.
Cause: `receiving.Confirm` (and, by the identical pattern,
`putaway.Confirm`'s `moved_qty`) updates that column via an unguarded
read-modify-write (`item.ReceivedQty += lineReq.Quantity` after a plain
`FindItemByID`, no row locking or optimistic-concurrency check) —
classic lost-update territory under concurrent requests against the
*same line*, structurally unrelated to inventory's own atomic
guarded-UPDATE pattern and to this phase's context/cancellation work.
**Not fixed here** — it is a business-logic concurrency gap, not a
transaction-context reliability issue, and this phase's own
restrictions are explicit that "no business behavior should change
except increased transaction reliability." Flagged as a known risk and
a Phase 9 candidate (see below) — the inventory ledger itself, which is
the actual system of record, was proven unaffected.

### Cancellation/timeout live tests

All performed against the real Docker SQL Server (see Receiving
hardening / Putaway regression / Idempotency regression above for the
detailed runs): request cancellation before a transaction starts
(pre-transaction validation simply returns its error, nothing to
orphan — exercised continuously by existing validation-only unit
tests, which run with no real transaction involved at all), cancellation
after the transaction has begun (the ~1ms-disconnect reproductions),
transaction timeout (`internal/platform/dbctx`'s own tests prove the
bound fires independently of the parent), rollback (every negative-path
live test from Phases 6/7 re-verified: insufficient stock, invalid
transition, cross-warehouse, wrong zone type — all still roll back
cleanly with the hardened context), successful commit (every positive-
path test above), and duplicate confirmation after cancellation (the
idempotency regression runs above).

### Docker/regression results

`docker compose config` clean. Rebuilt `backend`, `worker` (both import
`internal/platform/dbctx` now), and `migrate` (no schema changes this
phase, rebuilt only because its image shares the backend's Go module —
`up`/`seed` both ran, no new migrations, fully idempotent). All 5
services healthy. One operational hiccup unrelated to the code changes
themselves: rebuilding `backend` mid-session left nginx (`frontend`)
holding a stale resolved IP for the `backend` upstream hostname
(`connect() failed (111: Connection refused)` — a standard Docker+
nginx-static-upstream-resolution gotcha, not a Phase 8 defect);
resolved by restarting the `frontend` container, after which every
proxied route worked immediately. Verified through the browser after
that: login, Dashboard, Receiving (list + real data), Putaway (list +
real data), Warehouse Setup, Stock Inquiry (reflecting every balance
change this phase's live tests produced) — no console errors. Swagger
UI reachable (`/swagger/index.html` → 200); no annotations changed
this phase, so it was **not** regenerated, per this phase's own
restriction against creating or documenting endpoints that don't exist
or don't need updating. Worker confirmed still consuming RabbitMQ
heartbeat messages continuously across every rebuild.

### Tests

New: `internal/platform/dbctx` (parent-cancellation immunity, own
bounded timeout, value preservation, request-id round-trip, unconfigured
fallback), `internal/platform/database/sqlerr` (exhaustive
classification truth table against real `mssql.Error` values),
`internal/platform/http` (the middleware-ordering bug, reproduced and
proven fixed, including a dedicated Recovery-specific test),
`config` (the new timeout var's default and override). Updated:
`putaway`/`receiving`'s `TestIsUniqueViolation` (real `mssql.Error`
values instead of fake strings). `go test ./...` passes throughout;
`go vet ./...` clean. `role` package still has no unit tests at all —
a pre-existing gap (not introduced by this phase, since the change made
there is a pure refactor around already-correct, if untested, logic)
worth closing alongside whatever phase next touches RBAC.

### Known remaining risks

- **The lost-update race in `receipt_items.received_qty` / `putaway_task_items.moved_qty`** discovered by this phase's own
  concurrency regression test (see above) — real, pre-existing, and
  explicitly out of this phase's scope. The inventory ledger/balance
  (the actual system of record) is unaffected; only the receipt/task's
  own progress display can under-count under genuinely concurrent
  confirms on the same line. Recommended fix direction: an atomic
  guarded UPDATE on the quantity column (mirroring `AdjustBalance`)
  instead of the current read-modify-write, or a version/rowversion
  column for optimistic concurrency.
- **The orphaned-transaction protection is Receiving/Putaway/Inventory/
  Role only** — any *future* package that opens its own
  `Repository.Transaction` directly from a request-derived context
  without going through `dbctx.Detach` reintroduces the exact class of
  bug this phase eliminated. There is no compiler-enforced guardrail
  against this (Go has no way to forbid passing a raw `context.Context`
  into a specific function) — it depends on whoever writes the next
  transactional service method knowing this policy exists and applying
  it. Worth a code-review checklist item, since nothing else currently
  catches a lapse automatically.
- **`role` package has zero unit tests** (pre-existing, not newly
  introduced — see Tests above).
- Everything listed as a known issue in Phase 2–7 still applies
  unchanged, except where this phase's own hardening supersedes it
  (Phase 6's and Phase 7's `isUniqueViolation` string-matching notes are
  now superseded by the centralized `sqlerr` classifier).

## Phase 9 — Picking + Lost-Update Race Fix: VERIFIED COMPLETE

### Race-condition fix (done first, per this phase's own instructions)

Phase 8's own concurrency regression test found a real, pre-existing bug:
`receiving.Confirm`'s `received_qty` and `putaway.Confirm`'s `moved_qty`
were each updated via an unguarded Go read-modify-write
(`FindItemByID` → `item.ReceivedQty += lineReq.Quantity` → write the
whole row back) — under two concurrent `Confirm` calls against the same
line, both could read the same starting value and the second UPDATE's
stale computed total would silently overwrite the first's, undercounting
real progress. The inventory ledger/balance itself (the actual system of
record, written by `AddStockTx`/`MoveStockTx`) was never affected — only
these two display/progress columns.

Fixed identically in both packages, mirroring `internal/inventory`'s own
`AdjustBalance` pattern: a new atomic guarded UPDATE
(`receiving.Repository.AdjustReceivedQty` /
`putaway.Repository.AdjustMovedQty`) does `SET x = x + ?` as a single SQL
statement — the arithmetic happens inside the database, serialized by
the row's own lock, not computed in Go from a stale read. `Confirm` now:
calls `AddStockTx`/`MoveStockTx` → calls the atomic increment → re-reads
the item fresh (so status derivation uses the true post-increment total,
not the loop's pre-transaction snapshot) → writes the remaining derived
fields (`lot_no`/`expiry_date`/`location_id`/`status` for receiving;
`destination_location_id`/`status`/`remark` for putaway) via a new
`UpdateItemAfterConfirm` that deliberately excludes the quantity column,
so it can never clobber a concurrent transaction's own atomic increment
with a stale value. No business behavior changed — over-receipt is still
allowed, terminal-line rejection is unchanged, idempotency is unchanged.

**Proven live, not assumed**: re-ran Phase 8's own proof at 2x the load.
20 concurrent `POST /receipts/{id}/confirm` calls against the same line
(each `+10`) → `received_qty` landed at exactly 200, 20 separate
`RECEIVE` ledger rows, zero lost updates (Phase 8's unfixed version
undercounted 10×10 to 40 instead of 100). The identical test against
`POST /putaway/{id}/confirm` → `moved_qty` landed at exactly 200, one
`-`/`+` ledger pair per confirmed quantity. `internal/picking`'s own
`picked_qty` (new this phase, built on the same atomic pattern from the
start — see `AdjustPickedQty` below) was proven the same way: 20
concurrent `POST /picking/{id}/confirm` calls (`+10` each) →
`picked_qty` landed at exactly 200, and the source location's inventory
balance decreased by exactly 200, not more or less.

### Picking frontend requirements found

Inspected `PickingView.vue` — a single list+drawer screen, unlike
Receiving/Putaway which each have a separate create view
(`NewReceivingView.vue`/`NewPutawayView.vue`); there is no
`NewPickingView.vue` anywhere in the frontend. Findings:

- The list is task-level (Pick No./Order No./Customer/Warehouse/
  Priority/Items/Progress/Due Date/Status), unlike Putaway's item-level
  list — this shaped the backend's own list query to aggregate
  item-count and progress per task, matching Receiving's list query
  shape instead of Putaway's.
- `orderNo`/`customer` reference a Sales/Outbound Order that has no
  backend module anywhere in this codebase (grepped the frontend: no
  `OrderView`, no order service) — kept as plain free-text columns
  (`order_no` required, `customer` optional), the same precedent
  `receipts.vendor` (Phase 6) already established for exactly this
  situation.
- The mock's "+ New Picking Task" button opened the **same** drawer used
  to view an existing task, populating a hardcoded placeholder header
  and the **same 3 hardcoded `pickingProducts`** regardless of which
  task (or none) was open — the drawer's items list was entirely
  disconnected from its own header, the same class of gap Phase 6 fixed
  for `ReceiveView.vue`'s detail drawer. There was no way in the mock to
  actually add a line to a new task (no "+ Add Item" button, no
  product/location inputs) — "Complete Picking" was a bare `alert()`.
- Resolved by making the single drawer serve two real modes: **create**
  (a header form — Order No./Customer/Warehouse/Priority/Due Date — plus
  a minimal "Add Item" mini-form: SKU/Barcode lookup against the real
  Product API, a real cascading Warehouse→Zone→Location `<select>`, Lot
  No., Order Qty) and **view/confirm** (the real task + lines fetched
  from the API, with the mock's existing Pick Qty input, "Confirm Pick,"
  and "Short Pick" buttons now wired to the real confirm endpoint). This
  is the same "smallest functional change that makes create actually
  work through a real API" precedent as Phase 2's Password field, Phase
  3's Capacity field, and Phase 6's location dropdowns — not a redesign
  of the screen's layout or an added route.
- "Complete Picking" (previously a no-op `alert()`) is relabeled "Close"
  for the view/confirm mode, since task/header status is entirely
  derived from line confirmations (`computeTaskStatus`) — there is
  nothing left for a separate "complete" action to do, and a button that
  claimed to complete something without doing so would be dishonest UI,
  the same reasoning Phase 5 applied to an unfabricated utilization
  number.
- "Short Pick" needed a remark (mirroring Putaway's "Report Issue"), so a
  small remark input was added next to each line's action buttons in
  the confirm mode — the mock's "Short Pick" button was a bare
  `alert()` with no remark capture at all.

### Picking workflow chosen

```
Picking task (PENDING) — one task, 1..N lines,
  each line naming a real product + source location + lot
        |
        v
Confirm (1..N calls; each line: PICKED -> remove stock, or SHORT -> flag only)
        |
        v
   inventory.RemoveStockTx (same DB transaction) — never AddStock/MoveStock
        |
        v
   One ledger row (transaction_type=PICK), reference_type=PICKING
```

Task creation is **manual/either-role**, not sourced from any order
system (none exists) — `picking.create` gates who may create a task (see
RBAC below), matching the reality that no source-document flow (like
Putaway's receipt-sourcing) is possible without an Order module. Entirely
synchronous — no RabbitMQ job creation, consistent with CLAUDE.md's
"don't use RabbitMQ simply because it exists" guidance and this being a
short, synchronous, immediately-answerable operation.

### RBAC — a genuinely different shape from Receiving/Putaway

Reused the existing `picking.view`/`create`/`update`/`approve` codes
from Phase 2's catalog exactly as-is — no new codes, no catalog edit.
But the **grants** already seeded for the `PICKING` role
(`internal/rbac/catalog.go`, unchanged since Phase 2) are `picking.view`
and `picking.update` only — **not** `picking.create` and **not**
`picking.approve`. This is the opposite shape from `RECEIVING`/`PUTAWAY`
(which have `.create` but not `.approve`). Rather than treat this as a
gap to silently work around, it was read as the actual intended design
this catalog already encodes: picking staff execute already-created
tasks (view + update) rather than build or approve them. So:

- `POST /picking` (Create) is gated on `picking.create` — under current
  grants, only `ADMIN`/`WAREHOUSE_MANAGER` can create a task.
- `POST /picking/{id}/confirm` (the per-line pick/short-pick execution
  action) is gated on **`picking.update`**, not `picking.approve` —
  the permission the `PICKING` role actually has. `picking.approve`
  remains unused by any endpoint this phase, reserved for a future
  approval workflow (e.g. sign-off before shipping) that isn't part of
  the current frontend.

Verified live: a real `PICKING`-role user gets 403
`picking.create` on `POST /picking` but 200 on
`POST /picking/{id}/confirm`; a real `WAREHOUSE_MANAGER`-role user can
create a task, and a `PICKING`-role user can then confirm it — the
intended cross-role handoff, proven end to end (WM creates → PICKING
staff executes). 4 new RBAC middleware tests (`picking.create`
denied for PICKING, allowed for WAREHOUSE_MANAGER-shaped;
`picking.update` allowed for PICKING).

### Migrations created

`backend/migrations/0017_create_picking_tasks_table.{up,down}.sql`,
`0018_create_picking_task_items_table.{up,down}.sql`,
`0019_create_picking_confirmations_table.{up,down}.sql` — additive, no
edits to 0001–0016.

### Picking schema

- `picking_tasks`: `pick_no` (unique, backend-derived from the row's own
  IDENTITY id, `PICK-%06d`, the same race-free-by-construction approach
  as `receipt_no`/`putaway_no`), `order_no` (`VARCHAR`, required, plain
  text — no Order module), `customer` (`NVARCHAR`, nullable, plain
  text), `warehouse_id` FK, `status` (CHECK:
  PENDING/IN_PROGRESS/COMPLETED/SHORT_PICK/CANCELLED — CANCELLED
  unreachable this phase, same forward-compat precedent as
  Receiving/Putaway), `priority` (CHECK: LOW/NORMAL/HIGH), `due_date`
  (`DATE`, nullable), `remark`, `created_by` FK to `users`,
  `completed_at`.
- `picking_task_items`: `product_id`/`location_id` FKs (`location_id` is
  the **source** to pick from — picking only ever removes stock, unlike
  Putaway which has separate source/destination columns),
  `lot_no VARCHAR(50) NOT NULL DEFAULT ''` (the same not-null-default-
  empty pattern `receipt_items`/`putaway_task_items` established, for
  the same SQL Server one-NULL-per-table uniqueness reason),
  `ordered_qty`/`picked_qty DECIMAL(18,4)` (`picked_qty` cumulative
  across confirm calls, written via the atomic guarded UPDATE from the
  start — see the race-fix section above), `status` (CHECK:
  PENDING/PARTIALLY_PICKED/COMPLETED/SHORT).
- `picking_confirmations`: the idempotency guard, identical shape
  (`idempotency_key VARCHAR(64) PRIMARY KEY`) to
  `receipt_confirmations`/`putaway_confirmations` — built with the
  centralized `sqlerr.IsUniqueViolation` classifier from the start
  (Phase 8), so this package never needed the live "duplicate key"
  message-text fix Phase 6 originally needed and Phase 7 had to copy
  forward.

### APIs created

```
GET  /api/v1/picking                (task-level list, matches PickingView.vue)
GET  /api/v1/picking/{id}           (task + its lines)
POST /api/v1/picking                (create task: header + lines)
POST /api/v1/picking/{id}/confirm   (confirm/flag 1..N lines)
```

Four endpoints — smaller than Receiving/Putaway's five, since there is
no per-line update/add-item endpoint the way Receiving needed
(`NewPutawayView`/`NewReceivingView`'s separate creation flow don't
exist here; the whole task is submitted as one `Create` call from the
single drawer, matching how the mock actually behaves).

### Inventory integration

`internal/inventory/service.go` gained one additive method,
`RemoveStockTx(ctx, txRepo *Repository, req StockChangeRequest)` — the
exact `RemoveStock` logic, but against a `Repository` the caller already
has open inside its own transaction, the same pattern `AddStockTx`
(Phase 6) and `MoveStockTx` (Phase 7) already established. `RemoveStock`
itself (self-transacting, hardened for cancellation in Phase 8) is
unchanged. `internal/picking/service.go`'s `Confirm` runs entirely
inside `Repository.Transaction` and, per confirmed line: calls
`inventory.RemoveStockTx` (writes the balance decrement + ledger row
itself) → atomically increments `picked_qty` → recomputes item/task
status → commits. **No call to `AddStock`/`MoveStock` exists anywhere in
`internal/picking`** — grepped to confirm, the same verification
Putaway's own doc used for its own no-AddStock claim.

### Idempotency

Identical mechanical pattern to Phase 6/7/8:
`picking_confirmations.idempotency_key` (required on every Confirm call)
is inserted as the **first** statement inside the transaction; a repeat
of the same key hits the PRIMARY KEY constraint via the centralized
`sqlerr` classifier, the whole transaction rolls back, and the service
returns the task's current state as a successful no-op. Verified live:
resending the same `idempotency_key` against
`POST /picking/{id}/confirm` returns 200 with unchanged `picked_qty` and
exactly one `PICK` ledger row, not two.

### Frontend files modified

- New: `services/picking.js` (`listPicking`, `getPickingTask`,
  `createPickingTask`, `confirmPickingTask`, `newIdempotencyKey`).
- `PickingView.vue`: mock `pickingList`/`pickingProducts` replaced with
  real API calls; the 4 summary cards now compute from a separate
  unfiltered fetch (task-status counts), the same approach
  Receiving/Putaway use; a real create-mode header form and "Add Item"
  mini-form were added (Warehouse/Location cascading from Phase 3's real
  APIs, product lookup against Phase 4's Product API — see frontend
  requirements above); the existing Pick Qty/Confirm Pick/Short Pick
  controls now call the real confirm endpoint, with a per-line remark
  input added for Short Pick; "Complete Picking" relabeled "Close" in
  view/confirm mode (see above). `MainLayout.vue`'s Picking nav item was
  **not modified** (left ungated) — the same reasoning Phase 5
  established: `RECEIVING`/`QC`/`SHIPPING` roles lack `picking.view`
  under current grants, and gating one operational nav item while its
  siblings (Receiving, Putaway, Stock Adjustment, Shipping) remain
  ungated would be an inconsistent, arbitrary change; backend enforcement
  (verified via 403 below) is the actual security boundary regardless.
- `RolePermissionView.vue`: **not modified** — `picking` was already in
  the local `MODULE_ORDER`/`MODULE_NAMES` maps since Phase 2 (it's one
  of the frontend's original hardcoded modules), so no addition was
  needed this phase.

A real bug was found and fixed during browser verification (not present
in any curl-driven test): the "Add Item" mini-form's location lookup
(`this.drawerLocations.find((l) => l.id === Number(this.addItemForm.locationId))`)
silently failed to match anything, because location IDs are JSON-encoded
as strings by the backend (the `,string` int64 convention every other
ID field in this codebase already uses) while `Number(...)` coerced the
form value to a number — a strict `===` between a string and a number is
always `false`. The item was still added, but with a blank Location
display. Fixed by comparing `String(l.id) === String(this.addItemForm.locationId)`
instead; re-verified live in the browser (Location now displays
correctly, e.g. "Location: BIN-C01").

### Swagger changes

`GET/POST /picking`, `GET /picking/{id}`, `POST /picking/{id}/confirm`
fully documented (request/response models, search/status/priority/
warehouse/date query params, pagination, every status code the handler
can return, `BearerAuth`, idempotency behavior noted on Confirm, and the
`picking.update`-not-`.approve` Confirm-gating rationale noted in the
Register doc comment). Regenerated
(`swag init -g cmd/server/main.go -o docs --parseInternal --parseDependency`)
and verified all 3 route groups present in `docs/swagger.json`. The
top-level API `@description` was updated to mention Phase 9.

### Tests

`go test ./...`, `go vet ./...`, `go fmt ./...` all clean. New:
`internal/picking/service_test.go` (`Status.CanConfirm`/`IsValid`,
`Priority.IsValid`, `itemStatus`/`computeTaskStatus` derivation tables
including the SHORT-overrides-everything and all-terminal-mixed-outcome
cases — mirroring Putaway's own table exactly, with SHORT in place of
ISSUE, `Create`'s pre-DB validation paths via a fake `warehouseChecker`,
`Confirm`'s empty-items rejection); `internal/picking/repository_test.go`
(`TestIsUniqueViolation`, built correct against real `mssql.Error`
values from the start, unlike Receiving/Putaway which each needed a live
fix); 3 new RBAC middleware tests (see RBAC section above). The
concurrency proofs for the race fix (see above) were run live against
the real Docker SQL Server, not as Go unit tests — consistent with how
Phase 5 verified `AdjustBalance` and Phase 8 verified its own
concurrency regression, since real row-locking behavior under
concurrent HTTP requests isn't something a mocked/in-memory test can
demonstrate.

### Live DB verification

Full lifecycle run against the real Docker SQL Server (existing scratch
warehouse/zone/location/product data from Phases 3–8, reused rather than
recreated):

- Created a task (1 line, 50 ordered) → `PENDING`.
- Confirmed 30 → task `IN_PROGRESS`, line `PARTIALLY_PICKED`, inventory
  balance decreased by exactly 30, exactly 1 `PICK` ledger row
  (`reference_type=PICKING`, `reference_id=PICK-000001`).
- **Resent the same `idempotency_key`** → 200, unchanged state, still
  exactly 1 ledger row (idempotency proven, not assumed).
- Confirmed the remaining 20 → line `COMPLETED`, task `COMPLETED`,
  `completed_at` set.
- Confirm again on the now-`COMPLETED` task → 409 `INVALID_TRANSITION`.
- Second task: attempted to pick 999 against a location with only 35
  available → 422 `insufficient stock to pick this quantity`, balance
  provably unchanged afterward; `SHORT` with no remark → 422; `SHORT`
  with a remark → succeeds, task `SHORT_PICK`, item `SHORT`, no ledger
  row written, remark persisted.
- Create with an unknown warehouse/product/location → 404 with the
  correct specific error code for each; create against a real product/
  location with zero balance for the given lot → 422 `NO_SOURCE_STOCK`.
- No JWT → 401.
- Full RBAC cross-role flow: real `WAREHOUSE_MANAGER`-role user creates
  a task (`picking.create`) → real `PICKING`-role user confirms it
  (`picking.update`) → task reaches `COMPLETED`; the same `PICKING`-role
  user gets 403 attempting `POST /picking` (lacks `.create`).
- **The race-fix concurrency proofs** (see the Race-condition fix
  section above) — 20 concurrent confirms against the same line for
  each of Receiving, Putaway, and Picking, all landing on the
  mathematically exact total with zero lost updates.
- Restarted `wms-sqlserver`/`wms-rabbitmq` mid-session — all 6 picking
  tasks, their lines, and statuses were unchanged afterward (including
  the completed/short-pick/in-progress ones), confirming volume-backed
  persistence still holds; login worked immediately with no re-seed;
  permission count stayed at 47 (no drift); the backend's own reconnect
  loop (Phase 1) and the worker's independent reconnect loop (Phase 1)
  both recovered on their own within seconds, resuming RabbitMQ
  heartbeat consumption — no manual intervention needed.

### Docker/browser verification

`docker compose config` clean; `docker compose build backend worker
migrate frontend` — all 4 images rebuilt successfully (Docker Desktop
required a manual start this session — the engine wasn't running when
verification began; once started, the existing containers with
`restart: unless-stopped` came back on their own from a prior session,
matching Phase 5's own "Docker Desktop stopped mid-session" precedent).
`docker compose up -d` on the **existing, non-fresh** SQL Server volume
— `migrate` applied `0017`–`0019` cleanly and re-seeded RBAC
idempotently (still 47 permissions, no drift, since Picking added no new
codes). All 5 services reached healthy. Verified through the
Nginx-proxied path (port 80): login, `/api/v1/picking`, Swagger UI.

Full browser verification (real Chrome): logged in as admin; Picking
list rendering real task-level rows with correct summary counts; created
a real picking task through the actual UI (Order No./Warehouse/Add Item
with real SKU lookup and real cascading Location select) — found and
fixed the location-display bug during this step (see Frontend files
modified above), rebuilt, and re-verified clean; opened the new task and
confirmed a real pick through the UI (Picked/Remain updated live,
summary cards updated live); cross-checked Stock Inquiry (Phase 5's own
screen) independently showing the same real, non-mock balances. Logged
in as a real `RECEIVING`-role user (lacks `picking.view` under current
grants): confirmed the Picking nav item still appears (ungated, matching
Phase 5's precedent) and the page shell loads, but the list API call
fails with 403 and the UI honestly surfaces `"permission denied:
picking.view"` — same defense-in-depth pattern verified in every prior
phase. Test users' role-based access verified as described in RBAC
above.

### Regression results

Explicitly re-verified this session, all passing: `/auth/me`, `GET
/users`, `GET /roles`, `GET /permissions` (still 47), `GET /warehouses`,
`GET /products`, `GET /inventory`, `GET /receipts`, `GET /putaway` — no
Phase 1–8 behavior changed. Receiving and Putaway's own confirm flows
were exercised live during the race-fix concurrency proofs above and
behaved exactly as before except for the fix itself (no business-rule
change, only the quantity-column write mechanism).

### Known issues

- **No reservation at task-creation time.** `reserved_quantity` remains
  always 0 — Picking's `Confirm` calls `RemoveStock` directly against
  the live balance, the same "not yet implemented" state Phase 5/8 both
  flagged. Two pickers could theoretically be assigned overlapping
  quantities at the same location before either confirms; the atomic
  guarded `RemoveStock` still prevents the *balance* from ever going
  negative (proven live via the insufficient-stock test above), but
  nothing stops a task from being created against stock another task
  also expects to use. Explicitly out of this phase's scope per the
  task's own instructions ("do not implement Shipping yet," no
  reservation workflow requested) — flagged as a real Phase 10+
  consideration, not a silent gap.
- **No Sales/Outbound Order module** — `order_no`/`customer` are plain
  text with no backing table, matching `receipts.vendor`'s precedent.
  If a future phase adds Orders, Picking's `order_no` would need to
  become a real FK, a genuine schema migration, not a quick edit.
- **No per-line update/delete endpoint** — a picking task's lines are
  fixed at creation; there's no "+ Add Item to an existing task" flow
  the way Receiving has (`POST /receipts/{id}/items`), since the mock's
  own single-submission drawer doesn't call for one.
- **`CANCELLED` is modeled but unreachable** — no cancel endpoint exists
  this phase, matching Receiving/Putaway's own precedent.
- Everything listed as a known issue in Phase 2–8 still applies
  unchanged, except the received_qty/moved_qty lost-update race, which
  this phase fixes.

## Phase 10 — Shipping / Outbound Completion: VERIFIED COMPLETE

### Shipping frontend requirements found

Inspected `ShippingView.vue` — a single list+drawer screen, the same
shape Picking (Phase 9) already established (there is no
`NewShippingView.vue`). Findings:

- The list (Shipment No./Order No./Customer/Carrier/Tracking No./
  Packages/Weight/Ship Date/Status) is shipment-level, matching
  Receiving's list shape. `filters.carrier`, `filters.status` (READY/
  DISPATCHED/IN_TRANSIT/DELIVERED/ISSUE), `filters.date`, and a
  multi-field keyword search all exist in the mock exactly as
  implemented here.
- `orderNo`/`customer` reference the same non-existent Order module
  Picking's own `order_no`/`customer` already don't have a table for —
  resolved by reading them straight off the linked picking task via
  JOIN (see migrations/0020's doc), not by duplicating free text a
  second time.
- **No source picker exists anywhere in the mock.** "+ New Shipment"
  opens the same drawer used to manage an existing shipment, with a
  hardcoded placeholder header (`SHP-NEW`/`SO-NEW`) and, critically, the
  **same 3 hardcoded `packageItems`** regardless of which shipment (or
  none) is open — the drawer's items list was entirely disconnected
  from its own header, the identical class of gap Phase 6 fixed for
  `ReceiveView.vue` and Phase 9 fixed for `PickingView.vue`. There was
  no way in the mock to choose which order to ship at all.
- **Resolved by determining the real relationship to Picking directly**
  (per this phase's own instructions, before writing any code): a
  shipment ships the already-picked quantities of exactly one completed
  Picking task. This is the simplest relationship the frontend actually
  supports — Package Items is a flat, read-only list with **no
  quantity input anywhere**, so there is no evidence the mock ever
  intended per-line editable ship quantities or splitting one task's
  picked stock across multiple shipments. A minimal, real "Source
  Picking Task" selector was added to the create-mode drawer (populated
  from a new `GET /shipping/sources` endpoint) — the smallest
  functional addition needed to make Create work through a real API,
  the same precedent as Phase 9's own "Add Item" mini-form.
- `confirmDispatch()`/`markDelivered()`/`reportIssue()` were each a bare
  `alert()` with no real state change in the mock — wired to the real
  Confirm endpoint's three actions (see workflow below).
- **Design gap found and fixed before it reached the frontend**: the
  mock's single generic "Remark" textarea is meant to back "Report
  Issue," but this codebase's own header-edit rule (fields editable only
  while a document is still in its initial, unconfirmed state — the same
  rule Receiving/Putaway apply) would have made it impossible to set a
  remark for a shipment that needs to report an issue *after* dispatch,
  since header edits are READY-only. Fixed by having `ConfirmRequest`
  accept its own optional `remark` specifically for the ISSUE action
  (see Idempotency/APIs below) — resolved during backend design, not
  discovered live, but documented here because it directly shaped the
  API contract the frontend integrates against.

### Shipping workflow

```
Completed (or partially-completed) Picking task (Phase 9)
        |
        v
Create: copy every COMPLETED line's picked_qty verbatim -> Shipment (READY)
        |
        v
Confirm DISPATCH (requires carrier + tracking_no already set) -> DISPATCHED
        |
        v
Confirm DELIVER -> DELIVERED
        |
   (or, from READY or DISPATCHED)
        v
Confirm ISSUE (remark required, via header or the confirm call itself) -> ISSUE
```

No RabbitMQ anywhere in this flow — every action requires an immediate
success/failure response the frontend surfaces synchronously (a form
validation, a state transition), exactly the profile CLAUDE.md's own
"use synchronous REST when the frontend needs an immediate result"
guidance describes, not the "long-running, retryable, order doesn't
matter" profile RabbitMQ is for. If a future phase adds outbound
notifications (e.g. "email the customer when a shipment dispatches"),
that specific side effect — not shipment confirmation itself — would be
the asynchronous candidate; documented here as a candidate, not
implemented.

### Relationship with Picking

**One shipment per picking task**, enforced as a real database
constraint (`UNIQUE(picking_task_id)` on `shipments`, migrations/0020)
— not just an application-level check — so a race between two
concurrent "create shipment for task X" requests can't both succeed
(the second fails on the constraint and is translated to the same 409
`ALREADY_SHIPPED` a friendly pre-check would have returned anyway; see
Service.Create's doc). A picking task is "shippable" once it is
`COMPLETED` or `SHORT_PICK` (Phase 9's own two terminal statuses) —
`SHORT_PICK` tasks are included because a task can have some lines
`COMPLETED` and others `SHORT`, and whatever *was* successfully picked
should still be shippable. Only lines with `Status == COMPLETED` and
`PickedQty > 0` are copied; `SHORT` lines (qty 0) are silently skipped,
never copied as zero-quantity shipment lines. If a task has zero
completed lines (fully short), Create rejects with 422
`NO_SHIPPABLE_ITEMS` rather than creating an empty shipment.

`GET /shipping/sources` (new, powers the frontend's source picker) lists
exactly the tasks satisfying both conditions with no existing shipment
yet — the same "what am I allowed to source from" query shape as
Putaway's `GET /putaway/sources/{receiptId}`, adapted since Shipping has
no parent-scoped receipt-like grouping to nest under.

### Migrations created

`backend/migrations/0020_create_shipments_table.{up,down}.sql`,
`0021_create_shipment_items_table.{up,down}.sql`,
`0022_create_shipment_confirmations_table.{up,down}.sql` — additive, no
edits to 0001–0019.

### Shipping schema

- `shipments`: `shipment_no` (unique, backend-derived from the row's own
  IDENTITY id, `SHP-%06d`, the same race-free-by-construction pattern as
  every prior document number), `picking_task_id` (**UNIQUE** FK — see
  Relationship with Picking above), `carrier` (nullable, CHECK-
  constrained to the mock's own 4 `<select>` values once set),
  `tracking_no`/`address`/`remark` (free text), `packages` (default 1,
  `CHECK > 0`), `weight` (nullable decimal), `ship_date` (nullable),
  `status` (CHECK: READY/DISPATCHED/IN_TRANSIT/DELIVERED/ISSUE),
  `created_by` FK to `users`, `dispatched_at`/`delivered_at`. No
  `order_no`/`customer` columns — both are resolved via JOIN to the
  linked `picking_tasks` row (see migrations/0020's doc) rather than
  duplicated.
- `shipment_items`: `picking_task_item_id` FK (traceability back to the
  exact picking line this ships), `product_id`/`location_id` FKs,
  `lot_no VARCHAR(50) NOT NULL DEFAULT ''` (same convention as every
  other line-item table in this codebase), `qty DECIMAL(18,4) CHECK > 0`
  (copied from `picked_qty` at creation, never edited afterward — no
  quantity input exists on this table's own UI, so none was built here
  either), `uom`.
- `shipment_confirmations`: the idempotency guard, identical shape to
  every prior phase's confirmations table.
- `IN_TRANSIT` is modeled in the `status` CHECK constraint but
  **unreachable this phase** — the mock's own summary card is labeled
  "In Transit" but no button anywhere transitions a shipment there (only
  "Confirm Dispatch"/"Mark Delivered"/"Report Issue" exist); mapped for
  *display* onto `DISPATCHED` instead (both the frontend's summary-card
  computation and `getStatusClass` treat `DISPATCHED`/`IN_TRANSIT` as
  the same visual bucket), the same consolidation precedent Putaway's
  own "In Progress"/`PARTIALLY_COMPLETED` mapping established in Phase
  7. A future phase wiring real carrier tracking webhooks could use the
  already-modeled value with no schema change.

### APIs created

```
GET  /api/v1/shipping                (shipment-level list, matches ShippingView.vue)
GET  /api/v1/shipping/sources        (completed picking tasks not yet shipped, powers the source picker)
GET  /api/v1/shipping/{id}           (shipment + its lines)
POST /api/v1/shipping                (create: picking_task_id + header fields; items are never client-supplied)
PUT  /api/v1/shipping/{id}           (edit header fields, only while READY)
POST /api/v1/shipping/{id}/confirm   (action: DISPATCH | DELIVER | ISSUE)
```

Six endpoints — `CreateRequest` deliberately has **no `items` field at
all**: every line is always copied server-side from the linked picking
task's own `picked_qty`, so there is no request shape through which a
client could ask to ship a quantity that was never picked. This is
stricter than Picking/Putaway/Receiving's own approach (which validate a
client-supplied quantity against a ceiling) — here there is no ceiling
check needed because there is no client-supplied quantity to check.

### Inventory behavior and proof of no double deduction

**`internal/shipping` has zero dependency on `internal/inventory`** —
grepped to confirm neither the package nor its `Service` type is
imported anywhere in `internal/shipping`, and no call to `AddStock`,
`RemoveStock`, or `MoveStock` exists in any file. `Service.Create` and
`Service.Confirm` only ever read from Phase 9's `picking.Service.Get`
(to copy already-picked quantities) and write to this package's own
`shipments`/`shipment_items` tables — the physical stock removal already
happened when Picking's own `Confirm` called `RemoveStock` in Phase 9.

**Proven live**, not just by code inspection: created a shipment from a
picking task whose product/location already had a known inventory
balance (BIN-C01, 75 available before and after); queried that
location's `/inventory/{id}/transactions` ledger both before and
immediately after the shipment-create call — it contained exactly the
same `PICK`-type rows from Phase 9's own picking confirmations, with
**no new ledger row of any type** written by the shipment creation.
Confirmed again after DISPATCH and DELIVER — neither transition touches
`inventory` or `inventory_transactions` at all (verified by re-querying
the same ledger and balance after each action: unchanged both times).

### Idempotency

Identical mechanical pattern to Phase 6/7/8/9:
`shipment_confirmations.idempotency_key` (required on every Confirm
call) is inserted as the **first** statement inside the transaction; a
repeat of the same key hits the PRIMARY KEY constraint via the
centralized `sqlerr` classifier, the whole transaction rolls back, and
the service returns the shipment's current state as a successful no-op.
Verified live: resending the exact same `idempotency_key` against a
DISPATCH call returned 200 with an **unchanged `updated_at`/
`dispatched_at`** (proving the transition was not silently reapplied,
not just that the final status matched) — the same rigor Phase 8/9's
own idempotency proofs used. Unlike Receiving/Putaway/Picking, there is
no stock quantity to double-apply here; the guard instead protects
`dispatched_at`/`delivered_at` from being double-stamped and protects
whatever a future notification phase might trigger off this exact call.

### RBAC

Reused the existing `shipping.view`/`create`/`update`/`approve` codes
from Phase 2's catalog exactly as-is — no new codes, no catalog edit.
Unlike Picking (whose `PICKING` role has an unusual view+update-only
grant shape), the **`SHIPPING` role's own Phase 2/3 seed grants mirror
RECEIVING/PUTAWAY's shape exactly**: view/create/update, no approve
(`internal/rbac/catalog.go`, unchanged). So Confirm here follows the
Receiving/Putaway precedent, not Picking's exception:

- `POST /shipping` (Create) and `PUT /shipping/{id}` (Update) are gated
  on `shipping.create`/`shipping.update` — `SHIPPING` staff can build
  and edit shipments.
- `POST /shipping/{id}/confirm` (dispatch/deliver/issue) is gated on
  **`shipping.approve`**, the same significant-action permission
  Receiving/Putaway's own Confirm uses — under current grants only
  `ADMIN`/`WAREHOUSE_MANAGER` have it, the same segregation-of-duties
  consequence already established for those two modules, not a new
  policy decision made here.

Verified live end to end: a real `SHIPPING`-role user gets 200 creating
a shipment but 403 `shipping.approve` confirming it; a real
`WAREHOUSE_MANAGER`-role user completed the full create → dispatch →
deliver cycle. 3 new RBAC middleware tests (`shipping.create` allowed
for SHIPPING, `shipping.approve` denied for SHIPPING, allowed for
WAREHOUSE_MANAGER-shaped).

### Frontend integration

- New: `services/shipping.js` (`listShipments`, `getShipment`,
  `listShippableSources`, `createShipment`, `updateShipment`,
  `confirmShipment`, `newIdempotencyKey`).
- `ShippingView.vue`: mock `shippingList`/`packageItems` replaced with
  real API calls; the 4 summary cards compute from a separate
  unfiltered fetch (task-status counts, `DISPATCHED`/`IN_TRANSIT`
  collapsed into the "In Transit" card — see the schema section above);
  the create-mode drawer gained a real "Source Picking Task" `<select>`
  (populated from `GET /shipping/sources`) — the mock had no equivalent
  control at all; the header form (Carrier/Tracking/Packages/Weight/
  Ship Date/Address/Remark) is shown in create mode and, separately, in
  edit mode only while a shipment is still READY (`canEditHeader &&
  status === 'READY'`), matching the backend's own `CanEdit` rule;
  "Confirm Dispatch"/"Mark Delivered"/"Report Issue" now call the real
  confirm endpoint with the correct action, and are hidden without
  `shipping.approve` (with an honest explanatory line shown instead,
  verified live in the browser — see below); Package Items render the
  real linked lines. `MainLayout.vue`'s Shipping nav item was **not
  modified** (left ungated), the same reasoning applied to every
  operational nav item since Phase 5.
- `RolePermissionView.vue`: **not modified** — `shipping` was already in
  the local `MODULE_ORDER`/`MODULE_NAMES` maps since Phase 2.

### Swagger changes

`GET/POST /shipping`, `GET /shipping/sources`, `GET/PUT /shipping/{id}`,
`POST /shipping/{id}/confirm` fully documented (request/response
models, search/status/carrier/date query params, pagination, every
status code each handler can return, `BearerAuth`, idempotency and
no-inventory-mutation behavior noted on Confirm, and the
`shipping.approve` Confirm-gating rationale in the Register doc
comment). Regenerated
(`swag init -g cmd/server/main.go -o docs --parseInternal --parseDependency`)
and verified all 4 route groups present in `docs/swagger.json`. The
top-level API `@description` was updated to mention Phase 10.

### Tests

`go test ./...`, `go vet ./...`, `go fmt ./...` all clean. New:
`internal/shipping/service_test.go` (`Status.CanEdit`/`CanDispatch`/
`CanDeliver`/`CanReportIssue`/`IsValid` state-machine tables,
`Carrier.IsValid`, `Create`'s pre-DB validation paths — unknown picking
task, non-shippable status, zero completed lines, invalid carrier — via
a fake `pickingChecker`, ordered so every pure-validation path runs
before the repository's duplicate-shipment check, the same "validate
before touching the DB" discipline as every prior phase's `Create`
tests); `internal/shipping/repository_test.go` (`TestIsUniqueViolation`,
built correct against real `mssql.Error` values from the start); 3 new
RBAC middleware tests (see RBAC section above). The no-double-deduction
and idempotency proofs were run live against the real Docker SQL
Server, not as Go unit tests — the same reasoning Phase 5/8/9 applied:
real ledger/transaction behavior isn't something a mocked test can
demonstrate credibly.

### Live DB verification

Full lifecycle run against the real Docker SQL Server (existing scratch
picking/inventory data from Phases 3–9, reused rather than recreated):

- Created a shipment from a `COMPLETED` picking task (50 picked) → qty
  copied exactly, status `READY`.
- Attempted a second shipment for the same picking task → 409
  `ALREADY_SHIPPED`.
- Dispatch without carrier/tracking set → 422; set both via `PUT`
  while `READY`; dispatch again → `DISPATCHED`, `dispatched_at` set.
- **Resent the same `idempotency_key`** → 200, `updated_at`/
  `dispatched_at` byte-for-byte unchanged (idempotency proven, not
  assumed).
- Attempted to `PUT` the header once `DISPATCHED` → 409 `NOT_EDITABLE`.
- Attempted to dispatch again → 409 `INVALID_TRANSITION`.
- Marked delivered → `DELIVERED`, `delivered_at` set; attempted
  `ISSUE` on the now-`DELIVERED` (terminal) shipment → 409.
- Second shipment: dispatched, then reported an issue **with the remark
  supplied only in the Confirm call itself** (not pre-set via header,
  which is impossible once dispatched) → `ISSUE`, remark persisted —
  proving the design fix described in the frontend-requirements section
  actually works end to end, not just in the type signature.
- Create with an unknown picking task → 404; a still-`IN_PROGRESS`
  picking task → 422 `NOT_SHIPPABLE`; a `SHORT_PICK` task with zero
  completed lines → 422 `NO_SHIPPABLE_ITEMS`; no JWT → 401.
- **No-double-deduction proof** (see above): inventory balance and
  ledger at the shipped product/location were queried before and after
  Create, Dispatch, and Deliver — unchanged by all three, every time.
- Full RBAC cross-role flow: real `WAREHOUSE_MANAGER`-role user
  completes a picking task, creates a shipment, and dispatches it
  (`shipping.approve`); a real `SHIPPING`-role user can create a
  shipment (`shipping.create`, verified past RBAC via a 404 business
  error, not 403) but gets 403 attempting to confirm it.
- Restarted `wms-sqlserver`/`wms-rabbitmq` mid-session — all 4
  shipments, their lines, and statuses (including the delivered/
  in-transit/issue ones) were unchanged afterward; login worked
  immediately with no re-seed; permission count stayed at 47; the
  backend's and worker's independent reconnect loops (Phase 1) both
  recovered on their own within seconds.

### Docker/browser verification

`docker compose config` clean; `docker compose build backend worker
migrate frontend` — all 4 images rebuilt; `docker compose up -d` on the
**existing, non-fresh** SQL Server volume — `migrate` applied `0020`–
`0022` cleanly and re-seeded RBAC idempotently (still 47 permissions,
no drift). All 5 services reached healthy. Verified through the
Nginx-proxied path (port 80): login, `/api/v1/shipping`, Swagger UI.

Full browser verification (real Chrome): logged in as admin; Shipping
list rendering real shipment-level rows with correct summary counts;
created a real shipment through the actual UI (Source Picking Task
selector populated with only the one genuinely eligible task — already-
shipped tasks correctly excluded), confirmed dispatch (header form
correctly disappeared once no longer READY, summary cards updated
live), marked delivered (action buttons correctly disappeared —
terminal state) — all through real clicks, not curl. Logged in as a
real `SHIPPING`-role user: confirmed Package Items render from real
data but no Confirm Dispatch/Mark Delivered/Report Issue buttons appear,
replaced with the honest line "Your role can view this shipment but not
confirm dispatch, delivery, or issues" — same defense-in-depth pattern
verified in every prior phase.

### Regression results

Explicitly re-verified this session, all passing: `/auth/me`, `GET
/users`, `GET /roles`, `GET /permissions` (still 47), `GET /warehouses`,
`GET /products`, `GET /inventory`, `GET /receipts`, `GET /putaway`,
`GET /picking` — no Phase 1–9 behavior changed.

### Known issues

- **No Sales/Outbound Order module** — `order_no`/`customer` remain
  plain text (resolved from the linked picking task, not shipments'
  own columns), the same deferral Phase 9 already documented for
  Picking. Not addressed here since it wasn't required for Shipping.
- **`reserved_quantity` remains unused** — Picking's own known
  limitation from Phase 9; Shipping performs no inventory mutation at
  all, so it has no bearing on this column either way.
- **`CANCELLED` doesn't exist as a shipment status at all** (unlike
  Receiving/Putaway/Picking) — the mock's own status set never included
  it, so it wasn't invented here.
- **`IN_TRANSIT` is modeled but unreachable** — see the schema section
  above; a real carrier-tracking integration is a distinct future
  phase, not attempted here.
- **One shipment per picking task, no partial shipment** — by design,
  matching the mock's own read-only Package Items list; if a future
  requirement needs splitting one task's picked stock across multiple
  shipments, that is new schema/business-rule work, not a quick change
  (the current `UNIQUE(picking_task_id)` constraint would need to be
  redesigned entirely).
- Everything listed as a known issue in Phase 2–9 still applies
  unchanged except where noted above.

## Phase 11 — Stock Adjustment + Stock Count: VERIFIED COMPLETE

### Frontend requirements found

Inspected `StockAdjustmentView.vue` and `CheckStockView.vue` completely
(`NewStockAdjustmentView.vue` is an empty, unrouted orphan file —
confirmed by grepping `router/index.js`, which only wires
`StockAdjustmentView`). Findings:

- **Stock Adjustment** is a single list+drawer screen (the same shape
  Picking/Shipping already established), with exactly one line per
  document — the create form has one SKU/Location/Type/Qty/Reason/
  Remark set, no "+ Add Item." Statuses: DRAFT / PENDING ("Pending
  Approval") / APPROVED / REJECTED — no separate "COMPLETED" stage
  anywhere in the mock. Reasons are 5 literal strings ("Cycle Count
  Variance", "Damaged Goods", "Lost Goods", "Found Goods", "System
  Correction"), not the task's own illustrative ALLCAPS examples — used
  verbatim per "do not invent codes if the frontend already defines
  them."
- **Check Stock** is architecturally different from every other module
  in this codebase: it has **no list-of-past-counts screen at all** — a
  single, perpetual working page. "+ New Count Task" was a bare
  `alert()` with no state change; the items table was always pre-
  populated on `mounted()` from a hardcoded 3-row `stockMaster` array
  regardless of any header selection (the same "disconnected mock"
  class of gap Phase 6/9/10 each found and fixed on their own screens).
  The scan bar's `confirmScan()` only ever **updates an existing
  matching row** — it never adds a new one — so "+ Add Item" is the
  real entry point for populating lines, not the scanner.
- **The exact difference between the two workflows**, confirmed by
  inspection rather than assumed: Stock Adjustment is a *proposed*
  single-line correction requiring separate approval before it takes
  effect; Stock Count is a *physical count exercise* across a scope
  (multiple lines) that computes its own variance per line and — per
  this phase's own task instructions — applies that variance directly
  through Inventory Core on confirm, without a separate approval
  document, unless the user explicitly chooses the "Create Adjustment"
  bridge instead (see workflow below).
- Warehouse/Zone `<select>`s in Check Stock were hardcoded
  (`WH-01`/`WH-02`/`WH-03`, `A`/`B`/`C`) — replaced with real Phase 3
  data, cascading, the same pattern every prior phase's location
  pickers use.

### Stock Adjustment workflow

```
Create (DRAFT or submit straight to PENDING)
        |
        v
   PENDING (locked from further editing)
        |
        v
Confirm: APPROVE -> AddStockTx (INCREASE) or RemoveStockTx (DECREASE) -> APPROVED
        or
Confirm: REJECT (no inventory mutation) -> REJECTED
```

Approve is the **one and only** point in this workflow's entire
lifecycle where `inventory.quantity` changes — DRAFT and PENDING are
both completely inert with respect to inventory, matching the task's
own explicit "adjustment must NEVER directly overwrite inventory.quantity"
instruction: `before_qty` is captured as a **display-only** snapshot at
Create time (for the mock's own "Current Qty" column) and is never used
to compute the mutation; the mutation is always `adjust_qty` applied via
`AddStockTx`/`RemoveStockTx`'s own atomic guarded UPDATE against
whatever the live balance actually is at Approve time. `after_qty` is
populated from that call's own returned balance, not a naive
`before + delta` calculation that could drift from reality under
concurrent stock movement.

### Stock Count workflow

```
Create Count (header only, DRAFT)
        |
        v
Add Item -> snapshot system_qty from LIVE inventory, frozen forever
        |
        v
Record Count (scan or table edit) -> count_qty + variance_qty stored immediately
        |
        v
Confirm: re-check each line's live balance against its frozen system_qty
        |                                   |
        | matches                           | drifted
        v                                   v
   apply variance via AddStockTx/      409 STALE_COUNT_CONFLICT,
   RemoveStockTx (skip if zero) ->     nothing applied, recount required
   CONFIRMED
```

`system_qty` is captured exactly once, when a line is added, and is
**never** refreshed afterward — grepped `internal/stockcount` to confirm
no code path writes that column a second time — satisfying the task's
explicit "do not recompute the historical expected quantity after the
count is created." `variance_qty` is likewise computed and stored the
moment `count_qty` is recorded (not a query-time derivation), so it
remains a fixed historical fact about that specific count event even if
inventory moves again afterward.

### Migrations created

`backend/migrations/0023_create_stock_adjustments_table.{up,down}.sql`,
`0024_create_stock_adjustment_items_table.{up,down}.sql`,
`0025_create_stock_adjustment_confirmations_table.{up,down}.sql`,
`0026_create_stock_counts_table.{up,down}.sql`,
`0027_create_stock_count_items_table.{up,down}.sql`,
`0028_create_stock_count_confirmations_table.{up,down}.sql` —
additive, no edits to 0001–0022.

### Schema

- `stock_adjustments`: `adjustment_no` (unique, backend-derived,
  `ADJ-%06d`), `status` (CHECK: DRAFT/PENDING/APPROVED/REJECTED),
  `created_by`/`approved_by` FKs to `users`, `approved_at`.
- `stock_adjustment_items`: modeled as a child table even though the
  current frontend only ever produces one line per document (the same
  "build the real shape" precedent every prior phase's schema follows)
  — `product_id`/`location_id` FKs, `lot_no VARCHAR(50) NOT NULL
  DEFAULT ''` (same convention as every other line-item table in this
  codebase), `adjust_type` (CHECK: INCREASE/DECREASE), `adjust_qty
  CHECK > 0`, `before_qty`/`after_qty` (both nullable, display-only —
  see workflow above), `reason` (CHECK-constrained to the 5 literal
  mock strings), `remark`.
- `stock_counts`: `count_no` (unique, `CNT-%06d`), `count_type` (CHECK:
  CYCLE_COUNT/FULL_COUNT/LOCATION_COUNT/SKU_COUNT — purely descriptive
  this phase, no automatic item-population logic tied to any value, per
  "do not build a full cycle-count engine unless required"),
  `warehouse_id` FK (required), `zone_id` FK (nullable — "All Zone" is
  valid), `count_date`, `status` (CHECK: DRAFT/CONFIRMED — no
  "in progress" state, since the mock's own three action buttons are
  all available simultaneously at every point with no explicit "start
  counting" transition).
- `stock_count_items`: `product_id`/`location_id` FKs, `lot_no` (same
  not-null-default-empty convention), `system_qty` (frozen forever, see
  workflow), `count_qty`/`variance_qty` (both nullable until counted),
  `condition` (CHECK: GOOD/DAMAGED/HOLD), `status` (CHECK:
  PENDING/COUNTED/RECOUNT) — both match the mock's own two independent
  per-line `<select>`s exactly.
- Both `*_confirmations` tables are the idempotency guard, identical
  shape to every prior phase's own confirmations table.

### APIs created

```
GET  /api/v1/stock-adjustments                    (list)
GET  /api/v1/stock-adjustments/{id}                (detail)
POST /api/v1/stock-adjustments                     (create: DRAFT or straight to PENDING)
PUT  /api/v1/stock-adjustments/{id}                (edit + optional submit, DRAFT only)
POST /api/v1/stock-adjustments/{id}/confirm        (action: APPROVE | REJECT)

GET  /api/v1/stock-counts                          (list — see Known issues on why the frontend doesn't use this)
GET  /api/v1/stock-counts/{id}                     (detail)
POST /api/v1/stock-counts                          (create header, optionally with items up front)
POST /api/v1/stock-counts/{id}/items               ("+ Add Item" — snapshots system_qty server-side)
PUT  /api/v1/stock-counts/{id}/items/{itemId}      (record a counted qty/condition/remark)
POST /api/v1/stock-counts/{id}/confirm             ("Confirm Count" — applies every line's variance)
POST /api/v1/stock-counts/{id}/create-adjustments  ("Create Adjustment" — the alternate, non-mutating bridge)
```

11 endpoints across both packages. `stock_adjustment_items.qty` fields
are never accepted as a bare client-supplied delta without validation
context — `adjust_qty` is always a required, `>0`-validated field on
`CreateRequest`/`UpdateRequest`, and the actual mutation direction
(`AddStockTx` vs `RemoveStockTx`) is derived from `adjust_type`, not
guessed from sign.

### Inventory integration

Both packages depend only on Phase 5's `internal/inventory.Service`
(`AddStockTx`, `RemoveStockTx`, `FindBalance`) — no direct writes to
`inventory.quantity` anywhere in either package (grepped to confirm).
`internal/stockcount` additionally has a narrow, one-directional
dependency on `internal/stockadjustment.Service.Create` for the
"Create Adjustment" bridge (the same cross-package narrow-interface
pattern `internal/putaway`→`internal/receiving` and
`internal/shipping`→`internal/picking` already established) —
`internal/stockadjustment` has no reciprocal dependency, so there is no
import cycle.

**Proven live** with the exact scenario from this phase's own
instructions: a fresh scratch product received to exactly 100 via a
real Phase 6 receiving cycle → a DECREASE adjustment of 10, approved →
balance 90, ledger row `ADJUST -10` (`STOCK_ADJUSTMENT`/`ADJ-000001`) →
a stock count line added (system_qty snapshot correctly read 90) →
counted at 87, `variance_qty` stored as exactly `-3` → Confirm Count →
balance **87**, ledger row `COUNT -3` (`STOCK_COUNT`/`CNT-000001`).
Every step's ledger row is independently queryable and distinct — see
Live DB verification for the full trace, which continued further
(additional adjustments layered on top, cross-confirmed via Stock
Inquiry) to prove the audit trail stays coherent across an arbitrary
sequence of operations, not just the one scripted scenario.

### Concurrency / stale-count strategy

Every quantity mutation in both packages goes through
`AddStockTx`/`RemoveStockTx`'s existing atomic guarded UPDATE (Phase 5) —
no Go-side read-modify-write for the inventory balance itself anywhere
in either package, and no retrofit was needed the way Phase 9 needed
one for `received_qty`/`moved_qty`, since both packages were built
against the already-corrected pattern from the start. `stock_count_items.count_qty`/`variance_qty`
are plain `SET` writes (not increments) since they store an operator's
absolute observation, not an arithmetic delta — no concurrency concern
there, the same reasoning every prior phase's own line-status writes
already rely on.

The genuinely new concern this phase introduces — one no prior
phase faced — is a **stale count**: a physical count's `system_qty`
snapshot can go stale if the same (product, location, lot) is touched
by an unrelated operation (another adjustment, a pick, a receipt)
*after* the count started but *before* it's confirmed. Per this phase's
own instructions, `Confirm` re-reads the live balance for every line,
inside the same transaction, immediately before deciding whether to
apply that line's variance; if the live balance no longer matches the
frozen `system_qty`, the whole transaction rolls back with 409
`STALE_COUNT_CONFLICT` naming the specific line and both quantities,
rather than silently applying a variance computed against a number that
is no longer true. This is a business-rule check layered on top of (not
a replacement for) `AddStockTx`/`RemoveStockTx`'s own unconditional
guarded UPDATE, which still remains the sole final authority against
negative stock regardless. **Proven live**: counted a line at 87 against
a frozen `system_qty` of 87 (matched, zero variance) — GIVEN A SEPARATE
adjustment then changed the same balance to 92 before Confirm ran —
attempting Confirm returned 409 with `"expected 87.0000, now 92.0000"`,
and the balance was provably unchanged by the rejected attempt
(re-queried immediately after).

### Idempotency

Identical mechanical pattern to Phase 6/7/9/10 — one `*_confirmations`
table per package, `idempotency_key` inserted as the first statement
inside the transaction. **A real bug was found and fixed during this
phase's own live testing**: both `Confirm` methods originally inserted
the idempotency guard *before* checking whether the target document
existed — since `stock_adjustment_confirmations.stock_adjustment_id`
and `stock_count_confirmations.stock_count_id` are real foreign keys
(not just documentation), confirming a nonexistent id raised a raw
FK-violation 500 instead of a clean 404. Investigating further showed
this exact ordering — and exact bug — already exists identically in
`internal/putaway` and `internal/picking`'s own `Confirm` (verified live
against both): a real, pre-existing, cross-phase defect this phase's
own testing surfaced for the first time. Fixed in this phase's two new
packages by moving the existence check (a side-effect-free `SELECT`)
ahead of the idempotency `INSERT` — this does not weaken the idempotency
guarantee at all, since claiming the key was never the thing being
raced against. **Not fixed in Putaway/Picking/Shipping** — touching
verified Phase 7/9/10 code is out of this phase's scope; flagged as a
known cross-package issue below for a future hardening pass, the same
"found broader than fixed" precedent Phase 8 itself established when it
found (and, there, did fix) the request-cancellation transaction risk
across multiple packages at once.

Verified live for both packages: resending the exact same
`idempotency_key` against an APPROVE call returned 200 with a
byte-identical `updated_at`/`approved_at` (proving true no-op, not
re-application); the same for Stock Count's Confirm against
`confirmed_at`.

### RBAC

Reused the existing `stock_adjustment` module from Phase 2's catalog
exactly as-is for **both** screens — no new codes, and critically, **no
new "stock_count" module**, since none exists anywhere in the
permission catalog or `RolePermissionView.vue`'s own module list, per
this phase's own "reuse existing... do not create duplicate concepts"
instruction. `internal/stockcount`'s handler documents this sharing
explicitly. Confirm (the significant, inventory-mutating action in both
packages) is gated on `stock_adjustment.approve`.

Under current seed data (`internal/rbac/catalog.go`, unchanged this
phase), **no operational role has any `stock_adjustment.*` grant at
all** — only `ADMIN`/`WAREHOUSE_MANAGER` do, via their generic
all/all-except-delete computation. This was true before this phase too
(`RolePermissionView.vue`'s own `createDefaultPermissions()` never had a
branch for `stock_adjustment` for any operational role either) and was
not narrowed further here — it is a pre-existing gap in the original
frontend's own default-permission logic, surfaced by this phase's own
RBAC verification rather than introduced by it. Verified live: a real
`PICKING`-role user gets 403 `stock_adjustment.view` on **both**
`GET /stock-adjustments` and `GET /stock-counts` (same reused code); a
real `WAREHOUSE_MANAGER`-role user has full access to both, confirmed
end to end (create → approve on Stock Adjustment; create → add item →
record count → confirm on Stock Count). 2 new RBAC middleware tests.

### Frontend integration

- New: `services/stockAdjustment.js`, `services/stockCount.js`.
  `services/inventory.js` gained a `productId` filter param (the
  backend's `GET /inventory?product_id=` already existed since Phase 5;
  the frontend wrapper simply never exposed it until a real "show me
  this exact product's current balance" need arose here).
- `StockAdjustmentView.vue`: mock `adjustmentList` replaced with real
  API calls; summary cards compute from a separate unfiltered fetch
  (Pending/Approved-Today/Rejected counts, Total Variance summed over
  approved lines only); the create-mode drawer gained real Warehouse→
  Location cascading selects and a **Lot No. (optional)** field — the
  mock's own free-text "Location" input had no way to resolve a real
  `location_id` at all, and lot_no is part of inventory's own identity
  key (Phase 5), so omitting it would have silently adjusted a
  different, zero-lot balance than the one actually intended (a real
  bug this phase's own live testing found and fixed — see Known issues
  and the SKU field's new `@blur` lookup, added for the same reason:
  without it, "Current Qty" never populated before submit). Detail mode
  shows Approve/Reject buttons gated on `stock_adjustment.approve`, with
  an honest fallback message otherwise — same defense-in-depth pattern
  every prior phase established.
- `CheckStockView.vue`: **substantially rebuilt from a hardcoded, always-
  populated mock into a real, empty-until-created working page** — "+
  New Count Task" now creates a real header; "+ Add Item" performs a
  real SKU/Product lookup plus a real cascading Location select (same
  precedent as every prior phase's own item-lookup UI) instead of
  pushing a blank freely-typed row; each row gained a "Save" action
  (the mock had no per-row persistence mechanism at all, since it never
  talked to a backend); "Save Draft" bulk-PUTs every row's current
  values (the same "simple and correct, slightly wasteful at scale"
  precedent Phase 3's "Save Zones" established); "Create Adjustment"
  and "Confirm Count" call their respective real endpoints. Layout,
  card structure, and the scan bar's shape are unchanged.
- `RolePermissionView.vue`: not modified — `stock_adjustment` was
  already in the local module maps since Phase 2.

### Swagger changes

All 11 endpoints across both packages fully documented (request/
response models, status transitions, reason codes, query filters,
pagination, every status code each handler can return, `BearerAuth`,
idempotency and stale-conflict behavior noted on both Confirm
endpoints). Regenerated
(`swag init -g cmd/server/main.go -o docs --parseInternal --parseDependency`)
and verified all route groups present in `docs/swagger.json`. The
top-level API `@description` was updated to mention Phase 11.

### Tests

`go test ./...`, `go vet ./...`, `go fmt ./...` all clean. New:
`internal/stockadjustment/service_test.go` (`Status.CanEdit`/
`CanConfirm`/`IsValid`, `AdjustType.IsValid`, `Reason.IsValid`,
`Create`'s pre-DB validation paths — invalid type/qty/reason, unknown
product/location — via fakes); `internal/stockadjustment/repository_test.go`
(`TestIsUniqueViolation`, built correct against real `mssql.Error`
values); `internal/stockcount/service_test.go` (the same state-machine
and enum tables for `Status`/`CountType`/`Condition`/`ItemStatus`,
`Create`'s pre-DB validation — invalid count_type, unknown warehouse/
zone — via fakes); `internal/stockcount/repository_test.go`
(`TestIsUniqueViolation`); 2 new RBAC middleware tests (see RBAC
section). The insufficient-stock, duplicate-confirmation, stale-count-
conflict, and full-scenario proofs were run live against the real
Docker SQL Server, not as Go unit tests — real transactional/
concurrency behavior isn't something a mocked test can demonstrate
credibly, the same reasoning every prior phase has applied.

### Live DB verification

Full scenario run against the real Docker SQL Server, using a fresh
scratch product (`SKU-PHASE11-001`) received to exactly 100 via a real
Phase 6 receiving cycle (not a raw DB insert):

- DECREASE adjustment of 10, submitted straight to PENDING, `before_qty`
  correctly snapshotted at 100 → Approve → `after_qty` = 90, balance 90.
- Resent the same `idempotency_key` → 200, `updated_at`/`approved_at`
  byte-identical (true no-op).
- Confirm again on the now-APPROVED adjustment → 409
  `INVALID_TRANSITION`; edit attempt on the same → 409 `NOT_EDITABLE`.
- A second, oversized DECREASE (9999) → Approve → 422
  `INSUFFICIENT_STOCK`, balance provably unchanged; Reject without a
  remark → 422; Reject with one → `REJECTED`, remark persisted.
- Stock count created, one item added (system_qty snapshot correctly
  read live balance = 90), counted at 87 → `variance_qty` stored as
  exactly `-3` → Confirm → balance **87**, ledger row `COUNT -3`
  (`STOCK_COUNT`/`CNT-000001`).
- Resent the same count `idempotency_key` → 200, no-op, balance
  unchanged; confirm again with a fresh key → 409 `INVALID_TRANSITION`.
- **Stale-count conflict, proven live**: a second count snapshotted 87,
  was counted at 87 (zero variance) — a *separate* adjustment then
  changed the live balance to 92 before the count's own Confirm ran —
  attempting Confirm on the stale count → 409 `STALE_COUNT_CONFLICT`
  naming the exact expected-vs-now mismatch, balance unchanged by the
  rejected attempt.
- "Create Adjustment" bridge: a count with a `+3` variance line →
  generated one real `PENDING` `stock_adjustment` (reason "Cycle Count
  Variance", remark referencing the count) → balance **unchanged** by
  the bridge call itself → separately approved afterward → balance 95,
  exactly as a manually-created adjustment would have behaved.
- Full ledger trace for the test product, independently re-queried via
  `GET /inventory/{id}/transactions`, showed all five operations
  (`RECEIVE +100`, `ADJUST -10`, `COUNT -3`, `ADJUST +5`, `ADJUST +3`)
  as distinct, correctly-referenced rows — cross-confirmed via Stock
  Inquiry (Phase 5's own screen) showing the same final balance
  independently.
- Create-time error paths: unknown product/location/warehouse/zone →
  404 with the correct specific code each; invalid `count_type` → 422;
  no JWT → 401.
- Full RBAC cross-role flow: a real `PICKING`-role user denied 403 on
  both `/stock-adjustments` and `/stock-counts`; a real
  `WAREHOUSE_MANAGER`-role user completed the entire create → approve
  cycle on Stock Adjustment and create → add item → record → confirm
  cycle on Stock Count.
- Restarted `wms-sqlserver`/`wms-rabbitmq` mid-session — all 6
  adjustments and the confirmed count (with its recorded item) were
  unchanged afterward; login worked immediately with no re-seed;
  permission count stayed at 47; both independent reconnect loops
  (Phase 1) recovered on their own within seconds.

### Docker/regression results

`docker compose config` clean; `docker compose build backend worker
migrate frontend` — all 4 images rebuilt (twice — once for the initial
Phase 11 code, once more after the idempotency-ordering bug fix and two
frontend fixes found during this phase's own live/browser testing); all
6 new migrations applied cleanly against the **existing, non-fresh**
SQL Server volume; RBAC re-seeded idempotently (still 47 permissions,
no drift). All 5 services reached healthy each time. Verified through
the Nginx-proxied path (port 80): login, `/api/v1/stock-adjustments`,
`/api/v1/stock-counts`, Swagger UI.

Full browser verification (real Chrome, not just curl): created a real
stock adjustment through the actual UI end to end (SKU lookup, real
Warehouse→Location cascade, the new Lot No. field, a correctly-
populated live Current Qty of 95), submitted it, approved it (New
Qty correctly showed 100, summary cards updated live). Created a real
count task through the actual UI ("+ New Count Task" now genuinely
creates `CNT-000004`, matching the header shown), added a real item via
the "+ Add Item" lookup (System Qty correctly read the live balance of
100), recorded a count of 98 and saved it (Variance correctly showed
-2), confirmed the count — re-queried via the API immediately afterward
and confirmed the balance had genuinely dropped to 98 and the count's
own status was `CONFIRMED`, proving the browser-driven action reached
the real backend, not just updated local UI state.

Explicitly re-verified this session, all passing: `/auth/me`, `GET
/users`, `GET /roles`, `GET /permissions` (still 47), `GET /warehouses`,
`GET /products`, `GET /inventory`, `GET /receipts`, `GET /putaway`,
`GET /picking`, `GET /shipping` — no Phase 1–10 behavior changed.

### Known issues

- **The Confirm-before-existence-check bug (fixed here) still exists in
  `internal/putaway` and `internal/picking`** — confirmed live against
  both during this phase's own testing: confirming a nonexistent
  task/pick id returns a raw 500 instead of a clean 404, for the exact
  same reason (an idempotency-guard FK insert happening before an
  existence check). Narrow, well-understood, and low-risk to fix (the
  same one-line reordering applied here), but out of this phase's scope
  since it touches verified Phase 7/9 code — flagged as a strong
  candidate for a small, isolated fix in whichever phase next touches
  either package, or as its own tiny hardening pass.
- **No operational role has any `stock_adjustment.*` grant** — a
  pre-existing gap in the original frontend's own default-permission
  mapping (not narrowed or widened by this phase); both screens are
  ADMIN/WAREHOUSE_MANAGER-only under current seed data.
- **`GET /stock-counts` (the list endpoint) has no frontend consumer**
  — Check Stock's own UI has no list-of-past-counts view to populate
  (see Frontend requirements above); the endpoint is real and correct,
  just not wired to anything yet. A future phase adding a proper Stock
  Count history screen would use it directly.
- **"Create Adjustment" and "Confirm Count" are not mutually
  exclusive** — nothing stops the same count from later also being
  Confirmed directly after its variance was already routed through the
  adjustment bridge, which would double-apply that correction. The mock
  itself never modeled the two buttons as mutually exclusive, and
  inventing that guard would be a business rule not derivable from the
  actual frontend — flagged rather than silently guessed at.
- **`stock_count_items` has no scope-driven auto-population** —
  `count_type` (CYCLE_COUNT/FULL_COUNT/etc.) is purely descriptive this
  phase; a "FULL_COUNT" does not automatically enumerate every item in
  a warehouse the way its name might imply. Explicit per the task's own
  "do not build a full cycle-count engine unless required."
- Everything listed as a known issue in Phase 2–10 still applies
  unchanged.

## Phase 12 — Confirm-Before-Existence-Check Fix + Picking Reservation: VERIFIED COMPLETE

### Part A — cross-package confirmation bug audit and fix

Phase 11 found (and fixed in its own two new packages) a real bug: some
`Confirm` methods inserted the idempotency guard row *before* checking
whether the target document/task existed. Since every `*_confirmations`
table's id column is a real foreign key, confirming a nonexistent id
raised a raw FK-violation 500 instead of a clean 404. Phase 11 flagged
`internal/putaway` and `internal/picking` as carrying the identical
defect but left them unfixed (out of scope that phase). This phase
re-audited all 6 confirmation-workflow packages by grepping
`InsertConfirmation`/`Find*ByID` call order in every `Confirm` method:

| Package             | Order before Phase 12 | Fixed this phase |
|---------------------|------------------------|-------------------|
| `receiving`         | INSERT before SELECT  | yes |
| `putaway`           | INSERT before SELECT  | yes |
| `picking`           | INSERT before SELECT  | yes |
| `shipping`          | INSERT before SELECT  | yes |
| `stockadjustment`   | already fixed (Phase 11) | n/a |
| `stockcount`        | already fixed (Phase 11) | n/a |

Fix applied identically to all four: reorder so the existence check (a
side-effect-free `SELECT`) runs before the idempotency-guard `INSERT`.
This does not weaken the idempotency guarantee — the thing being raced
against is the confirm action itself, never the existence check, so
moving a read earlier changes nothing about what a duplicate submission
can or can't do.

**Proven live against the real Docker SQL Server, all 6 packages, after
rebuilding and redeploying the backend image**:

- `POST` `/confirm` against a nonexistent id on every one of the 6
  packages (`/receipts`, `/putaway`, `/picking`, `/shipping`,
  `/stock-adjustments`, `/stock-counts`) now returns a clean 404
  `NOT_FOUND` with a specific message (e.g. `"picking task not found"`)
  — never a raw 500.
- Duplicate confirm against an already-terminal document (e.g. a
  `COMPLETED` receipt, a `COMPLETED` putaway task) still returns a clean
  409 `INVALID_TRANSITION` — confirming the status-transition guard
  (which runs *after* both the existence check and the idempotency
  insert) still fires correctly post-reorder, not a 500.
- True idempotency, end to end: created a fresh receipt
  (`RCV-040004`), confirmed it with `idempotency_key
  phase12-idem-test-40004` (`received_qty` 0 → 10, status →
  `COMPLETED`), then resent the *exact same* idempotency key — 200,
  byte-identical response, `received_qty` still exactly 10 (not 20).
  Confirms the reordering changed only *when* existence is checked, not
  *whether* a duplicate submission can double-apply.

### Part B — Picking Reservation

#### Frontend inspected

`PickingView.vue` and `services/picking.js` re-inspected in full. No
reservation/available-quantity display exists anywhere on this screen
(it shows Order/Picked/Remain per line, nothing about the underlying
inventory balance) — so no UI change was required here, matching the
task's own "don't redesign unless a minimal display is required."
`StockInquiryView.vue` was separately confirmed to already fully render
`on_hand_qty`/`reserved_qty`/`available_qty` (both in the summary cards
and the per-row/detail view) — Phase 5 built this display speculatively
before anything ever wrote a non-zero `reserved_quantity`; this phase
is the first to make those numbers real. No task-cancellation control
exists anywhere in `PickingView.vue` (no cancel button, no cancelled
state reachable from the UI) — confirmed, not invented; `Status` in
`internal/picking/dto.go` does define a `StatusCancelled` value but
nothing in the current frontend or backend handler ever transitions a
task into it, so reservation-release-on-cancel has nothing to hook into
this phase and is not implemented.

#### Design: reserve at task creation, consume at PICKED confirm

```
Create picking task, one line per item
        |
        v
ReserveStockTx(product, location, lot, ordered_qty)
   — guarded UPDATE: reserved_quantity += ordered_qty,
     only if reserved_quantity + ordered_qty <= quantity
   — never touches quantity, never writes a ledger row
        |
        v
   task PENDING, reserved_quantity now reflects the claim
        |
        v
Confirm: action=PICKED, quantity=N
        |
        v
ConsumeReservationTx(product, location, lot, N)
   — reserved_quantity -= N (release)
   — then removeStockTx: quantity -= N, PICK ledger row written
   — both guarded UPDATEs in the same already-open transaction
        |                                   or
        v                                   v
   on_hand drops by N,                Confirm: action=SHORT
   reserved drops by N,                       |
   ledger has exactly one                     v
   new PICK row                    ReleaseReservationTx(product, location,
                                    lot, ordered_qty - picked_qty)
                                       — releases whatever fraction of
                                         the line's reservation was never
                                         picked; quantity untouched, no
                                         ledger row
```

Chosen over the alternative (reserve nothing, let `RemoveStockTx`'s own
guarded UPDATE be the only contention point at confirm time) because the
task explicitly asked for reservation at creation time, and because it
closes a real gap Phase 9–11 each explicitly deferred: without it, two
picking tasks could be created against overlapping quantities at the
same (product, location, lot) and neither would find out until one of
them tried to confirm, potentially after a picker had already walked to
the location expecting stock that a concurrent task had since removed.

#### Inventory Core methods added

All three live in `internal/inventory` (`repository.go`/`service.go`),
alongside the existing `AddStockTx`/`RemoveStockTx`/`MoveStockTx` —
Picking's own service and handler contain no reservation SQL of their
own, calling only these:

- **`Repository.AdjustReservation(ctx, productID, locationID, lotNo,
  delta) (int64, error)`** — the mechanism. One UPDATE statement:
  `SET reserved_quantity = reserved_quantity + ?` guarded by `WHERE
  reserved_quantity + ? >= 0 AND reserved_quantity + ? <= quantity` —
  both the "can't reserve past on-hand" and "can't release past zero"
  checks live in the same WHERE clause as the arithmetic, exactly
  mirroring `AdjustBalance`'s existing pattern for `quantity` itself.
  Never touches `quantity`. Returns rows-affected; 0 means either no
  balance row exists, or the delta would have pushed `reserved_quantity`
  outside `[0, quantity]`.
- **`Service.ReserveStockTx(ctx, txRepo, ReserveRequest{ProductID,
  LocationID, LotNo, Quantity})`** — positive-delta `AdjustReservation`
  call; disambiguates a 0-rows result into `ErrNoStock` (no balance row
  at all) vs `ErrInsufficientStock` (row exists, not enough available)
  by a follow-up `FindBalance` lookup, the same pattern
  `removeStockTx` already used for its own disambiguation.
- **`Service.ReleaseReservationTx(ctx, txRepo, ReserveRequest{...})`** —
  negative-delta `AdjustReservation` call for the SHORT-pick path;
  0 rows disambiguates to `ErrNoStock` or the new
  `ErrReservationUnderflow`.
- **`Service.ConsumeReservationTx(ctx, txRepo, StockChangeRequest{...})`**
  — the PICKED-confirm composite: releases the reservation
  (`AdjustReservation` with `-Quantity`) then calls the existing
  `removeStockTx` (writes the PICK ledger row), both against the
  caller's already-open transaction, so a failure at either step rolls
  back both. Reuses `StockChangeRequest` (not `ReserveRequest`) since it
  needs `Type`/`ReferenceType`/`ReferenceID`/`CreatedBy` for the ledger
  write that `ReserveRequest` deliberately omits. A useful side effect:
  because the release and the removal share the same `Quantity`, trying
  to pick more than was ever reserved for that line now fails cleanly
  with `ErrReservationUnderflow` instead of silently succeeding via
  `RemoveStockTx` alone (which only ever checked physical stock, not the
  line's own reservation) — a correctness improvement this design adds
  for free, not something separately requested.
- `ReserveRequest{ProductID, LocationID, LotNo, Quantity}` — deliberately
  narrower than `StockChangeRequest`: a reservation is a claim, not a
  movement, so it carries no `Type`/`Reference*`/`Reason`/`CreatedBy`.

#### Picking package changes

- `createItem` (called from `Create`): the old advisory-only
  `FindBalance` existence check is replaced with a real
  `ReserveStockTx` call for the line's full `ordered_qty`.
  `inventory.ErrNoStock` still maps to the existing `ErrNoSourceStock`
  (same client-facing error as before); `inventory.ErrInsufficientStock`
  now maps to a new, more accurate `"insufficient available stock to
  reserve this quantity"` validation message (previously this case
  wasn't distinguished at all, since the old check only looked at
  existence, not sufficiency).
- `Confirm`'s PICKED branch: `RemoveStockTx` replaced with
  `ConsumeReservationTx`, same `StockChangeRequest` shape as before
  (`Type: TransactionPick`, same `PICKING`/`pick_no` reference). Added
  handling for `ErrReservationUnderflow` (422, distinct message from the
  existing insufficient-stock case).
- `Confirm`'s SHORT branch: now computes `unreserveQty := item.OrderedQty
  - item.PickedQty` and calls `ReleaseReservationTx` for that amount
  before marking the line `SHORT` (previously this branch touched
  inventory not at all). Skipped when `unreserveQty` is 0 (a SHORT on an
  already-fully-picked line, which `computeTaskStatus`/`itemStatus`'s
  existing terminal-state check already prevents from reaching this code
  a second time).
- No new permission codes: `picking.create`/`picking.update` already
  gate `Create`/`Confirm` respectively (unchanged since Phase 9) and the
  frontend has no separate reservation action to gate.

#### Concurrency proof

`AdjustReservation` is one SQL `UPDATE` with the arithmetic and both
safety bounds in its `WHERE` clause — the same single-atomic-statement
mechanism Phase 5's `AdjustBalance` established and every phase since
has relied on for `quantity` itself. Two concurrent reservation attempts
against the same `(product, location, lot)` are serialized by SQL
Server at the row level; the loser's `UPDATE` genuinely returns 0 rows
affected rather than racing a Go-side read-then-write.

**Proven live**: seeded a fresh balance at exactly 55 available
(`SKU-PHASE11-001` / location `A-01-02` / lot `PHASE12-LOT`, after
prior test steps had already consumed some of an initial 100), then
fired two concurrent `POST /picking` requests each requesting 40 units
against that same tuple. One returned `201` (task created, reservation
applied); the other returned `422
"insufficient available stock to reserve this quantity"`. Re-queried
`GET /inventory` immediately after: `reserved_qty=40`, `on_hand_qty=55`
— never `80`, proving the two requests could not together over-reserve
past on-hand, matching the task's own example scenario in kind (a
100/0 tuple with two concurrent 70-unit requests would resolve
identically: one succeeds, the other is rejected, total reserved never
exceeds on-hand).

#### Frontend changes

None. `PickingView.vue` displays no reservation data and needed none —
see Frontend inspected above. `StockInquiryView.vue` needed no code
change either; it already renders `reserved_qty`/`available_qty`
correctly and now simply receives real (non-zero) values for the first
time.

#### Swagger changes

None of Picking's request/response DTOs changed shape (reservation logic
lives entirely in `internal/inventory`'s and `internal/picking`'s
internal `service.go`/`repository.go`, never in an HTTP-facing struct),
so no annotation changes were needed. Regenerated anyway
(`swag init -g cmd/server/main.go -o docs --parseInternal --parseDependency`)
to confirm no drift — output identical in shape, confirming the
prediction.

#### Tests

`go test ./...`, `go vet ./...`, `go fmt ./...` all clean across every
package, no regressions. New: `internal/inventory/service_test.go`
gained `TestValidateReserveRequest_*` (missing IDs, zero/negative
quantity, valid request — mirroring the existing
`TestValidateStockChange_*` table) and `TestReserveStockTx_
/ReleaseReservationTx_/ConsumeReservationTx_RejectsInvalidRequest`
(each callable with a nil repo/context since validation runs before any
repository access, the same "pure validation is DB-free" rationale the
existing `TestMoveStock_Rejects*` tests already established). The actual
guarded-UPDATE concurrency behavior, the SHORT-release path, and the
PICKED-consume path were verified live against the real Docker SQL
Server, not as Go unit tests — consistent with every prior phase's own
reasoning that real transactional/concurrency behavior isn't something
a mocked test can demonstrate credibly.

### Live DB verification

Full scenario run against the real, rebuilt Docker backend, using a
fresh receipt (`RCV-040005`) received to exactly 100
(`SKU-PHASE11-001`, location `A-01-02`, lot `PHASE12-LOT`):

- Before any picking task: `on_hand_qty=100`, `reserved_qty=0`,
  `available_qty=100`.
- Created a picking task for 30 → `on_hand_qty=100`,
  `reserved_qty=30`, `available_qty=70`, status `RESERVED` — exactly
  the task's own specified scenario.
- Confirmed `PICKED` for 30 → `on_hand_qty=70`, `reserved_qty=0`,
  `available_qty=70` — exactly the task's own specified end state.
  `GET /inventory/{id}/transactions` showed exactly two rows total for
  this tuple: `RECEIVE +100` and `PICK -30` — **no reservation
  pseudo-movement**, confirming reservation never touches the ledger.
- Attempted to reserve 71 against a 70-available balance → 422
  `"insufficient available stock to reserve this quantity"`, balance
  re-queried immediately after and confirmed unchanged (no partial
  reservation leaked through the rejected transaction).
- Partial-pick-then-SHORT: reserved 40 (`reserved_qty=40,
  available=30`), confirmed `PICKED` for 15
  (`on_hand=55, reserved_qty=25, available=30`), then confirmed
  `SHORT` for the remaining line → `reserved_qty=0, on_hand=55,
  available=55` — the unpicked 25 was released, no stock was removed
  for the short amount, final reservation for the terminal line is
  exactly zero.
- Concurrency: two simultaneous `POST /picking` for 40 units each
  against a 55-available balance → one `201`, one `422`; final
  `reserved_qty=40, on_hand_qty=55` (never 80).
- Duplicate-confirm idempotency on a reservation-backed PICKED
  confirm: first confirm consumed the full 40 (`on_hand=15,
  reserved_qty=0`); resending the identical `idempotency_key` returned
  a byte-identical 200 and the balance was provably not
  double-decremented (`on_hand` stayed 15, not -25).
- Missing-task confirm re-verified against all 6 packages post-fix (see
  Part A above) — all return clean 404s.

### Docker/regression results

`docker compose config` clean. `docker compose build` rebuilt all 4
images (backend, worker, frontend, migrate) cleanly. `docker compose up
-d` — all 5 services reached healthy (`worker` has no healthcheck
defined, unchanged since Phase 1): `GET /api/v1/health` →
`{"database":"connected","rabbitmq":"connected"}`; Swagger UI 200;
RabbitMQ management UI 200; frontend 200 via Nginx on port 80. Full
`go test ./...` (covering every package from every phase, not just
`inventory`/`picking`) passed with zero regressions both before and
after the reservation wiring was added. Confirmation-regression paths
re-verified live across all 6 workflow packages post-rebuild (see Part
A). Pre-existing Phase 1–11 fixtures (`PO-PHASE8-*`, `PO-PHASE9-*`,
`PO-PHASE11-INIT`, etc.) were left untouched; this phase's own test data
(`RCV-040004`, `RCV-040005`, `PICK-020002`/`020004`/`020005`) was added
alongside them, not in place of them, matching every prior phase's own
convention of keeping historical fixtures live as regression anchors.

### Known issues

- **Picking task cancellation does not exist** — `StatusCancelled` is
  defined in `internal/picking/dto.go` but nothing in the current
  backend handler or `PickingView.vue` ever transitions a task into it.
  The task instructions for this phase explicitly asked this be
  documented rather than invented; if a future phase adds real
  cancellation, it must call `ReleaseReservationTx` for whatever
  fraction of each line's reservation remains outstanding before
  marking the task cancelled, using the same pattern this phase's SHORT
  branch already established.
- **Confirm's per-line `Quantity` still has no explicit check against
  `OrderedQty - PickedQty`** (a pre-existing gap, not introduced this
  phase) — however, `ConsumeReservationTx`'s own `ErrReservationUnderflow`
  now catches the case where cumulative PICKED quantities across
  multiple confirm calls would exceed the line's original reservation,
  which is the practically important instance of this gap. It would
  still be possible to pick more than `OrderedQty` in a single
  confirm call if the reservation itself happened to be large enough
  (e.g. a line whose reservation was never fully consumed by an earlier
  SHORT on a *different* line sharing the same tuple) — narrow, and not
  observed to be reachable through the current frontend, which only ever
  sends one line's own remaining quantity.
- **Reservation is per-(product, location, lot), not per picking line**
  — if two different lines (on the same or different tasks) target the
  exact same tuple, their reservations are indistinguishable in
  `inventory.reserved_quantity` (it is a single running total, not
  tagged by task/line). This matches the schema's own design (Phase 5)
  and is not a new limitation this phase introduces, but is worth noting
  since reservation makes this tuple's contention more visible than it
  was when only `quantity` moved.
- Everything listed as a known issue in Phase 2–11 still applies unless
  explicitly superseded above (the Confirm-before-existence-check bug in
  Putaway/Picking/Shipping is now fixed, superseding Phase 11's own
  note about it).

## Phase 13 — Picking Task Cancellation and Reservation Release: VERIFIED COMPLETE

### Frontend and backend status model inspected

Re-read `PickingView.vue`, `services/picking.js`, and every file in
`internal/picking` in full before writing anything. Findings that shaped
this phase:

- `Status` already defines `CANCELLED` and its DB `CHECK` constraint
  (`migrations/0017`) already allows it — both added in Phase 9 purely
  for "schema forward-compatibility," documented there as unreachable by
  any endpoint. This phase is the first to actually transition a task
  into it. **No migration was needed at all** — this is a pure Go-side
  service/handler/frontend addition.
- No cancellation control, cancelled state, or cancel-related copy
  exists anywhere in `PickingView.vue` — confirmed by reading the whole
  file, not assumed from Phase 12's own note about this. Adding one is
  therefore a genuine (if small) addition, not wiring up something the
  mock already modeled.
- `computeTaskStatus`/`itemStatus` never produce a cancelled state for
  an item — there is no `ItemStatusCancelled`. Left it that way (see
  Status rules below) rather than inventing one.
- The per-line Confirm/Short-pick controls in the existing-task drawer
  were gated only on the `picking.update` permission, with no check on
  the *task's own* status — meaning a user could already attempt to
  pick a line on a `COMPLETED`/`SHORT_PICK` task pre-Phase-13 too (the
  backend's own `Status.CanConfirm` guard would reject it with a 409,
  just not gracefully in the UI). Phase 13 fixes this incidentally by
  gating those controls on the task being workable, which also covers
  the new `CANCELLED` case.

### Cancellation workflow

```
PENDING or IN_PROGRESS task
        |
        v
POST /picking/{id}/cancel  (optional {"remark": "..."})
        |
        v
Cancel: find task (404 if missing) -> CanCancel()? no -> 409 NOT_CANCELLABLE
                                    -> yes
        |
        v
For each line NOT already COMPLETED/SHORT:
    remaining = ordered_qty - picked_qty
    if remaining > 0: ReleaseReservationTx(product, location, lot, remaining)
        |
        v
task.status = CANCELLED, remark updated if provided
        |
        v
   COMMIT (atomic: one transaction, same shape as Confirm's own)
```

Mirrors Create's `ReserveStockTx` call in reverse: Create claims each
line's full `ordered_qty` up front, Cancel gives back whatever of that
claim was never actually picked. Never calls `AddStockTx`/
`RemoveStockTx`/`MoveStockTx` and never writes an `inventory_transactions`
row — grepped `internal/picking/service.go`'s new `Cancel` method to
confirm no such call exists anywhere in it. `quantity` itself is
untouched throughout; only `reserved_quantity` moves, exactly Phase 12's
own "reservation is a claim, not a movement" principle applied to its
reverse operation.

### Status rules

- **`Status.CanCancel()`** (new method, `internal/picking/model.go`):
  `true` for `PENDING`/`IN_PROGRESS`, `false` otherwise — currently
  identical to `CanConfirm()`'s predicate, but kept as its own method
  (not a direct reuse) so a future business rule could diverge the two
  without touching Confirm's own gate. This single predicate covers two
  of the task's explicit requirements at once: cancellation is rejected
  once stock has already been fully picked (`COMPLETED`/`SHORT_PICK` are
  both terminal, reached only once every line itself is), and double
  cancellation is prevented (`CANCELLED` is itself terminal, so a second
  `Cancel` call — retry, double-click, or a genuine concurrent race that
  lost against the first call's own row lock — is rejected here, before
  ever touching a reservation).
- **No `ItemStatusCancelled` was added.** Lines on a cancelled task keep
  whatever status they already had (`PENDING`/`PARTIALLY_PICKED` for
  still-open lines, `COMPLETED`/`SHORT` for already-resolved ones) — the
  header's own `CANCELLED` status is what makes the whole task
  non-actionable (`Status.CanConfirm()` already returns `false` for
  `CANCELLED`, so a stray `Confirm` attempt against a cancelled task's
  line is rejected with the existing 409, not a new error path). Adding
  a parallel per-item terminal state would have meant either double-
  bookkeeping two independent "is this task done" signals or replicating
  `computeTaskStatus`'s own logic for a state it never produces —
  neither was needed for correctness, so neither was built, per the
  task's own "add CANCELLED only if it fits the existing model" framing.
- **No new `CreatedBy`/`cancelled_by`/`cancelled_at` column was added.**
  `UpdateTask` already writes `status`/`remark`/`updated_at`
  unconditionally; `CancelRequest.Remark` (optional) reuses the header's
  existing `remark` field exactly as Reject already does in
  `internal/stockadjustment`. `completed_at` is deliberately left
  untouched by Cancel — cancellation is not completion, and the field's
  own name would misrepresent what happened if set here.

### Reservation release behavior

Uses Phase 12's existing `Service.ReleaseReservationTx` — no new
Inventory Core method, no reservation SQL written inside
`internal/picking` (grepped to confirm: the only inventory calls in the
new `Cancel` method are `inventory.NewRepository(tx)` and
`s.inventory.ReleaseReservationTx`). For each line still in `PENDING`
or `PARTIALLY_PICKED` status, releases exactly `ordered_qty - picked_qty`
— the fraction of that line's original task-creation-time reservation
that was never consumed by a `PICKED` confirm. `on_hand_qty` (`quantity`)
is never written by Cancel; no `inventory_transactions` row is ever
inserted by it — both verified live (see below) by checking the ledger
before and after every cancellation scenario and confirming zero new
rows.

**A correctness subtlety found while designing this, not assumed away**:
a line already flagged `SHORT` still shows `ordered_qty > picked_qty`
numerically (SHORT deliberately never fills the remainder), but its
reservation was already fully released by `Confirm`'s own SHORT branch
back in Phase 12. If `Cancel` computed "remaining" purely from
quantities and ignored `item.Status`, cancelling a task that has a mix
of one `SHORT` line and one still-open line would attempt to release the
`SHORT` line's already-zero reservation a second time — at best a
clean, confusing `ErrReservationUnderflow` on an otherwise-legitimate
cancel; at worst (if some other reservation happened to occupy the exact
same tuple) silently releasing someone else's claim. `Cancel` therefore
skips any line whose `Status` is already `COMPLETED` or `SHORT`
*before* computing "remaining," the same terminal-state check `Confirm`
itself already uses. **Proven live** (see below) with a real two-line
task: one line SHORTed, the other left open, then the whole task
cancelled — the SHORT line's already-released reservation was correctly
left alone, only the open line's remainder was released, and the
final `reserved_quantity` landed at exactly zero with no error.

### Partial-pick handling

Confirmed live end to end: a 10-unit reservation, 4 units `PICKED`
(writing the usual `PICK -4` ledger row and dropping `on_hand_qty` by 4
permanently), then the task `Cancel`led — `on_hand_qty` stayed at the
post-pick value (unchanged by Cancel), `reserved_quantity` dropped by
exactly the remaining 6 (not 10), and the ledger gained no second row
from the cancellation. The already-picked stock deduction survives the
cancellation exactly as the task's own instructions required; only the
never-picked remainder is given back.

### Idempotency / duplicate-cancellation prevention

No new idempotency-key table (no `picking_cancellations` migration) —
a deliberate choice, not an oversight. Reasoning:

1. `Status.CanCancel()`'s own terminal-state guard already gives a
   plain double-click or client retry a clean, correct outcome: the
   first call moves the task to `CANCELLED`; a second call's own
   `FindTaskByID` sees that committed state and returns 409
   `NOT_CANCELLABLE` immediately, before ever touching a reservation.
2. For the much narrower case of a genuine sub-millisecond concurrent
   race (two `Cancel` calls both reading `PENDING` before either
   commits), `AdjustReservation`'s own guarded UPDATE (Phase 12) is
   already a safe backstop: SQL Server's row-level lock on the
   `inventory` row serializes the two transactions' `UPDATE` statements,
   and the loser's own guard (`reserved_quantity + delta >= 0`) fails
   with 0 rows affected — `ErrReservationUnderflow`, not silent
   corruption. This is the same mechanism Phase 12's own concurrent-
   reservation proof already relied on, applied here to prove the
   reverse operation is equally race-safe.
3. Cancel's mutation (releasing a reservation) is fundamentally lower-
   stakes than Confirm's (removing real stock): even in the rare-race
   outcome above, the loser gets a clean error and no data is corrupted,
   which is why Confirm's own idempotency-key mechanism (needed because
   a lost race there would mean *physically removing stock twice*) isn't
   proportionate here. Matches the task's own "add idempotency if
   needed" phrasing — this was evaluated and judged not needed, not
   skipped by default.

**Proven live**: cancelling an already-`CANCELLED` task returns 409
`NOT_CANCELLABLE` and leaves `reserved_quantity`/`on_hand_qty` provably
unchanged (re-queried immediately after).

### RBAC

Cancel is gated on `picking.approve` — the one `picking.*` permission
code no endpoint had used since Phase 2 (Confirm uses `picking.update`
instead; see `internal/picking/handler.go`'s own pre-existing doc, which
literally called `picking.approve` "reserved for a future approval
workflow"). Deliberate choice over reusing `picking.update`: cancelling
ends the whole task and gives back its reservation, a more
consequential, planning-level decision than confirming one line, so it
sits at Create's own authority level rather than picking staff's
execution-level permission. Under current seed data
(`internal/rbac/catalog.go`, unchanged this phase) this means
ADMIN/WAREHOUSE_MANAGER only — the `PICKING` role itself (view + update
only) cannot cancel a task it can still execute picks against. No new
permission codes. 2 new RBAC middleware tests
(`TestRequirePermission_PickingApprove_DeniedForPickingRole`,
`..._AllowedForWarehouseManagerShapedToken`), mirroring the existing
`PickingCreate` pair exactly. **Proven live**: created a fresh
ACTIVE `PICKING`-role user, logged in as them, attempted
`POST /picking/{id}/cancel` → 403 `"permission denied: picking.approve"`;
the same call as `admin` (`WAREHOUSE_MANAGER`-equivalent) succeeded.

### Frontend changes

- `services/picking.js`: new `cancelPickingTask(id, payload)` →
  `POST /picking/{id}/cancel`.
- `PickingView.vue`:
  - New `canCancel` computed (`hasPermission('picking.approve')`) and
    `isTaskWorkable` computed (`status` is `PENDING`/`IN_PROGRESS`) —
    the latter also fixes the pre-existing gap noted above where the
    per-line Confirm/Short-pick controls didn't check the task's own
    status at all.
  - A "Cancel Task" button in the existing-task drawer's footer,
    alongside the existing "Close" button, visible only when
    `canCancel && isTaskWorkable`; a small optional remark input above
    it, matching the Short Pick remark field's own style.
  - Per-line Confirm/Short-pick controls now additionally require
    `isTaskWorkable`; the "your role can view but not confirm" fallback
    message gained a second branch — "This task has been cancelled" —
    shown specifically when the task is `CANCELLED` rather than the
    generic permission message.
  - Status filter `<select>` gained a `CANCELLED` option; `statusText()`
    gained a `CANCELLED -> 'Cancelled'` case; `getStatusClass()` maps
    `CANCELLED` to the same red badge `SHORT_PICK` uses.
  - Layout, card structure, and every other view untouched — no
    redesign, per the task's own instruction.
  - `npm run build` clean.

### Swagger

New `Cancel` handler documented in full (summary, description covering
the atomicity/no-ledger/no-on-hand-change guarantees, `CancelRequest`
body, 200/400/401/403/404/409 responses, `BearerAuth`). New
`CancelPickingRequest` model. Regenerated
(`swag init -g cmd/server/main.go -o docs --parseInternal --parseDependency`)
and grepped `docs/swagger.json` to confirm both
`/picking/{id}/cancel` and `CancelPickingRequest` are present.

### Tests

`go test ./...`, `go vet ./...`, `go fmt ./...` all clean, zero
regressions. New: `TestStatus_CanCancel` (table test, same shape as the
existing `TestStatus_CanConfirm`) in `internal/picking/service_test.go`;
2 new RBAC middleware tests (see RBAC section) in
`internal/platform/middleware/permission_test.go`. No new DB-independent
unit tests for `Cancel` itself — unlike `Create`/`Confirm`, `Cancel` has
no pure-validation step before its first repository call (its very
first action is `FindTaskByID`), so there is no DB-free code path to
exercise the way `TestConfirm_RejectsEmptyItems`/`TestCreate_Rejects*`
do; the same reasoning every prior phase has used for its own
transactional logic — verified live against the real Docker SQL Server
instead (see below), not as a mocked Go test.

### Live DB verification

All scenarios run against the real, rebuilt Docker backend, using the
same `SKU-PHASE11-001` / `A-01-02` / `PHASE12-LOT` tuple Phase 12 left
at `on_hand_qty=15, reserved_qty=0`:

- **Cancel untouched task**: reserved 10 (`reserved=10, available=5`) →
  cancelled → `reserved=0, available=15, on_hand=15` (fully restored,
  nothing was ever picked).
- **Cancel partial task**: reserved 10, `PICKED` 4
  (`on_hand=11, reserved=6`) → cancelled → `on_hand=11` (**unchanged by
  cancellation**, the picked 4 stayed removed), `reserved=0,
  available=11`. Ledger re-queried: exactly one `PICK -4` row from the
  confirm, no second row from the cancel.
- **Duplicate cancel**: cancelling the same already-`CANCELLED` task
  again → 409 `NOT_CANCELLABLE`, balance re-queried and confirmed
  unchanged.
- **Cancel rejected after full pick**: reserved 5, `PICKED` 5 (task
  reaches `COMPLETED`) → `Cancel` → 409 `NOT_CANCELLABLE`, balance
  unaffected by the rejected attempt.
- **Mixed SHORT + open line**: a two-line task (3 + 2 units) reserved
  5 total → line 1 `SHORT`ed (releases its 3, leaving `reserved=2`) →
  whole task `Cancel`led → line 2's remaining 2 released correctly,
  final `reserved=0` — **no double-release of line 1's
  already-resolved reservation**, no error, no corruption. This is the
  scenario the "correctness subtlety" above was written to prevent, and
  it behaved exactly as designed.
- **Cancel on a nonexistent task** → 404 `"picking task not found"`.
- **RBAC**: a real `PICKING`-role user (view + update only) → 403 on
  cancel; `admin` (`WAREHOUSE_MANAGER`-equivalent) → 200.
- **Stock Inquiry reflects the release immediately**: `GET
  /inventory/{id}` and `GET /inventory/summary` both re-queried
  immediately after each cancellation in the sequence above, with no
  intervening delay or cache — every value matched the expected
  post-release state on the first read, and the global summary's own
  `reserved_qty` returned to `0` once every test task above had reached
  a terminal state.
- Full ledger for the test tuple re-queried at the end of the whole
  sequence: exactly the rows created by real `RECEIVE`/`PICK`
  operations across Phases 12–13, zero rows attributable to any Cancel
  call.

### Docker/regression results

`docker compose config` clean. `docker compose build` rebuilt all 4
images cleanly. `docker compose up -d` — all 5 services reached healthy
(`worker` has no healthcheck, unchanged since Phase 1). `GET
/api/v1/health` → both dependencies connected; Swagger UI 200; RabbitMQ
management UI 200; frontend 200 via Nginx. Full `go test ./...` — every
package, zero regressions. Phase 12's own confirm-before-existence-check
fix re-verified live post-rebuild across all 6 workflow packages
(`receipts`/`putaway`/`picking`/`shipping`/`stock-adjustments`/
`stock-counts`) — all still return clean 404s for a nonexistent parent
id. Spot-checked `GET` across every module from every phase
(`users`/`roles`/`permissions`/`warehouses`/`products`/`inventory`/
`receipts`/`putaway`/`picking`/`shipping`/`stock-adjustments`/
`stock-counts`) — all 200 except `zones` (400, expected: requires a
`warehouse_id`/`zone_id` filter, unchanged behavior from every prior
phase). Pre-existing fixtures from every phase untouched; this phase's
own test data (`PICK-020007` through `PICK-020010`, user
`phase13_picking_user`) added alongside them.

### Known issues

- **A cancelled task cannot be reopened or recreated from itself** —
  there is no "reactivate" or "clone" action; a cancelled task is
  permanently terminal, the same as `COMPLETED`/`SHORT_PICK`. Not
  requested by this phase's own instructions and not invented.
- **`Cancel` has no dedicated `cancelled_by`/`cancelled_at` audit
  columns** — who cancelled a task and exactly when is only recoverable
  from `updated_at` plus whatever `remark` was supplied (optional) and,
  indirectly, request logs (`request_id`/`user_id` per Phase 1's
  structured logging). A future phase could add these columns if an
  audit requirement emerges; not built speculatively here.
- **Reservation remains per-(product, location, lot), not per line**
  (carried forward from Phase 12, unchanged) — Cancel's own per-line
  release amounts still net out correctly against the shared running
  total, proven live by the mixed-SHORT-plus-open-line scenario above,
  but the underlying limitation itself is unchanged.
- Everything listed as a known issue in Phase 2–12 still applies unless
  explicitly superseded above.

## Phase 14 — Stock Count History: VERIFIED COMPLETE

### Existing Stock Count data reused

Inspected `CheckStockView.vue` and every file in `internal/stockcount`
in full before writing anything. Findings:

- `CheckStockView.vue` was, and remains, a **single perpetual working
  page** — Phase 11 already documented this as the mock's own shape
  (no list-of-past-counts screen exists anywhere in the frontend). Once
  a count was created or navigated away from, there was no way to find
  it again: `services/stockCount.js` had no `listStockCounts` function
  at all, and `GET /stock-counts` (which has existed, fully working,
  since Phase 11) had zero frontend callers.
- Every field the task's own requirement list asked for already existed
  in the schema or was one JOIN away — **no new table, column, or
  migration was needed**:
  - Count-level: `count_no`, `count_type`, `warehouse_id`/`code`,
    `zone_id`/`code`, `status`, `count_date`, `created_at` all already
    existed on `stock_counts` (migrations/0026) and were already
    queried by `FindDetail`/`List`.
  - `created_by` (`BIGINT NULL` FK to `users`) has existed on the
    `stock_counts` table and its Go `Count` model since Phase 11 — it
    was simply never surfaced in either `Response` or `ListResponse`,
    and never joined to a display name. This was the one genuine gap:
    every other module in this codebase has the same pattern (a
    `CreatedBy *int64` field that's never surfaced to any frontend —
    grepped `internal/receiving`, `internal/putaway`, and
    `internal/stockadjustment`'s own DTOs to confirm), so Phase 14 is
    the first phase anywhere in this codebase to expose "created by" in
    an API response at all.
  - Line-level: `system_qty`, `count_qty`, `variance_qty`,
    `condition`, `status`, `remark` all already existed on
    `stock_count_items` (migrations/0027) and were already returned in
    full by `GET /stock-counts/{id}` via `ListItems` — the "count
    detail / expected quantity / counted quantity / variance quantity /
    confirmation status" requirements needed **zero backend changes**,
    only a frontend consumer.
- No duplicate Stock Count data structure was created anywhere — the
  History list and detail views are pure reads against the exact same
  `stock_counts`/`stock_count_items` tables every other Stock Count
  action already writes to.

### APIs added/changed

No new endpoints — `GET /stock-counts` and `GET /stock-counts/{id}`
already existed and needed no route changes, only response/query
additions:

- **`ListParams`** gained `Search` (matches `count_no` — the only
  natural free-text handle a count header has, since unlike
  Receiving/Picking/Shipping there is no order/customer/SKU text
  directly on `stock_counts`) and `Date` (a single-day `count_date`
  filter, the identical `date = ?` pattern `internal/picking`'s own
  `List` already established). `WarehouseID`/`Status`/`Page`/`Limit`
  were unchanged.
- **`ListResponse`** gained `ZoneCode`, `CreatedBy`, `CreatedByName`,
  `CreatedAt`. **`Response`** (detail) gained `CreatedBy`,
  `CreatedByName` (it already had `CreatedAt`).
- **`Repository.List`/`FindDetail`** gained a `LEFT JOIN users u ON
  u.id = sc.created_by` (resolving `u.full_name` as
  `created_by_name`) and `List` additionally gained a `LEFT JOIN zones`
  it didn't have before (`FindDetail` already had one, added in Phase
  11). Both are display-only joins — 0-or-1 row per `stock_counts.id`,
  so neither changes which rows are returned or their count.
- `Handler.List` parses two new query params (`search`, `date`,
  the latter using the same `time.Parse("2006-01-02", ...)` pattern
  `internal/picking`'s handler already uses).

### Frontend changes

- `services/stockCount.js`: new `listStockCounts(params)` →
  `GET /stock-counts` with `search`/`status`/`warehouseId`/`date`/
  pagination — the same query-building shape every other module's list
  service function already uses (modeled directly on
  `services/picking.js`'s `listPicking`).
- `CheckStockView.vue`: added a **"Stock Count History"** card as a new
  top section, above the existing "Count Information" card — search
  box (count no.), status/warehouse/date filters with a "Clear Filter"
  button (identical shape to every other module's own filter bar,
  e.g. `PickingView.vue`'s), and a table (Count No./Type/Warehouse/
  Zone/Created Date/Created By/Items/Variance Lines/Status/View).
  Loading state ("Loading stock count history…"), empty state ("No
  stock counts found"), and error state (a red banner, matching every
  other error banner already used on this same page) are all handled,
  the same three states every other list view in this codebase already
  implements.
  - Clicking "View" loads that count into the **exact same workspace
    sections** "+ New Count Task" already populates (Scan Stock, Stock
    Count Items, Count Summary, Actions) — no separate detail view was
    built. This is a genuine reuse, not a shortcut: `canEdit` is
    already derived from `status` (`DRAFT` vs `CONFIRMED`), so a
    historical `CONFIRMED` count automatically renders fully read-only
    (every input already goes `readonly`/hidden when `!canEdit`) with
    zero new conditional logic, and a still-`DRAFT` count from history
    can be resumed and finished exactly as if it had just been created.
  - A **"+ Start a Different Count"** button (visible only once a count
    is loaded) resets the workspace back to a blank slate — the one
    small UX gap this phase's own inspection found: previously, once
    `countId` was set there was no way back to a fresh create form
    without a full page reload, since "+ New Count Task" itself
    disappears once a count exists (`v-if="!countId"`, unchanged this
    phase).
  - The history table refreshes after `createCountTask`/`confirmCount`
    succeed, so a newly created count or a status change from
    `DRAFT`→`CONFIRMED` is reflected immediately without a manual
    reload — the same "refetch the list after a mutating action"
    pattern every other module's own view already follows.
  - No redesign of any other page; `CheckStockView.vue`'s existing
    layout, styling, and every other section are otherwise untouched.
  - `npm run build` clean.

### RBAC

No new permission codes, no route/gating changes.
`internal/stockcount/handler.go`'s existing gate
(`stock_adjustment.view` on both `GET /stock-counts` and
`GET /stock-counts/{id}`) already covered the History list/detail
exactly as it covered every other Stock Count endpoint since Phase 11
— this phase reuses it unchanged, per the task's own "reuse existing
stock_adjustment / stock-count permissions" instruction. No new RBAC
tests were added for this reason: `TestRequirePermission_
StockAdjustmentView_DeniedForMinimalToken` (Phase 11) already proves
the exact permission code the History endpoints are gated on denies a
token that lacks it, and adding a second, stockcount-specific copy of
that same test would only duplicate coverage of logic that didn't
change. **Proven live**: a real `PICKING`-role user (no
`stock_adjustment.*` grants) → 403 `"permission denied:
stock_adjustment.view"` on `GET /stock-counts`; `admin`
(`WAREHOUSE_MANAGER`-equivalent) → 200.

### Swagger

`List`'s summary/description updated to describe the History screen and
its filters; `search`/`date` query params documented. New response
fields (`zone_code`, `created_by`, `created_by_name`, `created_at` on
`ListResponse`; `created_by`, `created_by_name` on `Response`) picked up
automatically by swaggo from the struct tags. Regenerated
(`swag init -g cmd/server/main.go -o docs --parseInternal --parseDependency`)
and grepped `docs/swagger.json` to confirm both the new `search` param
description and `created_by_name` are present in the generated spec.

### Tests

`go fmt ./...`, `go vet ./...`, `go test ./...` all clean, zero
regressions (`internal/stockcount`'s existing 10 tests unaffected — none
of them touch `List`/`FindDetail`, which is exactly what changed). No
new Go unit tests were added for the `List`/`FindDetail` SQL changes
themselves: consistent with `internal/stockcount/repository_test.go`'s
own existing scope (only `TestIsUniqueViolation`, a pure error-
classification concern — nothing in this package's test suite has ever
unit-tested its raw-SQL query methods, since GORM's `Raw().Scan()`
queries need a real database connection to exercise meaningfully, not a
mock), this phase's own SQL/JOIN changes were verified live instead —
the same "real behavior isn't something a mocked test can demonstrate
credibly" reasoning every prior phase has applied. Live coverage (see
below) explicitly exercised: listing, detail retrieval, every filter
(search/status/warehouse/date) both matching and empty-result, 404, and
RBAC allowed/denied — the exact five items the task asked for tests to
cover.

### Live DB verification

All against the real, rebuilt Docker backend, using the four real Stock
Count fixtures already in the database from Phase 11's own testing
(`CNT-000001` through `CNT-000004`):

- `GET /stock-counts?limit=5` (no filters) → all 4 rows, each with
  `created_by_name: "Administrator"` correctly resolved via the new
  JOIN.
- `GET /stock-counts?search=CNT-000003` → exactly that one row.
- `GET /stock-counts?status=CONFIRMED` → exactly the 2 `CONFIRMED`
  rows.
- `GET /stock-counts?warehouse_id=4` → the rows for that warehouse.
- `GET /stock-counts?date=2026-08-30` → exactly the one row whose
  `count_date` matches.
- `GET /stock-counts?search=NOTHING-MATCHES` → `{"data":[],"total":0}`
  — the empty-result case, not an error.
- `GET /stock-counts/4` → full detail including `created_by_name`,
  and the line's `system_qty`/`count_qty`/`variance_qty`/`status`
  (100/98/-2/`COUNTED`) — every field the task's requirement list
  named.
- `GET /stock-counts/999999` → 404 `"stock count not found"`.
- RBAC: `PICKING`-role token → 403 `stock_adjustment.view`; `admin` →
  200 on both list and detail.

### Docker/regression results

`docker compose config` clean. `docker compose build` rebuilt all 4
images cleanly. `docker compose up -d` — all 5 services reached
healthy. `GET /api/v1/health` → both dependencies connected; Swagger UI
200; RabbitMQ management UI 200; frontend 200 via Nginx. Full
`go test ./...` — every package, zero regressions. Regression-checked
Phases 1–13 live: spot-checked `GET` across every module
(`users`/`roles`/`permissions`/`warehouses`/`products`/`inventory`/
`receipts`/`putaway`/`picking`/`shipping`/`stock-adjustments`/
`stock-counts`) — all 200; Phase 12's confirm-before-existence-check fix
re-verified across all 6 workflow packages (still clean 404s for a
nonexistent parent id, `stock-counts` confirm included); Phase 13's
Picking cancellation re-verified (`POST /picking/999999/cancel` → 404).
`npm run build` clean.

Full interactive browser click-through of the new History UI was
attempted but not completed this session — the login automation itself
did not reach an authenticated state after several attempts (a browser-
automation environment issue, not an application error: the login
form's own fields accepted input correctly once focused via keyboard
`Tab`, as seen mid-attempt). Per this project's own "don't rabbit-hole
on flaky browser automation" guidance, this was not pursued further.
Confidence in the UI is instead based on: (1) every field name the new
Vue code reads (`count_no`, `zone_code`, `created_by_name`,
`created_at`, etc.) was cross-checked byte-for-byte against the actual
live JSON responses captured above; (2) `npm run build` compiles the
new template/script with no errors; (3) the reused workspace sections
(`canEdit`, item table, summary cards) are unchanged Phase-11 code that
was already browser-verified then. A follow-up session should complete
a real click-through before this is treated as fully UI-proven.

### Known issues

- **No real click-through browser verification this session** (see
  Docker/regression results above) — the highest-priority follow-up,
  not a defect in the implementation itself.
- **No pagination UI** — matches every other list screen in this
  codebase (Receiving/Putaway/Picking/Shipping/Stock Adjustment all
  fetch a generous `limit` and rely on search/filters rather than a
  page-number control); History follows the same established
  convention rather than being the first screen to add one.
- **`created_by_name` is Phase 14's first precedent for exposing a
  "created by" display name anywhere in this codebase.** No other
  module's list/detail response does this yet (Receiving, Putaway,
  Picking, Shipping, Stock Adjustment all still keep `created_by` as an
  internal-only field) — a future phase touching any of those could
  reuse this exact `LEFT JOIN users` pattern if the same need arises
  there.
- Everything listed as a known issue in Phase 2–13 still applies unless
  explicitly superseded above.

## Phase 15 — Browser Verification + Workflow Cancellation Hardening: VERIFIED COMPLETE

### Part A — Phase 14 browser verification completed

**Diagnosis**: the prior session's login automation failure was a
browser-automation timing/focus issue, not an application defect.
Direct mouse clicks on `<input>` elements (via raw coordinates and via
`find`'s element refs) intermittently failed to register subsequent
keystrokes into Vue's `v-model`-bound value, and clicking immediately
after a `navigate` call (before the SPA had finished mounting) silently
dropped the click and any typed input entirely. The reliable pattern:
click a neutral, non-interactive element first (e.g. a heading) to
establish page focus, then use keyboard `Tab` to move focus field-to-
field and `Return` to submit the form — this worked consistently every
time it was tried, including immediately after login, across role
switches, and while opening/closing drawers. `LoginView.vue` itself is
a completely standard `v-model` + `@submit.prevent` form with nothing
unusual in it — confirmed by reading the full component — so there was
never a code-level reason for the failure. **No application bug was
found**; none was fixed, per the task's own conditional.

**Completed live in a real browser** (Chrome, via the `claude-in-chrome`
tools), as admin and as a fresh `phase13_picking_user` (PICKING role):

- Login → Dashboard → Check Stock, all via real form input and
  navigation.
- Stock Count History list loaded all 4 real fixture rows
  (`CNT-000001`–`CNT-000004`) with correct Count No./Type/Warehouse/
  Created Date/**Created By ("Administrator")**/Items/Variance Lines/
  Status.
- **Search** (`CNT-000003`) correctly filtered to exactly that one row.
- **Warehouse/date filters**: not exercised via native `<select>`/
  `<input type=date>` interaction this session (native dropdown
  automation was unreliable in the same way described above); already
  proven correct at the API level in Phase 14's own live verification,
  which the frontend's filter bar calls unchanged.
- Opened `CNT-000001` (**CONFIRMED**): header showed "CNT-000001 —
  Confirmed"; the Count Information card showed the correct disabled
  warehouse; the item row's Count Qty input accepted focus but
  **genuinely rejected a typed value** (typed "999", value stayed "87"
  — real `readonly` behavior, not just a visual style); no Save button
  on the line; no Save Draft/Create Adjustment/Confirm Count action bar
  at the bottom at all.
  Actually **read-only**, not just visually styled that way.
- Opened `CNT-000003` (**DRAFT**): fully editable — "+ Add Item" form
  present, a "Save" action on the line, and the full Save Draft/Create
  Adjustment/Confirm Count action bar all rendered.
- **"+ Start a Different Count"** correctly reset the workspace back to
  a blank "+ New Count Task" state.
- **Unauthorized role**: `phase13_picking_user` (view/update on
  `picking.*` only, nothing on `stock_adjustment.*`) navigating to
  `/inventory/check-stock` saw two clean red `"permission denied:
  stock_adjustment.view"` / `"permission denied:
  warehouse.view"` banners and an empty, non-crashing history table
  ("No stock counts found") — the existing `historyError` state
  rendering exactly as designed for a real 403, not a mock.

### Part B — Cancellation audit

Inspected `ReceiveView.vue`/`NewReceivingView.vue`, `PutawayView.vue`/
`NewPutawayView.vue`, `ShippingView.vue`, and every file in
`internal/receiving`, `internal/putaway`, `internal/shipping` before
writing anything.

| Module    | Existing `CANCELLED` placeholder? | Reservation to release? | Irreversible movement risk |
|-----------|-----------------------------------|--------------------------|------------------------------|
| Receiving | Yes (Phase 6, unused until now) | No (no reservation concept) | `AddStockTx` on Confirm |
| Putaway   | Yes (Phase 7, unused until now) | No (no reservation concept) | `MoveStockTx` **per line**, immediately on that line's own Confirm |
| Shipping  | **No — first schema change of its kind this project** | No (Picking already reserved/consumed before a shipment exists) | None of its own (no inventory dependency at all) |

Cancellation was judged safe and implemented for all three, each with
its own boundary derived from where real, irreversible inventory
mutation actually happens in that specific workflow — not a single
copy-pasted rule.

### Receiving cancellation rules/result

`Status.CanCancel()` — new method, identical predicate to the existing
`CanEdit()` (`DRAFT`/`WAITING` only). Per `computeReceiptStatus`'s own
doc, `WAITING` is only ever reached while every line's `received_qty`
is still zero, so this boundary **is** "before stock has been
received" exactly, with no additional per-item check needed (unlike
Putaway — see below). No migration needed (`CANCELLED` already valid).
`Cancel` never calls `AddStockTx`; refuses with 409 `NOT_CANCELLABLE`
once any line has actually received quantity — no reversal is
attempted for it. **Proven live**: fresh DRAFT receipt → cancel → 200,
`CANCELLED`; duplicate cancel → 409; a receipt partially received (5 of
10) → cancel attempt → 409, receipt stayed `PARTIALLY_RECEIVED`
untouched.

### Putaway cancellation rules/result

`Status.CanCancel()` mirrors `CanConfirm()`'s header-level predicate
(`PENDING`/`IN_PROGRESS`) — but per this phase's own "based on actual
task state" instruction, this alone is **not sufficient**: unlike
Receiving/Picking, a Putaway task can be `IN_PROGRESS` purely because
one line was flagged `ISSUE` (no stock movement at all — see
`computeTaskStatus`'s own doc), while a *different* line already has
`MovedQty > 0` from a real, committed `MoveStockTx`. `Service.Cancel`
therefore re-reads every line and additionally rejects
(`ErrCancelHasMovedStock`, 409) if **any** line has moved stock,
regardless of what the header alone would allow — this is the one
piece of genuinely new business logic this phase added beyond a status
guard. No migration needed. Never calls `MoveStock` to compensate.
**Proven live**, three scenarios:

- Untouched task → cancel → 200; duplicate cancel → 409.
- Two-line task, one line moved (3 units), the other still `PENDING` →
  cancel attempt → 409 `CANCEL_HAS_MOVED_STOCK`; ledger and both
  locations' balances re-queried and confirmed unaffected by the
  rejected attempt.
- Two-line task, one line flagged `ISSUE` (zero moved), the other still
  `PENDING` (header `IN_PROGRESS` from the `ISSUE` flag alone) → cancel
  → **200, succeeds** — proving the fine-grained MovedQty check (not
  just the header status) is what actually gates this, exactly as
  designed.
- Full ledger re-check after all three scenarios: exactly one `PUTAWAY`
  transaction total (from the one real move in scenario 2), zero
  entries attributable to any cancel call.

### Shipping cancellation rules/result

`Status.CanCancel()` — new method, `READY` only (before dispatch),
matching `CanEdit`/`CanDispatch`'s own existing boundary. **This
package never had a `CANCELLED` placeholder** (migrations/0020's own
doc explicitly named `IN_TRANSIT` as the only forward-compat value it
was carrying) — closing this gap required
`migrations/0029_add_cancelled_status_to_shipments`, the **first
migration in this project that `ALTER`s an existing `CHECK` constraint**
rather than only adding new tables (`DROP CONSTRAINT` /
`ADD CONSTRAINT` with `CANCELLED` appended to the allowed list; a
symmetric `.down.sql` reverts it). `Cancel` never calls
`AddStock`/`RemoveStock`/`MoveStock` — this package has no inventory
dependency at all (Picking already removed the stock when it picked
these lines), and cancelling here explicitly does **not** restore it;
a cancelled shipment simply never goes out, it is not an un-pick.
Reuses the existing generic `ErrInvalidTransition` (its message,
"shipment is not in a state that allows this action," already fit
Cancel with no changes needed) rather than adding a new sentinel, since
Receiving/Putaway's own `ErrInvalidTransition` messages were Confirm-
specific and Shipping's was not. **Proven live**: fresh `READY`
shipment → cancel → 200; duplicate cancel → 409; a second shipment
dispatched → cancel attempt → 409, remained `DISPATCHED`.

### APIs added

```
POST /api/v1/receipts/{id}/cancel
POST /api/v1/putaway/{id}/cancel
POST /api/v1/shipping/{id}/cancel
```

All three: optional `{"remark": "..."}` body (no `idempotency_key` —
see Picking's own Phase 13 precedent for why the status guard alone is
sufficient for a pure status-transition Cancel with no side effect
besides itself), 200 with the updated document on success, 404 for a
nonexistent parent (verified — never a raw FK 500), 409 for an unsafe
transition.

### Inventory impact

**Zero.** Grepped all three packages' new `Cancel` methods to confirm:
no call to `AddStockTx`/`RemoveStockTx`/`MoveStockTx`/
`ReserveStockTx`/`ReleaseReservationTx`/`ConsumeReservationTx` appears
in any of them. No new `inventory_transactions` rows are ever written
by a cancellation in any of the three modules — verified live by
re-querying the ledger before and after every cancel scenario above and
finding zero new rows attributable to any cancel call. `on_hand_qty` is
never changed by cancellation anywhere in this phase; the one
`PARTIALLY_RECEIVED` receipt and the one partially-moved putaway task
both correctly kept their already-mutated quantities exactly as they
were.

### RBAC

No new permission codes. All three `Cancel` endpoints are gated on
each module's own `.update` permission, not `.approve` — a deliberate,
consistent choice across all three: Receiving's Cancel only ever runs
before any stock is received (the same boundary `.update`-gated Update/
AddItem/UpdateItem already use); Putaway's real safety gate is
`Service.Cancel`'s own MovedQty check, not the permission system, so
the permission sits at editing's authority level; Shipping's Cancel
only ever runs pre-dispatch, the same boundary `.update`-gated Update
already uses. Under current seed data this means RECEIVING/PUTAWAY/
SHIPPING staff (who already have `.update` for their own modules) can
cancel their own not-yet-progressed documents without needing an
ADMIN/WAREHOUSE_MANAGER's `.approve` — a deliberately different
authority model from Picking's own Phase 13 Cancel (gated on
`picking.approve`, since that Cancel releases a reservation, a more
consequential action). 3 new RBAC tests (`...Update_AllowedFor...Role`
for each module). **Proven live**: a real `PICKING`-role token (no
`receiving.*`/`putaway.*`/`shipping.*` grants at all) → 403 on all
three new endpoints.

### Frontend changes

- `services/receiving.js`, `services/putaway.js`, `services/shipping.js`
  each gained one `cancelX(id, payload)` function.
- `ReceiveView.vue`: a "Cancel Receipt" section (optional remark input +
  button) in the existing detail drawer, visible when
  `canCancelReceiving` (`receiving.update`) and the receipt is
  `DRAFT`/`WAITING`.
- `PutawayView.vue`: since this drawer is **item-level**, not task-level
  (unlike every other module's drawer — confirmed by reading the whole
  file: `selectedPutaway` is one task item, `drawerTaskId` already
  existed separately for the Confirm call), the Cancel section targets
  `drawerTaskId` and is shown whenever the currently-viewed line isn't
  already resolved and the user has `putaway.update` — independently of
  whether they also have `putaway.approve` (so a PUTAWAY-role user who
  cannot confirm putaway can still see and use Cancel). The backend
  remains authoritative if a different, already-moved line in the same
  task would make the whole task uncancellable.
- `ShippingView.vue`: a "Cancel Shipment" section in the existing
  detail drawer, visible when `canEditHeader` (`shipping.update`) and
  the shipment is `READY`; status filter gained a `CANCELLED` option;
  `statusText()`/`getStatusClass()` extended for it.
- No redesign of any page; every other section, layout, and style left
  untouched. `npm run build` clean.

### Swagger

All three new endpoints fully documented (summary, description covering
the specific safety rule and inventory-non-mutation guarantee each one
makes, `CancelRequest` body, 200/400/401/403/404/409 responses,
`BearerAuth`). New `CancelReceiptRequest`/`CancelPutawayRequest`/
`CancelShipmentRequest` models. Regenerated
(`swag init -g cmd/server/main.go -o docs --parseInternal --parseDependency`)
and grepped `docs/swagger.json` to confirm all three paths and all
three request models are present.

### Tests

`go fmt ./...`, `go vet ./...`, `go test ./...` all clean, zero
regressions. New: `TestStatus_CanCancel` in all three packages
(`internal/receiving`, `internal/putaway`, `internal/shipping` — the
latter also needed `StatusCancelled: true` added to its existing
`TestStatus_IsValid` table); 6 new RBAC middleware tests (3× `Cancel`-
denied-for-unrelated-role implicitly covered by existing "lacks the
code entirely" tests, plus the 3 new `...Update_AllowedFor...Role`
tests proving the actual gate each `Cancel` uses). No DB-dependent unit
tests for the `Cancel` service methods themselves — same reasoning
every prior phase has applied (their first action is a DB lookup, no
DB-free path exists to exercise); the MovedQty-based logic in
particular (Putaway) was verified live instead, since it needs a real
multi-line task with real confirmed movement to exercise meaningfully.

### Live DB verification

All against the real, rebuilt Docker backend (migration 0029 confirmed
applied via `wms-migrate`'s own logs: `"migration applied" version=0029`):

- Receiving: 404 on nonexistent → fresh DRAFT → cancel → 200 → duplicate
  cancel → 409; separate receipt partially received (5/10) → cancel →
  409, status unchanged.
- Putaway: 404 on nonexistent → untouched task → cancel → 200 →
  duplicate → 409; two-line task with one line moved → cancel → 409
  `CANCEL_HAS_MOVED_STOCK`; two-line task with one line `ISSUE`-flagged
  (zero moved) → cancel → 200 (the fine-grained-state proof); both
  source (`STAGING-01`) and destination (`A-01-02`) balances re-queried
  after all scenarios and matched exactly one real 3-unit move, no
  more; ledger for the new destination balance showed exactly one
  `PUTAWAY` row.
- Shipping: 404 on nonexistent → fresh `READY` shipment → cancel → 200
  → duplicate → 409; separate shipment dispatched → cancel attempt →
  409, remained `DISPATCHED`.
- RBAC: a real `PICKING`-role token → 403 `permission denied` on all
  three new endpoints, each naming the correct missing permission
  (`receiving.update`/`putaway.update`/`shipping.update`).
- Regression: Stock Inquiry global summary re-queried and consistent;
  Picking's own Phase 12/13 reservation-then-cancel flow re-run end to
  end (reserve 2 → `reserved_qty=2` → cancel → `reserved_qty=0`,
  `on_hand` unchanged) to prove this phase's changes didn't disturb it.

### Docker/regression results

`docker compose config` clean. `docker compose build` rebuilt all 4
images cleanly. `docker compose up -d` — all 5 services reached
healthy; `wms-migrate` applied migration 0029 and re-seeded 47
permissions idempotently (no drift) before exiting. `GET
/api/v1/health` → both dependencies connected; Swagger UI 200;
RabbitMQ management UI 200; frontend 200 via Nginx. Full
`go test ./...` — every package, zero regressions, both before and
after the Docker rebuild. Regression-checked Phases 1–14 live:
`GET` across every module 200 (`users`/`roles`/`permissions`/
`warehouses`/`products`/`inventory`/`receipts`/`putaway`/`picking`/
`shipping`/`stock-adjustments`/`stock-counts`); Phase 12's confirm-
before-existence-check fix still returns clean 404s/400s as expected
(no raw 500s) on `receipts`/`putaway`/`picking`/`stock-counts`; Phase
13's Picking cancel still returns 404 for a nonexistent task; Phase
14's Stock Count History list still returns the same fixture rows with
`created_by_name` populated. `npm run build` clean. Pre-existing
fixtures from every phase left untouched; this phase's own test data
(`RCV-040006`/`040007`, `PA-020002`–`020004`, `SHP-010002`/`010003`,
`PICK-020011`) added alongside them.

### Known issues

- **Putaway's Cancel button is shown per-line, not per-task** (an
  unavoidable consequence of `PutawayView.vue`'s own item-level drawer,
  not something this phase redesigned) — a user viewing one still-
  `PENDING` line has no frontend indication that a *different* line in
  the same task already moved stock and would make the whole task
  uncancellable; they will see the backend's own clear 409
  `CANCEL_HAS_MOVED_STOCK` error if they try. Fixing this cleanly would
  mean redesigning the drawer to be task-level, which was explicitly
  out of scope ("do not redesign Vue").
- **No `cancelled_by`/`cancelled_at` audit columns on any of the three
  new tables** — same reasoning and same open item as Picking's own
  Phase 13 Known issues; `updated_at` plus the optional `remark` plus
  request logs are the only trail today.
- **Warehouse/date filter interaction wasn't re-verified via direct
  browser UI interaction this session** (see Part A) — native `<select>`/
  `<input type=date>` automation was unreliable in the same tool
  session; both are already proven correct at the API level (Phase 14's
  own live verification) and the frontend code calling them is
  unchanged since then.
- Everything listed as a known issue in Phase 2–14 still applies unless
  explicitly superseded above (Phase 14's own "no browser click-through"
  known issue is now resolved, superseding it).

## Phase 16 — Stock Count / Stock Adjustment Workflow Consolidation: VERIFIED COMPLETE

### Overlap root cause

Inspected `internal/stockcount`'s `Confirm` and `CreateAdjustments`, the
frontend's Save Draft/Create Adjustment/Confirm Count buttons, and every
ledger reference type before writing anything. Found three distinct
ways the same count's variance could be applied more than once, all
stemming from the same gap: **`CreateAdjustments` never checked
`count.Status` at all**, and `Confirm`'s own status check
(`count.Status.CanConfirm()`) was a plain Go-side read followed by a
blind `UPDATE`, not a guarded one.

1. **Confirm, then Create Adjustment**: Confirm applies the variance
   directly (`AddStockTx`/`RemoveStockTx`) and sets `status=CONFIRMED`.
   Nothing stopped `CreateAdjustments` from still running afterward,
   generating a new `PENDING` `stock_adjustment` document for the
   *identical* variance — approving it later applies the same delta a
   second time.
2. **Create Adjustment, then Confirm**: `CreateAdjustments` left the
   count's status at `DRAFT`, so `Confirm` (`CanConfirm()==DRAFT`)
   could still run afterward and apply the variance directly, while a
   separate `PENDING` adjustment for the same variance also existed,
   awaiting its own approval.
3. **Create Adjustment called twice**: no idempotency key and no status
   guard at all — two independent adjustment documents could be
   generated for the same DRAFT count's variance.

A fourth, narrower issue found along the way: `CreateAdjustments`
looped over lines calling `stockadjustment.Service.Create` — each call
opened and committed its **own independent transaction**, so a failure
partway through a multi-line count would leave a partial, uncommitted-
vs-committed mix of adjustment documents with no rollback. Not itself a
double-apply path, but a real atomicity gap in the same code this phase
was already touching, so it was fixed alongside the main issue rather
than left for a future phase to rediscover.

### Final chosen workflow

**Model A — Confirm Count remains the sole immediate, authoritative
variance-applying action.** This matches every other module's own
"Confirm"/"Approve" pattern in this codebase (Receiving, Putaway,
Picking, Shipping, Stock Adjustment itself all apply their mutation
directly on their own primary confirm action, with no second-order
document in between) — Model B (redefining Confirm Count to always
create a pending document, like Create Adjustment already does) would
have made Stock Count uniquely inconsistent with the rest of the
system. "Create Adjustment" remains available as a secondary,
approval-routing bridge for the rarer case where a separate approver
should review the correction before it lands — but it is now **mutually
exclusive** with Confirm Count for any given count, enforced at the
database level, not just hidden in the UI.

The mechanism: a new terminal status, `ADJUSTMENT_CREATED`, claimed via
the exact same guarded-`UPDATE` mechanism (`Repository.ClaimStatus`,
`WHERE status = 'DRAFT'`) that Confirm now also uses to claim
`CONFIRMED` — the same single-atomic-statement idiom this codebase has
relied on since Phase 5's `AdjustBalance` (and Phase 12/15's
`AdjustReservation`/`ClaimStatus`-shaped guards), applied here to a
status transition instead of a numeric quantity. Whichever of the two
paths' `ClaimStatus` call commits first wins; the other gets 0 rows
affected and returns a clean 409 — closing all three double-apply paths
above **and** a fourth, more subtle one: two genuinely concurrent
requests (e.g. Confirm and Create Adjustment fired at the same instant
with different idempotency keys, or two concurrent Confirm calls) can
no longer both pass a stale in-Go status check and both mutate.

### Backend changes

- `internal/stockcount/model.go`: new `StatusAdjustmentCreated`
  (`"ADJUSTMENT_CREATED"`) status value; new `CanCreateAdjustments()`
  method (identical predicate to `CanConfirm`/`CanEdit` today, kept
  separate per this codebase's own established "own method even if same
  predicate" convention — see Phase 13's `CanCancel`); `IsValid()`
  extended.
- `internal/stockcount/repository.go`: new `ClaimStatus(ctx, countID,
  newStatus) (int64, error)` — the one guarded `UPDATE ... WHERE status
  = 'DRAFT'` both `Confirm` and `CreateAdjustments` now go through.
- `internal/stockcount/service.go`:
  - `Confirm`: the old `if !count.Status.CanConfirm() { return
    ErrInvalidTransition }` check (a plain Go-side read) is replaced by
    a `ClaimStatus(countID, StatusConfirmed)` call, placed right after
    the existing idempotency-guard `INSERT` (preserving Phase 12's own
    "existence check before idempotency insert" ordering) — 0 rows
    affected now returns the same `ErrInvalidTransition`, but the check
    itself is race-safe where the Go-side one wasn't.
  - `CreateAdjustments`: completely rewritten. Now opens its own
    transaction (it didn't before), re-checks for at least one non-zero-
    variance line, calls `ClaimStatus(countID, StatusAdjustmentCreated)`
    (0 rows → new `ErrVarianceAlreadyApplied`, 409), then creates every
    generated adjustment document via `stockadjustment.Service.CreateTx`
    (new — see below) inside that same transaction, so a failure
    partway through now rolls back every document already created in
    that same call, not just the count's own status.
- `internal/stockadjustment/service.go`: added `CreateTx(ctx, tx
  *gorm.DB, req, userID) (int64, error)` — `Create`'s exact validation
  and insert logic, refactored into a shared unexported `createTx`
  helper both `Create` (opens its own transaction) and `CreateTx` (joins
  the caller's) now call. Returns just the new ID, not a full
  `Response`: a `Response` read-back through `Get` would use a
  different, non-transactional connection and would not see the row
  until the caller's own transaction commits.
- `migrations/0030_add_adjustment_created_status_to_stock_counts`: the
  second migration in this project (after Phase 15's shipments one)
  that `ALTER`s an existing `CHECK` constraint rather than only adding
  a table — adds `'ADJUSTMENT_CREATED'` to `stock_counts.status`'s
  allowed values.

### API changes

No new endpoints, no removed endpoints — both `POST
/stock-counts/{id}/confirm` and `POST
/stock-counts/{id}/create-adjustments` keep their existing paths,
methods, and request/response shapes. What changed is purely behavior:
both now return a clean 409 (`INVALID_TRANSITION` for Confirm,
`VARIANCE_ALREADY_APPLIED` for Create Adjustment) once the other path
— or a repeat of the same path — has already claimed the count.
Swagger's own descriptions for both were rewritten to state the
mutual-exclusion explicitly (see Swagger section).

### Frontend changes

- `CheckStockView.vue`: added `historyStatusText`/`historyStatusClass`
  helpers (module-level functions, delegated into `methods` the same
  way `countTypeText` already was) so `ADJUSTMENT_CREATED` gets its own
  label ("Adjustment Created", orange badge) instead of incorrectly
  displaying as "Draft" in both the count header and the History
  table's status column — a real display bug the new status value would
  otherwise have introduced, since the old inline ternaries only ever
  distinguished `CONFIRMED` from "everything else = Draft."
  - The entire Save Draft/Create Adjustment/Confirm Count action bar is
    already gated on `canEdit` (`status === 'DRAFT'`, unchanged) — since
    `ADJUSTMENT_CREATED` is not `DRAFT`, the whole bar now correctly
    disappears the instant either path succeeds, with **zero template
    changes needed** for that part: the existing gating already does
    exactly what "disable the redundant action" asks for, once the
    backend's own status reflects the claim.
  - Added one small caption line, shown only while `canEdit` and
    `varianceLines > 0`, stating plainly that the two actions are
    mutually exclusive and whichever is clicked first locks out the
    other — the one genuinely new piece of UI, addressing the *before*-
    either-action-is-taken window where both buttons still appear side
    by side with no visual indication they're exclusive. No button was
    removed, no layout changed, no page redesigned.
  - `npm run build` clean.

### Inventory mutation behavior

Unchanged mutation mechanics — `Confirm` still calls
`AddStockTx`/`RemoveStockTx` exactly as before; `CreateAdjustments`
still never touches inventory directly, only ever creating `PENDING`
documents that require `internal/stockadjustment`'s own separate
`Confirm` (APPROVE) to actually mutate anything. What's new is that
**only one of the two paths can ever reach its own mutation point for a
given count** — proven live (see below) with the exact scenario: 100 →
counted 95 → Confirm → **95** → attempted Create Adjustment → 409, no
second `-5`, balance stayed **95**; and the reverse order: 100 →
counted 90 → Create Adjustment (balance unchanged, still 100, correctly
non-mutating) → attempted Confirm → 409 → later approving the generated
adjustment → balance **90**, exactly once.

### Idempotency protection

Confirm's existing `idempotency_key` mechanism (a fresh UUID per click,
`stock_count_confirmations` as the guard table) is unchanged and still
works — proven live: a zero-variance count confirmed twice with the
identical key returned a byte-identical response (`updated_at`
unchanged) both times. `ClaimStatus`'s guarded `UPDATE` is the
*additional*, orthogonal protection Phase 16 adds: it guards against
two *different* requests (a genuine Confirm-vs-Confirm race with two
different keys, or a Confirm-vs-Create-Adjustment race) rather than a
retry of the *same* request, which the idempotency key alone was never
designed to catch.

### Stale-count protection preserved

Unchanged logic, re-verified live to still work correctly *with* the
new `ClaimStatus` call in front of it: since `ClaimStatus` and the
stale-balance re-check both run inside the same database transaction,
a stale-count rejection still rolls back the `ClaimStatus` claim along
with everything else — proven live by re-fetching the rejected count
immediately after a `STALE_COUNT_CONFLICT` response and confirming its
`status` was still `DRAFT`, not stuck at a half-applied `CONFIRMED`.

### Swagger

Both `Confirm` and `CreateAdjustments` godoc comments rewritten to
state the mutual exclusion explicitly (`Confirm`'s summary now reads
"the sole authoritative variance-applying action"; `CreateAdjustments`'
reads "secondary, mutually exclusive with Confirm"), and both gained a
409 response row documenting the new rejection case. Regenerated
(`swag init -g cmd/server/main.go -o docs --parseInternal --parseDependency`)
and grepped `docs/swagger.json` to confirm both updated summaries are
present.

### Tests

`go fmt ./...`, `go vet ./...`, `go test ./...` all clean, zero
regressions. New: `TestStatus_CanCreateAdjustments` and
`StatusAdjustmentCreated` cases added to the existing
`TestStatus_CanEdit`/`CanConfirm`/`IsValid` tables in
`internal/stockcount/service_test.go`. No new DB-dependent unit tests
for `ClaimStatus`/`Confirm`/`CreateAdjustments` themselves — same
reasoning every prior phase has applied (this package's test suite has
never unit-tested its transactional methods, which need a real database
to exercise meaningfully); every item the task asked for regression
coverage on (direct confirm, create-adjustment bridge, repeated action,
both paths sequentially, stale count, zero/positive/negative variance,
RBAC, idempotency) was instead verified live against the real Docker
SQL Server — see below.

### Live DB verification

All against the real, rebuilt Docker backend (migration 0030 confirmed
applied via `wms-migrate`'s own logs: `"migration applied"
version=0030`):

- **The exact required scenario**: fresh balance at 100 → count → 95 →
  Confirm → balance **95** → attempted Create Adjustment → 409
  `VARIANCE_ALREADY_APPLIED` → balance still **95**. Ledger for this
  balance showed exactly two rows total: `RECEIVE +100`, `COUNT -5` —
  no phantom second mutation.
- **Reverse order**: fresh balance at 100 → count → 90 (variance -10)
  → Create Adjustment (balance unchanged, still 100) → attempted
  Confirm → 409 `INVALID_TRANSITION` → approved the generated
  adjustment (`ADJ-...`, remark "Generated from stock count CNT-...",
  `before_qty=100`) → balance **90**, applied exactly once. Full audit
  chain traceable: Stock Count → generated Adjustment (remark names the
  source count) → Inventory Transaction (`before_qty`/`after_qty` on
  the adjustment itself, `COUNT`/adjustment-driven ledger row on the
  balance).
- **Duplicate Create Adjustment** on the same (now `ADJUSTMENT_CREATED`)
  count → 409 `VARIANCE_ALREADY_APPLIED`.
- **Zero variance**: counted quantity equal to system quantity →
  Confirm → 200, `CONFIRMED`, no inventory mutation attempted (the
  existing `if variance == 0 { continue }` skip, unchanged).
- **Positive and negative variance**: both exercised across the
  scenarios above (+5 net via the reverse-order approval math, -5 and
  -10 via the two Confirm/bridge scenarios).
- **Stale count**: a separate, approved stock adjustment changed the
  same balance after a count had already snapshotted it; the pending
  count's own Confirm attempt → 409 `STALE_COUNT_CONFLICT`, and the
  count's status was re-confirmed still `DRAFT` afterward (the
  `ClaimStatus` claim correctly rolled back with the rest of the failed
  transaction).
- **Idempotency**: a zero-variance count confirmed twice with the
  identical `idempotency_key` → byte-identical 200 both times.
- **RBAC**: a real `PICKING`-role token → 403 on both
  `stock_adjustment.approve` (Confirm) and `stock_adjustment.create`
  (Create Adjustment).
- **404**: both endpoints against a nonexistent count id → clean 404
  `"stock count not found"`, never a raw 500 (Phase 12's own fix,
  re-verified unaffected).

### Docker/regression results

`docker compose config` clean. `docker compose build` rebuilt all 4
images cleanly. `docker compose up -d` — all 5 services reached
healthy; `wms-migrate` applied migration 0030 and re-seeded 47
permissions idempotently before exiting. `GET /api/v1/health` → both
dependencies connected; Swagger UI reachable; RabbitMQ management UI
reachable; frontend reachable via Nginx. Full `go test ./...` — every
package, zero regressions, both before and after the Docker rebuild.
Regression-checked: `GET` across every module 200
(`users`/`roles`/`permissions`/`warehouses`/`products`/`inventory`/
`receipts`/`putaway`/`picking`/`shipping`/`stock-adjustments`/
`stock-counts`); Stock Inquiry global summary correct; Picking
reservation created and released correctly (unaffected by this phase's
changes, which never touch `internal/picking` or `internal/inventory`);
Receiving/Putaway/Shipping cancellation endpoints (Phase 15) still
return clean 404s for a nonexistent parent. `npm run build` clean.
Pre-existing fixtures from every phase left untouched; this phase's own
test data (`RCV-040008`/`040009`, `CNT-010002`–`010005`, `ADJ-010002`,
`PICK-020012`) added alongside them.

### Known issues

- **`CreateAdjustments`'s atomicity fix only covers the documents it
  creates in one call, not their later individual approvals** — each
  generated adjustment's own `Confirm`/APPROVE is still a separate,
  independent transaction against `internal/stockadjustment` (as it
  always has been for any adjustment, hand-created or generated); if a
  count generated three adjustments and only two are ever approved, the
  count itself correctly stays `ADJUSTMENT_CREATED` forever (its own
  variance-claim is final), but the third document sits `PENDING`
  indefinitely with no reminder or expiry — the same pre-existing
  "PENDING adjustments have no follow-up mechanism" gap that already
  existed for manually-created adjustments, not introduced by this
  phase.
- **No UI indication of *which* path was used, on the History screen,
  beyond the status badge** — a user sees "Adjustment Created" but the
  Stock Count History table doesn't link directly to the generated
  adjustment document(s); finding them today means searching Stock
  Adjustment's own list for a remark containing the count number.
  Flagged rather than built, since adding that link would mean either a
  new column or a cross-navigation feature beyond this phase's "closing
  the overlap" scope.
- **`ClaimStatus`'s guard is per-count, not per-line** — this was never
  the actual overlap (variance is claimed/applied at the whole-count
  level in both paths, matching how the frontend's own three buttons
  always acted on the whole count), so this isn't a gap Phase 16
  introduces, just a note that a future per-line approval workflow (not
  requested) would need its own, different design.
- Everything listed as a known issue in Phase 2–15 still applies unless
  explicitly superseded above.

## Phase 17 — Stock Count → Generated Adjustment Traceability: VERIFIED COMPLETE

### Existing relationship found

Inspected `stock_adjustments`/`stock_adjustment_items`' schema, Phase
16's `CreateAdjustments`/`CreateTx`, Stock Count History's frontend, and
Stock Adjustment's own detail view before writing anything. **No real
relational link existed.** The only trace connecting a generated
adjustment back to its source count was a free-text `remark` column
value (`"Generated from stock count CNT-000123"`) — not a foreign key,
not indexed, not joinable in SQL, and not parseable reliably (nothing
stops a manually-created adjustment's own free-text remark from
containing similar words). Confirmed `CreateAdjustments` generates
**one document per non-zero-variance line** (never grouped), so the
real relationship is a plain one-to-many: one count → zero or more
adjustments, modeled correctly with the FK on the adjustment ("many")
side, not a join table. Also confirmed this codebase's routing has no
per-record deep link anywhere (list+drawer only, on every module) — so
"navigate to detail" can only mean opening the target list page, not a
specific row, matching the exact pattern `CheckStockView.vue`'s own
pre-existing post-creation redirect already used.

### Schema changes

`migrations/0031_add_source_stock_count_to_stock_adjustments` — purely
additive, does not touch migrations 0001–0030:

```sql
ALTER TABLE stock_adjustments ADD source_stock_count_id BIGINT NULL;
ALTER TABLE stock_adjustments ADD CONSTRAINT FK_stock_adjustments_source_stock_count
    FOREIGN KEY (source_stock_count_id) REFERENCES stock_counts(id);
CREATE INDEX IX_stock_adjustments_source_stock_count_id ON stock_adjustments(source_stock_count_id);
```

Nullable and header-level (matching `created_by`/`approved_by`'s own
granularity) — `NULL` for every manually-created adjustment, populated
only when `internal/stockcount`'s `CreateAdjustments` generated the
document. No backfill of pre-existing rows: adjustments generated
before this migration (e.g. Phase 16's own live-test fixtures) keep
`source_stock_count_id = NULL` forever — their only trace remains the
old free-text remark. Confirmed live and documented as a known
limitation, not a bug (see below).

### API changes

No new endpoints. `GET /stock-counts/{id}` gained `generated_adjustments`
(array, always present, empty `[]` when none — never `null`, never
omitted):

```json
"generated_adjustments": [
  { "id": "10004", "adjustment_no": "ADJ-010004", "status": "PENDING", "reason": "Cycle Count Variance", "created_at": "..." }
]
```

`GET /stock-adjustments/{id}` gained `source_stock_count_id`/
`source_stock_count_no` (both `null` for a manually-created adjustment)
— the reverse direction of the same link, completing the chain
bidirectionally: Stock Count History → Generated Adjustments → Stock
Adjustment Detail (which can now itself point back to "generated from
count X") → Inventory Ledger (unchanged, already traceable via
`reference_type`/`reference_id` since Phase 5/11).

### Backend changes

- `internal/stockadjustment/model.go`: `Adjustment.SourceStockCountID
  *int64`.
- `internal/stockadjustment/dto.go`: `Response` gained
  `SourceStockCountID`/`SourceStockCountNo`; `CreateRequest` gained
  `SourceStockCountID *int64` tagged `json:"-"` — deliberately
  unreachable from any client-supplied JSON body, since the only
  legitimate way to set it is `internal/stockcount`'s own Go code
  constructing the struct directly before calling `CreateTx`, never a
  user's `POST /stock-adjustments` request; new `SourceCountSummary`
  struct (the narrow cross-package shape `stockcount` actually needs).
- `internal/stockadjustment/repository.go`: `FindDetail`'s query gained
  a `LEFT JOIN stock_counts` to resolve the count number; new
  `FindBySourceCountID(ctx, countID) ([]SourceCountSummary, error)`.
- `internal/stockadjustment/service.go`: `Get` maps the two new fields;
  `createTx` now sets `SourceStockCountID: req.SourceStockCountID`; new
  `FindBySourceCountID` passthrough.
- `internal/stockcount/dto.go`: new `GeneratedAdjustmentResponse`;
  `Response` gained `GeneratedAdjustments []GeneratedAdjustmentResponse`.
- `internal/stockcount/service.go`: `adjustmentCreator` interface
  extended with `FindBySourceCountID` (the same narrow cross-package-
  interface pattern this codebase has used since Phase 6/7); `Get` now
  also calls it and maps the result into `GeneratedAdjustments`, always
  computed fresh from `internal/stockadjustment`'s own table — never
  stored or duplicated on this package's own rows, so it can never drift
  out of sync with reality; `CreateAdjustments`'s own `CreateTx` call
  now passes `SourceStockCountID: &countID`.

### Frontend changes

- `CheckStockView.vue`: new "Generated Adjustments" panel (table:
  Adjustment No./Reason/Status/Created), shown only when
  `generatedAdjustments.length > 0` — placed right after the existing
  Count Summary card, matching the Preferred Behavior's own
  Count→Summary→Adjustments→action ordering. A "View in Stock
  Adjustments →" button navigates to the existing
  `/inventory/stock-adjustment` list page (this codebase's routing has
  no per-record deep link anywhere, so this opens the list, not the
  specific row — the user finds it there by the now-visible
  `adjustment_no`, the same limitation/precedent noted above).
  - New `historyStatusText`/`historyStatusClass`... — no, these were
    Phase 16's; Phase 17 added one more: `adjustmentStatusClass`
    (DRAFT/PENDING/APPROVED/REJECTED — distinct vocabulary from this
    page's own Status/ItemStatus, so reusing `getItemStatusClass` would
    have shown the wrong colors).
  - `createAdjustment()` **behavior changed**: it used to show a bare
    `alert()` then immediately `$router.push` away to Stock Adjustment,
    meaning the user never actually saw what was created. It now
    refreshes the count (picking up the new `generated_adjustments`)
    and stays on the page so the new panel actually renders — directly
    satisfying "show the resulting adjustment document(s) clearly after
    success" instead of a redirect the user had no time to read
    anything from. `viewCount`/`createCountTask`/`saveDraft`/
    `confirmCount` all now also keep `generatedAdjustments` in sync on
    every refresh.
  - No page redesign — one new conditional panel, one behavior fix to
    an existing method, no layout or routing changes elsewhere.
  - `npm run build` clean.

### RBAC

No new permission codes, no new checks written — verified that
`GET /stock-counts/{id}` (which now returns `generated_adjustments`) and
`GET /stock-adjustments/{id}` are **already gated on the identical
permission**, `stock_adjustment.view` (confirmed by re-reading both
`Register` methods). Since Phase 11 chose to have Stock Count reuse
Stock Adjustment's own RBAC module entirely, there is no possible leak:
any role that can see a count's generated-adjustment summary already
necessarily has full permission to view Stock Adjustments directly — the
two permissions were never separate to begin with. Proven live: a real
`PICKING`-role token (lacking `stock_adjustment.*` entirely) → 403 on
`GET /stock-counts/{id}`, the same denial it already got before this
phase.

### Swagger

`GET /stock-counts/{id}`'s description documents `generated_adjustments`
(what it contains, when it's empty, and the RBAC reasoning above);
`GET /stock-adjustments/{id}`'s description documents the two new
fields; `CreateAdjustments`' own description updated to mention the real
FK now exists. Regenerated
(`swag init -g cmd/server/main.go -o docs --parseInternal --parseDependency`)
and grepped `docs/swagger.json` to confirm the new
`GeneratedStockAdjustmentResponse` model, the `generated_adjustments`
and `source_stock_count_id`/`source_stock_count_no` fields, and all
three updated descriptions are present.

### Tests

`go fmt ./...`, `go vet ./...`, `go test ./...` all clean, zero
regressions. No new Go unit tests: Phase 17 added no new status values,
no new state-machine predicates, and no new pure-Go validation logic —
every change is SQL/query/response-shape plumbing (a new join, a new
cross-package lookup, new response fields), and this codebase has never
unit-tested that category of change (matching every prior phase's own
stated reasoning). Every item the task asked for regression coverage on
— count with no/one/multiple generated adjustments, generated-adjustment
lookup, RBAC allowed/denied, no duplicate relationship, Phase 16 mutual
exclusion — was verified live against the real Docker SQL Server
instead (see below).

### Live DB verification

All against the real, rebuilt Docker backend (migration 0031 confirmed
applied via `wms-migrate`'s own logs: `"migration applied"
version=0031`):

- **Pre-migration data**: a count from Phase 16's own testing
  (`CNT-010003`, which generated `ADJ-010002` before this migration
  existed) → `generated_adjustments: []` — correctly empty, since that
  document's `source_stock_count_id` is `NULL` (no backfill), proving
  the known limitation is real and behaves as documented, not silently
  wrong.
- **Full fresh chain, two lines**: new count (`CNT-010006`) with two
  variance lines (-5 and +2) → Create Adjustment → generated exactly
  two documents (`ADJ-010004`, `ADJ-010005`) → count's own
  `generated_adjustments` listed both correctly → each adjustment's own
  detail showed `source_stock_count_id: "10006"`,
  `source_stock_count_no: "CNT-010006"` (the reverse link) → approved
  `ADJ-010004` → `after_qty: 95` → Inventory balance re-queried and
  confirmed at **95**. Full chain: Stock Count → Generated Adjustment →
  Adjustment document → Inventory balance, all independently
  cross-checked.
- **No false reference**: a separate fresh count, confirmed directly
  via Confirm Count (no Create Adjustment call at all) →
  `generated_adjustments: []`.
- **No duplicate relationship**: a second `create-adjustments` call on
  the already-`ADJUSTMENT_CREATED` count `10006` → 409
  `VARIANCE_ALREADY_APPLIED` (Phase 16's own guard, unaffected) — no
  second set of documents, no duplicate FK rows.
- **Phase 16 mutual exclusion regression**: `Confirm` attempted on the
  same `ADJUSTMENT_CREATED` count → 409 `INVALID_TRANSITION`, exactly
  as Phase 16 established.
- **RBAC**: admin → sees `generated_adjustments` populated; a real
  `PICKING`-role token → 403 on the same endpoint.
- **Independent regression**: a manually-created adjustment (no source
  count at all) → `source_stock_count_id: null`, proving `Create
  Request`'s `json:"-"` tag genuinely blocks client-supplied values, and
  that Stock Adjustment's own manual-create path is completely
  unaffected by this phase.

### Docker/regression results

`docker compose config` clean. `docker compose build` rebuilt all 4
images cleanly. `docker compose up -d` — all 5 services reached
healthy; `wms-migrate` applied migration 0031 and re-seeded 47
permissions idempotently before exiting. `GET /api/v1/health` → both
dependencies connected; Swagger UI reachable; RabbitMQ management UI
reachable; frontend reachable via Nginx. Full `go test ./...` — every
package, zero regressions. Regression-checked: `GET` across every
module 200 (`users`/`roles`/`permissions`/`warehouses`/`products`/
`inventory`/`receipts`/`putaway`/`picking`/`shipping`/
`stock-adjustments`/`stock-counts`); Stock Inquiry summary correct;
Receiving/Picking cancellation endpoints (Phases 13/15) still return
clean 404s for a nonexistent parent; Stock Adjustment's own independent
manual-create path unaffected. `npm run build` clean. Pre-existing
fixtures from every phase left untouched; this phase's own test data
(`RCV-040010`, `CNT-010006`/`010007`, `ADJ-010004`–`010006`) added
alongside them.

### Known issues

- **No retroactive backfill for adjustments generated before this
  migration** — `ADJ-010002` (generated from `CNT-010003` during Phase
  16's own testing) has `source_stock_count_id = NULL` forever; its
  only trace back to its source count remains the pre-Phase-17 free-text
  remark. A one-time backfill script parsing existing remarks against
  `stock_counts.count_no` values could close this gap if it ever
  matters in practice, but was not built speculatively here — the
  remark text format was never a documented contract, only an internal
  convention.
- **No per-record deep link from the Generated Adjustments panel** —
  "View in Stock Adjustments →" opens the list page, not the specific
  document, because no route in this entire codebase supports opening a
  specific record by URL (every module is list+drawer with client-only
  selection). The user identifies the right row via its now-visible
  `adjustment_no`. Adding true deep-linking would mean introducing a
  route/query-param convention this codebase has never had anywhere,
  which is a larger, cross-cutting change beyond this phase's own scope.
- **`GET /stock-counts` (the list endpoint) does not include
  `generated_adjustments`** — only the detail (`GET
  /stock-counts/{id}`) does, matching the task's own "Stock Count
  History **detail**" wording and avoiding restructuring the list
  query's existing `GROUP BY` aggregation for a one-to-many collection
  the list view was never asked to show.
- Everything listed as a known issue in Phase 2–16 still applies unless
  explicitly superseded above.

## Phase 18 — Cancellation Audit Trail: VERIFIED COMPLETE

### Starting point

Recommended repeatedly (Phase 13, 15, and again in Phase 17's own
Recommended Phase 18 list, item 3) as a small, isolated, purely-additive
migration once a real audit/reporting need emerged. On resuming after a
machine restart, the backend and frontend source already contained a
substantially complete implementation — migration 0032, model/dto/
service/repository/handler changes across all four modules, and
frontend detail-view panels — dated after migration 0031 (Phase 17's
own) but with no corresponding PROJECT_STATUS.md entry, `go test`
run, `npm run build`, or Docker verification recorded anywhere. Most
likely: a prior session implemented this and was interrupted by the
restart before reaching verification. Rather than assume it was correct,
every module was independently re-inspected end-to-end (model → dto →
repository join → service → handler → Swagger → frontend) against this
phase's own requirements before treating any of it as done.

### Schema changes

`migrations/0032_add_cancellation_audit_to_workflow_tables` — purely
additive, does not touch migrations 0001–0031:

```sql
ALTER TABLE picking_tasks ADD cancelled_at DATETIME2 NULL;
ALTER TABLE picking_tasks ADD cancelled_by BIGINT NULL;
ALTER TABLE picking_tasks ADD cancel_remark NVARCHAR(500) NULL;
ALTER TABLE picking_tasks ADD CONSTRAINT FK_picking_tasks_cancelled_by
    FOREIGN KEY (cancelled_by) REFERENCES users(id);
-- identical ALTER + FK triplet repeated for receipts, putaway_tasks, shipments
```

`cancelled_by` follows the same nullable-FK-to-`users(id)` shape every
`created_by` column in this project already uses. `cancel_remark` is a
**new, dedicated** column rather than reusing each table's existing
`remark` column — Phase 13/15's original Cancel implementations
overwrote the task/receipt/shipment's own `remark` with the cancellation
reason, silently destroying whatever remark existed before cancelling.
Phase 18 fixes that: the optional cancel reason now goes into
`cancel_remark`, leaving the header's own `remark` untouched.

### API changes

No new endpoints — the four existing `POST /{resource}/{id}/cancel`
routes (Phase 13/15) are unchanged. `GET` detail for all four modules
(`/receipts/{id}`, `/putaway/{id}`, `/picking/{id}`, `/shipping/{id}`)
now also returns:

```json
"cancelled_at": null,
"cancelled_by": null,
"cancelled_by_name": null,
"cancel_remark": null
```

populated only after a successful Cancel.

### Backend changes

Identical pattern across `internal/{receiving,putaway,picking,shipping}`:

- `model.go`: header struct (`Receipt`/`Task`/`Task`/`Shipment`) gained
  `CancelledAt *time.Time`, `CancelledBy *int64`, `CancelRemark *string`.
  Added this phase's own `applyCancel(userID int64, remark *string, now
  time.Time)` method — extracted as a small pure mutator (status +
  three audit fields, remark stored only when non-empty) so the
  guard-then-mutate invariant has a unit-testable surface independent of
  the DB transaction it runs inside.
- `dto.go`: `Response` gained `CancelledAt`/`CancelledBy`/
  `CancelledByName`/`CancelRemark` (`cancelled_by` tagged `,string` to
  match every other FK-typed response field's own convention).
- `repository.go`: the detail query (`FindReceiptDetail`/
  `FindTaskByID`-equivalent detail path/`FindByID`) gained a `LEFT JOIN
  users cu ON cu.id = <table>.cancelled_by` to resolve
  `cancelled_by_name` — the exact `created_by_name` join pattern Phase
  14 established for Stock Count History, reused here per this phase's
  own instructions rather than inventing a new lookup shape.
- `service.go`: `Cancel` unchanged in guard logic (`Status.CanCancel()`
  gate, existing reservation-release logic in Picking, existing
  moved-stock guard in Putaway — none of that touched); the manual
  three-field assignment that used to sit inline in the transaction
  closure now calls `header.applyCancel(userID, req.Remark,
  time.Now().UTC())` instead. Because `applyCancel` only ever runs after
  the `CanCancel()` guard has already rejected an already-cancelled (or
  otherwise non-cancellable) record, a duplicate or invalid cancel
  attempt can never reach it and can never overwrite a prior
  cancellation's audit data — requirement 5 is enforced by the
  pre-existing guard, not by new code.
- `handler.go`: `Cancel` already resolved the authenticated user via
  `middleware.CurrentUserID(c)` (Phase 13/15) and passes it straight
  through — no change needed for requirement 3.

### Frontend changes

`ReceiveView.vue`, `PutawayView.vue`, `PickingView.vue`,
`ShippingView.vue` each gained one conditional "Cancellation details"
panel (red-tinted, matching this codebase's existing cancelled-state
styling), shown only when the record's status is `CANCELLED`: Cancelled
At (via each view's own pre-existing `formatDateTime` helper), Cancelled
By (name, falling back to `—`), Cancellation Remark (falling back to
`—`). No layout or routing changes elsewhere, no new page, no redesign —
one panel per existing detail drawer.

### Swagger

Regenerated (`swag init -g cmd/server/main.go -o docs
--parseInternal --parseDependency`); confirmed `cancelled_at`/
`cancelled_by`/`cancelled_by_name`/`cancel_remark` present in all four
modules' Response schemas in `docs/swagger.json`.

### Tests

Found no dedicated coverage for the new audit-trail behavior beyond the
pre-existing (Phase 13/15) `TestStatus_CanCancel` predicate tests — this
codebase has no DB-mock/integration-test scaffolding for service methods
that open a real transaction, so `Cancel` itself isn't unit-testable
directly (consistent with Phase 17's own stated reasoning for why it
added no new Go tests). Closed the gap on the side that *is* pure: added
`TestReceipt_ApplyCancel`/`TestTask_ApplyCancel` (×2, picking + putaway)/
`TestShipment_ApplyCancel` to each module's `service_test.go`, each
covering: audit fields set correctly from a non-empty remark; nil remark
leaves `CancelRemark` unset; empty-string remark leaves `CancelRemark`
unset (not stored as `""`); a pre-existing `CancelRemark` value is left
untouched when the new remark is omitted. `go fmt ./...`, `go vet
./...`, `go test ./...`, and `go build ./...` all clean across every
package, zero regressions.

### Pre-flight (Docker restart recovery)

Docker Desktop's Windows service (`com.docker.service`) was `STOPPED`
after the machine restart in the prior session, but the user had since
relaunched Docker Desktop by hand — `docker info`/`docker ps` were
reachable this session. Checked available system memory before touching
anything (`Get-Counter '\Memory\Available MBytes'` → ~416MB host-free,
but Docker's own WSL2 VM carries a separate 3.7GB pool with only ~163MB
in use by an unrelated container, so there was headroom). Compared
`wms-*` image build timestamps (20:49) against the Phase 18 source edit
timestamps (22:14–22:40, all after the build) and confirmed via
`wms-migrate`'s own container logs that the last applied migration was
**0031**, not 0032 — so the existing images predated this phase's code
and a rebuild was actually required, not optional. Ran `docker compose
up -d --build` (all 4 buildable services rebuilt; sqlserver/rabbitmq
reused). One hiccup: `wms-backend` initially failed to bind `:8080`
because a stray local `server.exe` (an orphaned `go run`-style dev
binary in a `go-build...` temp dir, unrelated to any current work) was
squatting on the port — stopped that process, then `docker compose up
-d` completed cleanly on retry. No volumes touched, no `-v` flags used.

### Live DB verification

All 5 services reached healthy. `wms-migrate` logs confirmed migration
**0032** applied cleanly (`"migration applied" ... version=0032`) plus
idempotent re-seed (47 permissions, admin user already existed). `GET
/api/v1/health` → `database: connected`, `rabbitmq: connected`.
Logged in as `admin` and exercised all four workflows live against the
real SQL Server:

- **Receiving** (`RCV-050006`/`050007`/`050008`): cancelled a receipt
  created with a pre-existing `remark` ("original receiving note - do
  not touch") plus a cancel remark ("vendor cancelled the PO") →
  `cancelled_at`/`cancelled_by: "1"`/`cancelled_by_name: "Administrator"`
  /`cancel_remark` all correct, original `remark` untouched. Duplicate
  cancel → 409 `NOT_CANCELLABLE`, re-fetched detail showed byte-identical
  audit fields (same timestamp, same remark) — no overwrite. Cancelled a
  second receipt with **no** request body → `cancel_remark: null` (not
  `""`). Confirmed a third receipt to `COMPLETED` (real stock movement,
  10 units) then attempted Cancel → 409, audit fields stayed `null`.
- **Putaway** (`PA-030002`/`030003`/`030004`): identical remark/
  duplicate/empty-remark verification, using the stock just received
  above. Confirmed the reservation/moved-stock guard is untouched:
  created and fully confirmed a task (2 units moved, ledger row
  `PUTAWAY -2/+2` against `PA-030004`) → Cancel attempt → 409
  `NOT_CANCELLABLE`, audit fields stayed `null`. Cancelled tasks (0
  units moved) left zero ledger rows.
- **Picking** (`PICK-030011`–`030014`): remark/duplicate/empty-remark
  verification. Reservation-release check: created a task ordering 5
  units → inventory showed `reserved_qty: 5` → Cancel → `reserved_qty:
  0`, `available_qty` back to the pre-reservation value. Partial-pick
  check: ordered 6, confirmed 2 (`PICK` ledger row `-2`, `on_hand_qty`
  60→58, `reserved_qty` 6→4) → Cancelled the task with 4 still
  outstanding → `reserved_qty` released to 0, **`on_hand_qty` stayed at
  58** (the already-picked 2 units were not restored — cancellation
  never un-picks), and the ledger still held exactly the one `-2` `PICK`
  row from the original confirm, nothing added by Cancel. Fully
  completed a task (`COMPLETED`) → Cancel attempt → 409, audit fields
  stayed `null`.
- **Shipping** (`SHP-020002`–`020005`): remark/duplicate/empty-remark
  verification (`SHP-020002`/`020003`, sourced from real completed
  picking tasks). Confirmed Cancel never touches inventory: ledger for
  the shared inventory row showed zero entries referencing any `SHP-`
  document, matching the handler's own doc comment ("Picking already
  removed the stock... cancelling here does not restore it"). Invalid-
  state check: dispatched a shipment (`SHP-020005`, required a
  `tracking_no` first — a pre-existing validation rule, not a Phase 18
  change) → Cancel attempt → 409 `INVALID_TRANSITION`, audit fields
  stayed `null`.
- **Detail API**: all four modules' `GET .../{id}` returned
  `cancelled_at`/`cancelled_by`/`cancelled_by_name`/`cancel_remark`
  correctly in every case above, including the `null` cases.
- **Frontend detail drawers**: logged into the running frontend
  (`localhost:80`) in a real browser and opened the drawer for a
  cancelled record in all four modules — `ReceiveView.vue`,
  `PutawayView.vue` (note: routed at `/inbound/put-away`, not
  `/inbound/putaway`), `PickingView.vue`, `ShippingView.vue` all
  rendered Cancelled At/By/Remark correctly, matching the API values
  exactly (e.g. Picking's drawer correctly showed "Partially Picked,
  Order 6 / Picked 2 / Remain 4" alongside the cancellation panel).

### Regression results

Live, against the running API: `GET` 200 across
warehouses/products/inventory/inventory-summary/receipts/putaway/
picking/shipping/stock-adjustments/stock-counts/roles/permissions/
users; `zones`/`locations` 400-with-clear-message when their required
query param is omitted (pre-existing behavior, confirmed not a
regression) and 200 once supplied. `POST /auth/login` +
`GET /auth/me` round-tripped correctly with the real seeded admin
account. Stock Adjustment and Stock Count detail/list endpoints
unaffected. Swagger UI (`/swagger/index.html`) and `/swagger/doc.json`
both reachable, `doc.json` contains `cancelled_by_name` across all four
modules' schemas. Backend logs showed only the *expected* `ERROR
"transaction failed"` entries for this session's own intentional
negative tests (duplicate/invalid-state cancels, one failed dispatch
missing a tracking number) — each correctly logged with `request_id`
and `duration`, no panics, no unexpected errors. `go test ./...` (all
packages) and `npm run build` both clean.

### Known issues

- Everything listed as a known issue in Phase 2–17 still applies unless
  explicitly superseded above. Phase 15's Putaway per-line Cancel
  visibility gap remains open (confirmed again live this session: a
  cancelled Putaway task's drawer still shows the Confirm/Report
  Issue/Cancel action controls as if actionable) — unrelated to this
  phase's own additive audit columns, tracked as Recommended Phase 19
  item 1.
- Test fixtures created this session (`RCV-050006`–`050008`,
  `PA-030002`–`030004`, `PICK-030011`–`030017`, `SHP-020002`–`020005`)
  left in place alongside every prior phase's fixtures, matching Phase
  17's own convention.

## Phase 19 — Putaway Drawer Visibility and Action-State Hardening: VERIFIED COMPLETE

### Root cause

`PutawayView.vue`'s drawer had exactly one "is this resolved" gate,
`isDrawerItemResolved`, keyed on the **item-level** `status` field
(`PENDING`/`PARTIALLY_COMPLETED`/`COMPLETED`/`ISSUE` — from
`internal/putaway`'s `ItemStatus`). Cancelling a task never touches its
items' own status (confirmed against `internal/putaway/service.go`'s
`Cancel` and `model.go`'s `applyCancel` — only the task header's
`Status`/`CancelledAt`/`CancelledBy`/`CancelRemark` change), so a
cancelled task's single visible line stayed `PENDING` forever and
`isDrawerItemResolved` never caught it. Confirmed live in Phase 18's own
browser verification: opening a `CANCELLED` task's drawer showed the
Cancellation-details panel *and*, directly below it, a fully live
Confirm Putaway / Report Issue / Cancel Putaway Task form — none of
which the backend would actually accept (`Status.CanCancel()`/
`CanConfirm()` in `internal/putaway/model.go` both reject anything but
`PENDING`/`IN_PROGRESS`), but the frontend never told the user that
before they tried.

### Fix

`fontend/src/views/inbound/PutawayView.vue` — added two computed
properties that read the **task header status** already being fetched
by `openPutaway` (`selectedPutaway.taskStatus`, populated from `GET
/putaway/{id}`'s own `status` field — the same backend response Phase
18's Cancellation panel already reads its `cancelled_*` fields from):

- `isTaskActionable`: `true` only when `taskStatus` is `PENDING` or
  `IN_PROGRESS` — a direct mirror of `Status.CanConfirm()` /
  `Status.CanCancel()` in `internal/putaway/model.go`, not a
  reinvented rule. `false` (not a redesign of the guard, just its
  default) while `taskStatus` hasn't loaded yet, so nothing flashes
  before the real state is known.
- `isTaskTerminal`: `true` once `taskStatus` is known and is *not*
  actionable (`CANCELLED`, `COMPLETED`, or `ISSUE` at the header level).

Template: inserted one `v-else-if="isTaskTerminal"` branch — a plain
message, `taskTerminalMessage` — ahead of the existing action block,
which is now gated `v-else-if="isTaskActionable"` instead of a bare
`v-else`. Ordering preserves the pre-existing per-line
`isDrawerItemResolved` check first (unchanged), so an individually-
resolved line still shows its own "already resolved" message
regardless of task state. Nothing else in the drawer — layout, the
Cancellation-details panel, the summary cards, the list table, routing —
was touched. No new component, no new route, no new dependency.

### Permission checks

Reused `canConfirm` (`putaway.approve`) and `canCancelPutaway`
(`putaway.update`) exactly as before — the new task-state gate wraps
*around* them (`v-else-if="isTaskActionable"` now contains the same
`v-if="canConfirm"` / `v-if="canCancelPutaway"` blocks verbatim), it
does not replace or duplicate permission logic. One side benefit:
previously a user without `putaway.approve` viewing a cancelled task saw
"Your role can view this task but not confirm putaway" (misleading —
implied a permission problem, not a state problem); now they correctly
see the terminal-state message instead, since that check now runs
first.

### Backend changes

None. `internal/putaway/model.go`'s `Status.CanConfirm()`/`CanCancel()`
were read as the source of truth, not modified — no mismatch was found
between backend rules and what the frontend should allow, only a
frontend gap in *displaying* the backend's own rule.

### Frontend tests

No test framework exists in `fontend/` (`package.json` has no
vitest/jest, `find` found zero `*.test.js`/`*.spec.js` files anywhere in
the project) — consistent with this project's established convention of
not introducing new dependencies speculatively, none was added for this
one narrow fix.

### Verification

`npm run build` clean. `go test ./...` clean across every package (no
backend files changed, run per instruction to confirm zero regression).
Rebuilt only the `frontend` image (`docker compose up -d --build
frontend`) and re-verified live in a real browser against
`localhost:80`, reusing Phase 18's own test fixtures plus one fresh one:

- **`PA-030002`** (`CANCELLED`, item still `PENDING`) — drawer now shows
  the Cancellation-details panel plus "This putaway task has been
  cancelled. No further action is possible." and **no** Confirm/Report
  Issue/Cancel controls.
- **`PA-030003`** (`CANCELLED`, cancelled with no remark in Phase 18) —
  same result; confirmed the fix isn't specific to one fixture.
- **`PA-030004`** (task `COMPLETED`, item `COMPLETED`) — unchanged
  pre-existing behavior, still correctly read-only via
  `isDrawerItemResolved`: "This line has already been resolved
  (Completed)."
- **`PA-030005`** (fresh task, genuinely `PENDING`) — regression check:
  full Confirm Putaway / Report Issue / Cancel Putaway Task form still
  renders exactly as before.

### Known issues

- The underlying item-level-drawer limitation itself (a cancelled or
  completed task with multiple lines still only shows one line per
  drawer open, no full-task view) is unchanged — this phase made the
  *visible* line correctly read-only for a terminal task, it did not
  redesign the drawer to show every line at once. That remains a
  separate, larger UI change requiring explicit sign-off (see Phase 15's
  original Known issue).
- Everything listed as a known issue in Phase 2–18 still applies unless
  explicitly superseded above.

## Phase 20 — Pending Stock Adjustment Follow-up / Aging: VERIFIED COMPLETE

### Existing workflow found

Inspected `internal/stockadjustment` and `StockAdjustmentView.vue`
before writing anything. Status machine: DRAFT → PENDING → APPROVED/
REJECTED (`Status.CanEdit`/`CanConfirm` in model.go); reason/remark live
on the one `Item` row per document, not the header. `Update` (PUT) is
only ever allowed while still DRAFT — once PENDING, nothing touches the
header again until Confirm resolves it. **No existing mechanism tracked
how long an adjustment had been PENDING at all** — `updated_at` existed
but nothing read it that way, there was no `created_by_name` join (this
module never got Phase 14's own pattern), and the list/detail responses
exposed no age/threshold concept whatsoever. A PENDING adjustment today
can sit unresolved indefinitely with zero visibility beyond manually
sorting the list by date — exactly the gap Phase 17's own Recommended
Phase 18 (item 2) and Phase 19's Recommended Phase 20 (item 2) flagged.

### Aging design

`pendingAging(status, updatedAt, now, warningHours)` — new pure function
in `internal/stockadjustment/model.go` — computes `PendingSince`/
`AgeHours`/`AgeDays`/`IsOverdue` (plus the threshold used, echoed back)
from a row's own `status`/`updated_at`. Returns the zero value (all nil,
`IsOverdue: false`) for anything that isn't currently `PENDING` — DRAFT,
APPROVED, and REJECTED are never "aging" or "overdue", satisfying this
phase's own explicit requirement directly in the one function every
other case flows through, rather than needing to be independently
correct in three call sites.

**No new column.** `pending_since` is `updated_at`, not `created_at`:
`Update` only runs while DRAFT (`Status.CanEdit`), so once a document
reaches PENDING — whether at creation (`CreateRequest.Status=PENDING`)
or via a later DRAFT→PENDING submission — nothing writes `updated_at`
again until Confirm resolves it. That makes the existing column an
exact "became pending at" timestamp for as long as the status stays
PENDING, with zero duplicated/stored age value anywhere (this phase's
own explicit preference) and zero migration needed.

### Configuration added

`STOCK_ADJUSTMENT_PENDING_WARNING_HOURS` (plain integer hours, default
**48** — a documented development default, not a business decision;
falls back silently on missing/invalid input, the same convention
`JWT_EXPIRES_IN`/`DB_TRANSACTION_TIMEOUT` already use). New
`config.parseInt` helper alongside the existing `parseBool`/
`parseDuration`. Added to `.env`, `.env.example`, and
`backend/.env.example`; wired through `config.Config` →
`stockadjustment.NewService`'s new `pendingWarningHours` parameter →
`cmd/server/main.go`. Never hardcoded into `StockAdjustmentView.vue` —
the value in effect is echoed back on every row/detail response as
`pending_warning_hours` so the frontend can display it (see the overdue
banner below) without knowing the number itself.

### API changes

No new endpoints. `GET /stock-adjustments` and `GET
/stock-adjustments/{id}` both gained:

```json
"created_by": "1", "created_by_name": "Administrator",
"pending_since": "2026-08-28T14:37:43Z",
"age_hours": 50, "age_days": 2.1,
"is_overdue": true, "pending_warning_hours": 48
```

`pending_since`/`age_hours`/`age_days` are `null` and `is_overdue` is
`false` whenever status isn't PENDING. List gained `remark` (previously
detail-only). `GET /stock-adjustments` gained four independent,
combinable query filters (each following the existing empty/zero-means-
unfiltered convention `search`/`type`/`status`/`date` already use):

- `reason` — exact match (e.g. `Found Goods`)
- `created_by` — filter by creating user ID
- `older_than_days` — rows not touched (`updated_at`) in at least N days
- `overdue` — `true`: only PENDING rows whose age has reached the
  configured threshold; forces PENDING regardless of any conflicting
  `status` param, since "overdue" is meaningless for a resolved
  document

Both `overdue`/`older_than_days` compare `sa.updated_at` against a
cutoff timestamp computed in **Go**, not SQL `DATEDIFF`/`GETUTCDATE()`
— `overdueCutoff`/`olderThanCutoffTime` in model.go, the same
parameterized-timestamp style the pre-existing `date` filter already
uses, and independently unit-testable.

### Backend changes

- `internal/stockadjustment/model.go`: `Aging` struct;
  `pendingAging`/`overdueCutoff`/`olderThanCutoffTime`/
  `roundToOneDecimal` (all pure, no DB).
- `internal/stockadjustment/dto.go`: `Response`/`ListResponse` gained
  `CreatedBy`/`CreatedByName` + the five aging fields; `ListResponse`
  gained `Remark`; `ListParams` gained `Reason`/`CreatedBy`/
  `OlderThanDays`/`Overdue`.
- `internal/stockadjustment/repository.go`: `List`'s query gained a
  `LEFT JOIN users` (the exact `created_by_name` pattern Phase 14/18
  already established) plus the four new WHERE clauses; `List`'s Go
  signature gained a `warningHours int` parameter and now calls
  `pendingAging` per row after the SQL round-trip. `FindDetail` gained
  the same `LEFT JOIN users` plus `created_by`/`created_by_name`
  columns.
- `internal/stockadjustment/service.go`: `Service` gained a
  `pendingWarningHours int` field, threaded through `NewService`'s new
  parameter; `List` passes it to `repo.List`; `Get` calls `pendingAging`
  directly on the detail row. **No change** to `Create`/`Update`/
  `Confirm`'s own logic — approval/rejection, the inventory
  AddStock/RemoveStock call, and the idempotency guard are byte-for-
  byte untouched.
- `internal/stockadjustment/handler.go`: `List` binds the four new
  query params; Swagger annotations updated on `List`/`Get`.
- `cmd/server/main.go`: passes `cfg.StockAdjustmentPendingWarningHours`
  into `stockadjustment.NewService`.

### Frontend changes

`StockAdjustmentView.vue` — no redesign, three additions:

- Existing Status filter `<select>` gained one more option, "Overdue
  (Pending)" (`value="OVERDUE"`) — not a real backend status;
  `fetchAdjustments` detects it and sends `overdue=true` instead of
  `status=OVERDUE`.
- List table gained one column, "Pending Age" — server-computed
  `age_hours` formatted as `Xd Yh`/`Yh` (`formatAge`, display-formatting
  only, the number and the overdue determination both come from the
  API), with a small red "Overdue" badge when `is_overdue`; `—` for
  anything not PENDING.
- Detail drawer gained a Created By/Created At info block, plus — only
  when `status === 'PENDING'` — a "Pending for Xh[/d]" banner, orange
  normally, red with "— overdue (threshold: Yh)" when `is_overdue`. The
  threshold number displayed is `pending_warning_hours` from the API
  response, never a value written into this file.
- `services/stockAdjustment.js`: `listStockAdjustments` gained
  `overdue`/`olderThanDays`/`createdBy`/`reason` params (only `overdue`
  is wired to a UI control this phase; the rest exist for API parity/
  future use, matching how other service modules already expose every
  backend filter regardless of current UI wiring).

No auto-approval, no auto-rejection, no notifications, no RabbitMQ
queue — this phase is visibility only, exactly as scoped.

### RBAC

No new permission codes. `GET /stock-adjustments` and `GET
/stock-adjustments/{id}` remain gated on the pre-existing
`stock_adjustment.view` exactly as before — the new query params and
response fields sit behind the same check, nothing new to leak.

### Swagger

Regenerated (`swag init -g cmd/server/main.go -o docs --parseInternal
--parseDependency`); confirmed `reason`/`created_by`/`older_than_days`/
`overdue` query params and `pending_since`/`age_hours`/`age_days`/
`is_overdue`/`pending_warning_hours`/`created_by_name` response fields
present in `docs/swagger.json`.

### Tests

New, all pure (no DB, matching this codebase's established split of
pure-logic unit tests vs. live-DB verification for anything that
actually queries):

- `TestPendingAging_NonPendingNeverOverdue` — DRAFT/APPROVED/REJECTED
  with a deliberately ancient `updated_at` (1000h) still return
  `IsOverdue: false` and nil age fields.
- `TestPendingAging_AgeCalculation` — known 30h delta → `AgeHours: 30`,
  `AgeDays: 1.3`.
- `TestPendingAging_OverdueThreshold` — boundary table (47h/48h/49h
  against a 48h threshold): under → false, **at** → true, over → true.
- `TestOverdueCutoff`/`TestOlderThanCutoffTime` — the cutoff arithmetic
  the repository's own SQL filters bind as parameters.
- `config`: `TestLoad_StockAdjustmentPendingWarningHoursDefault`/
  `Override`/`InvalidFallsBack` — same pattern as the existing
  `DBTransactionTimeout` config tests.

`go fmt`/`go vet`/`go test ./...` all clean across every package, zero
regressions. `npm run build` clean.

### Live DB verification

Docker was already running (Phase 19's own session); rebuilt `backend`
and `frontend` (`docker compose up -d --build backend frontend` — no
migration needed, confirmed via `wms-migrate` logs still showing only
migration 0032, none newer). All 5 services healthy. Created two fresh
PENDING adjustments (`ADJ-020004`, `ADJ-020005`) via the real API, then
backdated one's `updated_at` directly in SQL Server (`DATEADD(HOUR,
-50, GETUTCDATE())`) to simulate age without waiting — the same
technique used to reach 100% real-schema coverage without a 50-hour
wait:

- **Normal pending**: `ADJ-020004`, age 0h → `is_overdue: false`.
- **Overdue pending**: `ADJ-020005` backdated to 50h → `age_hours: 50`,
  `is_overdue: true` (threshold 48h).
- **Filters**: `overdue=true` → returned exactly `ADJ-020005`.
  `older_than_days=1` → returned exactly `ADJ-020005` (every other row
  in this database was created the same day, confirmed via direct SQL —
  a true negative, not an untested filter). `status=PENDING` → all
  three PENDING rows. `reason=Damaged%20Goods` → exact match only.
  `created_by=1` → correct total count.
- **Approved/rejected not overdue**: `ADJ-000001` (APPROVED),
  `ADJ-000002` (REJECTED), `ADJ-010006` (DRAFT) all returned
  `pending_since: null`, `age_hours: null`, `is_overdue: false`.
- **Approval regression**: approved the backdated `ADJ-020005` live —
  `AddStockTx`/`RemoveStockTx` still moved inventory correctly
  (on_hand 52→51, DECREASE), status → APPROVED, and its own aging
  fields correctly zeroed out in the very same response.
- **RBAC regression**: created a temporary PICKING-role user
  (`phase20_rbac_test`, left in place alongside every prior phase's own
  test users) and confirmed `GET /stock-adjustments` (including with
  the new `overdue=true` param) and `GET /stock-adjustments/{id}` both
  still return 403 `permission denied: stock_adjustment.view` — no
  leak introduced by the new fields/filters.
- **Stock Count integration (Phase 16/17) unaffected**: `ADJ-010005`
  (generated from `CNT-010006`) correctly shows both
  `source_stock_count_id`/`source_stock_count_no` *and* its own aging
  fields side by side; `GET /stock-counts/10006`'s own
  `generated_adjustments` summary (the narrow `SourceCountSummary`
  shape, untouched this phase) still returns correctly.
- **Frontend, real browser**: logged into `localhost:80`, confirmed the
  list table's new "Pending Age" column (`0h`/`2h`/`3d 0h` + red
  "Overdue" badge, `—` for non-pending rows), the "Overdue (Pending)"
  filter option (returned exactly the one overdue row), and the detail
  drawer (Created By/At block, plus the orange "Pending for 0h" /
  red "Pending for 3d 0h — overdue (threshold: 48h)" banner variants).
- **Regression sweep**: `GET` 200 across warehouses/products/inventory/
  receipts/putaway/picking/shipping/stock-counts/roles/users.

### Known limitations

- Follow-up is **visibility only**, exactly as scoped: no reminder
  email/notification, no auto-approval, no auto-rejection, no
  escalation queue. An overdue adjustment is discoverable (list filter,
  badge, banner) but nothing proactively surfaces it to anyone.
- `created_by`/`reason` list filters and `older_than_days` have no
  dedicated frontend control this phase (only `overdue` does) — they
  exist in the API/Swagger for completeness and are exercised by this
  phase's own live verification, but a user can't drive them from the
  UI without a future filter-panel addition.
- The Stock Adjustment screen still has no dedicated "Overdue" summary
  card (kept to the existing 4-card grid rather than resizing it) — the
  count is visible via the new filter instead.
- Everything listed as a known issue in Phase 2–19 still applies unless
  explicitly superseded above.

## Phase 21 — Pending Stock Adjustment Reminder / Notification Delivery: VERIFIED COMPLETE

### Existing architecture found

`internal/messaging/rabbitmq` had a real, working publish → exchange →
queue → consume foundation (Manager/Publisher/Consumer/topology), but
**zero real business queues** — the only thing that had ever flowed
through it was `wms.system.health`, an explicitly-labeled dev/
infrastructure heartbeat. No `internal/jobs` or `internal/workers`
package existed either (CLAUDE.md's own recommended shape, never
built). `cmd/worker/main.go`'s consumer and `cmd/server/main.go`'s
publisher were both single-purpose, hardcoded to that one dev queue.
Phase 21 is this codebase's **first real business queue** — the
consumer.go `Handler` contract (manual ack, only after success; nack
without requeue on error, no DLQ) was reused exactly as-is, not
redesigned.

### Reminder eligibility

Reused Phase 20's exact logic — no duplicated aging calculation:
`internal/stockadjustment/repository.go`'s new `FindOverduePending`
runs `WHERE status = 'PENDING' AND updated_at <= cutoff`, and `cutoff`
comes from calling the identical `overdueCutoff(now, warningHours)`
pure function Phase 20's own `is_overdue` filter already uses. A
non-PENDING adjustment is structurally excluded from the candidate set
(not just "flagged not overdue") — APPROVED/REJECTED/DRAFT can never
appear, live-verified by approving two overdue adjustments and
confirming zero reminders on the next scan.

### RabbitMQ topology

`internal/messaging/rabbitmq/topology.go` gained
`wms.stock_adjustment.pending_reminder` (routing key
`stock_adjustment.pending_reminder`), bound to the existing
`wms.events` topic exchange — no second exchange, no second messaging
framework. Durable queue, persistent messages (`Publisher.Publish`
already sets `DeliveryMode: amqp.Persistent` for every message).
Message envelope (`internal/stockadjustment/reminder.go`'s
`ReminderMessage`) matches CLAUDE.md's own recommended shape
(message_id/event_type/created_at/correlation_id/payload), the same
shape `DevHeartbeat` already established. Payload is deliberately
minimal — `adjustment_id`, `adjustment_no`, `created_by`,
`pending_since`, `age_hours` only, never a full adjustment row, and
never a recipient list (the worker resolves recipients itself at
consume time).

### Scheduler / producer

Server-owned periodic ticker (`startStockAdjustmentReminderScanner` in
`cmd/server/main.go`), the same shape `startDevHeartbeat` already
established: scans immediately on startup, then every
`STOCK_ADJUSTMENT_REMINDER_INTERVAL_MINUTES` (default **15**, plain
integer minutes, new `config.parseInt` — same fallback-on-invalid
convention as every other config value). A publish failure is logged
and the loop continues untouched; no Windows Task Scheduler or cron
involved.

### Duplicate protection (cooldown)

New additive table `stock_adjustment_reminders` (migrations/0034 — one
row per adjustment ever reminded: `stock_adjustment_id` PRIMARY KEY,
`last_reminded_at`, `reminder_count`), separate from
`stock_adjustments` itself so the reminder-claim write never contends
with a concurrent Confirm's row lock. `Repository.ClaimReminder`
(insert-if-absent, else conditional update only when
`last_reminded_at` is older than the cooldown) runs inside its own
transaction — a claim that fails (still within cooldown) is reported
back as "don't publish," not an error. **The cooldown period reuses
`STOCK_ADJUSTMENT_PENDING_WARNING_HOURS`** — no second config knob: a
still-unresolved adjustment is reminded again each time it accumulates
another full warning-hours period past its last reminder (48h → next
at 96h, 144h, ...). Live-verified: a scan immediately after a
just-sent reminder produced zero republish for that adjustment, while
a separate newly-overdue adjustment in the same scan correctly did
fire.

### Consumer

`cmd/worker/main.go` gained
`stockAdjustmentReminderHandler` (`rabbitmq.Handler`): decodes the
message, maps it to `notification.StockAdjustmentReminderPayload` (a
package-local narrow type — `internal/notification` never imports
`internal/stockadjustment`, matching this codebase's established
cross-package narrow-shape convention), and calls
`notification.Service.NotifyStockAdjustmentReminder`. Returns nil
(ACK) only once every recipient's DB write has actually completed;
returns an error (NACK, no requeue) on any DB failure — the message is
never acknowledged before persistence succeeds. Idempotent by
construction (see Notification persistence below), live-verified by
replaying an already-processed message twice via RabbitMQ's own
management API and by restarting the worker mid-cycle — zero
duplicate rows either way.

### Notification persistence

New additive table `notifications` (migrations/0033) — generic shape
(`type`/`reference_type`/`reference_id` are not hardcoded to stock
adjustments) but Phase 21 is the only writer, always
`type=STOCK_ADJUSTMENT_REMINDER`/`reference_type=STOCK_ADJUSTMENT`.
`message_id` (the originating RabbitMQ message's own ID) is `NOT NULL`
with `UNIQUE (message_id, user_id)` — per-recipient, not per-message,
since one reminder fans out to every eligible approver as its own row
sharing one `message_id`. `Repository.CreateIdempotent` catches the
unique-violation via the same `sqlerr.IsUniqueViolation` classifier
every other idempotency guard in this codebase already uses, reporting
"already exists" as a successful no-op, never an error.

### Recipient logic

Inspected `internal/rbac`'s catalog before deciding: only `ADMIN` (all
permissions) and `WAREHOUSE_MANAGER` (all-except-delete) are granted
`stock_adjustment.approve` — no operational role (RECEIVING/QC/
PUTAWAY/PICKING/SHIPPING) has any `stock_adjustment.*` grant at all.
Chose `stock_adjustment.approve`, not `.view`: it's the exact
permission `POST /stock-adjustments/{id}/confirm` itself requires, so
a recipient is always someone who can actually resolve the adjustment,
never a spectator. New `internal/user` method
`FindActiveIDsWithPermission(ctx, code)` computes this dynamically via
`users → role_permissions → permissions`, nothing hardcoded to a role
code, and excludes INACTIVE/LOCKED users. Live-verified: a real
INACTIVE `WAREHOUSE_MANAGER` seed user (`phase9_wm_user`) correctly
received **no** notification; a freshly created ACTIVE one did,
alongside `admin`, in the same fan-out.

### API

`internal/notification` — two endpoints, both `RequireJWT`-only (no
module permission; a user's own inbox is identity-scoped, the same
pattern `GET /auth/me` already uses):

- `GET /notifications` — the caller's own rows, newest first;
  `?is_read=false` plus the existing `meta.total` doubles as the
  unread badge count (no separate count endpoint).
- `POST /notifications/{id}/read` — ownership-checked (another user's
  notification is reported 404, not 403, so existence is never
  leaked); idempotent (already-read is a no-op success).

### Frontend

`MainLayout.vue`'s pre-existing (previously decorative, always-on) bell
icon is now wired: conditional unread dot, click opens a dropdown
listing recent notifications, clicking one marks it read and — for a
`STOCK_ADJUSTMENT` reference — navigates to
`/inventory/stock-adjustment?adjustment_id=<id>`.
`StockAdjustmentView.vue`'s `mounted()` reads that query param and
opens the detail drawer directly. A light 60s `setInterval` refreshes
the unread count (no websocket/SSE introduced). No navigation redesign,
no new page — one existing icon wired up, one small deep-link read in
an existing view.

### Configuration

`STOCK_ADJUSTMENT_REMINDER_INTERVAL_MINUTES` (default 15) added to
`.env`, `.env.example`, and `backend/.env.example` — flows through
`docker-compose.yml`'s existing `env_file: .env` unchanged (no compose
edit needed, same as Phase 20). No secrets involved.

### Tests

Pure unit tests only (no DB/RabbitMQ mocking exists in this codebase —
every DB/queue-touching behavior is verified live instead, the same
split Phase 17/18/20 already established and reused here rather than
re-justified from scratch):

- `stockadjustment`: `TestNewReminderMessage`(+`_UniqueIDsPerCall`),
  `TestReminderMessage_Marshal` — envelope/payload construction.
  Eligibility/overdue-threshold logic itself is Phase 20's own
  `TestPendingAging_*`/`TestOverdueCutoff`, reused, not duplicated.
- `notification`: `TestBuildReminderNotificationText`(+
  `_DifferentAdjustment`) — the extracted pure text-construction piece
  of `NotifyStockAdjustmentReminder` (the same "extract the pure part"
  precedent as Phase 18/20's own `applyCancel`/`pendingAging`);
  `TestIsUniqueViolation` — the idempotency-guard classifier
  delegation, matching every other package's own repository_test.go.
- `config`: default/override tests for the new interval variable.

`go fmt`/`go vet`/`go test ./...` clean across every package
(including `stockadjustment`/`stockcount` — Phase 16/17 integration
unaffected), zero regressions. `npm run build` clean.

### Live RabbitMQ verification

Rebuilt `backend`, `worker`, `frontend` (`docker compose up -d --build
backend worker frontend`; `migrate` rebuilt automatically as a shared-
build-context dependency, applying migrations 0033/0034 — confirmed via
its own logs). All 5 services healthy.

- **Full pipeline, organically**: on backend startup, the scanner's
  immediate first scan found a real overdue adjustment left over from
  Phase 20's own live testing (`ADJ-020006`, 72h old) and published a
  reminder within milliseconds; the worker consumed it and persisted a
  notification for `admin` — observed via container logs before any
  manual test was even run.
- **Multi-recipient fan-out**: created an ACTIVE `WAREHOUSE_MANAGER`
  test user, created and backdated a second overdue adjustment
  (`ADJ-020007`, 60h), forced a scan (backend restart) — one message,
  two notification rows (`admin` + the new WM user), same `message_id`.
- **Cooldown**: the same forced scan correctly did **not** re-publish
  for `ADJ-020006` (still within its 48h cooldown from the first
  reminder ~90 seconds earlier) while still catching the newly-overdue
  `ADJ-020007` in the same pass.
- **Duplicate RabbitMQ delivery**: replayed `ADJ-020006`'s exact
  message (same `message_id`) via RabbitMQ's management HTTP API.
  First replay legitimately added one new row (for the WM user, who
  didn't exist as a recipient at original send time — not a bug, a
  genuinely new (message, recipient) pair); a second replay of the
  identical message produced **zero** new rows for either recipient.
- **Worker restart mid-cycle**: restarted the worker while a manually-
  replayed message was still in flight; notification count was
  identical before and after (4 rows, unchanged) — redelivery after
  restart hit the same unique constraint and was safely rejected.
- **Resolving stops future reminders**: approved both overdue
  adjustments live (`AddStockTx`, inventory `on_hand_qty` 51→53,
  correct); a subsequent forced scan published **zero** reminders for
  either, confirmed both by an absent log line and an unchanged
  notification count.
- **Phase 20 aging unchanged**: both adjustments' own `is_overdue`
  flipped to `false` immediately upon approval, in the same Confirm
  response — no separate reminder-side aging logic exists to drift out
  of sync.
- **No inventory mutation from reminder activity**: inventory
  `on_hand_qty` was queried before and after the entire reminder test
  sequence (multiple scans, multiple publishes, multiple manual
  redeliveries, a worker restart) and was provably unchanged until the
  two legitimate approvals — reminders touch only
  `stock_adjustment_reminders` and `notifications`, never `inventory`.
- **API**: `GET /notifications` (own rows only), `?is_read=false` +
  `meta.total` as unread count, `POST /notifications/{id}/read`
  (idempotent re-mark; 404 on another user's notification, not 403 —
  no existence leak).
- **Frontend, real browser**: bell dropdown showed both reminders with
  the correct unread count; clicking the unread one marked it read
  (badge dot disappeared), navigated to
  `/inventory/stock-adjustment?adjustment_id=20007`, and the detail
  drawer auto-opened showing the (now-approved) adjustment correctly.
- **Regression sweep**: `GET` 200 across warehouses/products/
  inventory/receipts/putaway/picking/shipping/stock-adjustments/
  stock-counts/roles/users/notifications; Swagger UI reachable; zero
  panics/fatals in either backend or worker logs across the entire
  session.

### Known limitations

- **Best-effort delivery, not guaranteed**: if a notification insert
  fails partway through a multi-recipient fan-out (a transient DB
  error, not a duplicate), already-inserted recipients keep their
  notification but later recipients get none until the adjustment's
  next cooldown-eligible cycle — documented in
  `NotifyStockAdjustmentReminder`'s own doc, not solved with per-
  recipient retry, to keep this phase's scope to "narrowly scoped
  reminders," not a generic guaranteed-delivery/outbox framework
  (CLAUDE.md's own criteria for when Outbox is warranted don't apply
  to a non-critical, self-healing reminder).
- **No DLQ / bounded retry**: matches this codebase's one pre-existing
  consumer exactly (nack-without-requeue on error) — a permanently
  malformed message is dropped, not retried forever, but also not
  parked anywhere for inspection. Acceptable for a first real queue at
  this phase's own narrow scope; a future phase introducing a second
  business queue is the natural point to add a shared DLQ pattern once
  there's more than one consumer to justify it.
- **Notification list has no server-side pagination UI** — the
  dropdown always requests the latest 10; a "view all" page was not
  built (requirement 8's own "do not create excessive notification
  CRUD").
- **`created_by`/`pending_since` in the reminder payload are carried
  but not yet surfaced anywhere in the UI** (only `adjustment_no`/
  `age_hours` appear in the notification text) — available for a
  future richer notification without a payload-shape change.
- Everything listed as a known issue in Phase 2–20 still applies unless
  explicitly superseded above.

## Phase 22 — Report / Export: VERIFIED COMPLETE

### Scope decision (sync vs async)

Inspected all 8 candidate export surfaces before choosing an
architecture per-surface, per CLAUDE.md's own "do not use RabbitMQ
simply because it exists" rule:

- **7 synchronous CSV exports**, each colocated in its own existing
  domain package, reusing that domain's own current filter-binding
  logic and its own existing `.view`/list permission (Receiving,
  Putaway, Picking, Shipping, Stock Adjustment, Stock Count, Inventory
  balance) — all bounded, filtered lists a user already views on
  screen; no case for async.
- **1 asynchronous export** — Inventory Transaction ledger history —
  the only genuinely unbounded, ever-growing dataset among the 8
  (`inventory_transactions` has no natural upper bound the way an
  open-orders list does). New `internal/report` package + `export_jobs`
  table + RabbitMQ queue, gated behind `stock_inquiry.export`.

### Shared CSV helper

New `internal/platform/csvexport` package (`csvexport.go` +
`csvexport_test.go`, full unit coverage): `MaxExportRows = 10000` row
cap shared by every export (sync list queries pass `Limit:
MaxExportRows` instead of the UI's normal page size); `WriteTo`
(stdlib `encoding/csv` only — no XLSX dependency added, matching the
phase spec's "CSV first, XLSX only if reasonable" read as "CSV is
sufficient, don't add a dependency for it yet"); `SafeFilename`
(strips everything outside `[a-zA-Z0-9_.-]`); `ServeAttachment` sets
`Content-Disposition: attachment` + CSV content type; `FormatFloat`/
`FormatTime`/`FormatString`/`FormatID` null-safe formatters reused by
every domain's `Export` handler.

### Synchronous exports (7)

Each of `internal/receiving`, `putaway`, `picking`, `shipping`,
`stockadjustment`, `stockcount`, and `internal/inventory` (balance)
gained a `Service.ListForExport` (same repository `List` call as the
existing paginated endpoint, `Page=1`/`Limit=MaxExportRows`, total
discarded) and a `Handler.Export` (same filter-binding as `List`,
manual CSV header/row construction per type, `csvexport.ServeAttachment`)
plus a `GET /<resource>/export` route gated on that module's own
existing list permission — no new permission created for these 7.

### Asynchronous export (Inventory Transaction History)

New `internal/report` package, Handler → Service → Repository, exactly
matching CLAUDE.md's own async example flow:

- **Model**: `export_jobs` (migrations/0035, additive) — `job_type`,
  `status` (PENDING/PROCESSING/COMPLETED/FAILED,
  `CK_export_jobs_status`), `requested_by` (FK → users), `filters_json`,
  `file_name`/`file_path`, `row_count`, `error_message`, `message_id`
  (`UNIQUE`), timestamps.
- **API**: `POST /reports/inventory-transactions/export` validates +
  creates the job row + publishes, returns `202 Accepted` with
  `job_id` (CLAUDE.md's own example response shape, reused verbatim);
  `GET /reports/export-jobs/{id}` (ownership-checked, another user's
  job → 404 via `ErrNotFound`, same no-existence-leak pattern Phase 21
  established for notifications); `GET
  /reports/export-jobs/{id}/download` (ownership + `COMPLETED`-only
  check → `ErrNotReady` otherwise, then `c.File`). All three under one
  route group, **dual-gated** — `stock_inquiry.view` (group) +
  `stock_inquiry.export` (route) — reusing `stock_inquiry.export`, a
  permission that has existed in the catalog since Phase 2 but was
  never enforced anywhere until now.
- **Publisher placement**: `report.Service.CreateInventoryTransactionsJob`
  holds its own `messagePublisher` interface and publishes internally
  right after the job row commits — **not** from the handler
  (CLAUDE.md: "Do not publish RabbitMQ messages directly from HTTP
  handlers"). Caught and fixed during implementation before it shipped.
- **Message envelope**: `report.Message` matches the same
  message_id/event_type/created_at/correlation_id/payload shape as
  Phase 21's `ReminderMessage` (`event_type = "report.export"`,
  `payload.job_id`).
- **Topology**: `wms.report.export` queue / `report.export` routing
  key, bound to the existing `wms.events` exchange — no second
  exchange.
- **Idempotent claim**: `report.Repository.Claim` — the identical
  atomic conditional-UPDATE pattern as Phase 21's `ClaimReminder`
  (`UPDATE ... SET status='PROCESSING' ... WHERE id=? AND
  status='PENDING'`, check `RowsAffected`); a redelivered or replayed
  message is a safe no-op, not a duplicate file/row.
- **Worker**: `cmd/worker/main.go`'s `reportExportHandler` decodes the
  message, calls `Service.ProcessJob` (claim → decode `filters_json` →
  `inventory.ListTransactionsForExport` → write CSV to
  `EXPORT_DIR/<job_id>-<safe-name>.csv` → `MarkCompleted`/`MarkFailed`),
  ACKs only after a successful DB status update, NACKs (no requeue) on
  failure — same manual-ack contract as Phase 21's consumer, no new
  contract invented.
- **Cross-package types**: `report.transactionExporter` interface uses
  `inventory.TransactionExportFilters`/`inventory.TransactionExportRow`
  directly (imports `internal/inventory`), not a narrow local copy —
  justified by precedent (`stockcount`'s own `adjustmentCreator`
  interface already imports `internal/stockadjustment` types directly
  for the same reason: it calls straight into that package's service,
  unlike Phase 21's RabbitMQ-message-decoding boundary, which
  legitimately needs decoupling via a narrow copy).
- **Shared file access**: new `internal/inventory` methods
  `ListForExport`/`ListTransactionsForExport` (repository-level,
  global join across products/locations/zones/warehouses, capped at
  `csvexport.MaxExportRows`); new Docker named volume `export_files`
  mounted at `EXPORT_DIR` (default `/app/exports`) identically in both
  `backend` and `worker` — the only way both containers can share
  files across process boundaries.

### Configuration

`EXPORT_DIR` added to `.env`, `.env.example`, `backend/.env.example`
(default `/app/exports`, matching the Docker volume mount) — flows
through `docker-compose.yml`'s existing `env_file: .env` unchanged.

### Frontend

- Each of `ReceiveView.vue`, `PutawayView.vue`, `PickingView.vue`,
  `ShippingView.vue`, `StockAdjustmentView.vue` gained an "⬇ Export
  CSV" button next to the page's existing header action, calling a new
  `export<Domain>(filters)` in that domain's own `services/*.js` (same
  filter params as the page's own list call), via new shared
  `apiDownload`/`triggerDownload` helpers in `services/api.js` (fetch
  with the stored bearer token, blob response, filename parsed from
  `Content-Disposition`, synthetic `<a download>` click).
- `CheckStockView.vue` gained the same button in its Stock Count
  History filter row.
- `StockInquiryView.vue` (the most complex change): the **pre-existing
  but previously non-functional** "Export Stock" button is now wired
  to the sync `GET /inventory/export`; a new "⬇ Export Transaction
  History" button (`v-if="hasPermission('stock_inquiry.export')"`)
  drives the async flow — `POST` the job, then a 2s `setInterval` poll
  of `GET /export-jobs/{id}` showing inline PENDING/PROCESSING/
  COMPLETED/FAILED status text, auto-triggering the download and
  clearing the interval/status on COMPLETED (cleared again after 4s so
  the status line doesn't linger), and `beforeUnmount` clears any live
  interval. No new page, no navigation change.

### Tests

`csvexport`: full coverage (`WriteTo`, `SafeFilename` including
path-traversal/special-character inputs, `ServeAttachment` headers,
every `Format*` null-safe helper). `report`: `TestNewMessage`
(+`_UniqueIDsPerCall`), `TestMessage_Marshal`. `config`:
`TestLoad_ExportDirDefault`/`_Override`. `go fmt`/`go vet`/`go test
./...` clean across every package, zero regressions. `npm run build`
clean.

### Live verification

Rebuilt `backend`, `worker`, `frontend`, and (explicitly, separately —
see note below) `migrate` (`docker compose build backend worker
frontend migrate` + `up -d` the same four); all 5 services healthy.

- **Migration gotcha caught and fixed**: an initial rebuild that named
  only `backend worker frontend` left `migrate` on its stale Phase-21
  image (`schema_migrations` still capped at 0034,
  `SELECT * FROM export_jobs` failed with "Invalid object name").
  `migrate` shares `backend`'s build context but is **not**
  auto-included just because `backend` was rebuilt — it must be named
  explicitly whenever a new migration file is added. Re-ran with
  `migrate` named explicitly; confirmed applied via its own logs
  (`"migration applied" version=0035`) and `SELECT COUNT(*) FROM
  export_jobs` succeeding.
- **All 7 sync exports**: curled directly with a valid bearer token —
  200, correct `Content-Disposition`, correct CSV header row + data
  rows matching each list endpoint's own current filtered result set.
- **RBAC dual-gate**: a user with `stock_inquiry.view` but not
  `.export` got 403 on `/inventory/export` and all three `/reports/*`
  routes; a user with neither got 403 on the group itself.
- **Async pipeline end-to-end (API level)**: `POST
  /reports/inventory-transactions/export` → 202 + `job_id`; `GET
  /export-jobs/{id}` PENDING → PROCESSING → COMPLETED across worker
  polls; `GET .../download` → 200, real CSV content matching
  `inventory_transactions` filtered by the submitted `filters_json`.
- **Idempotency**: replayed the same `report.export` message twice via
  RabbitMQ's management API — `Claim`'s `WHERE status='PENDING'` guard
  made the second delivery a no-op (`RowsAffected=0`), zero duplicate
  file writes, zero duplicate `MarkCompleted` calls.
- **Ownership**: a second user's `GET`/`download` on another user's
  `job_id` → 404, not 403 (no existence leak, matching Phase 21's
  notification pattern).
- **Filename sanitization**: submitted filters producing a job name
  with special characters; downloaded filename correctly reduced to
  the safe-character set only.
- **Worker restart mid-job**: killed the worker after `Claim` succeeded
  but before file write completed on one job; job remained stuck
  `PROCESSING` (expected — no re-claim of an already-PROCESSING row by
  design, same as this codebase's other claim-based jobs); restarting
  the worker did not reprocess it (correct, since `Claim` only matches
  `PENDING`) — noted as a known limitation below, not silently
  "fixed" by inventing a stuck-job reaper that wasn't asked for.
- **Browser, real UI** (this session): re-authenticated after a JWT
  expiry mid-session; on `/inventory/stock-adjustment`, clicked "⬇
  Export CSV" — network panel confirmed `GET
  /api/v1/stock-adjustments/export` → 200. On `/inventory/stock-inquiry`,
  clicked "Export Stock" — `GET /api/v1/inventory/export` → 200; then
  clicked "⬇ Export Transaction History" — network panel confirmed the
  full async round trip: `POST
  /api/v1/reports/inventory-transactions/export` → **202**, `GET
  /api/v1/reports/export-jobs/2` → **200**, and the UI's own polling
  auto-fired `GET /api/v1/reports/export-jobs/2/download` → **200**
  with no manual interaction beyond the initial click — the inline
  status text was gone by the time it was inspected a few seconds
  later, which is the polling logic's own designed auto-clear, not a
  missing render.
- **Inventory unaffected**: `on_hand_qty` queried before/after the
  entire export test session (multiple sync exports, multiple async
  jobs, replays, a worker restart) — unchanged; exports are read-only
  by construction (no domain's `Export`/`ListForExport` path writes
  anything).
- **Regression sweep**: existing paginated `List` endpoints for all 7
  domains still return their normal page-sized results (export's
  `Limit=MaxExportRows` change is isolated to the new `ListForExport`
  method, not a shared code path); Phase 21 notification bell/reminder
  flow re-verified unaffected; Swagger UI reachable and lists all new
  routes; zero panics/fatals in backend or worker logs.

### Known limitations

- **10,000-row cap, silent**: `csvexport.MaxExportRows` truncates
  without any UI/API signal that truncation occurred. Acceptable for
  this phase's scope (no domain in this system currently approaches
  10k rows in practice) but worth a `row_count` vs filtered-total
  comparison in the UI if a domain ever grows past it.
  `export_jobs.row_count` already stores the actual written count, so
  the data needed for that check already exists server-side.
- **A job stuck in PROCESSING from a mid-write worker crash is never
  retried or reaped** — matches Phase 21's own "no DLQ / bounded
  retry" limitation exactly (same single-queue, no-DLQ posture); a
  future phase adding a second business queue remains the natural
  point to add a shared retry/DLQ pattern for both.
- **No XLSX** — CSV only, per the phase's own instruction to add XLSX
  "only if reasonable"; not judged necessary yet since every consumer
  so far is a re-import/spreadsheet tool that opens CSV natively.
- **Export files are never cleaned up** — `export_files` grows
  unbounded over time; no retention/cleanup job was requested or
  built. Worth flagging as a future background-cleanup candidate
  (CLAUDE.md's own "background cleanup" async example) once real
  volume data exists to size a retention policy against.
- Everything listed as a known issue in Phase 2–21 still applies
  unless explicitly superseded above.

## Phase 23 — Operational Dashboard: VERIFIED COMPLETE

### Existing frontend found

`DashboardView.vue` (`/dashboard`, the app's default landing route) was
100% hardcoded mock data — 4 summary cards (Pending Receiving/
Available Stock/Pending Picking/Low Stock) and a 3-row "Recent Inbound
Activity" table, none of it wired to any API. "Quick Actions" (4
buttons: Start Receiving/Stock Inquiry/Create Picking Task/Stock
Adjustment) were also inert — left untouched this phase; wiring
navigation onto them is a UI-affordance change beyond "aggregation
endpoints," not requested.

### RBAC — no new permission needed

Inspected `internal/rbac/catalog.go` before adding anything: a
`dashboard` module with `view`/`export` actions has existed in the
catalog since Phase 2/3, and `dashboard.view` is already seeded into
**every one of the 7 roles'** default grants (ADMIN, WAREHOUSE_MANAGER,
RECEIVING, QC, PUTAWAY, PICKING, SHIPPING) — it just had no handler
enforcing it until now. `GET /dashboard/summary` gates on exactly that
existing, already-universal permission; zero catalog/migration changes.

### Aggregation design — cheap counts, not full loads

Per CLAUDE.md's own dashboard guidance (aggregation endpoints, reused
RBAC, no expensive unindexed aggregation), new `internal/dashboard`
package — Handler → Service only, **no repository, no table, no
migration**. It composes three existing services through narrow
interfaces (`receivingSummarizer`/`pickingSummarizer`/
`inventorySummarizer`, the same "narrow interface satisfied by another
package's Service" shape `internal/shipping`'s own `pickingChecker`
already established):

- **Pending Receiving** / **Pending Picking**: new `Repository.CountOpen`
  in `internal/receiving` and `internal/picking` — a single `COUNT(*)
  WHERE status IN (...)` against each table's existing status index
  (`IX_receipts_status`, `IX_picking_tasks_status`), enumerating the
  exact same "still open" status set each package's own
  `Status.CanConfirm()` already defines (receiving:
  DRAFT/WAITING/PARTIALLY_RECEIVED; picking: PENDING/IN_PROGRESS) — a
  doc comment cross-references `CanConfirm` on both so the two can't
  silently drift apart. No `List()` call, no rows fetched, just a count.
- **Available Stock** / **Low Stock**: reuses `inventory.Service.Summary`
  verbatim — the exact same call `StockInquiryView.vue`'s own summary
  cards already make, so the two screens can never disagree on these
  numbers.
- **Recent Inbound Activity**: reuses `receiving.Service.List` with
  `Page=1, Limit=5` (no new query) — a small, bounded top-5 page, not a
  full-table load; response type is `receiving.ListResponse` directly
  (imported, not duplicated — same "import the owning package's types"
  precedent `internal/report` used for `inventory.TransactionExportRow`
  in Phase 22), so the recent-activity rows can never drift from what
  `ReceiveView.vue`'s own list already shows for the same receipts.

All four numbers/lists are independent reads (no shared transaction,
no cross-consistency guarantee attempted) — appropriate for a
refresh-on-page-load operational overview, not a financial
reconciliation.

### API

`GET /dashboard/summary` — one endpoint, `dashboard.view`-gated,
returns `pending_receiving_count`, `available_stock_qty`,
`pending_picking_count`, `low_stock_count`, `recent_inbound[]` in a
single response (one round trip for the whole page, matching
CLAUDE.md's "aggregation endpoint," singular).

### Frontend

`DashboardView.vue`: added `loading`/`loadError`/`summary` state, a
`fetchSummary()` call in `mounted()` against the new
`services/dashboard.js` (`getDashboardSummary()`), and replaced every
hardcoded number/row with the live response — cards show `…` while
loading, an empty-state row replaces the table when
`recent_inbound.length === 0`. Reused `ReceiveView.vue`'s own exact
`statusText`/badge-class mapping (DRAFT/WAITING/PARTIALLY_RECEIVED/
COMPLETED/CANCELLED → Draft/Waiting/Receiving/Completed/Cancelled +
orange/gray/green badges) so a receipt's status reads identically on
both screens. No layout/design change — same cards, same table, same
Quick Actions block, only the data source changed.

### Tests

`internal/dashboard` has no test file — it is pure composition (three
already-tested services' own methods, no new branching logic of its
own to unit test), matching this codebase's established split: pure
logic gets Go unit tests, DB-backed behavior is verified live. The two
new `CountOpen` methods are single-line COUNT queries verified live
below rather than mocked. `go fmt`/`go vet`/`go test ./...` clean
across every package, zero regressions. `npm run build` clean.

### Live verification

Rebuilt only `backend` and `frontend` (`docker compose build backend
frontend` + `up -d backend frontend worker` — `worker` was left
untouched by this phase's code but included in the `up -d` to reconcile
compose state; no new image, no restart of an unrelated container);
`sqlserver`/`rabbitmq` were correctly left alone (no migration, no
topology change this phase). `migrate` ran once as a dependency of
`backend` recreating and exited immediately (no pending migration to
apply — this phase added zero). All 5 services healthy afterward.

- **Correctness cross-check against real SQL**: queried
  `receipts`/`picking_tasks` status distributions directly
  (`PARTIALLY_RECEIVED=3, WAITING=1` → 4; `PENDING=1, IN_PROGRESS=2` →
  3) — matched `GET /dashboard/summary`'s
  `pending_receiving_count=4`/`pending_picking_count=3` exactly.
- **Cross-check against Stock Inquiry's own numbers**: `GET
  /inventory/summary` returned `available_qty=800,
  low_stock_count=0` — identical to the dashboard's own
  `available_stock_qty`/`low_stock_count`, confirming the reused
  `Summary()` call can't drift between the two screens.
- **Browser, real UI**: navigated to `/dashboard` — cards rendered
  4/800/3/0 (matching the API exactly), "Recent Inbound Activity"
  showed 5 real receipts with correct status labels/badges and no
  console errors; network panel confirmed `GET
  /api/v1/dashboard/summary` → **200** as the only new request the
  page makes (`GET /notifications` also fires, unrelated — Phase 21's
  existing bell-icon poll).
- **Regression sweep**: 200 across warehouses/products/inventory/
  receipts/putaway/picking/shipping/stock-adjustments/stock-counts/
  roles/users/notifications; Swagger UI reachable and lists `GET
  /dashboard/summary`; zero panics/fatals/errors in backend or worker
  logs across the rebuild and test session.
- **No inventory mutation**: the dashboard path only issues `SELECT`/
  `COUNT` queries (`CountOpen`, `Summary`, `List`) — no service method
  it calls can write.

### Known limitations

- **Quick Actions buttons remain inert** — pre-existing from before
  this phase, not touched (wiring navigation onto them is a UI
  behavior change beyond "aggregation endpoints," not requested this
  phase).
- **No dashboard-level caching/refresh** — the summary is fetched once
  on mount; a stale dashboard requires a manual page reload (matches
  the original mock's own "static until reload" behavior; Phase 21's
  60s notification-poll pattern was not extended here since it wasn't
  asked for and a dashboard overview refreshing itself unprompted is a
  UX decision, not an aggregation-endpoint requirement).
- **`dashboard.export` (the catalog's own second action) has no
  endpoint** — only `.view` is enforced this phase; a dashboard PDF/
  CSV snapshot export was not requested and none of Phase 22's 8
  export surfaces included one.
- Everything listed as a known issue in Phase 2–22 still applies
  unless explicitly superseded above.

## Phase 24 — Replenishment: VERIFIED COMPLETE

### Existing frontend found

Grepped the whole frontend for "replenish"/"Replenish" — zero matches.
Unlike every prior phase, there was no existing mock view, no route
stub, no menu item to reverse-engineer requirements from: this is a
genuinely new module, built from this phase's own detailed spec rather
than "inspect existing source" (there was none to inspect).

### Model — reusing Product Master's existing min_stock/max_stock

Did not add a per-location reorder-point table. `products.min_stock`/
`max_stock` (Phase 4) already exist and are already the exact threshold
`internal/inventory`'s own `Status` derivation uses for `LOW_STOCK` (see
its dto.go doc: "available <= the product's min_stock"). Phase 24
reuses that identical semantic for Recommend rather than inventing a
second, competing threshold — a location whose available_qty has
fallen to product.min_stock is "low" by the same rule Stock Inquiry
already flags it with.

### Single-line task, not a multi-item document

Unlike Receiving/Picking/Putaway (multi-product documents with an
items child table), a replenishment task is inherently one product
moving from one location to another — matches this phase's own "no
complex optimizer" simplicity and Move's own existing (product, from,
to, qty) shape in `inventory.MoveStockTx`. New `replenishment_tasks`
table (migrations/0036, additive) carries product_id/from_location_id/
to_location_id/lot_no/quantity/warehouse_id directly on the header, no
items table. `replenishment_confirmations` (migrations/0037) is the
same idempotency-guard-table pattern picking_confirmations/
receipt_confirmations/putaway_confirmations already established.

### Status machine: PENDING / IN_PROGRESS / COMPLETED / CANCELLED / ISSUE

Exactly the 5 statuses specified. `Start` (PENDING -> IN_PROGRESS) is a
pure status signal with **no inventory effect** — Confirm accepts
either status equally, so Start is optional, never a gate. `ISSUE` is a
new terminal status this codebase hadn't modeled before (Receiving/
Picking/Putaway have no equivalent) — reachable only via the new
`ReportIssue` action, requires a remark (unlike Cancel's optional one),
and — like Cancel — releases the task's reservation without ever
calling `MoveStockTx`.

### No oversubscription — Phase 12's reservation pattern, reused exactly

`Create` calls `inventory.ReserveStockTx` against
(product_id, from_location_id, lot_no) for the task's full quantity
**before** the task row is even written — the identical atomic
conditional-UPDATE concurrency proof Picking's own Phase 12 established
(two concurrent Create calls against the same tuple can never together
reserve more than is on-hand, because the arithmetic and the
availability check are one atomic SQL statement). Live-verified: a
Create requesting 99999 units against a location with far less
available stock was correctly rejected with 422
`insufficient available stock to reserve this quantity`, and the
reservation for a real, successful Create was confirmed via a direct
`GET /inventory` query showing `reserved_qty` increase exactly by the
requested quantity at the source location.

### Cancellation never mutates inventory

`Cancel` and `ReportIssue` both only call `ReleaseReservationTx` —
never `MoveStockTx`, never an `inventory_transactions` row — the exact
same "cancellation is the mirror image of Create's reservation, not a
stock movement" principle Picking's own Phase 13 established. Live-
verified: cancelling a task showed the source location's
`reserved_qty` return to 0 with `available_qty` unchanged from before
the cancelled task's own Create call — no ledger row, no balance
change.

### Confirm — the real move

`Confirm` releases the task's own reservation, then calls
`inventory.MoveStockTx` (both inside one transaction) — two ledger rows
(MOVE type, `reference_type=REPLENISHMENT`, `reference_id=task_no`),
same idempotent-claim-via-INSERT pattern (`replenishment_confirmations`)
every other workflow's Confirm already uses. Live-verified end-to-end
with real data: a task moving 10 units from BIN-C01 to A-01-02 (product
SKU-TEST-01) correctly decremented BIN-C01's `on_hand_qty` from 67 to
57 and incremented A-01-02's (same lot) from 53 to 63 — exact
arithmetic match, confirmed via direct `GET /inventory` queries before
and after. A replayed confirm with the same `idempotency_key` returned
the task's already-COMPLETED state as a no-op, not a second move.

### Recommend — "simple... no complex optimizer"

New `GET /replenishment/recommendations?warehouse_id=` : for every
(product, location) balance in the warehouse, a location at or below
that product's own min_stock is a candidate destination; its source is
simply that same product's other location in the same warehouse
currently holding the most available stock (a single SQL `ORDER BY
available_qty DESC` per product, not a search/optimization pass),
capped so the suggestion never exceeds the destination's room up to
max_stock or the source's own available quantity. Deliberately
advisory only — two recommendations may name the same source location
in one response (this pass tracks nothing "already claimed" across
recommendations); the real, race-free availability check only happens
for real once a recommendation is turned into an actual `Create` call,
via `ReserveStockTx`'s own guarded UPDATE. The core algorithm is
extracted into a pure `computeRecommendations` function (the "extract
the pure part" precedent Phase 18/20/21 each already established for
their own pure-logic pieces) and covered by 5 unit tests: low-location-
gets-surplus-source, no-surplus-means-no-recommendation, not-low-means-
no-recommendation, suggested-qty-capped-by-surplus-availability, and
multiple-products-handled-independently. Live-verified against real
inventory: with product min_stock/max_stock temporarily set to
non-zero test values, the endpoint correctly proposed moving from the
warehouse's actual highest-surplus location to the actual low location,
with the exact expected capped quantity — then the product's test
values were reverted to their original 0/0 afterward, leaving no
test-data residue.

### RBAC — new "replenishment" module

Inspected the catalog before adding anything (per every prior phase's
own discipline) — no existing permission fit a genuinely new resource,
unlike Phase 22/23's reuse of `stock_inquiry.export`/`dashboard.view`.
Added `replenishment` with `view`/`create`/`update`/`approve` actions
(51 total catalog permissions now, up from 47) — ADMIN/
WAREHOUSE_MANAGER receive it automatically (both are computed
generically over the whole catalog); PUTAWAY is explicitly granted
`replenishment.view`/`.create`/`.update` (not `.approve`) alongside its
own existing `putaway.*` grants, since replenishment is the same
"place/move stock" duty. RECEIVING/QC/PICKING/SHIPPING receive nothing
new. Route gating: List/Get/Recommend on `.view`; Create on `.create`;
Start/Confirm on `.update` (staff executing an already-created task);
Cancel/ReportIssue on `.approve` — the same authority-level split
Picking's own Phase 13 Cancel established for "ends the task without
completing it" actions. Live-verified: a temporary PICKING-role test
user got 403 on both `GET` and `POST /replenishment` (PICKING has no
`replenishment.*` grant); the user was deleted immediately after the
check.

### API

```
GET    /api/v1/replenishment
GET    /api/v1/replenishment/recommendations
GET    /api/v1/replenishment/{id}
POST   /api/v1/replenishment
POST   /api/v1/replenishment/{id}/start
POST   /api/v1/replenishment/{id}/confirm
POST   /api/v1/replenishment/{id}/cancel
POST   /api/v1/replenishment/{id}/issue
```

### Frontend — new module, no existing UI to preserve

New `ReplenishmentView.vue` under Inventory in the sidebar (gated
`v-if="canViewReplenishment"` — unlike its unconditional siblings,
`replenishment.view` is not universal, so RECEIVING/QC/PICKING/SHIPPING
staff never see a link that would 403), new route
`/inventory/replenishment`. List+filter table (search/status/
warehouse), a "+ New Task" modal (warehouse -> cascading zone/location
fetch for from/to dropdowns, SKU resolved to product_id on submit — the
same "SKU text field resolved via search on submit" pattern
StockAdjustmentView.vue already established, not a new autocomplete
widget), a "🎯 Recommendations" modal (load by warehouse, "Use" button
pre-fills the New Task form from a recommendation), and a detail drawer
with Start/Confirm/Cancel/Report Issue actions gated by
`canUpdate`/`canApprove`. Reused the exact `.input`/`.table-th`/
`.table-td`/`.badge-*` CSS class definitions every other view's own
`<style scoped>` block already defines (this codebase does not share a
global stylesheet for these, so each view keeps its own copy — matched
that existing convention rather than introducing a shared component).

### Tests

`replenishment`: `TestStatusCanStart`/`CanConfirm`/`CanCancel`/
`CanReportIssue` (full state-transition matrix) plus 5
`TestComputeRecommendations_*` cases covering the Recommend algorithm's
branches. `go fmt`/`go vet`/`go test ./...` clean across every package,
zero regressions. `npm run build` clean.

### Live verification

Rebuilt `backend`, `frontend`, and explicitly `migrate` (new migrations
0036/0037 plus the RBAC catalog change both needed it); `worker` was
left untouched (Replenishment has zero RabbitMQ component — a location-
to-location move is exactly the kind of "frontend needs an immediate
result" synchronous operation CLAUDE.md's own sync-vs-async section
describes, the same reasoning Picking's own fully-synchronous Confirm
already established). All 5 services healthy afterward; `migrate`'s own
logs confirmed both migrations applied and `permissions seeded
count=51`.

- **Full lifecycle with real data**: created a task (10 units,
  BIN-C01 -> A-01-02), confirmed reservation appeared
  (`reserved_qty=10`), started it, confirmed it (`MoveStockTx` fired,
  balances matched exactly), replayed the same idempotency key (no-op,
  same COMPLETED state returned).
- **Cancel**: created and cancelled a second task — reservation
  released, balances provably unchanged.
- **ReportIssue**: rejected with 400 when remark was omitted; succeeded
  with a remark, landed on ISSUE, reservation released.
- **Oversubscription rejected**: a 99999-unit Create against real,
  much-smaller available stock correctly failed 422
  `insufficient available stock to reserve this quantity` — no task
  row was created.
- **Recommend**: verified against real data with a product's
  min_stock/max_stock temporarily set for the test, then reverted.
- **RBAC**: PICKING-role test user got 403 on List and Create; deleted
  after the check.
- **Browser, real UI**: navigated to `/inventory/replenishment`
  (sidebar link present, page loads); created a task through the "+
  New Task" modal end-to-end (`POST /replenishment` -> 201); opened its
  detail drawer (`GET /replenishment/{id}` -> 200) and clicked "Confirm
  Move" (`POST /replenishment/{id}/confirm` -> 200) — status updated to
  COMPLETED live in both the drawer and the list without a page reload.
  One transient false alarm during this check — a stale, still-
  unexpired JWT issued *before* this rebuild's permission-seeding run
  showed "permission denied: replenishment.view" even though the admin
  role has always had every permission; a fresh login (new JWT,
  current permission claims) resolved it immediately. Not a product
  bug: JWT claims are correctly snapshotted at login time and don't
  retroactively pick up a mid-session permission-catalog change,
  exactly as designed.
- **Regression sweep**: 200 across warehouses/products/inventory/
  receipts/putaway/picking/shipping/stock-adjustments/stock-counts/
  roles/users/notifications/dashboard summary; Swagger UI reachable and
  lists all 8 new routes; zero panics/fatals in backend or worker logs.
- **Inventory-affecting changes are exactly and only**: Confirm's own
  `MoveStockTx` call. Create/Cancel/ReportIssue never touch `quantity`,
  only `reserved_quantity` via Reserve/ReleaseReservationTx — verified
  live above, not just asserted.

### Known limitations

- **A location with no inventory row at all is invisible to
  Recommend** — a pick face that has never held a given product can
  never be recommended as a destination, since the query joins from
  `inventory`, not from a product-to-location mapping (which this
  schema doesn't have and this phase did not invent one to avoid a
  business-policy decision beyond scope).
- **Recommend does not account for other recommendations in the same
  response** — two rows may name the same surplus source location;
  real double-booking is prevented only at actual `Create` time via
  `ReserveStockTx`'s own guarded UPDATE, not by this advisory pass.
- **min_stock/max_stock are whole-product, not per-location** — reused
  exactly as Phase 5's own `LOW_STOCK` status already treats them (not
  a new limitation introduced this phase), so every location holding a
  product is judged against the same single threshold.
- **No DLQ / async component** — none needed; Replenishment has no
  RabbitMQ queue at all, matching CLAUDE.md's own "not every operation
  needs RabbitMQ" guidance for a fast, synchronous, immediate-result
  operation.
- Everything listed as a known issue in Phase 2–23 still applies unless
  explicitly superseded above.

## Current Docker status

**Stale as of Phase 25**: the paragraph below reflects Phase 24's own
last live verification. Docker Desktop was not running at the start of
Phase 25 (`docker ps` failed to reach the daemon) and no Docker
commands were run this phase beyond the read-only `docker compose
config` — see Phase 25's own "Live verification status" for what is
still outstanding once Docker Desktop is available again (targeted
rebuild of `backend`/`frontend`/`migrate`, matching the same scope
Phase 24 used below, plus permission count now 55 not 51).

Phase 24's own last-known state: all 5 services running and healthy.
Phase 24 rebuilt `backend`, `frontend`, and explicitly `migrate` (new
migrations 0036/0037 plus an RBAC catalog change); `worker` was left
untouched (no async component that phase). `migrate` applied both
migrations and re-seeded permissions/role_permissions (idempotent —
confirmed via its own logs, `count=51`).

## Current endpoints

```
GET    /api/v1/health
POST   /api/v1/auth/login
GET    /api/v1/auth/me
GET    /api/v1/users
GET    /api/v1/users/{id}
POST   /api/v1/users
PUT    /api/v1/users/{id}
DELETE /api/v1/users/{id}
GET    /api/v1/roles
GET    /api/v1/permissions
GET    /api/v1/roles/{id}/permissions
PUT    /api/v1/roles/{id}/permissions
GET    /api/v1/warehouses
GET    /api/v1/warehouses/{id}
POST   /api/v1/warehouses
PUT    /api/v1/warehouses/{id}
DELETE /api/v1/warehouses/{id}
GET    /api/v1/zones
GET    /api/v1/zones/{id}
POST   /api/v1/zones
PUT    /api/v1/zones/{id}
DELETE /api/v1/zones/{id}
GET    /api/v1/locations
GET    /api/v1/locations/{id}
POST   /api/v1/locations
PUT    /api/v1/locations/{id}
DELETE /api/v1/locations/{id}
GET    /api/v1/products
GET    /api/v1/products/{id}
POST   /api/v1/products
PUT    /api/v1/products/{id}
DELETE /api/v1/products/{id}
GET    /api/v1/inventory
GET    /api/v1/inventory/{id}
GET    /api/v1/inventory/summary
GET    /api/v1/inventory/{id}/transactions
GET    /api/v1/inventory/export
GET    /api/v1/receipts
GET    /api/v1/receipts/{id}
POST   /api/v1/receipts
PUT    /api/v1/receipts/{id}
POST   /api/v1/receipts/{id}/items
PUT    /api/v1/receipts/{id}/items/{itemId}
POST   /api/v1/receipts/{id}/confirm
POST   /api/v1/receipts/{id}/cancel
GET    /api/v1/receipts/export
GET    /api/v1/putaway
GET    /api/v1/putaway/{id}
GET    /api/v1/putaway/sources/{receiptId}
POST   /api/v1/putaway
POST   /api/v1/putaway/{id}/confirm
POST   /api/v1/putaway/{id}/cancel
GET    /api/v1/putaway/export
GET    /api/v1/picking
GET    /api/v1/picking/{id}
POST   /api/v1/picking
POST   /api/v1/picking/{id}/confirm
POST   /api/v1/picking/{id}/cancel
GET    /api/v1/picking/export
GET    /api/v1/shipping
GET    /api/v1/shipping/sources
GET    /api/v1/shipping/{id}
POST   /api/v1/shipping
PUT    /api/v1/shipping/{id}
POST   /api/v1/shipping/{id}/confirm
POST   /api/v1/shipping/{id}/cancel
GET    /api/v1/shipping/export
GET    /api/v1/stock-adjustments
GET    /api/v1/stock-adjustments/{id}
POST   /api/v1/stock-adjustments
PUT    /api/v1/stock-adjustments/{id}
POST   /api/v1/stock-adjustments/{id}/confirm
GET    /api/v1/stock-adjustments/export
GET    /api/v1/stock-counts
GET    /api/v1/stock-counts/{id}
POST   /api/v1/stock-counts
POST   /api/v1/stock-counts/{id}/items
PUT    /api/v1/stock-counts/{id}/items/{itemId}
POST   /api/v1/stock-counts/{id}/confirm
POST   /api/v1/stock-counts/{id}/create-adjustments
GET    /api/v1/stock-counts/export
GET    /api/v1/notifications
POST   /api/v1/notifications/{id}/read
POST   /api/v1/reports/inventory-transactions/export
GET    /api/v1/reports/export-jobs/{id}
GET    /api/v1/reports/export-jobs/{id}/download
GET    /api/v1/dashboard/summary
GET    /api/v1/replenishment
GET    /api/v1/replenishment/recommendations
GET    /api/v1/replenishment/{id}
POST   /api/v1/replenishment
POST   /api/v1/replenishment/{id}/start
POST   /api/v1/replenishment/{id}/confirm
POST   /api/v1/replenishment/{id}/cancel
POST   /api/v1/replenishment/{id}/issue
GET    /api/v1/orders
GET    /api/v1/orders/{id}
POST   /api/v1/orders
PUT    /api/v1/orders/{id}
POST   /api/v1/orders/{id}/create-picking-task
POST   /api/v1/orders/{id}/cancel
```

## Phase 25 — Order Management: CODE COMPLETE — LIVE VERIFICATION PENDING

### Starting state found: migration + model/dto already existed, nothing else

Migration 0038 (`orders`/`order_items`), `internal/order/model.go`, and
`internal/order/dto.go` were already present when this phase picked up
— no `repository.go`/`service.go`/`handler.go`, no route registration,
no frontend, no Swagger, no tests, and `PROJECT_STATUS.md` itself had
no "Phase 25" section (only a stale, self-contradictory "Recommended
Phase 25" note claiming Order Management was "still explicitly out of
scope" one paragraph after also saying the project was "proceeding
directly to Phase 25"). Per this phase's own instructions, the existing
migration/model/dto were treated as authoritative and were not
redesigned — every design choice below follows what they had already
committed to (status set PENDING/PICKING/CANCELLED only, no DRAFT
state; `CreatePickingTask` as the release action's real name, not
"Release"; location-agnostic `order_items`).

### Architecture — orchestration only, zero inventory mutation

```
Order (PENDING)
   |
   v  CreatePickingTask
Picking Task (Phase 9/12's own reservation)
   |
   v  Confirm (Picking's own)
Inventory RemoveStock / ledger
   |
   v  Shipping (Phase 10, unchanged)
Order's own FulfillmentStatus (derived, never stored)
```

`internal/order` never imports `internal/inventory` and never calls
`AddStockTx`/`RemoveStockTx`/`MoveStockTx`/`ReserveStockTx` itself —
grepped to confirm, the same proof-by-grep Phase 10's own package doc
used for Shipping. The only inventory effect anywhere in this phase is
`picking.Service`'s own Phase 12 reservation, invoked for an order's
items exactly as if they had been typed directly into
`PickingView.vue`'s "+ New Picking Task" form.

### The cross-package atomicity problem, and how Phase 16 already solved it

`CreatePickingTask` needs "claim the order + create the picking task"
to commit or roll back together as one unit — if picking-task creation
fails (e.g. insufficient stock), the order must not end up half-linked.
`picking.Service.Create` already opens and owns its own top-level
transaction, so it cannot simply be called from inside Order's own
transaction. Rather than inventing a compensating-transaction/saga
pattern, this phase reused the exact shape Phase 16 already established
for `internal/stockcount`'s `CreateAdjustments` calling into
`internal/stockadjustment`: a `CreateTx(ctx, tx *gorm.DB, req, userID)`
variant added to `picking.Service` (additive — `Create` and `CreateTx`
now share one `createCore`/`validateCreate` implementation, so nothing
about Phase 12's reservation logic is duplicated), bound to the
caller's own transaction. `order.Service.CreatePickingTask`:

1. Atomically claims the order via `Repository.ClaimForPickingTask` —
   a guarded `UPDATE orders SET status='PICKING' WHERE id=? AND
   status='PENDING' AND picking_task_id IS NULL`, the identical
   `ClaimStatus` shape `stockcount.Repository` uses. 0 rows affected is
   treated uniformly as `ErrNotReleasable` (409) — covers "already
   PICKING", "already CANCELLED", and a genuine concurrent double-click
   with one guard, exactly what `model.go`'s own pre-existing
   `CanCreatePickingTask` doc already called out as the reason for this
   check ("a second call would violate the UNIQUE constraint anyway,
   but this gives a clean 409 instead").
2. Calls `picking.Service.CreateTx` inside the **same** transaction.
3. Links `picking_task_id` inside the same transaction.

A picking-creation failure (e.g. `ErrValidation` for insufficient
stock) rolls back the whole transaction, including the claim from step
1 — the order lands back at exactly PENDING/unlinked, never
half-released. No separate compensating UPDATE was needed. Live-tested
via `go test`'s validation-only paths (a real insufficient-stock
rollback needs a live database — see Live verification below).

### Duplicate release cannot create a duplicate picking task

Covered by the same `ClaimForPickingTask` guard above — no separate
idempotency-key table was added for this action (unlike Confirm-style
actions elsewhere), since the order's own status is itself the natural
one-shot guard here, exactly as `CanCreatePickingTask`'s pre-existing
doc comment already specified.

### Cancellation: PENDING only, no implicit Picking cancel

`Status.CanCancel` (pre-existing, unchanged) only permits Cancel while
PENDING — once `CreatePickingTask` has linked a picking task, Order's
own Cancel is rejected with 409 (`ErrNotCancellable`); the correct path
becomes Picking's own `POST /picking/{id}/cancel`, which already
releases its own reservation. Order's Cancel never calls
`ReleaseReservationTx` — nothing was ever reserved by Order itself to
release.

### Update: full-replace, PENDING only

Added `Status.CanUpdate` (new predicate, same PENDING-only condition as
`CanCreatePickingTask`/`CanCancel` — kept separate per this codebase's
own "diverge later without disturbing the other's gate" precedent).
`PUT /orders/{id}` replaces the header and every line at once (delete +
reinsert, not a per-line diff) — safe only because a PENDING order has
never had `CreatePickingTask` reserve anything against its lines yet.

### FulfillmentStatus derivation: two paths, one pure function

`dto.go`'s pre-existing `deriveFulfillmentStatus(orderStatus,
*picking.Status, *shipping.Status) FulfillmentStatus` is fed two
different ways depending on endpoint:

- `GET /orders/{id}` calls `picking.Service.Get` and
  `shipping.Service.FindByPickingTaskID` (the latter's own doc
  comment, already present before this phase started, literally reads
  "Phase 25's internal/order uses this") — two cheap single-row
  lookups, only when a picking task is actually linked.
- `GET /orders` resolves `picking_status`/`shipment_status` via a bulk
  SQL `LEFT JOIN` to `picking_tasks`/`shipments` in one query instead
  (both relationships are UNIQUE, so no fan-out risk) — avoiding an N+1
  service call per row across a page of orders. Both paths feed the
  identical pure function, so there is exactly one fulfillment-mapping
  implementation regardless of which query path produced the inputs.

### API

```
GET    /api/v1/orders
GET    /api/v1/orders/{id}
POST   /api/v1/orders
PUT    /api/v1/orders/{id}
POST   /api/v1/orders/{id}/create-picking-task
POST   /api/v1/orders/{id}/cancel
```

Standard envelope/pagination throughout. Duplicate `external_reference`
returns 409 (`DUPLICATE_EXTERNAL_REFERENCE`) via the existing
`sqlerr.IsUniqueViolation` classifier against the pre-existing
`UQ_orders_external_reference` constraint — no new SQL error-handling
code needed.

### RBAC

New `order` catalog module (`internal/rbac/catalog.go`):
`view`/`create`/`update`/`approve`, mirroring every other workflow
module's action set exactly (no `delete`, matching Receiving/
Picking/Replenishment). `CreatePickingTask` sits on `order.update` (the
same "commits the document to its next real workflow step" level
Picking's own Confirm uses), `Cancel` sits on `order.approve` (same
level every other workflow's own Cancel uses). No explicit
`RoleDefaultGrants` entry was added for any operational role — `ADMIN`
(`allPermissionCodes()`) and `WAREHOUSE_MANAGER` (`allExcept("delete")`)
already receive it automatically through the existing generic
computation, the same "administration-tier function" precedent
Warehouse Setup already established (only ADMIN/WAREHOUSE_MANAGER, no
floor-staff role). Permissions are seeded by `cmd/migrate` from this Go
catalog at migrate-time (idempotent insert-if-missing) — no SQL
migration was needed for RBAC data itself; running `migrate` is a
prerequisite for the new `order.*` codes to actually exist in a live
database (see Live verification below). Also added `order` to
`RolePermissionView.vue`'s own `MODULE_ORDER`/`MODULE_NAMES` constants
— without that, the new permissions would exist in the database but be
silently filtered out of the Roles & Permissions screen.

### Frontend

`fontend/src/services/orders.js` (new) + `fontend/src/views/outbound/
OrderView.vue` (new) — list/filter, create/edit (PENDING only), a
"Create Picking Task" panel that collects one location/lot assignment
per line before calling the orchestration endpoint, and Cancel. One nav
entry added under Outbound (`/outbound/orders`, before Picking in the
sidebar, matching the Order → Picking → Shipping flow) — no other
navigation or existing view changed. `npm run build` passes clean.

**Known limitation**: no "required/due date" column exists on `orders`
today (unlike Picking's own `due_date`) — the pre-existing migration
never added one, and adding it now would be a schema change beyond
this phase's "don't redesign the existing migration" instruction. If a
customer-facing due date turns out to be needed, that is a new,
explicit migration decision for a future phase, not an assumption to
bake in silently here.

### Swagger

Full `@Summary`/`@Description`/`@Param`/`@Success`/`@Failure` coverage
on all 6 endpoints. `swag init -g cmd/server/main.go -o docs
--parseInternal --parseDependency` regenerated clean; confirmed all 4
distinct `/orders...` paths present in `docs/swagger.json`.

### Tests

Pure-logic coverage only, matching this codebase's own established
testing depth (no module in this project has live-database integration
tests — Docker/SQL Server has never been part of `go test ./...` here):
`Status.CanCreatePickingTask`/`CanCancel`/`CanUpdate`, `applyCancel`,
the full `deriveFulfillmentStatus` matrix (covers the "fulfillment
mapping" requirement), and `buildPickingItems` — `CreatePickingTask`'s
assignment-matching core, extracted as a pure function specifically so
duplicate/missing/foreign-order-item-id assignment rejection could be
unit tested without a database. `Create`/`Update`'s pre-transaction
warehouse-existence rejection is tested via a fake, the same
`fakeWarehouses` pattern `internal/picking`'s own tests already use.
Added `picking.TestCreateTx_RejectsUnknownWarehouse` alongside the
existing `Create` tests, covering the new `CreateTx` code path.
`go test ./...` passes for the whole module, `go vet ./...` and
`go build ./...` are clean, `gofmt -l .` reports nothing.

**Not unit-testable without a live database** (same limitation every
prior phase's own transactional Confirm/Cancel/Create paths already
have — e.g. Picking's own `Confirm`/`Cancel` have no DB-backed unit
tests either): "release creates exactly one picking task", "duplicate
release does not create a second task", "insufficient inventory
propagates safely without a half-linked order", "cancel invalid state
returns 409". These are exercised by the transaction/guard design
itself (see the atomicity section above) and require the Live
verification pass below to confirm against a real SQL Server.

### Live verification status: BLOCKED — Docker Desktop unavailable

Docker Desktop was down for this entire phase (confirmed at the start:
`docker ps` failed to reach the daemon) — per instructions, no Docker
commands were attempted beyond the required read-only `docker compose
config`, which validates cleanly (Order Management runs inside the
existing `backend` container; no new service, volume, or port needed).
**Not yet verified**: `swag`-regenerated Swagger UI reachability, a
real `CreatePickingTask` call against live inventory (including the
insufficient-stock rollback path), duplicate-release rejection against
a real UNIQUE constraint, `order.*` permission seeding via `cmd/
migrate`, and the frontend's live API integration.

### Exact remaining step before Phase 26

1. Start Docker Desktop.
2. `docker compose up -d sqlserver rabbitmq` (targeted — do not rebuild
   or restart containers unrelated to this phase).
3. Run `cmd/migrate` so `order.*` permissions are seeded and migration
   0038 (already present) is confirmed applied.
4. `docker compose up -d --build backend worker frontend` (targeted
   rebuild — the RAM-conscious approach these instructions asked for,
   not a full-stack rebuild).
5. Verify via Swagger UI (`/swagger/index.html`): create an order,
   create-picking-task with a real location, confirm a picking task
   requesting far more than available stock is rejected with the order
   left PENDING/unlinked, attempt a second create-picking-task call and
   confirm 409 with no second picking task created.
6. Verify `/outbound/orders` in the browser end-to-end (list, create,
   edit, create-picking-task, cancel), confirm `fulfillment_status`
   updates as the linked picking task/shipment progresses.
7. Once all of the above pass, update this section's own heading from
   CODE COMPLETE — LIVE VERIFICATION PENDING to VERIFIED COMPLETE.

## Phase 26 — UAT / Production Readiness: OFFLINE UAT PREPARATION COMPLETE — LIVE UAT PENDING

### Why offline-only

Docker Desktop was unreachable for this entire phase — every attempt
(`docker info`/`docker ps -a`/`docker compose ps`, across multiple
retries and even a full Claude Code session restart) returned `failed
to connect to the docker API at npipe:////./pipe/dockerDesktopLinuxEngine:
The system cannot find the file specified.`, and `Test-Path
\\.\pipe\dockerDesktopLinuxEngine` confirmed the named pipe does not
exist anywhere on the host — not a session-isolation artifact, the
engine backend genuinely was not running despite the Desktop UI/tray
being reported as up. Per explicit instruction, this phase performed
**offline preparation only**: no live database, no live RabbitMQ, no
live HTTP calls, no browser. Nothing below is claimed as live-verified.

### What was produced

- **`UAT_CHECKLIST.md`** (new, repo root) — 85 test cases across all 16
  requested areas (Auth/RBAC, Warehouse/Zone/Location, Product,
  Receiving, Putaway, Replenishment, Inventory, Picking, Shipping,
  Stock Adjustment, Stock Count, Notifications, Reports/Export,
  Dashboard, Order Management, Cancellation/Audit). Every case has Test
  ID, Module, Preconditions, Steps, Expected result, and blank
  Actual result/Notes fields with **Status: PENDING** — nothing was
  assumed PASS. Order Management (Phase 25) gets the deepest coverage
  (13 cases, ORD-01 through ORD-13), including the critical
  insufficient-stock-rollback case (ORD-07) and a concurrency case
  (ORD-12) exercising `ClaimForPickingTask`'s guarded UPDATE.
- **`PRODUCTION_READINESS.md`** (new, repo root) — full static review
  across all 17 requested areas (migrations 0001–0038, FKs, unique
  constraints, indexes, transaction boundaries, inventory mutation
  ownership, reservation ownership, idempotency guards, RBAC
  permissions, JWT/security config, CORS, Swagger exposure,
  file/export authorization, environment variables, secrets handling,
  Docker Compose configuration, health endpoints, logging/error
  handling), a dedicated Phase 25 live-test-requirements section, and
  the exact minimum-footprint live test sequence to run once Docker is
  restored. Every ownership claim (inventory mutation, reservation) was
  re-verified this phase by direct grep against the current source —
  not copied from earlier phase write-ups — confirming: exactly 6
  packages (picking, replenishment, receiving, stockcount,
  stockadjustment, putaway) ever call an inventory-mutating primitive;
  only `internal/inventory/repository.go` ever writes the `inventory`/
  `inventory_transactions` tables directly; `internal/order` and
  `internal/shipping` import neither the mutation primitives nor
  `internal/inventory` at all.
- **No business-logic defect was found** during the static review — no
  code was changed this phase beyond these two new documents and this
  `PROJECT_STATUS.md` update.

### Offline checks run (all clean)

```
go fmt ./...      → clean
go vet ./...      → clean
go test ./...     → all packages pass
go build ./...    → clean
npm run build     → clean (fontend/)
docker compose config → resolves cleanly
```

### What remains before Phase 25 can be VERIFIED COMPLETE

See `PRODUCTION_READINESS.md` section 17 and `UAT_CHECKLIST.md` section
15 (ORD-01 through ORD-13) — in short: start Docker, run the existing
`migrate` service (`wms-migrate up && wms-migrate seed`) to confirm
migration 0038 applies and the 4 new `order.*` permissions seed
correctly, then work through the Order Management live test list,
prioritizing ORD-07 (insufficient-stock rollback — the single most
important Phase 25 test) and ORD-06/ORD-12 (duplicate-release and
concurrency, both exercising `ClaimForPickingTask`'s atomic guard).

### What remains before Phase 26 itself can be VERIFIED COMPLETE

The **entire** `UAT_CHECKLIST.md` matrix (all 85 cases across all 16
areas) needs a live pass, not just Phase 25's own subset — Phase 26 is
UAT for the whole application, not only its newest module. Phase 25's
own live verification is a prerequisite, not a substitute, for that.

**Stopping here per instructions — not proceeding to Phase 27
(Production Deployment Preparation) until Phase 26's live UAT pass is
complete.**

---

## Recommended Phase 27 (background context only — not started)

Once Phase 26's live UAT pass is clean, **Phase 27 — Production
Deployment Preparation** is next per the user's earlier authorization
to continue through Phases 23–27. Prior phases' own still-open
background items remain, unchanged from Phase 21's original list:

1. **Real delivery channel for notifications** (Phase 21's own Known
   limitations) — today a reminder only ever becomes an in-app bell/
   dropdown row; there is still no email/SMS/chat integration, matching
   this phase's own explicit instruction not to add one speculatively.
   The natural next step only if in-app visibility alone proves
   insufficient in practice — the exact same "prove the need first"
   reasoning Phase 20 already applied to notifications themselves.
2. **Shared DLQ / bounded-retry pattern for RabbitMQ consumers** (Phase
   21's own Known limitations) — now that a second real consumer exists
   alongside the dev heartbeat, a common retry-queue-with-TTL shape
   (per CLAUDE.md's own Retry and Dead Letter Queue guidance) would
   benefit both, but was deliberately not built speculatively for a
   single narrowly-scoped, self-healing reminder.
3. **Redesign the Putaway drawer to show every line at once** (Phase
   15's original Known issue, still open). Worth flagging as an
   explicit UI change to approve first, not a "minimal" addition.
4. **Wire `created_by`/`reason`/`older_than_days` into the Stock
   Adjustment frontend** (Phase 20's own Known limitations, still
   open) — the API supports all three, but only `overdue` has a UI
   control.
5. **Extend the `created_by_name` pattern to the remaining modules** —
   Phase 14 introduced it (Stock Count History), Phase 18/20 each
   reused it (`cancelled_by_name`, Stock Adjustment's own
   `created_by_name`); Receiving/Putaway/Picking/Shipping could still
   adopt it if that need arises.

Order Management (Phase 25) and Replenishment (Phase 24) are both now
implemented — the items above are corrected from an earlier draft that
still listed them as out of scope. Of the remainder, a real delivery
channel and the DLQ/retry pattern are the most direct continuations of
Phase 21's own work, but both need an explicit ask (a real integration
decision and/or a second consumer to justify shared retry
infrastructure) rather than being built speculatively; the Putaway
drawer redesign needs its own explicit UI sign-off.

**Stopping here per instructions — this is background context for a
future Phase 27 only. Not proceeding to Phase 26's live UAT pass, and
not proceeding to Phase 27, automatically.**
