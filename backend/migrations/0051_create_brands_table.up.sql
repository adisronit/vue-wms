-- EWTA Product Master enhancement: tenant-scoped Brand master, the
-- same minimal flat shape (code, name, status) brands.view/create/
-- update/delete will gate — mirrors customers/suppliers' own
-- CREATE-TABLE-with-inline-constraints shape (migrations/0049) exactly,
-- a brand-new table with no existing rows so tenant_id goes straight to
-- NOT NULL + FK + tenant-scoped UNIQUE in one step.
CREATE TABLE brands (
    id         BIGINT IDENTITY(1,1) NOT NULL,
    tenant_id  BIGINT       NOT NULL CONSTRAINT DF_brands_tenant_id DEFAULT (1),
    code       VARCHAR(50)  NOT NULL,
    name       VARCHAR(200) NOT NULL,
    status     VARCHAR(20)  NOT NULL CONSTRAINT DF_brands_status DEFAULT ('ACTIVE'),
    created_at DATETIME2    NOT NULL CONSTRAINT DF_brands_created_at DEFAULT (SYSUTCDATETIME()),
    updated_at DATETIME2    NOT NULL CONSTRAINT DF_brands_updated_at DEFAULT (SYSUTCDATETIME()),
    CONSTRAINT PK_brands PRIMARY KEY (id),
    CONSTRAINT FK_brands_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id),
    CONSTRAINT UQ_brands_tenant_code UNIQUE (tenant_id, code),
    CONSTRAINT CK_brands_status CHECK (status IN ('ACTIVE', 'INACTIVE'))
);
CREATE INDEX IX_brands_tenant_id ON brands(tenant_id);
