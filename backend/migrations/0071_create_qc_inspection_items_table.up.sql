-- Phase 31B — Quality Control: qc_inspection_items is one inspectable
-- line per receipt_item_splits row (migrations/0069) — NOT per
-- receipt_item, because a single receiving line confirmed across
-- multiple lots/pallets (internal/receiving's own Splits) produced one
-- distinct (product, location, lot) inventory balance tuple per split,
-- and QC's PASS/HOLD/REJECT dispositions must act on the exact tuple
-- inventory.AdjustHold guards — never on the line as a whole. Every
-- receiving Confirm call writes at least one split row (even the
-- implicit single-split case), so this is always a valid 1:1 or 1:N
-- target.
--
-- product_id/lot_no/location_id are denormalized from the split at
-- inspection-item creation time (read-only snapshot for display —
-- never re-synced), mirroring every other child table's denormalization
-- precedent in this codebase (e.g. receipt_items' own product_id/
-- location_id). The checklist columns (temperature_c, packaging_ok,
-- label_ok) are the "configurable inspection checklist" the task asked
-- for, modeled as real, queryable columns rather than a speculative
-- admin-configurable checklist-builder nothing else in this phase
-- requires.
--
-- status is derived at read time from the live inventory.HoldQuantity
-- for this item's own tuple (COMPLETED once hold reaches 0 — see
-- Service.itemStatus's own doc) and is not meant to be set directly by
-- any INSERT/UPDATE other than PENDING-on-create; kept as a real column
-- (not purely computed) so a list query can filter/sort on it without
-- joining back to inventory every time.
CREATE TABLE qc_inspection_items (
    id                     BIGINT        IDENTITY(1,1) NOT NULL,
    tenant_id              BIGINT        NOT NULL,
    inspection_id          BIGINT        NOT NULL,
    receipt_item_split_id  BIGINT        NOT NULL,
    product_id             BIGINT        NOT NULL,
    lot_no                 VARCHAR(50)   NOT NULL CONSTRAINT DF_qc_inspection_items_lot_no DEFAULT (''),
    expiry_date            DATE          NULL,
    location_id            BIGINT        NOT NULL,
    held_qty_snapshot      DECIMAL(18,4) NOT NULL,
    sample_qty             DECIMAL(18,4) NOT NULL CONSTRAINT DF_qc_inspection_items_sample_qty DEFAULT (0),
    temperature_c          DECIMAL(5,2)  NULL,
    packaging_ok           BIT           NULL,
    label_ok               BIT           NULL,
    status                 VARCHAR(20)   NOT NULL CONSTRAINT DF_qc_inspection_items_status DEFAULT ('PENDING'),
    created_at             DATETIME2     NOT NULL CONSTRAINT DF_qc_inspection_items_created_at DEFAULT (SYSUTCDATETIME()),
    updated_at             DATETIME2     NOT NULL CONSTRAINT DF_qc_inspection_items_updated_at DEFAULT (SYSUTCDATETIME()),
    CONSTRAINT PK_qc_inspection_items PRIMARY KEY (id),
    CONSTRAINT FK_qc_inspection_items_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id),
    CONSTRAINT FK_qc_inspection_items_inspection FOREIGN KEY (inspection_id) REFERENCES qc_inspections(id),
    CONSTRAINT FK_qc_inspection_items_split FOREIGN KEY (receipt_item_split_id) REFERENCES receipt_item_splits(id),
    CONSTRAINT FK_qc_inspection_items_product FOREIGN KEY (product_id) REFERENCES products(id),
    CONSTRAINT FK_qc_inspection_items_location FOREIGN KEY (location_id) REFERENCES locations(id),
    CONSTRAINT CK_qc_inspection_items_held_qty_snapshot_non_negative CHECK (held_qty_snapshot >= 0),
    CONSTRAINT CK_qc_inspection_items_sample_qty_non_negative CHECK (sample_qty >= 0),
    CONSTRAINT CK_qc_inspection_items_status CHECK (status IN ('PENDING', 'IN_PROGRESS', 'COMPLETED'))
);

CREATE INDEX IX_qc_inspection_items_tenant_id ON qc_inspection_items(tenant_id);
CREATE INDEX IX_qc_inspection_items_inspection_id ON qc_inspection_items(inspection_id);
CREATE INDEX IX_qc_inspection_items_split_id ON qc_inspection_items(receipt_item_split_id);
