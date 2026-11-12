-- Zones belong to exactly one warehouse, matching WarehouseView.vue's
-- Zone Setup tab (a list of zones scoped to the warehouse drawer
-- currently open). type mirrors that tab's fixed <select> options.
CREATE TABLE zones (
    id            BIGINT IDENTITY(1,1) NOT NULL,
    warehouse_id  BIGINT       NOT NULL,
    code          VARCHAR(20)  NOT NULL,
    name          VARCHAR(150) NOT NULL,
    type          VARCHAR(20)  NOT NULL,
    status        VARCHAR(20)  NOT NULL CONSTRAINT DF_zones_status DEFAULT ('ACTIVE'),
    created_at    DATETIME2    NOT NULL CONSTRAINT DF_zones_created_at DEFAULT (SYSUTCDATETIME()),
    updated_at    DATETIME2    NOT NULL CONSTRAINT DF_zones_updated_at DEFAULT (SYSUTCDATETIME()),
    CONSTRAINT PK_zones PRIMARY KEY (id),
    CONSTRAINT FK_zones_warehouse FOREIGN KEY (warehouse_id) REFERENCES warehouses(id),
    CONSTRAINT UQ_zones_warehouse_code UNIQUE (warehouse_id, code),
    CONSTRAINT CK_zones_status CHECK (status IN ('ACTIVE', 'INACTIVE')),
    CONSTRAINT CK_zones_type CHECK (type IN ('RECEIVING', 'STORAGE', 'PICKING', 'PACKING', 'SHIPPING', 'QC', 'HOLD'))
);

CREATE INDEX IX_zones_warehouse_id ON zones(warehouse_id);
