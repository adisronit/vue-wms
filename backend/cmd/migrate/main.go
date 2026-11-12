// Command migrate applies (or rolls back) backend/migrations/*.sql
// against SQL Server, and seeds Phase 2 RBAC reference data. It exists
// because CLAUDE.md and the Phase 2 plan explicitly forbid relying on
// GORM AutoMigrate as the migration strategy: schema changes must be
// reproducible, plain SQL files.
//
// Usage:
//
//	go run ./cmd/migrate up     # apply every pending migration
//	go run ./cmd/migrate down   # roll back the most recently applied one
//	go run ./cmd/migrate seed   # insert roles/permissions/dev admin user
package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"sort"
	"strings"

	_ "github.com/microsoft/go-mssqldb"
	"golang.org/x/crypto/bcrypt"

	"wms-backend/config"
	"wms-backend/internal/platform/logging"
	"wms-backend/internal/rbac"
	"wms-backend/migrations"
)

var versionPattern = regexp.MustCompile(`^(\d+)_`)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: migrate <up|down|seed>")
		os.Exit(1)
	}
	command := os.Args[1]

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "configuration error: "+err.Error())
		os.Exit(1)
	}

	logger := logging.New(cfg.AppEnv, "migrate")

	if err := ensureDatabaseExists(context.Background(), cfg, logger); err != nil {
		logger.Error("failed to ensure application database exists", slog.String("error", err.Error()))
		os.Exit(1)
	}

	db, err := sql.Open("sqlserver", cfg.DB.DSN())
	if err != nil {
		logger.Error("failed to open database", slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer db.Close()

	ctx := context.Background()
	if err := db.PingContext(ctx); err != nil {
		logger.Error("database not reachable", slog.String("error", err.Error()))
		os.Exit(1)
	}

	if err := ensureSchemaMigrationsTable(ctx, db); err != nil {
		logger.Error("failed to ensure schema_migrations table", slog.String("error", err.Error()))
		os.Exit(1)
	}

	switch command {
	case "up":
		err = migrateUp(ctx, db, logger)
	case "down":
		err = migrateDown(ctx, db, logger)
	case "seed":
		err = seed(ctx, db, logger, cfg)
	default:
		fmt.Fprintln(os.Stderr, "unknown command: "+command)
		os.Exit(1)
	}

	if err != nil {
		logger.Error("command failed", slog.String("command", command), slog.String("error", err.Error()))
		os.Exit(1)
	}

	logger.Info("command completed", slog.String("command", command))
}

// ensureDatabaseExists connects to SQL Server's master database and
// creates the application database (cfg.DB.Name) if it doesn't exist yet
// — the case on a brand new `docker compose up` with a fresh sqlserver
// volume, where only "master" exists until something creates "wms".
// CREATE DATABASE cannot run inside a transaction/batch with other
// statements, so this uses its own short-lived connection to master
// rather than reusing the main *sql.DB (which always targets cfg.DB.Name
// and would fail to even open before the database exists).
func ensureDatabaseExists(ctx context.Context, cfg *config.Config, logger *slog.Logger) error {
	masterDB, err := sql.Open("sqlserver", cfg.DB.MasterDSN())
	if err != nil {
		return err
	}
	defer masterDB.Close()

	if err := masterDB.PingContext(ctx); err != nil {
		return err
	}

	var exists int
	err = masterDB.QueryRowContext(ctx, "SELECT 1 FROM sys.databases WHERE name = @p1", cfg.DB.Name).Scan(&exists)
	if err == nil {
		return nil
	}
	if err != sql.ErrNoRows {
		return err
	}

	if _, err := masterDB.ExecContext(ctx, fmt.Sprintf("CREATE DATABASE [%s]", cfg.DB.Name)); err != nil {
		return err
	}

	logger.Info("application database created", slog.String("database", cfg.DB.Name))
	return nil
}

func ensureSchemaMigrationsTable(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
IF NOT EXISTS (SELECT 1 FROM sys.tables WHERE name = 'schema_migrations')
BEGIN
    CREATE TABLE schema_migrations (
        version    VARCHAR(20) NOT NULL PRIMARY KEY,
        applied_at DATETIME2   NOT NULL CONSTRAINT DF_schema_migrations_applied_at DEFAULT (SYSUTCDATETIME())
    );
END`)
	return err
}

type migrationFile struct {
	version string
	name    string
}

func loadMigrations(suffix string) ([]migrationFile, error) {
	entries, err := migrations.Files.ReadDir(".")
	if err != nil {
		return nil, err
	}

	var files []migrationFile
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), suffix) {
			continue
		}
		m := versionPattern.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		files = append(files, migrationFile{version: m[1], name: e.Name()})
	}

	sort.Slice(files, func(i, j int) bool { return files[i].version < files[j].version })
	return files, nil
}

func appliedVersions(ctx context.Context, db *sql.DB) (map[string]bool, error) {
	rows, err := db.QueryContext(ctx, "SELECT version FROM schema_migrations")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	applied := map[string]bool{}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		applied[v] = true
	}
	return applied, rows.Err()
}

func migrateUp(ctx context.Context, db *sql.DB, logger *slog.Logger) error {
	files, err := loadMigrations(".up.sql")
	if err != nil {
		return err
	}

	applied, err := appliedVersions(ctx, db)
	if err != nil {
		return err
	}

	for _, f := range files {
		if applied[f.version] {
			continue
		}

		if err := runMigrationFile(ctx, db, f); err != nil {
			return fmt.Errorf("migration %s failed: %w", f.name, err)
		}

		logger.Info("migration applied", slog.String("version", f.version), slog.String("file", f.name))
	}

	return nil
}

func migrateDown(ctx context.Context, db *sql.DB, logger *slog.Logger) error {
	applied, err := appliedVersions(ctx, db)
	if err != nil {
		return err
	}
	if len(applied) == 0 {
		logger.Info("no migrations to roll back")
		return nil
	}

	var versions []string
	for v := range applied {
		versions = append(versions, v)
	}
	sort.Strings(versions)
	last := versions[len(versions)-1]

	downFiles, err := loadMigrations(".down.sql")
	if err != nil {
		return err
	}

	for _, f := range downFiles {
		if f.version != last {
			continue
		}

		content, err := migrations.Files.ReadFile(f.name)
		if err != nil {
			return err
		}

		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}

		if _, err := tx.ExecContext(ctx, string(content)); err != nil {
			_ = tx.Rollback()
			return err
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM schema_migrations WHERE version = @p1", last); err != nil {
			_ = tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}

		logger.Info("migration rolled back", slog.String("version", last), slog.String("file", f.name))
		return nil
	}

	return fmt.Errorf("no down migration found for version %s", last)
}

func runMigrationFile(ctx context.Context, db *sql.DB, f migrationFile) error {
	content, err := migrations.Files.ReadFile(f.name)
	if err != nil {
		return err
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, string(content)); err != nil {
		_ = tx.Rollback()
		return err
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO schema_migrations (version) VALUES (@p1)", f.version); err != nil {
		_ = tx.Rollback()
		return err
	}

	return tx.Commit()
}

// seed inserts RBAC reference data and one development admin user. Every
// step is idempotent (checked by existence, not re-inserted) so `seed`
// can be run again safely — e.g. after a fresh `docker compose up` where
// the volume already has data from a previous run.
func seed(ctx context.Context, db *sql.DB, logger *slog.Logger, cfg *config.Config) error {
	roleIDs, err := seedRoles(ctx, db, logger)
	if err != nil {
		return fmt.Errorf("seed roles: %w", err)
	}

	permissionIDs, err := seedPermissions(ctx, db, logger)
	if err != nil {
		return fmt.Errorf("seed permissions: %w", err)
	}

	if err := seedRolePermissions(ctx, db, logger, roleIDs, permissionIDs); err != nil {
		return fmt.Errorf("seed role_permissions: %w", err)
	}

	adminUserID, err := seedAdminUser(ctx, db, logger, roleIDs)
	if err != nil {
		return fmt.Errorf("seed admin user: %w", err)
	}

	// Phase 29B: give the seeded dev admin an ACTIVE membership in a
	// bootstrap tenant, so RequireTenantContext's fail-closed check (no
	// handler ever runs without a verified active tenant membership) has
	// something to succeed against in a fresh dev/CI environment.
	// adminUserID is 0 when SEED_ADMIN_USERNAME/PASSWORD were not set —
	// seedAdminUser already warned about that, so this step is skipped
	// the same way, silently.
	if adminUserID != 0 {
		if err := seedDefaultTenantMembership(ctx, db, logger, adminUserID, roleIDs["ADMIN"]); err != nil {
			return fmt.Errorf("seed default tenant membership: %w", err)
		}
	}

	return nil
}

func seedRoles(ctx context.Context, db *sql.DB, logger *slog.Logger) (map[string]int64, error) {
	ids := map[string]int64{}

	for _, r := range rbac.RoleCatalog {
		var id int64
		err := db.QueryRowContext(ctx, "SELECT id FROM roles WHERE code = @p1", r.Code).Scan(&id)
		switch {
		case err == sql.ErrNoRows:
			err = db.QueryRowContext(ctx,
				`INSERT INTO roles (code, name, description) OUTPUT INSERTED.id VALUES (@p1, @p2, @p3)`,
				r.Code, r.Name, r.Description,
			).Scan(&id)
			if err != nil {
				return nil, err
			}
			logger.Info("role seeded", slog.String("code", r.Code))
		case err != nil:
			return nil, err
		}
		ids[r.Code] = id
	}

	return ids, nil
}

func seedPermissions(ctx context.Context, db *sql.DB, logger *slog.Logger) (map[string]int64, error) {
	ids := map[string]int64{}

	for _, p := range rbac.PermissionCatalog() {
		var id int64
		err := db.QueryRowContext(ctx, "SELECT id FROM permissions WHERE code = @p1", p.Code).Scan(&id)
		switch {
		case err == sql.ErrNoRows:
			err = db.QueryRowContext(ctx,
				`INSERT INTO permissions (code, module, action, description) OUTPUT INSERTED.id VALUES (@p1, @p2, @p3, @p4)`,
				p.Code, p.Module, p.Action, p.Description,
			).Scan(&id)
			if err != nil {
				return nil, err
			}
		case err != nil:
			return nil, err
		}
		ids[p.Code] = id
	}

	logger.Info("permissions seeded", slog.Int("count", len(ids)))
	return ids, nil
}

func seedRolePermissions(ctx context.Context, db *sql.DB, logger *slog.Logger, roleIDs, permissionIDs map[string]int64) error {
	for roleCode, grantedCodes := range rbac.RoleDefaultGrants {
		roleID, ok := roleIDs[roleCode]
		if !ok {
			return fmt.Errorf("seed role_permissions: unknown role code %q", roleCode)
		}

		for _, code := range grantedCodes {
			permissionID, ok := permissionIDs[code]
			if !ok {
				return fmt.Errorf("seed role_permissions: unknown permission code %q", code)
			}

			var exists int
			err := db.QueryRowContext(ctx,
				"SELECT 1 FROM role_permissions WHERE role_id = @p1 AND permission_id = @p2",
				roleID, permissionID,
			).Scan(&exists)
			switch {
			case err == sql.ErrNoRows:
				if _, err := db.ExecContext(ctx,
					"INSERT INTO role_permissions (role_id, permission_id) VALUES (@p1, @p2)",
					roleID, permissionID,
				); err != nil {
					return err
				}
			case err != nil:
				return err
			}
		}
	}

	logger.Info("role_permissions seeded")
	return nil
}

// seedAdminUser creates exactly one development admin user, from
// SEED_ADMIN_USERNAME/SEED_ADMIN_PASSWORD — never a hardcoded password.
// Both variables are required; seeding is skipped (not defaulted) when
// either is absent, and the password is never logged. Returns the
// admin's user id (whether just-created or pre-existing) so callers can
// grant it a tenant membership (see seedDefaultTenantMembership); returns
// 0, nil when seeding was skipped.
func seedAdminUser(ctx context.Context, db *sql.DB, logger *slog.Logger, roleIDs map[string]int64) (int64, error) {
	username := os.Getenv("SEED_ADMIN_USERNAME")
	password := os.Getenv("SEED_ADMIN_PASSWORD")

	if username == "" || password == "" {
		logger.Warn("skipping admin user seed: SEED_ADMIN_USERNAME/SEED_ADMIN_PASSWORD not set")
		return 0, nil
	}

	var existingID int64
	err := db.QueryRowContext(ctx, "SELECT id FROM users WHERE username = @p1", username).Scan(&existingID)
	if err == nil {
		logger.Info("admin user already exists, skipping seed", slog.String("username", username))
		return existingID, nil
	}
	if err != sql.ErrNoRows {
		return 0, err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return 0, err
	}

	adminRoleID, ok := roleIDs["ADMIN"]
	if !ok {
		return 0, fmt.Errorf("ADMIN role not found — run role seeding first")
	}

	var newID int64
	err = db.QueryRowContext(ctx,
		`INSERT INTO users (username, password_hash, full_name, email, role_id, status)
		 OUTPUT INSERTED.id VALUES (@p1, @p2, @p3, @p4, @p5, 'ACTIVE')`,
		username, string(hash), "Administrator", username+"@smartwms.local", adminRoleID,
	).Scan(&newID)
	if err != nil {
		return 0, err
	}

	logger.Info("admin user seeded", slog.String("username", username))
	return newID, nil
}

// seedDefaultTenantMembership is Phase 29B's minimal bootstrap: it creates
// exactly one tenant (slug "default") and grants adminUserID an ACTIVE
// membership in it with adminRoleID, so RequireTenantContext's fail-closed
// check and auth.Service.Login's DefaultMembership resolution have
// something to succeed against in a fresh dev/CI environment.
//
// This is deliberately NOT the Phase 29 production backfill: migrating
// every pre-existing user onto a tenant_id is a database-schema-change
// concern tracked separately (docs/proposals/wms-saas/migration-plan.md
// §2, migrations 0043+) and out of this auth-focused phase's scope — only
// the one admin user this same seed command already creates is granted
// a membership here, nothing else in the users table is touched.
func seedDefaultTenantMembership(ctx context.Context, db *sql.DB, logger *slog.Logger, adminUserID, adminRoleID int64) error {
	const slug = "default"

	var tenantID int64
	err := db.QueryRowContext(ctx, "SELECT id FROM tenants WHERE slug = @p1", slug).Scan(&tenantID)
	switch {
	case err == sql.ErrNoRows:
		err = db.QueryRowContext(ctx,
			`INSERT INTO tenants (slug, name, status) OUTPUT INSERTED.id VALUES (@p1, @p2, 'ACTIVE')`,
			slug, "Default Tenant",
		).Scan(&tenantID)
		if err != nil {
			return err
		}
		logger.Info("default tenant seeded", slog.String("slug", slug))
	case err != nil:
		return err
	}

	var exists int
	err = db.QueryRowContext(ctx,
		"SELECT 1 FROM tenant_users WHERE tenant_id = @p1 AND user_id = @p2", tenantID, adminUserID,
	).Scan(&exists)
	switch {
	case err == sql.ErrNoRows:
		if _, err := db.ExecContext(ctx,
			`INSERT INTO tenant_users (tenant_id, user_id, role_id, status, joined_at)
			 VALUES (@p1, @p2, @p3, 'ACTIVE', SYSUTCDATETIME())`,
			tenantID, adminUserID, adminRoleID,
		); err != nil {
			return err
		}
		logger.Info("default tenant membership seeded", slog.Int64("user_id", adminUserID), slog.Int64("tenant_id", tenantID))
	case err != nil:
		return err
	}

	return nil
}
