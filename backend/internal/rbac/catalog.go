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
	// "confirm" (Phase 31A) is split out of "approve" so ordinary
	// Receiving staff can confirm their own receipts (the stock-moving
	// action) without also being granted the separate, supervisor-level
	// "approve" permission — which remains reserved for a future
	// exception-handling action (over-receipt approval, etc.) and for
	// quality_check.approve's own QC release, neither built in this
	// phase. See RoleDefaultGrants' own doc.
	{Code: "receiving", Name: "Receiving", Actions: []string{"view", "create", "update", "delete", "confirm", "approve"}},
	// "confirm" (Phase 31B) gates recording PASS/HOLD/REJECT dispositions
	// — the stock-mutating action — split out of "approve" the same way
	// receiving.confirm was, so QC staff can record their own inspection
	// decisions. quality_check.approve stays reserved for a future
	// supervisor-level exception action; none exists yet.
	{Code: "quality_check", Name: "Quality Check", Actions: []string{"view", "create", "update", "confirm", "approve"}},
	// "confirm" (Phase 31C) gates the stock-moving action, split out of
	// "approve" the same way receiving.confirm/quality_check.confirm
	// were in 31A/31B, so PUTAWAY staff can confirm their own tasks.
	{Code: "putaway", Name: "Putaway", Actions: []string{"view", "create", "update", "confirm", "approve"}},
	// Phase 32 — Stock Relocation: "approve" gates Draft -> Ready
	// (locking a task's lines for execution); "confirm" gates the
	// stock-moving action, the same confirm/approve split receiving.
	// confirm/quality_check.confirm/putaway.confirm already established.
	// No default grant for any operational staff role yet (matches how
	// each new module is introduced conservatively) — only ADMIN (every
	// code, computed) and WAREHOUSE_MANAGER (every non-delete code,
	// computed) get it automatically; a future phase can extend e.g.
	// PUTAWAY's RoleDefaultGrants if relocation duties are assigned to
	// existing staff roles.
	{Code: "relocation", Name: "Stock Relocation", Actions: []string{"view", "create", "update", "confirm", "approve"}},
	// Phase 33C — Packing: "update" gates the packer's own routine
	// execution actions (AssignPacker, CreateCarton/UpdateCarton,
	// PackItem, FlagException); "confirm" gates the distinct, elevated
	// second-person checker-verification action (CheckerConfirm, which
	// marks a job COMPLETED) — a deliberately different split than
	// receiving.confirm/quality_check.confirm/putaway.confirm/
	// relocation.confirm/picking.confirm/order.confirm, which each gate
	// their own module's primary stock-mutating action; Packing makes no
	// inventory call at all, so "confirm" here instead marks the
	// two-person-verification boundary specifically. "approve" gates
	// Cancel, the same more-consequential/planning-level authority level
	// picking.approve's own Cancel gate uses. No default grant for any
	// operational staff role yet (matches relocation's own precedent) —
	// only ADMIN (every code) and WAREHOUSE_MANAGER (every non-delete
	// code) get it automatically.
	{Code: "packing", Name: "Packing", Actions: []string{"view", "create", "update", "confirm", "approve"}},
	{Code: "stock_inquiry", Name: "Stock Inquiry", Actions: []string{"view", "export"}},
	{Code: "stock_adjustment", Name: "Stock Adjustment", Actions: []string{"view", "create", "update", "delete", "approve"}},
	// "confirm" (Phase 33B) gates the stock-mutating/stock-adjacent
	// actions — Confirm (PICKED/SHORT) and Repick (re-reserving a
	// previously-short line) — split out of "approve" the same way
	// receiving.confirm/quality_check.confirm/putaway.confirm/
	// relocation.confirm/order.confirm already were. Before this split,
	// Confirm was gated on picking.update (see Handler.Register's own
	// doc for why); "update" now covers only non-stock-mutating actions
	// (operator assignment, scan validation). "approve" stays on Cancel,
	// its pre-existing authority level.
	{Code: "picking", Name: "Picking", Actions: []string{"view", "create", "update", "confirm", "approve"}},
	// Phase 33C.1 — Batch Picking / Wave Management: kept as its own
	// module rather than folded into "picking" above — a batch is its
	// own header/workflow object (consolidation preview, atomic
	// multi-order claim) distinct from executing a single already-
	// created task, the same reasoning relocation/packing each got
	// their own module instead of folding into an existing one.
	// "create" gates the DRAFT consolidation step (read-only against
	// inventory); "confirm" gates the real commit (claims every
	// included order, creates one real picking_tasks row via
	// internal/picking's own unmodified Service.CreateTx); "cancel"
	// gates Cancel (DRAFT only — a CONFIRMED batch's own task is
	// cancelled via picking.approve's existing Cancel instead). No
	// default grant for any operational staff role yet (matches
	// relocation/packing's own precedent) — only ADMIN (every code) and
	// WAREHOUSE_MANAGER (every non-delete code) get it automatically; a
	// future phase could extend PICKING's own RoleDefaultGrants if
	// batch-picking duties are assigned to existing picking staff.
	//
	// Phase 33C.2 adds "sortation" to this same module rather than a new
	// catalog module — Sortation is a continuation of one batch's own
	// lifecycle (same picking_batches header, same picking_batch_
	// allocations ledger extended with sorted_qty/shortage_qty), not a
	// separate workflow object. It gates the two mutating sortation
	// actions (POST .../sortation/scan and POST .../sortation/exception)
	// at an authority level distinct from "confirm" (which only ever
	// claims orders + creates the task) and "cancel" — the worklist read
	// (GET .../sortation) stays on the existing "view" code, matching
	// every other module's own "view gates every read" convention.
	{Code: "picking_batch", Name: "Batch Picking", Actions: []string{"view", "create", "confirm", "cancel", "sortation"}},
	{Code: "replenishment", Name: "Replenishment", Actions: []string{"view", "create", "update", "approve"}},
	// "confirm" (Phase 33D.1) gates Confirm (the Dispatch/Deliver/Issue
	// status transitions — the significant, caller-visible action this
	// module's whole workflow exists for) — split out of "approve" the
	// same way receiving.confirm/quality_check.confirm/putaway.confirm/
	// relocation.confirm/picking.confirm/order.confirm/packing.confirm
	// were each split before it. Before this split, Confirm was gated on
	// "approve" directly (see Handler.Register's own doc for the
	// historical reasoning); "approve" remains reserved, unused by any
	// handler in this module, matching every other confirm/approve
	// split's own "approve stays reserved for a future supervisor-level
	// exception action" precedent. See RoleDefaultGrants' own doc for why
	// SHIPPING now gets shipping.confirm by default.
	{Code: "shipping", Name: "Shipping", Actions: []string{"view", "create", "update", "confirm", "approve"}},
	// Phase 33D.2 — Loading: the "load" half of the dispatch
	// re-sequencing gate (33D.1 already added the "pack" half). Kept as
	// its own module rather than folded into "shipping" — a loading
	// trip is its own header/workflow object (vehicle/driver/dock,
	// multi-shipment/multi-drop, its own scan-to-load worklist) distinct
	// from a shipment's own status machine, the same reasoning
	// relocation/packing/picking_batch each got their own module
	// instead of folding into an existing one. "update" gates the
	// ordinary loading-dock execution actions (AssignShipment/
	// ScanCarton/ShortCarton); "confirm" gates ConfirmTrip — the action
	// that transitions the trip to COMPLETED and calls shipping.
	// Service.MarkLoaded for every shipment on it, the real dispatch-
	// gate payoff, at the same elevated authority level receiving.
	// confirm/picking.confirm/packing.confirm/shipping.confirm already
	// established for their own module's primary significant action;
	// "approve" gates CancelTrip, matching picking.approve/packing.
	// approve's own Cancel gate. No default grant for any operational
	// staff role yet (matches relocation/packing/picking_batch's own
	// precedent) — only ADMIN (every code) and WAREHOUSE_MANAGER (every
	// non-delete code) get it automatically; a future phase could
	// extend SHIPPING's own RoleDefaultGrants if loading-dock duties are
	// assigned to existing shipping staff.
	{Code: "loading", Name: "Loading & Dispatch Verification", Actions: []string{"view", "create", "update", "confirm", "approve"}},
	// "confirm" (Phase 33A) gates the new Review & Release action
	// (Service.Release: PENDING -> RELEASED, the availability-check +
	// controlled-shortage-approval gate in front of the existing
	// CreatePickingTask orchestration) — split out of "approve" the same
	// way receiving.confirm/quality_check.confirm/putaway.confirm/
	// relocation.confirm already were. "approve" stays on Cancel, its
	// pre-existing authority level.
	{Code: "order", Name: "Order Management", Actions: []string{"view", "create", "update", "confirm", "approve"}},
	// Phase 33E.2 — Destination Receiving: a new, lightweight module
	// (internal/transferreceipt) that closes the inter-warehouse
	// transfer loop 33E.1 proved — crediting the DESTINATION warehouse
	// via inventory.AddStockTx once a dispatched transfer shipment
	// physically arrives. Kept as its own module rather than folded into
	// "receiving" above — that module is deeply supplier-centric
	// (vendor, PO reference, QC/hold integration), the same reasoning
	// relocation/packing/picking_batch/loading each got their own module
	// instead of distorting an existing one. No "update": the header has
	// nothing to edit after creation (every field is either copied from
	// the source shipment/order or stamped by a mutating action) and no
	// "delete". "create" gates creating a receipt from a DISPATCHED (or
	// later) transfer shipment; "confirm" gates BOTH of this module's
	// stock/status-mutating actions — ReceiveItem (the AddStockTx-calling
	// action, called incrementally) and Confirm (DRAFT -> COMPLETED,
	// once every line is fully received or has a recorded discrepancy) —
	// the same elevated authority level receiving.confirm/putaway.
	// confirm/relocation.confirm/packing.confirm already established for
	// their own module's primary stock-mutating action; "approve" gates
	// Cancel (DRAFT only), matching loading.approve/picking_batch's own
	// "cancel"-at-elevated-authority precedent rather than relocation's
	// own "update gates Cancel" choice. No default grant for any
	// operational staff role yet (matches every other new module's own
	// precedent) — only ADMIN (every code) and WAREHOUSE_MANAGER (every
	// non-delete code) get it automatically.
	{Code: "transfer_receipt", Name: "Destination Receiving", Actions: []string{"view", "create", "confirm", "approve"}},
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
	// EWTA Product Master enhancement: Brand and Product Family are new
	// admin-level master data, the same view/create/update/delete shape
	// and ADMIN/WAREHOUSE_MANAGER-only default grants as
	// product_category/customer/supplier above. Product image upload/
	// replace/remove deliberately reuses product.update/product.view
	// rather than minting its own codes — see internal/product/handler.go's
	// Register doc.
	{Code: "brand", Name: "Brands", Actions: []string{"view", "create", "update", "delete"}},
	{Code: "product_family", Name: "Product Families", Actions: []string{"view", "create", "update", "delete"}},
	// Phase 34A — Reports & Analytics: one module gates the whole Reports
	// menu (view) and every report's PDF/Excel/CSV export action
	// (export), the same coarse, module-level granularity stock_inquiry
	// already uses, rather than minting a separate code per report family
	// (receiving report, QC report, ...) — there is no per-report access
	// control requirement yet, and splitting now would be a speculative
	// abstraction with no caller. No default grant for any operational
	// staff role yet (matches relocation/packing/loading/transfer_receipt's
	// own precedent) — only ADMIN (every code) and WAREHOUSE_MANAGER
	// (every non-delete code) get it automatically.
	{Code: "report", Name: "Reports & Analytics", Actions: []string{"view", "export"}},
	// "layout" (Phase 34I) gates the Interactive 2D Warehouse Layout
	// editor — deliberately its own module, separate from both
	// "report" (viewing KPI dashboards implies nothing about authoring
	// physical layout data) and "warehouse" (managing warehouse/zone/
	// location master data implies nothing about who may reposition
	// their visual representation). Only two real actions exist:
	// "view" the current/historical layout, and "update" (save a new
	// version, upload a calibration background image) — named "update"
	// rather than the more natural-sounding "edit" to match every other
	// module's own convention (RolePermissionView.vue's Roles &
	// Permissions matrix hardcodes its "Edit" column to the action code
	// "update" for every module uniformly; a per-module override would
	// be far more invasive than just following the existing convention
	// here). There is no meaningful create/update/delete distinction
	// the way a list-of-records module has, since a layout is a single
	// mutable resource per warehouse, always upserted via one Save
	// endpoint (see internal/warehouselayout's own package doc). No
	// default grant for any operational staff role yet (matches report/
	// relocation/packing's own precedent) — only ADMIN (every code) and
	// WAREHOUSE_MANAGER (every non-delete code, which for this module
	// is both codes) get it automatically.
	{Code: "layout", Name: "Warehouse Layout", Actions: []string{"view", "update"}},
}

