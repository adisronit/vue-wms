-- Picking task lines, matching PickingView.vue's "Picking Items" cards
-- in the detail drawer (sku, productName, location, orderQty,
-- pickedQty, plus the per-line "Confirm Pick"/"Short Pick" actions).
--
-- location_id is the SOURCE location to pick FROM (where the stock
-- currently sits) — the mock's read-only "Location: {{ product.location }}"
-- display, not a destination the way putaway_task_items has one, since
-- picking only ever removes stock (Confirm calls
-- inventory.RemoveStockTx, never MoveStockTx/AddStockTx — see
-- internal/picking/service.go).
--
-- lot_no uses the same NOT NULL DEFAULT '' pattern as receipt_items/
-- putaway_task_items (migrations/0012, 0015) rather than nullable, for
-- the same SQL Server one-NULL-per-table uniqueness reason documented
-- there — a lot-tracked product's picking line must identify which lot
-- it's drawing down, matching inventory's own (product, location, lot)
-- balance identity (migrations/0009).
--
-- picked_qty is cumulative across every confirm call for this line, not
-- one batch's delta — the same convention receipt_items.received_qty
-- and putaway_task_items.moved_qty established. Written via an atomic
-- guarded UPDATE from the start (internal/picking/repository.go's
-- AdjustPickedQty) rather than a Go read-modify-write, per this phase's
-- own fix for the lost-update race found on those two sibling columns —
-- see PROJECT_STATUS.md.
CREATE TABLE picking_task_items (
    id              BIGINT        IDENTITY(1,1) NOT NULL,
    picking_task_id BIGINT        NOT NULL,
    product_id      BIGINT        NOT NULL,
    lot_no          VARCHAR(50)   NOT NULL CONSTRAINT DF_picking_task_items_lot_no DEFAULT (''),
    location_id     BIGINT        NOT NULL,
    ordered_qty     DECIMAL(18,4) NOT NULL,
    picked_qty      DECIMAL(18,4) NOT NULL CONSTRAINT DF_picking_task_items_picked_qty DEFAULT (0),
    uom             VARCHAR(20)   NOT NULL,
    status          VARCHAR(20)   NOT NULL CONSTRAINT DF_picking_task_items_status DEFAULT ('PENDING'),
    remark          NVARCHAR(500) NULL,
    created_at      DATETIME2     NOT NULL CONSTRAINT DF_picking_task_items_created_at DEFAULT (SYSUTCDATETIME()),
    updated_at      DATETIME2     NOT NULL CONSTRAINT DF_picking_task_items_updated_at DEFAULT (SYSUTCDATETIME()),
    CONSTRAINT PK_picking_task_items PRIMARY KEY (id),
    CONSTRAINT FK_picking_task_items_task FOREIGN KEY (picking_task_id) REFERENCES picking_tasks(id),
    CONSTRAINT FK_picking_task_items_product FOREIGN KEY (product_id) REFERENCES products(id),
    CONSTRAINT FK_picking_task_items_location FOREIGN KEY (location_id) REFERENCES locations(id),
    CONSTRAINT CK_picking_task_items_status CHECK (status IN
        ('PENDING', 'PARTIALLY_PICKED', 'COMPLETED', 'SHORT')),
    CONSTRAINT CK_picking_task_items_ordered_qty CHECK (ordered_qty > 0)
);

CREATE INDEX IX_picking_task_items_task_id ON picking_task_items(picking_task_id);
CREATE INDEX IX_picking_task_items_location_id ON picking_task_items(location_id);
