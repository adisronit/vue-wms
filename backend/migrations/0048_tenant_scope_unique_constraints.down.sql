-- Reverses 0048. Only safe while every row still belongs to a single
-- tenant (true in the disposable test environment this was verified
-- against); if a second tenant's data has been created in the
-- meantime, restoring the original single-column constraint can fail
-- with a duplicate-key error — this is the same caveat
-- migration-plan.md §3e/§7 documents for its own equivalent step, not a
-- new limitation introduced here.
ALTER TABLE replenishment_tasks DROP CONSTRAINT UQ_replenishment_tasks_tenant_task_no;
ALTER TABLE replenishment_tasks ADD CONSTRAINT UQ_replenishment_tasks_task_no UNIQUE (task_no);

ALTER TABLE stock_counts DROP CONSTRAINT UQ_stock_counts_tenant_count_no;
ALTER TABLE stock_counts ADD CONSTRAINT UQ_stock_counts_count_no UNIQUE (count_no);

ALTER TABLE stock_adjustments DROP CONSTRAINT UQ_stock_adjustments_tenant_adjustment_no;
ALTER TABLE stock_adjustments ADD CONSTRAINT UQ_stock_adjustments_adjustment_no UNIQUE (adjustment_no);

ALTER TABLE orders DROP CONSTRAINT UQ_orders_tenant_external_reference;
ALTER TABLE orders ADD CONSTRAINT UQ_orders_external_reference UNIQUE (external_reference);

ALTER TABLE orders DROP CONSTRAINT UQ_orders_tenant_order_no;
ALTER TABLE orders ADD CONSTRAINT UQ_orders_order_no UNIQUE (order_no);

ALTER TABLE shipments DROP CONSTRAINT UQ_shipments_tenant_shipment_no;
ALTER TABLE shipments ADD CONSTRAINT UQ_shipments_shipment_no UNIQUE (shipment_no);

ALTER TABLE picking_tasks DROP CONSTRAINT UQ_picking_tasks_tenant_pick_no;
ALTER TABLE picking_tasks ADD CONSTRAINT UQ_picking_tasks_pick_no UNIQUE (pick_no);

ALTER TABLE putaway_tasks DROP CONSTRAINT UQ_putaway_tasks_tenant_putaway_no;
ALTER TABLE putaway_tasks ADD CONSTRAINT UQ_putaway_tasks_putaway_no UNIQUE (putaway_no);

ALTER TABLE receipts DROP CONSTRAINT UQ_receipts_tenant_receipt_no;
ALTER TABLE receipts ADD CONSTRAINT UQ_receipts_receipt_no UNIQUE (receipt_no);

DROP INDEX UQ_products_barcode ON products;
CREATE UNIQUE INDEX UQ_products_barcode ON products(barcode) WHERE barcode IS NOT NULL;

ALTER TABLE products DROP CONSTRAINT UQ_products_tenant_sku;
ALTER TABLE products ADD CONSTRAINT UQ_products_sku UNIQUE (sku);

ALTER TABLE locations DROP CONSTRAINT UQ_locations_tenant_zone_code;
ALTER TABLE locations ADD CONSTRAINT UQ_locations_zone_code UNIQUE (zone_id, code);

ALTER TABLE zones DROP CONSTRAINT UQ_zones_tenant_warehouse_code;
ALTER TABLE zones ADD CONSTRAINT UQ_zones_warehouse_code UNIQUE (warehouse_id, code);

ALTER TABLE warehouses DROP CONSTRAINT UQ_warehouses_tenant_code;
ALTER TABLE warehouses ADD CONSTRAINT UQ_warehouses_code UNIQUE (code);

ALTER TABLE roles DROP CONSTRAINT UQ_roles_tenant_code;
ALTER TABLE roles ADD CONSTRAINT UQ_roles_code UNIQUE (code);
