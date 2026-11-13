// Package rbac is the single source of truth for the permission catalog,
// role catalog, and each role's default permission grants. It exists so
// the seed command and the role/permission validation logic never
// maintain two copies of the same list.
//
// The catalog mirrors fontend/src/views/administration/RolePermissionView.vue's
// hardcoded permissionModules/actions/roles exactly, with two renames:
// USER_MANAGEMENT -> "users" and ROLE_PERMISSION -> "roles", so the
// enforced codes match CLAUDE.md's own examples (users.view, roles.update,
// ...) and the REST resources they gate (/api/v1/users, /api/v1/roles).
// "Edit" is renamed to "update" for the same reason. 6 of the remaining
// original modules keep their frontend codes lowercased; they are
// catalog-only (no handler enforces them yet) since Inventory/Inbound/
// Outbound/Orders/Picking business logic is still out of scope.
// "warehouse" (Phase 3) and "product" (Phase 4) are real, enforced
// modules that did not exist in the frontend's original hardcoded
// permissionModules at all — neither Warehouse Setup nor Product Master
// had a permission mapping in Phase 1/2; each was added in the phase
// that actually implemented and protected it.
package rbac

// PermissionDef describes one row of the permission catalog.
type PermissionDef struct {
	Code        string
	Module      string
	Action      string
	Description string
}

// ModuleDef groups a module's allowed actions, mirroring the frontend's
// permissionModules[].actions restriction (not every module supports
// every action).
type ModuleDef struct {
	Code    string
	Name    string
	Actions []string
}

// Modules is the catalog of modules and the actions valid for each,
// matching RolePermissionView.vue's permissionModules exactly (renames
// noted in the package doc).
var Modules = []ModuleDef{
	{Code: "dashboard", Name: "Dashboard", Actions: []string{"view", "export"}},
	{Code: "receiving", Name: "Receiving", Actions: []string{"view", "create", "update", "delete", "approve"}},
	{Code: "quality_check", Name: "Quality Check", Actions: []string{"view", "create", "update", "approve"}},
	{Code: "putaway", Name: "Putaway", Actions: []string{"view", "create", "update", "approve"}},
	{Code: "stock_inquiry", Name: "Stock Inquiry", Actions: []string{"view", "export"}},
	{Code: "stock_adjustment", Name: "Stock Adjustment", Actions: []string{"view", "create", "update", "delete", "approve"}},
	{Code: "picking", Name: "Picking", Actions: []string{"view", "create", "update", "approve"}},
	{Code: "replenishment", Name: "Replenishment", Actions: []string{"view", "create", "update", "approve"}},
	{Code: "shipping", Name: "Shipping", Actions: []string{"view", "create", "update", "approve"}},
	{Code: "order", Name: "Order Management", Actions: []string{"view", "create", "update", "approve"}},
	{Code: "users", Name: "User Management", Actions: []string{"view", "create", "update", "delete"}},
	{Code: "roles", Name: "Roles & Permissions", Actions: []string{"view", "create", "update", "delete", "approve"}},
	{Code: "warehouse", Name: "Warehouse Setup", Actions: []string{"view", "create", "update", "delete"}},
	{Code: "product", Name: "Product Master", Actions: []string{"view", "create", "update", "delete"}},
	// tenant_users (WMS-29E) gates the Tenant Admin membership-management
	// API (/api/v1/admin/tenant-users/*) — deliberately separate from
	// "users" above, which gates the legacy, not-yet-tenant-scoped global
	// Users Management screen (/api/v1/users). "suspend" covers both
	// directions of PATCH .../status (suspend and reactivate are the same
	// request shape, just a different target status), the same one-
	// permission-covers-the-whole-transition shape the Platform Admin
	// tenant status API already uses. There is no "delete" action: a
	// membership is only ever status-changed (ACTIVE/DISABLED), never
	// deleted.
	{Code: "tenant_users", Name: "Tenant User Management", Actions: []string{"view", "create", "update", "suspend"}},
	// Master Data Expansion: product_category/customer/supplier are
	// admin-level master data, the same "view/create/update/delete"
	// shape warehouse/product already use — not given to the
	// operational staff roles (RECEIVING/QC/PUTAWAY/PICKING/SHIPPING)
	// below, matching warehouse's own precedent of restricting Setup
	// screens to ADMIN/WAREHOUSE_MANAGER only.
	{Code: "product_category", Name: "Product Categories", Actions: []string{"view", "create", "update", "delete"}},
	{Code: "customer", Name: "Customers", Actions: []string{"view", "create", "update", "delete"}},
	{Code: "supplier", Name: "Suppliers", Actions: []string{"view", "create", "update", "delete"}},
}

