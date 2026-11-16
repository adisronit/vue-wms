ALTER TABLE picking_batches DROP COLUMN sortation_completed_at;
ALTER TABLE picking_batches DROP COLUMN sortation_started_at;
ALTER TABLE picking_batches DROP COLUMN sortation_assigned_to;
ALTER TABLE picking_batches DROP COLUMN sortation_station;
ALTER TABLE picking_batches DROP CONSTRAINT DF_picking_batches_sortation_status;
ALTER TABLE picking_batches DROP COLUMN sortation_status;

ALTER TABLE picking_batch_allocations DROP COLUMN exception_reason;
ALTER TABLE picking_batch_allocations DROP COLUMN shortage_qty;
ALTER TABLE picking_batch_allocations DROP CONSTRAINT DF_picking_batch_allocations_sorted_qty;
ALTER TABLE picking_batch_allocations DROP COLUMN sorted_qty;
