-- Phase 33E.1 -- constraints for migrations/0113's new columns, split
-- into its own transaction per migrations/0053's own ADD-COLUMN-then-
-- ADD-CONSTRAINT-in-one-batch doc (an ADD CONSTRAINT referencing a
-- column added earlier in the SAME un-GO'd batch fails with "Invalid
-- column name" against SQL Server; this file runs as its own batch,
-- after 0113 has already committed).
--
-- CK_orders_destination_warehouse_requires_transfer is the database-
-- enforced half of Service.Create/Update's own validateTransferFields:
-- destination_warehouse_id must be set for an INTERNAL_TRANSFER order
-- and must be NULL for anything else (today, only SALES_ORDER) -- the
-- same "explicit rejection, never silently ignored" decision the
-- service layer makes, backed by a real constraint so a future direct-
-- SQL write (or a service-layer bug) can never desync the two order
-- shapes.
--
-- CK_orders_destination_warehouse_not_source rejects
-- destination_warehouse_id = warehouse_id whenever destination_
-- warehouse_id is set -- a transfer can never name the same warehouse
-- as both its own source and destination.
--
-- FK_orders_destination_warehouse_id ties destination_warehouse_id to
-- warehouses(id), the same FK shape orders.warehouse_id itself would
-- have if Phase 3 had added one -- see Service.validateTransferFields'
-- own doc for why cross-TENANT destination ownership is additionally
-- verified at the service layer (a FK alone cannot express "same
-- tenant as the order").
ALTER TABLE orders ADD CONSTRAINT CK_orders_destination_warehouse_requires_transfer
    CHECK (
        (order_type = 'INTERNAL_TRANSFER' AND destination_warehouse_id IS NOT NULL)
        OR (order_type <> 'INTERNAL_TRANSFER' AND destination_warehouse_id IS NULL)
    );

ALTER TABLE orders ADD CONSTRAINT CK_orders_destination_warehouse_not_source
    CHECK (destination_warehouse_id IS NULL OR destination_warehouse_id <> warehouse_id);

ALTER TABLE orders ADD CONSTRAINT FK_orders_destination_warehouse_id
    FOREIGN KEY (destination_warehouse_id) REFERENCES warehouses(id);

CREATE INDEX IX_orders_destination_warehouse_id ON orders(destination_warehouse_id);
