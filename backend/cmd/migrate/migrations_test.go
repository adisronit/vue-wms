package main

import (
	"context"
	"database/sql"
	"testing"

	_ "github.com/microsoft/go-mssqldb"

	"wms-backend/config"
	"wms-backend/internal/platform/logging"
)

// Static checks over the embedded migration file set — no database
// required, so these always run (they are not skippable).

func TestMigrationFiles_EveryUpHasMatchingDown(t *testing.T) {
	ups, err := loadMigrations(".up.sql")
	if err != nil {
		t.Fatalf("load up migrations: %v", err)
	}
	downs, err := loadMigrations(".down.sql")
	if err != nil {
		t.Fatalf("load down migrations: %v", err)
	}

	downVersions := map[string]bool{}
	for _, d := range downs {
		downVersions[d.version] = true
	}

	for _, u := range ups {
		if !downVersions[u.version] {
			t.Errorf("migration %s (version %s) has no matching .down.sql file", u.name, u.version)
		}
	}
}

func TestMigrationFiles_NoDuplicateVersions(t *testing.T) {
	ups, err := loadMigrations(".up.sql")
	if err != nil {
		t.Fatalf("load up migrations: %v", err)
	}

	seen := map[string]string{}
	for _, u := range ups {
		if prev, ok := seen[u.version]; ok {
			t.Errorf("version %s used by both %s and %s", u.version, prev, u.name)
		}
		seen[u.version] = u.name
	}
}

func TestMigrationFiles_Phase28FilesPresentAndNamed(t *testing.T) {
	ups, err := loadMigrations(".up.sql")
	if err != nil {
		t.Fatalf("load up migrations: %v", err)
	}

	want := map[string]string{
		"0041": "0041_create_tenants_and_platform_admins.up.sql",
		"0042": "0042_create_tenant_support_tables.up.sql",
	}
	found := map[string]string{}
	for _, u := range ups {
		found[u.version] = u.name
	}

	for version, name := range want {
		got, ok := found[version]
		if !ok {
			t.Errorf("expected migration version %s to exist, found none", version)
			continue
		}
		if got != name {
			t.Errorf("version %s: expected file %s, got %s", version, name, got)
		}
	}
}

// TestMigrationFiles_Phase29aFilesPresentAndNamed mirrors
// TestMigrationFiles_Phase28FilesPresentAndNamed for the six Phase 29A
// tenant-isolation migrations. 0047/0048 are intentionally NOT in
// migration-plan.md's own numbering order (that draft put the unique-
// constraint swap before the NOT NULL/FK step) — see 0047's own header
// comment for why reality disagreed with the draft.
func TestMigrationFiles_Phase29aFilesPresentAndNamed(t *testing.T) {
	ups, err := loadMigrations(".up.sql")
	if err != nil {
		t.Fatalf("load up migrations: %v", err)
	}

	want := map[string]string{
		"0043": "0043_add_tenant_id_nullable_master_inventory_rbac.up.sql",
		"0044": "0044_add_tenant_id_nullable_inbound_outbound.up.sql",
		"0045": "0045_add_tenant_id_nullable_operations.up.sql",
		"0046": "0046_backfill_tenant_id.up.sql",
		"0047": "0047_tenant_id_not_null_and_fk.up.sql",
		"0048": "0048_tenant_scope_unique_constraints.up.sql",
	}
	found := map[string]string{}
	for _, u := range ups {
		found[u.version] = u.name
	}

	for version, name := range want {
		got, ok := found[version]
		if !ok {
			t.Errorf("expected migration version %s to exist, found none", version)
			continue
		}
		if got != name {
			t.Errorf("version %s: expected file %s, got %s", version, name, got)
		}
	}
}

func TestMigrationFiles_SortedByVersionAscending(t *testing.T) {
	ups, err := loadMigrations(".up.sql")
	if err != nil {
		t.Fatalf("load up migrations: %v", err)
	}
	for i := 1; i < len(ups); i++ {
		if ups[i-1].version >= ups[i].version {
			t.Fatalf("migrations not strictly ascending at index %d: %s then %s", i, ups[i-1].version, ups[i].version)
		}
	}
}

// connectTestDB mirrors main()'s own connection setup against whatever
// DB_* environment variables are configured, skipping (never failing)
// the calling test when no usable database is reachable. Duplicated in
// miniature from internal/platform/testsupport.ConnectDB because this
// package talks to *sql.DB directly, not *gorm.DB — callers remain
// responsible for pointing DB_* at a disposable database, never at a
// real environment, before running this test.
func connectTestDB(t *testing.T) *sql.DB {
	t.Helper()

	cfg, err := config.Load()
	if err != nil {
		t.Skipf("skipping: no test database configured (%v)", err)
	}

	db, err := sql.Open("sqlserver", cfg.DB.DSN())
	if err != nil {
		t.Skipf("skipping: could not open test database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if err := db.PingContext(context.Background()); err != nil {
		t.Skipf("skipping: test database not reachable: %v", err)
	}

	return db
}

// TestMigrateUp_IsIdempotent proves the actual safe-rerun behavior the
// migrate tool promises in its own doc comment: running `up` twice in a
// row applies every pending migration once, then does nothing on the
// second run. This exercises the real functions `main` calls, against
// a real database, rather than only inspecting file names.
func TestMigrateUp_IsIdempotent(t *testing.T) {
	db := connectTestDB(t)
	logger := logging.New("test", "migrate-test")

	if err := ensureSchemaMigrationsTable(context.Background(), db); err != nil {
		t.Fatalf("ensure schema_migrations table: %v", err)
	}

	if err := migrateUp(context.Background(), db, logger); err != nil {
		t.Fatalf("first migrateUp: %v", err)
	}

	before, err := appliedVersions(context.Background(), db)
	if err != nil {
		t.Fatalf("read applied versions: %v", err)
	}

	if err := migrateUp(context.Background(), db, logger); err != nil {
		t.Fatalf("second migrateUp (expected no-op): %v", err)
	}

	after, err := appliedVersions(context.Background(), db)
	if err != nil {
		t.Fatalf("read applied versions after rerun: %v", err)
	}

	if len(before) != len(after) {
		t.Fatalf("expected re-running `up` to apply nothing new, had %d applied versions before and %d after", len(before), len(after))
	}
	for v := range before {
		if !after[v] {
			t.Fatalf("version %s present before rerun but missing after — rerun should be a pure no-op", v)
		}
	}
}
