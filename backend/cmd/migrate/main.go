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
	// Deployment Readiness fix: resolved FIRST (rather than lazily inside
	// seedDefaultTenantMembership, which used to create it last) so
	// seedRoles below has an explicit, unambiguous tenant_id to scope
	// every role lookup/insert to — see seedRoles' own doc for the bug
	// this closes.
	defaultTenantID, err := ensureDefaultTenant(ctx, db, logger)
	if err != nil {
		return fmt.Errorf("seed default tenant: %w", err)
	}

	roleIDs, err := seedRoles(ctx, db, logger, defaultTenantID)
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

	// WMS-29E-fix: internal/tenant.Repository.CreateWithAudit seeds a new
	// tenant's own role catalog (roles.tenant_id + UQ_roles_tenant_code,
	// migrations/0043/0048) inside the Platform Admin API's create
	// transaction — but only for a tenant created by a backend binary that
	// already contains that code. Any tenant created against an older
	// binary (or, as discovered for tenant "EWTA", created in the window
	// between the fix merging to main and the running backend container
	// actually being rebuilt from it) ends up with zero roles and can
	// never be assigned a membership at all ("role_code does not match
	// any role in this tenant" for every code, including a perfectly
	// valid one). backfillTenantRoleCatalogs closes that gap for every
	// *existing* tenant, the same idempotent, additive, re-run-safe shape
	// seedRoles/seedPermissions/seedRolePermissions above already use —
	// and, because this seed command already runs on every
	// `docker compose up` (see docker-compose.yml's migrate service), it
	// also means a future redeploy self-heals this class of gap instead
	// of requiring a manual SQL repair each time.
	if err := backfillTenantRoleCatalogs(ctx, db, logger, permissionIDs); err != nil {
		return fmt.Errorf("backfill tenant role catalogs: %w", err)
	}

	// Deployment Readiness fix: backfillTenantRoleCatalogs above only
	// ever touches a tenant with literally ZERO roles — it closes the
	// "tenant never got a role catalog at all" gap, not the "tenant has
	// a role catalog, but internal/rbac.catalog.go gained a new module
	// since that catalog was seeded" gap (e.g. Master Data Expansion's
	// product_category/customer/supplier). Nothing before this line
	// revisits an already-populated tenant's roles at all. See this
	// function's own doc for the full explanation; strictly additive and
	// idempotent, like every seed step above.
	if err := grantCatalogPermissionsToAllTenants(ctx, db, logger, permissionIDs); err != nil {
		return fmt.Errorf("grant catalog permissions to all tenants: %w", err)
	}

	adminUserID, err := seedAdminUser(ctx, db, logger, roleIDs)
	if err != nil {
		return fmt.Errorf("seed admin user: %w", err)
	}

	// Phase 29B: give the seeded dev admin an ACTIVE membership in the
	// bootstrap tenant resolved above, so RequireTenantContext's
	// fail-closed check (no handler ever runs without a verified active
	// tenant membership) has something to succeed against in a fresh
	// dev/CI environment. adminUserID is 0 when SEED_ADMIN_USERNAME/
	// PASSWORD were not set — seedAdminUser already warned about that,
	// so this step is skipped the same way, silently.
	if adminUserID != 0 {
		if err := seedDefaultTenantMembership(ctx, db, logger, defaultTenantID, adminUserID, roleIDs["ADMIN"]); err != nil {
			return fmt.Errorf("seed default tenant membership: %w", err)
		}
	}

	return nil
}

// ensureDefaultTenant resolves the bootstrap/default tenant's id (slug
// "default"), creating it only if it somehow doesn't exist yet — in
// normal operation migrations/0046 already creates this row (forced to
// id = 1) strictly before `cmd/migrate seed` ever runs, so the INSERT
// branch below is a defensive fallback, not the expected path. Moved
// here, ahead of every other seed step, specifically so seedRoles has
// an explicit, unambiguous tenant_id to scope its role lookups/inserts
// to, rather than relying on roles.tenant_id's DEFAULT(1) constraint
// implicitly doing the right thing.
func ensureDefaultTenant(ctx context.Context, db *sql.DB, logger *slog.Logger) (int64, error) {
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
			return 0, err
		}
		logger.Info("default tenant seeded", slog.String("slug", slug))
		return tenantID, nil
	case err != nil:
		return 0, err
	}

	return tenantID, nil
}

