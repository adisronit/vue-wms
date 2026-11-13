package tenantaudit

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// backendRoot resolves to backend/ regardless of the working directory
// `go test` happens to run from (internal/platform/tenantaudit is three
// levels below it).
func backendRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not resolve caller for backendRoot")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", ".."))
}

// TestAuditDetectsSyntheticViolationAndExemption is a sanity check on
// the checker itself, independent of the real codebase's current state
// — a synthetic repository.go with one clearly-violating method and one
// clearly-compliant method, so a future change to the matching logic in
// tenantaudit.go has something to regress against beyond "did it find
// violations in the real tree."
func TestAuditDetectsSyntheticViolationAndExemption(t *testing.T) {
	dir := t.TempDir()
	modDir := filepath.Join(dir, "internal", "widget")
	if err := os.MkdirAll(modDir, 0o755); err != nil {
		t.Fatal(err)
	}

	const src = `package widget

import "context"

type Repository struct{}

// FindByID has no tenant parameter — must be flagged.
func (r *Repository) FindByID(ctx context.Context, id int64) (*Widget, error) {
	return nil, nil
}

// FindByIDScoped does have one — must not be flagged.
func (r *Repository) FindByIDScoped(ctx context.Context, tenantID, id int64) (*Widget, error) {
	return nil, nil
}

// notExported is skipped regardless of signature.
func (r *Repository) notExported(ctx context.Context, id int64) (*Widget, error) {
	return nil, nil
}

// helper takes no context — not a DB call by this codebase's
// convention, so it's skipped rather than flagged.
func (r *Repository) ComputeSomething(id int64) int64 {
	return id
}
`
	if err := os.WriteFile(filepath.Join(modDir, "repository.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	violations, err := AuditRepositories(dir)
	if err != nil {
		t.Fatalf("AuditRepositories: %v", err)
	}

	if len(violations) != 1 {
		t.Fatalf("got %d violations, want exactly 1: %v", len(violations), violations)
	}
	if violations[0].Func != "FindByID" {
		t.Errorf("got violation for %s, want FindByID", violations[0].Func)
	}
}

// TestNoRepositoryMethodOmitsTenantID is the real audit, run against
// this checkout's actual backend/internal tree. It is EXPECTED TO FAIL
// today — the schema has no tenant_id column on any business table yet
// (see docs/proposals/wms-saas/migration-plan.md, migrations 0043-0046
// not yet written) and no repository method anywhere takes a tenantID
// parameter, so every one of the 17 repository.go files' exported,
// DB-touching methods is a violation by construction, matching
// phase27-audit-report.md's cross-cutting finding (a): "roughly 60+
// distinct repository/service call sites follow the identical shape...
// the default, universal data-access pattern."
//
// This failure is this suite's intended, documented state until Phase
// 29 (and 30-32 for the modules outside this phase's own scope) thread
// a mandatory tenantID parameter through every one of these methods per
// tenant-isolation-security.md §4. Once that lands, re-running this
// exact test with no changes to tenantaudit.go is the regression gate:
// it goes green, and a future PR that adds a new bare-ID repository
// method turns it red again before merge, not after a security review
// catches it later.
func TestNoRepositoryMethodOmitsTenantID(t *testing.T) {
	root := backendRoot(t)

	violations, err := AuditRepositories(root)
	if err != nil {
		t.Fatalf("AuditRepositories(%s): %v", root, err)
	}

	if len(violations) == 0 {
		t.Fatal("no violations found — if Phase 29-32 tenant-scoping work has landed, great: this test should now be deleted or repointed, not left vacuously passing")
	}

	t.Logf("tenant-scoping audit: %d exported repository method(s) across %d files have no tenant-scoping parameter (expected pre-Phase-29 — see phase27-audit-report.md)",
		len(violations), countFiles(violations))
	for _, v := range violations {
		t.Log(v.String())
	}

	t.Errorf("EXPECTED FAILURE (Phase 29 pending): %d repository method(s) have no tenant-scoping parameter — see the full list logged above, and docs/proposals/wms-saas/phase27-audit-report.md for the per-method risk rating", len(violations))
}

func countFiles(violations []Violation) int {
	seen := map[string]bool{}
	for _, v := range violations {
		seen[v.File] = true
	}
	return len(seen)
}
