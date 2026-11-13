// Package tenantaudit mechanically enforces the convention
// tenant-isolation-security.md §4 proposes and leaves as an "Unresolved
// Design Choice" (§11) whether to automate: "no exported Repository
// method on a tenant-owned model may omit a tenantID parameter." It
// resolves that open question by implementing the AST-based check
// directly, rather than leaving it to code review alone — the same
// design doc's own rationale for why a convention needs a mechanical
// gate: "a global [GORM] scope can silently be bypassed... the explicit
// parameter is what the... security test suite asserts against."
//
// This package does the audit; backend/test/isolation's
// TestNoRepositoryMethodOmitsTenantID is the test that runs it and
// reports the result. Nothing here is wired into cmd/server, cmd/
// worker, or cmd/migrate — like internal/platform/testsupport, it
// exists purely to support `go test`.
package tenantaudit

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"
	"sort"
)

// Violation is one exported Repository method that touches the database
// (its first parameter is context.Context, the existing convention
// every repository method in this codebase already follows) with no
// parameter whose name mentions "tenant" anywhere in its signature.
type Violation struct {
	Package  string // e.g. "warehouse"
	File     string // path relative to the backend module root
	Line     int
	Receiver string // e.g. "*Repository"
	Func     string // e.g. "FindByID"
}

func (v Violation) String() string {
	return fmt.Sprintf("%s:%d: %s.%s (receiver %s) has no tenant-scoping parameter", v.File, v.Line, v.Package, v.Func, v.Receiver)
}

// exemptMethods lists the specific Package.Func pairs that are
// correctly exempt because the data they touch is Platform-Global, not
// Tenant-Owned, per phase27-audit-report.md's own ownership column —
// not a growing escape hatch for convenience. Each entry must cite the
// audit finding that justifies it.
var exemptMethods = map[string]bool{
	// phase27-audit-report.md §2.1, role module: "Permission =
	// Platform-Global (catalog)" — the permissions catalog is shared
	// identically across every tenant; only role/role_permissions
	// (already covered by role.Repository's other methods) are
	// Tenant-Owned.
	"role.ListPermissions":     true,
	"role.PermissionsByCodes":  true,
	"role.RolePermissionCodes": true, // reads the Platform-Global permissions table, joined only through an already-tenant-checked role_id upstream per the audit's own recommendation

	// WMS-29E: users is Platform-Global identity by design (one `users`
	// row can hold memberships in many tenants, tenant-er-design.md §1) —
	// there is no tenant_id on this table to thread through, and
	// shouldn't be; the tenant-owned side of "link an existing user" is
	// the tenant_users row created afterward, which does go through a
	// tenant-scoped repository method (tenant.Repository's
	// CreateMembershipWithExistingUser).
	"user.FindByUsernameOrEmail": true,

	// Location Master Management: these two are NOT Platform-Global
	// data (locations are Tenant-Owned) — a different, narrower
	// justification than every entry above. Both are deliberate
	// backward-compatibility exceptions, not unaddressed gaps:
	// location.FindByIDUnscoped exists only so internal/receiving/
	// internal/stockadjustment/internal/stockcount's own narrow
	// locationChecker interfaces (none of which have tenant context
	// themselves — out of this phase's scope) keep compiling against
	// *Repository unchanged (see its own doc comment); every other
	// caller uses the real, tenant-scoped FindByID. location.CountByZone
	// exists solely for internal/zone.Service's own (also
	// tenant-oblivious today) child-count check before deleting a zone —
	// see that method's own doc comment.
	"location.FindByIDUnscoped": true,
	"location.CountByZone":      true,
}

var tenantNamePattern = regexp.MustCompile(`(?i)tenant`)

// AuditRepositories parses every internal/<module>/repository.go file
// under backendRoot and returns one Violation per exported, DB-touching
// Repository method whose parameter list carries no tenant-scoping
// parameter. The returned slice is sorted by file then line, so output
// is stable across runs.
func AuditRepositories(backendRoot string) ([]Violation, error) {
	paths, err := filepath.Glob(filepath.Join(backendRoot, "internal", "*", "repository.go"))
	if err != nil {
		return nil, err
	}

	var violations []Violation
	fset := token.NewFileSet()

	for _, path := range paths {
		pkgName := filepath.Base(filepath.Dir(path))

		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}

		relPath, err := filepath.Rel(backendRoot, path)
		if err != nil {
			relPath = path
		}

		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || len(fn.Recv.List) != 1 {
				continue
			}
			if !fn.Name.IsExported() {
				continue
			}

			receiver := exprString(fn.Recv.List[0].Type)
			if !isRepositoryReceiver(receiver) {
				continue
			}

			if !firstParamIsContext(fn.Type) {
				// Not a DB-touching call in this codebase's own
				// convention (every repository method takes ctx
				// context.Context first) — skip rather than flag.
				continue
			}

			if exemptMethods[pkgName+"."+fn.Name.Name] {
				continue
			}

			if hasTenantParam(fn.Type) {
				continue
			}

			pos := fset.Position(fn.Pos())
			violations = append(violations, Violation{
				Package:  pkgName,
				File:     relPath,
				Line:     pos.Line,
				Receiver: receiver,
				Func:     fn.Name.Name,
			})
		}
	}

	sort.Slice(violations, func(i, j int) bool {
		if violations[i].File != violations[j].File {
			return violations[i].File < violations[j].File
		}
		return violations[i].Line < violations[j].Line
	})

	return violations, nil
}

// isRepositoryReceiver matches "*Repository" and "Repository" — every
// module in this codebase names its single repository type exactly
// Repository (see internal/warehouse/repository.go et al.), so this
// does not need to match arbitrary type names.
func isRepositoryReceiver(receiver string) bool {
	return receiver == "Repository" || receiver == "*Repository"
}

func firstParamIsContext(ft *ast.FuncType) bool {
	if ft.Params == nil || len(ft.Params.List) == 0 {
		return false
	}
	return exprString(ft.Params.List[0].Type) == "context.Context"
}

func hasTenantParam(ft *ast.FuncType) bool {
	if ft.Params == nil {
		return false
	}
	for _, field := range ft.Params.List {
		for _, name := range field.Names {
			if tenantNamePattern.MatchString(name.Name) {
				return true
			}
		}
	}
	return false
}

// exprString renders a type expression back to source text for simple
// cases (identifiers, pointers, selectors) — enough to recognize
// "*Repository" and "context.Context" without pulling in go/printer for
// a one-line job.
func exprString(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.StarExpr:
		return "*" + exprString(e.X)
	case *ast.SelectorExpr:
		return exprString(e.X) + "." + e.Sel.Name
	default:
		return fmt.Sprintf("%T", expr)
	}
}
