// Package testsupport provides a shared, skip-if-unavailable SQL Server
// connection helper for integration tests that need to exercise real
// database constraints (uniqueness, foreign keys, CHECK constraints)
// rather than a mock — consistent with this codebase's existing
// convention of verifying repository/DB behavior against real SQL
// Server instead of a mocking framework (see internal/role's own
// service_test.go doc comment).
//
// Nothing in cmd/server, cmd/worker, or cmd/migrate imports this
// package, so it never ships in any built binary — it exists purely
// for `go test`.
package testsupport

import (
	"context"
	"testing"
	"time"

	"gorm.io/driver/sqlserver"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"wms-backend/config"
)

// ConnectDB opens a *gorm.DB against whatever database the standard
// DB_* environment variables point at (the same ones config.Load reads
// for the real application) and skips the calling test — never fails
// it — when no usable configuration or live connection is available.
// Callers are responsible for pointing DB_* at a disposable database
// before running these tests; this helper does not know or care
// whether the target is disposable, so it must never be wired into any
// automated pipeline that targets a real environment without that
// guarantee being made explicitly elsewhere.
func ConnectDB(t *testing.T) *gorm.DB {
	t.Helper()

	cfg, err := config.Load()
	if err != nil {
		t.Skipf("skipping: no test database configured (%v)", err)
		return nil
	}

	db, err := gorm.Open(sqlserver.Open(cfg.DB.DSN()), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Skipf("skipping: could not open test database: %v", err)
		return nil
	}

	sqlDB, err := db.DB()
	if err != nil {
		t.Skipf("skipping: could not access test database handle: %v", err)
		return nil
	}

	pingCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(pingCtx); err != nil {
		t.Skipf("skipping: test database not reachable: %v", err)
		return nil
	}

	return db
}

// BeginRollbackTx starts a transaction on db and registers a cleanup
// that always rolls it back, regardless of test outcome — so constraint
// tests can INSERT freely without leaving rows behind on a disposable
// database that may be reused across test runs. SQL Server enforces
// UNIQUE/FK/CHECK constraints within an uncommitted transaction exactly
// as it would after commit, so this does not weaken what is verified.
func BeginRollbackTx(t *testing.T, db *gorm.DB) *gorm.DB {
	t.Helper()

	tx := db.Begin()
	if tx.Error != nil {
		t.Fatalf("begin transaction: %v", tx.Error)
	}

	t.Cleanup(func() {
		_ = tx.Rollback()
	})

	return tx
}
