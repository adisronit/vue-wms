ALTER TABLE stock_adjustments DROP COLUMN tenant_id;
ALTER TABLE stock_adjustment_items DROP COLUMN tenant_id;
ALTER TABLE stock_adjustment_confirmations DROP COLUMN tenant_id;
ALTER TABLE stock_adjustment_reminders DROP COLUMN tenant_id;

ALTER TABLE stock_counts DROP COLUMN tenant_id;
ALTER TABLE stock_count_items DROP COLUMN tenant_id;
ALTER TABLE stock_count_confirmations DROP COLUMN tenant_id;

ALTER TABLE replenishment_tasks DROP COLUMN tenant_id;
ALTER TABLE replenishment_confirmations DROP COLUMN tenant_id;

ALTER TABLE notifications DROP COLUMN tenant_id;

ALTER TABLE export_jobs DROP COLUMN tenant_id;
