-- Phase 34I — Interactive 2D Warehouse Layout, step 1 of 3.
-- warehouse_layout_versions is the header row for one saved snapshot
-- of a warehouse's 2D layout. Every Save (Service.SaveLayout) creates
-- a NEW version row rather than updating one in place — version_no is
-- a simple per-warehouse incrementing counter, kept purely for
-- display/history ("Version 3, saved 2026-11-17"); there is no
-- separate DRAFT/PUBLISHED workflow — the highest version_no per
-- (tenant_id, warehouse_id) is always what Service.GetLayout and the
-- viewer return, and every older version remains readable as history/
-- rollback reference via its own id, never deleted automatically.
--
-- background_image_id/width_px/height_px and scale_px_per_meter
-- together are the "optional background image import/calibration"
-- requirement: a version may have no background image at all
-- (background_image_id NULL, scale_px_per_meter NULL — a caller must
-- show "not calibrated" rather than fabricate a scale), or a real
-- uploaded image plus a user-entered calibration (how many on-canvas
-- pixels equal one real-world meter, established by the user drawing
-- a reference line of known length — see the frontend's own
-- calibration step).
--
-- This package performs NO mutation of zones/locations/warehouses —
-- warehouse_layout_objects (migrations/0121) may optionally reference
-- a real zone/location row purely for display, but this table and its
-- sibling never write back to those tables, matching CLAUDE.md's "no
-- stock mutation from layout editing" requirement (broadened here to
-- "no master-data mutation at all" — a layout is a visual overlay,
-- never a source of truth for warehouse structure).
CREATE TABLE warehouse_layout_versions (
    id                      BIGINT        IDENTITY(1,1) NOT NULL,
    tenant_id               BIGINT        NOT NULL,
    warehouse_id            BIGINT        NOT NULL,
    version_no              INT           NOT NULL,
    background_image_id     BIGINT        NULL,
    background_image_width_px  INT        NULL,
    background_image_height_px INT        NULL,
    scale_px_per_meter      FLOAT         NULL,
    remark                  NVARCHAR(500) NULL,
    created_by              BIGINT        NULL,
    created_at              DATETIME2     NOT NULL CONSTRAINT DF_warehouse_layout_versions_created_at DEFAULT (SYSUTCDATETIME()),
    CONSTRAINT PK_warehouse_layout_versions PRIMARY KEY (id),
    CONSTRAINT UQ_warehouse_layout_versions_tenant_warehouse_version UNIQUE (tenant_id, warehouse_id, version_no),
    CONSTRAINT FK_warehouse_layout_versions_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id),
    CONSTRAINT FK_warehouse_layout_versions_warehouse FOREIGN KEY (warehouse_id) REFERENCES warehouses(id),
    CONSTRAINT FK_warehouse_layout_versions_created_by FOREIGN KEY (created_by) REFERENCES users(id),
    CONSTRAINT CK_warehouse_layout_versions_scale CHECK (scale_px_per_meter IS NULL OR scale_px_per_meter > 0)
);

CREATE INDEX IX_warehouse_layout_versions_tenant_warehouse ON warehouse_layout_versions(tenant_id, warehouse_id);
