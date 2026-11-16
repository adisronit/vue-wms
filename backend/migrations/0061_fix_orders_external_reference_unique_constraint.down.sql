-- Reverses 0061. Only safe while at most one order per tenant has a
-- NULL external_reference at the moment this runs — restoring the
-- original plain composite UNIQUE constraint fails with a duplicate-key
-- error if any tenant has two or more orders with no external_reference,
-- the same caveat 0048's and 0060's own down migrations document for
-- their own constraint-narrowing steps. This is not a new limitation
-- introduced here: it is exactly the bug 0061 fixes, reintroduced on
-- purpose by rolling back.
DROP INDEX UQ_orders_tenant_external_reference ON orders;
ALTER TABLE orders ADD CONSTRAINT UQ_orders_tenant_external_reference UNIQUE (tenant_id, external_reference);
