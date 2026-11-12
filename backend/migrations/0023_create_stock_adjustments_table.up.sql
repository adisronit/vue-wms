-- Stock adjustment header, matching StockAdjustmentView.vue's list table
-- and its single list+drawer screen (there is no separate "New
-- Adjustment" view — NewStockAdjustmentView.vue exists as a file but is
-- an empty, unrouted orphan, confirmed by grepping router/index.js).
--
-- No date column: the create form has no date input at all (unlike
-- Receiving/Putaway, which do) — the mock's list "Date" column is
-- simply the row's own creation date, derived from created_at at read
-- time rather than duplicated into its own column.
CREATE TABLE stock_adjustments (
    id            BIGINT      IDENTITY(1,1) NOT NULL,
    adjustment_no VARCHAR(30) NOT NULL,
    status        VARCHAR(20) NOT NULL CONSTRAINT DF_stock_adjustments_status DEFAULT ('DRAFT'),
    created_by    BIGINT      NULL,
    approved_by   BIGINT      NULL,
    created_at    DATETIME2   NOT NULL CONSTRAINT DF_stock_adjustments_created_at DEFAULT (SYSUTCDATETIME()),
    updated_at    DATETIME2   NOT NULL CONSTRAINT DF_stock_adjustments_updated_at DEFAULT (SYSUTCDATETIME()),
    approved_at   DATETIME2   NULL,
    CONSTRAINT PK_stock_adjustments PRIMARY KEY (id),
    CONSTRAINT UQ_stock_adjustments_adjustment_no UNIQUE (adjustment_no),
    CONSTRAINT FK_stock_adjustments_created_by FOREIGN KEY (created_by) REFERENCES users(id),
    CONSTRAINT FK_stock_adjustments_approved_by FOREIGN KEY (approved_by) REFERENCES users(id),
    -- Matches the mock's own status filter exactly: Draft / Pending
    -- Approval / Approved / Rejected — no separate "COMPLETED" stage,
    -- since approval itself is the one and only inventory-mutating
    -- step (see internal/stockadjustment's doc) — unlike Receiving/
    -- Putaway/Picking, which separate "confirm exists" from "confirm
    -- moves stock" because those can be called multiple times for
    -- partial quantities; an adjustment is a single atomic correction
    -- with no partial-application concept the frontend ever models.
    CONSTRAINT CK_stock_adjustments_status CHECK (status IN
        ('DRAFT', 'PENDING', 'APPROVED', 'REJECTED'))
);

CREATE INDEX IX_stock_adjustments_status ON stock_adjustments(status);
