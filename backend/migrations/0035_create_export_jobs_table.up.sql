-- Phase 22 — async export job tracking (Report/Export). Only Inventory
-- Transaction History uses this pipeline this phase (an append-only,
-- ever-growing ledger — the one report genuinely expected to outgrow a
-- synchronous request); every other report/export in this phase is
-- synchronous CSV with no job row at all. job_type is a plain string,
-- not a CHECK-constrained enum, so a future async report type doesn't
-- need a migration just to widen the allowed value set.
--
-- file_path is a server-side filesystem path under the shared
-- export_files Docker volume (mounted into both backend and worker),
-- never a value returned to or accepted from the client directly — the
-- download endpoint resolves it internally by job id + requesting-user
-- ownership check, so nothing here becomes a path-traversal surface.
CREATE TABLE export_jobs (
    id             BIGINT IDENTITY(1,1) NOT NULL,
    job_type       VARCHAR(50)   NOT NULL,
    status         VARCHAR(20)   NOT NULL CONSTRAINT DF_export_jobs_status DEFAULT ('PENDING'),
    requested_by   BIGINT        NOT NULL,
    filters_json   NVARCHAR(MAX) NULL,
    file_name      NVARCHAR(255) NULL,
    file_path      NVARCHAR(500) NULL,
    row_count      INT           NULL,
    error_message  NVARCHAR(1000) NULL,
    message_id     VARCHAR(36)   NOT NULL,
    created_at     DATETIME2     NOT NULL CONSTRAINT DF_export_jobs_created_at DEFAULT (SYSUTCDATETIME()),
    started_at     DATETIME2     NULL,
    completed_at   DATETIME2     NULL,
    CONSTRAINT PK_export_jobs PRIMARY KEY (id),
    CONSTRAINT FK_export_jobs_requested_by FOREIGN KEY (requested_by) REFERENCES users(id),
    CONSTRAINT CK_export_jobs_status CHECK (status IN ('PENDING', 'PROCESSING', 'COMPLETED', 'FAILED')),
    CONSTRAINT UQ_export_jobs_message_id UNIQUE (message_id)
);

CREATE INDEX IX_export_jobs_requested_by ON export_jobs(requested_by);
