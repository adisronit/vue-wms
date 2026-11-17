-- Phase 34I — Interactive 2D Warehouse Layout, step 2 of 3.
-- warehouse_layout_objects is one drawn shape belonging to exactly one
-- warehouse_layout_versions row (migrations/0120) — a rack, aisle,
-- dock, wall, free-text label, or generic shape, positioned in the
-- version's own 2D coordinate space (x/y is the shape's top-left
-- corner; rotation_degrees is applied about its own center by the
-- frontend renderer, never persisted pre-rotated).
--
-- reference_type/reference_id optionally link a shape back to a real
-- zone or location row PURELY for display (so a caller can show "this
-- rectangle is Zone RECEIVING-A" using that row's own real name/code)
-- — never enforced by a foreign key, since reference_type is
-- polymorphic (ZONE or LOCATION) and the referenced row's own tenant
-- scoping is re-verified in application code (Service.resolveReference)
-- rather than at the database level, the same polymorphic-reference
-- shape internal/receiving's own document_renders table already uses
-- for a different polymorphic case. Deleting a referenced zone/
-- location never cascades here — a layout shape with a now-dangling
-- reference simply falls back to displaying its own free-text label.
CREATE TABLE warehouse_layout_objects (
    id                  BIGINT        IDENTITY(1,1) NOT NULL,
    tenant_id           BIGINT        NOT NULL,
    layout_version_id   BIGINT        NOT NULL,
    object_type         VARCHAR(20)   NOT NULL,
    reference_type      VARCHAR(20)   NULL,
    reference_id        BIGINT        NULL,
    x                   FLOAT         NOT NULL,
    y                   FLOAT         NOT NULL,
    width               FLOAT         NOT NULL,
    height              FLOAT         NOT NULL,
    rotation_degrees    FLOAT         NOT NULL CONSTRAINT DF_warehouse_layout_objects_rotation DEFAULT (0),
    fill_color          VARCHAR(9)    NULL,
    stroke_color        VARCHAR(9)    NULL,
    label               NVARCHAR(200) NULL,
    z_index             INT           NOT NULL CONSTRAINT DF_warehouse_layout_objects_z_index DEFAULT (0),
    created_at          DATETIME2     NOT NULL CONSTRAINT DF_warehouse_layout_objects_created_at DEFAULT (SYSUTCDATETIME()),
    CONSTRAINT PK_warehouse_layout_objects PRIMARY KEY (id),
    CONSTRAINT FK_warehouse_layout_objects_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id),
    CONSTRAINT FK_warehouse_layout_objects_version FOREIGN KEY (layout_version_id) REFERENCES warehouse_layout_versions(id),
    CONSTRAINT CK_warehouse_layout_objects_type CHECK (object_type IN ('ZONE', 'LOCATION', 'RACK', 'AISLE', 'DOCK', 'WALL', 'LABEL', 'SHAPE')),
    CONSTRAINT CK_warehouse_layout_objects_reference_type CHECK (reference_type IS NULL OR reference_type IN ('ZONE', 'LOCATION')),
    CONSTRAINT CK_warehouse_layout_objects_size CHECK (width > 0 AND height > 0)
);

CREATE INDEX IX_warehouse_layout_objects_version ON warehouse_layout_objects(layout_version_id);
CREATE INDEX IX_warehouse_layout_objects_tenant ON warehouse_layout_objects(tenant_id);
