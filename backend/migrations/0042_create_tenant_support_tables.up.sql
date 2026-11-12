-- Phase 28 — Multi-tenant database foundation, continued from 0041.
-- Creates the remaining six platform/tenant tables from
-- docs/proposals/wms-saas/tenant-er-design.md §2: the platform-global
-- subscription catalog, the per-tenant subscription/settings/feature-
-- override/audit-log tables, and tenant_users — the join table that is
-- what actually grants one `users` login access to one tenant ("one
-- user, many tenants"; "one tenant, many users" both hold because
-- identity is platform-global and membership+role are per-tenant, see
-- tenant-er-design.md §1 and §4).
--
-- Two values are resolved here where the Phase 27 proposal's two
-- documents disagreed with each other (see docs/proposals/wms-saas/
-- phase28-implementation-notes.md for the full writeup):
--   * tenant_users.status defaults to 'INVITED', not 'ACTIVE' — a brand
--     new membership row has not been accepted yet. (migration-plan.md's
--     draft used 'ACTIVE' only because its example was the Phase 29
--     backfill INSERT for *already-active* existing users, not this
--     table's general-purpose default.)
--   * tenant_users.joined_at stays NULL until status transitions to
--     ACTIVE, matching tenant-er-design.md's column description; it is
--     not stamped at row-creation time.
-- Status/code-shaped columns use VARCHAR, matching every existing
-- status column in this schema (e.g. orders.status, receipts.status,
-- migrations/0011, 0038) — NVARCHAR is reserved for human-entered
-- display text (name, company_legal_name, logo_url).

-- Platform-global sellable product catalog — identical for every
-- prospective tenant, not owned by any one of them.
CREATE TABLE subscription_plans (
    id              BIGINT        IDENTITY(1,1) NOT NULL,
    code            VARCHAR(50)   NOT NULL,
    name            NVARCHAR(100) NOT NULL,
    billing_cycle   VARCHAR(20)   NOT NULL,
    price_cents     INT           NOT NULL CONSTRAINT DF_subscription_plans_price_cents DEFAULT (0),
    currency        CHAR(3)       NOT NULL CONSTRAINT DF_subscription_plans_currency DEFAULT ('USD'),
    max_warehouses  INT           NULL,
    max_users       INT           NULL,
    max_active_skus INT           NULL,
    feature_flags   NVARCHAR(MAX) NOT NULL CONSTRAINT DF_subscription_plans_feature_flags DEFAULT ('{}'),
    is_public       BIT           NOT NULL CONSTRAINT DF_subscription_plans_is_public DEFAULT (1),
    created_at      DATETIME2     NOT NULL CONSTRAINT DF_subscription_plans_created_at DEFAULT (SYSUTCDATETIME()),
    CONSTRAINT PK_subscription_plans PRIMARY KEY (id),
    CONSTRAINT UQ_subscription_plans_code UNIQUE (code),
    CONSTRAINT CK_subscription_plans_billing_cycle CHECK (billing_cycle IN ('TRIAL', 'MONTHLY', 'ANNUAL'))
);

-- Current-state-only (v1): one row per tenant, no billing-period
-- history beyond what tenant_audit_logs separately captures (an
-- unresolved design choice tenant-er-design.md §6 leaves open for a
-- later phase to revisit).
CREATE TABLE tenant_subscriptions (
    id                      BIGINT      IDENTITY(1,1) NOT NULL,
    tenant_id               BIGINT      NOT NULL,
    plan_id                 BIGINT      NOT NULL,
    status                  VARCHAR(20) NOT NULL,
    trial_ends_at           DATETIME2   NULL,
    current_period_start    DATETIME2   NOT NULL,
    current_period_end      DATETIME2   NOT NULL,
    cancel_at_period_end    BIT         NOT NULL CONSTRAINT DF_tenant_subscriptions_cancel_at_period_end DEFAULT (0),
    cancelled_at            DATETIME2   NULL,
    created_at              DATETIME2   NOT NULL CONSTRAINT DF_tenant_subscriptions_created_at DEFAULT (SYSUTCDATETIME()),
    updated_at              DATETIME2   NOT NULL CONSTRAINT DF_tenant_subscriptions_updated_at DEFAULT (SYSUTCDATETIME()),
    CONSTRAINT PK_tenant_subscriptions PRIMARY KEY (id),
    CONSTRAINT UQ_tenant_subscriptions_tenant UNIQUE (tenant_id),
    CONSTRAINT FK_tenant_subscriptions_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id),
    CONSTRAINT FK_tenant_subscriptions_plan FOREIGN KEY (plan_id) REFERENCES subscription_plans(id),
    CONSTRAINT CK_tenant_subscriptions_status CHECK (status IN ('TRIALING', 'ACTIVE', 'PAST_DUE', 'CANCELLED', 'EXPIRED'))
);

CREATE INDEX IX_tenant_subscriptions_plan_id ON tenant_subscriptions(plan_id);

