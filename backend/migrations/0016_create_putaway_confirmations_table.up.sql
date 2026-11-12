-- Idempotency guard for POST /api/v1/putaway/{id}/confirm — the exact
-- same pattern as receipt_confirmations (migrations/0013): the client
-- generates one idempotency_key per confirm click, the service INSERTs
-- it here as the very first statement inside the confirm transaction,
-- and a repeat of the same key (double-click, retry, duplicate scan)
-- fails the INSERT on the PRIMARY KEY constraint, rolling back the
-- whole transaction before any stock moves twice.
CREATE TABLE putaway_confirmations (
    idempotency_key  VARCHAR(64) NOT NULL,
    putaway_task_id  BIGINT      NOT NULL,
    created_at       DATETIME2   NOT NULL CONSTRAINT DF_putaway_confirmations_created_at DEFAULT (SYSUTCDATETIME()),
    CONSTRAINT PK_putaway_confirmations PRIMARY KEY (idempotency_key),
    CONSTRAINT FK_putaway_confirmations_task FOREIGN KEY (putaway_task_id) REFERENCES putaway_tasks(id)
);

CREATE INDEX IX_putaway_confirmations_task_id ON putaway_confirmations(putaway_task_id);
