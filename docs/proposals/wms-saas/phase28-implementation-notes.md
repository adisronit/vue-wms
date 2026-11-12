# Phase 28 — Multi-Tenant Database Foundation: Implementation Notes

**Status:** Implemented and tested against a disposable SQL Server instance. Not applied to any real/shared environment. Branch `feat/wms-multitenant`, building on the Phase 27 audit commit `d95ef22`.

This document records what Phase 28 actually built, the inconsistencies found while verifying Phase 27's proposal against itself and the real codebase, and what remains for Phase 29.

## 1. Step 1 — Phase 27 Verification Findings

Before writing any code, all six `docs/proposals/wms-saas/` documents were re-read against each other and against the live backend. Three concrete discrepancies were found and resolved:

- **Table count.** `multi-tenant-architecture.md` says "8 requested + `platform_admins` = 9." The Phase 27 brief's list of eight includes `users`, which is *reclassified*, not created. The actual count of **new** tables is **8**: `tenants`, `tenant_users`, `tenant_settings`, `subscription_plans`, `tenant_subscriptions`, `tenant_features`, `tenant_audit_logs`, `platform_admins`. This document and the migration/model code below use 8 consistently.
- **`tenant_users.status` default.** `tenant-er-design.md` specifies `DEFAULT 'INVITED'`; `migration-plan.md`'s draft SQL used `DEFAULT 'ACTIVE'`. Resolved in favor of `'INVITED'` — the ER design's semantics (a new membership has not been accepted yet) is the general-purpose table default; `migration-plan.md`'s `'ACTIVE'` was specific to its own example, the Phase 29 backfill `INSERT` for *already-active* existing users, which sets the value explicitly at insert time regardless of the column default.
- **`tenant_users.joined_at`.** Same root cause: `tenant-er-design.md` describes it as nullable, set only when status becomes ACTIVE; `migration-plan.md`'s draft had it `NOT NULL DEFAULT GETUTCDATE()`. Resolved as nullable, no default, per the ER design — verified by `TestTenantUser_StatusDefaultsToInvited`.

Minor, non-blocking style inconsistencies (`NVARCHAR` vs `VARCHAR` for status/code columns, `slug` typed `NVARCHAR` in one draft) were resolved by following this schema's own existing convention — every status/code-shaped column in migrations 0001–0040 uses `VARCHAR`, `NVARCHAR` is reserved for human-entered display text — confirmed by inspecting `migrations/0011_create_receipts_table.up.sql` and `0038_create_orders_tables.up.sql` directly rather than assuming.

**Correction to a Phase 27 claim:** `tenant-er-design.md` §4 describes four personas resolving to roles "ADMIN, WAREHOUSE_MANAGER, WAREHOUSE_OPERATOR." Seeding the live RBAC catalog (`internal/rbac/catalog.go`, via `go run ./cmd/migrate seed`) against the disposable test database produced **seven** role codes: `ADMIN`, `WAREHOUSE_MANAGER`, `RECEIVING`, `QC`, `PUTAWAY`, `PICKING`, `SHIPPING` — there is no `WAREHOUSE_OPERATOR` code today, and the catalog is more fine-grained than the brief assumed. This does not change Phase 28's schema (`tenant_users.role_id` is a plain FK regardless of how many role codes exist), but Phase 29's per-tenant role-copy provisioning logic must seed from the *actual* seven-role catalog, not the three assumed in Phase 27.

**Migration ordering/dependencies confirmed:** existing migrations run `0001`–`0040` (verified via `ls backend/migrations`), so this phase's new files are `0041` and `0042`. `0041` (`tenants`, `platform_admins`) has no dependency beyond the existing `users` table; `0042` (`subscription_plans`, `tenant_subscriptions`, `tenant_settings`, `tenant_features`, `tenant_audit_logs`, `tenant_users`) depends on `0041`'s `tenants` table and the existing `users`, `roles`, `warehouses` tables — ordering is enforced naturally by the migrate tool's version-number sort, same as every existing migration pair.

