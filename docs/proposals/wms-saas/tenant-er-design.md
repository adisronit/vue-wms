# Phase 27 — Tenant ER Design

Status: **Proposal / reviewed draft only.** No schema change in this document has been executed against any database. See `migration-plan.md` for rollout mechanics and `tenant-isolation-security.md` for how this schema is enforced at query time.

## 1. Overview / Goals

The Phase 27 brief asks for eight supporting tables — `tenants`, `users`, `tenant_users`, `tenant_settings`, `subscription_plans`, `tenant_subscriptions`, `tenant_features`, `tenant_audit_logs` — to turn the existing single-tenant WMS into a multi-tenant SaaS product while preserving every existing business table and record. This design treats `users` as an existing table being **reclassified**, not recreated (see §3), and adds a **ninth** table, `platform_admins`, beyond the brief's list: the Step-1 audit's requirement for a "Platform Super Admin" persona needs an authorization path that is structurally separate from any tenant's RBAC, and a role row — even a specially-flagged one — living inside `roles`/`tenant_users` cannot cleanly guarantee that separation (a bug in tenant-role resolution would otherwise also be a platform-privilege bug). `platform_admins` is therefore a deliberate, minimal, separate grant table.

The central design choice this document makes: **identity is platform-global, membership and role are per-tenant.** A `users` row is a login; a `tenant_users` row is what grants that login access to one specific tenant's data, with one specific role *inside that tenant*. This is what makes "one tenant, many users" and "one user, many tenants" both true simultaneously without contradiction.

## 2. New Platform/Tenant Tables

All types are SQL Server (T-SQL) syntax, consistent with the existing migration style (plain numbered `.up.sql`/`.down.sql` pairs, `BIGINT IDENTITY(1,1)` primary keys, `SYSUTCDATETIME()` for timestamps — matching the existing `users`/`warehouses` convention found in the Step-1 audit).

### `tenants`

| Column | Type | Null | Default | Notes |
|---|---|---|---|---|
| `id` | `BIGINT IDENTITY(1,1)` | NO | — | PK |
| `slug` | `VARCHAR(63)` | NO | — | URL/subdomain-safe identifier. `UNIQUE` |
| `name` | `NVARCHAR(255)` | NO | — | Display/company name |
| `status` | `VARCHAR(20)` | NO | `'TRIAL'` | `CHECK (status IN ('TRIAL','ACTIVE','SUSPENDED','CANCELLED'))` |
| `created_at` | `DATETIME2` | NO | `SYSUTCDATETIME()` | |
| `updated_at` | `DATETIME2` | NO | `SYSUTCDATETIME()` | |
| `suspended_at` | `DATETIME2` | YES | `NULL` | Set when status → SUSPENDED |
| `cancelled_at` | `DATETIME2` | YES | `NULL` | Set when status → CANCELLED |

This is the root of every tenant-owned FK chain in the system.

### `tenant_users`

| Column | Type | Null | Default | Notes |
|---|---|---|---|---|
| `id` | `BIGINT IDENTITY(1,1)` | NO | — | PK |
| `tenant_id` | `BIGINT` | NO | — | FK → `tenants.id` |
| `user_id` | `BIGINT` | NO | — | FK → `users.id` |
| `role_id` | `BIGINT` | NO | — | FK → `roles.id`; **must** resolve to a role whose own `tenant_id` equals this row's `tenant_id` (app-layer enforced, see §5) |
| `status` | `VARCHAR(20)` | NO | `'INVITED'` | `CHECK (status IN ('INVITED','ACTIVE','DISABLED'))` |
| `invited_by` | `BIGINT` | YES | `NULL` | FK → `users.id` |
| `invited_at` | `DATETIME2` | YES | `NULL` | |
| `joined_at` | `DATETIME2` | YES | `NULL` | Set when status → ACTIVE |
| `created_at` | `DATETIME2` | NO | `SYSUTCDATETIME()` | |
| `updated_at` | `DATETIME2` | NO | `SYSUTCDATETIME()` | |

`UNIQUE (tenant_id, user_id)` — one membership row per (tenant, user) pair; a user with N tenants has N rows, each with its own role.

### `tenant_settings`

