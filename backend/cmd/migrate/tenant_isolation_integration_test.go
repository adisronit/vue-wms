package main

import (
	"context"
	"database/sql"
	"testing"

	"wms-backend/internal/platform/logging"
)

// Phase 29A integration tests — exercise the real tenant_id migrations
// (0043-0048) against a live SQL Server, mirroring
// TestMigrateUp_IsIdempotent's own "skip, never fail, when no DB_* env
// points at a reachable database" convention. Every test that mutates
// data runs inside its own transaction and always rolls back (never
// commits), so repeated runs never accumulate residue and tests never
// interfere with each other or with TestMigrateUp_IsIdempotent's own
// up/up assertions.
//
// Caller responsibility, same as connectTestDB's own doc: DB_* must
// point at a disposable database, never at a real environment.

// ensureMigratedUp runs migrateUp against the test database so these
// tests work whether or not TestMigrateUp_IsIdempotent already ran in
// this same `go test` invocation — migrateUp is idempotent, so calling
// it again here is always safe.
func ensureMigratedUp(t *testing.T, db *sql.DB) {
	t.Helper()
	ctx := context.Background()
	if err := ensureSchemaMigrationsTable(ctx, db); err != nil {
		t.Fatalf("ensure schema_migrations table: %v", err)
	}
	logger := logging.New("test", "migrate-test")
	if err := migrateUp(ctx, db, logger); err != nil {
		t.Fatalf("migrateUp: %v", err)
	}
}

// bootstrapTenantID looks up the Phase 29A bootstrap tenant created by
// 0046's backfill — fetched live rather than assumed to be 1, even
// though 0046 forces it to be exactly 1 (see 0046's own header comment
// for why that forcing exists at all).
func bootstrapTenantID(t *testing.T, db *sql.DB) int64 {
	t.Helper()
	var id int64
	if err := db.QueryRowContext(context.Background(),
		"SELECT id FROM tenants WHERE slug = 'default'").Scan(&id); err != nil {
		t.Fatalf("lookup bootstrap tenant: %v", err)
	}
	return id
}

// TestPhase29a_BootstrapTenantIsIDOne proves 0046's IDENTITY_INSERT
// forcing actually holds: the bootstrap tenant must be id = 1, which is
// what every 0047/0048 `DEFAULT (1)` constraint hardcodes. If this ever
// drifts (e.g. a future migration inserts a tenant row before 0046
// runs), every unmodified INSERT path relying on the DEFAULT silently
// starts referencing a nonexistent tenant and fails its FK check.
func TestPhase29a_BootstrapTenantIsIDOne(t *testing.T) {
	db := connectTestDB(t)
	ensureMigratedUp(t, db)

	if got := bootstrapTenantID(t, db); got != 1 {
		t.Fatalf("expected bootstrap tenant id = 1, got %d", got)
	}
}

// TestPhase29a_NoOrphanedTenantID is the orphan-detection check
// migration-plan.md §4 calls for, automated: every Tenant-Owned table
// touched by 0043-0045 must have zero NULL tenant_id rows once 0046's
// backfill (and 0047's NOT NULL) has run — not merely assumed, checked
// against the live schema the same way the audit found the gap in the
// first place.
func TestPhase29a_NoOrphanedTenantID(t *testing.T) {
	db := connectTestDB(t)
	ensureMigratedUp(t, db)

	tables := []string{
		"roles", "warehouses", "zones", "locations", "products",
		"inventory", "inventory_transactions",
		"receipts", "receipt_items", "receipt_confirmations",
		"putaway_tasks", "putaway_task_items", "putaway_confirmations",
		"picking_tasks", "picking_task_items", "picking_confirmations",
		"shipments", "shipment_items", "shipment_confirmations",
		"orders", "order_items",
		"stock_adjustments", "stock_adjustment_items", "stock_adjustment_confirmations", "stock_adjustment_reminders",
		"stock_counts", "stock_count_items", "stock_count_confirmations",
		"replenishment_tasks", "replenishment_confirmations",
		"notifications", "export_jobs",
	}

	for _, table := range tables {
		var n int
		q := "SELECT COUNT(*) FROM " + table + " WHERE tenant_id IS NULL"
		if err := db.QueryRowContext(context.Background(), q).Scan(&n); err != nil {
			t.Fatalf("%s: query orphan count: %v", table, err)
		}
		if n != 0 {
			t.Errorf("%s: found %d row(s) with NULL tenant_id after backfill", table, n)
		}
	}
}

