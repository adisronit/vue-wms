-- Phase 32 — Stock Relocation lines: one row per (product, lot,
-- source location, destination location) the task intends to move.
-- Mirrors putaway_task_items' shape (migrations/0015) closely —
-- requested_qty/moved_qty/uom/status, moved_qty cumulative across every
-- confirm call for this line, not one batch's delta.
--
-- lpn (pallet/license-plate number) is a new column with no existing
-- precedent in this schema — it is pure descriptive/audit metadata for
-- the Transfer Note PDF and operator display, never consulted by
-- inventory.MoveAvailableStockTx (which has no LPN concept at the
-- balance level); the inventory table is not changed by this
-- migration.
--
-- expiry_date is deliberately NOT stored here, for the same reason
-- putaway_task_items omits it (see migrations/0015's own doc): the
-- moved lot's expiry must remain whatever the source inventory balance
-- row already has, always read live at confirm time, never carried as
-- a second, potentially-stale copy on the task line. The API still
-- surfaces it for display by joining to the live inventory balance.
--
-- lot_no uses the same NOT NULL DEFAULT '' pattern as inventory.lot_no/
-- putaway_task_items.lot_no (migrations/0009/0015) rather than
-- nullable, for the same SQL Server one-NULL-per-table uniqueness
-- reason documented there (not applicable to a non-unique column here,
-- but kept consistent so '' uniformly means "not lot-tracked" project-
-- wide).
CREATE TABLE relocation_task_items (
    id                        BIGINT        IDENTITY(1,1) NOT NULL,
    tenant_id                 BIGINT        NOT NULL,
    relocation_task_id        BIGINT        NOT NULL,
    product_id                BIGINT        NOT NULL,
    lot_no                     VARCHAR(50)   NOT NULL CONSTRAINT DF_relocation_task_items_lot_no DEFAULT (''),
    lpn                        VARCHAR(50)   NULL,
    source_location_id         BIGINT        NOT NULL,
    destination_location_id    BIGINT        NOT NULL,
    requested_qty               DECIMAL(18,4) NOT NULL,
    moved_qty                   DECIMAL(18,4) NOT NULL CONSTRAINT DF_relocation_task_items_moved_qty DEFAULT (0),
    uom                          VARCHAR(20)   NOT NULL,
    status                       VARCHAR(20)   NOT NULL CONSTRAINT DF_relocation_task_items_status DEFAULT ('PENDING'),
    remark                       NVARCHAR(500) NULL,
    created_at                   DATETIME2     NOT NULL CONSTRAINT DF_relocation_task_items_created_at DEFAULT (SYSUTCDATETIME()),
    updated_at                   DATETIME2     NOT NULL CONSTRAINT DF_relocation_task_items_updated_at DEFAULT (SYSUTCDATETIME()),
    CONSTRAINT PK_relocation_task_items PRIMARY KEY (id),
    CONSTRAINT FK_relocation_task_items_task FOREIGN KEY (relocation_task_id) REFERENCES relocation_tasks(id),
    CONSTRAINT FK_relocation_task_items_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id),
    CONSTRAINT FK_relocation_task_items_product FOREIGN KEY (product_id) REFERENCES products(id),
    CONSTRAINT FK_relocation_task_items_source_location FOREIGN KEY (source_location_id) REFERENCES locations(id),
    CONSTRAINT FK_relocation_task_items_destination_location FOREIGN KEY (destination_location_id) REFERENCES locations(id),
    CONSTRAINT CK_relocation_task_items_status CHECK (status IN
        ('PENDING', 'PARTIALLY_COMPLETED', 'COMPLETED')),
    CONSTRAINT CK_relocation_task_items_requested_qty CHECK (requested_qty > 0),
    CONSTRAINT CK_relocation_task_items_moved_qty CHECK (moved_qty >= 0)
);

CREATE INDEX IX_relocation_task_items_tenant_id ON relocation_task_items(tenant_id);
CREATE INDEX IX_relocation_task_items_task_id ON relocation_task_items(relocation_task_id);
