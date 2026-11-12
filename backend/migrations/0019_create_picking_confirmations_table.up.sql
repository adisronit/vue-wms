-- Idempotency guard for POST /api/v1/picking/{id}/confirm — the exact
-- same pattern as receipt_confirmations (migrations/0013) and
-- putaway_confirmations (migrations/0016): the client generates one
-- idempotency_key per confirm click, the service INSERTs it here as the
-- very first statement inside the confirm transaction, and a repeat of
-- the same key (double-click, retry, duplicate scan) fails the INSERT
-- on the PRIMARY KEY constraint, rolling back the whole transaction
-- before any stock is removed twice.
CREATE TABLE picking_confirmations (
    idempotency_key VARCHAR(64) NOT NULL,
    picking_task_id BIGINT      NOT NULL,
    created_at      DATETIME2   NOT NULL CONSTRAINT DF_picking_confirmations_created_at DEFAULT (SYSUTCDATETIME()),
    CONSTRAINT PK_picking_confirmations PRIMARY KEY (idempotency_key),
    CONSTRAINT FK_picking_confirmations_task FOREIGN KEY (picking_task_id) REFERENCES picking_tasks(id)
);

CREATE INDEX IX_picking_confirmations_task_id ON picking_confirmations(picking_task_id);
