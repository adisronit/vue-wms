-- Idempotency guard for POST /api/v1/packing-jobs/{id}/pack — the exact
-- same pattern as relocation_confirmations (migrations/0078) /
-- picking_confirmations (migrations/0019): the client generates one
-- idempotency_key per pack action, the service INSERTs it here as the
-- first statement inside the pack transaction, and a repeat of the same
-- key (double-click, retry, duplicate scan) fails the INSERT on the
-- PRIMARY KEY constraint, rolling back the whole transaction before
-- packed_qty is ever incremented twice for the same action.
--
-- The checker-verification completion action (POST .../checker-confirm)
-- deliberately does NOT use this guard — it carries no quantity math of
-- its own (a pure DRAFT/IN_PROGRESS -> COMPLETED status transition),
-- the same reason Cancel across every module in this codebase has never
-- needed an idempotency table: a repeat call is already naturally
-- rejected by the status-machine guard (job no longer IN_PROGRESS) once
-- the first call succeeds.
CREATE TABLE packing_confirmations (
    idempotency_key   VARCHAR(64) NOT NULL,
    tenant_id         BIGINT      NOT NULL,
    packing_job_id    BIGINT      NOT NULL,
    created_at        DATETIME2   NOT NULL CONSTRAINT DF_packing_confirmations_created_at DEFAULT (SYSUTCDATETIME()),
    CONSTRAINT PK_packing_confirmations PRIMARY KEY (idempotency_key),
    CONSTRAINT FK_packing_confirmations_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id),
    CONSTRAINT FK_packing_confirmations_job FOREIGN KEY (packing_job_id) REFERENCES packing_jobs(id)
);

CREATE INDEX IX_packing_confirmations_tenant_id ON packing_confirmations(tenant_id);
CREATE INDEX IX_packing_confirmations_job_id ON packing_confirmations(packing_job_id);
