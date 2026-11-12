-- Inbound receiving document header, matching NewReceivingView.vue's
-- "Document Information" section and ReceiveView.vue's list.
--
-- vendor is a plain string, not a FK to a supplier master table: no
-- supplier entity exists anywhere in this codebase or any other
-- frontend view — the create form's vendor <select> is 4 hardcoded
-- strings with nothing behind them. Building a suppliers table now
-- would be inventing a requirement the frontend doesn't actually make.
--
-- receipt_no is generated deterministically from the IDENTITY id right
-- after insert (see cmd/migrate-adjacent internal/receiving/repository.go),
-- so uniqueness is race-free by construction — no separate sequence or
-- locking scheme needed.
--
-- staging_location_id is a real locations(id) FK, not a fabricated
-- "STAGING-01" string: the frontend's Staging Location field is exactly
-- the receiving-side half of the Warehouse -> Zone -> Location model
-- already built in Phase 3 (typically a RECEIVING/HOLD-type zone's
-- location) — receiving deliberately does not bypass that model.
CREATE TABLE receipts (
    id                    BIGINT        IDENTITY(1,1) NOT NULL,
    receipt_no            VARCHAR(30)   NOT NULL,
    receiving_type        VARCHAR(20)   NOT NULL,
    reference_no          VARCHAR(50)   NULL,
    vendor                VARCHAR(150)  NULL,
    warehouse_id          BIGINT        NOT NULL,
    staging_location_id   BIGINT        NOT NULL,
    status                VARCHAR(20)   NOT NULL CONSTRAINT DF_receipts_status DEFAULT ('DRAFT'),
    receiving_date        DATE          NULL,
    remark                NVARCHAR(500) NULL,
    created_by            BIGINT        NULL,
    created_at            DATETIME2     NOT NULL CONSTRAINT DF_receipts_created_at DEFAULT (SYSUTCDATETIME()),
    updated_at            DATETIME2     NOT NULL CONSTRAINT DF_receipts_updated_at DEFAULT (SYSUTCDATETIME()),
    CONSTRAINT PK_receipts PRIMARY KEY (id),
    CONSTRAINT UQ_receipts_receipt_no UNIQUE (receipt_no),
    CONSTRAINT FK_receipts_warehouse FOREIGN KEY (warehouse_id) REFERENCES warehouses(id),
    CONSTRAINT FK_receipts_staging_location FOREIGN KEY (staging_location_id) REFERENCES locations(id),
    CONSTRAINT FK_receipts_created_by FOREIGN KEY (created_by) REFERENCES users(id),
    -- CANCELLED exists for schema forward-compatibility (the task's own
    -- suggested state machine includes it) but no frontend UI triggers
    -- it this phase — not reachable from any endpoint implemented here.
    CONSTRAINT CK_receipts_status CHECK (status IN
        ('DRAFT', 'WAITING', 'PARTIALLY_RECEIVED', 'COMPLETED', 'CANCELLED')),
    CONSTRAINT CK_receipts_receiving_type CHECK (receiving_type IN
        ('PO', 'ASN', 'RETURN', 'TRANSFER'))
);

CREATE INDEX IX_receipts_warehouse_id ON receipts(warehouse_id);
CREATE INDEX IX_receipts_status ON receipts(status);
