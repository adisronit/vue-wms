-- Order Management — Phase 25. A lightweight header+lines layer that
-- orchestrates an existing Picking task rather than ever touching
-- inventory itself: orders.picking_task_id links to exactly one
-- picking_tasks row (UNIQUE — one order becomes one picking task in
-- this phase's scope), created via internal/picking's own
-- Service.Create so Picking retains sole ownership of reservation
-- (ReserveStockTx) and deduction (ConsumeReservationTx/RemoveStockTx).
-- Order Management never writes inventory or inventory_transactions.
--
-- status is intentionally only PENDING/PICKING/CANCELLED — Order's own
-- three explicit actions (Create, CreatePickingTask, Cancel). The
-- richer PICKED/SHIPPING/DELIVERED states are NOT stored here: they
-- are derived at read time from the linked picking task's and (if any)
-- shipment's own live status, the same "derived, never stored, can
-- never drift" principle internal/inventory's own Status already
-- established for LOW_STOCK/RESERVED/etc.
CREATE TABLE orders (
    id                 BIGINT        IDENTITY(1,1) NOT NULL,
    order_no           VARCHAR(30)   NOT NULL,
    customer           NVARCHAR(150) NULL,
    -- Nullable + unique: most orders in this phase have no external
    -- system yet, but when one does, this is the idempotency key a
    -- future ERP integration re-POSTs safely against (CLAUDE.md's own
    -- "external_reference for future ERP idempotency" instruction) — a
    -- duplicate external_reference fails the UNIQUE constraint instead
    -- of silently creating a second order for the same upstream event.
    external_reference VARCHAR(100)  NULL,
    warehouse_id       BIGINT        NOT NULL,
    picking_task_id    BIGINT        NULL,
    status             VARCHAR(20)   NOT NULL CONSTRAINT DF_orders_status DEFAULT ('PENDING'),
    remark             NVARCHAR(500) NULL,
    created_by         BIGINT        NULL,
    created_at         DATETIME2     NOT NULL CONSTRAINT DF_orders_created_at DEFAULT (SYSUTCDATETIME()),
    updated_at         DATETIME2     NOT NULL CONSTRAINT DF_orders_updated_at DEFAULT (SYSUTCDATETIME()),
    cancelled_at       DATETIME2     NULL,
    cancelled_by       BIGINT        NULL,
    cancel_remark      NVARCHAR(500) NULL,
    CONSTRAINT PK_orders PRIMARY KEY (id),
    CONSTRAINT UQ_orders_order_no UNIQUE (order_no),
    CONSTRAINT UQ_orders_external_reference UNIQUE (external_reference),
    CONSTRAINT UQ_orders_picking_task_id UNIQUE (picking_task_id),
    CONSTRAINT FK_orders_warehouse FOREIGN KEY (warehouse_id) REFERENCES warehouses(id),
    CONSTRAINT FK_orders_picking_task FOREIGN KEY (picking_task_id) REFERENCES picking_tasks(id),
    CONSTRAINT FK_orders_created_by FOREIGN KEY (created_by) REFERENCES users(id),
    CONSTRAINT FK_orders_cancelled_by FOREIGN KEY (cancelled_by) REFERENCES users(id),
    CONSTRAINT CK_orders_status CHECK (status IN ('PENDING', 'PICKING', 'CANCELLED'))
);

CREATE INDEX IX_orders_warehouse_id ON orders(warehouse_id);
CREATE INDEX IX_orders_status ON orders(status);

-- order_items is what a picked-task-creation call is built from
-- (product + ordered_qty only — no location/lot: Order Management is
-- location-agnostic, exactly like a real sales order; Picking is where
-- a location/lot gets chosen, same as every other phase's own
-- division of concerns).
CREATE TABLE order_items (
    id           BIGINT        IDENTITY(1,1) NOT NULL,
    order_id     BIGINT        NOT NULL,
    product_id   BIGINT        NOT NULL,
    ordered_qty  DECIMAL(18,4) NOT NULL,
    uom          VARCHAR(20)   NOT NULL,
    CONSTRAINT PK_order_items PRIMARY KEY (id),
    CONSTRAINT FK_order_items_order FOREIGN KEY (order_id) REFERENCES orders(id),
    CONSTRAINT FK_order_items_product FOREIGN KEY (product_id) REFERENCES products(id),
    CONSTRAINT CK_order_items_ordered_qty_positive CHECK (ordered_qty > 0)
);

CREATE INDEX IX_order_items_order_id ON order_items(order_id);
