-- Constraints for migrations/0099's new columns, split into their own
-- transaction per migrations/0092's own ADD-COLUMN-then-ADD-CONSTRAINT
-- doc (0099 has already committed by the time this file runs).
ALTER TABLE picking_batch_allocations ADD CONSTRAINT CK_picking_batch_allocations_sorted_qty CHECK (sorted_qty >= 0 AND sorted_qty <= allocated_qty);
ALTER TABLE picking_batch_allocations ADD CONSTRAINT CK_picking_batch_allocations_shortage_qty CHECK (shortage_qty IS NULL OR shortage_qty >= 0);

ALTER TABLE picking_batches ADD CONSTRAINT CK_picking_batches_sortation_status CHECK (sortation_status IN ('NOT_STARTED', 'IN_PROGRESS', 'COMPLETED'));
ALTER TABLE picking_batches ADD CONSTRAINT FK_picking_batches_sortation_assigned_to FOREIGN KEY (sortation_assigned_to) REFERENCES users(id);

CREATE INDEX IX_picking_batches_sortation_status ON picking_batches(sortation_status);
