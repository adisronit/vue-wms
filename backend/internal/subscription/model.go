// Package subscription owns the platform-global subscription plan
// catalog and each tenant's current subscription and feature
// entitlement overrides — the billing side of the Phase 28 multi-tenant
// database foundation (see docs/proposals/wms-saas/tenant-er-design.md
// and saas-module-roadmap.md's "Subscription Model" section).
//
// Schema-only for Phase 28: plan-limit enforcement, trial/expiry
// lifecycle checks, and any handler/service/repository are a later
// phase (saas-module-roadmap.md's Phase 33).
package subscription

import "time"

// Plan is the GORM model for the subscription_plans table
// (migrations/0042) — the platform-global, sellable product catalog,
// identical for every prospective tenant.
type Plan struct {
	ID            int64     `gorm:"column:id;primaryKey"`
	Code          string    `gorm:"column:code"`
	Name          string    `gorm:"column:name"`
	BillingCycle  string    `gorm:"column:billing_cycle"`
	PriceCents    int       `gorm:"column:price_cents"`
	Currency      string    `gorm:"column:currency"`
	MaxWarehouses *int      `gorm:"column:max_warehouses"`
	MaxUsers      *int      `gorm:"column:max_users"`
	MaxActiveSKUs *int      `gorm:"column:max_active_skus"`
	FeatureFlags  string    `gorm:"column:feature_flags"`
	IsPublic      bool      `gorm:"column:is_public"`
	CreatedAt     time.Time `gorm:"column:created_at"`
}

func (Plan) TableName() string { return "subscription_plans" }

// Billing cycle values (subscription_plans.billing_cycle, CK_subscription_plans_billing_cycle).
const (
	BillingCycleTrial   = "TRIAL"
	BillingCycleMonthly = "MONTHLY"
	BillingCycleAnnual  = "ANNUAL"
)

// TenantSubscription is the GORM model for the tenant_subscriptions
// table (migrations/0042) — current-state-only (v1): one row per
// tenant, no billing-period history beyond what tenant_audit_logs
// separately captures.
type TenantSubscription struct {
	ID                 int64      `gorm:"column:id;primaryKey"`
	TenantID           int64      `gorm:"column:tenant_id"`
	PlanID             int64      `gorm:"column:plan_id"`
	Status             string     `gorm:"column:status"`
	TrialEndsAt        *time.Time `gorm:"column:trial_ends_at"`
	CurrentPeriodStart time.Time  `gorm:"column:current_period_start"`
	CurrentPeriodEnd   time.Time  `gorm:"column:current_period_end"`
	CancelAtPeriodEnd  bool       `gorm:"column:cancel_at_period_end"`
	CancelledAt        *time.Time `gorm:"column:cancelled_at"`
	CreatedAt          time.Time  `gorm:"column:created_at"`
	UpdatedAt          time.Time  `gorm:"column:updated_at"`
}

func (TenantSubscription) TableName() string { return "tenant_subscriptions" }

// Subscription status values (tenant_subscriptions.status, CK_tenant_subscriptions_status).
const (
	SubscriptionStatusTrialing  = "TRIALING"
	SubscriptionStatusActive    = "ACTIVE"
	SubscriptionStatusPastDue   = "PAST_DUE"
	SubscriptionStatusCancelled = "CANCELLED"
	SubscriptionStatusExpired   = "EXPIRED"
)

// Feature is the GORM model for the tenant_features table
// (migrations/0042) — a sales-negotiated exception or staged rollout
// override layered over a plan's FeatureFlags defaults.
type Feature struct {
	ID          int64      `gorm:"column:id;primaryKey"`
	TenantID    int64      `gorm:"column:tenant_id"`
	FeatureCode string     `gorm:"column:feature_code"`
	Enabled     bool       `gorm:"column:enabled"`
	LimitValue  *int       `gorm:"column:limit_value"`
	GrantedBy   *int64     `gorm:"column:granted_by"`
	GrantedAt   time.Time  `gorm:"column:granted_at"`
	ExpiresAt   *time.Time `gorm:"column:expires_at"`
}

func (Feature) TableName() string { return "tenant_features" }
