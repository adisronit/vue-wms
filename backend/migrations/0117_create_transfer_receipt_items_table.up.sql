-- Phase 33E.2 — Destination Receiving, step 2 of 3. One row per shipped
-- line: created from the source shipment_item at receipt-creation time
-- with dispatched_qty frozen as a snapshot of THAT shipment item's own
-- shipped quantity (post-33D.1, the real packed_qty-sourced amount —
-- never the original order line's ordered_qty, which may have been
-- short-picked) — see internal/transferreceipt/service.go's own
-- Create doc. dispatched_qty is the hard ceiling every ReceiveItem call
-- is bounded against (received_qty + delta <= dispatched_qty), the same
-- "one UPDATE, bounded in the WHERE clause" over-receipt guard
-- packing_job_items.packed_qty already uses against picked_qty
-- (migrations/0091's own AdjustPackedQty) — rejected outright, never
-- clamped.
--
-- received_qty is cumulative across every ReceiveItem call for this
-- line, not one call's delta — the same convention receipt_items/
-- relocation_task_items/packing_job_items already establish.
--
-- location_id is nullable: unlike receipt_items.location_id (which
-- always has a value, defaulted from the receiving header's own
-- staging_location_id at AddItem time), a transfer_receipt_item is
-- created with no location at all — the receiver chooses the
-- destination STAGING location at the moment they actually receive
-- each line (ReceiveItemRequest.LocationID, re-validated against
-- destination_warehouse_id and RECEIVING zone type on every call — see
-- Service.ReceiveItem's own doc), and this column is simply updated to
-- the most recent call's value for display, the same "most recent
-- confirm call's values" meaning receipt_items.location_id already
-- carries.
--
-- discrepancy_qty/discrepancy_reason (both nullable) record a line
-- explicitly closed short or damaged: received_qty permanently below
-- dispatched_qty, with a required reason — see Service.ReceiveItem's
-- own "close short" branch. Both stay NULL for every ordinary,
-- fully-received line.
--
-- status adds DISCREPANCY (closed short/damaged, terminal) to the
-- familiar PENDING/PARTIALLY_RECEIVED/COMPLETED set every other line
-- table in this codebase already uses.
CREATE TABLE transfer_receipt_items (
    id                   BIGINT        IDENTITY(1,1) NOT NULL,
    tenant_id            BIGINT        NOT NULL,
    transfer_receipt_id  BIGINT        NOT NULL,
    shipment_item_id     BIGINT        NOT NULL,
    product_id           BIGINT        NOT NULL,
    lot_no               VARCHAR(50)   NOT NULL CONSTRAINT DF_transfer_receipt_items_lot_no DEFAULT (''),
    uom                  VARCHAR(20)   NOT NULL,
    dispatched_qty       DECIMAL(18,4) NOT NULL,
    received_qty         DECIMAL(18,4) NOT NULL CONSTRAINT DF_transfer_receipt_items_received_qty DEFAULT (0),
    location_id          BIGINT        NULL,
    discrepancy_qty      DECIMAL(18,4) NULL,
    discrepancy_reason   NVARCHAR(500) NULL,
    status               VARCHAR(20)   NOT NULL CONSTRAINT DF_transfer_receipt_items_status DEFAULT ('PENDING'),
    created_at           DATETIME2     NOT NULL CONSTRAINT DF_transfer_receipt_items_created_at DEFAULT (SYSUTCDATETIME()),
    updated_at           DATETIME2     NOT NULL CONSTRAINT DF_transfer_receipt_items_updated_at DEFAULT (SYSUTCDATETIME()),
    CONSTRAINT PK_transfer_receipt_items PRIMARY KEY (id),
    CONSTRAINT UQ_transfer_receipt_items_receipt_shipment_item UNIQUE (transfer_receipt_id, shipment_item_id),
    CONSTRAINT FK_transfer_receipt_items_receipt FOREIGN KEY (transfer_receipt_id) REFERENCES transfer_receipts(id),
    CONSTRAINT FK_transfer_receipt_items_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id),
    CONSTRAINT FK_transfer_receipt_items_shipment_item FOREIGN KEY (shipment_item_id) REFERENCES shipment_items(id),
    CONSTRAINT FK_transfer_receipt_items_product FOREIGN KEY (product_id) REFERENCES products(id),
    CONSTRAINT FK_transfer_receipt_items_location FOREIGN KEY (location_id) REFERENCES locations(id),
    CONSTRAINT CK_transfer_receipt_items_status CHECK (status IN
        ('PENDING', 'PARTIALLY_RECEIVED', 'COMPLETED', 'DISCREPANCY')),
    CONSTRAINT CK_transfer_receipt_items_dispatched_qty CHECK (dispatched_qty > 0),
    CONSTRAINT CK_transfer_receipt_items_received_qty CHECK (received_qty >= 0 AND received_qty <= dispatched_qty),
    CONSTRAINT CK_transfer_receipt_items_discrepancy_qty CHECK (discrepancy_qty IS NULL OR discrepancy_qty > 0)
);

CREATE INDEX IX_transfer_receipt_items_tenant_id ON transfer_receipt_items(tenant_id);
CREATE INDEX IX_transfer_receipt_items_receipt_id ON transfer_receipt_items(transfer_receipt_id);