**Git/working-tree state confirmed before any file was written:** branch `feat/wms-multitenant`, clean except the Phase 27 commit `d95ef22` already present; `docs/architecture/` was present but untouched by this session (another session's files).

## 2. Deliberate Deviation from `migration-plan.md`'s Draft

`migration-plan.md` §3a's draft `0041_create_tenants_and_platform_admins.up.sql` includes `INSERT INTO tenants (id, slug, name, status) VALUES (1, 'default', 'Default Organization', 'ACTIVE')` — a bootstrap tenant row, intended for the Phase 29 backfill to target.

Phase 28's own instructions are explicit: **do not add `tenant_id` to existing business tables in this phase, and do not create production tenants automatically.** Inserting a tenant row now, before any backfill is scheduled or approved, would be creating tenant data in a phase scoped to schema-only work. **`0041` as implemented is structure-only — no `INSERT` statement.** The bootstrap tenant row is deferred to whichever future migration actually performs the Phase 29 backfill (`migration-plan.md`'s `0044`), where it belongs alongside the backfill it exists to support.

## 3. What Was Built

### 3a. Migrations (additive only; nothing existing was altered)

| File | Creates |
|---|---|
| `0041_create_tenants_and_platform_admins.up/down.sql` | `tenants`, `platform_admins` |
| `0042_create_tenant_support_tables.up/down.sql` | `subscription_plans`, `tenant_subscriptions`, `tenant_settings`, `tenant_features`, `tenant_audit_logs`, `tenant_users` |

All column types, constraints, and FK targets match `tenant-er-design.md` §2 exactly, except the two resolved inconsistencies in §1 above and the deferred bootstrap `INSERT` in §2. Constraint naming follows this codebase's existing convention exactly (`PK_<table>`, `FK_<table>_<col>`, `UQ_<table>_<cols>`, `CK_<table>_<col>`, `DF_<table>_<col>`), matching `migrations/0038_create_orders_tables.up.sql` as the style reference.

### 3b. Go models (schema-only — no handler, service, or repository; those are Phase 29+ per `saas-module-roadmap.md`)

| Package | File | Types |
|---|---|---|
| `internal/tenant` | `model.go` | `Tenant`, `TenantUser`, `TenantSetting`, `TenantAuditLog` |
| `internal/subscription` | `model.go` | `Plan`, `TenantSubscription`, `Feature` |
| `internal/platformadmin` | `model.go` | `Admin` |

Package boundaries match the module names already used in `saas-module-roadmap.md`'s phased rollout (Platform Admin, Tenant, Subscription are distinct future modules), so Phase 29+ does not need to relocate these types.

### 3c. Test infrastructure

| File | Purpose |
|---|---|
| `internal/platform/testsupport/db.go` | Shared, skip-if-unavailable `*gorm.DB` connector (reads the same `DB_*` env vars `config.Load` uses) plus a rollback-after-every-test transaction helper. Not imported by any binary (`cmd/server`, `cmd/worker`, `cmd/migrate`), so it never ships. |
| `internal/tenant/model_integration_test.go` | 10 tests: `tenants` uniqueness/CHECK, `tenant_users` duplicate-membership rejection, one-user/multiple-tenants, referential integrity (tenant/user/role FKs), status default, `tenant_settings` one-row-per-tenant, `tenant_audit_logs` nullable tenant_id + actor_type CHECK. |
| `internal/subscription/model_integration_test.go` | 6 tests: `subscription_plans` uniqueness/CHECK, `tenant_subscriptions` referential integrity, one-subscription-per-tenant, status CHECK, `tenant_features` per-(tenant, feature) uniqueness. |
| `cmd/migrate/migrations_test.go` | 4 DB-free static checks (every `.up.sql` has a matching `.down.sql`, no duplicate version numbers, `0041`/`0042` present and correctly named, versions sort ascending) + 1 live idempotency test (`up` run twice applies nothing new the second time). |

