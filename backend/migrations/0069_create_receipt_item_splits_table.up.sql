-- Phase 31A — Receiving: receipt_item_splits is the additive, immutable
-- history of every lot/expiry/pallet split a receipt line was actually
-- confirmed with. receipt_items.lot_no/expiry_date (migrations/0012)
-- are deliberately left untouched and keep meaning exactly what they
-- always did — "the most recent confirm call's lot/expiry for this
-- line" — so every already-confirmed receipt displays identically to
-- before this migration. A NEW confirm call against a line now ALSO
-- inserts one split row per distinct lot/expiry/pallet batch supplied
-- in that call, in addition to updating the line's own lot_no/
-- expiry_date as before — see internal/receiving's Confirm doc. Rows
-- here are never updated or deleted once written (append-only, like
-- inventory_transactions).
--
-- One CREATE TABLE statement: every column and FK is new together, so
-- none of the ADD-COLUMN-then-ADD-CONSTRAINT batch-compilation risk
-- migrations/0050/0053/0065/0067 document applies — same reasoning as
-- the original receipts/receipt_items/receipt_confirmations tables
-- (migrations/0011-0013), each created in one statement with every
-- constraint inline.
--
-- tenant_id is denormalized onto this child table too (not inherited
-- only via receipt_item_id's FK), matching the precedent
-- migrations/0044 established for every other inbound/outbound child
-- table (receipt_items, picking_task_items, shipment_items, ...) —
-- avoids a join back up to the header for every tenant-scoped query.
-- receipt_id is also denormalized (not just receipt_item_id) for the
-- same reason ReceiptItem itself denormalizes receipt_id rather than
-- requiring a join through nothing else.
CREATE TABLE receipt_item_splits (
    id                 BIGINT        IDENTITY(1,1) NOT NULL,
    tenant_id          BIGINT        NOT NULL,
    receipt_id         BIGINT        NOT NULL,
    receipt_item_id    BIGINT        NOT NULL,
    lot_no             VARCHAR(50)   NOT NULL CONSTRAINT DF_receipt_item_splits_lot_no DEFAULT (''),
    mfg_date           DATE          NULL,
    expiry_date        DATE          NULL,
    quantity           DECIMAL(18,4) NOT NULL,
    uom                VARCHAR(20)   NOT NULL,
    pallet_id          VARCHAR(50)   NULL,
    carton_count       INT           NULL,
    location_id        BIGINT        NOT NULL,
    temperature_c      DECIMAL(5,2)  NULL,
    condition_note     VARCHAR(200)  NULL,
    country_of_origin  VARCHAR(100)  NULL,
    created_by         BIGINT        NULL,
    created_at         DATETIME2     NOT NULL CONSTRAINT DF_receipt_item_splits_created_at DEFAULT (SYSUTCDATETIME()),
    CONSTRAINT PK_receipt_item_splits PRIMARY KEY (id),
    CONSTRAINT FK_receipt_item_splits_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id),
    CONSTRAINT FK_receipt_item_splits_receipt FOREIGN KEY (receipt_id) REFERENCES receipts(id),
    CONSTRAINT FK_receipt_item_splits_receipt_item FOREIGN KEY (receipt_item_id) REFERENCES receipt_items(id),
    CONSTRAINT FK_receipt_item_splits_location FOREIGN KEY (location_id) REFERENCES locations(id),
    CONSTRAINT FK_receipt_item_splits_created_by FOREIGN KEY (created_by) REFERENCES users(id),
    CONSTRAINT CK_receipt_item_splits_quantity_positive CHECK (quantity > 0)
);

CREATE INDEX IX_receipt_item_splits_tenant_id ON receipt_item_splits(tenant_id);
CREATE INDEX IX_receipt_item_splits_receipt_item_id ON receipt_item_splits(receipt_item_id);
CREATE INDEX IX_receipt_item_splits_receipt_id ON receipt_item_splits(receipt_id);
