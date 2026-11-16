-- Phase 33D.2 -- the scan-to-load worklist: one row per carton that is
-- expected to be loaded for a trip, auto-populated from carton_id FK to
-- packing's own cartons(id) -- a shipment's own cartons resolve to
-- "the cartons belonging to that shipment's own packing_job_id" (see
-- internal/loading/service.go's own doc) -- the moment a shipment is
-- assigned to the trip (Service.AssignShipment). "Planned qty" for a
-- trip/shipment is COUNT(*) of these rows; "loaded qty" is COUNT(*)
-- WHERE status = 'LOADED'.
--
-- status PLANNED -> LOADED (the ordinary scan-to-load action,
-- Service.ScanCarton) / SHORT (the damaged/missing exception,
-- Service.ShortCarton) is a terminal-once-resolved state machine, the
-- same two-terminal-outcome shape packing_job_items.status (PACKED/
-- EXCEPTION) and picking_task_items.status (COMPLETED/SHORT) already
-- established. The transition itself is applied via a single atomic
-- guarded UPDATE (... WHERE status = 'PLANNED') in
-- internal/loading/repository.go -- not a Go read-modify-write -- so
-- two concurrent scans for the very same carton can never both
-- succeed: exactly one UPDATE affects a row, the other affects zero
-- rows and is rejected by Service as "already loaded"/"already
-- resolved". This status-machine guard is also why this table needs no
-- separate idempotency-key table of its own, the same reasoning
-- packing_confirmations' own migration doc gives for why
-- CheckerConfirm/Cancel never needed one either: a retried/duplicate
-- scan is already naturally rejected once the first one succeeds.
--
-- No unique constraint on carton_id alone: a cancelled trip's own rows
-- stay here as history (mirrors loading_trip_shipments' own shape), and
-- the shipment-level "not already assigned to another active trip"
-- check in Service.AssignShipment is what actually prevents the same
-- carton from ever legitimately appearing PLANNED on two
-- simultaneously-active trips. UNIQUE(trip_id, carton_id) still guards
-- against the same carton being added to the very same trip twice.
CREATE TABLE loading_trip_cartons (
    id          BIGINT        IDENTITY(1,1) NOT NULL,
    tenant_id   BIGINT        NOT NULL,
    trip_id     BIGINT        NOT NULL,
    carton_id   BIGINT        NOT NULL,
    shipment_id BIGINT        NOT NULL,
    status      VARCHAR(20)   NOT NULL CONSTRAINT DF_loading_trip_cartons_status DEFAULT ('PLANNED'),
    loaded_at   DATETIME2     NULL,
    loaded_by   BIGINT        NULL,
    short_reason VARCHAR(20)  NULL,
    short_remark NVARCHAR(500) NULL,
    created_at  DATETIME2     NOT NULL CONSTRAINT DF_loading_trip_cartons_created_at DEFAULT (SYSUTCDATETIME()),
    updated_at  DATETIME2     NOT NULL CONSTRAINT DF_loading_trip_cartons_updated_at DEFAULT (SYSUTCDATETIME()),
    CONSTRAINT PK_loading_trip_cartons PRIMARY KEY (id),
    CONSTRAINT UQ_loading_trip_cartons_trip_carton UNIQUE (trip_id, carton_id),
    CONSTRAINT FK_loading_trip_cartons_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id),
    CONSTRAINT FK_loading_trip_cartons_trip FOREIGN KEY (trip_id) REFERENCES loading_trips(id),
    CONSTRAINT FK_loading_trip_cartons_carton FOREIGN KEY (carton_id) REFERENCES cartons(id),
    CONSTRAINT FK_loading_trip_cartons_shipment FOREIGN KEY (shipment_id) REFERENCES shipments(id),
    CONSTRAINT FK_loading_trip_cartons_loaded_by FOREIGN KEY (loaded_by) REFERENCES users(id),
    CONSTRAINT CK_loading_trip_cartons_status CHECK (status IN ('PLANNED', 'LOADED', 'SHORT')),
    CONSTRAINT CK_loading_trip_cartons_short_reason CHECK (short_reason IS NULL OR short_reason IN ('DAMAGED', 'MISSING', 'OTHER'))
);

CREATE INDEX IX_loading_trip_cartons_tenant_id ON loading_trip_cartons(tenant_id);
CREATE INDEX IX_loading_trip_cartons_trip_id ON loading_trip_cartons(trip_id);
CREATE INDEX IX_loading_trip_cartons_carton_id ON loading_trip_cartons(carton_id);
CREATE INDEX IX_loading_trip_cartons_shipment_id ON loading_trip_cartons(shipment_id);