// seedRoles seeds rbac.RoleCatalog's 7 roles for tenantID specifically —
// deliberately the ONE bootstrap/default tenant (ensureDefaultTenant's
// id), not "whichever tenant happens to own the first row matching this
// code." Before this fix, the lookup below had no tenant_id filter at
// all (`WHERE code = @p1`): that was safe only as long as roles.code was
// globally unique, which migrations 0043-0047 ended when they rescoped
// it to UQ_roles_tenant_code (tenant_id, code) — once more than one
// tenant exists with its own "ADMIN"/"WAREHOUSE_MANAGER"/... role (the
// normal state of any real multi-tenant deployment), an unscoped SELECT
// matches an arbitrary one of them with no ORDER BY to make that
// deterministic, silently handing seedRolePermissions/seedAdminUser/
// seedDefaultTenantMembership below the wrong tenant's role id. Scoping
// every lookup and insert to tenantID closes that gap; every OTHER
// tenant's own role catalog is grantCatalogPermissionsToAllTenants'
// responsibility below, never this function's.
func seedRoles(ctx context.Context, db sqlExecutor, logger *slog.Logger, tenantID int64) (map[string]int64, error) {
	ids := map[string]int64{}

	for _, r := range rbac.RoleCatalog {
		var id int64
		err := db.QueryRowContext(ctx, "SELECT id FROM roles WHERE tenant_id = @p1 AND code = @p2", tenantID, r.Code).Scan(&id)
		switch {
		case err == sql.ErrNoRows:
			err = db.QueryRowContext(ctx,
				`INSERT INTO roles (tenant_id, code, name, description, is_active) OUTPUT INSERTED.id VALUES (@p1, @p2, @p3, @p4, 1)`,
				tenantID, r.Code, r.Name, r.Description,
			).Scan(&id)
			if err != nil {
				return nil, err
			}
			logger.Info("role seeded", slog.String("code", r.Code), slog.Int64("tenant_id", tenantID))
		case err != nil:
			return nil, err
		}
		ids[r.Code] = id
	}

	return ids, nil
}

// seedPermissions inserts every rbac.PermissionCatalog() entry missing
// from the permissions table. Each genuinely new row is logged
// individually (mirroring seedRoles' own per-row "role seeded" log) —
// deliberately not just a final count — so that running this against a
// database whose permissions table has fallen behind the catalog (e.g.
// a module added to internal/rbac/catalog.go after this database was
// last seeded) produces a visible, auditable record of exactly which
// permission codes were missing and are now backfilled, rather than a
// silent no-op that leaves the gap undiagnosed until a user hits a 403
// for a permission the code already grants their role.
func seedPermissions(ctx context.Context, db *sql.DB, logger *slog.Logger) (map[string]int64, error) {
	ids := map[string]int64{}
	newCount := 0

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
			logger.Info("permission seeded", slog.String("code", p.Code))
			newCount++
		case err != nil:
			return nil, err
		}
		ids[p.Code] = id
	}

	logger.Info("permissions seeded", slog.Int("count", len(ids)), slog.Int("newly_inserted", newCount))
	return ids, nil
}

// seedRolePermissions grants every rbac.RoleDefaultGrants entry missing
// from role_permissions. Like seedPermissions above, each newly-granted
// (role, permission) pair is logged individually — the same
// drift-must-be-visible reasoning applies: a role whose catalog grants
// were extended (e.g. ADMIN gaining a newly-added module) must show
// that correction in the log, not just a blanket "done".
func seedRolePermissions(ctx context.Context, db *sql.DB, logger *slog.Logger, roleIDs, permissionIDs map[string]int64) error {
	newCount := 0

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
				logger.Info("role_permission granted", slog.String("role", roleCode), slog.String("permission", code))
				newCount++
			case err != nil:
				return err
			}
		}
	}

	logger.Info("role_permissions seeded", slog.Int("newly_granted", newCount))
	return nil
}

