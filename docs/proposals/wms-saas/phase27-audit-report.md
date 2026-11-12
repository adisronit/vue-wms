# Phase 27 — Multi-Tenant SaaS Tenant-Isolation Audit Report

**Status:** Audit record only. No code, schema, or data was changed while producing this document.

## 1. Purpose and Method

This is the Step 1 deliverable of the Phase 27 multi-tenant SaaS architecture audit for the WMS backend at `C:\vue\vue-wms\backend` (Go / Gin / GORM / Microsoft SQL Server).

**Purpose:** identify every place in the existing, single-tenant codebase where a future Tenant A could see, create, modify, or delete a future Tenant B's data, *before* any multi-tenancy schema or code changes are designed or implemented.

**Method:** six parallel, independent, read-only source-code audits, each scoped to a distinct module group:

1. Auth / RBAC / Middleware — `internal/auth`, `internal/user`, `internal/role`, `internal/rbac`, `internal/platform/{middleware,http,database,dbctx}`, `config`
2. Master Data — `internal/warehouse`, `internal/zone`, `internal/location`, `internal/product`
3. Inbound — `internal/inventory`, `internal/receiving`, `internal/putaway`
4. Outbound — `internal/picking`, `internal/shipping`, `internal/order`
5. Operations — `internal/stockadjustment`, `internal/stockcount`, `internal/replenishment`, `internal/notification`, `internal/report`, `internal/dashboard`
6. Infra / Background — `internal/messaging/rabbitmq`, `cmd/worker`, `cmd/migrate`, `backend/migrations`, `internal/report` file storage, `internal/health`

No files were modified, no git operations were performed, and no build/test/migration commands were run as part of this audit.

**Headline finding:** the schema contains **zero** `tenant_id` columns anywhere today. Every bare-ID lookup (`WHERE id = ?`), every raw SQL/`Exec` statement, and every cross-module "does this referenced ID exist" check in the codebase is, by construction, an IDOR or cross-tenant-write candidate the moment a second tenant's data enters the same tables. This is expected for a single-tenant system and is not a defect in the existing product — it is exactly the gap this Phase 27 audit exists to enumerate.

Mitigation design is intentionally **not** included in this document — see `tenant-isolation-security.md` (isolation strategy) and `tenant-er-design.md` (schema design) for the proposed fixes. The "Required Change" column below is the audit agents' own per-finding note, not a finalized design.

---

## 2. Master Audit Table

### 2.1 Auth / RBAC / Middleware

