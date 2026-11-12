# Multi-Tenant SaaS Architecture — Executive Overview

**Phase 27 deliverable.** This document is the entry point for the Phase 27 proposal to turn the existing single-tenant WMS into a multi-tenant SaaS product. It summarizes the findings and decisions spread across five sibling documents in this folder and tells you which one to open next for detail. **Nothing in this phase has been implemented.** No schema change has been applied, no application code has been written, and no production database has been touched. This is audit and design only, produced for human review before any Phase 28 implementation work begins.

## Executive Summary

The existing WMS — a mature, single-tenant, production-oriented system (26 completed development phases) built on Go/Gin/GORM/SQL Server with a Vue 3 frontend — was audited end-to-end for what it would take to safely serve multiple independent customer organizations from one codebase. The headline finding is stark: **zero tenant isolation exists anywhere today.** There is no `tenant_id` column on any table, and every bare-ID database lookup, every hand-written SQL quantity update, and every "does this referenced record exist" check across the inbound, outbound, and operations modules would become a cross-tenant data leak or a cross-tenant data corruption bug the moment a second tenant's data entered the same database. The background worker process is a special case of concern: it has no authorization path at all, by construction, since it never passes through HTTP middleware. The recommendation is a phased, additive, non-destructive migration — a new tenant/membership/subscription schema layered on top of the existing tables, enforced primarily by mandatory tenant-scoped application code (not database features alone) — rolled out module-by-module over several follow-on phases, starting only after this proposal has been reviewed.

## Current Architecture

- **Backend:** Go 1.23, Gin, GORM, Microsoft SQL Server. Roughly 24 internal modules: `auth`, `user`, `role`, `rbac`, `warehouse`, `zone`, `location`, `product`, `inventory`, `receiving`, `putaway`, `picking`, `shipping`, `order`, `stockadjustment`, `stockcount`, `replenishment`, `notification`, `report`, `dashboard`, `health`, and `messaging/rabbitmq`.
- **Frontend:** Vue 3 + Vite.
- **AuthN/AuthZ:** JWT + bcrypt, with RBAC permissions embedded as claims at login time.
- **Async processing:** RabbitMQ backs three background flows today — a health heartbeat, stock-adjustment reminders, and report/export generation — each consumed by a separate worker process.
- **Documentation & ops:** Swagger/OpenAPI, Docker Compose.

This is not a greenfield project. The audit treated the existing business logic, schema, and API surface as fixed constraints to design around, not to rewrite.

## Target SaaS Architecture

The target shape keeps every existing module and endpoint in place and threads a verified tenant identity through each request and each background job, rather than trusting any tenant identifier supplied by the client.

**Synchronous HTTP path:**
Browser → Vue (tenant-aware — e.g. a tenant switcher in the UI, carrying the active tenant in the session) → Gin API, through a middleware chain of `RequireJWT` → `RequireTenantContext` → `RequirePermission` → Handler → Service (tenant ID threaded through every call) → Repository (tenant ID is a mandatory parameter on every method touching a tenant-owned table) → SQL Server (every tenant-owned table carries a `tenant_id` column; Row-Level Security is an optional later hardening layer, not the primary control).

**Asynchronous path:**
HTTP request → Service validates, writes, and publishes → RabbitMQ (message envelope now carries `tenant_id` alongside the existing `message_id`/`event_type`/`correlation_id`/`user_id` fields) → Worker consumes → a new `dbctx.WithTenantID` call seeds the tenant into the processing context → Service → Repository → SQL Server, using exactly the same mandatory-tenant-ID repository layer as the HTTP path.

The point of routing both paths through the same repository convention is that tenant enforcement lives in one place conceptually, even though it has to be mechanically applied across roughly 30 tables and 40+ raw-SQL call sites.

## Why Zero Trust in tenant_id From the Client

The single rule underlying every design decision in this proposal: **a tenant identifier is never trusted merely because a request or a token carries one — it is re-verified against live membership on the server before any data access.** This is not a hypothetical precaution. The audit found that the existing permission-check middleware already has a related, currently-accepted gap: it checks only the permissions embedded in a JWT at login time and never re-queries the database, so a permission or role change takes effect only on the user's next login. That gap is tolerable today because there is only one tenant. Once tenants exist, the same stale-claim pattern becomes a cross-tenant risk — a user removed from a tenant, or whose role changes, would otherwise keep acting with yesterday's tenant context until their token expires. The target design's `RequireTenantContext` middleware exists specifically to re-check live tenant membership on each request (with a short cache for latency, never skipped entirely), closing this gap rather than inheriting it.

