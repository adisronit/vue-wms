-- Constraint for migrations/0092's new column, split into its own
-- transaction per migrations/0053's own ADD-COLUMN-then-ADD-CONSTRAINT
-- doc (0092 has already committed by the time this file runs).
ALTER TABLE orders ADD CONSTRAINT CK_orders_picking_mode CHECK (picking_mode IN ('PICK_BY_ORDER', 'PICK_BY_PRODUCT'));

CREATE INDEX IX_orders_picking_mode ON orders(picking_mode);
