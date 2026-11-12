-- Permission catalog. code is the enforceable "module.action" identifier
-- (e.g. "users.view"); module/action are kept as separate columns too so
-- the Roles & Permissions screen's module x action matrix can be rendered
-- directly from a single SELECT without parsing the code string.
CREATE TABLE permissions (
    id          BIGINT IDENTITY(1,1) NOT NULL,
    code        VARCHAR(100)  NOT NULL,
    module      VARCHAR(50)   NOT NULL,
    action      VARCHAR(30)   NOT NULL,
    description NVARCHAR(255) NULL,
    CONSTRAINT PK_permissions PRIMARY KEY (id),
    CONSTRAINT UQ_permissions_code UNIQUE (code)
);

CREATE INDEX IX_permissions_module ON permissions(module);