## Document Map

| Document | Covers | Key Decisions |
|---|---|---|
| `phase27-audit-report.md` | Step 1 — the full module-by-module tenant-risk audit | Catalogs every bare-ID lookup, raw-SQL/Exec statement, and existence-only check across all ~24 backend modules; confirms zero existing isolation |
| `tenant-er-design.md` | Step 2 — the tenant data model | New tables `tenants`, `tenant_users`, `tenant_settings`, `subscription_plans`, `tenant_subscriptions`, `tenant_features`, `tenant_audit_logs`, plus `platform_admins`; `users` stays platform-global identity; `roles` becomes tenant-scoped with per-tenant seeded copies; ~30 existing tables reclassified as tenant-owned |
| `tenant-isolation-security.md` | Step 3 — enforcement design | JWT `active_tenant_id`/`session_type` claims, `RequireTenantContext` middleware, mandatory tenant-ID repository parameters, raw-SQL fix patterns, worker/message tenant propagation, file-storage partitioning, RLS treated as optional defense-in-depth |
| `migration-plan.md` | Step 4 — rollout sequencing | Safe expand-then-contract migration (nullable add → single-tenant backfill → verify → NOT NULL + constraint rebuild), fully additive and reversible until the final contract step, with mandatory backup/restore testing |
| `saas-module-roadmap.md` | Steps 5 & 6 — platform features, security tests, phased rollout | Platform admin and tenant admin APIs, subscription/plan enforcement as an async RabbitMQ lifecycle check, a tenant-aware integration framework, ~15 designed security test cases, and a proposed Phase 28+ rollout sequence |
| `multi-tenant-architecture.md` *(this document)* | Executive summary and navigation | Ties the above together; states the one flagged conflict and the cross-cutting open questions |

## The One Flagged Conflict

This project's `CLAUDE.md` contains a standing instruction that AGV, AMR, PLC, MQTT, ROS, and fleet/traffic-management integrations are explicitly out of scope and should not be added unless explicitly requested. The Phase 27 task's own Step 5 asks for integration design that names PLC and AGV/AMR specifically. `saas-module-roadmap.md` resolves this by designing a generic, tenant-aware integration framework (credentials, enablement, and a webhook/adapter abstraction) that covers Odoo/ERP, barcode/QR, and external REST APIs in real detail — but deliberately stops short of any protocol-level PLC or AGV/AMR design. This conflict is surfaced here rather than resolved silently in either direction: **please confirm explicitly whether PLC/AGV/AMR integration design should proceed** before any further work touches that area.

## Assumptions, Verified Facts, and Unresolved Design Choices

**Verified facts** (confirmed directly by the Step 1 code audit, detailed with file:line citations in `phase27-audit-report.md`): no table in the current schema has a tenant concept in any form; every repository method audited that fetches or mutates a business record by ID does so without any ownership check; the background worker never touches Gin middleware and therefore has no authorization path of any kind.

**Assumptions** (reasonable but not independently verified beyond the audit's scope): there is exactly one existing tenant's worth of data to migrate — the plan backfills everything into a single default tenant, with no need to retroactively split existing records across multiple tenants; the existing seeded admin user and RBAC catalog can be cleanly assigned to that one default tenant without manual data cleanup first.

**Unresolved design choices** (need a decision, business or technical, before Phase 28): final subscription tiers and pricing are a business decision outside engineering scope; whether Odoo/ERP integration is wanted now or should remain a future-proofing placeholder; the PLC/AGV/AMR conflict above; and the exact cache lifetime for the tenant-membership re-check in `RequireTenantContext`, which trades off latency against how quickly a revoked membership takes effect.

## Recommended Next Step

This proposal needs a human review pass before Phase 28 implementation begins. A thorough review should: read this document first for the overall shape; spot-check a handful of rows in `phase27-audit-report.md` against the actual source files to confirm the audit's accuracy; read `tenant-er-design.md` and `tenant-isolation-security.md` together, since the schema and the enforcement design must agree with each other; confirm the `migration-plan.md` rollout sequence is acceptable given the no-destructive-changes and no-production-changes constraints this phase was run under; and make an explicit call on the PLC/AGV/AMR conflict above. Only once that review is complete should Phase 28 — the first implementation phase, adding the new platform/tenant tables — begin.