// TestPhase29a_UsersTableHasNoTenantColumn guards the central design
// decision in tenant-er-design.md §1/§3: identity (`users`) stays
// platform-global; membership and role are per-tenant via tenant_users.
// A future migration accidentally adding tenant_id directly to `users`
// would contradict that design without this regression test catching
// it.
func TestPhase29a_UsersTableHasNoTenantColumn(t *testing.T) {
	db := connectTestDB(t)
	ensureMigratedUp(t, db)

	var n int
	err := db.QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS WHERE TABLE_NAME = 'users' AND COLUMN_NAME = 'tenant_id'").Scan(&n)
	if err != nil {
		t.Fatalf("query information_schema: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected users to have no tenant_id column (identity stays platform-global), found %d", n)
	}
}

// TestPhase29a_TenantScopedUniqueConstraint proves 0048's constraint
// swap actually enforces tenant isolation both ways: a duplicate code
// within the SAME tenant is rejected, and the SAME code is allowed
// across two DIFFERENT tenants — the exact behavior change this phase
// exists to deliver on warehouses.code (representative of every other
// rescoped document-number/code constraint in 0048).
func TestPhase29a_TenantScopedUniqueConstraint(t *testing.T) {
	db := connectTestDB(t)
	ensureMigratedUp(t, db)

	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback()

	bootstrapID := bootstrapTenantID(t, db)

	insertWarehouse := `INSERT INTO warehouses (tenant_id, code, name, type, address, contact_person, phone)
		VALUES (@p1, @p2, 'Test WH', 'DC', 'addr', 'person', 'phone')`

	if _, err := tx.ExecContext(ctx, insertWarehouse, bootstrapID, "PHASE29A-WH"); err != nil {
		t.Fatalf("first insert into bootstrap tenant should succeed: %v", err)
	}

	if _, err := tx.ExecContext(ctx, insertWarehouse, bootstrapID, "PHASE29A-WH"); err == nil {
		t.Fatal("expected duplicate code within the same tenant to be rejected by UQ_warehouses_tenant_code")
	}

	var otherTenantID int64
	if err := tx.QueryRowContext(ctx,
		"INSERT INTO tenants (slug, name, status) OUTPUT INSERTED.id VALUES ('phase29a-test-tenant', 'Phase29A Test Tenant', 'ACTIVE')").
		Scan(&otherTenantID); err != nil {
		t.Fatalf("insert second tenant: %v", err)
	}

	if _, err := tx.ExecContext(ctx, insertWarehouse, otherTenantID, "PHASE29A-WH"); err != nil {
		t.Fatalf("same code under a DIFFERENT tenant should succeed, got: %v", err)
	}
}

// TestPhase29a_DefaultConstraintBackfillsUnmodifiedInserts is the
// single most important behavioral guarantee of this phase: every
// existing, unmodified repository.Create() call in the codebase inserts
// its row without ever setting TenantID (Phase 29A's scope is
// migrations + GORM model structs + migration tests only — no
// repository/service/handler code is touched, per the task's own
// boundary). This proves the database-level safety net that makes that
// safe: an INSERT that omits tenant_id entirely must land on the
// bootstrap tenant via the column's own DEFAULT constraint, not fail or
// silently write NULL/0.
func TestPhase29a_DefaultConstraintBackfillsUnmodifiedInserts(t *testing.T) {
	db := connectTestDB(t)
	ensureMigratedUp(t, db)

	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback()

	// Deliberately omits tenant_id from the column list, the same shape
	// every pre-existing repository INSERT/GORM Create() call has.
	var gotTenantID int64
	err = tx.QueryRowContext(ctx,
		`INSERT INTO warehouses (code, name, type, address, contact_person, phone)
		 OUTPUT INSERTED.tenant_id
		 VALUES ('PHASE29A-NODEFAULT', 'No Default WH', 'DC', 'addr', 'person', 'phone')`).
		Scan(&gotTenantID)
	if err != nil {
		t.Fatalf("insert omitting tenant_id should succeed via DEFAULT constraint, got: %v", err)
	}

	bootstrapID := bootstrapTenantID(t, db)
	if gotTenantID != bootstrapID {
		t.Fatalf("expected DEFAULT constraint to backfill tenant_id = %d (bootstrap tenant), got %d", bootstrapID, gotTenantID)
	}
}

