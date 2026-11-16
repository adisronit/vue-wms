-- Phase 33C — Packing lines: one row per picking_task_item being
-- packed, created once (at job-creation time) from that line's own
-- authoritative PickedQty — ordered_qty/picked_qty are a frozen,
-- read-only-after-insert snapshot copied verbatim from
-- picking_task_items.ordered_qty/picked_qty, the exact same "copy once,
-- never re-derive, never re-sync" convention shipment_items.qty
-- (migrations/0021) already established for the same source column.
-- Packing is explicitly forbidden from re-deriving or duplicating
-- PickedQty — everything in this package treats this row's own
-- picked_qty as the one authoritative "how much may ever be packed for
-- this line" ceiling, not a live join back to picking_task_items.
--
-- packed_qty is cumulative across every pack action for this line
-- (potentially split across multiple cartons — see carton_contents),
-- the same convention picking_task_items.picked_qty/relocation_task_items.
-- moved_qty established, written via an atomic guarded UPDATE (see
-- internal/packing/repository.go's AdjustPackedQty) rather than a Go
-- read-modify-write. CK_packing_job_items_packed_qty is the
-- database-level backstop for "packed quantity per line can never
-- exceed picked quantity" (overpacking) — belt-and-suspenders on top of
-- the guarded UPDATE's own WHERE-clause bound, matching this codebase's
-- existing pattern of a CHECK constraint backing an application-level
-- guard rather than replacing it.
--
-- exception_flag/exception_reason/exception_qty record a damage/
-- shortage exception raised during packing — deliberately separate from
-- picking_task_items.short_qty/short_reason (migrations/0084): a
-- picking-stage short pick and a packing-stage damage/shortage
-- discovery are different events at different workflow stages, and
-- conflating them would lose which stage actually found the problem.
-- status EXCEPTION is terminal for a line regardless of packed_qty,
-- mirroring picking_task_items' own SHORT-is-terminal-regardless-of-
-- picked_qty precedent (see internal/picking/model.go's itemStatus doc).
--
-- UQ on (tenant_id, picking_task_item_id) additionally enforces "at
-- most one packing_job_item per picking line, ever" as a database
-- constraint, not just a consequence of packing_jobs' own one-job-per-
-- task UNIQUE.
CREATE TABLE packing_job_items (
    id                      BIGINT        IDENTITY(1,1) NOT NULL,
    tenant_id               BIGINT        NOT NULL,
    packing_job_id          BIGINT        NOT NULL,
    picking_task_item_id    BIGINT        NOT NULL,
    product_id              BIGINT        NOT NULL,
    lot_no                  VARCHAR(50)   NOT NULL CONSTRAINT DF_packing_job_items_lot_no DEFAULT (''),
    ordered_qty             DECIMAL(18,4) NOT NULL,
    picked_qty              DECIMAL(18,4) NOT NULL,
    packed_qty              DECIMAL(18,4) NOT NULL CONSTRAINT DF_packing_job_items_packed_qty DEFAULT (0),
    uom                     VARCHAR(20)   NOT NULL,
    status                  VARCHAR(20)   NOT NULL CONSTRAINT DF_packing_job_items_status DEFAULT ('PENDING'),
    exception_flag          BIT           NOT NULL CONSTRAINT DF_packing_job_items_exception_flag DEFAULT (0),
    exception_reason        VARCHAR(30)   NULL,
    exception_qty           DECIMAL(18,4) NULL,
    remark                  NVARCHAR(500) NULL,
    created_at              DATETIME2     NOT NULL CONSTRAINT DF_packing_job_items_created_at DEFAULT (SYSUTCDATETIME()),
    updated_at              DATETIME2     NOT NULL CONSTRAINT DF_packing_job_items_updated_at DEFAULT (SYSUTCDATETIME()),
    CONSTRAINT PK_packing_job_items PRIMARY KEY (id),
    CONSTRAINT UQ_packing_job_items_tenant_picking_item UNIQUE (tenant_id, picking_task_item_id),
    CONSTRAINT FK_packing_job_items_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id),
    CONSTRAINT FK_packing_job_items_job FOREIGN KEY (packing_job_id) REFERENCES packing_jobs(id),
    CONSTRAINT FK_packing_job_items_picking_item FOREIGN KEY (picking_task_item_id) REFERENCES picking_task_items(id),
    CONSTRAINT FK_packing_job_items_product FOREIGN KEY (product_id) REFERENCES products(id),
    CONSTRAINT CK_packing_job_items_status CHECK (status IN ('PENDING', 'PARTIALLY_PACKED', 'PACKED', 'EXCEPTION')),
    CONSTRAINT CK_packing_job_items_exception_reason CHECK (exception_reason IS NULL OR exception_reason IN
        ('DAMAGED', 'SHORTAGE', 'WRONG_ITEM', 'OTHER')),
    CONSTRAINT CK_packing_job_items_picked_qty CHECK (picked_qty >= 0),
    CONSTRAINT CK_packing_job_items_packed_qty CHECK (packed_qty >= 0 AND packed_qty <= picked_qty),
    CONSTRAINT CK_packing_job_items_exception_qty CHECK (exception_qty IS NULL OR exception_qty >= 0)
);

CREATE INDEX IX_packing_job_items_tenant_id ON packing_job_items(tenant_id);
CREATE INDEX IX_packing_job_items_job_id ON packing_job_items(packing_job_id);
