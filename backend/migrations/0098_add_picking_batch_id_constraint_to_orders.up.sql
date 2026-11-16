-- Constraint for migrations/0097's new column, split into its own
-- transaction (0097 has already committed by the time this file runs) --
-- see migrations/0092's own doc for why.
--
-- Also adds picking_batches.picking_task_id's own filtered unique index
-- here (same migration, no ordering dependency on orders.
-- picking_batch_id) -- see migrations/0094's own doc on why a plain
-- UNIQUE constraint would be wrong for a column that is NULL for every
-- still-DRAFT batch.
ALTER TABLE orders ADD CONSTRAINT FK_orders_picking_batch FOREIGN KEY (picking_batch_id) REFERENCES picking_batches(id);
CREATE INDEX IX_orders_picking_batch_id ON orders(picking_batch_id);

CREATE UNIQUE INDEX UQ_picking_batches_picking_task_id ON picking_batches(picking_task_id) WHERE picking_task_id IS NOT NULL;
