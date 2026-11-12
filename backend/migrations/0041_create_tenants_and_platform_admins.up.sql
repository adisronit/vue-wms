-- Phase 28 — Multi-tenant database foundation (docs/proposals/wms-saas/
-- tenant-er-design.md, migration-plan.md). Creates the two tables that
-- have no dependency on any tenant yet existing: `tenants` is the root
-- of every future tenant-owned FK chain, and `platform_admins` backs
-- the Platform Super Admin persona, kept deliberately separate from
-- tenant_users/roles per tenant-er-design.md §4 — a bug in tenant-role
-- resolution must never also be a platform-privilege bug.
--
-- This migration is intentionally table-structure-only: unlike the
-- Phase 27 migration-plan.md draft (§3a), it does NOT insert a
-- bootstrap tenant row. Backfilling existing single-tenant data onto
-- tenant_id = 1 is a Phase 29 concern (migration-plan.md §2, migrations
-- 0043-0046) — Phase 28 adds the additive platform schema only and
-- never creates a tenant automatically.
CREATE TABLE tenants (
    id              BIGINT        IDENTITY(1,1) NOT NULL,
    slug            VARCHAR(63)   NOT NULL,
    name            NVARCHAR(255) NOT NULL,
    status          VARCHAR(20)   NOT NULL CONSTRAINT DF_tenants_status DEFAULT ('TRIAL'),
    created_at      DATETIME2     NOT NULL CONSTRAINT DF_tenants_created_at DEFAULT (SYSUTCDATETIME()),
    updated_at      DATETIME2     NOT NULL CONSTRAINT DF_tenants_updated_at DEFAULT (SYSUTCDATETIME()),
    suspended_at    DATETIME2     NULL,
    cancelled_at    DATETIME2     NULL,
    CONSTRAINT PK_tenants PRIMARY KEY (id),
    CONSTRAINT UQ_tenants_slug UNIQUE (slug),
    CONSTRAINT CK_tenants_status CHECK (status IN ('TRIAL', 'ACTIVE', 'SUSPENDED', 'CANCELLED'))
);

-- Deliberately tiny and separate — see tenant-er-design.md §2. This is
-- the only table `RequirePlatformAdmin` middleware (a future phase)
-- will ever consult; no tenant-scoped code path reads or writes it.
CREATE TABLE platform_admins (
    id          BIGINT      IDENTITY(1,1) NOT NULL,
    user_id     BIGINT      NOT NULL,
    granted_by  BIGINT      NULL,
    granted_at  DATETIME2   NOT NULL CONSTRAINT DF_platform_admins_granted_at DEFAULT (SYSUTCDATETIME()),
    revoked_at  DATETIME2   NULL,
    CONSTRAINT PK_platform_admins PRIMARY KEY (id),
    CONSTRAINT UQ_platform_admins_user UNIQUE (user_id),
    CONSTRAINT FK_platform_admins_user FOREIGN KEY (user_id) REFERENCES users(id),
    CONSTRAINT FK_platform_admins_granted_by FOREIGN KEY (granted_by) REFERENCES users(id)
);

CREATE INDEX IX_platform_admins_revoked_at ON platform_admins(revoked_at);
