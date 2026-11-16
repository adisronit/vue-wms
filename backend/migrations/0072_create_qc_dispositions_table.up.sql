-- Phase 31B — Quality Control: qc_dispositions is the append-only
-- ledger of every PASS/HOLD/REJECT decision recorded against a
-- qc_inspection_items row, mirroring inventory_transactions' own
-- append-only, never-updated-or-deleted shape. Quantity is this
-- decision's own magnitude, not a running total — the same per-call
-- delta convention receipt_items.received_qty / inventory_transactions
-- already use. A single inspection item can accumulate many rows over
-- time (partial dispositions across one or more Confirm calls, and
-- reinspection of whatever quantity a HOLD decision left untouched —
-- see internal/qualitycheck's own Service doc for why "remaining to
-- inspect" is always just the live inventory.HoldQuantity for the
-- item's tuple, not a column tracked here).
--
-- defect_code/evidence_reference are plain strings, not FKs to a
-- defect-code or evidence-file master table — no such master exists
-- anywhere in this codebase, and building one is out of this phase's
-- scope; defect_code mirrors the exact set NewQualityCheckView.vue's
-- pre-existing mock already offered (DAMAGED/WRONG_ITEM/EXPIRED/
-- SHORTAGE/PACKAGING_ISSUE), evidence_reference is a free-text pointer
-- (e.g. a filename or URL) to wherever supporting evidence actually
-- lives.
CREATE TABLE qc_dispositions (
    id                    BIGINT        IDENTITY(1,1) NOT NULL,
    tenant_id             BIGINT        NOT NULL,
    inspection_item_id    BIGINT        NOT NULL,
    disposition           VARCHAR(10)   NOT NULL,
    quantity              DECIMAL(18,4) NOT NULL,
    defect_code           VARCHAR(30)   NULL,
    evidence_reference    VARCHAR(500)  NULL,
    remark                NVARCHAR(500) NULL,
    decided_by            BIGINT        NULL,
    created_at            DATETIME2     NOT NULL CONSTRAINT DF_qc_dispositions_created_at DEFAULT (SYSUTCDATETIME()),
    CONSTRAINT PK_qc_dispositions PRIMARY KEY (id),
    CONSTRAINT FK_qc_dispositions_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id),
    CONSTRAINT FK_qc_dispositions_item FOREIGN KEY (inspection_item_id) REFERENCES qc_inspection_items(id),
    CONSTRAINT FK_qc_dispositions_decided_by FOREIGN KEY (decided_by) REFERENCES users(id),
    CONSTRAINT CK_qc_dispositions_disposition CHECK (disposition IN ('PASS', 'HOLD', 'REJECT')),
    CONSTRAINT CK_qc_dispositions_quantity_positive CHECK (quantity > 0)
);

CREATE INDEX IX_qc_dispositions_tenant_id ON qc_dispositions(tenant_id);
CREATE INDEX IX_qc_dispositions_inspection_item_id ON qc_dispositions(inspection_item_id);
