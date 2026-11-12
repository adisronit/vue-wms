# Phase 27 — Backward-Compatible Multi-Tenant Migration Plan

**Status:** Proposal / draft for review. No migration has been executed. No `.sql` file has been added to `backend/migrations/`. No database (local, UAT, or production) has been touched while producing this document.

## 1. Overview

The existing schema (migrations `0001`–`0040`) has no tenant concept anywhere (confirmed by direct inspection — see `phase27-audit-report.md`, infra/background section). Every table identified as Tenant-Owned must gain a `tenant_id` column, and a new cluster of platform/tenant tables must be created, **without** breaking the single-tenant data that already exists.

The whole plan follows one rule: **expand, then contract, as separate migrations.**

1. **Expand** — add new tables, and add `tenant_id` to existing tables as **nullable**. Nothing existing breaks; no application code needs to change yet; the step is trivially reversible (drop the column/table).
2. **Backfill** — populate `tenant_id = 1` (the single default tenant created in step 1) on every existing row, and build `tenant_users` from the current `users`/`role` assignments. This is a data-only step, run and verified before anything depends on it.
3. **Verify** — orphan detection and quantity reconciliation (sections 4–5) run against the backfilled data, by hand, before proceeding.
4. **Contract** — only after verification passes, convert `tenant_id` to `NOT NULL`, add foreign keys, and rebuild unique constraints as tenant-scoped composites.

Each of the four phases is its own migration (or small group of migrations), so a problem discovered at step 3 never forces an irreversible step 4 to be undone — step 4 simply isn't run yet.

See `tenant-er-design.md` for the full target column-level schema; this document is about the **order and safety** of getting there, not the destination shape itself (though the draft SQL below necessarily restates the relevant column shapes).

## 2. Migration Sequence

| # | File | What it does | Risk | Reversibility |
|---|------|---------------|------|----------------|
| 0041 | `0041_create_tenants_and_platform_admins.up/down.sql` | Creates `tenants`, `platform_admins`. Inserts the single bootstrap tenant (id=1). | Additive | Trivial — drop both tables |
| 0042 | `0042_create_tenant_support_tables.up/down.sql` | Creates `subscription_plans`, `tenant_subscriptions`, `tenant_settings`, `tenant_features`, `tenant_audit_logs`, `tenant_users`. | Additive | Trivial — drop all six tables |
| 0043a | `0043a_add_tenant_id_nullable_master_inventory.up/down.sql` | Adds nullable `tenant_id` to `warehouses`, `zones`, `locations`, `products`, `inventory`, `inventory_transactions`, `roles`, `role_permissions`, `role_warehouse_access`. | Additive (nullable column) | Trivial — drop the column on each table |
| 0043b | `0043b_add_tenant_id_nullable_inbound_outbound.up/down.sql` | Adds nullable `tenant_id` to `receipts`, `receipt_items`, `receipt_confirmations`, `putaway_tasks`, `putaway_task_items`, `putaway_confirmations`, `picking_tasks`, `picking_task_items`, `picking_confirmations`, `shipments`, `shipment_items`, `shipment_confirmations`, `orders`, `order_items`. | Additive (nullable column) | Trivial — drop the column on each table |
| 0043c | `0043c_add_tenant_id_nullable_operations.up/down.sql` | Adds nullable `tenant_id` to `stock_adjustments`, `stock_adjustment_items`, `stock_adjustment_confirmations`, `stock_adjustment_reminders`, `stock_counts`, `stock_count_items`, `stock_count_confirmations`, `replenishment_tasks`, `replenishment_confirmations`, `notifications`, `export_jobs`. | Additive (nullable column) | Trivial — drop the column on each table |
| 0044 | `0044_backfill_tenant_id.up/down.sql` | `UPDATE ... SET tenant_id = 1 WHERE tenant_id IS NULL` on every table touched by 0043a–c; backfills `tenant_users` (one row per existing `users` row, using their current `role_id`). | **Data-only, no schema change** | `.down.sql` is a documented no-op (see §7 — backfilled values are not "unbackfilled") |
| 0045 | `0045_tenant_scope_unique_constraints.up/down.sql` | Drops old single-column unique constraints (`roles.code`, `warehouses.code`, `zones(warehouse_id,code)`, `locations(zone_id,code)`, `products.sku`, `products.barcode`, and any globally-unique document numbers such as `receipt_no`/`putaway_no`/`order_no` if those currently carry their own uniqueness) and recreates them as composite `(tenant_id, ...)` constraints. | Constraint swap (not a table rebuild) | Reversible — drop the composite constraint, recreate the original single-column one (safe only because, at this point, every row still has `tenant_id = 1`) |
| 0046 | `0046_tenant_id_not_null_and_fk.up/down.sql` | Converts every `tenant_id` column added in 0043a–c to `NOT NULL`, adds `FOREIGN KEY (tenant_id) REFERENCES tenants(id)`, adds a supporting index on `tenant_id` (or a composite leading index where the table is already indexed on another column). | Constraint tightening | Reversible — drop FK/index, `ALTER COLUMN ... NULL` (data itself is untouched, so down-then-up is always safe) |

