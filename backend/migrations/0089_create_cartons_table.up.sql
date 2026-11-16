-- Phase 33C — Packing: cartons is the physical container a job's picked
-- quantities get boxed into. lpn (license-plate number) is this table's
-- own system-generated, UNIQUE-within-tenant identifier — unlike
-- relocation_task_items.lpn (migrations/0077), which is a nullable,
-- non-unique, purely descriptive/audit column a user may optionally
-- type in, cartons.lpn is this row's primary handle: it is what the
-- Carton/Pallet Label PDF's barcode encodes and what a picker/checker
-- scans to identify the carton, so it is generated server-side from the
-- new IDENTITY id (the same race-free-by-construction "CTN-%06d"
-- pattern as packing_jobs.job_no / picking_tasks.pick_no) and is never
-- user-supplied.
--
-- status OPEN (still accepting packed items) -> CLOSED (sealed, no
-- further packing into it — see internal/packing/service.go's
-- CanPackInto doc) is the only state machine this table needs; there is
-- no CANCELLED state because a carton is never an independent
-- transactional unit the way a job is — it is discarded by simply never
-- being used again, not by a tracked cancellation.
--
-- packaging_type/weight/dimensions are plain descriptive metadata for
-- the Carton/Pallet Label PDF and operator display — never consulted by
-- any inventory call (this package makes none at all, see
-- packing_jobs' own doc).
CREATE TABLE cartons (
    id                  BIGINT        IDENTITY(1,1) NOT NULL,
    tenant_id           BIGINT        NOT NULL,
    packing_job_id      BIGINT        NOT NULL,
    lpn                 VARCHAR(30)   NOT NULL,
    packaging_type      VARCHAR(20)   NOT NULL CONSTRAINT DF_cartons_packaging_type DEFAULT ('BOX'),
    gross_weight        DECIMAL(10,3) NULL,
    net_weight          DECIMAL(10,3) NULL,
    weight_unit         VARCHAR(10)   NOT NULL CONSTRAINT DF_cartons_weight_unit DEFAULT ('KG'),
    carton_length       DECIMAL(10,2) NULL,
    carton_width        DECIMAL(10,2) NULL,
    carton_height       DECIMAL(10,2) NULL,
    dimension_unit      VARCHAR(10)   NOT NULL CONSTRAINT DF_cartons_dimension_unit DEFAULT ('CM'),
    status              VARCHAR(20)   NOT NULL CONSTRAINT DF_cartons_status DEFAULT ('OPEN'),
    remark              NVARCHAR(500) NULL,
    created_by          BIGINT        NULL,
    created_at          DATETIME2     NOT NULL CONSTRAINT DF_cartons_created_at DEFAULT (SYSUTCDATETIME()),
    updated_at          DATETIME2     NOT NULL CONSTRAINT DF_cartons_updated_at DEFAULT (SYSUTCDATETIME()),
    CONSTRAINT PK_cartons PRIMARY KEY (id),
    CONSTRAINT UQ_cartons_tenant_lpn UNIQUE (tenant_id, lpn),
    CONSTRAINT FK_cartons_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id),
    CONSTRAINT FK_cartons_job FOREIGN KEY (packing_job_id) REFERENCES packing_jobs(id),
    CONSTRAINT FK_cartons_created_by FOREIGN KEY (created_by) REFERENCES users(id),
    CONSTRAINT CK_cartons_status CHECK (status IN ('OPEN', 'CLOSED')),
    CONSTRAINT CK_cartons_packaging_type CHECK (packaging_type IN ('BOX', 'PALLET', 'BAG', 'CRATE', 'OTHER')),
    CONSTRAINT CK_cartons_weight_unit CHECK (weight_unit IN ('KG', 'LB')),
    CONSTRAINT CK_cartons_dimension_unit CHECK (dimension_unit IN ('CM', 'IN')),
    CONSTRAINT CK_cartons_gross_weight CHECK (gross_weight IS NULL OR gross_weight >= 0),
    CONSTRAINT CK_cartons_net_weight CHECK (net_weight IS NULL OR net_weight >= 0)
);

CREATE INDEX IX_cartons_tenant_id ON cartons(tenant_id);
CREATE INDEX IX_cartons_job_id ON cartons(packing_job_id);
