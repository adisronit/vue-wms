-- Idempotency guard for POST /api/v1/relocations/{id}/confirm — the
-- exact same pattern as putaway_confirmations (migrations/0016): the
-- client generates one idempotency_key per confirm click, the service
-- INSERTs it here as the first statement inside the confirm
-- transaction, and a repeat of the same key (double-click, retry,
-- duplicate scan) fails the INSERT on the PRIMARY KEY constraint,
-- rolling back the whole transaction before any stock moves twice.
CREATE TABLE relocation_confirmations (
    idempotency_key      VARCHAR(64) NOT NULL,
    tenant_id             BIGINT      NOT NULL,
    relocation_task_id    BIGINT      NOT NULL,
    created_at            DATETIME2   NOT NULL CONSTRAINT DF_relocation_confirmations_created_at DEFAULT (SYSUTCDATETIME()),
    CONSTRAINT PK_relocation_confirmations PRIMARY KEY (idempotency_key),
    CONSTRAINT FK_relocation_confirmations_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id),
    CONSTRAINT FK_relocation_confirmations_task FOREIGN KEY (relocation_task_id) REFERENCES relocation_tasks(id)
);

CREATE INDEX IX_relocation_confirmations_tenant_id ON relocation_confirmations(tenant_id);
CREATE INDEX IX_relocation_confirmations_task_id ON relocation_confirmations(relocation_task_id);
