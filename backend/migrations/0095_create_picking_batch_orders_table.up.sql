-- Phase 33C.1 -- the set of orders consolidated into one picking_batches
-- header. A row here is inserted at DRAFT creation time already (not
-- deferred to Confirm) -- that is what reserves an order against being
-- added to a SECOND concurrent DRAFT batch: internal/pickingbatch's own
-- Service checks "is this order_id already linked to a batch whose
-- status is DRAFT or CONFIRMED" via this table before creating a new
-- DRAFT, not via a column on orders itself (orders.picking_batch_id,
-- migrations/0097, is only ever written at Confirm -- see that file's
-- own doc).
--
-- No unique constraint on order_id alone: a cancelled batch's own rows
-- stay here as history, and that same order_id must be eligible to join
-- a brand new batch afterward (exactly the "cancelled batches don't
-- lock the order forever" rule). UNIQUE(batch_id, order_id) still
-- guards against the same order being added to the very same batch
-- twice.
CREATE TABLE picking_batch_orders (
    id         BIGINT    IDENTITY(1,1) PRIMARY KEY,
    tenant_id  BIGINT    NOT NULL,
    batch_id   BIGINT    NOT NULL,
    order_id   BIGINT    NOT NULL,
    created_at DATETIME2 NOT NULL CONSTRAINT DF_picking_batch_orders_created_at DEFAULT (SYSUTCDATETIME()),
    CONSTRAINT FK_picking_batch_orders_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id),
    CONSTRAINT FK_picking_batch_orders_batch FOREIGN KEY (batch_id) REFERENCES picking_batches(id),
    CONSTRAINT FK_picking_batch_orders_order FOREIGN KEY (order_id) REFERENCES orders(id),
    CONSTRAINT UQ_picking_batch_orders_batch_order UNIQUE (batch_id, order_id)
);

CREATE INDEX IX_picking_batch_orders_tenant_id ON picking_batch_orders(tenant_id);
CREATE INDEX IX_picking_batch_orders_order_id ON picking_batch_orders(order_id);
