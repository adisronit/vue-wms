-- Warehouse master data, matching WarehouseView.vue's Warehouse Info tab.
-- capacity is basic master data (a planning figure the admin sets); it is
-- deliberately NOT paired with an "occupied" column here — that would
-- require real inventory quantities, which are out of scope this phase.
-- Utilization is therefore not computable yet and the API returns it as
-- null until the Inventory phase exists.
CREATE TABLE warehouses (
    id              BIGINT IDENTITY(1,1) NOT NULL,
    code            VARCHAR(20)   NOT NULL,
    name            VARCHAR(150)  NOT NULL,
    type            VARCHAR(50)   NOT NULL,
    address         NVARCHAR(500) NULL,
    contact_person  VARCHAR(150)  NULL,
    phone           VARCHAR(30)   NULL,
    capacity        INT           NULL,
    status          VARCHAR(20)   NOT NULL CONSTRAINT DF_warehouses_status DEFAULT ('ACTIVE'),
    created_at      DATETIME2     NOT NULL CONSTRAINT DF_warehouses_created_at DEFAULT (SYSUTCDATETIME()),
    updated_at      DATETIME2     NOT NULL CONSTRAINT DF_warehouses_updated_at DEFAULT (SYSUTCDATETIME()),
    CONSTRAINT PK_warehouses PRIMARY KEY (id),
    CONSTRAINT UQ_warehouses_code UNIQUE (code),
    CONSTRAINT CK_warehouses_status CHECK (status IN ('ACTIVE', 'INACTIVE'))
);
