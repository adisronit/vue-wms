-- Phase 26 — User Management shows "—" for every row's Warehouse and
-- Last Login column: neither column exists on users at all (confirmed
-- by inspecting migrations/0004 and internal/user/model.go before this
-- migration — the frontend's Warehouse <select> in the create/edit
-- drawer has never been wired to a real column, and login has never
-- written anything back to the user's own row). This migration adds
-- both, additive and nullable, so no existing user row needs backfill.
--
-- warehouse_id is nullable: no warehouse-scoped authorization logic
-- exists anywhere in this codebase today (grepped internal/platform/
-- middleware and every handler before writing this migration), so a
-- user is not required to have exactly one warehouse the way, say, a
-- receipt or picking task is. NULL means "not assigned to a specific
-- warehouse" (e.g. ADMIN, or any user created before this migration) —
-- it is not being overloaded to mean "all warehouses," since nothing in
-- the backend currently grants or restricts access based on it; the
-- frontend displays a NULL warehouse as "-", not "All Warehouses".
ALTER TABLE users ADD warehouse_id BIGINT NULL;

ALTER TABLE users ADD CONSTRAINT FK_users_warehouse
    FOREIGN KEY (warehouse_id) REFERENCES warehouses(id);

CREATE INDEX IX_users_warehouse_id ON users(warehouse_id);

-- last_login_at is nullable and written only by a successful login
-- (internal/auth's Service.Login, after credentials are already
-- verified and the account is confirmed ACTIVE) via the database's own
-- SYSUTCDATETIME(), the same server-authoritative-clock convention
-- every other timestamp default in this schema already uses. It is
-- never written by token refresh, /auth/me, or any other authenticated
-- request. NULL means "has never logged in."
ALTER TABLE users ADD last_login_at DATETIME2 NULL;
