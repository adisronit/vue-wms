-- Phase 33C — Packing: packing_jobs is the header for the physical-
-- handling/traceability layer that sits between Picking and Shipping.
-- Physical stock already left the building at Picking.Confirm(PICKED)
-- (via inventory.ConsumeReservationTx) — this package and every table
-- below is a pure record of how already-picked quantities get boxed
-- into cartons, and NEVER calls any internal/inventory mutation
-- function (grep this package to confirm: no AddStockTx/RemoveStockTx/
-- MoveStockTx/ReserveStockTx/ConsumeReservationTx/ReleaseReservationTx
-- call anywhere).
--
-- One packing job per picking task — picking_task_id is UNIQUE within
-- tenant_id, the same "create a dependent document from a terminal
-- picking task, at most once" shape shipments.picking_task_id (migrations/
-- 0020) already established (that one predates tenant scoping and is a
-- bare global UNIQUE; this one includes tenant_id, matching every table
-- created since Phase 29A).
--
-- order_no/customer/customer_id/ship_to_* are a one-way snapshot copy
-- from the source picking_tasks row at job-creation time (internal/
-- picking's own CustomerID/ShipTo* columns, migrations/0062, are
-- themselves already a one-way copy from orders) — deliberately NO FK
-- on customer_id/ship_to_id, matching picking_tasks' own precedent of
-- never re-verifying that copied ownership a second time. warehouse_id
-- is likewise copied from the picking task, not independently chosen.
--
-- Status is DRAFT (job created, no packing activity yet) ->
-- IN_PROGRESS (at least one item has been packed into a carton or
-- flagged an exception) -> COMPLETED (only via the explicit two-person
-- checker-verification action, POST .../checker-confirm — never
-- auto-derived from line activity the way Picking/Relocation's own
-- COMPLETED is, since Phase 33C explicitly asks for a *second person*
-- to confirm before completion) or CANCELLED. assigned_packer_id/
-- assigned_at mirror picking_tasks' own AssignedPickerID/AssignedAt
-- (migrations/0082) exactly — a planning/ownership annotation, not an
-- authorization gate. checker_id/checker_confirmed_at are written only
-- by a successful checker-confirm.
--
-- One CREATE TABLE statement — every column and FK is new together, so
-- none of the ADD-COLUMN-then-ADD-CONSTRAINT batch-compilation risk
-- migrations/0050/0053/0065/0067/0084 document applies (same reasoning
-- as relocation_tasks, migrations/0076).
CREATE TABLE packing_jobs (
    id                      BIGINT        IDENTITY(1,1) NOT NULL,
    tenant_id               BIGINT        NOT NULL,
    job_no                  VARCHAR(30)   NOT NULL,
    picking_task_id         BIGINT        NOT NULL,
    order_no                VARCHAR(50)   NOT NULL,
    customer                NVARCHAR(150) NULL,
    customer_id             BIGINT        NULL,
    ship_to_id              BIGINT        NULL,
    ship_to_name            NVARCHAR(200) NULL,
    ship_to_address         NVARCHAR(500) NULL,
    ship_to_province        VARCHAR(100)  NULL,
    ship_to_district        VARCHAR(100)  NULL,
    ship_to_postal_code     VARCHAR(20)   NULL,
    ship_to_contact_person  VARCHAR(150)  NULL,
    ship_to_contact_phone   VARCHAR(50)   NULL,
    warehouse_id            BIGINT        NOT NULL,
    status                  VARCHAR(20)   NOT NULL CONSTRAINT DF_packing_jobs_status DEFAULT ('DRAFT'),
    assigned_packer_id      BIGINT        NULL,
    assigned_at             DATETIME2     NULL,
    checker_id              BIGINT        NULL,
    checker_confirmed_at    DATETIME2     NULL,
    remark                  NVARCHAR(500) NULL,
    created_by              BIGINT        NULL,
    created_at              DATETIME2     NOT NULL CONSTRAINT DF_packing_jobs_created_at DEFAULT (SYSUTCDATETIME()),
    updated_at              DATETIME2     NOT NULL CONSTRAINT DF_packing_jobs_updated_at DEFAULT (SYSUTCDATETIME()),
    completed_at            DATETIME2     NULL,
    cancelled_at            DATETIME2     NULL,
    cancelled_by            BIGINT        NULL,
    cancel_remark           NVARCHAR(500) NULL,
    CONSTRAINT PK_packing_jobs PRIMARY KEY (id),
    CONSTRAINT UQ_packing_jobs_tenant_job_no UNIQUE (tenant_id, job_no),
    CONSTRAINT UQ_packing_jobs_tenant_picking_task UNIQUE (tenant_id, picking_task_id),
    CONSTRAINT FK_packing_jobs_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id),
    CONSTRAINT FK_packing_jobs_picking_task FOREIGN KEY (picking_task_id) REFERENCES picking_tasks(id),
    CONSTRAINT FK_packing_jobs_warehouse FOREIGN KEY (warehouse_id) REFERENCES warehouses(id),
    CONSTRAINT FK_packing_jobs_assigned_packer FOREIGN KEY (assigned_packer_id) REFERENCES users(id),
    CONSTRAINT FK_packing_jobs_checker FOREIGN KEY (checker_id) REFERENCES users(id),
    CONSTRAINT FK_packing_jobs_created_by FOREIGN KEY (created_by) REFERENCES users(id),
    CONSTRAINT FK_packing_jobs_cancelled_by FOREIGN KEY (cancelled_by) REFERENCES users(id),
    CONSTRAINT CK_packing_jobs_status CHECK (status IN ('DRAFT', 'IN_PROGRESS', 'COMPLETED', 'CANCELLED'))
);

CREATE INDEX IX_packing_jobs_tenant_id ON packing_jobs(tenant_id);
CREATE INDEX IX_packing_jobs_warehouse_id ON packing_jobs(warehouse_id);
CREATE INDEX IX_packing_jobs_status ON packing_jobs(status);
CREATE INDEX IX_packing_jobs_order_no ON packing_jobs(order_no);
