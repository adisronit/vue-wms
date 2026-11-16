-- Phase 33E.2 — Destination Receiving, step 1 of 3. transfer_receipts is
-- the header for the action that finally credits the DESTINATION
-- warehouse once an INTERNAL_TRANSFER order's dispatched shipment
-- physically arrives — closing the status-window 33E.1 proved (stock
-- left the SOURCE warehouse at Picking.Confirm(PICKED) time; nothing
-- credits the destination until this module's own Confirm runs).
--
-- Deliberately its own new, lightweight module (internal/transferreceipt)
-- rather than an extension of internal/receiving — see that package's
-- own doc for why forcing transfer semantics into Receiving's deeply
-- supplier-centric shape (vendor, PO reference, QC/hold integration)
-- would distort it for a workflow that needs none of that. Mirrors
-- receipts' OWN simple header shape instead (migrations/0011), minus
-- every vendor/QC/logistics-document field that has no meaning for an
-- internal transfer.
--
-- shipment_id is UNIQUE per tenant: at most one receipt may ever exist
-- for a given transfer shipment (a transfer is received exactly once,
-- incrementally across any number of ReceiveItem calls, never re-opened
-- under a second header). order_id is the SOURCE INTERNAL_TRANSFER
-- order (for display/traceability only — Service.Create resolves it
-- from the shipment's own linked order, never trusts client input).
-- destination_warehouse_id is copied from that order's own column
-- (migrations/0113) at Create time, a one-time snapshot exactly like
-- every other "copied at creation, never re-synced" field in this
-- codebase (e.g. receipts.supplier_name_snapshot).
--
-- Status is DRAFT -> COMPLETED/CANCELLED only — no intermediate
-- PARTIALLY_RECEIVED state the way receipts.status has: a transfer
-- receipt stays DRAFT across any number of incremental ReceiveItem
-- calls (see transfer_receipt_items' own received_qty/dispatched_qty),
-- and moves to COMPLETED only via the explicit, idempotency-guarded
-- Confirm action once every line is either fully received or has a
-- recorded discrepancy — the same explicit "ready to confirm" gate
-- shape loading_trips' own ConfirmTrip already established, not a
-- status derived automatically from line quantities the way receipts.
-- status is.
--
-- received_by/received_at are stamped only by that successful Confirm
-- call (nil throughout DRAFT), matching the cancellation audit columns'
-- own "nil until it actually happens" shape (migrations/0032).
--
-- One CREATE TABLE statement — every column and FK is new together, so
-- none of the ADD-COLUMN-then-ADD-CONSTRAINT batch-compilation risk
-- migrations/0050/0053/0065/0067 document applies.
CREATE TABLE transfer_receipts (
    id                         BIGINT        IDENTITY(1,1) NOT NULL,
    tenant_id                  BIGINT        NOT NULL,
    receipt_no                 VARCHAR(30)   NOT NULL,
    shipment_id                BIGINT        NOT NULL,
    order_id                   BIGINT        NOT NULL,
    destination_warehouse_id   BIGINT        NOT NULL,
    status                     VARCHAR(20)   NOT NULL CONSTRAINT DF_transfer_receipts_status DEFAULT ('DRAFT'),
    received_by                BIGINT        NULL,
    received_at                DATETIME2     NULL,
    remark                     NVARCHAR(500) NULL,
    created_by                 BIGINT        NULL,
    created_at                 DATETIME2     NOT NULL CONSTRAINT DF_transfer_receipts_created_at DEFAULT (SYSUTCDATETIME()),
    updated_at                 DATETIME2     NOT NULL CONSTRAINT DF_transfer_receipts_updated_at DEFAULT (SYSUTCDATETIME()),
    cancelled_at               DATETIME2     NULL,
    cancelled_by               BIGINT        NULL,
    cancel_remark              NVARCHAR(500) NULL,
    CONSTRAINT PK_transfer_receipts PRIMARY KEY (id),
    CONSTRAINT UQ_transfer_receipts_tenant_receipt_no UNIQUE (tenant_id, receipt_no),
    CONSTRAINT UQ_transfer_receipts_tenant_shipment UNIQUE (tenant_id, shipment_id),
    CONSTRAINT FK_transfer_receipts_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id),
    CONSTRAINT FK_transfer_receipts_shipment FOREIGN KEY (shipment_id) REFERENCES shipments(id),
    CONSTRAINT FK_transfer_receipts_order FOREIGN KEY (order_id) REFERENCES orders(id),
    CONSTRAINT FK_transfer_receipts_destination_warehouse FOREIGN KEY (destination_warehouse_id) REFERENCES warehouses(id),
    CONSTRAINT FK_transfer_receipts_received_by FOREIGN KEY (received_by) REFERENCES users(id),
    CONSTRAINT FK_transfer_receipts_created_by FOREIGN KEY (created_by) REFERENCES users(id),
    CONSTRAINT FK_transfer_receipts_cancelled_by FOREIGN KEY (cancelled_by) REFERENCES users(id),
    CONSTRAINT CK_transfer_receipts_status CHECK (status IN ('DRAFT', 'COMPLETED', 'CANCELLED'))
);

CREATE INDEX IX_transfer_receipts_tenant_id ON transfer_receipts(tenant_id);
CREATE INDEX IX_transfer_receipts_destination_warehouse_id ON transfer_receipts(destination_warehouse_id);
CREATE INDEX IX_transfer_receipts_status ON transfer_receipts(status);
CREATE INDEX IX_transfer_receipts_order_id ON transfer_receipts(order_id);
