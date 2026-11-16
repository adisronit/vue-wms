DROP INDEX UQ_picking_batches_picking_task_id ON picking_batches;

DROP INDEX IX_orders_picking_batch_id ON orders;
ALTER TABLE orders DROP CONSTRAINT FK_orders_picking_batch;