One row per tenant (1:1), structured columns for everything currently known to matter, plus one JSON escape hatch rather than full EAV (key/value rows). EAV would let settings proliferate untyped and unvalidated, and every read would need an extra join/pivot; a fixed column set keeps the common path type-safe and queryable with a plain `SELECT`, while `custom_json` absorbs anything genuinely tenant-specific and rare (e.g. a one-off UI preference) without a migration. This is a reviewed proposal, not yet implemented — if usage shows the fixed columns are too restrictive, that's a future migration, not a reason to default to EAV now.

| Column | Type | Null | Default | Notes |
|---|---|---|---|---|
| `tenant_id` | `BIGINT` | NO | — | PK, FK → `tenants.id` (1:1) |
| `timezone` | `VARCHAR(64)` | NO | `'UTC'` | IANA tz name |
| `locale` | `VARCHAR(10)` | NO | `'en-US'` | |
| `currency` | `CHAR(3)` | NO | `'USD'` | ISO 4217 |
| `default_warehouse_id` | `BIGINT` | YES | `NULL` | FK → `warehouses.id`; must belong to same tenant (app-layer check) |
| `company_legal_name` | `NVARCHAR(255)` | YES | `NULL` | |
| `logo_url` | `NVARCHAR(500)` | YES | `NULL` | |
| `custom_json` | `NVARCHAR(MAX)` | YES | `NULL` | JSON escape hatch for anything not yet promoted to a real column |
| `updated_at` | `DATETIME2` | NO | `SYSUTCDATETIME()` | |

### `subscription_plans` (platform-global catalog)

| Column | Type | Null | Default | Notes |
|---|---|---|---|---|
| `id` | `BIGINT IDENTITY(1,1)` | NO | — | PK |
| `code` | `VARCHAR(50)` | NO | — | `UNIQUE`, e.g. `TRIAL`, `MONTHLY_STANDARD`, `ANNUAL_PRO` |
| `name` | `NVARCHAR(100)` | NO | — | |
| `billing_cycle` | `VARCHAR(20)` | NO | — | `CHECK (billing_cycle IN ('TRIAL','MONTHLY','ANNUAL'))` |
| `price_cents` | `INT` | NO | `0` | Smallest-currency-unit integer, never float |
| `currency` | `CHAR(3)` | NO | `'USD'` | |
| `max_warehouses` | `INT` | YES | `NULL` | `NULL` = unlimited |
| `max_users` | `INT` | YES | `NULL` | |
| `max_active_skus` | `INT` | YES | `NULL` | |
| `feature_flags` | `NVARCHAR(MAX)` | NO | `'{}'` | JSON map of feature code → bool/limit, plan defaults |
| `is_public` | `BIT` | NO | `1` | `0` for internal/negotiated-only plans |
| `created_at` | `DATETIME2` | NO | `SYSUTCDATETIME()` | |

Not tenant-owned — this is the sellable product catalog, identical for every prospective tenant.

### `tenant_subscriptions`

| Column | Type | Null | Default | Notes |
|---|---|---|---|---|
| `id` | `BIGINT IDENTITY(1,1)` | NO | — | PK |
| `tenant_id` | `BIGINT` | NO | — | FK → `tenants.id` |
| `plan_id` | `BIGINT` | NO | — | FK → `subscription_plans.id` |
| `status` | `VARCHAR(20)` | NO | — | `CHECK (status IN ('TRIALING','ACTIVE','PAST_DUE','CANCELLED','EXPIRED'))` |
| `trial_ends_at` | `DATETIME2` | YES | `NULL` | |
| `current_period_start` | `DATETIME2` | NO | — | |
| `current_period_end` | `DATETIME2` | NO | — | |
| `cancel_at_period_end` | `BIT` | NO | `0` | |
| `cancelled_at` | `DATETIME2` | YES | `NULL` | |
| `created_at` | `DATETIME2` | NO | `SYSUTCDATETIME()` | |
| `updated_at` | `DATETIME2` | NO | `SYSUTCDATETIME()` | |

`UNIQUE (tenant_id)` for v1 — see §6, this assumes current-state-only, no subscription history row-per-period.

### `tenant_features`