Migrations 0043a–c are split by module group purely for reviewability; they could be merged into one file. 0044's backfill must run strictly after **all** of 0041–0043 are applied (it needs `tenants.id = 1` to exist and every nullable column to exist), and 0045–0046 must run strictly after 0044 is verified (§4–5) — this ordering is enforced naturally by the existing migration tool's sequential version numbering.

## 3. Draft SQL — NOT EXECUTED, FOR REVIEW ONLY

All statements below are T-SQL drafts consistent with the existing GORM + SQL Server schema conventions (`BIGINT IDENTITY` surrogate keys, `NVARCHAR`, `GETUTCDATE()` defaults, named constraints). None of this has been run.

### 3a. `0041_create_tenants_and_platform_admins.up.sql`

```sql
CREATE TABLE tenants (
    id              BIGINT IDENTITY(1,1) PRIMARY KEY,
    slug            NVARCHAR(63)  NOT NULL,
    name            NVARCHAR(255) NOT NULL,
    status          NVARCHAR(20)  NOT NULL CONSTRAINT DF_tenants_status DEFAULT 'TRIAL',
    created_at      DATETIME2     NOT NULL CONSTRAINT DF_tenants_created_at DEFAULT GETUTCDATE(),
    updated_at      DATETIME2     NOT NULL CONSTRAINT DF_tenants_updated_at DEFAULT GETUTCDATE(),
    suspended_at    DATETIME2     NULL,
    cancelled_at    DATETIME2     NULL,
    CONSTRAINT CK_tenants_status CHECK (status IN ('TRIAL','ACTIVE','SUSPENDED','CANCELLED')),
    CONSTRAINT UQ_tenants_slug UNIQUE (slug)
);

-- Bootstrap tenant for 100% of pre-existing data.
INSERT INTO tenants (id, slug, name, status)
VALUES (1, 'default', 'Default Organization', 'ACTIVE');

CREATE TABLE platform_admins (
    id          BIGINT IDENTITY(1,1) PRIMARY KEY,
    user_id     BIGINT      NOT NULL,
    granted_by  BIGINT      NULL,
    granted_at  DATETIME2   NOT NULL CONSTRAINT DF_platform_admins_granted_at DEFAULT GETUTCDATE(),
    revoked_at  DATETIME2   NULL,
    CONSTRAINT FK_platform_admins_user FOREIGN KEY (user_id) REFERENCES users(id),
    CONSTRAINT UQ_platform_admins_user UNIQUE (user_id)
);
```

### 3a-down. `0041_create_tenants_and_platform_admins.down.sql`

```sql
DROP TABLE platform_admins;
DROP TABLE tenants;
```

### 3b. `0042_create_tenant_support_tables.up.sql` (excerpt — `tenant_users` only; see `tenant-er-design.md` for the other five tables' full specs)

```sql
CREATE TABLE tenant_users (
    id           BIGINT IDENTITY(1,1) PRIMARY KEY,
    tenant_id    BIGINT      NOT NULL,
    user_id      BIGINT      NOT NULL,
    role_id      BIGINT      NOT NULL,
    status       NVARCHAR(20) NOT NULL CONSTRAINT DF_tenant_users_status DEFAULT 'ACTIVE',
    invited_by   BIGINT      NULL,
    invited_at   DATETIME2   NULL,
    joined_at    DATETIME2   NOT NULL CONSTRAINT DF_tenant_users_joined_at DEFAULT GETUTCDATE(),
    CONSTRAINT FK_tenant_users_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id),
    CONSTRAINT FK_tenant_users_user   FOREIGN KEY (user_id)   REFERENCES users(id),
    CONSTRAINT FK_tenant_users_role   FOREIGN KEY (role_id)   REFERENCES roles(id),
    CONSTRAINT UQ_tenant_users_tenant_user UNIQUE (tenant_id, user_id),
    CONSTRAINT CK_tenant_users_status CHECK (status IN ('INVITED','ACTIVE','DISABLED'))
);
```

### 3c. `0043a_add_tenant_id_nullable_master_inventory.up.sql` (excerpt — `warehouses` only; the real file repeats this pattern for every table in the 0043a/b/c groups)

