-- Idempotency guard for POST /api/v1/stock-counts/{id}/confirm — the
-- exact same pattern as every prior phase's confirmations table: the
-- client generates one idempotency_key per "Confirm Count" click, the
-- service INSERTs it here as the first statement inside the
-- transaction, and a repeat of the same key fails the INSERT on the
-- PRIMARY KEY constraint, rolling back before any line's variance is
-- applied through Inventory Core twice.
CREATE TABLE stock_count_confirmations (
    idempotency_key VARCHAR(64) NOT NULL,
    stock_count_id  BIGINT      NOT NULL,
    created_at      DATETIME2   NOT NULL CONSTRAINT DF_stock_count_confirmations_created_at DEFAULT (SYSUTCDATETIME()),
    CONSTRAINT PK_stock_count_confirmations PRIMARY KEY (idempotency_key),
    CONSTRAINT FK_stock_count_confirmations_count FOREIGN KEY (stock_count_id) REFERENCES stock_counts(id)
);

CREATE INDEX IX_stock_count_confirmations_count_id ON stock_count_confirmations(stock_count_id);