// TestPhase29a_ForeignKeyRejectsUnknownTenant proves the FK added in
// 0047 is a real, enforced constraint — not just documentation — by
// attempting to insert a row referencing a tenant_id that does not
// exist in `tenants`.
func TestPhase29a_ForeignKeyRejectsUnknownTenant(t *testing.T) {
	db := connectTestDB(t)
	ensureMigratedUp(t, db)

	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback()

	const impossibleTenantID = 9_000_000_000

	_, err = tx.ExecContext(ctx,
		`INSERT INTO warehouses (tenant_id, code, name, type, address, contact_person, phone)
		 VALUES (@p1, 'PHASE29A-GHOST', 'Ghost WH', 'DC', 'addr', 'person', 'phone')`,
		impossibleTenantID)
	if err == nil {
		t.Fatal("expected FK_warehouses_tenant to reject a tenant_id with no matching tenants row")
	}
}

// TestPhase29a_BackfillLogicEnrollsExistingUserOnce exercises 0046's
// exact tenant_users backfill statement in isolation, inside a
// transaction that always rolls back, rather than asserting it ran
// correctly during the real historical migration (not safely
// observable here: whether a real-world `users` row existed "before
// 0046" depends on the order migrate/seed were invoked when this test
// database was built, which this test has no control over — by the
// time `go test` runs, every migration has already been applied once).
// Re-running the same backfill shape (copied from 0046's up.sql)
// against a fresh test user proves the SQL itself is correct: it
// creates exactly one ACTIVE tenant_users row carrying that user's own
// role_id, and running it twice (the idempotency guard 0046 itself
// relies on, `WHERE NOT EXISTS (...)`) creates no duplicate.
func TestPhase29a_BackfillLogicEnrollsExistingUserOnce(t *testing.T) {
	db := connectTestDB(t)
	ensureMigratedUp(t, db)

	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback()

	bootstrapID := bootstrapTenantID(t, db)

	var roleID int64
	if err := tx.QueryRowContext(ctx, "SELECT TOP 1 id FROM roles WHERE tenant_id = @p1 ORDER BY id", bootstrapID).Scan(&roleID); err != nil {
		t.Fatalf("lookup a bootstrap-tenant role: %v", err)
	}

	var preExistingUserID int64
	err = tx.QueryRowContext(ctx, `
		INSERT INTO users (username, password_hash, full_name, email, role_id, status)
		OUTPUT INSERTED.id
		VALUES ('phase29a-pre-existing-user', 'x', 'Pre-Existing User', 'phase29a-pre-existing-user@test.local', @p1, 'ACTIVE')
	`, roleID).Scan(&preExistingUserID)
	if err != nil {
		t.Fatalf("insert test user simulating a pre-existing single-tenant user: %v", err)
	}

	backfillOnce := func() {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO tenant_users (tenant_id, user_id, role_id, status, joined_at)
			SELECT @p1, u.id, u.role_id, 'ACTIVE', SYSUTCDATETIME()
			FROM users u
			WHERE u.id = @p2 AND NOT EXISTS (
				SELECT 1 FROM tenant_users tu WHERE tu.tenant_id = @p1 AND tu.user_id = u.id
			)
		`, bootstrapID, preExistingUserID)
		if err != nil {
			t.Fatalf("run 0046-shaped backfill insert: %v", err)
		}
	}

	backfillOnce()
	backfillOnce() // idempotency: must not create a duplicate row

	var count int
	var gotStatus string
	var gotRoleID int64
	err = tx.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM tenant_users WHERE tenant_id = @p1 AND user_id = @p2",
		bootstrapID, preExistingUserID).Scan(&count)
	if err != nil {
		t.Fatalf("count tenant_users rows: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 tenant_users row after running the backfill twice, got %d", count)
	}

	err = tx.QueryRowContext(ctx,
		"SELECT status, role_id FROM tenant_users WHERE tenant_id = @p1 AND user_id = @p2",
		bootstrapID, preExistingUserID).Scan(&gotStatus, &gotRoleID)
	if err != nil {
		t.Fatalf("read backfilled tenant_users row: %v", err)
	}
	if gotStatus != "ACTIVE" {
		t.Errorf("expected backfilled status ACTIVE (pre-existing user, not a fresh invitation), got %q", gotStatus)
	}
	if gotRoleID != roleID {
		t.Errorf("expected backfilled role_id to carry the user's own existing role %d, got %d", roleID, gotRoleID)
	}
}
