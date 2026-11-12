-- Bin locations belong to exactly one zone, matching WarehouseView.vue's
-- Location Structure tab (zone -> aisle -> rack -> bin tree, e.g. bin
-- code "A-01-01-01" under zone "A"). aisle/rack/bin are kept as separate
-- columns (not parsed back out of code at read time) so the tree view
-- can group without string parsing.
CREATE TABLE locations (
    id          BIGINT IDENTITY(1,1) NOT NULL,
    zone_id     BIGINT      NOT NULL,
    code        VARCHAR(50) NOT NULL,
    aisle       VARCHAR(10) NULL,
    rack        VARCHAR(10) NULL,
    bin         VARCHAR(10) NULL,
    status      VARCHAR(20) NOT NULL CONSTRAINT DF_locations_status DEFAULT ('ACTIVE'),
    created_at  DATETIME2   NOT NULL CONSTRAINT DF_locations_created_at DEFAULT (SYSUTCDATETIME()),
    updated_at  DATETIME2   NOT NULL CONSTRAINT DF_locations_updated_at DEFAULT (SYSUTCDATETIME()),
    CONSTRAINT PK_locations PRIMARY KEY (id),
    CONSTRAINT FK_locations_zone FOREIGN KEY (zone_id) REFERENCES zones(id),
    CONSTRAINT UQ_locations_zone_code UNIQUE (zone_id, code),
    CONSTRAINT CK_locations_status CHECK (status IN ('ACTIVE', 'INACTIVE'))
);

CREATE INDEX IX_locations_zone_id ON locations(zone_id);
