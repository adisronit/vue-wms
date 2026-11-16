-- Reverses 0060. Only safe while at most one order in the whole table
-- has a NULL picking_task_id at the moment this runs — restoring the
-- original plain UNIQUE constraint fails with a duplicate-key error if
-- two or more orders are PENDING/unlinked, the same caveat 0048's own
-- down migration documents for its own constraint-narrowing steps. This
-- is not a new limitation introduced here: it is exactly the bug 0060
-- fixes, reintroduced on purpose by rolling back.
DROP INDEX UQ_orders_picking_task_id ON orders;
ALTER TABLE orders ADD CONSTRAINT UQ_orders_picking_task_id UNIQUE (picking_task_id);