```sql
ALTER TABLE warehouses ADD tenant_id BIGINT NULL;
```

### 3c-down.

```sql
ALTER TABLE warehouses DROP COLUMN tenant_id;
```

### 3d. `0044_backfill_tenant_id.up.sql` (excerpt — `warehouses` + `tenant_users` backfill; the real file repeats the `UPDATE` for every table touched by 0043a/b/c)

```sql
UPDATE warehouses SET tenant_id = 1 WHERE tenant_id IS NULL;

-- ... one equivalent UPDATE per table added in 0043a/0043b/0043c ...

INSERT INTO tenant_users (tenant_id, user_id, role_id, status, joined_at)
SELECT 1, u.id, u.role_id, 'ACTIVE', GETUTCDATE()
FROM users u
WHERE NOT EXISTS (
    SELECT 1 FROM tenant_users tu WHERE tu.tenant_id = 1 AND tu.user_id = u.id
);
```

### 3e. `0045_tenant_scope_unique_constraints.up.sql` (excerpt — `warehouses.code` only; the real file repeats this for `roles.code`, `zones(warehouse_id,code)`, `locations(zone_id,code)`, `products.sku`, `products.barcode`, and any globally-unique document numbers)

```sql
ALTER TABLE warehouses DROP CONSTRAINT UQ_warehouses_code;
ALTER TABLE warehouses ADD CONSTRAINT UQ_warehouses_tenant_code UNIQUE (tenant_id, code);
```

### 3e-down.

```sql
ALTER TABLE warehouses DROP CONSTRAINT UQ_warehouses_tenant_code;
ALTER TABLE warehouses ADD CONSTRAINT UQ_warehouses_code UNIQUE (code);
```