| Column | Type | Null | Default | Notes |
|---|---|---|---|---|
| `id` | `BIGINT IDENTITY(1,1)` | NO | — | PK |
| `tenant_id` | `BIGINT` | NO | — | FK → `tenants.id` |
| `feature_code` | `VARCHAR(100)` | NO | — | Matches a key in some plan's `feature_flags` JSON, or a net-new flag |
| `enabled` | `BIT` | NO | `1` | |
| `limit_value` | `INT` | YES | `NULL` | Overrides the plan default limit for this one tenant |
| `granted_by` | `BIGINT` | YES | `NULL` | FK → `users.id` (expected to be a platform admin) |
| `granted_at` | `DATETIME2` | NO | `SYSUTCDATETIME()` | |
| `expires_at` | `DATETIME2` | YES | `NULL` | |

`UNIQUE (tenant_id, feature_code)`. This is the override layer for sales-negotiated exceptions or staged feature rollouts — plan defaults apply when no row exists here.

### `tenant_audit_logs`

| Column | Type | Null | Default | Notes |
|---|---|---|---|---|
| `id` | `BIGINT IDENTITY(1,1)` | NO | — | PK |
| `tenant_id` | `BIGINT` | YES | `NULL` | FK → `tenants.id`; `NULL` for actions not tied to one tenant (e.g. tenant creation itself) |
| `actor_user_id` | `BIGINT` | NO | — | FK → `users.id` |
| `actor_type` | `VARCHAR(20)` | NO | — | `CHECK (actor_type IN ('PLATFORM_ADMIN','TENANT_ADMIN','SYSTEM'))` |
| `action` | `VARCHAR(100)` | NO | — | e.g. `tenant.suspend`, `tenant_user.invite`, `subscription.change_plan` |
| `target_type` | `VARCHAR(50)` | YES | `NULL` | e.g. `tenant`, `user`, `warehouse` |
| `target_id` | `BIGINT` | YES | `NULL` | |
| `metadata` | `NVARCHAR(MAX)` | YES | `NULL` | JSON, free-form context (old/new values, reason) |
| `ip_address` | `VARCHAR(45)` | YES | `NULL` | |
| `created_at` | `DATETIME2` | NO | `SYSUTCDATETIME()` | |

Append-only — no `updated_at`, no `UPDATE`/`DELETE` path should exist in application code for this table.

### `platform_admins`

| Column | Type | Null | Default | Notes |
|---|---|---|---|---|
| `id` | `BIGINT IDENTITY(1,1)` | NO | — | PK |
| `user_id` | `BIGINT` | NO | — | FK → `users.id`, `UNIQUE` |
| `granted_by` | `BIGINT` | YES | `NULL` | FK → `users.id` |
| `granted_at` | `DATETIME2` | NO | `SYSUTCDATETIME()` | |
| `revoked_at` | `DATETIME2` | YES | `NULL` | Presence of a non-null value = not currently a platform admin, row kept for audit history rather than deleted |

Deliberately tiny and separate. This table's existence is what `RequirePlatformAdmin` middleware checks (see `tenant-isolation-security.md`) — it is never consulted by any tenant-scoped code path, and no tenant-scoped code path can write to it.

## 3. Existing Table Reclassification

