-- Receiving line items, matching NewReceivingView.vue's item table
-- (SKU, Expected Qty, Receive Qty, UOM, Lot No., Expiry Date, Location).
--
-- received_qty is CUMULATIVE across every confirm call for this line
-- (a receipt can be confirmed more than once — partial receiving today,
-- the rest tomorrow), not "this batch's" quantity — that per-call delta
-- only exists in the POST .../confirm request body, never stored on its
-- own.
--
-- No CHECK(received_qty <= expected_qty): the create form's own
-- Variance display colors a positive variance (over-receipt) green, not
-- as an error, so over-receipt is treated as a supported business case,
-- not rejected.
--
-- lot_no is NOT NULL DEFAULT '' — deliberately matching Inventory
-- Core's exact convention (migrations/0009) so a receipt line's lot_no
-- can be passed straight through to AddStock without translation.
CREATE TABLE receipt_items (
    id            BIGINT        IDENTITY(1,1) NOT NULL,
    receipt_id    BIGINT        NOT NULL,
    product_id    BIGINT        NOT NULL,
    expected_qty  DECIMAL(18,4) NOT NULL CONSTRAINT DF_receipt_items_expected_qty DEFAULT (0),
    received_qty  DECIMAL(18,4) NOT NULL CONSTRAINT DF_receipt_items_received_qty DEFAULT (0),
    uom           VARCHAR(20)   NOT NULL,
    lot_no        VARCHAR(50)   NOT NULL CONSTRAINT DF_receipt_items_lot_no DEFAULT (''),
    expiry_date   DATE          NULL,
    location_id   BIGINT        NOT NULL,
    status        VARCHAR(20)   NOT NULL CONSTRAINT DF_receipt_items_status DEFAULT ('PENDING'),
    created_at    DATETIME2     NOT NULL CONSTRAINT DF_receipt_items_created_at DEFAULT (SYSUTCDATETIME()),
    updated_at    DATETIME2     NOT NULL CONSTRAINT DF_receipt_items_updated_at DEFAULT (SYSUTCDATETIME()),
    CONSTRAINT PK_receipt_items PRIMARY KEY (id),
    CONSTRAINT FK_receipt_items_receipt FOREIGN KEY (receipt_id) REFERENCES receipts(id),
    CONSTRAINT FK_receipt_items_product FOREIGN KEY (product_id) REFERENCES products(id),
    CONSTRAINT FK_receipt_items_location FOREIGN KEY (location_id) REFERENCES locations(id),
    CONSTRAINT CK_receipt_items_expected_qty_non_negative CHECK (expected_qty >= 0),
    CONSTRAINT CK_receipt_items_received_qty_non_negative CHECK (received_qty >= 0),
    CONSTRAINT CK_receipt_items_status CHECK (status IN ('PENDING', 'PARTIALLY_RECEIVED', 'COMPLETED'))
);

CREATE INDEX IX_receipt_items_receipt_id ON receipt_items(receipt_id);
CREATE INDEX IX_receipt_items_product_id ON receipt_items(product_id);
