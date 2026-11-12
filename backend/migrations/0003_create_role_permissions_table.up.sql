-- Many-to-many grant: which permissions a role currently has. One user
-- has exactly one role (see 0004_create_users_table), so a role's granted
-- permissions are effectively the permission set for every user in that
-- role.
CREATE TABLE role_permissions (
    role_id       BIGINT NOT NULL,
    permission_id BIGINT NOT NULL,
    CONSTRAINT PK_role_permissions PRIMARY KEY (role_id, permission_id),
    CONSTRAINT FK_role_permissions_role
        FOREIGN KEY (role_id) REFERENCES roles(id) ON DELETE CASCADE,
    CONSTRAINT FK_role_permissions_permission
        FOREIGN KEY (permission_id) REFERENCES permissions(id) ON DELETE CASCADE
);

CREATE INDEX IX_role_permissions_permission_id ON role_permissions(permission_id);