| Table | Classification | Tenant-ID Strategy | Old Unique Constraint | New Unique Constraint |
|---|---|---|---|---|
| `users` | Platform-Global | N/A | `UNIQUE(username)`, `UNIQUE(email)` | **Unchanged** — identity stays globally unique |
| `permissions` | Platform-Global | N/A | `UNIQUE(code)` | Unchanged |
| `subscription_plans` | Platform-Global | N/A | — | `UNIQUE(code)` (new table) |
| `roles` | Tenant-Owned | Direct column | `UNIQUE(code)` | `UNIQUE(tenant_id, code)` |
| `role_permissions` | Tenant-Owned (via role) | Inherited, no own column needed | `UNIQUE(role_id, permission_id)` | Unchanged (role_id already tenant-scoped once `roles.tenant_id` exists) |
| `role_warehouse_access` | Tenant-Owned (via role) | Inherited | `UNIQUE(role_id, warehouse_id)` | Unchanged, but app-layer must verify warehouse_id's tenant == role's tenant (§5) |
| `warehouses` | Tenant-Owned | Direct column | `UNIQUE(code)` | `UNIQUE(tenant_id, code)` |
| `zones` | Tenant-Owned | **Denormalized** direct column (not inherited via `warehouse_id` join) | `UNIQUE(warehouse_id, code)` | `UNIQUE(tenant_id, warehouse_id, code)` |
| `locations` | Tenant-Owned | **Denormalized** direct column | `UNIQUE(zone_id, code)` | `UNIQUE(tenant_id, zone_id, code)` |
| `products` | Tenant-Owned | Direct column | `UNIQUE(sku)`, filtered `UNIQUE(barcode) WHERE barcode IS NOT NULL` | `UNIQUE(tenant_id, sku)`, filtered `UNIQUE(tenant_id, barcode) WHERE barcode IS NOT NULL` |
| `inventory` | Tenant-Owned | Direct column | — (no cross-tenant uniqueness today) | Composite index `(tenant_id, product_id, location_id, lot_no)` |
| `inventory_transactions` | Tenant-Owned | Direct column | — | Index `(tenant_id, ...)` for ledger queries |
| `receipts`, `receipt_items`, `receipt_confirmations` | Tenant-Owned | Direct column | Global `receipt_no` sequence implied unique | `UNIQUE(tenant_id, receipt_no)` |
| `putaway_tasks`, `putaway_task_items`, `putaway_confirmations` | Tenant-Owned | Direct column | Global task number | `UNIQUE(tenant_id, putaway_no)` |
| `picking_tasks`, `picking_task_items`, `picking_confirmations` | Tenant-Owned | Direct column | Global task number | `UNIQUE(tenant_id, picking_no)` |
| `shipments`, `shipment_items`, `shipment_confirmations` | Tenant-Owned | Direct column | Global shipment number | `UNIQUE(tenant_id, shipment_no)` |
| `orders`, `order_items` | Tenant-Owned | Direct column | `order_no` / external reference presumably global | `UNIQUE(tenant_id, order_no)`, `UNIQUE(tenant_id, external_reference)` |
| `stock_adjustments`, `stock_adjustment_items`, `stock_adjustment_confirmations`, `stock_adjustment_reminders` | Tenant-Owned | Direct column | — | Index `(tenant_id, status)` for the overdue-reminder scan |
| `stock_counts`, `stock_count_items`, `stock_count_confirmations` | Tenant-Owned | Direct column | — | Index `(tenant_id, status)` |
| `replenishment_tasks`, `replenishment_confirmations` | Tenant-Owned | Direct column | — | Index `(tenant_id, warehouse_id)` |
| `notifications` | Tenant-Owned (defense-in-depth; `user_id` remains the primary scoping control per the operations audit) | Direct column | — | Index `(tenant_id, user_id)` |
| `export_jobs` | Tenant-Owned | Direct column | — | Index `(tenant_id, requested_by)`; file path also partitions by tenant (see `tenant-isolation-security.md`) |

Every "Tenant-Owned" row above starts as `tenant_id BIGINT NULL` and is tightened to `NOT NULL` only after backfill — see `migration-plan.md`.

## 4. Supporting Multi-Tenant Membership and Roles

The four personas in the brief map onto this schema as follows:

- **Platform Super Admin** — a row in `platform_admins`, full stop. Not a `role`, not a `tenant_users` membership. A platform admin may have zero tenant memberships and still administer the platform, or may also happen to be a normal tenant user in some tenant via a separate, ordinary `tenant_users` row — the two are unrelated grants that happen to share a `user_id`.
- **Tenant Admin** — a `tenant_users` row whose `role_id` points at that tenant's own copy of the `ADMIN` role (seeded from `internal/rbac/catalog.go`'s existing `RoleDefaultGrants` at tenant-provisioning time — the Go-code catalog itself stays the single source of truth for *what permissions exist*; provisioning copies the catalog's default grants into that one tenant's brand-new `roles`/`role_permissions` rows).
- **Warehouse Manager** / **Warehouse Operator** — same pattern: each tenant gets its own `roles` rows for these codes at provisioning, customizable afterward without affecting any other tenant, because `roles.tenant_id` makes every tenant's `WAREHOUSE_MANAGER` a genuinely distinct row.

