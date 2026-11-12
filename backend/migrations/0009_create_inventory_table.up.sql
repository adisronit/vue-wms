-- Current stock balance, matching StockInquiryView.vue's table/detail
-- drawer (available/reserved/on-hand qty, lot, expiry, status).
--
-- Key design decision (see PROJECT_STATUS.md for full reasoning):
-- balance is keyed by (product_id, location_id, lot_no), NOT just
-- (product_id, location_id). lot_no is NOT NULL DEFAULT '' rather than
-- nullable: a nullable lot_no would let SQL Server's unique-index NULL
-- semantics (learned the hard way with products.barcode in migration
-- 0008) silently allow duplicate "no-lot" balance rows for the same
-- product+location instead of merging into one row. '' uniformly means
-- "this product isn't lot-tracked" for products with lot_required=false.
--
-- expiry_date is an attribute of the row, not part of the key — a given
-- lot has one expiry in practice, so it doesn't need to be a separate
-- identity dimension.
--
-- Serial-level (per-unit) tracking is deliberately NOT modeled here:
-- no existing frontend view displays individual serial numbers anywhere
-- (Product Master's serial_required is a flag with no consuming UI yet).
-- Deferred to whichever future phase (Receiving/Picking) actually needs
-- unit-level records.
--
-- reserved_quantity and on_hold exist so the API can express the
-- frontend's Available/Reserved/On Hand distinction and its
-- AVAILABLE/RESERVED/HOLD/LOW_STOCK status, but neither is written by
-- any endpoint this phase (no reservation or hold workflow exists yet)
-- — both simply default to 0/false on every row this phase creates.
--
-- quantity uses DECIMAL, not INT: base_uom includes KG, a weight unit
-- where fractional quantities are realistic even though no mock example
-- shows one.
CREATE TABLE inventory (
    id                 BIGINT IDENTITY(1,1) NOT NULL,
    product_id         BIGINT         NOT NULL,
    location_id        BIGINT         NOT NULL,
    lot_no             VARCHAR(50)    NOT NULL CONSTRAINT DF_inventory_lot_no DEFAULT (''),
    expiry_date        DATE           NULL,
    quantity           DECIMAL(18,4)  NOT NULL CONSTRAINT DF_inventory_quantity DEFAULT (0),
    reserved_quantity  DECIMAL(18,4)  NOT NULL CONSTRAINT DF_inventory_reserved_quantity DEFAULT (0),
    on_hold            BIT            NOT NULL CONSTRAINT DF_inventory_on_hold DEFAULT (0),
    created_at         DATETIME2      NOT NULL CONSTRAINT DF_inventory_created_at DEFAULT (SYSUTCDATETIME()),
    updated_at         DATETIME2      NOT NULL CONSTRAINT DF_inventory_updated_at DEFAULT (SYSUTCDATETIME()),
    CONSTRAINT PK_inventory PRIMARY KEY (id),
    CONSTRAINT FK_inventory_product FOREIGN KEY (product_id) REFERENCES products(id),
    CONSTRAINT FK_inventory_location FOREIGN KEY (location_id) REFERENCES locations(id),
    CONSTRAINT UQ_inventory_product_location_lot UNIQUE (product_id, location_id, lot_no),
    -- Enforced here as a hard backstop, not just in application code —
    -- available inventory (quantity - reserved_quantity) must never go
    -- negative, and reserved can never exceed what's actually on hand.
    CONSTRAINT CK_inventory_quantity_non_negative CHECK (quantity >= 0),
    CONSTRAINT CK_inventory_reserved_within_quantity CHECK (reserved_quantity >= 0 AND reserved_quantity <= quantity)
);

CREATE INDEX IX_inventory_product_id ON inventory(product_id);
CREATE INDEX IX_inventory_location_id ON inventory(location_id);