// sqlExecutor is the narrow slice of *sql.DB's API backfillTenantRoleCatalogs
// needs — satisfied by *sql.DB itself (seed()'s real call) and by *sql.Tx
// (TestBackfillTenantRoleCatalogs_OnlyFillsTenantsWithZeroRoles's own
// roll-back-at-the-end fixture), so the same function body is exercised
// by both without a single behavioral difference between test and
// production.
type sqlExecutor interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// backfillTenantRoleCatalogs gives every EXISTING tenant with zero rows
// in roles its own copy of rbac.RoleCatalog + rbac.RoleDefaultGrants —
// see this function's call site in seed() for why such a tenant can
// exist at all. A tenant is touched only when it has literally zero
// roles; a tenant with any roles already (including a partially custom
// catalog) is left completely alone, so this can never duplicate a row
// or disturb a role/grant a human has since edited.
func backfillTenantRoleCatalogs(ctx context.Context, db sqlExecutor, logger *slog.Logger, permissionIDs map[string]int64) error {
	rows, err := db.QueryContext(ctx, "SELECT id FROM tenants ORDER BY id")
	if err != nil {
		return err
	}
	var tenantIDs []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		tenantIDs = append(tenantIDs, id)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	rows.Close()

	backfilledTenants := 0
	for _, tenantID := range tenantIDs {
		var existingRoleCount int
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM roles WHERE tenant_id = @p1", tenantID).Scan(&existingRoleCount); err != nil {
			return err
		}
		if existingRoleCount > 0 {
			continue
		}

		tenantRoleIDs := map[string]int64{}
		for _, r := range rbac.RoleCatalog {
			var id int64
			err := db.QueryRowContext(ctx,
				`INSERT INTO roles (tenant_id, code, name, description, is_active) OUTPUT INSERTED.id VALUES (@p1, @p2, @p3, @p4, 1)`,
				tenantID, r.Code, r.Name, r.Description,
			).Scan(&id)
			if err != nil {
				return fmt.Errorf("backfill tenant %d role %s: %w", tenantID, r.Code, err)
			}
			tenantRoleIDs[r.Code] = id
			logger.Info("tenant role backfilled", slog.Int64("tenant_id", tenantID), slog.String("code", r.Code))
		}

		grantCount := 0
		for roleCode, codes := range rbac.RoleDefaultGrants {
			roleID, ok := tenantRoleIDs[roleCode]
			if !ok {
				return fmt.Errorf("backfill tenant %d: unknown role code %q", tenantID, roleCode)
			}
			for _, code := range codes {
				permissionID, ok := permissionIDs[code]
				if !ok {
					// Same tolerance seedRolePermissions itself has: the
					// global permission catalog is seeded earlier in this
					// same seed() call, so this only triggers if the
					// catalog and RoleDefaultGrants have drifted apart.
					continue
				}
				if _, err := db.ExecContext(ctx,
					"INSERT INTO role_permissions (role_id, permission_id) VALUES (@p1, @p2)",
					roleID, permissionID,
				); err != nil {
					return fmt.Errorf("backfill tenant %d role %s grant %s: %w", tenantID, roleCode, code, err)
				}
				grantCount++
			}
		}

		logger.Info("tenant role catalog backfilled",
			slog.Int64("tenant_id", tenantID), slog.Int("roles_created", len(tenantRoleIDs)), slog.Int("grants_created", grantCount))
		backfilledTenants++
	}

	logger.Info("tenant role catalog backfill complete", slog.Int("tenants_backfilled", backfilledTenants), slog.Int("tenants_scanned", len(tenantIDs)))
	return nil
}

