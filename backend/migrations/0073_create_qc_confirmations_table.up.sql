-- Idempotency guard for POST /api/v1/qc/inspections/{id}/confirm —
-- identical shape and rationale to receipt_confirmations
-- (migrations/0013): the client generates one idempotency_key per
-- confirm click; the service INSERTs it as the very first statement
-- inside the confirm transaction, and the UNIQUE primary key makes a
-- double-click/retry a race-free no-op instead of a double disposition.
CREATE TABLE qc_confirmations (
    idempotency_key  VARCHAR(64) NOT NULL,
    tenant_id        BIGINT      NOT NULL,
    inspection_id    BIGINT      NOT NULL,
    created_at       DATETIME2   NOT NULL CONSTRAINT DF_qc_confirmations_created_at DEFAULT (SYSUTCDATETIME()),
    CONSTRAINT PK_qc_confirmations PRIMARY KEY (idempotency_key),
    CONSTRAINT FK_qc_confirmations_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id),
    CONSTRAINT FK_qc_confirmations_inspection FOREIGN KEY (inspection_id) REFERENCES qc_inspections(id)
);

CREATE INDEX IX_qc_confirmations_inspection_id ON qc_confirmations(inspection_id);
