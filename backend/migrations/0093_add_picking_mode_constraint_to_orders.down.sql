DROP INDEX IX_orders_picking_mode ON orders;
ALTER TABLE orders DROP CONSTRAINT CK_orders_picking_mode;
