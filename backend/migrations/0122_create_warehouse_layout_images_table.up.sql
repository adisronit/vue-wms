-- Phase 34I — Interactive 2D Warehouse Layout, step 3 of 3.
-- warehouse_layout_images stores one uploaded background image's
-- metadata — the actual bytes live on disk under config.ImageStorageDir
-- (reusing the exact same server-generated-UUID-filename, content-
-- sniffed-not-trusted, tenant-prefixed-path pattern internal/product's
-- own image.go already established for product images), never served
-- through a static mount — only through Handler.ServeImage's own
-- tenant-checked lookup.
--
-- warehouse_layout_versions.background_image_id (migrations/0120)
-- intentionally has NO foreign key reference to this table: it is
-- created first (so a version can exist before any image is ever
-- uploaded against it) and this package's own Service.resolveImage
-- re-verifies tenant ownership in application code on every read,
-- the same deliberately-not-FK-enforced polymorphic-reference shape
-- warehouse_layout_objects.reference_id (migrations/0121) already
-- uses for the same reason.
CREATE TABLE warehouse_layout_images (
    id              BIGINT        IDENTITY(1,1) NOT NULL,
    tenant_id       BIGINT        NOT NULL,
    warehouse_id    BIGINT        NOT NULL,
    file_name       VARCHAR(80)   NOT NULL,
    original_filename NVARCHAR(255) NULL,
    content_type    VARCHAR(50)   NOT NULL,
    byte_size       INT           NOT NULL,
    width_px        INT           NOT NULL,
    height_px       INT           NOT NULL,
    created_by      BIGINT        NULL,
    created_at      DATETIME2     NOT NULL CONSTRAINT DF_warehouse_layout_images_created_at DEFAULT (SYSUTCDATETIME()),
    CONSTRAINT PK_warehouse_layout_images PRIMARY KEY (id),
    CONSTRAINT FK_warehouse_layout_images_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id),
    CONSTRAINT FK_warehouse_layout_images_warehouse FOREIGN KEY (warehouse_id) REFERENCES warehouses(id),
    CONSTRAINT FK_warehouse_layout_images_created_by FOREIGN KEY (created_by) REFERENCES users(id)
);

CREATE INDEX IX_warehouse_layout_images_tenant_warehouse ON warehouse_layout_images(tenant_id, warehouse_id);
