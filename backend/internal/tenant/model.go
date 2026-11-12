// Package tenant owns the tenant master record, tenant membership
// (tenant_users), per-tenant settings, and the append-only tenant audit
// log — the Phase 28 database foundation for turning this WMS into a
// multi-tenant SaaS product (see docs/proposals/wms-saas/tenant-er-design.md).
//
// This package is schema-only for Phase 28: no handler, service, or
// repository exists yet. Enforcing tenant isolation at the API/query
// layer is Phase 29+ (docs/proposals/wms-saas/saas-module-roadmap.md),
// once tenant_id has been backfilled onto the existing business tables.
package tenant

import "time"

// Tenant is the GORM model for the tenants table (migrations/0041) —
// the root of every future tenant-owned foreign-key chain.
type Tenant struct {
	ID          int64      `gorm:"column:id;primaryKey"`
	Slug        string     `gorm:"column:slug"`
	Name        string     `gorm:"column:name"`
	Status      string     `gorm:"column:status"`
	CreatedAt   time.Time  `gorm:"column:created_at"`
	UpdatedAt   time.Time  `gorm:"column:updated_at"`
	SuspendedAt *time.Time `gorm:"column:suspended_at"`
	CancelledAt *time.Time `gorm:"column:cancelled_at"`
}

func (Tenant) TableName() string { return "tenants" }

// Tenant status values (tenants.status, CK_tenants_status).
const (
	TenantStatusTrial     = "TRIAL"
	TenantStatusActive    = "ACTIVE"
	TenantStatusSuspended = "SUSPENDED"
	TenantStatusCancelled = "CANCELLED"
)

// TenantUser is the GORM model for the tenant_users table
// (migrations/0042) — the join table that grants one `users` login
// access to one tenant, with one role inside that tenant. A user with
// access to three tenants has one users row and three TenantUser rows.
type TenantUser struct {
	ID        int64      `gorm:"column:id;primaryKey"`
	TenantID  int64      `gorm:"column:tenant_id"`
	UserID    int64      `gorm:"column:user_id"`
	RoleID    int64      `gorm:"column:role_id"`
	Status    string     `gorm:"column:status"`
	InvitedBy *int64     `gorm:"column:invited_by"`
	InvitedAt *time.Time `gorm:"column:invited_at"`
	JoinedAt  *time.Time `gorm:"column:joined_at"`
	CreatedAt time.Time  `gorm:"column:created_at"`
	UpdatedAt time.Time  `gorm:"column:updated_at"`
}

func (TenantUser) TableName() string { return "tenant_users" }

// TenantUser status values (tenant_users.status, CK_tenant_users_status).
const (
	TenantUserStatusInvited  = "INVITED"
	TenantUserStatusActive   = "ACTIVE"
	TenantUserStatusDisabled = "DISABLED"
)

// TenantSetting is the GORM model for the tenant_settings table
// (migrations/0042) — one row per tenant (1:1), structured columns for
// everything currently known to matter plus a JSON escape hatch.
type TenantSetting struct {
	// autoIncrement:false — tenant_id is a plain FK-as-PK (1:1 with
	// tenants), not an IDENTITY column; without this, GORM's default
	// assumption that any primaryKey field auto-increments makes SQL
	// Server reject the INSERT ("does not have the identity property").
	TenantID           int64     `gorm:"column:tenant_id;primaryKey;autoIncrement:false"`
	Timezone           string    `gorm:"column:timezone"`
	Locale             string    `gorm:"column:locale"`
	Currency           string    `gorm:"column:currency"`
	DefaultWarehouseID *int64    `gorm:"column:default_warehouse_id"`
	CompanyLegalName   *string   `gorm:"column:company_legal_name"`
	LogoURL            *string   `gorm:"column:logo_url"`
	CustomJSON         *string   `gorm:"column:custom_json"`
	UpdatedAt          time.Time `gorm:"column:updated_at"`
}

func (TenantSetting) TableName() string { return "tenant_settings" }

// TenantAuditLog is the GORM model for the tenant_audit_logs table
// (migrations/0042) — append-only; no application code path should
// ever UPDATE or DELETE a row here.
type TenantAuditLog struct {
	ID          int64     `gorm:"column:id;primaryKey"`
	TenantID    *int64    `gorm:"column:tenant_id"`
	ActorUserID int64     `gorm:"column:actor_user_id"`
	ActorType   string    `gorm:"column:actor_type"`
	Action      string    `gorm:"column:action"`
	TargetType  *string   `gorm:"column:target_type"`
	TargetID    *int64    `gorm:"column:target_id"`
	Metadata    *string   `gorm:"column:metadata"`
	IPAddress   *string   `gorm:"column:ip_address"`
	CreatedAt   time.Time `gorm:"column:created_at"`
}

func (TenantAuditLog) TableName() string { return "tenant_audit_logs" }

// Actor type values (tenant_audit_logs.actor_type, CK_tenant_audit_logs_actor_type).
const (
	AuditActorPlatformAdmin = "PLATFORM_ADMIN"
	AuditActorTenantAdmin   = "TENANT_ADMIN"
	AuditActorSystem        = "SYSTEM"
)
