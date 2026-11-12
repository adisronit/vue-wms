-- Phase 29A — Tenant database isolation, step 1 of 4 ("expand").
-- Adds tenant_id as a NULLable column to every master-data/inventory/RBAC
-- table identified as Tenant-Owned by docs/proposals/wms-saas/
-- tenant-er-design.md §3 and phase27-audit-report.md. Nullable and
-- additive only — no existing row is touched, no application code needs
-- to change yet, and this step alone is trivially reversible (drop the
-- column). Backfill (0046), tenant-scoped unique constraints (0047), and
-- NOT NULL + FK + indexes (0048) are later, separate migrations, per
-- migration-plan.md's "expand, then contract, as separate migrations"
-- rule.
--
-- role_permissions and role_warehouse_access deliberately do NOT get
-- their own tenant_id column here: tenant-er-design.md §3 classifies
-- both as "Tenant-Owned (via role), Inherited, no own column needed" —
-- role_id already resolves to a tenant-scoped roles row once this
-- migration runs, and a redundant tenant_id would need its own
-- consistency enforcement (trigger or app-layer check) against roles.
-- tenant_id for no query-time benefit, since nothing joins through these
-- two join tables without already having role_id in hand. This resolves
-- an inconsistency between tenant-er-design.md §3 (excludes them) and
-- migration-plan.md §2's 0043a file list (includes them) in favor of the
-- ER design's explicit, reasoned table — see phase29a-implementation-
-- notes.md §1 for the full writeup, mirroring how phase28-implementation-
-- notes.md resolved its own §1 cross-document conflicts.
ALTER TABLE roles ADD tenant_id BIGINT NULL;
ALTER TABLE warehouses ADD tenant_id BIGINT NULL;
ALTER TABLE zones ADD tenant_id BIGINT NULL;
ALTER TABLE locations ADD tenant_id BIGINT NULL;
ALTER TABLE products ADD tenant_id BIGINT NULL;
ALTER TABLE inventory ADD tenant_id BIGINT NULL;
ALTER TABLE inventory_transactions ADD tenant_id BIGINT NULL;