var actionLabels = map[string]string{
	"view":    "View",
	"create":  "Create",
	"update":  "Update",
	"delete":  "Delete",
	"confirm": "Confirm",
	"approve": "Approve",
	"export":  "Export",
	"cancel":  "Cancel",
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
		"receiving.view", "receiving.create", "receiving.update", "receiving.confirm",
		"stock_inquiry.view",
	},
	"QC": {
		"dashboard.view",
		"quality_check.view", "quality_check.create", "quality_check.update", "quality_check.confirm",
		"stock_inquiry.view",
	},
	"PUTAWAY": {
		"dashboard.view",
		"putaway.view", "putaway.create", "putaway.update", "putaway.confirm",
		"replenishment.view", "replenishment.create", "replenishment.update",
		"stock_inquiry.view",
	},
	"PICKING": {
		"dashboard.view",
		// picking.confirm (Phase 33B) added alongside the pre-existing
		// picking.update so PICKING staff keep the ability to confirm/
		// short-pick/re-pick lines after Confirm/Repick moved off
		// picking.update onto this new split code — see catalog's own
		// doc on the "picking" module above.
		"picking.view", "picking.update", "picking.confirm",
		"stock_inquiry.view",
	},
	"SHIPPING": {
		"dashboard.view",
		// shipping.confirm (Phase 33D.1) added alongside the pre-existing
		// shipping.update so SHIPPING staff keep the ability to confirm
		// dispatch/delivery/issue after Confirm moved off shipping.approve
		// onto this new split code — see catalog's own doc on the
		// "shipping" module above, the same shape receiving.confirm/
		// putaway.confirm/picking.confirm already established for their
		// own operational staff roles.
		"shipping.view", "shipping.create", "shipping.update", "shipping.confirm",
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