This codebase's existing test convention (see `internal/role/service_test.go`'s own doc comment) is pure-function unit tests only, with DB/constraint behavior "verified live against real SQL Server" by hand. Phase 28's tests follow the same philosophy but automate the live-verification step: they run against a real SQL Server and skip cleanly (not fail) when `DB_*` env vars don't point at a reachable database, so `go test ./...` stays green with no database configured, exactly like every pre-existing test package.

**Caller responsibility, stated explicitly in `testsupport.ConnectDB`'s doc comment:** these tests read the standard `DB_*` environment variables — whoever runs them is responsible for pointing those at a disposable database, never at a real environment. They were not wired into any CI/automation in this phase.

## 4. Verification Performed This Session

A disposable, ephemeral SQL Server 2022 container (`wms-phase28-disposable-db`, no persistent volume, removed at the end of the session) was used — not any shared or production database:

1. `go run ./cmd/migrate up` applied the existing `0001`–`0040` migrations cleanly, then `0041`–`0042`.
2. `go run ./cmd/migrate seed` populated the real RBAC catalog (7 roles, 55 permissions) and one admin user — this is what surfaced the 7-vs-3-role discrepancy in §1.
3. Full round-trip: `down` (rolls back `0042`), `down` again (rolls back `0041`), `up` (reapplies both), `up` again — the second `up` logged no new migrations, confirming idempotent/safe-rerun behavior manually, in addition to the automated `TestMigrateUp_IsIdempotent`.
4. `go build ./...`, `go vet ./...`, `go fmt ./...`, `go mod verify` — all clean.
5. `go test ./...` — full suite, including the 20 new Phase 28 tests — all passed. See §5.

One real bug was caught by `TestTenantSettings_OneRowPerTenant` during this process: the initial `TenantSetting.TenantID` GORM tag (`primaryKey` without `autoIncrement:false`) made GORM assume an IDENTITY column, and SQL Server rejected the insert ("does not have the identity property"). Fixed by adding `autoIncrement:false` to the tag — `tenant_id` is a plain FK-as-PK, not auto-incrementing.

## 5. Test Results

```
go test ./...   (against the disposable container, all packages)
```

All 36 test-bearing packages reported `ok`; packages with no test files reported `[no test files]` (expected, matches the Phase 26 baseline). Zero failures, zero skips (a live database was available this session). The 20 Phase 28 tests specifically:

- `internal/tenant`: 10/10 passed
- `internal/subscription`: 6/6 passed
- `cmd/migrate`: 5/5 passed (4 static + 1 live idempotency; the 4 static ones also count toward the pre-existing `cmd/migrate` package total)

No existing test in any of the 24 pre-existing modules regressed.

## 6. Remaining Risks / Deferred to Phase 29

- **No `tenant_id` on any existing business table yet** — this phase is additive-only by design. Every IDOR/cross-tenant risk catalogued in `phase27-audit-report.md` is still fully present in the running application; Phase 28 does not mitigate any of them, it only lays the foundation tables.
- **No bootstrap tenant exists.** Deliberately deferred (§2) — Phase 29 must create it as part of the backfill migration, not before.
- **Per-tenant role-copy provisioning is not implemented.** `tenant_users.role_id` currently has nothing enforcing "this role belongs to the same tenant" at the database level (by design — see `tenant-er-design.md` §5, Option B, application-layer enforcement) and no provisioning code exists yet to create a tenant's own role copies in the first place. The 7-role catalog correction in §1 must inform that provisioning logic when it's written.
- **`RequirePlatformAdmin` middleware and any tenant-context middleware do not exist yet** — `platform_admins` and `tenant_users` are pure data tables with nothing consulting them. This is explicitly Phase 29+/33 scope per `saas-module-roadmap.md`, not a Phase 28 gap.
- **Tests were run against a disposable container, once, this session.** They were not added to any CI pipeline or Docker Compose test target — doing so is a reasonable near-term follow-up but was outside this phase's explicit scope (database foundation only).
