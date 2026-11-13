-- Makes the Product Master screen's "Barcode Mapping" tab real: it has
-- existed in the frontend since Phase 2 but was always decorative
-- (rebuilt from the single products.barcode column on every open, never
-- persisted) — see internal/product/model.go's own package doc. This
-- table is a plain child of products, no own tenant_id column (the same
-- "scoped via the parent" shape role_warehouse_access already uses):
-- every Repository method here takes the owning product's already-
-- tenant-verified ID, never a bare barcode-row ID a caller could guess
-- across tenants.
CREATE TABLE product_barcodes (
    id               BIGINT IDENTITY(1,1) NOT NULL,
    product_id       BIGINT       NOT NULL,
    barcode_value    VARCHAR(50)  NOT NULL,
    barcode_type     VARCHAR(30)  NOT NULL CONSTRAINT DF_product_barcodes_type DEFAULT ('SUPPLIER_BARCODE'),
    uom              VARCHAR(20)  NULL,
    qty_per_barcode  INT          NOT NULL CONSTRAINT DF_product_barcodes_qty DEFAULT (1),
    is_primary       BIT          NOT NULL CONSTRAINT DF_product_barcodes_is_primary DEFAULT (0),
    status           VARCHAR(20)  NOT NULL CONSTRAINT DF_product_barcodes_status DEFAULT ('ACTIVE'),
    created_at       DATETIME2    NOT NULL CONSTRAINT DF_product_barcodes_created_at DEFAULT (SYSUTCDATETIME()),
    updated_at       DATETIME2    NOT NULL CONSTRAINT DF_product_barcodes_updated_at DEFAULT (SYSUTCDATETIME()),
    CONSTRAINT PK_product_barcodes PRIMARY KEY (id),
    CONSTRAINT FK_product_barcodes_product FOREIGN KEY (product_id) REFERENCES products(id) ON DELETE CASCADE,
    CONSTRAINT UQ_product_barcodes_product_value UNIQUE (product_id, barcode_value),
    CONSTRAINT CK_product_barcodes_type CHECK (barcode_type IN ('SUPPLIER_BARCODE', 'INTERNAL_BARCODE', 'BOX_CODE', 'QR_CODE')),
    CONSTRAINT CK_product_barcodes_status CHECK (status IN ('ACTIVE', 'INACTIVE')),
    CONSTRAINT CK_product_barcodes_qty CHECK (qty_per_barcode > 0)
);
CREATE INDEX IX_product_barcodes_product_id ON product_barcodes(product_id);
