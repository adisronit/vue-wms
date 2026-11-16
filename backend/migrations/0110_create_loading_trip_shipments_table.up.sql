-- Phase 33D.2 -- which shipments are assigned to a loading trip (a trip
-- may carry multiple shipments/multi-drop). A row here is inserted the
-- moment a shipment is assigned (Service.AssignShipment), not deferred
-- to Confirm.
--
-- No unique constraint on shipment_id alone -- mirrors
-- picking_batch_orders' own documented "no double-claim" shape
-- (migrations/0095) exactly: a CANCELLED trip's own rows stay here as
-- history, and that same shipment_id must be eligible to join a brand
-- new trip afterward. internal/loading's own Service enforces "not
-- already assigned to another ACTIVE (non-CANCELLED) trip" at the
-- application layer, via a query joining this table to loading_trips'
-- own status column, before ever inserting a new row here -- exactly
-- the same shape, for the same reason, picking_batch_orders' own doc
-- explains. UNIQUE(trip_id, shipment_id) still guards against the same
-- shipment being added to the very same trip twice.
CREATE TABLE loading_trip_shipments (
    id          BIGINT    IDENTITY(1,1) NOT NULL,
    tenant_id   BIGINT    NOT NULL,
    trip_id     BIGINT    NOT NULL,
    shipment_id BIGINT    NOT NULL,
    created_at  DATETIME2 NOT NULL CONSTRAINT DF_loading_trip_shipments_created_at DEFAULT (SYSUTCDATETIME()),
    CONSTRAINT PK_loading_trip_shipments PRIMARY KEY (id),
    CONSTRAINT UQ_loading_trip_shipments_trip_shipment UNIQUE (trip_id, shipment_id),
    CONSTRAINT FK_loading_trip_shipments_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id),
    CONSTRAINT FK_loading_trip_shipments_trip FOREIGN KEY (trip_id) REFERENCES loading_trips(id),
    CONSTRAINT FK_loading_trip_shipments_shipment FOREIGN KEY (shipment_id) REFERENCES shipments(id)
);

CREATE INDEX IX_loading_trip_shipments_tenant_id ON loading_trip_shipments(tenant_id);
CREATE INDEX IX_loading_trip_shipments_shipment_id ON loading_trip_shipments(shipment_id);
CREATE INDEX IX_loading_trip_shipments_trip_id ON loading_trip_shipments(trip_id);
