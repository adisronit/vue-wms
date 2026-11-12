-- Reverses 0047. Safe at any time on its own — data is untouched, only
-- nullability/default/FK/index metadata changes. The migrate tool only
-- ever rolls back the single most-recently-applied version, so this is
-- only reachable after 0048 (the unique-constraint swap) has already
-- been rolled back — at that point no constraint depends on tenant_id
-- anymore, so nothing blocks the ALTER COLUMN ... NULL below.
DROP INDEX IX_export_jobs_tenant_requested_by ON export_jobs;
ALTER TABLE export_jobs DROP CONSTRAINT FK_export_jobs_tenant;
ALTER TABLE export_jobs DROP CONSTRAINT DF_export_jobs_tenant_id;
ALTER TABLE export_jobs ALTER COLUMN tenant_id BIGINT NULL;

DROP INDEX IX_notifications_tenant_user ON notifications;
ALTER TABLE notifications DROP CONSTRAINT FK_notifications_tenant;
ALTER TABLE notifications DROP CONSTRAINT DF_notifications_tenant_id;
ALTER TABLE notifications ALTER COLUMN tenant_id BIGINT NULL;

DROP INDEX IX_replenishment_confirmations_tenant_id ON replenishment_confirmations;
ALTER TABLE replenishment_confirmations DROP CONSTRAINT FK_replenishment_confirmations_tenant;
ALTER TABLE replenishment_confirmations DROP CONSTRAINT DF_replenishment_confirmations_tenant_id;
ALTER TABLE replenishment_confirmations ALTER COLUMN tenant_id BIGINT NULL;

DROP INDEX IX_replenishment_tasks_tenant_warehouse ON replenishment_tasks;
ALTER TABLE replenishment_tasks DROP CONSTRAINT FK_replenishment_tasks_tenant;
ALTER TABLE replenishment_tasks DROP CONSTRAINT DF_replenishment_tasks_tenant_id;
ALTER TABLE replenishment_tasks ALTER COLUMN tenant_id BIGINT NULL;

DROP INDEX IX_stock_count_confirmations_tenant_id ON stock_count_confirmations;
ALTER TABLE stock_count_confirmations DROP CONSTRAINT FK_stock_count_confirmations_tenant;
ALTER TABLE stock_count_confirmations DROP CONSTRAINT DF_stock_count_confirmations_tenant_id;
ALTER TABLE stock_count_confirmations ALTER COLUMN tenant_id BIGINT NULL;

DROP INDEX IX_stock_count_items_tenant_id ON stock_count_items;
ALTER TABLE stock_count_items DROP CONSTRAINT FK_stock_count_items_tenant;
ALTER TABLE stock_count_items DROP CONSTRAINT DF_stock_count_items_tenant_id;
ALTER TABLE stock_count_items ALTER COLUMN tenant_id BIGINT NULL;

DROP INDEX IX_stock_counts_tenant_status ON stock_counts;
ALTER TABLE stock_counts DROP CONSTRAINT FK_stock_counts_tenant;
ALTER TABLE stock_counts DROP CONSTRAINT DF_stock_counts_tenant_id;
ALTER TABLE stock_counts ALTER COLUMN tenant_id BIGINT NULL;

DROP INDEX IX_stock_adjustment_reminders_tenant_id ON stock_adjustment_reminders;
ALTER TABLE stock_adjustment_reminders DROP CONSTRAINT FK_stock_adjustment_reminders_tenant;
ALTER TABLE stock_adjustment_reminders DROP CONSTRAINT DF_stock_adjustment_reminders_tenant_id;
ALTER TABLE stock_adjustment_reminders ALTER COLUMN tenant_id BIGINT NULL;

DROP INDEX IX_stock_adjustment_confirmations_tenant_id ON stock_adjustment_confirmations;
ALTER TABLE stock_adjustment_confirmations DROP CONSTRAINT FK_stock_adjustment_confirmations_tenant;
ALTER TABLE stock_adjustment_confirmations DROP CONSTRAINT DF_stock_adjustment_confirmations_tenant_id;
ALTER TABLE stock_adjustment_confirmations ALTER COLUMN tenant_id BIGINT NULL;

