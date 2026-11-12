-- Product master data, matching ProductMasterView.vue's "Product Info"
-- and "Inventory Rules" tabs. barcode is a single optional field, not a
-- child table — the frontend's "Barcode Mapping" tab (multiple barcodes
-- per product with type/UOM/qty) and "UOM Conversion" tab are deferred
-- (see PROJECT_STATUS.md); this is deliberately the minimum schema the
-- task's own guidance called for, not an oversight.
-- default_zone is a plain string, not a FK to zones(id): the frontend's
-- picker (A/B/C) has no warehouse context, so it cannot be resolved to a
-- real zone without inventing a relationship the UI doesn't establish.
CREATE TABLE products (
    id               BIGINT IDENTITY(1,1) NOT NULL,
    sku              VARCHAR(50)   NOT NULL,
    name             VARCHAR(200)  NOT NULL,
    description      NVARCHAR(1000) NULL,
    category         VARCHAR(30)   NOT NULL,
    base_uom         VARCHAR(20)   NOT NULL,
    pack_size        INT           NOT NULL CONSTRAINT DF_products_pack_size DEFAULT (1),
    barcode          VARCHAR(50)   NULL,
    lot_required     BIT           NOT NULL CONSTRAINT DF_products_lot_required DEFAULT (0),
    serial_required  BIT           NOT NULL CONSTRAINT DF_products_serial_required DEFAULT (0),
    expiry_required  BIT           NOT NULL CONSTRAINT DF_products_expiry_required DEFAULT (0),
    default_zone     VARCHAR(10)   NULL,
    min_stock        INT           NOT NULL CONSTRAINT DF_products_min_stock DEFAULT (0),
    max_stock        INT           NOT NULL CONSTRAINT DF_products_max_stock DEFAULT (0),
    status           VARCHAR(20)   NOT NULL CONSTRAINT DF_products_status DEFAULT ('ACTIVE'),
    created_at       DATETIME2     NOT NULL CONSTRAINT DF_products_created_at DEFAULT (SYSUTCDATETIME()),
    updated_at       DATETIME2     NOT NULL CONSTRAINT DF_products_updated_at DEFAULT (SYSUTCDATETIME()),
    CONSTRAINT PK_products PRIMARY KEY (id),
    CONSTRAINT UQ_products_sku UNIQUE (sku),
    CONSTRAINT CK_products_status CHECK (status IN ('ACTIVE', 'INACTIVE')),
    CONSTRAINT CK_products_category CHECK (category IN ('PACKAGING', 'LABEL', 'RFID', 'RAW_MATERIAL')),
    CONSTRAINT CK_products_base_uom CHECK (base_uom IN ('PCS', 'BOX', 'ROLL', 'KG'))
);

-- A plain UNIQUE constraint would only allow a single NULL barcode
-- across the whole table in SQL Server (unlike Postgres/MySQL, which
-- treat NULLs as distinct) — a filtered unique index is the correct way
-- to express "unique when present" here.
CREATE UNIQUE INDEX UQ_products_barcode ON products(barcode) WHERE barcode IS NOT NULL;

CREATE INDEX IX_products_category ON products(category);
CREATE INDEX IX_products_status ON products(status);
