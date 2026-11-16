-- Phase 33C.1 -- Batch Picking / Wave Management. picking_batches is
-- the consolidation header: a set of RELEASED, PICK_BY_PRODUCT orders
-- (see picking_batch_orders, migrations/0095) consolidated by
-- product/lot/location into one real picking_tasks row at Confirm.
--
-- Status machine: DRAFT (the consolidation preview -- lines and
-- allocations already computed and persisted, see picking_batch_
-- allocations/0096, but no picking_tasks row exists yet and no stock is
-- reserved) -> CONFIRMED (a real picking_tasks row now exists, created
-- via internal/picking's own unmodified Service.CreateTx, and execution
-- proceeds entirely through internal/picking's own engine from this
-- point on) -> COMPLETED/CANCELLED (mirrors the underlying task's own
-- terminal state -- written by internal/pickingbatch's own Service at
-- read time when it observes the linked task has reached COMPLETED/
-- SHORT_PICK/CANCELLED; never a cross-package DB trigger).
--
-- picking_task_id is nullable (NULL until Confirm) and, once set, is
-- unique: exactly one batch may ever claim a given picking task. Needs
-- a FILTERED unique index, not a plain UNIQUE constraint -- see
-- migrations/0060's own doc on why a plain UNIQUE constraint would
-- treat every other NULL (every still-DRAFT batch) as a duplicate under
-- SQL Server's non-ANSI NULL-uniqueness behavior. Added in its own
-- migration (0098) alongside the FK, after this table and orders.
-- picking_batch_id both exist -- see that file's own doc.
CREATE TABLE picking_batches (
    id               BIGINT        IDENTITY(1,1) PRIMARY KEY,
    tenant_id        BIGINT        NOT NULL,
    batch_no         VARCHAR(20)   NOT NULL,
    warehouse_id     BIGINT        NOT NULL,
    status           VARCHAR(20)   NOT NULL CONSTRAINT DF_picking_batches_status DEFAULT ('DRAFT'),
    picking_task_id  BIGINT        NULL,
    created_by       BIGINT        NULL,
    created_at       DATETIME2     NOT NULL CONSTRAINT DF_picking_batches_created_at DEFAULT (SYSUTCDATETIME()),
    updated_at       DATETIME2     NOT NULL CONSTRAINT DF_picking_batches_updated_at DEFAULT (SYSUTCDATETIME()),
    confirmed_at     DATETIME2     NULL,
    cancelled_at     DATETIME2     NULL,
    cancelled_by     BIGINT        NULL,
    cancel_remark    NVARCHAR(500) NULL,
    CONSTRAINT FK_picking_batches_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id),
    CONSTRAINT FK_picking_batches_warehouse FOREIGN KEY (warehouse_id) REFERENCES warehouses(id),
    CONSTRAINT FK_picking_batches_created_by FOREIGN KEY (created_by) REFERENCES users(id),
    CONSTRAINT FK_picking_batches_cancelled_by FOREIGN KEY (cancelled_by) REFERENCES users(id),
    CONSTRAINT CK_picking_batches_status CHECK (status IN ('DRAFT', 'CONFIRMED', 'COMPLETED', 'CANCELLED'))
);

CREATE INDEX IX_picking_batches_tenant_id ON picking_batches(tenant_id);
CREATE INDEX IX_picking_batches_warehouse_id ON picking_batches(warehouse_id);
CREATE INDEX IX_picking_batches_status ON picking_batches(status);