// grantCatalogPermissionsToAllTenants is the multi-tenant counterpart to
// seedRolePermissions (which only ever grants the bootstrap/default
// tenant's roles — see seedRoles' own doc for why that scoping is
// correct for THAT function) and to backfillTenantRoleCatalogs (which
// only ever touches a tenant with literally zero roles). Every tenant
// provisioned via internal/tenant.Repository.CreateWithAudit (or
// backfillTenantRoleCatalogs above) already has its own copy of every
// rbac.RoleCatalog role — but when a NEW permission module is added to
// rbac.Modules after those tenants were created (e.g. Master Data
// Expansion's product_category/customer/supplier), nothing before this
// function ever revisits an EXISTING, already-populated tenant's role
// catalog to grant it the newly-added permissions, which is exactly the
// multi-tenant RBAC seed drift this closes.
//
// For every role row in the whole roles table, across every tenant: if
// its code matches a known rbac.RoleDefaultGrants entry, grant every
// permission in that entry missing from role_permissions. Strictly
// additive (only ever INSERTs a missing grant; never touches an
// existing one, never revokes anything a human may have since changed)
// and idempotent (a missing-row check backs every insert, so re-running
// this on every `docker compose up`, same as every other seed step, is
// always safe). A role whose code has no matching catalog entry — a
// hand-rolled custom role, e.g. this file's own test fixture
// "CUSTOM_ONLY" — is left completely untouched, the same guarantee
// backfillTenantRoleCatalogs' own test already proves for role
// *creation*; this is that same guarantee for grant *maintenance*.
func grantCatalogPermissionsToAllTenants(ctx context.Context, db sqlExecutor, logger *slog.Logger, permissionIDs map[string]int64) error {
	rows, err := db.QueryContext(ctx, "SELECT id, tenant_id, code FROM roles")
	if err != nil {
		return err
	}

	type roleRow struct {
		id, tenantID int64
		code         string
	}
	var allRoles []roleRow
	for rows.Next() {
		var r roleRow
		if err := rows.Scan(&r.id, &r.tenantID, &r.code); err != nil {
			rows.Close()
			return err
		}
		allRoles = append(allRoles, r)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	rows.Close()

	newCount := 0
	for _, r := range allRoles {
		codes, ok := rbac.RoleDefaultGrants[r.code]
		if !ok {
			continue // a hand-rolled custom role with no catalog default — leave it alone
		}

		for _, code := range codes {
			permissionID, ok := permissionIDs[code]
			if !ok {
				// Same drift tolerance seedRolePermissions/
				// backfillTenantRoleCatalogs already use: only
				// triggers if the catalog and RoleDefaultGrants have
				// themselves drifted apart, which seedPermissions
				// earlier in this same seed() call already guards.
				continue
			}

			var exists int
			err := db.QueryRowContext(ctx,
				"SELECT 1 FROM role_permissions WHERE role_id = @p1 AND permission_id = @p2",
				r.id, permissionID,
			).Scan(&exists)
			switch {
			case err == sql.ErrNoRows:
				if _, err := db.ExecContext(ctx,
					"INSERT INTO role_permissions (role_id, permission_id) VALUES (@p1, @p2)",
					r.id, permissionID,
				); err != nil {
					return fmt.Errorf("grant tenant %d role %s permission %s: %w", r.tenantID, r.code, code, err)
				}
				logger.Info("tenant role_permission granted",
					slog.Int64("tenant_id", r.tenantID), slog.String("role", r.code), slog.String("permission", code))
				newCount++
			case err != nil:
				return err
			}
		}
	}

	logger.Info("tenant role_permissions grant sweep complete", slog.Int("newly_granted", newCount))
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

// seedDefaultTenantMembership is Phase 29B's minimal bootstrap: it
// grants adminUserID an ACTIVE membership in tenantID (ensureDefaultTenant's
// resolved id) with adminRoleID, so RequireTenantContext's fail-closed
// check and auth.Service.Login's DefaultMembership resolution have
// something to succeed against in a fresh dev/CI environment. Creating
// the tenant itself moved to ensureDefaultTenant (Deployment Readiness
// fix) — this function only ever creates the membership row now.
//
// This is deliberately NOT the Phase 29 production backfill: migrating
// every pre-existing user onto a tenant_id is a database-schema-change
// concern tracked separately (docs/proposals/wms-saas/migration-plan.md
// §2, migrations 0043+) and out of this auth-focused phase's scope — only
// the one admin user this same seed command already creates is granted
// a membership here, nothing else in the users table is touched.
func seedDefaultTenantMembership(ctx context.Context, db *sql.DB, logger *slog.Logger, tenantID, adminUserID, adminRoleID int64) error {
	var exists int
	err := db.QueryRowContext(ctx,
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
