package docrender

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"wms-backend/internal/platform/middleware"
	"wms-backend/internal/platform/testsupport"
	"wms-backend/internal/user"
)

// newTestContext builds a *gin.Context with the SAME two context keys
// RequireJWT/RequireTenantContext would have set on a real request,
// without actually running either — this package's own two endpoints
// check middleware.HasPermission dynamically (the required permission
// depends on data only known at request time, see Handler.Register's
// own doc), so the RBAC behavior under test lives entirely in the
// handler body, not in a chained gin.HandlerFunc a full router would be
// needed to exercise.
func newTestContext(tenantID int64, permissions []string, params gin.Params) (*gin.Context, *httptest.ResponseRecorder) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/documents/test", nil)
	c.Params = params
	c.Set(middleware.TenantContextKey, tenantID)
	permsAny := make([]any, len(permissions))
	for i, p := range permissions {
		permsAny[i] = p
	}
	c.Set(middleware.ClaimsContextKey, jwt.MapClaims{"sub": "1", "permissions": permsAny})
	return c, rec
}

// TestHandler_History_DeniedWithoutPermission proves History rejects a
// caller whose token lacks the SAME permission the owning document
// type's live PDF endpoint requires — here PACKING_LIST maps to
// packing.view (permissionByDocumentType), and the token only carries
// order.view.
func TestHandler_History_DeniedWithoutPermission(t *testing.T) {
	h := NewHandler(nil) // never reached: HasPermission fails first.
	c, rec := newTestContext(1, []string{"order.view"}, gin.Params{
		{Key: "type", Value: "PACKING_LIST"}, {Key: "sourceId", Value: "42"},
	})

	h.History(c)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusForbidden, rec.Body.String())
	}
}

// TestHandler_History_UnknownDocumentType proves an unrecognized :type
// path param is rejected as a 400, not silently treated as "no
// permission required."
func TestHandler_History_UnknownDocumentType(t *testing.T) {
	h := NewHandler(nil)
	c, rec := newTestContext(1, []string{"order.view", "packing.view"}, gin.Params{
		{Key: "type", Value: "NOT_A_REAL_TYPE"}, {Key: "sourceId", Value: "42"},
	})

	h.History(c)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

// TestHandler_History_AllowedWithPermission proves the SAME call
// succeeds (reaches the real Service, over a real seeded render row)
// once the caller's token carries the owning document type's own view
// permission.
func TestHandler_History_AllowedWithPermission(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	tenantID := newTenant(t, tx, "hist-allow")
	userID := seedUser(t, tx, tenantID, "hist-allow")
	svc := NewService(NewRepository(tx), user.NewRepository(tx))
	if err := svc.Record(context.Background(), tenantID, TypePackingList, "PACK-H1", "packing_jobs", 7, userID, false, []byte("%PDF- hist")); err != nil {
		t.Fatalf("Record: %v", err)
	}

	h := NewHandler(svc)
	c, rec := newTestContext(tenantID, []string{"packing.view"}, gin.Params{
		{Key: "type", Value: "PACKING_LIST"}, {Key: "sourceId", Value: "7"},
	})

	h.History(c)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

// TestHandler_DownloadRender_DeniedWithoutPermission proves the reprint
// path checks the SAME permission as the owning document type, derived
// from the fetched row's own DocumentType — it must reject a caller
// whose token lacks packing.view even though the render id itself
// exists and belongs to their own tenant.
func TestHandler_DownloadRender_DeniedWithoutPermission(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	tenantID := newTenant(t, tx, "dl-deny")
	userID := seedUser(t, tx, tenantID, "dl-deny")
	svc := NewService(NewRepository(tx), user.NewRepository(tx))
	originalBytes := []byte("%PDF- protected content")
	if err := svc.Record(context.Background(), tenantID, TypePackingList, "PACK-D1", "packing_jobs", 8, userID, false, originalBytes); err != nil {
		t.Fatalf("Record: %v", err)
	}
	rows, err := svc.History(context.Background(), tenantID, TypePackingList, 8)
	if err != nil || len(rows) != 1 {
		t.Fatalf("History: rows=%v err=%v", rows, err)
	}

	h := NewHandler(svc)
	c, rec := newTestContext(tenantID, []string{"order.view"}, gin.Params{
		{Key: "id", Value: itoa(rows[0].ID)},
	})

	h.DownloadRender(c)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusForbidden, rec.Body.String())
	}
}

// TestHandler_DownloadRender_AllowedWithPermission_ReturnsOriginalBytes
// proves the authorized path returns the exact original bytes as the
// raw HTTP body.
func TestHandler_DownloadRender_AllowedWithPermission_ReturnsOriginalBytes(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	tenantID := newTenant(t, tx, "dl-allow")
	userID := seedUser(t, tx, tenantID, "dl-allow")
	svc := NewService(NewRepository(tx), user.NewRepository(tx))
	originalBytes := []byte("%PDF- allowed content")
	if err := svc.Record(context.Background(), tenantID, TypePackingList, "PACK-D2", "packing_jobs", 9, userID, false, originalBytes); err != nil {
		t.Fatalf("Record: %v", err)
	}
	rows, err := svc.History(context.Background(), tenantID, TypePackingList, 9)
	if err != nil || len(rows) != 1 {
		t.Fatalf("History: rows=%v err=%v", rows, err)
	}

	h := NewHandler(svc)
	c, rec := newTestContext(tenantID, []string{"packing.view"}, gin.Params{
		{Key: "id", Value: itoa(rows[0].ID)},
	})

	h.DownloadRender(c)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if rec.Body.String() != string(originalBytes) {
		t.Errorf("body = %q, want %q", rec.Body.String(), originalBytes)
	}
}

func itoa(id int64) string {
	return strconv.FormatInt(id, 10)
}
