-- Phase 29A — Tenant database isolation, step 1 of 4 ("expand"),
-- continued from 0043/0044. Adds nullable tenant_id to every operations
-- table (stock adjustment, stock count, replenishment, notification,
-- export job) per tenant-er-design.md §3.
--
-- stock_adjustment_reminders has no GORM model (internal/stockadjustment
-- writes it via raw SQL, see repository.go's ClaimReminder) but is still
-- a Tenant-Owned table per the audit's operations findings — its
-- tenant_id is added here for the same reason every other reminder/
-- confirmation table gets one, even though no Go struct change
-- accompanies it.
ALTER TABLE stock_adjustments ADD tenant_id BIGINT NULL;
ALTER TABLE stock_adjustment_items ADD tenant_id BIGINT NULL;
ALTER TABLE stock_adjustment_confirmations ADD tenant_id BIGINT NULL;
ALTER TABLE stock_adjustment_reminders ADD tenant_id BIGINT NULL;

ALTER TABLE stock_counts ADD tenant_id BIGINT NULL;
ALTER TABLE stock_count_items ADD tenant_id BIGINT NULL;
ALTER TABLE stock_count_confirmations ADD tenant_id BIGINT NULL;

ALTER TABLE replenishment_tasks ADD tenant_id BIGINT NULL;
ALTER TABLE replenishment_confirmations ADD tenant_id BIGINT NULL;

ALTER TABLE notifications ADD tenant_id BIGINT NULL;

ALTER TABLE export_jobs ADD tenant_id BIGINT NULL;