*(Safe only immediately after 0044's backfill and before any second tenant is ever created — at that point every row still has `tenant_id = 1`, so the composite constraint is equivalent to the original single-column one. This is why rollback ordering in §7 matters.)*

### 3f. `0046_tenant_id_not_null_and_fk.up.sql` (excerpt — `warehouses` only)

```sql
ALTER TABLE warehouses ALTER COLUMN tenant_id BIGINT NOT NULL;

ALTER TABLE warehouses WITH CHECK ADD CONSTRAINT FK_warehouses_tenant
    FOREIGN KEY (tenant_id) REFERENCES tenants(id);

CREATE INDEX IX_warehouses_tenant_id ON warehouses (tenant_id);
```

### 3f-down.

```sql
DROP INDEX IX_warehouses_tenant_id ON warehouses;
ALTER TABLE warehouses DROP CONSTRAINT FK_warehouses_tenant;
ALTER TABLE warehouses ALTER COLUMN tenant_id BIGINT NULL;
```

## 4. Orphan and Inconsistency Detection

Run **before** 0046 (the `NOT NULL` + FK step), against the backfilled-but-still-nullable state:

**(a) Tenant-Owned row whose parent failed to receive a `tenant_id`** (defensive check — should return zero rows given the single-tenant backfill, but must be verified, not assumed):

```sql
SELECT z.id, z.warehouse_id
FROM zones z
JOIN warehouses w ON w.id = z.warehouse_id
WHERE z.tenant_id IS NULL OR w.tenant_id IS NULL OR z.tenant_id <> w.tenant_id;
```

**(b) Pre-existing orphaned foreign key** (a data-integrity problem that predates this migration — must be manually reviewed, never silently forced onto tenant 1):

```sql
SELECT l.id, l.zone_id
FROM locations l
LEFT JOIN zones z ON z.id = l.zone_id
WHERE z.id IS NULL;
```

Any row returned by (b) indicates existing data corruption unrelated to multi-tenancy and must be resolved (or explicitly accepted and documented) before 0046 runs — it is **not** safe to default such rows to `tenant_id = 1` without understanding why the parent reference is already broken.

## 5. Inventory Quantity Reconciliation

Run once immediately **before** 0043–0046 and once immediately **after** 0046, against the same warehouse set, and diff the two result sets byte-for-byte:

```sql
SELECT warehouse_id,
       SUM(quantity)          AS total_quantity,
       SUM(reserved_quantity) AS total_reserved
FROM inventory
GROUP BY warehouse_id
ORDER BY warehouse_id;
```

The migration never writes to `quantity` or `reserved_quantity` — it only adds and backfills `tenant_id`. If this query's output differs before vs. after, the migration must be treated as failed and rolled back regardless of what caused the difference.

## 6. Backup and Restore Testing Requirement

This is **required**, not optional, per the task's own constraints (no destructive migrations, no production database changes):

1. Take a full backup of the target database before running any migration in this plan against it.
2. Restore that backup into an isolated, non-production environment.
3. Run §5's reconciliation query against the restored copy to establish the "before" baseline.
4. Apply migrations 0041–0046 against the restored copy only.
5. Re-run §4's orphan checks and §5's reconciliation query against the restored copy; both must pass cleanly.
6. Only after step 5 passes in the isolated environment does this plan authorize applying the same migrations to the real target (UAT, then production) — and only with explicit, separate sign-off at that point, since this document is a design proposal, not an execution order.

## 7. Rollback and Reconciliation Procedures

- **Rolling back 0041–0043 (new tables + nullable columns, before any backfill):** non-destructive. Each `.down.sql` simply drops the table(s) or column(s) it added. No data is lost because nothing but new, empty structures existed.
- **Rolling back 0044 (backfill):** the `tenant_id = 1` values written by the backfill are not meaningfully "unbackfillable" — setting them back to `NULL` is the only sensible down action, and is safe as long as 0045/0046 haven't run yet (nothing depends on `tenant_id` being populated). `0044_backfill_tenant_id.down.sql` is therefore a documented no-op/no-restore step: it is intentionally the same as not having a down migration that reverses data, only a comment noting that rolling back past this point means re-running 0043's down migrations, which drop the column entirely (the `NULL`-ing is implicit).
- **Rolling back 0045 (constraint swap):** must run *before* rolling back 0046, never after — reverse order of application. If 0046 has already made `tenant_id NOT NULL` with an FK, that FK and the `NOT NULL` constraint must be removed first (0046's down), *then* 0045's down can safely drop the composite unique constraint and restore the original single-column one. Attempting 0045's down while 0046 is still applied risks the database rejecting the constraint rebuild if any later-created second-tenant data would violate the original single-column uniqueness.
- **Rolling back 0046 (NOT NULL + FK):** safe at any time — data is untouched, only nullability/FK/index metadata changes. Must run before 0045's rollback, per the above ordering.

## 8. Explicit Non-Destructive Guarantees

- No existing table is dropped or rebuilt automatically at any step in this plan.
- Every step is either purely additive (new table, new nullable column) or a constraint swap (drop-and-recreate a constraint, never a table).
- Existing primary keys and all existing relationships are preserved unchanged.
- No existing row's business data is modified, with the single exception of the `tenant_id` column itself being set by the 0044 backfill — `quantity`, `reserved_quantity`, and every other existing column are untouched, and this is independently verified by §5.
- Nothing in this plan is executed as part of producing this document; it is a reviewed draft only.

## 9. Assumptions, Verified Facts, and Unresolved Design Choices

**Verified Facts**
- Migrations currently run `0001`–`0040`; sampled files (`0004_create_users_table`, `0005_create_warehouses_table`, `0009_create_inventory_table`) confirmed clean of any `org_id`/`company_id`/`account_id`/`tenant_id` concept (per the infra/background Step-1 audit).
- The migration tool applies versioned `up`/`down` SQL pairs tracked in a `schema_migrations` table and its `seed` step creates exactly one global admin user with no default warehouse — this is the row that must land in `tenant_users` for tenant 1 during 0044.
- `warehouses.code`, `zones(warehouse_id, code)`, `locations(zone_id, code)`, and `products.sku`/`products.barcode` are confirmed to carry their stated unique constraints today (per the master-data Step-1 audit), which is why 0045 targets exactly those five.

**Assumptions**
- All existing data belongs to exactly one initial tenant (`tenants.id = 1`) with no requirement to retroactively split existing rows across multiple tenants. If any existing data actually needs to be partitioned among more than one tenant from day one, this plan does not cover that — it would require a manual, row-by-row reassignment step inserted between 0044 and 0045.
- The single seeded admin user from the existing `seed` step should become tenant 1's first `tenant_users` row with their existing role, not a platform admin — platform admin status (`platform_admins`) is assumed to be granted separately and deliberately, never automatically during this migration.

**Unresolved Design Choices**
- Whether any currently-globally-unique human-readable document numbers (e.g. `receipt_no`, `putaway_no`, `order_no`, if they carry their own uniqueness constraint outside the primary key) should become tenant-scoped sequences. Recommendation: scope uniqueness going forward only (`UNIQUE(tenant_id, receipt_no)`), and never renumber existing business documents — but this needs explicit confirmation since it affects how new document numbers are generated per tenant.
- Whether 0043a/b/c should instead be one single migration file — split here purely for reviewability; either is mechanically equivalent.
- Whether the orphan-detection queries in §4 should become a permanent, repeatable pre-flight script (e.g. checked into `backend/migrations/` as a non-versioned helper) rather than a one-time manual run — not decided here.