-- One row per tenant (1:1). Structured columns for everything currently
-- known to matter, plus one JSON escape hatch rather than full EAV —
-- see tenant-er-design.md §2 for the reasoning (EAV lets settings
-- proliferate untyped and needs an extra join/pivot on every read; a
-- fixed column set keeps the common path type-safe and plain-SELECT-
-- able, and custom_json absorbs the rare one-off case without a
-- migration).
CREATE TABLE tenant_settings (
    tenant_id             BIGINT        NOT NULL,
    timezone              VARCHAR(64)   NOT NULL CONSTRAINT DF_tenant_settings_timezone DEFAULT ('UTC'),
    locale                VARCHAR(10)   NOT NULL CONSTRAINT DF_tenant_settings_locale DEFAULT ('en-US'),
    currency               CHAR(3)       NOT NULL CONSTRAINT DF_tenant_settings_currency DEFAULT ('USD'),
    default_warehouse_id   BIGINT        NULL,
    company_legal_name     NVARCHAR(255) NULL,
    logo_url               NVARCHAR(500) NULL,
    custom_json            NVARCHAR(MAX) NULL,
    updated_at             DATETIME2     NOT NULL CONSTRAINT DF_tenant_settings_updated_at DEFAULT (SYSUTCDATETIME()),
    CONSTRAINT PK_tenant_settings PRIMARY KEY (tenant_id),
    CONSTRAINT FK_tenant_settings_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id),
    CONSTRAINT FK_tenant_settings_default_warehouse FOREIGN KEY (default_warehouse_id) REFERENCES warehouses(id)
);

-- Sales-negotiated exceptions or staged feature rollouts, layered over
-- a plan's feature_flags defaults — plan defaults apply when no row
-- exists here for a given (tenant, feature_code).
CREATE TABLE tenant_features (
    id            BIGINT        IDENTITY(1,1) NOT NULL,
    tenant_id     BIGINT        NOT NULL,
    feature_code  VARCHAR(100)  NOT NULL,
    enabled       BIT           NOT NULL CONSTRAINT DF_tenant_features_enabled DEFAULT (1),
    limit_value   INT           NULL,
    granted_by    BIGINT        NULL,
    granted_at    DATETIME2     NOT NULL CONSTRAINT DF_tenant_features_granted_at DEFAULT (SYSUTCDATETIME()),
    expires_at    DATETIME2     NULL,
    CONSTRAINT PK_tenant_features PRIMARY KEY (id),
    CONSTRAINT UQ_tenant_features_tenant_feature UNIQUE (tenant_id, feature_code),
    CONSTRAINT FK_tenant_features_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id),
    CONSTRAINT FK_tenant_features_granted_by FOREIGN KEY (granted_by) REFERENCES users(id)
);

-- Append-only — no updated_at, and no application code path should
-- ever UPDATE or DELETE a row here. tenant_id is nullable for actions
-- not tied to one tenant (e.g. the act of creating a tenant itself).
CREATE TABLE tenant_audit_logs (
    id             BIGINT        IDENTITY(1,1) NOT NULL,
    tenant_id      BIGINT        NULL,
    actor_user_id  BIGINT        NOT NULL,
    actor_type     VARCHAR(20)   NOT NULL,
    action         VARCHAR(100)  NOT NULL,
    target_type    VARCHAR(50)   NULL,
    target_id      BIGINT        NULL,
    metadata       NVARCHAR(MAX) NULL,
    ip_address     VARCHAR(45)   NULL,
    created_at     DATETIME2     NOT NULL CONSTRAINT DF_tenant_audit_logs_created_at DEFAULT (SYSUTCDATETIME()),
    CONSTRAINT PK_tenant_audit_logs PRIMARY KEY (id),
    CONSTRAINT FK_tenant_audit_logs_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id),
    CONSTRAINT FK_tenant_audit_logs_actor FOREIGN KEY (actor_user_id) REFERENCES users(id),
    CONSTRAINT CK_tenant_audit_logs_actor_type CHECK (actor_type IN ('PLATFORM_ADMIN', 'TENANT_ADMIN', 'SYSTEM'))
);

CREATE INDEX IX_tenant_audit_logs_tenant_id ON tenant_audit_logs(tenant_id);
CREATE INDEX IX_tenant_audit_logs_actor_user_id ON tenant_audit_logs(actor_user_id);

-- The core membership table: a `users` row is a login; a `tenant_users`
-- row is what grants that login access to one specific tenant's data,
-- with one specific role *inside that tenant*. role_id must resolve to
-- a role whose own tenant_id equals this row's tenant_id — enforced at
-- the application layer (tenant-er-design.md §5 Option B), not by a
-- database trigger, consistent with tenant-isolation-security.md's
-- repository-layer enforcement design.
CREATE TABLE tenant_users (
    id           BIGINT       IDENTITY(1,1) NOT NULL,
    tenant_id    BIGINT       NOT NULL,
    user_id      BIGINT       NOT NULL,
    role_id      BIGINT       NOT NULL,
    status       VARCHAR(20)  NOT NULL CONSTRAINT DF_tenant_users_status DEFAULT ('INVITED'),
    invited_by   BIGINT       NULL,
    invited_at   DATETIME2    NULL,
    joined_at    DATETIME2    NULL,
    created_at   DATETIME2    NOT NULL CONSTRAINT DF_tenant_users_created_at DEFAULT (SYSUTCDATETIME()),
    updated_at   DATETIME2    NOT NULL CONSTRAINT DF_tenant_users_updated_at DEFAULT (SYSUTCDATETIME()),
    CONSTRAINT PK_tenant_users PRIMARY KEY (id),
    CONSTRAINT UQ_tenant_users_tenant_user UNIQUE (tenant_id, user_id),
    CONSTRAINT FK_tenant_users_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id),
    CONSTRAINT FK_tenant_users_user FOREIGN KEY (user_id) REFERENCES users(id),
    CONSTRAINT FK_tenant_users_role FOREIGN KEY (role_id) REFERENCES roles(id),
    CONSTRAINT FK_tenant_users_invited_by FOREIGN KEY (invited_by) REFERENCES users(id),
    CONSTRAINT CK_tenant_users_status CHECK (status IN ('INVITED', 'ACTIVE', 'DISABLED'))
);

CREATE INDEX IX_tenant_users_user_id ON tenant_users(user_id);
CREATE INDEX IX_tenant_users_role_id ON tenant_users(role_id);
