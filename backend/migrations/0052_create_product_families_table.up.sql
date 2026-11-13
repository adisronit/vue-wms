-- EWTA Product Master enhancement: tenant-scoped Product Family
-- master (groups sellable SKUs/variants under one family, e.g.
-- "Cheddar Block" family with 150g/300g variants) — same minimal flat
-- shape as brands above.
CREATE TABLE product_families (
    id         BIGINT IDENTITY(1,1) NOT NULL,
    tenant_id  BIGINT       NOT NULL CONSTRAINT DF_product_families_tenant_id DEFAULT (1),
    code       VARCHAR(50)  NOT NULL,
    name       VARCHAR(200) NOT NULL,
    status     VARCHAR(20)  NOT NULL CONSTRAINT DF_product_families_status DEFAULT ('ACTIVE'),
    created_at DATETIME2    NOT NULL CONSTRAINT DF_product_families_created_at DEFAULT (SYSUTCDATETIME()),
    updated_at DATETIME2    NOT NULL CONSTRAINT DF_product_families_updated_at DEFAULT (SYSUTCDATETIME()),
    CONSTRAINT PK_product_families PRIMARY KEY (id),
    CONSTRAINT FK_product_families_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id),
    CONSTRAINT UQ_product_families_tenant_code UNIQUE (tenant_id, code),
    CONSTRAINT CK_product_families_status CHECK (status IN ('ACTIVE', 'INACTIVE'))
);
CREATE INDEX IX_product_families_tenant_id ON product_families(tenant_id);
