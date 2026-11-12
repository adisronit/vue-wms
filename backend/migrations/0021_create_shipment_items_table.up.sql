-- Shipment lines, matching ShippingView.vue's "Package Items" cards
-- (sku, productName, qty — a flat, read-only display with no per-line
-- edit control anywhere in the mock).
--
-- qty is copied from the source picking_task_items.picked_qty at
-- shipment-creation time and never edited afterward — there is no
-- input field on Package Items in the mock, so no partial-shipment or
-- per-line quantity adjustment is invented here (see
-- internal/shipping/service.go's Create doc). This is also how "ship
-- quantity must not exceed picked quantity" is enforced: the client
-- never supplies a quantity at all, it is always read authoritatively
-- from the linked picking task's own already-picked lines.
--
-- lot_no uses the same NOT NULL DEFAULT '' pattern as receipt_items/
-- putaway_task_items/picking_task_items for the same SQL Server
-- one-NULL-per-table uniqueness reason documented there.
--
-- location_id is the location the stock was picked FROM (carried
-- forward for traceability only — Shipping performs no inventory
-- mutation of its own, see internal/shipping's package doc).
CREATE TABLE shipment_items (
    id                    BIGINT        IDENTITY(1,1) NOT NULL,
    shipment_id           BIGINT        NOT NULL,
    picking_task_item_id  BIGINT        NOT NULL,
    product_id            BIGINT        NOT NULL,
    lot_no                VARCHAR(50)   NOT NULL CONSTRAINT DF_shipment_items_lot_no DEFAULT (''),
    location_id           BIGINT        NOT NULL,
    qty                   DECIMAL(18,4) NOT NULL,
    uom                   VARCHAR(20)   NOT NULL,
    created_at            DATETIME2     NOT NULL CONSTRAINT DF_shipment_items_created_at DEFAULT (SYSUTCDATETIME()),
    CONSTRAINT PK_shipment_items PRIMARY KEY (id),
    CONSTRAINT FK_shipment_items_shipment FOREIGN KEY (shipment_id) REFERENCES shipments(id),
    CONSTRAINT FK_shipment_items_picking_task_item FOREIGN KEY (picking_task_item_id) REFERENCES picking_task_items(id),
    CONSTRAINT FK_shipment_items_product FOREIGN KEY (product_id) REFERENCES products(id),
    CONSTRAINT FK_shipment_items_location FOREIGN KEY (location_id) REFERENCES locations(id),
    CONSTRAINT CK_shipment_items_qty CHECK (qty > 0)
);

CREATE INDEX IX_shipment_items_shipment_id ON shipment_items(shipment_id);
