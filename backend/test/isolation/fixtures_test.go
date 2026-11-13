package isolation

import (
	"context"
	"fmt"

	"wms-backend/internal/location"
	"wms-backend/internal/product"
	"wms-backend/internal/role"
	"wms-backend/internal/user"
	"wms-backend/internal/warehouse"
	"wms-backend/internal/zone"
)

// tenantFixture is one logical tenant's full master-data set: a
// warehouse, one zone inside it, one location inside that zone, one
// product, an ADMIN-equivalent user scoped to the warehouse, and —
// because today's schema has no tenant_id column anywhere (see
// phase27-audit-report.md) — a custom role of this tenant's "own" that
// exists purely to give TestTenantA_CannotMutateTenantB_RolePermissions
// something tenant-shaped to target, since the seeded ADMIN/
// WAREHOUSE_MANAGER/etc. roles are shared catalog rows every fixture
// otherwise uses.
//
// There is deliberately no tenant_id field here: these fixtures model
// what Tenant A and Tenant B's data WILL look like once Phase 29 adds
// that column, by using disjoint random-suffixed codes in place of a
// real tenant boundary the schema can't express yet.
type tenantFixture struct {
	Label     string
	Role      role.Role
	Warehouse warehouse.Warehouse
	Zone      zone.Zone
	Location  location.Location
	Product   product.Product
	User      user.User
	Password  string
	Token     string
}

// seedTenant inserts one complete tenantFixture directly via GORM
// (bypassing the HTTP layer entirely for setup, the same "arrange" role
// internal/role's own service_test.go reserves for direct DB access) and
// logs the fixture's user in through the real /api/v1/auth/login
// endpoint so h.Token exercises production token issuance.
func (h *harness) seedTenant(label string) *tenantFixture {
	h.t.Helper()
	suffix := uniqueSuffix()

	adminRole, err := h.deps.RoleRepo.FindRoleByCode(context.Background(), "ADMIN")
	if err != nil {
		h.t.Fatalf("seedTenant(%s): find ADMIN role: %v", label, err)
	}

	tenantRole := role.Role{
		Code:        fmt.Sprintf("TENANT_%s_ROLE_%s", label, suffix),
		Name:        fmt.Sprintf("Tenant %s Custom Role", label),
		Description: "Isolation-suite fixture role, owned by tenant " + label,
		IsActive:    true,
	}
	if err := h.db.Create(&tenantRole).Error; err != nil {
		h.t.Fatalf("seedTenant(%s): create role: %v", label, err)
	}

	wh := warehouse.Warehouse{
		Code:    fmt.Sprintf("WH-%s-%s", label, suffix),
		Name:    fmt.Sprintf("Tenant %s Warehouse", label),
		Type:    "Distribution Center",
		Address: "Fixture address",
		Status:  warehouse.StatusActive,
	}
	if err := h.db.Create(&wh).Error; err != nil {
		h.t.Fatalf("seedTenant(%s): create warehouse: %v", label, err)
	}

	z := zone.Zone{
		WarehouseID: wh.ID,
		Code:        fmt.Sprintf("Z-%s-%s", label, suffix),
		Name:        fmt.Sprintf("Tenant %s Zone", label),
		Type:        zone.TypeStorage,
		Status:      zone.StatusActive,
	}
	if err := h.db.Create(&z).Error; err != nil {
		h.t.Fatalf("seedTenant(%s): create zone: %v", label, err)
	}

	loc := location.Location{
		ZoneID: z.ID,
		Code:   fmt.Sprintf("LOC-%s-%s", label, suffix),
		Aisle:  "01",
		Rack:   "01",
		Bin:    "01",
		Status: location.StatusActive,
	}
	if err := h.db.Create(&loc).Error; err != nil {
		h.t.Fatalf("seedTenant(%s): create location: %v", label, err)
	}

	prod := product.Product{
		SKU:      fmt.Sprintf("SKU-%s-%s", label, suffix),
		Name:     fmt.Sprintf("Tenant %s Product", label),
		Category: product.CategoryPackaging,
		BaseUOM:  product.UOMPieces,
		PackSize: 1,
		Status:   product.StatusActive,
	}
	if err := h.db.Create(&prod).Error; err != nil {
		h.t.Fatalf("seedTenant(%s): create product: %v", label, err)
	}

	password := "Fixture-Pass-" + suffix + "!"
	u := user.User{
		Username:     fmt.Sprintf("tenant_%s_%s", label, suffix),
		PasswordHash: hashPassword(h.t, password),
		FullName:     fmt.Sprintf("Tenant %s Admin", label),
		Email:        fmt.Sprintf("tenant.%s.%s@isolation.test", label, suffix),
		RoleID:       adminRole.ID,
		WarehouseID:  &wh.ID,
		Status:       user.StatusActive,
	}
	if err := h.db.Create(&u).Error; err != nil {
		h.t.Fatalf("seedTenant(%s): create user: %v", label, err)
	}

	token := h.login(u.Username, password)

	return &tenantFixture{
		Label:     label,
		Role:      tenantRole,
		Warehouse: wh,
		Zone:      z,
		Location:  loc,
		Product:   prod,
		User:      u,
		Password:  password,
		Token:     token,
	}
}
