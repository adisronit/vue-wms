-- EWTA Customer Master + Multi Ship-to: tenant-scoped Customer Delivery
-- Locations (1:N) — the branches, distribution centers, and delivery
-- addresses a Thai domestic B2B customer orders against, distinct from
-- the customer's own billing address (customers.address). Brand new
-- table with no existing rows, so every constraint (NOT NULL, FK,
-- CHECK, the two UNIQUE indexes below) is created in one step, the same
-- "no phased nullable -> backfill dance needed" reasoning migrations/
-- 0049 gives for product_categories/customers/suppliers.
--
-- tenant_id is carried directly on this table (not inferred solely via
-- customer_id's own tenant_id) — the same defense-in-depth convention
-- every other tenant-owned master-data table in this schema follows
-- (see internal/productcategory/repository.go's doc): Service/
-- Repository always set it explicitly from a verified request context,
-- and it is re-checked against the row's own FindByID/Exists lookups,
-- so a location can never be read, updated, or deactivated through a
-- customer_id that belongs to a different tenant even if a future bug
-- skips the owning-customer check.
CREATE TABLE customer_delivery_locations (
    id               BIGINT IDENTITY(1,1) NOT NULL,
    tenant_id        BIGINT         NOT NULL CONSTRAINT DF_customer_delivery_locations_tenant_id DEFAULT (1),
    customer_id      BIGINT         NOT NULL,
    code             VARCHAR(50)    NOT NULL,
    name             NVARCHAR(200)  NOT NULL,
    -- BRANCH/DISTRIBUTION_CENTER/DELIVERY_ADDRESS are exactly the three
    -- kinds of ship-to location named in this phase's own goal statement
    -- ("branches, distribution centers and delivery addresses") — not an
    -- invented taxonomy. Allows '' alongside NULL for the same reason
    -- migrations/0050's locations.type CHECK does: a Go non-pointer
    -- string field's zero value is '' on every INSERT/UPDATE, never
    -- actual SQL NULL.
    type             VARCHAR(30)    NULL,
    address          NVARCHAR(500)  NULL,
    province         NVARCHAR(100)  NULL,
    district         NVARCHAR(100)  NULL,
    postal_code      VARCHAR(20)    NULL,
    contact_person   VARCHAR(150)   NULL,
    contact_phone    VARCHAR(50)    NULL,
    receiving_window NVARCHAR(200)  NULL,
    instructions     NVARCHAR(1000) NULL,
    is_default       BIT            NOT NULL CONSTRAINT DF_customer_delivery_locations_is_default DEFAULT (0),
    status           VARCHAR(20)    NOT NULL CONSTRAINT DF_customer_delivery_locations_status DEFAULT ('ACTIVE'),
    created_at       DATETIME2      NOT NULL CONSTRAINT DF_customer_delivery_locations_created_at DEFAULT (SYSUTCDATETIME()),
    updated_at       DATETIME2      NOT NULL CONSTRAINT DF_customer_delivery_locations_updated_at DEFAULT (SYSUTCDATETIME()),
    CONSTRAINT PK_customer_delivery_locations PRIMARY KEY (id),
    CONSTRAINT FK_customer_delivery_locations_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id),
    CONSTRAINT FK_customer_delivery_locations_customer FOREIGN KEY (customer_id) REFERENCES customers(id),
    -- Ship-to codes are scoped to their own customer, not the whole
    -- tenant (task's own instruction: "prevent duplicate ship-to codes
    -- per customer") — two different customers of the same tenant may
    -- both use code "001" for their own head office.
    CONSTRAINT UQ_customer_delivery_locations_customer_code UNIQUE (customer_id, code),
    CONSTRAINT CK_customer_delivery_locations_type CHECK (type IS NULL OR type = '' OR type IN ('BRANCH', 'DISTRIBUTION_CENTER', 'DELIVERY_ADDRESS')),
    CONSTRAINT CK_customer_delivery_locations_status CHECK (status IN ('ACTIVE', 'INACTIVE'))
);
CREATE INDEX IX_customer_delivery_locations_tenant_id ON customer_delivery_locations(tenant_id);
CREATE INDEX IX_customer_delivery_locations_customer_id ON customer_delivery_locations(customer_id);

-- Enforces "at most one default ship-to per customer" at the DB level —
-- a filtered unique index only indexes rows where is_default = 1, so it
-- rejects a second default for the same customer but places no
-- constraint at all on non-default rows. The complementary half of the
-- task's invariant ("exactly one default when locations exist" — i.e.
-- at least one, not just at most one) cannot be expressed as a single-
-- table constraint across multiple rows and is enforced in
-- Service instead (first location for a customer is always forced
-- default; removing/deactivating the current default is rejected while
-- another ACTIVE location still exists for that customer).
CREATE UNIQUE INDEX UQ_customer_delivery_locations_one_default
    ON customer_delivery_locations(customer_id)
    WHERE is_default = 1;
