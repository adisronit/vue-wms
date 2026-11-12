# Tenant Isolation & Enforcement Design (Phase 27, Step 3)

**Status:** Proposal / reviewed draft. No code has been changed. This document designs *how* tenant isolation will be enforced once the schema in `tenant-er-design.md` exists; it does not implement anything.

## 1. Overview / Threat Model

Tenant isolation must guarantee that no request, background job, or generated file ever reads or mutates a row that does not belong to the tenant the caller is currently authenticated into — regardless of which ID a client supplies, how deep the call chain goes, or whether the path runs through HTTP, a worker, or a file download. Step 1's audit (`phase27-audit-report.md`) found **zero existing isolation anywhere** in the current codebase: every bare-ID lookup, every raw SQL/`Exec` statement, every "does this ID exist" checker, and the entire worker process are all isolation-blind today, because there is no `tenant_id` column to filter on yet. This document assumes the schema from `tenant-er-design.md` is in place and specifies the enforcement layered on top of it.

## 2. Authentication & JWT Design

**Claims shape — before (current):**
```
{ sub: userID, username, role_code, permissions: [...], iat, exp }
```

**Claims shape — after:**
```
{
  sub: userID,
  active_tenant_id: tenantID,
  tenant_role_code: "WAREHOUSE_MANAGER",
  permissions: [...]            // scoped to active_tenant_id's role, not global
  session_type: "TENANT" | "PLATFORM_ADMIN",   // mutually exclusive
  iat, exp
}
```

A token never carries more than one tenant's context. A user who belongs to multiple tenants (via `tenant_users`) picks one active tenant per session.

**Tenant switch:** `POST /api/v1/auth/switch-tenant { tenant_id }`. The handler re-verifies, server-side, that an `ACTIVE` `tenant_users` row exists for `(caller_user_id, requested_tenant_id)` before issuing a *new* token with that `active_tenant_id`. The client-supplied `tenant_id` is never trusted as the basis for the claim — it only selects which membership to re-verify. This is the same principle applied everywhere else in this design: a client-supplied tenant identifier is an input to a server-side check, never a credential by itself.

**Platform Super Admin session:** `session_type: "PLATFORM_ADMIN"` is issued only through a separate, step-up-authenticated flow (e.g. re-entering a password or a second factor immediately before elevation), and such a token carries no `active_tenant_id` at all — platform-admin and tenant-member are different session types, not a flag layered onto a normal tenant session. This means a stolen or leaked tenant-session token can never be used against platform-admin routes, even if the underlying user happens to also hold a `platform_admins` row.

## 3. Middleware Chain

Exact order, applied to every `/api/v1` route except `auth/*`:

```
RequireJWT → RequireTenantContext → RequirePermission → handler
```

Platform-admin routes use a parallel chain instead:
```
RequireJWT → RequirePlatformAdmin → handler
```
(`RequirePlatformAdmin` rejects any token whose `session_type != "PLATFORM_ADMIN"`, independent of `RequireTenantContext`/`RequirePermission` entirely — the two chains never both apply to the same route.)

**`RequireTenantContext`** (new):
1. Reads `active_tenant_id` from the validated JWT claims. Missing or malformed → `401`, request stops here. This is the fail-closed anchor for the whole design: no downstream code ever runs without a tenant context having been affirmatively established.
2. Re-verifies, against the database, that `tenant_users` has an `ACTIVE` row for `(sub, active_tenant_id)` with a role that still exists. This is the fix for the auth audit's critical finding that today's `RequirePermission` trusts the JWT's embedded claims with no DB re-check — once tenant membership can be revoked or changed mid-session, that staleness becomes a cross-tenant access risk, not just a stale-permission one.
3. Caches a positive result for 30–60 seconds (per `user_id, tenant_id`) to bound the added latency; a revocation is visible within that window, not instantly — call out explicitly as an accepted tradeoff, not a silent gap (see §11, unresolved: exact TTL).
4. On success, sets `tenant_id` on the request context for every downstream handler/service/repository call. No handler is permitted to read `active_tenant_id` directly off the raw JWT again — they consume it from the context this middleware populated, so there is exactly one place where the claim is turned into enforced context.

**`RequirePermission`** runs after, unchanged in spirit, but now checks a permission set that was issued scoped to `active_tenant_id`'s role — it has nothing new to do with tenant_id itself; isolation is `RequireTenantContext`'s job, authorization is `RequirePermission`'s.

## 4. Repository & Query Layer

**Convention:** every repository method that touches a tenant-owned table takes `tenantID` as a mandatory parameter — not optional, not inferred from an ambient scope, not something that can be forgotten by omission. This directly satisfies the constraint that GORM scopes must not be the *only* isolation control.

Before/after shape (pseudocode):
```
// Before
func (r *Repository) FindByID(ctx, id int64) (*Warehouse, error)

// After
func (r *Repository) FindByID(ctx, tenantID, id int64) (*Warehouse, error)
    // implementation: WHERE id = ? AND tenant_id = ?
```