DROP INDEX IX_stock_adjustment_items_tenant_id ON stock_adjustment_items;
ALTER TABLE stock_adjustment_items DROP CONSTRAINT FK_stock_adjustment_items_tenant;
ALTER TABLE stock_adjustment_items DROP CONSTRAINT DF_stock_adjustment_items_tenant_id;
ALTER TABLE stock_adjustment_items ALTER COLUMN tenant_id BIGINT NULL;

DROP INDEX IX_stock_adjustments_tenant_status ON stock_adjustments;
ALTER TABLE stock_adjustments DROP CONSTRAINT FK_stock_adjustments_tenant;
ALTER TABLE stock_adjustments DROP CONSTRAINT DF_stock_adjustments_tenant_id;
ALTER TABLE stock_adjustments ALTER COLUMN tenant_id BIGINT NULL;

DROP INDEX IX_order_items_tenant_id ON order_items;
ALTER TABLE order_items DROP CONSTRAINT FK_order_items_tenant;
ALTER TABLE order_items DROP CONSTRAINT DF_order_items_tenant_id;
ALTER TABLE order_items ALTER COLUMN tenant_id BIGINT NULL;

ALTER TABLE orders DROP CONSTRAINT FK_orders_tenant;
ALTER TABLE orders DROP CONSTRAINT DF_orders_tenant_id;
ALTER TABLE orders ALTER COLUMN tenant_id BIGINT NULL;

DROP INDEX IX_shipment_confirmations_tenant_id ON shipment_confirmations;
ALTER TABLE shipment_confirmations DROP CONSTRAINT FK_shipment_confirmations_tenant;
ALTER TABLE shipment_confirmations DROP CONSTRAINT DF_shipment_confirmations_tenant_id;
ALTER TABLE shipment_confirmations ALTER COLUMN tenant_id BIGINT NULL;

DROP INDEX IX_shipment_items_tenant_id ON shipment_items;
ALTER TABLE shipment_items DROP CONSTRAINT FK_shipment_items_tenant;
ALTER TABLE shipment_items DROP CONSTRAINT DF_shipment_items_tenant_id;
ALTER TABLE shipment_items ALTER COLUMN tenant_id BIGINT NULL;

ALTER TABLE shipments DROP CONSTRAINT FK_shipments_tenant;
ALTER TABLE shipments DROP CONSTRAINT DF_shipments_tenant_id;
ALTER TABLE shipments ALTER COLUMN tenant_id BIGINT NULL;

DROP INDEX IX_picking_confirmations_tenant_id ON picking_confirmations;
ALTER TABLE picking_confirmations DROP CONSTRAINT FK_picking_confirmations_tenant;
ALTER TABLE picking_confirmations DROP CONSTRAINT DF_picking_confirmations_tenant_id;
ALTER TABLE picking_confirmations ALTER COLUMN tenant_id BIGINT NULL;

DROP INDEX IX_picking_task_items_tenant_id ON picking_task_items;
ALTER TABLE picking_task_items DROP CONSTRAINT FK_picking_task_items_tenant;
ALTER TABLE picking_task_items DROP CONSTRAINT DF_picking_task_items_tenant_id;
ALTER TABLE picking_task_items ALTER COLUMN tenant_id BIGINT NULL;

ALTER TABLE picking_tasks DROP CONSTRAINT FK_picking_tasks_tenant;
ALTER TABLE picking_tasks DROP CONSTRAINT DF_picking_tasks_tenant_id;
ALTER TABLE picking_tasks ALTER COLUMN tenant_id BIGINT NULL;

DROP INDEX IX_putaway_confirmations_tenant_id ON putaway_confirmations;
ALTER TABLE putaway_confirmations DROP CONSTRAINT FK_putaway_confirmations_tenant;
ALTER TABLE putaway_confirmations DROP CONSTRAINT DF_putaway_confirmations_tenant_id;
ALTER TABLE putaway_confirmations ALTER COLUMN tenant_id BIGINT NULL;

