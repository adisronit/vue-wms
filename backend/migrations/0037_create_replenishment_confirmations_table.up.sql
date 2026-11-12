-- Idempotency guard for POST /api/v1/replenishment/{id}/confirm — the
-- exact same pattern as picking_confirmations (migrations/0019),
-- receipt_confirmations (migrations/0013), and putaway_confirmations
-- (migrations/0016): the client generates one idempotency_key per
-- confirm click, the service INSERTs it here as the very first
-- statement inside the confirm transaction, and a repeat of the same
-- key (double-click, retry) fails the INSERT on the PRIMARY KEY
-- constraint, rolling back before MoveStockTx ever runs twice.
CREATE TABLE replenishment_confirmations (
    idempotency_key       VARCHAR(64) NOT NULL,
    replenishment_task_id BIGINT      NOT NULL,
    created_at            DATETIME2   NOT NULL CONSTRAINT DF_replenishment_confirmations_created_at DEFAULT (SYSUTCDATETIME()),
    CONSTRAINT PK_replenishment_confirmations PRIMARY KEY (idempotency_key),
    CONSTRAINT FK_replenishment_confirmations_task FOREIGN KEY (replenishment_task_id) REFERENCES replenishment_tasks(id)
);

CREATE INDEX IX_replenishment_confirmations_task_id ON replenishment_confirmations(replenishment_task_id);
