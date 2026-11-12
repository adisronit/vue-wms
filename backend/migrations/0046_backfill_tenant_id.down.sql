-- Reverses 0046's data-only backfill. Only ever reachable by the
-- migrate tool after 0048 and 0047 have already been rolled back first
-- (the tool only ever rolls back the single most-recently-applied
-- version), so tenant_id is still nullable and unconstrained here —
-- setting it back to NULL and removing the rows this migration inserted
-- is safe and exact, not the documented no-op migration-plan.md §7
-- originally anticipated when it assumed 0044's backfill would be the
-- last reversible step before an irreversible contract phase.
DECLARE @bootstrap_tenant_id BIGINT = (SELECT id FROM tenants WHERE slug = 'default');

DELETE FROM tenant_users WHERE tenant_id = @bootstrap_tenant_id;

UPDATE roles SET tenant_id = NULL WHERE tenant_id = @bootstrap_tenant_id;
UPDATE warehouses SET tenant_id = NULL WHERE tenant_id = @bootstrap_tenant_id;
UPDATE zones SET tenant_id = NULL WHERE tenant_id = @bootstrap_tenant_id;
UPDATE locations SET tenant_id = NULL WHERE tenant_id = @bootstrap_tenant_id;
UPDATE products SET tenant_id = NULL WHERE tenant_id = @bootstrap_tenant_id;
UPDATE inventory SET tenant_id = NULL WHERE tenant_id = @bootstrap_tenant_id;
UPDATE inventory_transactions SET tenant_id = NULL WHERE tenant_id = @bootstrap_tenant_id;

UPDATE receipts SET tenant_id = NULL WHERE tenant_id = @bootstrap_tenant_id;
UPDATE receipt_items SET tenant_id = NULL WHERE tenant_id = @bootstrap_tenant_id;
UPDATE receipt_confirmations SET tenant_id = NULL WHERE tenant_id = @bootstrap_tenant_id;

UPDATE putaway_tasks SET tenant_id = NULL WHERE tenant_id = @bootstrap_tenant_id;
UPDATE putaway_task_items SET tenant_id = NULL WHERE tenant_id = @bootstrap_tenant_id;
UPDATE putaway_confirmations SET tenant_id = NULL WHERE tenant_id = @bootstrap_tenant_id;

UPDATE picking_tasks SET tenant_id = NULL WHERE tenant_id = @bootstrap_tenant_id;
UPDATE picking_task_items SET tenant_id = NULL WHERE tenant_id = @bootstrap_tenant_id;
UPDATE picking_confirmations SET tenant_id = NULL WHERE tenant_id = @bootstrap_tenant_id;

UPDATE shipments SET tenant_id = NULL WHERE tenant_id = @bootstrap_tenant_id;
UPDATE shipment_items SET tenant_id = NULL WHERE tenant_id = @bootstrap_tenant_id;
UPDATE shipment_confirmations SET tenant_id = NULL WHERE tenant_id = @bootstrap_tenant_id;

UPDATE orders SET tenant_id = NULL WHERE tenant_id = @bootstrap_tenant_id;
UPDATE order_items SET tenant_id = NULL WHERE tenant_id = @bootstrap_tenant_id;

UPDATE stock_adjustments SET tenant_id = NULL WHERE tenant_id = @bootstrap_tenant_id;
UPDATE stock_adjustment_items SET tenant_id = NULL WHERE tenant_id = @bootstrap_tenant_id;
UPDATE stock_adjustment_confirmations SET tenant_id = NULL WHERE tenant_id = @bootstrap_tenant_id;
UPDATE stock_adjustment_reminders SET tenant_id = NULL WHERE tenant_id = @bootstrap_tenant_id;

UPDATE stock_counts SET tenant_id = NULL WHERE tenant_id = @bootstrap_tenant_id;
UPDATE stock_count_items SET tenant_id = NULL WHERE tenant_id = @bootstrap_tenant_id;
UPDATE stock_count_confirmations SET tenant_id = NULL WHERE tenant_id = @bootstrap_tenant_id;

UPDATE replenishment_tasks SET tenant_id = NULL WHERE tenant_id = @bootstrap_tenant_id;
UPDATE replenishment_confirmations SET tenant_id = NULL WHERE tenant_id = @bootstrap_tenant_id;

UPDATE notifications SET tenant_id = NULL WHERE tenant_id = @bootstrap_tenant_id;

UPDATE export_jobs SET tenant_id = NULL WHERE tenant_id = @bootstrap_tenant_id;

DELETE FROM tenants WHERE id = @bootstrap_tenant_id;
