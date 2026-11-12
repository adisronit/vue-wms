-- Putaway task lines, matching NewPutawayView.vue's items table and
-- PutawayView.vue's list rows (each list row is one item, joined with
-- its task header — there is no separate item-list drawer, unlike
-- Receiving).
--
-- receipt_item_id is a NULLABLE FK to receipt_items(id): set when this
-- line was loaded from a real completed receipt line via "Source
-- Document" (used to compute how much of that line is still eligible
-- for putaway — internal/putaway's GET /putaway/sources/{receiptId}),
-- left NULL for a line added via "+ Add Item" or the scan bar with no
-- source document (real behavior in NewPutawayView.vue — addItem()/
-- scanItem() both push a line with no receiving reference at all).
--
-- No suggested_bin column: no bin-suggestion algorithm exists anywhere
-- in this codebase or was asked for, and the mock's "Suggested Bin"
-- values are hardcoded strings with nothing computing them. Rather than
-- fabricate a suggestion engine, destination_location_id is the one
-- real location value this line has — it is what the frontend's
-- "Suggested Bin"/"Actual Bin" columns both end up displaying (see
-- internal/putaway/repository.go's list query doc for exactly how).
--
-- expiry_date is deliberately NOT stored here: the moved lot's expiry
-- must remain unchanged by definition (see Confirm's design doc in
-- internal/putaway/service.go), so it is always read from the source
-- inventory balance row at confirm time, never carried as a second,
-- potentially-stale copy on the task line.
--
-- lot_no uses the same NOT NULL DEFAULT '' pattern as receipt_items.lot_no
-- (migrations/0012) rather than nullable, for the same SQL Server
-- one-NULL-per-table uniqueness reason documented there.
CREATE TABLE putaway_task_items (
    id                       BIGINT        IDENTITY(1,1) NOT NULL,
    putaway_task_id          BIGINT        NOT NULL,
    receipt_item_id          BIGINT        NULL,
    product_id               BIGINT        NOT NULL,
    lot_no                   VARCHAR(50)   NOT NULL CONSTRAINT DF_putaway_task_items_lot_no DEFAULT (''),
    source_location_id       BIGINT        NOT NULL,
    destination_location_id  BIGINT        NOT NULL,
    requested_qty            DECIMAL(18,4) NOT NULL,
    moved_qty                DECIMAL(18,4) NOT NULL CONSTRAINT DF_putaway_task_items_moved_qty DEFAULT (0),
    uom                      VARCHAR(20)   NOT NULL,
    status                   VARCHAR(20)   NOT NULL CONSTRAINT DF_putaway_task_items_status DEFAULT ('PENDING'),
    remark                   NVARCHAR(500) NULL,
    created_at               DATETIME2     NOT NULL CONSTRAINT DF_putaway_task_items_created_at DEFAULT (SYSUTCDATETIME()),
    updated_at               DATETIME2     NOT NULL CONSTRAINT DF_putaway_task_items_updated_at DEFAULT (SYSUTCDATETIME()),
    CONSTRAINT PK_putaway_task_items PRIMARY KEY (id),
    CONSTRAINT FK_putaway_task_items_task FOREIGN KEY (putaway_task_id) REFERENCES putaway_tasks(id),
    CONSTRAINT FK_putaway_task_items_receipt_item FOREIGN KEY (receipt_item_id) REFERENCES receipt_items(id),
    CONSTRAINT FK_putaway_task_items_product FOREIGN KEY (product_id) REFERENCES products(id),
    CONSTRAINT FK_putaway_task_items_source_location FOREIGN KEY (source_location_id) REFERENCES locations(id),
    CONSTRAINT FK_putaway_task_items_destination_location FOREIGN KEY (destination_location_id) REFERENCES locations(id),
    CONSTRAINT CK_putaway_task_items_status CHECK (status IN
        ('PENDING', 'PARTIALLY_COMPLETED', 'COMPLETED', 'ISSUE')),
    CONSTRAINT CK_putaway_task_items_requested_qty CHECK (requested_qty > 0)
);

CREATE INDEX IX_putaway_task_items_task_id ON putaway_task_items(putaway_task_id);
CREATE INDEX IX_putaway_task_items_receipt_item_id ON putaway_task_items(receipt_item_id);
