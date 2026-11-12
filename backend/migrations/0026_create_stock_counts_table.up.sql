-- Stock count header, matching CheckStockView.vue's "Count Information"
-- panel. Unlike every other module, this screen has no list-of-past-
-- counts table at all — it is a single, perpetual working page. "+ New
-- Count Task" is made real here (it was a bare alert() in the mock)
-- by actually creating a header row; the frontend then works against
-- that real count's id instead of a hardcoded stockMaster array.
--
-- zone_id is nullable ("All Zone" is a valid selection in the mock);
-- warehouse_id is required, matching confirmCount()'s own client-side
-- validation ("Warehouse is required").
--
-- count_date is a plain DATE, matching the mock's date input exactly —
-- unlike stock_adjustments, this screen does have a real date field.
CREATE TABLE stock_counts (
    id           BIGINT      IDENTITY(1,1) NOT NULL,
    count_no     VARCHAR(30) NOT NULL,
    count_type   VARCHAR(20) NOT NULL,
    warehouse_id BIGINT      NOT NULL,
    zone_id      BIGINT      NULL,
    count_date   DATE        NULL,
    status       VARCHAR(20) NOT NULL CONSTRAINT DF_stock_counts_status DEFAULT ('DRAFT'),
    created_by   BIGINT      NULL,
    created_at   DATETIME2   NOT NULL CONSTRAINT DF_stock_counts_created_at DEFAULT (SYSUTCDATETIME()),
    updated_at   DATETIME2   NOT NULL CONSTRAINT DF_stock_counts_updated_at DEFAULT (SYSUTCDATETIME()),
    confirmed_at DATETIME2   NULL,
    CONSTRAINT PK_stock_counts PRIMARY KEY (id),
    CONSTRAINT UQ_stock_counts_count_no UNIQUE (count_no),
    CONSTRAINT FK_stock_counts_warehouse FOREIGN KEY (warehouse_id) REFERENCES warehouses(id),
    CONSTRAINT FK_stock_counts_zone FOREIGN KEY (zone_id) REFERENCES zones(id),
    CONSTRAINT FK_stock_counts_created_by FOREIGN KEY (created_by) REFERENCES users(id),
    CONSTRAINT CK_stock_counts_type CHECK (count_type IN
        ('CYCLE_COUNT', 'FULL_COUNT', 'LOCATION_COUNT', 'SKU_COUNT')),
    -- No separate "in progress" state: a count is DRAFT while items are
    -- still being added/scanned/edited (the mock's own bottom buttons —
    -- Save Draft, Create Adjustment, Confirm Count — are all available
    -- simultaneously at every point, with no explicit "start counting"
    -- transition), and CONFIRMED once Confirm Count has applied every
    -- line's variance through Inventory Core.
    CONSTRAINT CK_stock_counts_status CHECK (status IN ('DRAFT', 'CONFIRMED'))
);

CREATE INDEX IX_stock_counts_warehouse_id ON stock_counts(warehouse_id);
CREATE INDEX IX_stock_counts_status ON stock_counts(status);
