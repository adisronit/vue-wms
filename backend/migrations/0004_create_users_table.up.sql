-- status carries three real frontend states (ACTIVE/INACTIVE/LOCKED) --
-- a plain is_active boolean would lose the LOCKED distinction the User
-- Management screen already relies on. role_id is a single FK: the
-- frontend's user form uses a single-select role field, not multi-select.
CREATE TABLE users (
    id            BIGINT IDENTITY(1,1) NOT NULL,
    username      VARCHAR(50)   NOT NULL,
    password_hash VARCHAR(255)  NOT NULL,
    full_name     NVARCHAR(150) NOT NULL,
    email         VARCHAR(150)  NOT NULL,
    role_id       BIGINT        NOT NULL,
    status        VARCHAR(20)   NOT NULL CONSTRAINT DF_users_status DEFAULT ('ACTIVE'),
    created_at    DATETIME2     NOT NULL CONSTRAINT DF_users_created_at DEFAULT (SYSUTCDATETIME()),
    updated_at    DATETIME2     NOT NULL CONSTRAINT DF_users_updated_at DEFAULT (SYSUTCDATETIME()),
    CONSTRAINT PK_users PRIMARY KEY (id),
    CONSTRAINT UQ_users_username UNIQUE (username),
    CONSTRAINT UQ_users_email UNIQUE (email),
    CONSTRAINT FK_users_role FOREIGN KEY (role_id) REFERENCES roles(id),
    CONSTRAINT CK_users_status CHECK (status IN ('ACTIVE', 'INACTIVE', 'LOCKED'))
);

CREATE INDEX IX_users_role_id ON users(role_id);
CREATE INDEX IX_users_status ON users(status);
