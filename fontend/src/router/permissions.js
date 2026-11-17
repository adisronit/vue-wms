import { hasPermission, isPlatformAdmin } from '../services/auth'

// Mirrors the canView* permission gates MainLayout.vue already computes
// for Sidebar's props, keyed by route name instead of by sidebar item, so
// restored tabs (REQ-05) can be revalidated the same way the sidebar
// already hides items the caller isn't authorized for. null = no
// permission required beyond being authenticated. 'PLATFORM_ADMIN' is
// its own case because that status is resolved asynchronously and cached
// separately (see services/auth.js), not a plain permission code.
//
// This is a UX-layer guard only, same as the existing requiresPlatformAdmin
// check in router/index.js — every protected API call behind these pages
// is independently re-verified server-side regardless of what this says.
export const PLATFORM_ADMIN = 'PLATFORM_ADMIN'

export const ROUTE_PERMISSIONS = {
  Dashboard: null,
  Receiving: null,
  NewReceiving: null,
  QualityCheck: null,
  NewQualityCheck: null,
  PutAway: null,
  NewPutAway: null,
  TransferReceiving: 'transfer_receipt.view',
  StockInquiry: null,
  StockAdjustment: null,
  CheckStock: null,
  Replenishment: 'replenishment.view',
  Relocation: 'relocation.view',
  NewRelocation: 'relocation.view',
  Orders: null,
  NewOrder: null,
  EditOrder: null,
  OrderReview: null,
  OrderDetail: null,
  Picking: null,
  PickingBatches: 'picking_batch.view',
  NewPickingBatch: 'picking_batch.view',
  PickingBatchDetail: 'picking_batch.view',
  Sortation: 'picking_batch.view',
  Packing: 'packing.view',
  PackingDetail: 'packing.view',
  Shipping: null,
  Loading: 'loading.view',
  LoadingTripDetail: 'loading.view',
  UserManagement: 'users.view',
  Roles: 'roles.view',
  Warehouse: 'warehouse.view',
  Locations: 'warehouse.view',
  ProductMaster: 'product.view',
  ProductCategories: 'product_category.view',
  Customers: 'customer.view',
  Suppliers: 'supplier.view',
  Brands: 'brand.view',
  ProductFamilies: 'product_family.view',
  PlatformTenants: PLATFORM_ADMIN,
  PlatformTenantDetail: PLATFORM_ADMIN,
  // Phase 34B-34I. ReportsWarehouseLayout deliberately uses layout.view,
  // not report.view — its backend requires a separate permission (see
  // internal/warehouselayout), and Sidebar.vue/MainLayout.vue gate its
  // menu item the same way via canViewWarehouseLayout.
  ReportsReceiving: 'report.view',
  ReportsStockCount: 'report.view',
  ReportsPickingIssue: 'report.view',
  ReportsInventory: 'report.view',
  ReportsPickingKpi: 'report.view',
  ReportsQc: 'report.view',
  ReportsDelivery: 'report.view',
  ReportsWarehouseLayout: 'layout.view'
}

// isRouteAllowed fails closed: an unrecognized route name (e.g. a route
// removed/renamed since a tab was persisted to sessionStorage), or any
// error thrown while evaluating the permission itself, is treated as not
// allowed rather than silently permitted or crashing the caller (this
// runs at router-restore time, during module load — an exception here
// must never take the whole app down over one stale persisted tab).
// permissionRequirementFor exposes the raw requirement for a route name
// (null = no permission beyond auth, a code string, PLATFORM_ADMIN, or
// undefined for an unrecognized route) — used by tabs.js restore() to
// tell "not allowed" apart from "can't tell yet" for the one requirement
// (PLATFORM_ADMIN) that depends on an async check.
export function permissionRequirementFor(routeName) {
  return Object.prototype.hasOwnProperty.call(ROUTE_PERMISSIONS, routeName)
    ? ROUTE_PERMISSIONS[routeName]
    : undefined
}

export function isRouteAllowed(routeName) {
  try {
    if (!Object.prototype.hasOwnProperty.call(ROUTE_PERMISSIONS, routeName)) return false
    const perm = ROUTE_PERMISSIONS[routeName]
    if (perm === null) return true
    if (perm === PLATFORM_ADMIN) return isPlatformAdmin()
    return hasPermission(perm)
  } catch {
    return false
  }
}