A user with access to three tenants has one `users` row and three `tenant_users` rows, each carrying an independent `role_id` — they could be `ADMIN` in Tenant A and `WAREHOUSE_OPERATOR` in Tenant B. Session-time "which tenant am I acting as" is a JWT claim (`active_tenant_id`), not a database concept — see `tenant-isolation-security.md` for how that claim is issued, verified, and switched.

## 5. Foreign Keys and Referential Integrity

The schema's hardest integrity problem: SQL Server foreign keys enforce "this `warehouse_id` exists," not "this `warehouse_id` belongs to the same tenant as the row referencing it." Two alternatives were evaluated:

**Option A — Trigger-enforced tenant consistency.** An `AFTER INSERT, UPDATE` trigger on, say, `zones` that re-fetches the referenced `warehouses.tenant_id` and rejects the write if it doesn't match `zones.tenant_id`. This gives a database-level backstop that *cannot* be bypassed by an application bug, at the cost of a trigger per tenant-owned table with a tenant-owned parent (a non-trivial maintenance surface — roughly 20+ triggers across the tables in §3), plus the usual trigger downsides (harder to reason about in bulk operations, additional round-trip per write, another thing migrations must keep in sync as the schema evolves).

**Option B — Application-layer enforcement only.** Every service-layer "does this parent belong to my tenant" check (the exact `locationChecker`/`productChecker`/`warehouseChecker`/`zoneChecker` interfaces the Step-1 audit found already exist, currently doing existence-only checks) is upgraded to take and verify `tenant_id`, backed by the Step 6 security test suite asserting the behavior end-to-end. Cheaper, consistent with "GORM scopes/app logic are not the *only* control, but can be the *primary* one" as long as tests genuinely cover it — but a missed call site is a silent cross-tenant write, not a hard database error.

**Recommendation:** Option B as the primary, mandatory control — it is what the isolation design in `tenant-isolation-security.md` already requires at every repository boundary — with Option A left as an **optional later hardening pass** on the highest-value tables (`inventory`, `orders`, `picking_tasks`) once the application-layer path is proven in production, rather than built up front. Building ~20 triggers before the application-layer checks even exist would be solving the easier 20% of the problem first.

## 6. Assumptions, Verified Facts, and Unresolved Design Choices

**Verified Facts** (confirmed by the Step-1 audits already in this conversation):
- No existing table has a `tenant_id`, `org_id`, `company_id`, or `account_id` column (confirmed by sampling early migrations and by all six audit passes).
- `users.username`/`users.email` are today globally unique, and `roles.code`, `warehouses.code`, `zones(warehouse_id, code)`, `locations(zone_id, code)`, `products.sku`/`barcode` are the specific existing unique constraints that must change.
- The existing RBAC permission catalog (`internal/rbac/catalog.go`) is static Go code, not a database table — it does not need a `tenant_id` itself.

**Assumptions** (reasonable but not yet confirmed against a product decision):
- That every tenant should start with its own full copy of all three non-super-admin role templates (`ADMIN`, `WAREHOUSE_MANAGER`, `WAREHOUSE_OPERATOR`) at provisioning, rather than some subset.
- That `export_jobs`, despite being ephemeral/operational, is worth a full `tenant_id` column rather than relying solely on the existing `requested_by` user check — taken as a given here per the Step-1 infra audit's explicit recommendation.
- That notifications, while already scoped correctly by `user_id`, still warrant a `tenant_id` column for query performance and defense-in-depth rather than leaving `user_id` as the sole control.

**Unresolved Design Choices** (need a product/business decision before Phase 28 implementation):
- Should `tenant_subscriptions` retain full billing-period history (one row per period) or stay current-state-only (the `UNIQUE(tenant_id)` design above)? The current design picks the simpler option for v1 but has no audit trail of past plan changes beyond what `tenant_audit_logs` separately captures.
- Should a tenant admin ever be allowed to "reset" a customized role back to the platform's current default grants, and if so, does that require versioning the platform catalog itself (currently just a Go literal with no version number)?
- Is the Option A/Option B trade-off in §5 acceptable as application-layer-only for the full set of tenant-owned tables, or does the business require the stronger trigger-based guarantee on specific high-value tables (e.g. `inventory`) before launch, not after?
- Whether `tenant_settings.default_warehouse_id` should be enforced NOT NULL once a tenant has at least one warehouse, or remain optional indefinitely — left optional here pending a UX decision.