var actionLabels = map[string]string{
	"view":    "View",
	"create":  "Create",
	"update":  "Update",
	"delete":  "Delete",
	"approve": "Approve",
	"export":  "Export",
}

// PermissionCatalog expands Modules into the full flat list of
// permissions (55 entries as of Phase 25's "order" module —
// len(Modules[i].Actions) summed across Modules; this count drifts as
// modules are added, so don't trust a stale copy of this comment over
// actually calling PermissionCatalog()) that gets seeded into the
// permissions table.
func PermissionCatalog() []PermissionDef {
	var defs []PermissionDef
	for _, m := range Modules {
		for _, a := range m.Actions {
			defs = append(defs, PermissionDef{
				Code:        m.Code + "." + a,
				Module:      m.Code,
				Action:      a,
				Description: actionLabels[a] + " " + m.Name,
			})
		}
	}
	return defs
}

// RoleDef describes one row of the role catalog.
type RoleDef struct {
	Code        string
	Name        string
	Description string
}

// RoleCatalog is the canonical 7-role set taken from the User Management
// form's role dropdown (the actual assignable set), not the narrower
// 4-role list the Roles & Permissions screen happened to hardcode.
var RoleCatalog = []RoleDef{
	{Code: "ADMIN", Name: "Administrator", Description: "Full access to all modules and system configuration"},
	{Code: "WAREHOUSE_MANAGER", Name: "Warehouse Manager", Description: "Manage warehouse operations and approval tasks"},
	{Code: "RECEIVING", Name: "Receiving Staff", Description: "Receive goods and update inbound transactions"},
	{Code: "QC", Name: "Quality Check Staff", Description: "Performs incoming quality inspection tasks"},
	{Code: "PUTAWAY", Name: "Putaway Staff", Description: "Places received goods into storage locations"},
	{Code: "PICKING", Name: "Picking Staff", Description: "Execute outbound picking tasks"},
	{Code: "SHIPPING", Name: "Shipping Staff", Description: "Prepares and confirms outbound shipments"},
}

// RoleDefaultGrants maps role code to the permission codes it is seeded
// with. ADMIN and WAREHOUSE_MANAGER are computed generically (everything,
// and everything-except-delete, respectively) so adding the "warehouse"
// module above automatically extends both without an edit here.
// RECEIVING and PICKING mirror RolePermissionView.vue's
// createDefaultPermissions() exactly, as originally observed.
//
// QC, PUTAWAY, and SHIPPING originally had no branch in that frontend
// function at all (Phase 2 preserved that as an empty grant list rather
// than inventing defaults). Phase 3 resolves the gap: each is now given
// the same shape RECEIVING and PICKING already had — dashboard.view,
// view/create/update on its own module, and stock_inquiry.view — so all
// three become usable by real staff. Not given: delete or approve on
// their own module (matches RECEIVING/PICKING's existing conservative
// baseline), and no warehouse.* access (Warehouse Setup is an
// administration screen; only ADMIN/WAREHOUSE_MANAGER get it).
var RoleDefaultGrants = map[string][]string{
	"ADMIN":             allPermissionCodes(),
	"WAREHOUSE_MANAGER": allExcept("delete"),
	"RECEIVING": {
		"dashboard.view",
		"receiving.view", "receiving.create", "receiving.update",
		"stock_inquiry.view",
	},
	"QC": {
		"dashboard.view",
		"quality_check.view", "quality_check.create", "quality_check.update",
		"stock_inquiry.view",
	},
	"PUTAWAY": {
		"dashboard.view",
		"putaway.view", "putaway.create", "putaway.update",
		"replenishment.view", "replenishment.create", "replenishment.update",
		"stock_inquiry.view",
	},
	"PICKING": {
		"dashboard.view",
		"picking.view", "picking.update",
		"stock_inquiry.view",
	},
	"SHIPPING": {
		"dashboard.view",
		"shipping.view", "shipping.create", "shipping.update",
		"stock_inquiry.view",
	},
}

func allPermissionCodes() []string {
	var codes []string
	for _, p := range PermissionCatalog() {
		codes = append(codes, p.Code)
	}
	return codes
}

// allExcept returns every catalog code whose action is not excludedAction,
// matching the frontend's WAREHOUSE_MANAGER default (every module/action
// except Delete).
func allExcept(excludedAction string) []string {
	var codes []string
	for _, p := range PermissionCatalog() {
		if p.Action == excludedAction {
			continue
		}
		codes = append(codes, p.Code)
	}
	return codes
}
