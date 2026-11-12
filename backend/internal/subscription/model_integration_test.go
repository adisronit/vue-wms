package subscription

import (
	"testing"
	"time"

	"gorm.io/gorm"

	"wms-backend/internal/platform/testsupport"
	"wms-backend/internal/tenant"
)

// Integration tests against real SQL Server constraints created by
// migrations/0042. See internal/tenant/model_integration_test.go for
// the shared skip-if-unavailable / rollback-after-every-test
// conventions these tests follow.

func seedTenant(t *testing.T, tx *gorm.DB, slug string) int64 {
	t.Helper()

	tn := tenant.Tenant{Slug: slug, Name: "Subscription Fixture Co", Status: tenant.TenantStatusActive}
	if err := tx.Create(&tn).Error; err != nil {
		t.Fatalf("seed fixture tenant: %v", err)
	}
	return tn.ID
}

func TestSubscriptionPlan_UniqueCode(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	first := Plan{Code: "MONTHLY_STANDARD", Name: "Monthly Standard", BillingCycle: BillingCycleMonthly}
	if err := tx.Create(&first).Error; err != nil {
		t.Fatalf("create first plan: %v", err)
	}

	second := Plan{Code: "MONTHLY_STANDARD", Name: "Monthly Standard Duplicate", BillingCycle: BillingCycleMonthly}
	if err := tx.Create(&second).Error; err == nil {
		t.Fatalf("expected UQ_subscription_plans_code violation inserting a duplicate plan code, got no error")
	}
}

func TestSubscriptionPlan_BillingCycleCheckConstraint(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	bad := Plan{Code: "BOGUS_CYCLE", Name: "Bogus Cycle", BillingCycle: "WEEKLY"}
	if err := tx.Create(&bad).Error; err == nil {
		t.Fatalf("expected CK_subscription_plans_billing_cycle violation for billing_cycle %q, got no error", bad.BillingCycle)
	}
}

func TestTenantSubscription_ReferentialIntegrity(t *testing.T) {
	db := testsupport.ConnectDB(t)

	t.Run("unknown tenant_id rejected", func(t *testing.T) {
		tx := testsupport.BeginRollbackTx(t, db)
		plan := Plan{Code: "REF_TEST_PLAN_A", Name: "Ref Test Plan A", BillingCycle: BillingCycleTrial}
		if err := tx.Create(&plan).Error; err != nil {
			t.Fatalf("create plan: %v", err)
		}

		sub := TenantSubscription{
			TenantID: 999999999, PlanID: plan.ID, Status: SubscriptionStatusTrialing,
			CurrentPeriodStart: time.Now().UTC(), CurrentPeriodEnd: time.Now().UTC().Add(30 * 24 * time.Hour),
		}
		if err := tx.Create(&sub).Error; err == nil {
			t.Fatalf("expected FK_tenant_subscriptions_tenant violation for a nonexistent tenant_id, got no error")
		}
	})

	t.Run("unknown plan_id rejected", func(t *testing.T) {
		tx := testsupport.BeginRollbackTx(t, db)
		tenantID := seedTenant(t, tx, "ref-test-plan-tenant")

		sub := TenantSubscription{
			TenantID: tenantID, PlanID: 999999999, Status: SubscriptionStatusTrialing,
			CurrentPeriodStart: time.Now().UTC(), CurrentPeriodEnd: time.Now().UTC().Add(30 * 24 * time.Hour),
		}
		if err := tx.Create(&sub).Error; err == nil {
			t.Fatalf("expected FK_tenant_subscriptions_plan violation for a nonexistent plan_id, got no error")
		}
	})
}

func TestTenantSubscription_OnlyOnePerTenant(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	tenantID := seedTenant(t, tx, "one-sub-per-tenant")
	plan := Plan{Code: "ONE_SUB_PLAN", Name: "One Sub Plan", BillingCycle: BillingCycleAnnual}
	if err := tx.Create(&plan).Error; err != nil {
		t.Fatalf("create plan: %v", err)
	}

	now := time.Now().UTC()
	first := TenantSubscription{
		TenantID: tenantID, PlanID: plan.ID, Status: SubscriptionStatusActive,
		CurrentPeriodStart: now, CurrentPeriodEnd: now.Add(365 * 24 * time.Hour),
	}
	if err := tx.Create(&first).Error; err != nil {
		t.Fatalf("create first subscription: %v", err)
	}

	second := TenantSubscription{
		TenantID: tenantID, PlanID: plan.ID, Status: SubscriptionStatusActive,
		CurrentPeriodStart: now, CurrentPeriodEnd: now.Add(365 * 24 * time.Hour),
	}
	if err := tx.Create(&second).Error; err == nil {
		t.Fatalf("expected UQ_tenant_subscriptions_tenant violation inserting a second subscription for the same tenant, got no error")
	}
}

func TestTenantSubscription_StatusCheckConstraint(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	tenantID := seedTenant(t, tx, "status-check-sub")
	plan := Plan{Code: "STATUS_CHECK_PLAN", Name: "Status Check Plan", BillingCycle: BillingCycleMonthly}
	if err := tx.Create(&plan).Error; err != nil {
		t.Fatalf("create plan: %v", err)
	}

	now := time.Now().UTC()
	bad := TenantSubscription{
		TenantID: tenantID, PlanID: plan.ID, Status: "NOT_A_REAL_STATUS",
		CurrentPeriodStart: now, CurrentPeriodEnd: now.Add(30 * 24 * time.Hour),
	}
	if err := tx.Create(&bad).Error; err == nil {
		t.Fatalf("expected CK_tenant_subscriptions_status violation, got no error")
	}
}

func TestTenantFeature_UniquePerTenantAndCode(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	tenantID := seedTenant(t, tx, "feature-unique-co")

	first := Feature{TenantID: tenantID, FeatureCode: "ADVANCED_REPORTING", Enabled: true}
	if err := tx.Create(&first).Error; err != nil {
		t.Fatalf("create first feature override: %v", err)
	}

	second := Feature{TenantID: tenantID, FeatureCode: "ADVANCED_REPORTING", Enabled: false}
	if err := tx.Create(&second).Error; err == nil {
		t.Fatalf("expected UQ_tenant_features_tenant_feature violation inserting a duplicate (tenant_id, feature_code) pair, got no error")
	}

	// A different feature_code for the same tenant must succeed —
	// the uniqueness is per (tenant, feature), not per tenant alone.
	third := Feature{TenantID: tenantID, FeatureCode: "BARCODE_SCANNING", Enabled: true}
	if err := tx.Create(&third).Error; err != nil {
		t.Fatalf("expected a different feature_code for the same tenant to succeed, got error: %v", err)
	}
}