A GORM global scope (e.g. a per-request `db.Where("tenant_id = ?", tenantID)` session derived from context) is still recommended as an **additional** defense-in-depth layer — if a future call site forgets to pass `tenantID` explicitly, the scope is a second net. It must never be the *only* net: the explicit parameter is what the Step 6 security test suite (see `saas-module-roadmap.md`) asserts against, because a global scope can silently be bypassed by `.Unscoped()`, a raw `.Raw()`/`.Exec()` call, or a new session built outside the scoped helper — all three of which the Step 1 audit found in active use today.

## 5. Raw SQL and Atomic Quantity Operations

The Step 1 audits found 40+ raw `Raw`/`Exec`/string-joined queries with no tenant predicate. The highest-risk subset is the hand-written atomic quantity math — these mutate stock and must never be fixed only at the application layer, because the risk is a wrong *row*, not a wrong *response*:

- `inventory.Repository.AdjustBalance` / `AdjustReservation`
- `receiving.Repository.AdjustReceivedQty`
- `putaway.Repository.AdjustMovedQty`
- `picking.Repository.AdjustPickedQty`
- `order.Repository.ClaimForPickingTask`

Fix pattern, applied identically to all five:
```
-- Before
UPDATE inventory SET quantity = quantity + ? WHERE product_id = ? AND location_id = ? AND lot_no = ?

-- After
UPDATE inventory SET quantity = quantity + ? WHERE tenant_id = ? AND product_id = ? AND location_id = ? AND lot_no = ?
```

`tenant_id` is always a bound parameter supplied from server-side context, never string-concatenated into the query text. The same pattern applies to every `Raw`/`Exec` list/detail/export query the audits flagged (e.g. `inventory.Summary`, `ListTransactionsForExport`, `receiving.FindReceiptDetail`, `putaway.resolveLocation`, `stockadjustment.FindOverduePending`) — add `tenant_id = ?` to the `WHERE`/`JOIN` condition, bound, not interpolated.

## 6. Cross-Tenant Reference Validation

Every "existence checker" interface the audits found — `locationChecker`, `productChecker`, `warehouseChecker`, `zoneChecker` (used across receiving, putaway, picking, order, stockadjustment, replenishment services) — currently answers only "does this ID exist at all." It must become an *ownership* checker:

```
// Before
FindByID(ctx, id) (*Entity, error)   // existence only

// After
FindByID(ctx, tenantID, id) (*Entity, error)
    // returns ErrNotFound if the row doesn't exist OR belongs to a different tenant — same error either way
```

The "same error either way" rule is deliberate: if a cross-tenant ID returned a different, distinguishable error (e.g. `ErrForbidden` vs `ErrNotFound`), a caller could enumerate which IDs exist in other tenants purely from the error shape, even without ever reading their data. Fail closed, and fail *identically*, for "doesn't exist" and "exists but isn't yours."

## 7. Background Jobs (RabbitMQ)

The infra audit's critical finding: the worker (`cmd/worker/main.go`) never touches `gin.Context`, never runs through `RequireJWT`/`RequireTenantContext`, and today has **zero authorization path by construction** — whatever a message says, it does, full stop.

Fix: the message envelope (already recommended by CLAUDE.md to carry `message_id`/`event_type`/`correlation_id`/`user_id`/`payload`) gains a mandatory `tenant_id` field alongside `user_id`. Every publisher sets it from the originating request's tenant context at publish time:
- `internal/stockadjustment/reminder.go` — stamps the adjustment's own `tenant_id` when publishing the pending-reminder message.
- `internal/report/service.go` — stamps the requesting user's `active_tenant_id` when publishing the export job message.

Every consumer handler in `cmd/worker/main.go` extracts `tenant_id` from the decoded payload and threads it through a new `dbctx.WithTenantID(ctx, tenantID)` — mirroring the existing `dbctx.WithRequestID` pattern — *before* calling into any service. Every service/repository call the worker makes must then take that tenant_id exactly as the HTTP path does (§4); there is no separate "worker mode" that skips the mandatory-parameter convention. This closes the gap without needing the worker to run any HTTP middleware at all — the tenant context travels in the message, not in a request.

## 8. File Storage and Reports

