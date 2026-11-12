package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

func newPermissionTestRouter(secret, requiredCode string) *gin.Engine {
	r := gin.New()
	r.GET("/protected", RequireJWT(secret), RequirePermission(requiredCode), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	return r
}

func signTokenWithPermissions(t *testing.T, secret string, permissions []string) string {
	t.Helper()
	claims := jwt.MapClaims{
		"sub":         "1",
		"exp":         time.Now().Add(1 * time.Hour).Unix(),
		"permissions": permissions,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("failed to sign test token: %v", err)
	}
	return signed
}

func TestRequirePermission_Allowed(t *testing.T) {
	router := newPermissionTestRouter("test-secret", "users.view")
	token := signTokenWithPermissions(t, "test-secret", []string{"users.view", "users.update"})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestRequirePermission_Denied(t *testing.T) {
	router := newPermissionTestRouter("test-secret", "users.delete")
	token := signTokenWithPermissions(t, "test-secret", []string{"users.view", "users.update"})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusForbidden, rec.Body.String())
	}
}

func TestRequirePermission_NoPermissionsClaim(t *testing.T) {
	router := newPermissionTestRouter("test-secret", "users.view")
	token := signToken(t, "test-secret", time.Now().Add(1*time.Hour)) // from jwt_test.go — no permissions claim

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusForbidden, rec.Body.String())
	}
}

