// Phase 33C.3 -- Document History / Reprint tests. Mirrors internal/
// pickingbatch's own tenant_isolation_test.go shape (real seeded rows
// via testsupport.ConnectDB + BeginRollbackTx, not a mocked repository)
// — this package's own Repository is a thin GORM wrapper with no
// business logic worth mocking around.
package docrender

import (
	"bytes"
	"context"
	"testing"

	"gorm.io/gorm"

	"wms-backend/internal/platform/testsupport"
	"wms-backend/internal/user"
)

func newTenant(t *testing.T, tx *gorm.DB, label string) int64 {
	t.Helper()
	var tenantID int64
	if err := tx.Raw(
		"INSERT INTO tenants (slug, name, status) OUTPUT INSERTED.id VALUES (?, ?, 'ACTIVE')",
		"dr-"+label, "DocRender Test Tenant "+label,
	).Scan(&tenantID).Error; err != nil {
		t.Fatalf("newTenant(%s): %v", label, err)
	}
	return tenantID
}

func seedUser(t *testing.T, tx *gorm.DB, tenantID int64, label string) int64 {
	t.Helper()
	var roleID int64
	if err := tx.Raw(
		"INSERT INTO roles (tenant_id, code, name) OUTPUT INSERTED.id VALUES (?, ?, ?)",
		tenantID, "DR-"+label, "DocRender Role "+label,
	).Scan(&roleID).Error; err != nil {
		t.Fatalf("seedUser(%s): insert role: %v", label, err)
	}
	var userID int64
	if err := tx.Raw(
		"INSERT INTO users (username, password_hash, full_name, email, role_id) OUTPUT INSERTED.id VALUES (?, 'x', ?, ?, ?)",
		"dr-"+label, "DocRender User "+label, "dr-"+label+"@example.com", roleID,
	).Scan(&userID).Error; err != nil {
		t.Fatalf("seedUser(%s): insert user: %v", label, err)
	}
	return userID
}

// TestService_Record_History_Fetch_ImmutableSnapshot is the core
// "immutable historical snapshot" proof this phase's own spec requires:
// render the same logical document twice (simulating a data change
// between the two renders — here, just two different byte payloads),
// confirm History reports two distinct rows newest-first, and confirm
// Fetch against the EARLIER row's id returns the ORIGINAL bytes byte-
// for-byte, never the later payload — proving a reprint is a lookup,
// never a re-render.
func TestService_Record_History_Fetch_ImmutableSnapshot(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	tenantID := newTenant(t, tx, "snapshot")
	userID := seedUser(t, tx, tenantID, "snapshot")

	svc := NewService(NewRepository(tx), user.NewRepository(tx))
	ctx := context.Background()

	firstBytes := []byte("%PDF-1.4 FIRST VERSION OF THE DOCUMENT")
	if err := svc.Record(ctx, tenantID, TypePackingList, "PACK-000001", "packing_jobs", 42, userID, true, firstBytes); err != nil {
		t.Fatalf("Record(first): %v", err)
	}

	// Simulate the underlying packing_jobs row changing (e.g. more items
	// packed, checker-confirmed) between the two renders -- a second,
	// DIFFERENT payload recorded for the exact same source row.
	secondBytes := []byte("%PDF-1.4 SECOND VERSION AFTER CHECKER CONFIRM -- LONGER PAYLOAD")
	if err := svc.Record(ctx, tenantID, TypePackingList, "PACK-000001", "packing_jobs", 42, userID, false, secondBytes); err != nil {
		t.Fatalf("Record(second): %v", err)
	}

	history, err := svc.History(ctx, tenantID, TypePackingList, 42)
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("History returned %d rows, want 2", len(history))
	}
	// Newest first.
	if history[0].IsDraft {
		t.Error("history[0] (the second, non-draft render) reported IsDraft = true")
	}
	if !history[1].IsDraft {
		t.Error("history[1] (the first, draft render) reported IsDraft = false")
	}
	if history[0].RenderedByName == nil || *history[0].RenderedByName == "" {
		t.Error("history[0].RenderedByName was not resolved")
	}

	earlierID := history[1].ID
	laterID := history[0].ID

	earlier, err := svc.Fetch(ctx, tenantID, earlierID)
	if err != nil {
		t.Fatalf("Fetch(earlier): %v", err)
	}
	if !bytes.Equal(earlier.PDFData, firstBytes) {
		t.Errorf("Fetch(earlier) returned %q, want the ORIGINAL first-render bytes %q — a reprint must never reflect the later change", earlier.PDFData, firstBytes)
	}
	if bytes.Equal(earlier.PDFData, secondBytes) {
		t.Fatal("Fetch(earlier) returned the SECOND render's bytes — this is exactly the re-render-on-demand bug the immutable snapshot design must prevent")
	}

	later, err := svc.Fetch(ctx, tenantID, laterID)
	if err != nil {
		t.Fatalf("Fetch(later): %v", err)
	}
	if !bytes.Equal(later.PDFData, secondBytes) {
		t.Errorf("Fetch(later) returned %q, want the second-render bytes %q", later.PDFData, secondBytes)
	}
}

// TestService_History_ScopedToTenant proves one tenant's render rows
// never leak into another tenant's History call, even for the exact
// same document_type/source_id pair — the same tenant-isolation
// guarantee every other tenant-scoped Repository/Service in this
// codebase carries.
func TestService_History_ScopedToTenant(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	tenantA := newTenant(t, tx, "iso-a")
	tenantB := newTenant(t, tx, "iso-b")
	userA := seedUser(t, tx, tenantA, "iso-a")

	svc := NewService(NewRepository(tx), user.NewRepository(tx))
	ctx := context.Background()

	if err := svc.Record(ctx, tenantA, TypeOrder, "ORD-A", "orders", 99, userA, false, []byte("%PDF- tenant A")); err != nil {
		t.Fatalf("Record: %v", err)
	}

	historyA, err := svc.History(ctx, tenantA, TypeOrder, 99)
	if err != nil {
		t.Fatalf("History(tenantA): %v", err)
	}
	if len(historyA) != 1 {
		t.Fatalf("History(tenantA) = %d rows, want 1", len(historyA))
	}

	historyB, err := svc.History(ctx, tenantB, TypeOrder, 99)
	if err != nil {
		t.Fatalf("History(tenantB): %v", err)
	}
	if len(historyB) != 0 {
		t.Fatalf("History(tenantB) = %d rows, want 0 (tenant A's render must not leak)", len(historyB))
	}

	if _, err := svc.Fetch(ctx, tenantB, historyA[0].ID); err != ErrNotFound {
		t.Fatalf("Fetch(tenantB, tenantA's own render id) = %v, want ErrNotFound", err)
	}
}
