-- Phase 33A — constraints for migrations/0079/0080's new columns, split
-- into its own transaction per migrations/0053's own ADD-COLUMN-then-
-- ADD-CONSTRAINT-in-one-batch doc (an ADD CONSTRAINT referencing a
-- column added earlier in the SAME un-GO'd batch fails with "Invalid
-- column name" against SQL Server; this file runs as its own batch,
-- after 0079/0080 have already committed).
--
-- CK_orders_status is redefined (DROP + ADD) to admit the new RELEASED
-- state — PENDING -> RELEASED -> PICKING -> CANCELLED — a column that
-- already existed long before this migration, not one added in this
-- same batch, so this redefinition carries none of the risk above (the
-- same reasoning migrations/0074's own doc gives for widening
-- CK_inventory_transactions_type).
ALTER TABLE orders DROP CONSTRAINT CK_orders_status;
ALTER TABLE orders ADD CONSTRAINT CK_orders_status CHECK (status IN ('PENDING', 'RELEASED', 'PICKING', 'CANCELLED'));

ALTER TABLE orders ADD CONSTRAINT CK_orders_order_type CHECK (order_type IN ('SALES_ORDER', 'INTERNAL_TRANSFER'));
ALTER TABLE orders ADD CONSTRAINT CK_orders_priority CHECK (priority IN ('LOW', 'NORMAL', 'HIGH'));
ALTER TABLE orders ADD CONSTRAINT FK_orders_released_by FOREIGN KEY (released_by) REFERENCES users(id);

ALTER TABLE order_items ADD CONSTRAINT CK_order_items_shortfall_qty CHECK (shortfall_qty IS NULL OR shortfall_qty >= 0);

CREATE INDEX IX_orders_order_type ON orders(order_type);
