-- Phase 27 — Role & Permission's "Warehouse Access" panel has been
-- local-only mock UI since it was first built: a hardcoded 3-warehouse
-- array (WH-01/02/03, fake names) and roleForm.warehouseAccess that was
-- never sent to or read from the backend (confirmed in
-- RolePermissionView.vue before this migration — no warehouse table or
-- role-warehouse relationship existed anywhere in this schema). This
-- migration adds the real mapping table, following role_permissions'
-- own composite-primary-key join-table shape exactly (migrations/0003).
--
-- created_at/created_by are additional audit columns role_permissions
-- itself doesn't have — warehouse-scoped access is a more security-
-- sensitive grant (which warehouses' data a whole role can touch) than
-- an individual module.action checkbox, so who granted it and when is
-- worth keeping. created_by is nullable, matching every other
-- created_by column in this schema (e.g. receipts.created_by,
-- migrations/0011) — access could in principle be set by a system
-- process with no acting user.
--
-- role_id's FK cascades on delete, matching role_permissions' own
-- FK_role_permissions_role exactly (roles are never actually deleted by
-- the app today — internal/role implements no delete endpoint — but the
-- schema-level behavior mirrors its closest sibling table).
-- warehouse_id's FK does NOT cascade, matching every other warehouse_id
-- FK in this schema (receipts, putaway_tasks, picking_tasks,
-- stock_counts, replenishment_tasks, orders, users) — none of them
-- cascade on warehouse delete either. That is a pre-existing,
-- out-of-scope gap (warehouse delete today only guards against zones,
-- not any of these references) and is neither introduced nor fixed by
-- this migration.
CREATE TABLE role_warehouse_access (
    role_id      BIGINT    NOT NULL,
    warehouse_id BIGINT    NOT NULL,
    created_at   DATETIME2 NOT NULL CONSTRAINT DF_role_warehouse_access_created_at DEFAULT (SYSUTCDATETIME()),
    created_by   BIGINT    NULL,
    CONSTRAINT PK_role_warehouse_access PRIMARY KEY (role_id, warehouse_id),
    CONSTRAINT FK_role_warehouse_access_role
        FOREIGN KEY (role_id) REFERENCES roles(id) ON DELETE CASCADE,
    CONSTRAINT FK_role_warehouse_access_warehouse
        FOREIGN KEY (warehouse_id) REFERENCES warehouses(id),
    CONSTRAINT FK_role_warehouse_access_created_by
        FOREIGN KEY (created_by) REFERENCES users(id)
);

CREATE INDEX IX_role_warehouse_access_warehouse_id ON role_warehouse_access(warehouse_id);
