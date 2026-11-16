-- Phase 33D.2 -- Loading module. loading_trips is the vehicle/trip
-- header a warehouse plans before loading starts: which vehicle/driver/
-- carrier is coming, which dock, when it is scheduled, and (once
-- loading actually begins/ends) the real start/end timestamps and the
-- vehicle seal number recorded at close-out.
--
-- Status machine: PLANNED (header created, shipments may already be
-- assigned -- see loading_trip_shipments -- but no carton has been
-- scanned loaded or flagged short yet) -> LOADING (at least one carton
-- has been scanned/flagged -- actual_loading_start_at is stamped at
-- that same moment) -> COMPLETED (every planned carton for this trip is
-- LOADED or SHORT, and the explicit Confirm action has run --
-- actual_loading_end_at is stamped then, and internal/shipping's own
-- Service.MarkLoaded is called for every shipment on this trip, the
-- real dispatch-gate payoff) / CANCELLED (abandoned before completion;
-- allowed from PLANNED or LOADING only, matching every other module's
-- own CanCancel shape).
--
-- trip_no is system-generated from the new IDENTITY id (the same
-- race-free-by-construction "TRIP-%06d" pattern as packing_jobs.job_no/
-- picking_tasks.pick_no/shipments.shipment_no), never user-supplied.
CREATE TABLE loading_trips (
    id                      BIGINT        IDENTITY(1,1) NOT NULL,
    tenant_id               BIGINT        NOT NULL,
    trip_no                 VARCHAR(20)   NOT NULL,
    vehicle_no              VARCHAR(30)   NULL,
    driver_name             VARCHAR(100)  NULL,
    carrier                 VARCHAR(20)   NULL,
    loading_dock            VARCHAR(30)   NULL,
    route                   VARCHAR(200)  NULL,
    scheduled_loading_time  DATETIME2     NULL,
    actual_loading_start_at DATETIME2     NULL,
    actual_loading_end_at   DATETIME2     NULL,
    vehicle_seal            VARCHAR(50)   NULL,
    status                  VARCHAR(20)   NOT NULL CONSTRAINT DF_loading_trips_status DEFAULT ('PLANNED'),
    remark                  NVARCHAR(500) NULL,
    created_by              BIGINT        NULL,
    created_at              DATETIME2     NOT NULL CONSTRAINT DF_loading_trips_created_at DEFAULT (SYSUTCDATETIME()),
    updated_at              DATETIME2     NOT NULL CONSTRAINT DF_loading_trips_updated_at DEFAULT (SYSUTCDATETIME()),
    completed_at            DATETIME2     NULL,
    cancelled_at            DATETIME2     NULL,
    cancelled_by            BIGINT        NULL,
    cancel_remark           NVARCHAR(500) NULL,
    CONSTRAINT PK_loading_trips PRIMARY KEY (id),
    CONSTRAINT UQ_loading_trips_tenant_trip_no UNIQUE (tenant_id, trip_no),
    CONSTRAINT FK_loading_trips_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id),
    CONSTRAINT FK_loading_trips_created_by FOREIGN KEY (created_by) REFERENCES users(id),
    CONSTRAINT FK_loading_trips_cancelled_by FOREIGN KEY (cancelled_by) REFERENCES users(id),
    CONSTRAINT CK_loading_trips_status CHECK (status IN ('PLANNED', 'LOADING', 'COMPLETED', 'CANCELLED'))
);

CREATE INDEX IX_loading_trips_tenant_id ON loading_trips(tenant_id);
CREATE INDEX IX_loading_trips_status ON loading_trips(status);
