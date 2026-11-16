-- Phase 32 — Stock Relocation: relocation_tasks is the header for an
-- internal, intra-warehouse stock transfer, independent of Receiving/
-- Putaway (it never calls AddStock — see internal/relocation's own
-- package doc). tenant_id is included from creation (unlike putaway_
-- tasks' retrofit in migrations/0044/0048), matching the pattern every
-- table created since Phase 29A (e.g. qc_inspections, migrations/0070)
-- already uses.
--
-- One CREATE TABLE statement — every column and FK is new together, so
-- none of the ADD-COLUMN-then-ADD-CONSTRAINT batch-compilation risk
-- migrations/0050/0053/0065/0067 document applies.
--
-- warehouse_id is singular (not a from/to warehouse pair): Phase 32
-- scopes relocation to moving stock between locations WITHIN one
-- warehouse, the same cross-warehouse guard shape putaway_tasks already
-- uses (see internal/relocation/service.go's ErrCrossWarehouse). A
-- future phase can add cross-warehouse transfer as its own extension
-- without reshaping this table.
--
-- Status includes READY as an explicit, separate state from DRAFT and
-- IN_PROGRESS — unlike putaway_tasks (PENDING/IN_PROGRESS/COMPLETED/
-- ISSUE/CANCELLED), Phase 32 was specifically asked for a
-- Draft -> Ready -> In Progress -> Completed/Cancelled machine, so a
-- task's lines can be locked for review before any confirm call is
-- allowed. No ISSUE state: no per-line "report issue" action was
-- requested for Phase 32 (unlike putaway's Report Issue button).
--
-- Cancellation audit columns (cancelled_at/cancelled_by/cancel_remark)
-- are included from creation, matching the shape Phase 18 retrofitted
-- onto picking_tasks/receipts/putaway_tasks/shipments in
-- migrations/0032 — no later ALTER needed here.
CREATE TABLE relocation_tasks (
    id                BIGINT        IDENTITY(1,1) NOT NULL,
    tenant_id         BIGINT        NOT NULL,
    relocation_no     VARCHAR(30)   NOT NULL,
    warehouse_id      BIGINT        NOT NULL,
    status            VARCHAR(20)   NOT NULL CONSTRAINT DF_relocation_tasks_status DEFAULT ('DRAFT'),
    remark            NVARCHAR(500) NULL,
    created_by        BIGINT        NULL,
    created_at        DATETIME2     NOT NULL CONSTRAINT DF_relocation_tasks_created_at DEFAULT (SYSUTCDATETIME()),
    updated_at        DATETIME2     NOT NULL CONSTRAINT DF_relocation_tasks_updated_at DEFAULT (SYSUTCDATETIME()),
    ready_at          DATETIME2     NULL,
    completed_at      DATETIME2     NULL,
    cancelled_at      DATETIME2     NULL,
    cancelled_by      BIGINT        NULL,
    cancel_remark     NVARCHAR(500) NULL,
    CONSTRAINT PK_relocation_tasks PRIMARY KEY (id),
    CONSTRAINT UQ_relocation_tasks_tenant_relocation_no UNIQUE (tenant_id, relocation_no),
    CONSTRAINT FK_relocation_tasks_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id),
    CONSTRAINT FK_relocation_tasks_warehouse FOREIGN KEY (warehouse_id) REFERENCES warehouses(id),
    CONSTRAINT FK_relocation_tasks_created_by FOREIGN KEY (created_by) REFERENCES users(id),
    CONSTRAINT FK_relocation_tasks_cancelled_by FOREIGN KEY (cancelled_by) REFERENCES users(id),
    CONSTRAINT CK_relocation_tasks_status CHECK (status IN
        ('DRAFT', 'READY', 'IN_PROGRESS', 'COMPLETED', 'CANCELLED'))
);

CREATE INDEX IX_relocation_tasks_tenant_id ON relocation_tasks(tenant_id);
CREATE INDEX IX_relocation_tasks_warehouse_id ON relocation_tasks(warehouse_id);
CREATE INDEX IX_relocation_tasks_status ON relocation_tasks(status);
