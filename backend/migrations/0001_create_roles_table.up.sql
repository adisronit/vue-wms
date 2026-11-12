-- Roles catalog. Codes match the values already used by the frontend's
-- role <select> options (ADMIN, WAREHOUSE_MANAGER, RECEIVING, QC, PUTAWAY,
-- PICKING, SHIPPING) so no frontend value mapping is needed.
CREATE TABLE roles (
    id          BIGINT IDENTITY(1,1) NOT NULL,
    code        VARCHAR(50)   NOT NULL,
    name        VARCHAR(100)  NOT NULL,
    description NVARCHAR(255) NULL,
    is_active   BIT           NOT NULL CONSTRAINT DF_roles_is_active DEFAULT (1),
    created_at  DATETIME2     NOT NULL CONSTRAINT DF_roles_created_at DEFAULT (SYSUTCDATETIME()),
    updated_at  DATETIME2     NOT NULL CONSTRAINT DF_roles_updated_at DEFAULT (SYSUTCDATETIME()),
    CONSTRAINT PK_roles PRIMARY KEY (id),
    CONSTRAINT UQ_roles_code UNIQUE (code)
);
