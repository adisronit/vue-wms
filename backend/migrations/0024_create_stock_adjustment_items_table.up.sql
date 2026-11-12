-- Stock adjustment line — modeled as a child table (matching this
-- codebase's header+items convention throughout) even though the
-- current frontend's create form only ever produces exactly one line
-- per document (a single SKU/Location/Adjust Qty set, no "+ Add Item"
-- button anywhere on this screen) — the same "build the real shape,
-- not a shortcut" precedent as every prior phase's schema.
--
-- before_qty is a display snapshot only (captured at create time, for
-- the mock's own "Current Qty" column) — it is NEVER used to compute
-- the actual mutation; adjust_qty is the sole authoritative delta,
-- applied via AddStockTx/RemoveStockTx's own atomic guarded UPDATE
-- against whatever the live balance actually is at approval time (see
-- internal/stockadjustment/service.go's Approve doc). after_qty is
-- populated from AddStockTx/RemoveStockTx's own returned balance at
-- approval time — the mock's "New Qty" column, but the REAL post-
-- mutation value, not a naive current+delta calculation that could
-- drift from reality under concurrent stock movement.
--
-- reason is CHECK-constrained to the mock's own 5 <option> values
-- verbatim (human-readable strings, not synthesized ALLCAPS codes) —
-- per the task's own "do not invent codes if the frontend already
-- defines them" instruction.
--
-- lot_no uses the same NOT NULL DEFAULT '' pattern as every other
-- line-item table in this codebase.
CREATE TABLE stock_adjustment_items (
    id                  BIGINT        IDENTITY(1,1) NOT NULL,
    stock_adjustment_id BIGINT        NOT NULL,
    product_id          BIGINT        NOT NULL,
    location_id         BIGINT        NOT NULL,
    lot_no              VARCHAR(50)   NOT NULL CONSTRAINT DF_stock_adjustment_items_lot_no DEFAULT (''),
    adjust_type         VARCHAR(10)   NOT NULL,
    adjust_qty          DECIMAL(18,4) NOT NULL,
    before_qty          DECIMAL(18,4) NULL,
    after_qty           DECIMAL(18,4) NULL,
    reason              NVARCHAR(50)  NOT NULL,
    remark              NVARCHAR(500) NULL,
    created_at          DATETIME2     NOT NULL CONSTRAINT DF_stock_adjustment_items_created_at DEFAULT (SYSUTCDATETIME()),
    updated_at          DATETIME2     NOT NULL CONSTRAINT DF_stock_adjustment_items_updated_at DEFAULT (SYSUTCDATETIME()),
    CONSTRAINT PK_stock_adjustment_items PRIMARY KEY (id),
    CONSTRAINT FK_stock_adjustment_items_adjustment FOREIGN KEY (stock_adjustment_id) REFERENCES stock_adjustments(id),
    CONSTRAINT FK_stock_adjustment_items_product FOREIGN KEY (product_id) REFERENCES products(id),
    CONSTRAINT FK_stock_adjustment_items_location FOREIGN KEY (location_id) REFERENCES locations(id),
    CONSTRAINT CK_stock_adjustment_items_type CHECK (adjust_type IN ('INCREASE', 'DECREASE')),
    CONSTRAINT CK_stock_adjustment_items_qty CHECK (adjust_qty > 0),
    CONSTRAINT CK_stock_adjustment_items_reason CHECK (reason IN
        ('Cycle Count Variance', 'Damaged Goods', 'Lost Goods', 'Found Goods', 'System Correction'))
);

CREATE INDEX IX_stock_adjustment_items_adjustment_id ON stock_adjustment_items(stock_adjustment_id);
