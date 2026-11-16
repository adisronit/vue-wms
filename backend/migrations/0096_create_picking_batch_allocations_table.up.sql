-- Phase 33C.1 -- the critical traceability table: which share of a
-- consolidated (product, lot, location) picking demand belongs to which
-- original order/order line. One row per (order_item, lot, location)
-- slice -- a single order's line can legitimately produce more than one
-- row here if its own share had to be split across multiple FEFO lots
-- to be covered (see internal/pickingbatch's own allocation-splitting
-- doc), and multiple order lines always collapse onto the same
-- (product_id, lot_no, location_id) tuple when they share it, which is
-- exactly the consolidation this phase exists to produce.
--
-- lot_no/location_id are populated at DRAFT creation time already (the
-- FEFO suggestion), before any real picking_tasks row exists --
-- allocated_qty/lot_no/location_id together are what Confirm sums back
-- into picking_task_items' own ordered_qty per tuple. picking_task_item_id
-- stays NULL until Confirm backfills it (see Service.Confirm's own doc)
-- once the real picking_task_items rows exist and each one's own
-- (product_id, lot_no, location_id) tuple can be matched back to the
-- allocation rows that share it.
CREATE TABLE picking_batch_allocations (
    id                   BIGINT         IDENTITY(1,1) PRIMARY KEY,
    tenant_id            BIGINT         NOT NULL,
    batch_id             BIGINT         NOT NULL,
    order_id             BIGINT         NOT NULL,
    order_item_id        BIGINT         NOT NULL,
    product_id           BIGINT         NOT NULL,
    lot_no               VARCHAR(50)    NOT NULL CONSTRAINT DF_picking_batch_allocations_lot_no DEFAULT (''),
    location_id          BIGINT         NOT NULL,
    picking_task_item_id BIGINT         NULL,
    allocated_qty        DECIMAL(18,4)  NOT NULL,
    uom                  VARCHAR(20)    NOT NULL,
    created_at           DATETIME2      NOT NULL CONSTRAINT DF_picking_batch_allocations_created_at DEFAULT (SYSUTCDATETIME()),
    CONSTRAINT FK_picking_batch_allocations_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id),
    CONSTRAINT FK_picking_batch_allocations_batch FOREIGN KEY (batch_id) REFERENCES picking_batches(id),
    CONSTRAINT FK_picking_batch_allocations_order FOREIGN KEY (order_id) REFERENCES orders(id),
    CONSTRAINT FK_picking_batch_allocations_order_item FOREIGN KEY (order_item_id) REFERENCES order_items(id),
    CONSTRAINT FK_picking_batch_allocations_product FOREIGN KEY (product_id) REFERENCES products(id),
    CONSTRAINT FK_picking_batch_allocations_location FOREIGN KEY (location_id) REFERENCES locations(id),
    CONSTRAINT FK_picking_batch_allocations_task_item FOREIGN KEY (picking_task_item_id) REFERENCES picking_task_items(id),
    CONSTRAINT CK_picking_batch_allocations_qty CHECK (allocated_qty > 0)
);

CREATE INDEX IX_picking_batch_allocations_tenant_id ON picking_batch_allocations(tenant_id);
CREATE INDEX IX_picking_batch_allocations_batch_id ON picking_batch_allocations(batch_id);
CREATE INDEX IX_picking_batch_allocations_order_id ON picking_batch_allocations(order_id);
CREATE INDEX IX_picking_batch_allocations_order_item_id ON picking_batch_allocations(order_item_id);
CREATE INDEX IX_picking_batch_allocations_task_item_id ON picking_batch_allocations(picking_task_item_id);
