ALTER TABLE shipments DROP CONSTRAINT FK_shipments_cancelled_by;
ALTER TABLE shipments DROP COLUMN cancel_remark;
ALTER TABLE shipments DROP COLUMN cancelled_by;
ALTER TABLE shipments DROP COLUMN cancelled_at;

ALTER TABLE putaway_tasks DROP CONSTRAINT FK_putaway_tasks_cancelled_by;
ALTER TABLE putaway_tasks DROP COLUMN cancel_remark;
ALTER TABLE putaway_tasks DROP COLUMN cancelled_by;
ALTER TABLE putaway_tasks DROP COLUMN cancelled_at;

ALTER TABLE receipts DROP CONSTRAINT FK_receipts_cancelled_by;
ALTER TABLE receipts DROP COLUMN cancel_remark;
ALTER TABLE receipts DROP COLUMN cancelled_by;
ALTER TABLE receipts DROP COLUMN cancelled_at;

ALTER TABLE picking_tasks DROP CONSTRAINT FK_picking_tasks_cancelled_by;
ALTER TABLE picking_tasks DROP COLUMN cancel_remark;
ALTER TABLE picking_tasks DROP COLUMN cancelled_by;
ALTER TABLE picking_tasks DROP COLUMN cancelled_at;
