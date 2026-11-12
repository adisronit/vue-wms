-- Idempotency guard for POST /api/v1/shipping/{id}/confirm — the exact
-- same pattern as receipt_confirmations/putaway_confirmations/
-- picking_confirmations: the client generates one idempotency_key per
-- action click (Confirm Dispatch / Mark Delivered / Report Issue), the
-- service INSERTs it here as the first statement inside the transition
-- transaction, and a repeat of the same key (double-click, retry) fails
-- the INSERT on the PRIMARY KEY constraint, rolling back before the
-- status transition (and its side effects, e.g. dispatched_at/
-- delivered_at) is ever applied twice.
--
-- Unlike Receiving/Putaway/Picking's confirmations tables, this guard
-- protects a status TRANSITION, not a stock-quantity mutation — there
-- is no inventory change to duplicate here (Shipping never calls
-- AddStock/RemoveStock/MoveStock, see internal/shipping's package doc)
-- — but the same exactly-once guarantee still matters for
-- dispatched_at/delivered_at timestamps and for whatever a future
-- notification phase might trigger off this same confirm call.
CREATE TABLE shipment_confirmations (
    idempotency_key VARCHAR(64) NOT NULL,
    shipment_id     BIGINT      NOT NULL,
    created_at      DATETIME2   NOT NULL CONSTRAINT DF_shipment_confirmations_created_at DEFAULT (SYSUTCDATETIME()),
    CONSTRAINT PK_shipment_confirmations PRIMARY KEY (idempotency_key),
    CONSTRAINT FK_shipment_confirmations_shipment FOREIGN KEY (shipment_id) REFERENCES shipments(id)
);

CREATE INDEX IX_shipment_confirmations_shipment_id ON shipment_confirmations(shipment_id);