The shared `export_files` Docker volume is flat today — every tenant's generated CSVs land in one directory, filenames keyed by a sequential `export_jobs.id`. Fix: partition the directory by tenant — `exportDir/<tenant_id>/inventory_transactions_<jobID>_<ts>.csv` — and add `tenant_id` to `export_jobs`. `ResolveDownload` checks `job.tenant_id == caller's active_tenant_id` **in addition to** the existing `job.RequestedBy == userID` check, not instead of it — the two checks protect against different failure modes (wrong tenant vs. wrong user within the right tenant) and both must hold.

## 9. Row-Level Security Evaluation

SQL Server Row-Level Security was evaluated as explicitly requested, and the conclusion is **defense-in-depth only, not the primary control** — for a specific, verified reason, not a general preference.

RLS via `SESSION_CONTEXT` requires `EXEC sp_set_session_context 'tenant_id', @tenant_id` to run as the *first* statement on every connection before any tenant-scoped query executes on it. The infra audit confirmed `internal/platform/database`'s `Manager` holds one shared GORM connection pool (`SetMaxOpenConns(25)`) with **no per-request connection pinning and no session-reset hook today** — connections are checked out, used, and returned to the pool for reuse by the *next* request, which may belong to a different tenant entirely.

This creates two distinct failure modes if RLS were adopted naively:
1. **Fails open** — if a code path (a new query, a missed hook registration, a `.Raw()` call that bypasses GORM's session machinery) doesn't set `SESSION_CONTEXT` first, SQL Server applies no row filter at all, and the query behaves as if RLS didn't exist — silently, with no error.
2. **Leaks across requests** — worse, if `SESSION_CONTEXT` is set once per connection lifetime rather than once per *checkout*, a pooled connection can carry the *previous* request's tenant_id into the next request that reuses it, filtering the new request's results by the wrong tenant (visible as either a confusing empty result or, if application code also applies its own correct filter, a double-filtered false negative that masks the leak rather than surfacing it).

**Recommendation:** ship the application-layer `tenant_id` filtering in §4–§8 as the mandatory control first — it does not depend on connection lifecycle behavior at all. Treat RLS as an *optional* later hardening pass, and only adopt it after a dedicated spike proves, under load with the real connection pool, that a GORM callback hook (e.g. on `Session`/`Begin`) sets `SESSION_CONTEXT` unconditionally on *every* connection checkout, not just the first use of a fresh connection. Do not let RLS's eventual existence become a reason to relax or defer any application-layer check described above — the two are not substitutes for each other even once RLS is proven safe, because `SESSION_CONTEXT` protects the database from the application's own bugs, while the application-layer checks protect against a bug in connection-pool behavior itself. Belt and suspenders, deliberately redundant.

## 10. Fail-Closed Enforcement Summary

| Failure Mode | Current Behavior (pre-tenant) | Required Behavior (fail-closed) |
|---|---|---|
| JWT missing `active_tenant_id` | N/A (claim doesn't exist yet) | `401` — request never reaches a handler |
| `tenant_users` membership revoked mid-session | N/A (no membership concept) | Next request after cache TTL expiry → `401`/`403`, not served with stale permissions |
| Repository method called without `tenantID` | N/A (no parameter exists) | Does not compile / fails code review — convention enforced at the signature level |
| Raw SQL missing tenant predicate | Returns all tenants' rows | Must return zero rows for any tenant but the caller's — enforced by the security test suite, not just review |
| Cross-tenant ID passed to an existence checker | Resource found, used | `ErrNotFound`, identical to a truly nonexistent ID — no distinguishable error |
| Worker message missing `tenant_id` | Processed anyway, full DB access | Reject the message / dead-letter it rather than process with no tenant context |
| Export file requested across tenants | Served if `RequestedBy` matched (user-level only) | `404` unless both `tenant_id` and `RequestedBy` match |
| RLS `SESSION_CONTEXT` not set on a pooled connection | N/A (RLS not in use) | If adopted: must fail the query/connection rather than silently apply no filter |

## 11. Assumptions, Verified Facts, and Unresolved Design Choices

**Verified Facts** (confirmed by the Step 1 audits, cited in `phase27-audit-report.md`):
- `internal/platform/middleware/permission.go`'s `RequirePermission` checks only the JWT's own embedded claims today, with no DB re-check — explicitly documented in-code as an accepted tradeoff for permission staleness.
- `internal/platform/database`'s `Manager` uses a single shared connection pool with no per-request session binding.
- `cmd/worker/main.go` never invokes any HTTP middleware and passes a bare background context into service calls.
- 40+ raw `Raw`/`Exec` call sites exist across the inventory/receiving/putaway/picking/shipping/order/stockadjustment/stockcount/replenishment modules with no tenant predicate.

**Assumptions** (reasonable but not independently verified against a running system):
- A 30–60 second membership-revocation cache window is an acceptable latency/consistency tradeoff; this has not been validated against actual latency budgets or a security requirement for instant revocation.
- GORM's `Session`/`Begin` hooks are capable of running unconditionally on every connection checkout for the RLS spike — this is asserted as plausible based on GORM's documented plugin/callback model, not confirmed against this specific GORM version's pooling behavior.

**Unresolved Design Choices** (need a decision before Phase 28 implementation):
- Exact cache TTL for `RequireTenantContext`'s membership re-check, and whether it should be invalidated eagerly (e.g. on a membership-change event) rather than purely time-based.
- Whether to add a custom Go static-analysis lint rule (e.g. a `go vet`-style check or an AST-based CI gate) that mechanically enforces "no exported Repository method on a tenant-owned model may omit a `tenantID` parameter," versus relying on code review plus the Step 6 security test suite to catch regressions.
- Whether the RLS spike is worth doing at all in the near term, given the application-layer design above is sufficient on its own and RLS adds operational complexity (session-context management, additional SQL Server permissions) for a second layer of defense rather than a required one.

---
See `tenant-er-design.md` for the schema this design enforces, and `migration-plan.md` for how the schema and this enforcement are rolled out without breaking existing data. See `saas-module-roadmap.md` for the full security test suite that validates this design.
