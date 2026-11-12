ALTER TABLE receipts DROP COLUMN tenant_id;
ALTER TABLE receipt_items DROP COLUMN tenant_id;
ALTER TABLE receipt_confirmations DROP COLUMN tenant_id;

ALTER TABLE putaway_tasks DROP COLUMN tenant_id;
ALTER TABLE putaway_task_items DROP COLUMN tenant_id;
ALTER TABLE putaway_confirmations DROP COLUMN tenant_id;

ALTER TABLE picking_tasks DROP COLUMN tenant_id;
ALTER TABLE picking_task_items DROP COLUMN tenant_id;
ALTER TABLE picking_confirmations DROP COLUMN tenant_id;

ALTER TABLE shipments DROP COLUMN tenant_id;
ALTER TABLE shipment_items DROP COLUMN tenant_id;
ALTER TABLE shipment_confirmations DROP COLUMN tenant_id;

ALTER TABLE orders DROP COLUMN tenant_id;
ALTER TABLE order_items DROP COLUMN tenant_id;
