-- Stock count lines, matching CheckStockView.vue's items table (SKU/
-- Product/Location/Lot/System Qty/Count Qty/Variance/UOM/Condition/
-- Status/Remark).
--
-- system_qty is captured ONCE, when the line is added to the count
-- (from Inventory Core's live balance at that moment), and is NEVER
-- refreshed afterward — the task's own explicit "do not recompute the
-- historical expected quantity after the count is created" requirement.
-- count_qty starts NULL (not yet counted) and is set once the operator
-- scans or types a quantity; variance_qty is computed and stored at
-- that same moment (count_qty - system_qty) — stored explicitly for
-- traceability rather than left as a query-time computation, per the
-- task's own "expected_qty, counted_qty, variance_qty... must be
-- traceable" requirement (contrast with internal/inventory's `status`,
-- which IS deliberately query-time-only — see that package's own doc
-- for why the two cases differ: inventory status is a live business
-- classification that should never go stale, whereas a count's
-- variance is a historical fact about one specific count event and
-- must NOT silently change if the item is looked at again later).
--
-- condition (GOOD/DAMAGED/HOLD) and status (PENDING/COUNTED/RECOUNT)
-- match the mock's own two independent <select> columns exactly.
CREATE TABLE stock_count_items (
    id             BIGINT        IDENTITY(1,1) NOT NULL,
    stock_count_id BIGINT        NOT NULL,
    product_id     BIGINT        NOT NULL,
    location_id    BIGINT        NOT NULL,
    lot_no         VARCHAR(50)   NOT NULL CONSTRAINT DF_stock_count_items_lot_no DEFAULT (''),
    uom            VARCHAR(20)   NOT NULL,
    system_qty     DECIMAL(18,4) NOT NULL,
    count_qty      DECIMAL(18,4) NULL,
    variance_qty   DECIMAL(18,4) NULL,
    condition      VARCHAR(10)   NOT NULL CONSTRAINT DF_stock_count_items_condition DEFAULT ('GOOD'),
    status         VARCHAR(10)   NOT NULL CONSTRAINT DF_stock_count_items_status DEFAULT ('PENDING'),
    remark         NVARCHAR(500) NULL,
    created_at     DATETIME2     NOT NULL CONSTRAINT DF_stock_count_items_created_at DEFAULT (SYSUTCDATETIME()),
    updated_at     DATETIME2     NOT NULL CONSTRAINT DF_stock_count_items_updated_at DEFAULT (SYSUTCDATETIME()),
    CONSTRAINT PK_stock_count_items PRIMARY KEY (id),
    CONSTRAINT FK_stock_count_items_count FOREIGN KEY (stock_count_id) REFERENCES stock_counts(id),
    CONSTRAINT FK_stock_count_items_product FOREIGN KEY (product_id) REFERENCES products(id),
    CONSTRAINT FK_stock_count_items_location FOREIGN KEY (location_id) REFERENCES locations(id),
    CONSTRAINT CK_stock_count_items_condition CHECK (condition IN ('GOOD', 'DAMAGED', 'HOLD')),
    CONSTRAINT CK_stock_count_items_status CHECK (status IN ('PENDING', 'COUNTED', 'RECOUNT'))
);

CREATE INDEX IX_stock_count_items_count_id ON stock_count_items(stock_count_id);