DROP INDEX IX_putaway_task_items_tenant_id ON putaway_task_items;
ALTER TABLE putaway_task_items DROP CONSTRAINT FK_putaway_task_items_tenant;
ALTER TABLE putaway_task_items DROP CONSTRAINT DF_putaway_task_items_tenant_id;
ALTER TABLE putaway_task_items ALTER COLUMN tenant_id BIGINT NULL;

ALTER TABLE putaway_tasks DROP CONSTRAINT FK_putaway_tasks_tenant;
ALTER TABLE putaway_tasks DROP CONSTRAINT DF_putaway_tasks_tenant_id;
ALTER TABLE putaway_tasks ALTER COLUMN tenant_id BIGINT NULL;

DROP INDEX IX_receipt_confirmations_tenant_id ON receipt_confirmations;
ALTER TABLE receipt_confirmations DROP CONSTRAINT FK_receipt_confirmations_tenant;
ALTER TABLE receipt_confirmations DROP CONSTRAINT DF_receipt_confirmations_tenant_id;
ALTER TABLE receipt_confirmations ALTER COLUMN tenant_id BIGINT NULL;

DROP INDEX IX_receipt_items_tenant_id ON receipt_items;
ALTER TABLE receipt_items DROP CONSTRAINT FK_receipt_items_tenant;
ALTER TABLE receipt_items DROP CONSTRAINT DF_receipt_items_tenant_id;
ALTER TABLE receipt_items ALTER COLUMN tenant_id BIGINT NULL;

ALTER TABLE receipts DROP CONSTRAINT FK_receipts_tenant;
ALTER TABLE receipts DROP CONSTRAINT DF_receipts_tenant_id;
ALTER TABLE receipts ALTER COLUMN tenant_id BIGINT NULL;

DROP INDEX IX_inventory_transactions_tenant_created_at ON inventory_transactions;
ALTER TABLE inventory_transactions DROP CONSTRAINT FK_inventory_transactions_tenant;
ALTER TABLE inventory_transactions DROP CONSTRAINT DF_inventory_transactions_tenant_id;
ALTER TABLE inventory_transactions ALTER COLUMN tenant_id BIGINT NULL;

DROP INDEX IX_inventory_tenant_product_location_lot ON inventory;
ALTER TABLE inventory DROP CONSTRAINT FK_inventory_tenant;
ALTER TABLE inventory DROP CONSTRAINT DF_inventory_tenant_id;
ALTER TABLE inventory ALTER COLUMN tenant_id BIGINT NULL;

ALTER TABLE products DROP CONSTRAINT FK_products_tenant;
ALTER TABLE products DROP CONSTRAINT DF_products_tenant_id;
ALTER TABLE products ALTER COLUMN tenant_id BIGINT NULL;

ALTER TABLE locations DROP CONSTRAINT FK_locations_tenant;
ALTER TABLE locations DROP CONSTRAINT DF_locations_tenant_id;
ALTER TABLE locations ALTER COLUMN tenant_id BIGINT NULL;

ALTER TABLE zones DROP CONSTRAINT FK_zones_tenant;
ALTER TABLE zones DROP CONSTRAINT DF_zones_tenant_id;
ALTER TABLE zones ALTER COLUMN tenant_id BIGINT NULL;

ALTER TABLE warehouses DROP CONSTRAINT FK_warehouses_tenant;
ALTER TABLE warehouses DROP CONSTRAINT DF_warehouses_tenant_id;
ALTER TABLE warehouses ALTER COLUMN tenant_id BIGINT NULL;

ALTER TABLE roles DROP CONSTRAINT FK_roles_tenant;
ALTER TABLE roles DROP CONSTRAINT DF_roles_tenant_id;
ALTER TABLE roles ALTER COLUMN tenant_id BIGINT NULL;
