-- Idempotency guard for POST /api/v1/receipts/{id}/confirm. The client
-- generates one idempotency_key per confirm click; the service INSERTs
-- it here as the very first statement inside the confirm transaction.
-- If that INSERT fails on the UNIQUE constraint (a double-click, a
-- browser retry, a duplicate scan submission all resend the same key),
-- the whole transaction rolls back immediately and no stock is
-- double-added — the request instead returns the receipt's current
-- (already-correct) state. The UNIQUE constraint is what makes this
-- race-free: two concurrent requests with the same key can't both
-- succeed, regardless of timing.
CREATE TABLE receipt_confirmations (
    idempotency_key  VARCHAR(64) NOT NULL,
    receipt_id       BIGINT      NOT NULL,
    created_at       DATETIME2   NOT NULL CONSTRAINT DF_receipt_confirmations_created_at DEFAULT (SYSUTCDATETIME()),
    CONSTRAINT PK_receipt_confirmations PRIMARY KEY (idempotency_key),
    CONSTRAINT FK_receipt_confirmations_receipt FOREIGN KEY (receipt_id) REFERENCES receipts(id)
);

CREATE INDEX IX_receipt_confirmations_receipt_id ON receipt_confirmations(receipt_id);
