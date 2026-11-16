DROP INDEX IX_orders_order_type ON orders;

ALTER TABLE order_items DROP CONSTRAINT CK_order_items_shortfall_qty;

ALTER TABLE orders DROP CONSTRAINT FK_orders_released_by;
ALTER TABLE orders DROP CONSTRAINT CK_orders_priority;
ALTER TABLE orders DROP CONSTRAINT CK_orders_order_type;

ALTER TABLE orders DROP CONSTRAINT CK_orders_status;
ALTER TABLE orders ADD CONSTRAINT CK_orders_status CHECK (status IN ('PENDING', 'PICKING', 'CANCELLED'));
