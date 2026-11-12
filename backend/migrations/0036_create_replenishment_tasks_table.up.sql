-- Replenishment task — Phase 24. Deliberately single-line (product +
-- source location + destination location + quantity directly on the
-- header, no items child table): unlike Receiving/Picking/Putaway
-- (which exist to move a whole multi-product document through a
-- workflow), a replenishment task is inherently one product moving
-- from one bulk/reserve location to one pick-face location — "no
-- complex optimizer" per this phase's own scope, matching Move's own
-- existing (product, from, to, qty) shape in internal/inventory's
-- MoveStockTx.
--
-- lot_no/quantity/status/cancellation columns mirror the exact
-- conventions already established by picking_tasks (migrations/0017)
-- and its own Phase 18 cancellation-audit columns (migrations/0032),
-- reused verbatim rather than reinvented.
CREATE TABLE replenishment_tasks (
    id                BIGINT        IDENTITY(1,1) NOT NULL,
    task_no           VARCHAR(30)   NOT NULL,
    product_id        BIGINT        NOT NULL,
    from_location_id  BIGINT        NOT NULL,
    to_location_id    BIGINT        NOT NULL,
    lot_no            VARCHAR(50)   NOT NULL CONSTRAINT DF_replenishment_tasks_lot_no DEFAULT (''),
    quantity          DECIMAL(18,4) NOT NULL,
    warehouse_id      BIGINT        NOT NULL,
    status            VARCHAR(20)   NOT NULL CONSTRAINT DF_replenishment_tasks_status DEFAULT ('PENDING'),
    remark            NVARCHAR(500) NULL,
    issue_remark      NVARCHAR(500) NULL,
    created_by        BIGINT        NULL,
    created_at        DATETIME2     NOT NULL CONSTRAINT DF_replenishment_tasks_created_at DEFAULT (SYSUTCDATETIME()),
    updated_at        DATETIME2     NOT NULL CONSTRAINT DF_replenishment_tasks_updated_at DEFAULT (SYSUTCDATETIME()),
    started_at        DATETIME2     NULL,
    completed_at      DATETIME2     NULL,
    cancelled_at      DATETIME2     NULL,
    cancelled_by      BIGINT        NULL,
    cancel_remark     NVARCHAR(500) NULL,
    CONSTRAINT PK_replenishment_tasks PRIMARY KEY (id),
    CONSTRAINT UQ_replenishment_tasks_task_no UNIQUE (task_no),
    CONSTRAINT FK_replenishment_tasks_product FOREIGN KEY (product_id) REFERENCES products(id),
    CONSTRAINT FK_replenishment_tasks_from_location FOREIGN KEY (from_location_id) REFERENCES locations(id),
    CONSTRAINT FK_replenishment_tasks_to_location FOREIGN KEY (to_location_id) REFERENCES locations(id),
    CONSTRAINT FK_replenishment_tasks_warehouse FOREIGN KEY (warehouse_id) REFERENCES warehouses(id),
    CONSTRAINT FK_replenishment_tasks_created_by FOREIGN KEY (created_by) REFERENCES users(id),
    CONSTRAINT FK_replenishment_tasks_cancelled_by FOREIGN KEY (cancelled_by) REFERENCES users(id),
    CONSTRAINT CK_replenishment_tasks_status CHECK (status IN
        ('PENDING', 'IN_PROGRESS', 'COMPLETED', 'CANCELLED', 'ISSUE')),
    CONSTRAINT CK_replenishment_tasks_quantity_positive CHECK (quantity > 0),
    CONSTRAINT CK_replenishment_tasks_locations_differ CHECK (from_location_id <> to_location_id)
);

CREATE INDEX IX_replenishment_tasks_warehouse_id ON replenishment_tasks(warehouse_id);
CREATE INDEX IX_replenishment_tasks_status ON replenishment_tasks(status);
CREATE INDEX IX_replenishment_tasks_product_id ON replenishment_tasks(product_id);
