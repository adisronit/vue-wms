DROP INDEX IX_picking_tasks_ship_to_id ON picking_tasks;
DROP INDEX IX_picking_tasks_customer_id ON picking_tasks;
ALTER TABLE picking_tasks DROP CONSTRAINT FK_picking_tasks_ship_to;
ALTER TABLE picking_tasks DROP CONSTRAINT FK_picking_tasks_customer;

DROP INDEX IX_orders_ship_to_id ON orders;
DROP INDEX IX_orders_customer_id ON orders;
ALTER TABLE orders DROP CONSTRAINT FK_orders_ship_to;
ALTER TABLE orders DROP CONSTRAINT FK_orders_customer;
