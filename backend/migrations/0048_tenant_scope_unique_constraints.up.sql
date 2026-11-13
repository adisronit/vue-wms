-- Phase 29A — Tenant database isolation, step 4 of 4 ("contract", part
-- 2). The constraint-swap half of migration-plan.md's "contract" phase,
-- split from 0047's NOT NULL/FK/index changes so a problem in one is
-- independently reversible.
--
-- Runs AFTER 0047 (NOT NULL/FK), the reverse of migration-plan.md §2's
-- own draft order — see 0047's header comment for why: SQL Server
-- rejects ALTER COLUMN on a column a UNIQUE constraint's index already
-- depends on, verified against a real disposable SQL Server during this
-- phase.
--
-- Must also run strictly after 0046's backfill: every row has
-- tenant_id = 1 at this point (no second tenant has ever been created),
-- so each composite constraint below is exactly equivalent to the
-- single-column constraint it replaces — safe only under that
-- condition, per migration-plan.md §3e.
--
-- Rescopes every existing human-readable-code / document-number unique
-- constraint found by direct inspection of migrations 0001-0040 (not
-- assumed from the design docs alone — see phase29a-implementation-
-- notes.md §2 for three corrections this file makes to
-- tenant-er-design.md §3's own table, which omitted
-- stock_adjustments.adjustment_no, stock_counts.count_no, and
-- replenishment_tasks.task_no despite each one carrying the exact same
-- global-UNIQUE-document-number pattern as receipt_no/putaway_no/
-- pick_no/shipment_no/order_no, which the ER design table did flag).
ALTER TABLE roles DROP CONSTRAINT UQ_roles_code;
ALTER TABLE roles ADD CONSTRAINT UQ_roles_tenant_code UNIQUE (tenant_id, code);

ALTER TABLE warehouses DROP CONSTRAINT UQ_warehouses_code;
ALTER TABLE warehouses ADD CONSTRAINT UQ_warehouses_tenant_code UNIQUE (tenant_id, code);

ALTER TABLE zones DROP CONSTRAINT UQ_zones_warehouse_code;
ALTER TABLE zones ADD CONSTRAINT UQ_zones_tenant_warehouse_code UNIQUE (tenant_id, warehouse_id, code);

ALTER TABLE locations DROP CONSTRAINT UQ_locations_zone_code;
ALTER TABLE locations ADD CONSTRAINT UQ_locations_tenant_zone_code UNIQUE (tenant_id, zone_id, code);

ALTER TABLE products DROP CONSTRAINT UQ_products_sku;
ALTER TABLE products ADD CONSTRAINT UQ_products_tenant_sku UNIQUE (tenant_id, sku);

-- A plain UNIQUE constraint only allows a single NULL barcode per the
-- indexed key in SQL Server; this stays a filtered index (not a
-- constraint) for the same reason 0008 originally chose one.
DROP INDEX UQ_products_barcode ON products;
CREATE UNIQUE INDEX UQ_products_barcode ON products(tenant_id, barcode) WHERE barcode IS NOT NULL;

ALTER TABLE receipts DROP CONSTRAINT UQ_receipts_receipt_no;
ALTER TABLE receipts ADD CONSTRAINT UQ_receipts_tenant_receipt_no UNIQUE (tenant_id, receipt_no);

ALTER TABLE putaway_tasks DROP CONSTRAINT UQ_putaway_tasks_putaway_no;
ALTER TABLE putaway_tasks ADD CONSTRAINT UQ_putaway_tasks_tenant_putaway_no UNIQUE (tenant_id, putaway_no);

ALTER TABLE picking_tasks DROP CONSTRAINT UQ_picking_tasks_pick_no;
ALTER TABLE picking_tasks ADD CONSTRAINT UQ_picking_tasks_tenant_pick_no UNIQUE (tenant_id, pick_no);

ALTER TABLE shipments DROP CONSTRAINT UQ_shipments_shipment_no;
ALTER TABLE shipments ADD CONSTRAINT UQ_shipments_tenant_shipment_no UNIQUE (tenant_id, shipment_no);

ALTER TABLE orders DROP CONSTRAINT UQ_orders_order_no;
ALTER TABLE orders ADD CONSTRAINT UQ_orders_tenant_order_no UNIQUE (tenant_id, order_no);

ALTER TABLE orders DROP CONSTRAINT UQ_orders_external_reference;
ALTER TABLE orders ADD CONSTRAINT UQ_orders_tenant_external_reference UNIQUE (tenant_id, external_reference);

-- Correction vs. tenant-er-design.md §3 — see file header.
ALTER TABLE stock_adjustments DROP CONSTRAINT UQ_stock_adjustments_adjustment_no;
ALTER TABLE stock_adjustments ADD CONSTRAINT UQ_stock_adjustments_tenant_adjustment_no UNIQUE (tenant_id, adjustment_no);

-- Correction vs. tenant-er-design.md §3 — see file header.
ALTER TABLE stock_counts DROP CONSTRAINT UQ_stock_counts_count_no;
ALTER TABLE stock_counts ADD CONSTRAINT UQ_stock_counts_tenant_count_no UNIQUE (tenant_id, count_no);

-- Correction vs. tenant-er-design.md §3 — see file header.
ALTER TABLE replenishment_tasks DROP CONSTRAINT UQ_replenishment_tasks_task_no;
ALTER TABLE replenishment_tasks ADD CONSTRAINT UQ_replenishment_tasks_tenant_task_no UNIQUE (tenant_id, task_no);
