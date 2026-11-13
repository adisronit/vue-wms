-- Phase 29A — Tenant database isolation, step 1 of 4 ("expand"),
-- continued from 0043. Adds nullable tenant_id to every inbound/outbound
-- workflow table (receiving, putaway, picking, shipping, orders) per
-- tenant-er-design.md §3. Each document's child item/confirmation table
-- gets its own denormalized tenant_id column too (not inherited only via
-- the parent header's FK) — matching the zones/locations denormalization
-- precedent already established for warehouse_id/zone_id in 0006/0007,
-- and avoiding a join back up to the header for every tenant-scoped
-- query once the application layer starts filtering by it (Phase 29B+).
ALTER TABLE receipts ADD tenant_id BIGINT NULL;
ALTER TABLE receipt_items ADD tenant_id BIGINT NULL;
ALTER TABLE receipt_confirmations ADD tenant_id BIGINT NULL;

ALTER TABLE putaway_tasks ADD tenant_id BIGINT NULL;
ALTER TABLE putaway_task_items ADD tenant_id BIGINT NULL;
ALTER TABLE putaway_confirmations ADD tenant_id BIGINT NULL;

ALTER TABLE picking_tasks ADD tenant_id BIGINT NULL;
ALTER TABLE picking_task_items ADD tenant_id BIGINT NULL;
ALTER TABLE picking_confirmations ADD tenant_id BIGINT NULL;

ALTER TABLE shipments ADD tenant_id BIGINT NULL;
ALTER TABLE shipment_items ADD tenant_id BIGINT NULL;
ALTER TABLE shipment_confirmations ADD tenant_id BIGINT NULL;

ALTER TABLE orders ADD tenant_id BIGINT NULL;
ALTER TABLE order_items ADD tenant_id BIGINT NULL;
