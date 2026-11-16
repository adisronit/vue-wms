-- Phase 31B — Quality Control: qc_inspections is the inspection header,
-- always created against one receipt (the "Receiving No." the mock
-- QualityCheckView.vue/NewQualityCheckView.vue already showed a
-- picker for). inspection_no is generated deterministically from the
-- IDENTITY id right after insert, the exact same race-free pattern
-- receipts.receipt_no (migrations/0011) and every other document-number
-- column in this codebase uses.
--
-- One CREATE TABLE statement — every column and FK is new together, so
-- none of the ADD-COLUMN-then-ADD-CONSTRAINT batch-compilation risk
-- migrations/0050/0053/0065/0067 document applies.
CREATE TABLE qc_inspections (
    id              BIGINT        IDENTITY(1,1) NOT NULL,
    tenant_id       BIGINT        NOT NULL,
    inspection_no   VARCHAR(30)   NOT NULL,
    receipt_id      BIGINT        NOT NULL,
    warehouse_id    BIGINT        NOT NULL,
    inspector_id    BIGINT        NULL,
    qc_type         VARCHAR(20)   NOT NULL CONSTRAINT DF_qc_inspections_qc_type DEFAULT ('NORMAL'),
    status          VARCHAR(20)   NOT NULL CONSTRAINT DF_qc_inspections_status DEFAULT ('PENDING'),
    remark          NVARCHAR(500) NULL,
    created_by      BIGINT        NULL,
    created_at      DATETIME2     NOT NULL CONSTRAINT DF_qc_inspections_created_at DEFAULT (SYSUTCDATETIME()),
    updated_at      DATETIME2     NOT NULL CONSTRAINT DF_qc_inspections_updated_at DEFAULT (SYSUTCDATETIME()),
    CONSTRAINT PK_qc_inspections PRIMARY KEY (id),
    CONSTRAINT UQ_qc_inspections_tenant_inspection_no UNIQUE (tenant_id, inspection_no),
    CONSTRAINT FK_qc_inspections_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id),
    CONSTRAINT FK_qc_inspections_receipt FOREIGN KEY (receipt_id) REFERENCES receipts(id),
    CONSTRAINT FK_qc_inspections_warehouse FOREIGN KEY (warehouse_id) REFERENCES warehouses(id),
    CONSTRAINT FK_qc_inspections_inspector FOREIGN KEY (inspector_id) REFERENCES users(id),
    CONSTRAINT FK_qc_inspections_created_by FOREIGN KEY (created_by) REFERENCES users(id),
    CONSTRAINT CK_qc_inspections_qc_type CHECK (qc_type IN ('NORMAL', 'FULL', 'RANDOM', 'DAMAGE')),
    CONSTRAINT CK_qc_inspections_status CHECK (status IN ('PENDING', 'IN_PROGRESS', 'COMPLETED'))
);

CREATE INDEX IX_qc_inspections_tenant_id ON qc_inspections(tenant_id);
CREATE INDEX IX_qc_inspections_receipt_id ON qc_inspections(receipt_id);
CREATE INDEX IX_qc_inspections_status ON qc_inspections(status);
