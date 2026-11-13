-- Master Data Expansion: three new tenant-scoped master-data tables
-- (Product Categories, Customers, Suppliers) plus an additive,
-- nullable category_id link from products to the new category
-- hierarchy. Unlike migrations 0043-0048's phased nullable -> backfill
-- -> not-null-and-FK -> tenant-scoped-unique dance (required there
-- because those tables already held data under an implicit tenant_id
-- of 1), these are brand-new tables with no existing rows, so tenant_id
-- can go straight to NOT NULL + FK + tenant-scoped UNIQUE in one
-- migration — the same shape 0047/0048 left every other master-data
-- table in, just reached in a single step instead of four.
--
-- The DEFAULT (1) on tenant_id is kept purely for consistency with
-- every other tenant-owned table in this schema (products, warehouses,
-- ...) in case a future direct-SQL/tooling insert ever omits it — unlike
-- those tables' Go models, this module's Service/Repository always set
-- TenantID explicitly from a verified request context and never rely on
-- this default (see internal/productcategory/repository.go's doc).
CREATE TABLE product_categories (
    id          BIGINT IDENTITY(1,1) NOT NULL,
    tenant_id   BIGINT         NOT NULL CONSTRAINT DF_product_categories_tenant_id DEFAULT (1),
    code        VARCHAR(50)    NOT NULL,
    name        VARCHAR(200)   NOT NULL,
    parent_id   BIGINT         NULL,
    description NVARCHAR(1000) NULL,
    status      VARCHAR(20)    NOT NULL CONSTRAINT DF_product_categories_status DEFAULT ('ACTIVE'),
    created_at  DATETIME2      NOT NULL CONSTRAINT DF_product_categories_created_at DEFAULT (SYSUTCDATETIME()),
    updated_at  DATETIME2      NOT NULL CONSTRAINT DF_product_categories_updated_at DEFAULT (SYSUTCDATETIME()),
    CONSTRAINT PK_product_categories PRIMARY KEY (id),
    CONSTRAINT FK_product_categories_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id),
    CONSTRAINT FK_product_categories_parent FOREIGN KEY (parent_id) REFERENCES product_categories(id),
    CONSTRAINT UQ_product_categories_tenant_code UNIQUE (tenant_id, code),
    CONSTRAINT CK_product_categories_status CHECK (status IN ('ACTIVE', 'INACTIVE'))
);
CREATE INDEX IX_product_categories_tenant_id ON product_categories(tenant_id);
CREATE INDEX IX_product_categories_parent_id ON product_categories(parent_id);

-- Customers and Suppliers are deliberately identical in shape (code,
-- company_name, tax_id, one contact person/phone, address, email,
-- phone, status) — the task's own field list for both, modeled as
-- plain columns rather than a child "contacts" table since nothing in
-- the current frontend/workflow needs more than one contact per
-- party, and a child table with no consumer would be an unused
-- abstraction (see CLAUDE.md's "avoid unnecessary abstractions").
CREATE TABLE customers (
    id             BIGINT IDENTITY(1,1) NOT NULL,
    tenant_id      BIGINT        NOT NULL CONSTRAINT DF_customers_tenant_id DEFAULT (1),
    code           VARCHAR(50)   NOT NULL,
    company_name   VARCHAR(200)  NOT NULL,
    tax_id         VARCHAR(50)   NULL,
    contact_person VARCHAR(150)  NULL,
    contact_phone  VARCHAR(50)   NULL,
    address        NVARCHAR(500) NULL,
    email          VARCHAR(200)  NULL,
    phone          VARCHAR(50)   NULL,
    status         VARCHAR(20)   NOT NULL CONSTRAINT DF_customers_status DEFAULT ('ACTIVE'),
    created_at     DATETIME2     NOT NULL CONSTRAINT DF_customers_created_at DEFAULT (SYSUTCDATETIME()),
    updated_at     DATETIME2     NOT NULL CONSTRAINT DF_customers_updated_at DEFAULT (SYSUTCDATETIME()),
    CONSTRAINT PK_customers PRIMARY KEY (id),
    CONSTRAINT FK_customers_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id),
    CONSTRAINT UQ_customers_tenant_code UNIQUE (tenant_id, code),
    CONSTRAINT CK_customers_status CHECK (status IN ('ACTIVE', 'INACTIVE'))
);
CREATE INDEX IX_customers_tenant_id ON customers(tenant_id);

CREATE TABLE suppliers (
    id             BIGINT IDENTITY(1,1) NOT NULL,
    tenant_id      BIGINT        NOT NULL CONSTRAINT DF_suppliers_tenant_id DEFAULT (1),
    code           VARCHAR(50)   NOT NULL,
    company_name   VARCHAR(200)  NOT NULL,
    tax_id         VARCHAR(50)   NULL,
    contact_person VARCHAR(150)  NULL,
    contact_phone  VARCHAR(50)   NULL,
    address        NVARCHAR(500) NULL,
    email          VARCHAR(200)  NULL,
    phone          VARCHAR(50)   NULL,
    status         VARCHAR(20)   NOT NULL CONSTRAINT DF_suppliers_status DEFAULT ('ACTIVE'),
    created_at     DATETIME2     NOT NULL CONSTRAINT DF_suppliers_created_at DEFAULT (SYSUTCDATETIME()),
    updated_at     DATETIME2     NOT NULL CONSTRAINT DF_suppliers_updated_at DEFAULT (SYSUTCDATETIME()),
    CONSTRAINT PK_suppliers PRIMARY KEY (id),
    CONSTRAINT FK_suppliers_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id),
    CONSTRAINT UQ_suppliers_tenant_code UNIQUE (tenant_id, code),
    CONSTRAINT CK_suppliers_status CHECK (status IN ('ACTIVE', 'INACTIVE'))
);
CREATE INDEX IX_suppliers_tenant_id ON suppliers(tenant_id);

-- Additive, nullable link from products to the new category hierarchy —
-- the existing `category` fixed-enum column (CK_products_category) is
-- completely untouched; a product may have either, both, or neither.
ALTER TABLE products ADD category_id BIGINT NULL;
ALTER TABLE products ADD CONSTRAINT FK_products_category FOREIGN KEY (category_id) REFERENCES product_categories(id);
CREATE INDEX IX_products_category_id ON products(category_id);
