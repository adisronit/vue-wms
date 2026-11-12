# Phase 29 — Tenant Isolation Security Test Suite (WMS-29)

Status: **test-only deliverable.** Nothing in this directory, or in
`internal/platform/testsupport/router.go` / `internal/platform/tenantaudit/`,
changes any production handler, service, repository, model, or migration.
This is the acceptance-test suite Phase 29's real implementation must
satisfy, written against the design already approved in
`docs/proposals/wms-saas/tenant-isolation-security.md` and
`docs/proposals/wms-saas/saas-module-roadmap.md` ("Security Test
Design").

## Why every isolation test here fails today — on purpose

`docs/proposals/wms-saas/phase27-audit-report.md`'s headline finding is
unchanged as of this suite: **zero `tenant_id` columns exist anywhere**
in the schema. Migrations 0041/0042 (Phase 28, already merged) added
only the platform-root tables (`tenants`, `platform_admins`,
`subscription_plans`, `tenant_subscriptions`, `tenant_settings`,
`tenant_features`, `tenant_audit_logs`, `tenant_users`). The business
tables — `warehouses`, `zones`, `locations`, `products`, `users`,
`roles`, and everything below them — have no `tenant_id` yet.
Migrations 0043–0046 (add the column, backfill, re-scope uniqueness,
add `NOT NULL`+FK) are themselves Phase 29 deliverables, not yet
written, and this task was explicitly scoped to **not** write them or
touch any production code. The JWT also carries no tenant claim yet
(`tenant-isolation-security.md` §2's redesigned `Claims` shape is not
implemented).

Given that, every test in this package that asserts a tenant-isolation
guarantee is **expected to fail** — a failure here means "Tenant A can
currently reach Tenant B's data," which is true for every such test by
the audit's own construction. These are not regressions; they are the
executable version of the Phase 27 audit's findings, and they are this
suite's way of saying "implementation pending" in a way `go test` can
report precisely, per-finding, instead of a design doc asserting it in
prose.

**A handful of tests are expected to PASS** — they check things that
*are* already correct today and must stay correct as Phase 29 lands:
RBAC permission enforcement (`TestMissingPermission_IsRejected`), JWT
presence/validity (`TestMissingOrInvalidJWT_IsRejected`), connection-pool
query-parameter correctness under concurrency
(`TestConnectionPool_NoTenantLeakage`), the existing
`ReplaceRoleWarehouseAccess` transaction's rollback behavior
(`TestTransactionRollback_NoPartialWrite`), and the warehouse-delete
referential-safety guard
(`TestWarehouseDelete_NoPartialState_WhenZonesExist`). If any of these
start failing, that is a real regression, not expected implementation
debt.

## How to re-run this suite

These are real integration tests against a live SQL Server — they do
**not** run against mocks, and `internal/platform/testsupport.ConnectDB`
skips (does not fail) every test if no database is configured. Point
`DB_*` at a disposable database (never production/UAT) and run:

```
cd backend
go run ./cmd/migrate up
go run ./cmd/migrate seed
go test ./test/isolation/... -v
go test ./internal/platform/tenantaudit/... -v
```

A throwaway SQL Server for this is trivial via Docker:

```
docker run -d --name wms-isolation-db -e ACCEPT_EULA=Y \
  -e MSSQL_SA_PASSWORD='<pick-one>' -p 11433:1433 \
  mcr.microsoft.com/mssql/server:2022-latest
```

then set `DB_HOST=localhost DB_PORT=11433 DB_USER=sa
DB_PASSWORD=<pick-one> DB_NAME=wms_isolation_test` (plus
`APP_ENV`/`APP_PORT`/`JWT_SECRET`/`JWT_EXPIRES_IN`/`DB_TRANSACTION_TIMEOUT`
and dummy `RABBITMQ_*` values — `cmd/migrate` and this suite never touch
RabbitMQ) before the commands above. This is exactly how this suite was
authored and verified — see "Verification record" below.

## Scope: what this suite covers, and what it doesn't

This suite covers exactly Phase 29's own module scope per
`saas-module-roadmap.md`'s "Implementation Phases" table: **auth, user,
role, rbac, warehouse, zone, location, product.** It deliberately does
**not** build fixtures/HTTP tests for inventory, receiving, putaway,
picking, shipping, order, stockadjustment, stockcount, replenishment,
notification, report, or dashboard — those are Phase 30–32's own
isolation-enforcement scope, and building a parallel fixture-and-HTTP
harness for all eleven of them now would duplicate work Phase 30-32
will redo anyway once those modules actually gain `tenant_id`.

**Every repository in the codebase is still covered**, just by a
different mechanism: `internal/platform/tenantaudit` is a small
AST-based static checker (resolving the "Unresolved Design Choice" left
open in `tenant-isolation-security.md` §11 about whether to automate
this) that parses all 17 `internal/*/repository.go` files and flags
every exported, DB-touching method with no tenant-scoping parameter.
Its test, `TestNoRepositoryMethodOmitsTenantID`, found **170 such
methods across all 17 files** as of this writing — this is the "audit
every repository" requirement, operationalized as a test instead of a
one-time document, and it will go green incrementally as Phase 29-32
thread a `tenantID` parameter through each module's repository. A
regression (a new method added without one) will make it fail again
immediately, which a prose audit document cannot do.

## File map

| File | Covers |
|---|---|
| `harness_test.go` | Shared httptest router + request/response helpers |
| `fixtures_test.go` | Tenant A/B fixture builder (warehouse→zone→location, product, ADMIN user, custom role) |
| `idor_master_data_test.go` | `TestTenantA_CannotReadTenantB_ByID`, `..._CannotUpdateTenantB_ByID`, `..._CannotDeleteTenantB_ByID`, zone/location list query-param IDOR |
| `cross_tenant_fk_test.go` | `TestCrossTenantForeignKey_RejectedOnCreate` (zone→warehouse, location→zone, user→warehouse), role warehouse-access grant |
| `role_user_isolation_test.go` | Cross-tenant role read/mutate, RBAC-permission negative test, JWT fail-closed negative test |
| `enumeration_concurrency_test.go` | `TestIDOR_SequentialIDEnumeration`, `TestConnectionPool_NoTenantLeakage` |
| `transaction_isolation_test.go` | Rollback-on-partial-failure, referential-safety delete guard |

## Verification record (as run while authoring this suite)

Against a disposable `mcr.microsoft.com/mssql/server:2022-latest`
container with migrations 0001–0042 applied and the RBAC catalog
seeded:

- `go test ./test/isolation/... -v`: **9 of 14 top-level tests FAIL**
  (every isolation-guarantee test, each logging the exact HTTP
  status/body/row-state that confirms the finding: `TestTenantA_CannotReadTenantB_ByID`,
  `..._CannotUpdateTenantB_ByID`, `..._CannotDeleteTenantB_ByID`,
  `TestZoneList_CannotEnumerateViaWarehouseIDParam`,
  `TestLocationList_CannotEnumerateViaZoneIDParam`,
  `TestCrossTenantForeignKey_RejectedOnCreate`,
  `TestTenantA_CannotGrantTenantB_WarehouseToOwnRole`,
  `TestIDOR_SequentialIDEnumeration`,
  `TestTenantA_CannotReadOrMutateTenantB_CustomRole`), **5 PASS**
  (`TestMissingPermission_IsRejected`,
  `TestMissingOrInvalidJWT_IsRejected`,
  `TestConnectionPool_NoTenantLeakage`,
  `TestTransactionRollback_NoPartialWrite`,
  `TestWarehouseDelete_NoPartialState_WhenZonesExist`).
- `go test ./internal/platform/tenantaudit/... -v`: the synthetic
  sanity test passes; `TestNoRepositoryMethodOmitsTenantID` fails,
  reporting 170 violations across all 17 repository files.
- `go build ./internal/... ./config/...` and
  `go vet ./internal/... ./config/... ./test/...`: clean.
  (`go build ./...`/`go vet ./...` from the repo root also reports a
  pre-existing, unrelated failure on `cmd/server` — `package
  wms-backend/docs is not in std` — because the swaggo-generated `docs`
  package has never been generated in this checkout; run `swag init -g
  cmd/server/main.go` to produce it. This is not caused by, or fixed by,
  this test suite.)

Re-run the two `go test` commands above after any Phase 29-32 change and
diff the pass/fail list against this record.
