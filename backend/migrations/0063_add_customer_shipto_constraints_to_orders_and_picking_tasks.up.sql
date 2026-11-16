-- Constraints for migrations/0062's new columns — kept in its own
-- migration (own transaction, runs after 0062 has already committed)
-- specifically to avoid the ADD-COLUMN-then-ADD-CONSTRAINT-in-one-batch
-- bug migrations/0050/0053 each noted.
ALTER TABLE orders ADD CONSTRAINT FK_orders_customer FOREIGN KEY (customer_id) REFERENCES customers(id);
ALTER TABLE orders ADD CONSTRAINT FK_orders_ship_to FOREIGN KEY (ship_to_id) REFERENCES customer_delivery_locations(id);
CREATE INDEX IX_orders_customer_id ON orders(customer_id);
CREATE INDEX IX_orders_ship_to_id ON orders(ship_to_id);

ALTER TABLE picking_tasks ADD CONSTRAINT FK_picking_tasks_customer FOREIGN KEY (customer_id) REFERENCES customers(id);
ALTER TABLE picking_tasks ADD CONSTRAINT FK_picking_tasks_ship_to FOREIGN KEY (ship_to_id) REFERENCES customer_delivery_locations(id);
CREATE INDEX IX_picking_tasks_customer_id ON picking_tasks(customer_id);
CREATE INDEX IX_picking_tasks_ship_to_id ON picking_tasks(ship_to_id);
