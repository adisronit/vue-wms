-- Phase 29A — Tenant database isolation, step 2 of 4 ("backfill").
-- Data-only: no schema change in this file. Must run strictly after
-- 0041-0045 (needs every nullable tenant_id column to already exist,
-- and creates the tenants.id = 1 row every UPDATE below targets) and
-- strictly before 0047/0048 (which assume every row already has a
-- non-null tenant_id before tightening constraints).
--
-- Deliberately deviates from migration-plan.md §3a's draft, which
-- inserted this same bootstrap row inside 0041 itself: 0041 is already
-- committed (phase28-implementation-notes.md §2) as structure-only, by
-- design, specifically to defer this insert to "whichever future
-- migration actually performs the Phase 29 backfill" — this file is
-- that migration.
-- Forces the bootstrap tenant to id = 1 explicitly, rather than relying
-- on IDENTITY(1,1) happening to still be sitting at 1. In a real,
-- one-time application of this migration against a database that has
-- never had a tenants row before, the two are equivalent — but 0047/
-- 0048's `DEFAULT (1)` constraints (the backward-compatibility safety
-- net for every existing, unmodified INSERT path — see 0047's header
-- comment) hardcode the literal value 1, not "whatever id the bootstrap
-- tenant happens to have." Verified against a real disposable SQL
-- Server during this phase: after an iterative down/up cycle during
-- development (DELETE then re-INSERT via 0046's own down/up), IDENTITY
-- does not reuse the deleted value, so a second application without
-- this forced id would land on 3 — silently breaking every DEFAULT (1)
-- constraint's FK reference the moment any unmodified code path relied
-- on it. Forcing id = 1 explicitly makes this migration safe to
-- roll back and reapply, not just safe to apply once.
SET IDENTITY_INSERT tenants ON;
INSERT INTO tenants (id, slug, name, status)
VALUES (1, 'default', 'Default Organization', 'ACTIVE');
SET IDENTITY_INSERT tenants OFF;

DECLARE @bootstrap_tenant_id BIGINT = (SELECT id FROM tenants WHERE slug = 'default');

UPDATE roles SET tenant_id = @bootstrap_tenant_id WHERE tenant_id IS NULL;
UPDATE warehouses SET tenant_id = @bootstrap_tenant_id WHERE tenant_id IS NULL;
UPDATE zones SET tenant_id = @bootstrap_tenant_id WHERE tenant_id IS NULL;
UPDATE locations SET tenant_id = @bootstrap_tenant_id WHERE tenant_id IS NULL;
UPDATE products SET tenant_id = @bootstrap_tenant_id WHERE tenant_id IS NULL;
UPDATE inventory SET tenant_id = @bootstrap_tenant_id WHERE tenant_id IS NULL;
UPDATE inventory_transactions SET tenant_id = @bootstrap_tenant_id WHERE tenant_id IS NULL;

UPDATE receipts SET tenant_id = @bootstrap_tenant_id WHERE tenant_id IS NULL;
UPDATE receipt_items SET tenant_id = @bootstrap_tenant_id WHERE tenant_id IS NULL;
UPDATE receipt_confirmations SET tenant_id = @bootstrap_tenant_id WHERE tenant_id IS NULL;

UPDATE putaway_tasks SET tenant_id = @bootstrap_tenant_id WHERE tenant_id IS NULL;
UPDATE putaway_task_items SET tenant_id = @bootstrap_tenant_id WHERE tenant_id IS NULL;
UPDATE putaway_confirmations SET tenant_id = @bootstrap_tenant_id WHERE tenant_id IS NULL;

UPDATE picking_tasks SET tenant_id = @bootstrap_tenant_id WHERE tenant_id IS NULL;
UPDATE picking_task_items SET tenant_id = @bootstrap_tenant_id WHERE tenant_id IS NULL;
UPDATE picking_confirmations SET tenant_id = @bootstrap_tenant_id WHERE tenant_id IS NULL;

UPDATE shipments SET tenant_id = @bootstrap_tenant_id WHERE tenant_id IS NULL;
UPDATE shipment_items SET tenant_id = @bootstrap_tenant_id WHERE tenant_id IS NULL;
UPDATE shipment_confirmations SET tenant_id = @bootstrap_tenant_id WHERE tenant_id IS NULL;

UPDATE orders SET tenant_id = @bootstrap_tenant_id WHERE tenant_id IS NULL;
UPDATE order_items SET tenant_id = @bootstrap_tenant_id WHERE tenant_id IS NULL;

UPDATE stock_adjustments SET tenant_id = @bootstrap_tenant_id WHERE tenant_id IS NULL;
UPDATE stock_adjustment_items SET tenant_id = @bootstrap_tenant_id WHERE tenant_id IS NULL;
UPDATE stock_adjustment_confirmations SET tenant_id = @bootstrap_tenant_id WHERE tenant_id IS NULL;
UPDATE stock_adjustment_reminders SET tenant_id = @bootstrap_tenant_id WHERE tenant_id IS NULL;

UPDATE stock_counts SET tenant_id = @bootstrap_tenant_id WHERE tenant_id IS NULL;
UPDATE stock_count_items SET tenant_id = @bootstrap_tenant_id WHERE tenant_id IS NULL;
UPDATE stock_count_confirmations SET tenant_id = @bootstrap_tenant_id WHERE tenant_id IS NULL;

UPDATE replenishment_tasks SET tenant_id = @bootstrap_tenant_id WHERE tenant_id IS NULL;
UPDATE replenishment_confirmations SET tenant_id = @bootstrap_tenant_id WHERE tenant_id IS NULL;

UPDATE notifications SET tenant_id = @bootstrap_tenant_id WHERE tenant_id IS NULL;

UPDATE export_jobs SET tenant_id = @bootstrap_tenant_id WHERE tenant_id IS NULL;

-- tenant_users backfill: one membership row per existing `users` row,
-- using their current role_id, status ACTIVE (not the table's own
-- 'INVITED' default — these are already-active pre-existing users, not
-- new invitations), joined_at = now. Matches migration-plan.md §3d and
-- its §9 assumption that the single seeded admin user becomes tenant
-- 1's first tenant_users row with their existing role, never an
-- automatic platform_admins grant (that remains a deliberate, separate,
-- manual action).
INSERT INTO tenant_users (tenant_id, user_id, role_id, status, joined_at)
SELECT @bootstrap_tenant_id, u.id, u.role_id, 'ACTIVE', SYSUTCDATETIME()
FROM users u
WHERE NOT EXISTS (
    SELECT 1 FROM tenant_users tu
    WHERE tu.tenant_id = @bootstrap_tenant_id AND tu.user_id = u.id
);
