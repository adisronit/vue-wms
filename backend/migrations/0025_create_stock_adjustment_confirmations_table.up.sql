-- Idempotency guard for POST /api/v1/stock-adjustments/{id}/confirm —
-- the exact same pattern as every prior phase's confirmations table:
-- the client generates one idempotency_key per Approve/Reject click,
-- the service INSERTs it here as the first statement inside the
-- transaction, and a repeat of the same key fails the INSERT on the
-- PRIMARY KEY constraint, rolling back before the approval (and its
-- AddStockTx/RemoveStockTx call) is ever applied twice.
CREATE TABLE stock_adjustment_confirmations (
    idempotency_key     VARCHAR(64) NOT NULL,
    stock_adjustment_id BIGINT      NOT NULL,
    created_at          DATETIME2   NOT NULL CONSTRAINT DF_stock_adjustment_confirmations_created_at DEFAULT (SYSUTCDATETIME()),
    CONSTRAINT PK_stock_adjustment_confirmations PRIMARY KEY (idempotency_key),
    CONSTRAINT FK_stock_adjustment_confirmations_adjustment FOREIGN KEY (stock_adjustment_id) REFERENCES stock_adjustments(id)
);

CREATE INDEX IX_stock_adjustment_confirmations_adjustment_id ON stock_adjustment_confirmations(stock_adjustment_id);
