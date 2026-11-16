-- Phase 33E.2 — Destination Receiving, step 3 of 3. Idempotency guard
-- shared by BOTH of this module's mutating actions —
-- POST /transfer-receipts/{id}/items/{itemId}/receive (the stock-
-- mutating, AddStockTx-calling action, idempotency-guarded the same
-- way packing_confirmations guards PackItem) and
-- POST /transfer-receipts/{id}/confirm (the header DRAFT -> COMPLETED
-- transition). One shared table is enough: idempotency_key is the
-- primary key and the caller mints a fresh UUID per logical action
-- instance (one ReceiveItem call, or the one Confirm call), exactly the
-- same "client generates one idempotency_key per click, the service
-- INSERTs it here as the first statement inside that action's own
-- transaction" pattern every other *_confirmations table in this
-- codebase already uses (receipt_confirmations, relocation_
-- confirmations, packing_confirmations, ...). A repeat of the same key
-- fails the INSERT on the PRIMARY KEY constraint, rolling back the
-- whole transaction before any stock moves twice or the header
-- transitions twice.
CREATE TABLE transfer_receipt_confirmations (
    idempotency_key       VARCHAR(64) NOT NULL,
    tenant_id             BIGINT      NOT NULL,
    transfer_receipt_id   BIGINT      NOT NULL,
    created_at            DATETIME2   NOT NULL CONSTRAINT DF_transfer_receipt_confirmations_created_at DEFAULT (SYSUTCDATETIME()),
    CONSTRAINT PK_transfer_receipt_confirmations PRIMARY KEY (idempotency_key),
    CONSTRAINT FK_transfer_receipt_confirmations_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id),
    CONSTRAINT FK_transfer_receipt_confirmations_receipt FOREIGN KEY (transfer_receipt_id) REFERENCES transfer_receipts(id)
);

CREATE INDEX IX_transfer_receipt_confirmations_tenant_id ON transfer_receipt_confirmations(tenant_id);
CREATE INDEX IX_transfer_receipt_confirmations_receipt_id ON transfer_receipt_confirmations(transfer_receipt_id);