| Module | File | Function | Data Ownership | Tenant Risk | Required Change |
|---|---|---|---|---|---|
| auth | internal/auth/token.go | `Claims` struct / `IssueToken` | Tenant-Owned (a user's session) | **Critical** — JWT has no tenant_id/active_tenant_id claim at all; nothing exists for a future check to enforce against | Add `tenant_id` (and if multi-tenant-membership is allowed, `active_tenant_id`) to `Claims` and `IssueToken` signature; sign it in at login |
| auth | internal/auth/service.go | `Login` | Tenant-Owned (credentials + role) | **Critical** — `FindByUsername` has no tenant scoping; usernames are presumably globally unique today, so cross-tenant username collision and cross-tenant login are both unresolved | Scope username uniqueness and lookup by tenant (e.g. `(tenant_id, username)` composite), require tenant selector at login or derive tenant from user row |
| auth | internal/auth/service.go | `Me` / `buildSummary` | Tenant-Owned (user profile + role + permissions) | High — loads role/permissions purely by `RoleID` with no tenant check; if roles become tenant-scoped, a stale/forged RoleID could resolve to another tenant's role | Verify role belongs to caller's tenant before use |
| auth | internal/auth/handler.go | `Register`, `Me` | Platform-Global (routing) | Low — just wires middleware; risk is inherited from JWT/permission layers | None directly; ensure a future `RequireTenantContext` is inserted ahead of handlers once added |
| user | internal/user/model.go | `User` struct | Tenant-Owned (one account) | Critical — no `tenant_id` column; `RoleID`, `WarehouseID` are bare FKs with no tenant boundary | Add `tenant_id` FK, scope unique(username)/unique(email) to `(tenant_id, username)`/`(tenant_id, email)` |
| user | internal/user/repository.go | `FindByID`, `FindByUsername`, `Update`, `Delete`, `List` | Tenant-Owned | Critical — every query filters only by `id`/`username`; any numeric ID from another tenant is fully readable/writable/deletable (IDOR by design once multi-tenant) | Add mandatory `tenant_id` predicate to every query; pass tenant from request context, never trust path/body alone |
| user | internal/user/repository.go:171-177 | `FindActiveIDsWithPermission` | Tenant-Owned | Critical — raw SQL joins `users`/`role_permissions`/`permissions` with zero tenant filter; used by notification fan-out, so a cross-tenant leak here fans out to another system (notification) | Add `tenant_id` filter in the JOIN/WHERE once tenant_id exists on users/roles |
| user | internal/user/service.go | `List/Get/Create/Update/Delete` | Tenant-Owned | High — service layer adds validation but no tenant check; relies entirely on repository, which currently has none | Thread tenant_id from auth context through Service into every Repository call |
| user | internal/user/service.go | `resolveWarehouseID` | Tenant-Owned (cross-module) | High — validates warehouse_id exists at all, not that it belongs to the caller's tenant; same risk will exist in warehouse module | Validate warehouse belongs to same tenant as the user being created/updated |
| role | internal/role/model.go | `Role`, `Permission`, `RolePermission`, `RoleWarehouseAccess` | `Permission` = Platform-Global (catalog); `Role`, `RolePermission`, `RoleWarehouseAccess` = would become Tenant-Owned | Critical — `Role` has no tenant_id; unique is presumably on `code` alone, so two tenants cannot both have a role coded e.g. "ADMIN" | Add `tenant_id` to roles; change unique constraint on `code` to `(tenant_id, code)`; keep `permissions` catalog global (no tenant_id needed) |
| role | internal/role/repository.go | `FindRoleByID`, `FindRoleByCode`, `ListRoles`, `RolePermissionCodes`, `ReplaceRolePermissions`, `RoleWarehouseAccessIDs`, `ReplaceRoleWarehouseAccess` | Tenant-Owned | Critical — all scoped only by `id`/`code`/`role_id`; a role ID from another tenant can be read, have its permissions replaced, or have warehouse access replaced | Add `tenant_id` filter to every query; `ReplaceRolePermissions`/`ReplaceRoleWarehouseAccess` must verify roleID belongs to caller's tenant before delete+insert |
| role | internal/role/repository.go:72-77 | `RolePermissionCodes` (`.Table("permissions").Joins(...)`) | Tenant-Owned (role) joined to Platform-Global (permissions) | High — no tenant filter on `role_permissions.role_id` | Add tenant check upstream (role ownership) since permissions table itself is global and fine |
| role | internal/role/service.go | `SetRolePermissions`, `SetRoleWarehouseAccess`, `GetRoleWarehouseAccess` | Tenant-Owned | Critical — no tenant verification before mutating a roleID's grants; `warehouses.FindByIDs` also has no tenant scoping | Verify role's tenant_id matches caller's tenant before any read/write; same for resolved warehouse IDs |
| rbac | internal/rbac/catalog.go | `Modules`, `PermissionCatalog`, `RoleCatalog`, `RoleDefaultGrants` | Platform-Global (static in-code catalog) | None — universal permission/module catalog seeded identically for every tenant | No change to the catalog itself; only the seeding step (which tenant's `roles`/`role_permissions` rows get created) needs a tenant_id at insert time |
| middleware | internal/platform/middleware/jwt.go | `RequireJWT` | N/A (transport) | High — validates signature/expiry only; no concept of tenant, and cannot since claims carry none | Extend to also require and expose a tenant claim once added, or add a separate `RequireTenantContext` middleware |
| middleware | internal/platform/middleware/claims.go | `CurrentUserID` | N/A | Medium — only extracts `sub`; no equivalent `CurrentTenantID` helper exists yet | Add `CurrentTenantID(c)` reading a future tenant claim, used everywhere `CurrentUserID` is used today |
| middleware | internal/platform/middleware/permission.go | `RequirePermission`, `hasPermission` | N/A (authorization) | **Critical** — checks only the JWT's own embedded `permissions` list; never re-queries the DB. Documented tradeoff: "a permission or role change only takes effect the next time the affected user logs in." A user **removed from or reassigned to a different tenant** keeps their old tenant's permissions/role claims valid until the token expires | Add tenant verification per request, or a token-version/"active_tenant" revocation check for tenant-membership changes |
| middleware | internal/platform/middleware/cors.go | `CORS` | Platform-Global | None — origin policy, not data access | None |
| middleware | internal/platform/middleware/request_id.go | `RequestID`, `GetRequestID` | Platform-Global | None | None |
| middleware | internal/platform/middleware/logging.go | `RequestLogging` | Platform-Global | Low — logs have no tenant_id field, hurting traceability but not an isolation leak itself | Add tenant_id to structured log fields |
| middleware | internal/platform/middleware/recovery.go | `Recovery` | Platform-Global | None | None |
| http | internal/platform/http/response.go | `OK/List/Fail/Status` | Platform-Global (envelope) | None | None |
| http | internal/platform/http/router.go | `NewEngine` | Platform-Global (routing) | None directly, but the single shared `/api/v1` group is where a future `RequireTenantContext` middleware must be inserted globally | Insert a tenant-resolution middleware into the global chain once JWT carries tenant claims |
| database | internal/platform/database/database.go | `Manager`, `Connect`, `dial` | Platform-Global (single shared connection/pool for all tenants) | High — single shared `*gorm.DB`/pool with no per-tenant session context; isolation depends entirely on application-layer WHERE clauses | If Row-Level Security is adopted, pool-level `SESSION_CONTEXT` would need to be set per request |
| dbctx | internal/platform/dbctx/dbctx.go | `Detach`, `WithRequestID` | Platform-Global (context plumbing) | Medium — `Detach` carries arbitrary context values forward, but there is no tenant-carrying context key today, so a tenant value would be silently dropped unless a parallel helper is added | Add `WithTenantID`/`TenantIDFromContext` mirroring `WithRequestID`/`RequestIDFromContext` |
| dbctx | internal/platform/dbctx/log.go | `LogTransaction` | Platform-Global (logging) | Low — no tenant_id field in transaction logs | Add tenant_id to log attrs once available |
| config | config/config.go | `Config`, `DatabaseConfig`, `Load` | Platform-Global | None — confirmed no multi-DB, multi-schema, or tenant-related fields exist; single `DB_NAME` targets one shared database for all tenants | Decide shared-DB-shared-schema (needs tenant_id everywhere, per this report) vs. any per-tenant DB/schema approach |

**Supplementary findings (from this audit's targeted questions):**
- JWT issuance embeds **no** tenant/org concept whatsoever (`internal/auth/token.go:17-22`, `service.go:83`).
- `RequirePermission` never re-queries the database — a stale permission claim becomes a **cross-tenant** risk, not just a stale-permission one, the moment tenant membership can change.
- The DB layer is one shared connection pool (`SetMaxOpenConns(25)`) with no per-request session binding — relevant to the SQL Server Row-Level Security evaluation in `tenant-isolation-security.md`.
- Raw SQL in this layer: `internal/user/repository.go:103-105` (`Exec` last-login update), `internal/user/repository.go:171-177` (`Raw` permission-holder join, no tenant filter), `internal/role/repository.go:72-76` (string-based `.Table().Joins()`, no tenant filter).

### 2.2 Master Data (Warehouse / Zone / Location / Product)

| Module | File | Function | Data Ownership | Tenant Risk | Required Change |
|---|---|---|---|---|---|
| warehouse | repository.go | `FindByID` | Tenant-Owned | Critical — no scoping at all; any ID returns any warehouse | Add `tenantID` param, `WHERE id = ? AND tenant_id = ?` |
| warehouse | repository.go | `FindWithCounts` | Tenant-Owned | Critical — bare `WHERE warehouses.id = ?`, feeds GET/POST/PUT responses | Add tenant filter; subqueries on zones/locations also need tenant_id |
| warehouse | repository.go | `FindByIDs` | Tenant-Owned | High — `WHERE id IN ?` with no tenant filter; used by role/warehouse-access resolution | Add tenant filter param |
| warehouse | repository.go | `Update` | Tenant-Owned | Critical — `WHERE id = ?` only; any known/guessed warehouse ID can be rewritten | Add `AND tenant_id = ?`; verify RowsAffected==0 → NotFound |
| warehouse | repository.go | `Delete` | Tenant-Owned | Critical — `Delete(&Warehouse{}, "id = ?", id)`, no tenant check | Add `AND tenant_id = ?` to the delete clause |
| warehouse | repository.go | `List` | Tenant-Owned | Critical — no tenant filter at all; lists every tenant's warehouses together | Add mandatory `WHERE tenant_id = ?` |
| warehouse | repository.go | `Create` | Tenant-Owned | High — doesn't stamp tenant_id; code uniqueness is global | Add tenant_id to insert; constraint becomes `UNIQUE(tenant_id, code)` |
| warehouse | handler.go / service.go | `Get/Update/Delete` (id from path param) | Tenant-Owned | Critical — handler never extracts tenant from JWT/context and passes it down | Thread tenantID from auth context through Service → Repository on every method |
| warehouse | service.go | `Delete` → `zones.CountByWarehouse` | Tenant-Owned | Medium — count check itself not tenant-scoped once zones gain tenant_id | Scope CountByWarehouse by tenant too |
| zone | repository.go | `FindByID` | Tenant-Owned (inherits via warehouse_id) | Critical — bare `id = ?`; zone has no tenant_id, nothing joins to warehouse to check tenant | Add tenant_id directly on zones (denormalized) + filter |
| zone | repository.go | `Update` / `Delete` | Tenant-Owned | Critical — bare `id = ?`, no ownership check | Add tenant_id column + filter |
| zone | repository.go | `List` | Tenant-Owned | High — scoped by caller-supplied `warehouse_id` only, unverified | Verify warehouse_id belongs to caller's tenant, or add tenant_id filter directly |
| zone | repository.go | `CountByWarehouse` | Tenant-Owned | Medium — trusts warehouse_id without tenant check | Add tenant_id filter |
| zone | service.go | `Create` | Tenant-Owned | Critical (cross-tenant reference) — `warehouses.FindByID` checks existence only, not tenant ownership; a zone can be created under another tenant's warehouse_id | FindByID must verify tenant ownership; reject if warehouse's tenant != caller's tenant |
| zone | service.go | `Get/Update/Delete(id)` | Tenant-Owned | Critical — id-only lookups, no tenant propagated from handler | Thread tenantID through Service and Repository |
| zone | handler.go | `List` (warehouse_id query param) | Tenant-Owned | High (IDOR via query param) — `warehouse_id` taken directly from query string with zero ownership check | Validate warehouse_id belongs to caller's tenant before calling service |
| location | repository.go | `FindByID` | Tenant-Owned (inherits via zone→warehouse) | Critical — bare `id = ?`, two hops removed from tenant with no check anywhere | Add tenant_id column (denormalized) + filter |
| location | repository.go | `Update` / `Delete` | Tenant-Owned | Critical — bare `id = ?`; any tenant can mutate/delete any location by ID | Add tenant_id filter |
| location | repository.go | `List` / `CountByZone` | Tenant-Owned | High — trusts caller-supplied `zone_id` with no tenant check | Add tenant_id filter or verify zone_id's tenant upstream |
| location | service.go | `Create` | Tenant-Owned | Critical (cross-tenant reference) — `zones.FindByID` only checks zone exists, not tenant ownership | zoneChecker.FindByID must verify zone's tenant matches caller's tenant |
| location | handler.go | `List` (zone_id query param) | Tenant-Owned | High (IDOR via query param) — zone_id trusted blindly | Validate zone_id ownership before listing |
| product | repository.go | `FindByID` / `Update` / `Delete` | Tenant-Owned | Critical — bare `id = ?` everywhere | Add tenant_id column + filter on all three; `UNIQUE(sku)` → `UNIQUE(tenant_id, sku)` |
| product | repository.go | `List` | Tenant-Owned | Critical — no tenant filter; search/category/status filters run across all tenants' products together | Add mandatory `WHERE tenant_id = ?` |
| product | service.go | `Create` | Tenant-Owned | High — doesn't stamp tenant_id | Add tenant_id to insert from auth context |
| product | handler.go | all routes | Tenant-Owned | Critical — handler never reads tenant from JWT context at all | Add tenant extraction middleware/context plumbing |
| warehouse/zone/location/product | model.go (all 4) | struct definitions | N/A | High (structural) — none of the 4 models has a `TenantID` field | Add `TenantID int64` to all 4 models + migration |

**Supplementary findings:**
- No `.Raw()`/`.Exec()` calls found in this scope; the only hand-built SQL fragment is `warehouse/repository.go:25-29`'s correlated-subquery `countSelect` used by `List`/`FindWithCounts` — not an injection risk, but needs a tenant_id predicate once added.
- Unique constraints needing a tenant_id component: `warehouses.UQ_warehouses_code` (migrations/0005:20), `zones.UQ_zones_warehouse_code` (migrations/0006:15), `locations.UQ_locations_zone_code` (migrations/0007:18), `products.UQ_products_sku` (migrations/0008:29), filtered index `UQ_products_barcode` (migrations/0008:39).
- Cross-tenant reference risk (child created under a parent ID whose tenant was never checked): `zone/service.go:96 Create`, `location/service.go:80 Create`, plus the `warehouse_id`/`zone_id` query-param IDOR on both modules' `List` handlers.

### 2.3 Inbound (Inventory / Receiving / Putaway)

| Module | File | Function | Data Ownership | Tenant Risk | Required Change |
|---|---|---|---|---|---|
| inventory | model.go | `Inventory` struct | Tenant-Owned | Critical – no tenant_id column; balance row for any company visible/writable by any tenant | Add `tenant_id` to `inventory` table; index on (tenant_id, product_id, location_id, lot_no) |
| inventory | model.go | `Transaction` struct | Tenant-Owned | Critical – ledger has no tenant_id; cross-tenant audit trail leakage | Add `tenant_id` to `inventory_transactions`, denormalized onto the ledger row |
| inventory | repository.go | `List` / `joinedSelect` | Tenant-Owned | High – raw SQL joins warehouses/zones/locations/products but WHERE has no tenant filter | Add `AND w.tenant_id = ?` as a bound param |
| inventory | repository.go | `FindByID` | Tenant-Owned | Critical – fetch by bare `id`, zero tenant check; IDOR | `WHERE id = ? AND tenant_id = ?` |
| inventory | repository.go | `Summary` | Tenant-Owned | Critical – aggregate SUM over entire `inventory`/`products` tables, no tenant filter at all | Add `WHERE inv.tenant_id = ?` to both outer query and `low_stock_count` subquery |
| inventory | repository.go | `Transactions` | Tenant-Owned | High – resolves inventory row via `findRaw(id)` with no tenant check | `findRaw` must filter by tenant_id |
| inventory | repository.go | `ListTransactionsForExport` | Tenant-Owned | Critical – global ledger export query, no tenant filter; async report could dump every tenant's transactions | Add tenant filter param + WHERE clause |
| inventory | repository.go | `findRaw` | Tenant-Owned | Critical – `First(&inv, "id = ?", id)` bare ID | Scope to tenant_id |
| inventory | repository.go | `FindBalance` | Tenant-Owned | Critical – keyed by (product_id, location_id, lot_no) only; no ownership check anywhere in the call chain | Add tenant_id column + filter |
| inventory | repository.go | `AdjustBalance` (raw Exec) | Tenant-Owned | Critical – atomic UPDATE keyed only on product_id/location_id/lot_no; a tenant-mismatched pair colliding numerically would corrupt another tenant's stock | Add `tenant_id = ?` into the Exec's WHERE clause itself |
| inventory | repository.go | `AdjustReservation` (raw Exec) | Tenant-Owned | Critical – same atomic-UPDATE pattern, no tenant_id in WHERE | Add `tenant_id = ?` to WHERE |
| inventory | repository.go | `CreateBalance` | Tenant-Owned | High – inserts row without tenant_id | Stamp tenant_id on insert from caller's auth context |
| inventory | repository.go | `InsertTransaction` | Tenant-Owned | High – ledger insert has no tenant_id | Stamp tenant_id on insert |
| inventory | service.go | `AddStock`/`AddStockTx`/`RemoveStock`/`RemoveStockTx`/`MoveStock`/`MoveStockTx` | Tenant-Owned | Critical – request structs carry no tenant_id; service trusts caller already validated | Thread tenant_id through every request struct and repository call |
| inventory | service.go | `ReserveStockTx`/`ReleaseReservationTx`/`ConsumeReservationTx` | Tenant-Owned | Critical – reservation claims against balance with no tenant check | tenant_id required in `ReserveRequest` and in `AdjustReservation`'s WHERE |
| inventory | service.go | `FindBalance` (passthrough) | Tenant-Owned | Critical – exposed to putaway/receiving with no tenant param | Add tenant_id parameter |
| inventory | handler.go | `List`/`Get`/`Summary`/`Transactions`/`Export` | Tenant-Owned | High – handler never extracts/passes tenant context to service | Add tenant_id extraction from JWT claims/middleware |
| receiving | model.go | `Receipt`/`ReceiptItem`/`Confirmation` | Tenant-Owned | Critical – no tenant_id anywhere; receipt_no from global IDENTITY | Add tenant_id to `receipts`, `receipt_items`, `receipt_confirmations` |
| receiving | repository.go | `FindReceiptByID` | Tenant-Owned | Critical – bare `First(&rec, "id = ?", id)`; IDOR path for Update/Confirm/Cancel | Scope by tenant_id |
| receiving | repository.go | `FindReceiptDetail` (raw SQL) | Tenant-Owned | Critical – raw join `WHERE rc.id = ?`, no tenant filter | Add tenant filter to raw SQL |
| receiving | repository.go | `ListReceipts` (raw SQL) | Tenant-Owned | High – no tenant filter in WHERE builder | Add tenant_id param to WHERE |
| receiving | repository.go | `CountOpen` | Tenant-Owned | Medium – dashboard count across all tenants' open receipts | Add tenant filter |
| receiving | repository.go | `FindItemByID` | Tenant-Owned | Critical – bare ID lookup; only extra check is unscoped `item.ReceiptID != receiptID` | Scope by tenant_id |
| receiving | repository.go | `FindItemsByReceipt` | Tenant-Owned | Medium – trusts receiptID already validated | Fine once receiptID lookups upstream are tenant-scoped |
| receiving | repository.go | `AdjustReceivedQty` (raw Exec) | Tenant-Owned | High – atomic increment keyed only by `id` | Add tenant_id to WHERE for defense in depth |
| receiving | repository.go | `InsertConfirmation` | Tenant-Owned | Medium – idempotency key is global; cross-tenant key collision could suppress/confuse another tenant's confirm | Consider composite key (tenant_id, idempotency_key) |
| receiving | service.go | `Create` | Tenant-Owned | Critical – validates warehouse/location/product existence only, not tenant ownership | Pass tenant_id into every `FindByID` call; reject cross-tenant resolution |
| receiving | service.go | `Update`/`AddItem`/`createItem`/`UpdateItem` | Tenant-Owned | Critical – `createItem` never checks tenant ownership of referenced product/location | Tenant-scoped existence checks |
| receiving | service.go | `Confirm` | Tenant-Owned | Critical – the write path into inventory; locationID checked only for existence before `inventory.AddStockTx` | `FindByID` calls and `AddStockTx` must carry/verify tenant_id |
| receiving | service.go | `Cancel` | Tenant-Owned | High – `FindReceiptByID` unscoped | Scope lookup by tenant |
| receiving | handler.go | all routes | Tenant-Owned | High – no tenant context extracted/forwarded anywhere | Add tenant extraction/propagation |
| putaway | model.go | `Task`/`TaskItem`/`Confirmation` | Tenant-Owned | Critical – no tenant_id; putaway_no sequence global | Add tenant_id to `putaway_tasks`, `putaway_task_items`, `putaway_confirmations` |
| putaway | repository.go | `FindTaskByID` | Tenant-Owned | Critical – bare ID lookup | Scope by tenant_id |
| putaway | repository.go | `FindTaskDetail` (raw SQL) | Tenant-Owned | Critical – `WHERE pt.id = ?`, no tenant filter | Add tenant filter |
| putaway | repository.go | `List`/`ListItems` (raw SQL) | Tenant-Owned | High – no tenant filter | Add tenant_id param |
| putaway | repository.go | `FindItemByID` | Tenant-Owned | Critical – bare ID; only extra check is unscoped `item.PutawayTaskID != taskID` | Scope by tenant_id |
| putaway | repository.go | `AdjustMovedQty` (raw Exec) | Tenant-Owned | High – atomic increment by bare `id` | Add tenant_id to WHERE |
| putaway | repository.go | `resolveLocation` (raw SQL) | Tenant-Owned | Critical – resolves warehouse/zone/status for a locationID with zero tenant filter; this is the sole gatekeeper for "is this location mine" and cannot answer that today | Add tenant_id filter / return tenant_id for caller comparison |
| putaway | repository.go | `SourceLines` (raw SQL) | Tenant-Owned | Critical – joins receipt_items/products/locations by receiptID only | Add tenant filter |
| putaway | service.go | `Create`/`createItem` | Tenant-Owned | Critical – `ErrCrossWarehouse` check only verifies warehouse_id equality, not tenant ownership | Verify `task.WarehouseID` belongs to caller's tenant before any location resolution |
| putaway | service.go | `Confirm` | Tenant-Owned | Critical – `destinationID`/`item.SourceLocationID` go straight into `resolveLocation` then `inventory.MoveStockTx` with no tenant check anywhere | See cross-cutting findings §3 |
| putaway | service.go | `Cancel` | Tenant-Owned | High – `FindTaskByID` unscoped | Scope by tenant |
| putaway | handler.go | all routes | Tenant-Owned | High – no tenant extraction/propagation | Add tenant context plumbing |

**Supplementary findings — end-to-end trace (Receiving → Putaway → Inventory):**
Receipt line added via `createItem` (`receiving/service.go:230`) checks `s.locations.FindByID` for existence only. `Confirm` (`receiving/service.go:317`) re-checks the same way, then calls `inventory.AddStockTx` → `inventory/repository.go:303 AdjustBalance`, keyed by product_id/location_id/lot_no with **no tenant check anywhere in this chain**. Putaway's `createItem` (`putaway/service.go:184`) calls `resolveLocation` (`putaway/repository.go:359`), which returns warehouse_id/zone_type/status with **no tenant_id at all** — the only check possible is a numeric `WarehouseID` match, not a tenant match, and `task.WarehouseID` itself was never tenant-verified at `Create` either. **Single highest-leverage slip point:** `putaway/repository.go:359 resolveLocation` and the `locationChecker`/`productChecker`/`warehouseChecker` interfaces in receiving/putaway's service layer — these are the only gatekeepers between a request body's IDs and a stock-mutating call, and none of them can express "does this belong to my tenant" today.

Raw SQL/Exec quantity-math sites in this scope: `inventory/repository.go:304-308` (`AdjustBalance`), `:325-331` (`AdjustReservation`), `receiving/repository.go:324-328` (`AdjustReceivedQty`), `putaway/repository.go:315-319` (`AdjustMovedQty`) — plus list/detail/export Raw SQL at `inventory/repository.go:122-134,146,171-184,261`, `receiving/repository.go:92-106,176,194,247-256`, `putaway/repository.go:92-104,180,197,235-248,361-367,399-409`.

### 2.4 Outbound (Picking / Shipping / Order)

| Module | File | Function | Data Ownership | Tenant Risk | Required Change |
|---|---|---|---|---|---|
| picking | model.go | Task, TaskItem, Confirmation | Tenant-Owned | Critical — no tenant_id on picking_tasks/items/confirmations | Add tenant_id to picking_tasks (denormalize onto items or join); backfill via warehouse_id |
| picking | repository.go:51 `FindTaskByID` | bare `id=?` | Tenant-Owned | Critical — any caller can fetch/mutate any task by ID | Add `AND tenant_id = ?` to all task lookups |
| picking | repository.go:89 `FindTaskDetail` (raw) | bare `pt.id=?` | Tenant-Owned | Critical — detail GET is cross-tenant readable | Add tenant filter to raw SQL WHERE |
| picking | repository.go:126 `CancelTask` | `Where("id=?")` | Tenant-Owned | Critical — cross-tenant cancel releases Tenant B's reservation | Scope UPDATE by tenant_id; verify tenant before calling |
| picking | repository.go:279 `FindItemByID` / :308 `AdjustPickedQty` | bare `id` | Tenant-Owned | Critical — item-level ops unscoped | Scope by tenant via parent task join or denormalized column |
| picking | repository.go:352 `resolveLocation` | location/warehouse lookup | Tenant-Owned (via warehouse) | High — no check that location belongs to caller's tenant | Join tenant_id on warehouses/locations and compare |
| picking | service.go:111 `Create` / :198 `createItem` | reserves stock by product/location/lot | Tenant-Owned | Critical — product_id/location_id trusted from client body with zero tenant validation before `ReserveStockTx` | Validate warehouse_id, product_id, location_id all belong to caller's tenant before reserving |
| picking | service.go:261 `Confirm` | `ConsumeReservationTx`/`ReleaseReservationTx` by taskID | Tenant-Owned | Critical — stock-reservation integrity bug: wrong-tenant task could consume/release another tenant's reserved balance | Verify task.tenant_id == caller tenant before transaction opens |
| picking | service.go:419 `Cancel` | `ReleaseReservationTx` | Tenant-Owned | Critical — cross-tenant cancel-and-release is a stock integrity bug, not just a privacy leak | Same: verify tenant_id before CanCancel gate |
| picking | service.go:153 `CreateTx` | callable by order.Service | Tenant-Owned | Critical — trusts caller (order) implicitly; no tenant param at all | Thread tenant_id through CreateTx signature and validate |
| picking | handler.go | all routes | — | High — RBAC permission checked, but no ownership/tenant check anywhere in the chain | Add tenant-scoping middleware/service-layer check project-wide |
| shipping | model.go | Shipment, Item, Confirmation | Tenant-Owned | Critical — no tenant_id; linked only via picking_task_id | Add tenant_id to shipments (denormalized from picking task) |
| shipping | repository.go:48 `FindByID` / :64 `FindByPickingTaskID` | bare id | Tenant-Owned | Critical — unscoped | Add tenant filter |
| shipping | repository.go:151 `UpdateStatus` / :167 `CancelShipment` | bare `id=?` | Tenant-Owned | Critical — cross-tenant dispatch/deliver/cancel | Scope by tenant_id |
| shipping | repository.go:297 `Sources` | lists all completed picking tasks warehouse-wide | Tenant-Owned | Critical — exposes every tenant's shippable tasks in one query | Add tenant filter to WHERE/JOIN |
| shipping | service.go:129 `Create` | `s.picking.Get(ctx, req.PickingTaskID)` by bare ID | Tenant-Owned | Critical — Tenant A can ship Tenant B's completed picking task | Verify picking task's tenant_id before creating shipment |
| shipping | service.go:343 `Cancel` / :249 `Confirm` | bare shipment id | Tenant-Owned | Critical — cross-tenant status transitions | Scope by tenant_id |
| order | model.go | Order, Item | Tenant-Owned | Critical — no tenant_id; only warehouse_id | Add tenant_id to orders table |
| order | repository.go:58 `FindByID` / :74 `FindByOrderNo` / :92 `FindByExternalReference` | bare id/no/ref | Tenant-Owned | Critical — any order can be fetched/mutated by ID or guessed order_no | Scope all three by tenant_id |
| order | repository.go:144 `ClaimForPickingTask` (raw Exec) | bare `id=?` | Tenant-Owned | Critical — claims/locks any order regardless of tenant | Add `AND tenant_id=?` to UPDATE |
| order | repository.go:156 `LinkPickingTask` / :165 `CancelOrder` | bare `id=?` | Tenant-Owned | Critical — unscoped writes | Scope by tenant_id |
| order | service.go:181 `Create` | validates warehouse_id/product_id existence only, not ownership | Tenant-Owned | High — Tenant A order can reference Tenant B's warehouse/product | Validate warehouse and every product belong to caller's tenant |
| order | service.go:297 `CreatePickingTask` | fetches order by bare ID, passes client `LocationAssignment` straight into `picking.CreateTx` | Tenant-Owned | Critical — the core cross-module IDOR: no tenant check on the order fetch or on client-supplied location_id/lot_no | Verify order.tenant_id == caller; verify every LocationAssignment.LocationID belongs to caller's tenant |
| order | handler.go | all routes | — | High — permission-gated only, no ownership check | Add tenant middleware |

**Supplementary findings — Order → Picking → Shipping trace:**
`order.Service.CreatePickingTask` (order/service.go:297) fetches the order via a bare-ID `FindByID`, then passes client-supplied `LocationAssignment{LocationID, LotNo}` into `picking.Service.CreateTx` → `createItem` (picking/service.go:198), where `resolveLocation` only checks the location's warehouse equals the order's own `WarehouseID` — never that order/warehouse/location belong to the caller's tenant. Downstream, `shipping.Service.Create` (shipping/service.go:129) calls `s.picking.Get` with a bare, client-supplied ID — Tenant A can create a shipment for any completed picking task regardless of owner. **Single highest-risk choke point: `order.Service.CreatePickingTask`.**

All picking reservation mutation funnels through exactly three `internal/inventory` entry points — `ReserveStockTx`, `ConsumeReservationTx`/`ReleaseReservationTx` — all backed by `inventory.Repository.AdjustReservation`, keyed only on `(product_id, location_id, lot_no)`. Shipping never calls inventory directly; order only reaches it via `picking.CreateTx`. A tenant check inserted once in picking's service layer (`createItem`/`Confirm`/`Cancel`) would cover every reservation path in this scope — but it must live there, since `inventory`'s Tx methods have no tenant notion at all.

### 2.5 Operations (Stock Adjustment / Stock Count / Replenishment / Notification / Report / Dashboard)

| Module | File | Function | Data Ownership | Tenant Risk | Required Change |
|---|---|---|---|---|---|
| stockadjustment | model.go | `Adjustment`, `Item`, `Confirmation` structs | Tenant-Owned | Critical – no tenant_id column at all | Add `tenant_id` to `stock_adjustments`, `stock_adjustment_items`, `stock_adjustment_confirmations`, `stock_adjustment_reminders`; index `(tenant_id, id)` |
| stockadjustment | repository.go | `FindByID`, `FindItemByAdjustmentID`, `UpdateItemFields`, `UpdateItemAfterApprove`, `UpdateStatus` | Tenant-Owned | Critical – all lookups/updates are `WHERE id = ?` only | Every method must take tenant_id and add `AND tenant_id = ?` |
| stockadjustment | repository.go | `List`, `FindDetail`, `FindBySourceCountID`, `FindOverduePending` | Tenant-Owned | Critical – raw SQL joins with no tenant predicate | Add `sa.tenant_id = ?` as a bound param, never string-concat |
| stockadjustment | repository.go | `InsertConfirmation`, `ClaimReminder` | Tenant-Owned | High – idempotency key table has no tenant scoping; reminder claim has no tenant check | Add tenant_id to confirmation/reminder tables; scope ClaimReminder by tenant_id |
| stockadjustment | service.go | `Confirm` (Approve/Reject) | Tenant-Owned | Critical – an approver fetched adjustment with no tenant check could approve another tenant's adjustment and mutate their inventory | `FindByID` inside the tx must verify `adjustment.TenantID == callerTenantID` |
| stockadjustment | service.go | `Create`, `CreateTx`, `Update` | Tenant-Owned | High – new rows never stamped with tenant_id; existence checks only, not ownership | Stamp `TenantID` from authenticated context; checkers must verify tenant ownership |
| stockadjustment | handler.go | `Get`, `Update`, `Confirm`, `Export` | Tenant-Owned | Critical – `id` path param passed straight through with no tenant context extracted | Middleware must inject tenant_id into ctx; propagate end-to-end |
| stockadjustment | reminder.go | `ClaimAndListOverdueForReminder` | Tenant-Owned | High – scans ALL pending adjustments warehouse-wide with no tenant filter | `FindOverduePending` must filter by tenant_id |
| stockcount | model.go | `Count`, `Item`, `Confirmation` | Tenant-Owned | Critical – no tenant_id anywhere | Add tenant_id to `stock_counts`, `stock_count_items`, `stock_count_confirmations` |
| stockcount | repository.go | `FindByID`, `UpdateStatus`, `ClaimStatus`, `FindItemByID`, `FindItemsByCount`, `RecordCount` | Tenant-Owned | Critical – bare-ID queries/updates, even the atomic `ClaimStatus` guard is unscoped | Add tenant_id to every WHERE clause including the atomic guard |
| stockcount | repository.go | `List`, `ListItems`, `FindDetail` | Tenant-Owned | Critical – raw SQL with no tenant filter in joins | Add tenant_id predicate |
| stockcount | service.go | `RecordCount`, `Confirm`, `CreateAdjustments` | Tenant-Owned | Critical – Confirm/CreateAdjustments mutate inventory and create stockadjustment rows with no tenant check; cross-module call doesn't pass/verify tenant either | Verify count.TenantID matches; pass tenant_id through to generated CreateTx call |
| stockcount | handler.go | `Get`, `RecordCount`, `Confirm`, `CreateAdjustments` | Tenant-Owned | Critical – same bare path params, no tenant scoping | Thread tenant_id end-to-end |
| replenishment | model.go | `Task` | Tenant-Owned | Critical – no tenant_id | Add tenant_id to `replenishment_tasks`, `replenishment_confirmations` |
| replenishment | repository.go | `FindTaskByID`, `UpdateTask`, `CancelTask`, `FindTaskDetail`, `List`, `resolveLocation`, `ListForRecommend` | Tenant-Owned | Critical – all bare-ID/raw-SQL, no tenant filter; `ListForRecommend` scans entire inventory/products/locations for a warehouse with no tenant predicate | Add tenant_id to all queries; Recommend must only consider caller's tenant's own data |
| replenishment | service.go | `Confirm`, `Cancel`, `ReportIssue`, `Start` | Tenant-Owned | Critical – fetch task by bare ID then call `inventory.MoveStockTx`/`ReleaseReservationTx` with no tenant verification | FindTaskByID must enforce tenant match before any mutation |
| replenishment | handler.go | `Get`, `Start`, `Confirm`, `Cancel`, `ReportIssue`, `Recommend` | Tenant-Owned | Critical – `warehouse_id` is a caller-supplied query param with no tenant check | `warehouseChecker.FindByID` must be tenant-scoped |
| notification | model.go | `Notification` | Tenant-Owned | Medium – no tenant_id, but access is already user_id-scoped so cross-tenant leak requires a cross-tenant user_id collision | Add tenant_id for defense-in-depth and query efficiency |
| notification | repository.go | `FindByID`, `SetRead`, `List` | Tenant-Owned | Low-Medium – `FindByID` itself has no tenant/user filter (relies on Service.MarkRead's post-fetch check) | Add `AND tenant_id = ?` for defense in depth |
| notification | service.go | `MarkRead`, `NotifyStockAdjustmentReminder` | Tenant-Owned | Medium – good ownership check on MarkRead, but recipient resolution (`FindActiveIDsWithPermission`) is not tenant-scoped, so a reminder could be delivered to another tenant's approvers | `FindActiveIDsWithPermission` (internal/user) must be tenant-scoped to the adjustment's own tenant |
| report | model.go | `ExportJob` | Tenant-Owned | Critical – no tenant_id; `RequestedBy` is the only scoping field | Add tenant_id to `export_jobs` |
| report | repository.go | `FindByID`, `Claim`, `MarkCompleted`, `MarkFailed`, `ListForUser` | Tenant-Owned | High – bare-ID queries; ownership enforced only in Service layer via `RequestedBy`, not at repo/SQL level | Add tenant_id to FindByID/Claim as defense-in-depth |
| report | service.go | `GetJob`, `ResolveDownload`, `ProcessJob`, `writeCSV` | Tenant-Owned | Critical – `ProcessJob`'s `ListTransactionsForExport` call is not tenant-scoped; the generated CSV could contain all tenants' inventory transactions regardless of who requested it | `TransactionExportFilters` must carry tenant_id; `ListTransactionsForExport` must filter by it |
| report | handler.go | `Download`, `GetJob`, `CreateInventoryTransactionsExport` | Tenant-Owned | Medium (download path itself is fine) – correctly checks `job.RequestedBy != userID`; underlying data in the file is unscoped (see above) | Also verify job.TenantID == caller tenant once that field exists |
| dashboard | service.go | `Summary` | Platform-Global aggregation of Tenant-Owned data | Critical (inherited) – calls receiving/picking/inventory summarizers with no tenant_id passed; once dependencies are tenant-scoped, dashboard numbers will silently mix or leak unless ctx carries tenant_id | Ensure ctx passed into Summary carries tenant_id |
| dashboard | handler.go | `Summary` | — | Low (thin passthrough) | None beyond ensuring middleware sets tenant_id in ctx first |

**Supplementary findings:**
- Report export jobs are keyed by a sequential `int64` IDENTITY (not a UUID); the file name (`inventory_transactions_<jobID>_<timestamp>.csv`) is derived from it. The HTTP download handler never trusts a client-supplied path and always re-checks `RequestedBy` before serving — so pre-tenant, guessing a job_id does not leak another user's file. The real leak surface is the **content** of the file (`ListTransactionsForExport` is not tenant-filtered).
- Dashboard has no own repository/raw SQL — it is a clean pass-through aggregator over receiving/picking/inventory summarizers, so fixing those three closes dashboard's exposure with no dashboard-specific code change, but this also makes a missed scope in any one dependency an easy regression to overlook.
- Notifications are always addressed to a specific `user_id`, never broadcast; the one indirect leak surface is recipient *resolution* at write time (`FindActiveIDsWithPermission`, audited under §2.1), not notification's own storage/read path.
- Raw SQL in this scope: `stockadjustment/repository.go:195,213,268-285,303-310,349-354,390-394,405-409`; `stockcount/repository.go:82-87,175-176,194,230-241,283-295`; `replenishment/repository.go:116-136,169-184,200-206,248-259`.

### 2.6 Infra / Background (Messaging, Worker, Migrations, File Storage, Health)

| Module | File | Function | Data Ownership | Tenant Risk | Required Change |
|---|---|---|---|---|---|
| messaging | internal/messaging/rabbitmq/connection.go | `Manager` | Infra-only (AMQP conn) | None — no business data touched | None |
| messaging | internal/messaging/rabbitmq/consumer.go | `Consume` | Generic delivery loop, no payload awareness | High (structural) — handler contract has no tenant hook; any future tenant check must be added inside each Handler | Add optional tenant-context extraction point in the generic Handler signature |
| messaging | internal/messaging/rabbitmq/publisher.go | `Publish` | Generic byte-body publish, no envelope awareness | High (structural) — publisher cannot enforce that callers include tenant_id since it only takes `[]byte` | Enforce tenant_id at the per-domain message-constructor layer |
| messaging | internal/messaging/rabbitmq/topology.go | `DeclareTopology` / queue consts | All 3 queues (`wms.system.health`, `wms.stock_adjustment.pending_reminder`, `wms.report.export`) are single global queues shared by all tenants | High — no per-tenant partitioning; a backlog for Tenant A sits in the same queue as Tenant B | If per-tenant isolation/throttling is ever required, routing key or queue naming must incorporate tenant_id; at minimum message body must carry tenant_id |
| messaging | internal/messaging/rabbitmq/dev_heartbeat.go | `DevHeartbeat`/`NewDevHeartbeat` | Dev/infra only | None | None (dev-only, should be removed before multi-tenant work per its own doc comment) |
| worker | cmd/worker/main.go | `stockAdjustmentReminderHandler` | Decodes `stockadjustment.ReminderMessage`, calls `notifications.NotifyStockAdjustmentReminder` with bare background ctx | **Critical** — no tenant_id in message, no tenant check before DB write; approver resolution is global | Message must carry tenant_id (or resolvable warehouse_id); notification fan-out must filter by same tenant as the adjustment |
| worker | cmd/worker/main.go | `reportExportHandler` | Decodes `report.Message` (job_id only), calls `reports.ProcessJob` with bare background ctx | **Critical** — `ProcessJob` queries `inventory.ListTransactionsForExport` with zero tenant scoping | export_jobs row must carry tenant_id; ProcessJob must pass it through |
| worker | cmd/worker/main.go | `main`/`startConsumingWhenReady` | Single shared `dbManager.DB()` for all tenants, no per-tenant scoping | High (structural) — confirms single shared GORM pool | Any RLS strategy must be enforced in every repository query, not here |
| platform | internal/platform/dbctx/dbctx.go | `Detach`/`WithRequestID` | Carries request_id across HTTP boundary only; **never called by worker** | **Critical** — confirms worker path has zero connection to request-scoped context machinery; nothing resembling tenant_id would propagate automatically | A worker-side `WithTenantID` must be added and threaded explicitly from the decoded message into ctx |
| report | internal/report/service.go | `CreateInventoryTransactionsJob`/`ProcessJob`/`ResolveDownload`/`GetJob` | `export_jobs` row keyed by `RequestedBy` (user_id) only | High — ownership check by user_id, not tenant; `ProcessJob` (worker side) does zero ownership/tenant check, only trusts the job row | Add tenant_id to export_jobs; check it in ResolveDownload/GetJob and filter the ProcessJob query by it |
| report (file storage) | internal/report/service.go | `ProcessJob` (writeCSV) + csvexport `SafeFilename` | Filename = `inventory_transactions_<jobID>_<timestamp>.csv`, written to one shared `exportDir` volume for all tenants | **Critical** — all tenants' files land in the same flat directory; filename derived from a sequential/guessable job_id; only gate is the DB `RequestedBy` check | Partition exportDir by tenant_id; keep DB-level ownership+tenant check as the access gate |
| report (download) | internal/report/handler.go | `Download` | Serves `c.File(filePath)` after `ResolveDownload` ownership check | High — correctly gated by user_id today; a future regression (e.g. a support-tooling bypass) could serve any tenant's file with no tenant_id to also check | Add tenant_id to the same ownership check once introduced |
| migrate tool | cmd/migrate/main.go | `migrateUp`/`migrateDown`/`seed` | DDL runner + RBAC/admin-user seed | High (process risk) — `seed` creates one global admin user with no tenant assignment; no default warehouse seeded | Phase-27 backfill plan must assign the seeded admin user to "tenant #1"; seed script needs a tenant_id parameter |
| migrations (sampled) | 0004_create_users_table, 0005_create_warehouses_table, 0009_create_inventory_table | — | Schema definitions | Confirmed clean — no org_id/company_id/account_id/tenant_id anywhere in sampled files | All tenant-scoped tables need a new tenant_id column + updated unique constraints |
| health | internal/health/health.go, handler.go | `Checker.Check`/`Handler.Get` | Returns only `{status, database, rabbitmq}` connectivity | None — no tenant-identifying data, no counts, no connection strings | None |

**Supplementary findings:**
- **No current message schema carries a usable tenant_id.** `report.Message` carries only `job_id` (the worker re-reads everything from the DB row). `stockadjustment.ReminderMessage` carries `CreatedBy *int64` (a real user_id, documented as "context for notification text, not a recipient list") — a correlatable field, but the consumer's actual recipient-resolution logic ignores it and is unscoped today.
- **The worker talks to the database with zero authorization or tenant check by construction.** `RequireJWT`/`RequirePermission` exist only as Gin HTTP middleware, never invoked in the `cmd/worker` binary. Whatever a consumed message's payload says, the worker does — full stop. This is flagged as the single highest architectural risk in the infra/background scope.
- Migration tool mechanism: version-prefixed `NNNN_name.up.sql`/`.down.sql`, tracked in a `schema_migrations` table, applied inside a transaction each; `down` rolls back only the most-recently-applied version; `seed` is idempotent.

---

## 3. Cross-Cutting Findings

**(a) The universal "bare-ID lookup, no ownership check" pattern.** Across all six audits, roughly **60+ distinct repository/service call sites** follow the identical shape: a handler reads an `id` path/query parameter, passes it straight to a repository method that does `WHERE id = ?` (or an equivalent raw-SQL `WHERE x.id = ?`), with no ownership/tenant predicate anywhere in the call chain. This is not a handful of isolated bugs — it is the default, universal data-access pattern in the current codebase, consistent with it being single-tenant by design. Every one of these becomes an IDOR candidate the day a second tenant's row exists in the same table.

**(b) Raw SQL / Exec quantity-math statements requiring `tenant_id` directly in their WHERE clause** (not just an app-layer check beforehand, since these are atomic, hand-written UPDATEs):
- `inventory/repository.go` `AdjustBalance`, `AdjustReservation`
- `receiving/repository.go` `AdjustReceivedQty`
- `putaway/repository.go` `AdjustMovedQty`
- `picking/repository.go` `AdjustPickedQty`
- `order/repository.go` `ClaimForPickingTask`

These five are the highest-value targets: a tenant-mismatched write here doesn't just leak data, it silently corrupts another tenant's on-hand quantity, reserved quantity, or task progress.

**(c) The "existence checker, not ownership checker" pattern.** Every cross-module reference validation found — `receiving`'s `locationChecker`/`productChecker`/`warehouseChecker`, `putaway`'s equivalents, `picking`'s `resolveLocation`, `order`'s warehouse/product validation in `Create`, `zone`/`location`'s parent-FK checks in `Create`, `stockadjustment`/`replenishment`'s item/location/warehouse checkers — all answer "does this ID exist at all?" and none currently can answer "does this ID belong to the caller's tenant?" This is the single most common defect class found, spanning every module group.

**(d) The worker process has zero authorization path.** `cmd/worker/main.go` never invokes `RequireJWT` or `RequirePermission` (they are Gin HTTP middleware only) and never calls `dbctx.WithRequestID`/a tenant equivalent. Once a message is on the queue, the worker will act on it against the database with no tenant or permission check of any kind, by construction. Combined with finding (a) — repository methods have no tenant parameter to enforce against even if the worker wanted to check — this is assessed as **the single highest architectural risk** in the entire audit, because it is a systemic gap rather than a single fixable call site.

**(e) Export file storage is a shared, unpartitioned volume.** `internal/report`'s generated CSVs all land in one `export_files` Docker volume with sequential, guessable job-ID-based filenames. The only current access control is a DB-row `RequestedBy` check inside the HTTP download handler — there is no filesystem-level or path-level tenant partitioning, and the *content* of the file (via `ListTransactionsForExport`) is not tenant-filtered at all.

**(f) Stale JWT permission claims become a cross-tenant risk.** `RequirePermission` enforces only the permission list embedded in the JWT at login time and never re-queries the database (`internal/platform/middleware/permission.go`, a documented existing tradeoff). Today this means a revoked/changed permission takes up to one token lifetime (default 1h) to take effect. Once tenant membership can change (a user removed from a tenant, or a tenant suspended), the same mechanism means a token issued before the change continues to grant access to that tenant's data for up to the token's remaining lifetime — turning an accepted staleness tradeoff into an active cross-tenant access window.

---

## 4. Risk Summary

| Module Group | Critical | High | Medium | Low | None | Total Rows |
|---|---|---|---|---|---|---|
| Auth / RBAC / Middleware | 9 | 6 | 2 | 3 | 7 | 27 |
| Master Data | 16 | 8 | 2 | 0 | 0 | 26 |
| Inbound | 27 | 13 | 3 | 0 | 0 | 43 |
| Outbound | 20 | 4 | 0 | 0 | 0 | 24 |
| Operations | 17 | 4 | 4 | 1 | 0 | 26 |
| Infra / Background | 4 | 8 | 0 | 0 | 3 | 15 |
| **Total** | **93** | **43** | **11** | **4** | **10** | **161** |

93 of 161 audited findings (58%) are rated Critical — consistent with auditing a system that has no tenant concept at all today. This is the expected shape of a pre-implementation audit, not evidence the existing single-tenant system is insecure for its current, actual use.

---

## 5. Assumptions, Verified Facts, and Unresolved Questions

### Verified Facts (confirmed directly by reading source code)
- No `tenant_id` column exists anywhere in the current schema (confirmed by all six audits; explicitly sampled in migrations 0004, 0005, 0009 by the infra audit).
- JWT claims carry no tenant/org concept (`internal/auth/token.go:17-22`).
- `RequirePermission` checks only the JWT's embedded claim, never the database (`internal/platform/middleware/permission.go`, with an existing doc comment acknowledging the staleness tradeoff).
- The database layer is a single shared GORM connection pool (`internal/platform/database/database.go`, `SetMaxOpenConns(25)`) with no per-request session binding.
- `cmd/worker/main.go` never invokes HTTP auth middleware and never calls `dbctx.WithRequestID` or any tenant-equivalent context helper.
- Report export files are stored in one shared Docker volume with no tenant/user subdirectory (`internal/report/service.go`, `csvexport.go`).
- `cmd/migrate/main.go`'s `seed` step creates exactly one global admin user and no default warehouse.

### Assumptions (inferred, not 100% confirmed by code alone)
- We assume the seeded admin user, and any existing production users/warehouses/data, would all be assigned to a single initial tenant during backfill (this is a Phase-27 design proposal, not something the code states).
- We assume `username`/`email` uniqueness is intended to remain global (a single login identity usable across multiple tenants) rather than becoming tenant-scoped — this is a product decision the audits could not resolve from code alone, since the code simply has no tenant concept yet either way. (The per-module auditors, working in isolation without the full Step 2 design, suggested a `(tenant_id, username)` composite constraint on `users`; the Step 2 design in `tenant-er-design.md` takes the opposite position — see that document's rationale.)
- We assume the RBAC permission catalog (`internal/rbac/catalog.go`) is intended to stay platform-global and shared, with only role *assignments* becoming tenant-scoped — plausible given its static, in-code nature, but not something the code itself declares as a future intent.

### Unresolved Questions (require a product/business decision before Phase 28 implementation)
- Should tenant-scoped roles (ADMIN, WAREHOUSE_MANAGER, WAREHOUSE_OPERATOR) be fully independent per-tenant copies that a tenant admin can freely edit, or platform-managed templates where tenants can only toggle a constrained subset of permissions? This changes the `roles` table design materially (see `tenant-er-design.md` §"Unresolved Design Choices").
- Should SQL Server Row-Level Security be pursued at all, given the audit's finding that the connection pool has no per-request session-reset hook today? (See `tenant-isolation-security.md` for the full evaluation — this report only notes the structural fact, not the recommendation.)
- Should RabbitMQ queues be partitioned per tenant (for fairness/throttling under multi-tenant load) or remain global with tenant_id carried only in the message body? The current topology (`wms.system.health`, `wms.stock_adjustment.pending_reminder`, `wms.report.export`) offers no per-tenant isolation today.
- What is the correct tenant-scope for notification recipient resolution (`FindActiveIDsWithPermission`) — should it always resolve within the triggering event's own tenant, or are there legitimate platform-wide notifications (e.g. a platform-admin broadcast) that need a different, explicitly separate code path?