// Phase 3: the same middleware now also gates the Warehouse Setup
// module. These two tests document that warehouse.view specifically is
// enforced — an ADMIN-shaped token (has it) is let through, a
// QC-shaped token (Phase 3's RBAC gap fix — real permissions, but none
// for warehouse.*) is rejected.
func TestRequirePermission_WarehouseView_Allowed(t *testing.T) {
	router := newPermissionTestRouter("test-secret", "warehouse.view")
	token := signTokenWithPermissions(t, "test-secret", []string{"warehouse.view", "warehouse.create", "warehouse.update", "warehouse.delete"})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestRequirePermission_WarehouseView_DeniedForQCShapedToken(t *testing.T) {
	router := newPermissionTestRouter("test-secret", "warehouse.view")
	token := signTokenWithPermissions(t, "test-secret", []string{
		"dashboard.view", "quality_check.view", "quality_check.create", "quality_check.update", "stock_inquiry.view",
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusForbidden, rec.Body.String())
	}
}

// Phase 4: same pattern for the Product Master module.
func TestRequirePermission_ProductView_Allowed(t *testing.T) {
	router := newPermissionTestRouter("test-secret", "product.view")
	token := signTokenWithPermissions(t, "test-secret", []string{"product.view", "product.create", "product.update", "product.delete"})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestRequirePermission_ProductView_DeniedForPickingShapedToken(t *testing.T) {
	router := newPermissionTestRouter("test-secret", "product.view")
	token := signTokenWithPermissions(t, "test-secret", []string{
		"dashboard.view", "picking.view", "picking.update", "stock_inquiry.view",
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusForbidden, rec.Body.String())
	}
}

// Phase 5: Inventory Core reuses stock_inquiry.view (no new permission
// module) — every existing role already has it (confirmed in
// internal/rbac/catalog.go), so a realistic "denied" test has no real
// role-shaped token to use. The denial case below uses a deliberately
// synthetic minimal token instead, to prove the middleware itself still
// enforces the check correctly regardless of whether any current role
// happens to trigger it.
func TestRequirePermission_StockInquiryView_Allowed(t *testing.T) {
	router := newPermissionTestRouter("test-secret", "stock_inquiry.view")
	token := signTokenWithPermissions(t, "test-secret", []string{"dashboard.view", "stock_inquiry.view"})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestRequirePermission_StockInquiryView_DeniedForMinimalToken(t *testing.T) {
	router := newPermissionTestRouter("test-secret", "stock_inquiry.view")
	token := signTokenWithPermissions(t, "test-secret", []string{"dashboard.view"})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusForbidden, rec.Body.String())
	}
}

// Phase 6: Receiving reuses the existing "receiving" module — no new
// permission codes. receiving.create is allowed for a real RECEIVING-
// role-shaped token (they can build receipts). receiving.approve
// (which Confirm is gated on) is denied for that same token — the
// RECEIVING role's Phase 2/3 seed grants never included .approve, so
// this is a real role-shaped denial, unlike Phase 5's stock_inquiry
// case where every role had the permission.
func TestRequirePermission_ReceivingCreate_AllowedForReceivingRole(t *testing.T) {
	router := newPermissionTestRouter("test-secret", "receiving.create")
	token := signTokenWithPermissions(t, "test-secret", []string{
		"dashboard.view", "receiving.view", "receiving.create", "receiving.update", "stock_inquiry.view",
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestRequirePermission_ReceivingApprove_DeniedForReceivingRole(t *testing.T) {
	router := newPermissionTestRouter("test-secret", "receiving.approve")
	token := signTokenWithPermissions(t, "test-secret", []string{
		"dashboard.view", "receiving.view", "receiving.create", "receiving.update", "stock_inquiry.view",
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusForbidden, rec.Body.String())
	}
}

func TestRequirePermission_ReceivingApprove_AllowedForWarehouseManagerShapedToken(t *testing.T) {
	router := newPermissionTestRouter("test-secret", "receiving.approve")
	token := signTokenWithPermissions(t, "test-secret", []string{
		"dashboard.view", "receiving.view", "receiving.create", "receiving.update", "receiving.approve", "stock_inquiry.view",
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

// Phase 15: Cancel (POST /receipts/{id}/cancel) is gated on
// receiving.update, not receiving.approve — it only ever runs before
// any stock has been received, so it sits at editing's authority level,
// which a real RECEIVING-role-shaped token already has.
func TestRequirePermission_ReceivingUpdate_AllowedForReceivingRole(t *testing.T) {
	router := newPermissionTestRouter("test-secret", "receiving.update")
	token := signTokenWithPermissions(t, "test-secret", []string{
		"dashboard.view", "receiving.view", "receiving.create", "receiving.update", "stock_inquiry.view",
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

// Phase 7: Putaway reuses the existing "putaway" module — no new
// permission codes. putaway.create is allowed for a real PUTAWAY-
// role-shaped token (they can build tasks). putaway.approve (which
// Confirm is gated on) is denied for that same token — the PUTAWAY
// role's Phase 3 seed grants never included .approve, the same
// segregation-of-duties shape as Receiving's own .approve gate.
func TestRequirePermission_PutawayCreate_AllowedForPutawayRole(t *testing.T) {
	router := newPermissionTestRouter("test-secret", "putaway.create")
	token := signTokenWithPermissions(t, "test-secret", []string{
		"dashboard.view", "putaway.view", "putaway.create", "putaway.update", "stock_inquiry.view",
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestRequirePermission_PutawayApprove_DeniedForPutawayRole(t *testing.T) {
	router := newPermissionTestRouter("test-secret", "putaway.approve")
	token := signTokenWithPermissions(t, "test-secret", []string{
		"dashboard.view", "putaway.view", "putaway.create", "putaway.update", "stock_inquiry.view",
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusForbidden, rec.Body.String())
	}
}

func TestRequirePermission_PutawayApprove_AllowedForWarehouseManagerShapedToken(t *testing.T) {
	router := newPermissionTestRouter("test-secret", "putaway.approve")
	token := signTokenWithPermissions(t, "test-secret", []string{
		"dashboard.view", "putaway.view", "putaway.create", "putaway.update", "putaway.approve", "stock_inquiry.view",
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

// Phase 15: Cancel (POST /putaway/{id}/cancel) is gated on
// putaway.update, not putaway.approve — Service.Cancel's own MovedQty
// check is what actually enforces "no stock moved yet," so the
// permission gate sits at editing's authority level, which a real
// PUTAWAY-role-shaped token already has.
func TestRequirePermission_PutawayUpdate_AllowedForPutawayRole(t *testing.T) {
	router := newPermissionTestRouter("test-secret", "putaway.update")
	token := signTokenWithPermissions(t, "test-secret", []string{
		"dashboard.view", "putaway.view", "putaway.create", "putaway.update", "stock_inquiry.view",
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

// Phase 9: Picking reuses the existing "picking" module — no new
// permission codes. Unlike Receiving/Putaway, the PICKING role's own
// Phase 2 seed grants were only ever picking.view/picking.update — never
// picking.create and never picking.approve (internal/rbac/catalog.go).
// picking.create is therefore denied for a real PICKING-role-shaped
// token (only ADMIN/WAREHOUSE_MANAGER can build tasks under current
// grants) while picking.update — which internal/picking's Confirm is
// gated on, not picking.approve — is allowed, since that's the
// permission picking staff actually have to execute already-created
// tasks.
func TestRequirePermission_PickingCreate_DeniedForPickingRole(t *testing.T) {
	router := newPermissionTestRouter("test-secret", "picking.create")
	token := signTokenWithPermissions(t, "test-secret", []string{
		"dashboard.view", "picking.view", "picking.update", "stock_inquiry.view",
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusForbidden, rec.Body.String())
	}
}

func TestRequirePermission_PickingCreate_AllowedForWarehouseManagerShapedToken(t *testing.T) {
	router := newPermissionTestRouter("test-secret", "picking.create")
	token := signTokenWithPermissions(t, "test-secret", []string{
		"dashboard.view", "picking.view", "picking.create", "picking.update", "picking.approve", "stock_inquiry.view",
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestRequirePermission_PickingUpdate_AllowedForPickingRole(t *testing.T) {
	router := newPermissionTestRouter("test-secret", "picking.update")
	token := signTokenWithPermissions(t, "test-secret", []string{
		"dashboard.view", "picking.view", "picking.update", "stock_inquiry.view",
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

// Phase 13: Cancel is gated on picking.approve — the one picking.*
// code no endpoint had used before this phase (Confirm uses
// picking.update instead, see the doc above). A real PICKING-role-shaped
// token (view/update only, no approve) is denied; a WAREHOUSE_MANAGER-
// shaped one (which includes approve) is allowed — the same shape as
// picking.create's own pair of tests above, since Cancel sits at that
// same authority level.

func TestRequirePermission_PickingApprove_DeniedForPickingRole(t *testing.T) {
	router := newPermissionTestRouter("test-secret", "picking.approve")
	token := signTokenWithPermissions(t, "test-secret", []string{
		"dashboard.view", "picking.view", "picking.update", "stock_inquiry.view",
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusForbidden, rec.Body.String())
	}
}

func TestRequirePermission_PickingApprove_AllowedForWarehouseManagerShapedToken(t *testing.T) {
	router := newPermissionTestRouter("test-secret", "picking.approve")
	token := signTokenWithPermissions(t, "test-secret", []string{
		"dashboard.view", "picking.view", "picking.create", "picking.update", "picking.approve", "stock_inquiry.view",
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

// Phase 10: Shipping reuses the existing "shipping" module — no new
// permission codes. Unlike Picking, the SHIPPING role's own Phase 2/3
// seed grants (internal/rbac/catalog.go) mirror RECEIVING/PUTAWAY's
// shape exactly: view/create/update, no approve. shipping.create is
// therefore allowed for a real SHIPPING-role-shaped token (they can
// build shipments), and shipping.approve — which internal/shipping's
// Confirm is gated on, the same significant-action permission
// Receiving/Putaway use, not Picking's exception — is denied for that
// same token and allowed for a WAREHOUSE_MANAGER-shaped one.
func TestRequirePermission_ShippingCreate_AllowedForShippingRole(t *testing.T) {
	router := newPermissionTestRouter("test-secret", "shipping.create")
	token := signTokenWithPermissions(t, "test-secret", []string{
		"dashboard.view", "shipping.view", "shipping.create", "shipping.update", "stock_inquiry.view",
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestRequirePermission_ShippingApprove_DeniedForShippingRole(t *testing.T) {
	router := newPermissionTestRouter("test-secret", "shipping.approve")
	token := signTokenWithPermissions(t, "test-secret", []string{
		"dashboard.view", "shipping.view", "shipping.create", "shipping.update", "stock_inquiry.view",
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusForbidden, rec.Body.String())
	}
}

func TestRequirePermission_ShippingApprove_AllowedForWarehouseManagerShapedToken(t *testing.T) {
	router := newPermissionTestRouter("test-secret", "shipping.approve")
	token := signTokenWithPermissions(t, "test-secret", []string{
		"dashboard.view", "shipping.view", "shipping.create", "shipping.update", "shipping.approve", "stock_inquiry.view",
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

// Phase 15: Cancel (POST /shipping/{id}/cancel) is gated on
// shipping.update, not shipping.approve — it only ever runs pre-
// dispatch (Status.CanCancel is READY-only), the same boundary Update's
// own CanEdit already uses, so it sits at editing's authority level,
// which a real SHIPPING-role-shaped token already has.
func TestRequirePermission_ShippingUpdate_AllowedForShippingRole(t *testing.T) {
	router := newPermissionTestRouter("test-secret", "shipping.update")
	token := signTokenWithPermissions(t, "test-secret", []string{
		"dashboard.view", "shipping.view", "shipping.create", "shipping.update", "stock_inquiry.view",
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

// Phase 11: Stock Adjustment AND Stock Count both reuse the existing
// "stock_adjustment" module — no new codes, and no separate
// "stock_count" module exists anywhere in the catalog or
// RolePermissionView.vue's own module list, per the task's own "do not
// create duplicate concepts" instruction. Under current seed data, no
// operational role (RECEIVING/QC/PUTAWAY/PICKING/SHIPPING) has ANY
// stock_adjustment.* grant at all — only ADMIN/WAREHOUSE_MANAGER do via
// their generic all/all-except-delete computation. This was true before
// this phase (RolePermissionView.vue's own createDefaultPermissions()
// never had a branch for it either) and is not narrowed further here.
func TestRequirePermission_StockAdjustmentView_DeniedForMinimalToken(t *testing.T) {
	router := newPermissionTestRouter("test-secret", "stock_adjustment.view")
	token := signTokenWithPermissions(t, "test-secret", []string{
		"dashboard.view", "picking.view", "picking.update", "stock_inquiry.view",
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusForbidden, rec.Body.String())
	}
}

func TestRequirePermission_StockAdjustmentApprove_AllowedForWarehouseManagerShapedToken(t *testing.T) {
	router := newPermissionTestRouter("test-secret", "stock_adjustment.approve")
	token := signTokenWithPermissions(t, "test-secret", []string{
		"dashboard.view", "stock_adjustment.view", "stock_adjustment.create", "stock_adjustment.update",
		"stock_adjustment.approve", "stock_inquiry.view",
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}
