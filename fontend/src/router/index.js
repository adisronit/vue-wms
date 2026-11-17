import { createRouter, createWebHistory } from 'vue-router'
import { isAuthenticated, checkPlatformAdminStatus } from '../services/auth'
import { setUnauthorizedHandler } from '../services/api'
import { canOpen, openTab, clearAllTabs, restore as restoreTabs } from '../services/tabs'
import { pruneTabComponentCache } from '../services/tabComponentCache'
import { clearAllDirtyChecks } from '../services/dirtyRegistry'
import { showToast } from '../services/toast'
import i18n from '../i18n'
import LoginView from '../views/auth/LoginView.vue'
import MainLayout from '../layouts/MainLayout.vue'
import DashboardView from '../views/dashboard/DashboardView.vue'
import ReceiveView from '../views/inbound/ReceiveView.vue'
import QualityCheckView from '../views/inbound/QualityCheckView.vue'
import PutawayView from '../views/inbound/PutawayView.vue'
import TransferReceivingView from '../views/inbound/TransferReceivingView.vue'
import StockInquiryView from '../views/inventory/StockInquiryView.vue'
import StockAdjustmentView from '../views/inventory/StockAdjustmentView.vue'
import OrderView from '../views/outbound/OrderView.vue'
import OrderFormView from '../views/outbound/OrderFormView.vue'
import OrderDetailView from '../views/outbound/OrderDetailView.vue'
import OrderReviewView from '../views/outbound/OrderReviewView.vue'
import PickingView from '../views/outbound/PickingView.vue'
import PickingBatchView from '../views/outbound/PickingBatchView.vue'
import NewPickingBatchView from '../views/outbound/NewPickingBatchView.vue'
import PickingBatchDetailView from '../views/outbound/PickingBatchDetailView.vue'
import SortationView from '../views/outbound/SortationView.vue'
import PackingView from '../views/outbound/PackingView.vue'
import PackingDetailView from '../views/outbound/PackingDetailView.vue'
import ShippingView from '../views/outbound/ShippingView.vue'
import LoadingView from '../views/outbound/LoadingView.vue'
import LoadingTripDetailView from '../views/outbound/LoadingTripDetailView.vue'
import UserManagementView from '../views/administration/UserManagementView.vue'
import RolePermissionView from '../views/administration/RolePermissionView.vue'
import WarehouseView from '../views/administration/WarehouseView.vue'
import LocationView from '../views/administration/LocationView.vue'
import NewReceivingView from '../views/inbound/NewReceivingView.vue'
import NewQualityCheckView from '../views/inbound/NewQualityCheckView.vue'
import NewPutawayView from '../views/inbound/NewPutawayView.vue'
import CheckStockView from '../views/inventory/CheckStockView.vue'
import ProductMasterView from '../views/administration/ProductMasterView.vue'
import ProductCategoryView from '../views/administration/ProductCategoryView.vue'
import CustomerView from '../views/administration/CustomerView.vue'
import SupplierView from '../views/administration/SupplierView.vue'
import BrandView from '../views/administration/BrandView.vue'
import ProductFamilyView from '../views/administration/ProductFamilyView.vue'
import ReplenishmentView from '../views/inventory/ReplenishmentView.vue'
import RelocationView from '../views/inventory/RelocationView.vue'
import NewRelocationView from '../views/inventory/NewRelocationView.vue'
import TenantManagementView from '../views/platform/TenantManagementView.vue'
import TenantDetailView from '../views/platform/TenantDetailView.vue'
import ReceivingReportView from '../views/reports/ReceivingReportView.vue'
import StockCountReportView from '../views/reports/StockCountReportView.vue'
import PickingReportView from '../views/reports/PickingReportView.vue'
import InventoryReportView from '../views/reports/InventoryReportView.vue'
import PickingKpiView from '../views/reports/PickingKpiView.vue'
import QCReportView from '../views/reports/QCReportView.vue'
import DeliveryReportView from '../views/reports/DeliveryReportView.vue'
import WarehouseLayoutView from '../views/reports/WarehouseLayoutView.vue'

const routes = [
  {
    path: '/',
    redirect: '/login',
  },
  {
    path: '/login',
    name: 'Login',
    component: LoginView,
  },{
    path: '/',
    component: MainLayout,
    children: [
      {
        path: 'dashboard',
        name: 'Dashboard',
        component: DashboardView,
        meta: { titleKey: 'nav.dashboard' },
      },
      {
        path: 'inbound/receiving',
        name: 'Receiving',
        component: ReceiveView,
        meta: { titleKey: 'nav.receiving' },
      },
      {
        path: 'inbound/quality-check',
        name: 'QualityCheck',
        component: QualityCheckView,
        meta: { titleKey: 'nav.qualityCheck' },
      },
      {
        path: 'inbound/put-away',
        name: 'PutAway',
        component: PutawayView,
        meta: { titleKey: 'nav.putaway' },
      },
      {
        path: 'inbound/transfer-receiving',
        name: 'TransferReceiving',
        component: TransferReceivingView,
        meta: { titleKey: 'nav.transferReceiving' },
      },
      {
        path: 'inventory/stock-inquiry',
        name: 'StockInquiry',
        component: StockInquiryView,
        meta: { titleKey: 'nav.stockInquiry' },
      },
      {
        path: 'inventory/stock-adjustment',
        name: 'StockAdjustment',
        component: StockAdjustmentView,
        meta: { titleKey: 'nav.stockAdjustment' },
      },
      {
        path: 'outbound/orders',
        name: 'Orders',
        component: OrderView,
        meta: { titleKey: 'nav.orders' },
      },
      {
        path: 'outbound/orders/new',
        name: 'NewOrder',
        component: OrderFormView,
        meta: { titleKey: 'nav.newOrder' },
      },
      {
        path: 'outbound/orders/:id/edit',
        name: 'EditOrder',
        component: OrderFormView,
        props: true,
        meta: { titleKey: 'nav.editOrder' },
      },
      {
        path: 'outbound/orders/:id/review',
        name: 'OrderReview',
        component: OrderReviewView,
        props: true,
        meta: { titleKey: 'nav.orderReview' },
      },
      {
        path: 'outbound/orders/:id',
        name: 'OrderDetail',
        component: OrderDetailView,
        props: true,
        meta: { titleKey: 'nav.orderDetail' },
      },
      {
        path: 'outbound/picking',
        name: 'Picking',
        component: PickingView,
        meta: { titleKey: 'nav.picking' },
      },
      {
        path: 'outbound/picking-batches',
        name: 'PickingBatches',
        component: PickingBatchView,
        meta: { titleKey: 'nav.pickingBatches' },
      },
      {
        path: 'outbound/picking-batches/new',
        name: 'NewPickingBatch',
        component: NewPickingBatchView,
        meta: { titleKey: 'nav.newPickingBatch' },
      },
      {
        path: 'outbound/picking-batches/:id',
        name: 'PickingBatchDetail',
        component: PickingBatchDetailView,
        props: true,
        meta: { titleKey: 'nav.pickingBatchDetail' },
      },
      {
        path: 'outbound/picking-batches/:id/sortation',
        name: 'Sortation',
        component: SortationView,
        props: true,
        meta: { titleKey: 'nav.sortation' },
      },
      {
        path: 'outbound/packing',
        name: 'Packing',
        component: PackingView,
        meta: { titleKey: 'nav.packing' },
      },
      {
        path: 'outbound/packing/:id',
        name: 'PackingDetail',
        component: PackingDetailView,
        props: true,
        meta: { titleKey: 'nav.packingDetail' },
      },
      {
        path: 'outbound/shipping',
        name: 'Shipping',
        component: ShippingView,
        meta: { titleKey: 'nav.shipping' },
      },
      {
        path: 'outbound/loading',
        name: 'Loading',
        component: LoadingView,
        meta: { titleKey: 'nav.loading' },
      },
      {
        path: 'outbound/loading/:id',
        name: 'LoadingTripDetail',
        component: LoadingTripDetailView,
        props: true,
        meta: { titleKey: 'nav.loadingTripDetail' },
      },
      {
        path: 'administration/user-management',
        name: 'UserManagement',
        component: UserManagementView,
        meta: { titleKey: 'nav.userManagement' },
      },
      {
        path: 'administration/roles',
        name: 'Roles',
        component: RolePermissionView,
        meta: { titleKey: 'nav.roles' },
      },
      {
        path: 'administration/warehouse',
        name: 'Warehouse',
        component: WarehouseView,
        meta: { titleKey: 'nav.warehouse' },
      },
      {
        path: 'administration/locations',
        name: 'Locations',
        component: LocationView,
        meta: { titleKey: 'nav.locations' },
      },
      {
        path: 'inbound/receiving/new',
        name: 'NewReceiving',
        component: NewReceivingView,
        meta: { titleKey: 'nav.newReceiving' }
      },
      {
        path: 'inbound/quality-check/new',
        name: 'NewQualityCheck',
        component: NewQualityCheckView,
        meta: { titleKey: 'nav.newQualityCheck' }
      },
      {
        path: 'inbound/putaway/new',
        name: 'NewPutAway',
        component: NewPutawayView,
        meta: { titleKey: 'nav.newPutaway' }
      },
      {
        path: 'inventory/check-stock',
        name: 'CheckStock',
        component: CheckStockView,
        meta: { titleKey: 'nav.checkStock' }
      },
      {
        path: 'administration/products',
        name: 'ProductMaster',
        component: ProductMasterView,
        meta: { titleKey: 'nav.productMaster' }
      },
      {
        path: 'administration/product-categories',
        name: 'ProductCategories',
        component: ProductCategoryView,
        meta: { titleKey: 'nav.productCategories' }
      },
      {
        path: 'administration/customers',
        name: 'Customers',
        component: CustomerView,
        meta: { titleKey: 'nav.customers' }
      },
      {
        path: 'administration/suppliers',
        name: 'Suppliers',
        component: SupplierView,
        meta: { titleKey: 'nav.suppliers' }
      },
      {
        path: 'administration/brands',
        name: 'Brands',
        component: BrandView,
        meta: { titleKey: 'nav.brands' }
      },
      {
        path: 'administration/product-families',
        name: 'ProductFamilies',
        component: ProductFamilyView,
        meta: { titleKey: 'nav.productFamilies' }
      },
      {
        path: 'inventory/replenishment',
        name: 'Replenishment',
        component: ReplenishmentView,
        meta: { titleKey: 'nav.replenishment' }
      },
      {
        path: 'inventory/relocation',
        name: 'Relocation',
        component: RelocationView,
        meta: { titleKey: 'nav.relocation' }
      },
      {
        path: 'inventory/relocation/new',
        name: 'NewRelocation',
        component: NewRelocationView,
        meta: { titleKey: 'nav.newRelocation' }
      },
      {
        path: 'platform/tenants',
        name: 'PlatformTenants',
        component: TenantManagementView,
        meta: { requiresPlatformAdmin: true, titleKey: 'nav.platformTenants' }
      },
      {
        path: 'platform/tenants/:id',
        name: 'PlatformTenantDetail',
        component: TenantDetailView,
        meta: { requiresPlatformAdmin: true, titleKey: 'nav.platformTenantDetail' }
      },
      // Phase 34B-34I report & warehouse-layout routes. Each needs
      // meta.titleKey (for the tab bar) and a matching entry in
      // router/permissions.js's ROUTE_PERMISSIONS (for tab-restore
      // revalidation) — both added as part of reconciling Phase 34
      // against Navigation Enhancement's architecture.
      {
        path: 'reports/receiving',
        name: 'ReportsReceiving',
        component: ReceivingReportView,
        meta: { titleKey: 'nav.reportsReceiving' }
      },
      {
        path: 'reports/stock-count',
        name: 'ReportsStockCount',
        component: StockCountReportView,
        meta: { titleKey: 'nav.reportsStockCount' }
      },
      {
        path: 'reports/picking-issue',
        name: 'ReportsPickingIssue',
        component: PickingReportView,
        meta: { titleKey: 'nav.reportsPickingIssue' }
      },
      {
        path: 'reports/inventory',
        name: 'ReportsInventory',
        component: InventoryReportView,
        meta: { titleKey: 'nav.reportsInventory' }
      },
      {
        path: 'reports/picking-kpi',
        name: 'ReportsPickingKpi',
        component: PickingKpiView,
        meta: { titleKey: 'nav.reportsPickingKpi' }
      },
      {
        path: 'reports/qc',
        name: 'ReportsQc',
        component: QCReportView,
        meta: { titleKey: 'nav.reportsQc' }
      },
      {
        path: 'reports/delivery',
        name: 'ReportsDelivery',
        component: DeliveryReportView,
        meta: { titleKey: 'nav.reportsDelivery' }
      },
      {
        // Gated on layout.view at the router/permissions.js level, not
        // report.view — see Sidebar.vue/MainLayout.vue's
        // canViewWarehouseLayout for why.
        path: 'reports/warehouse-layout',
        name: 'ReportsWarehouseLayout',
        component: WarehouseLayoutView,
        meta: { titleKey: 'nav.reportsWarehouseLayout' }
      }
    ]
  }
]

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes: routes,
})

// REQ-05: restore persisted tabs *before* the router's first navigation
// fires (which happens once main.js mounts the app against this router,
// strictly after this module finishes evaluating) — the first
// navigation's afterEach below calls openTab() unconditionally, which
// persists whatever is in memory at that moment. Restoring any later
// (e.g. from MainLayout's mounted()) would already have been clobbered
// by that first single-tab write. Known limitation: a tab gated on
// Platform Admin status won't restore on a hard refresh, because that
// status is resolved async (see services/auth.js) and isn't known yet at
// this synchronous, module-load-time point — it fails closed rather than
// risk restoring an unauthorized tab.
restoreTabs((name, params) => {
  try {
    return router.resolve({ name, params })
  } catch {
    return null
  }
})

// Guard: unauthenticated users are redirected to /login for any other
// route; an already-authenticated user landing on /login is sent
// straight to /dashboard instead of being asked to log in again.
//
// requiresPlatformAdmin routes get one more check, re-verified fresh
// against GET /api/v1/platform/me on every navigation into them — never
// relying on a cached in-memory answer from earlier in the session, so
// a mid-session revocation (or a user who was never a platform admin
// deep-linking straight to a tenant-management URL) is denied
// immediately in the UI too. This is a UX guard only: every actual
// /api/v1/platform/tenants/* call the resulting page makes is
// independently re-verified server-side by RequirePlatformAdmin
// regardless of what this guard decides.
router.beforeEach(async (to) => {
  const authed = isAuthenticated()

  if (to.path !== '/login' && !authed) {
    return '/login'
  }

  if (to.path === '/login' && authed) {
    return '/dashboard'
  }

  if (to.matched.some((record) => record.meta.requiresPlatformAdmin)) {
    const allowed = await checkPlatformAdminStatus()
    if (!allowed) {
      return '/dashboard'
    }
  }

  // REQ-07: the 10-tab cap is enforced here, *before* the navigation
  // commits, so a blocked attempt leaves the URL and visible workspace
  // exactly as they were — not after redirecting and then having to
  // explain why. Reactivating an already-open tab is always allowed
  // (canOpen), regardless of count. Login/unnamed routes are never tabs.
  if (to.name && to.name !== 'Login' && !canOpen(to)) {
    showToast(i18n.global.t('tabs.maxReached'), 'error')
    return false
  }

  return true
})

// Registers/activates the internal tab for every committed navigation
// into a tabbable route (REQ-02/03/09) — sidebar clicks, direct URL
// entry, browser back/forward, and programmatic pushes (e.g. the
// notification bell's deep link) all funnel through here the same way,
// so none of them need their own tab-opening logic.
router.afterEach((to) => {
  if (to.name && to.name !== 'Login') {
    openTab(to)
  }
})

// A 401 from any API call (expired/invalidated token) sends the user
// back to /login the same way the guard above does. Also clears tabs,
// their cached component instances, and dirty-check registrations, the
// same as a normal logout (MainLayout.vue's logout()) — an invalidated
// session must not leave previously loaded protected tabs/cache sitting
// around for whoever uses this browser next.
setUnauthorizedHandler(() => {
  clearAllTabs()
  pruneTabComponentCache([])
  clearAllDirtyChecks()
  if (router.currentRoute.value.path !== '/login') {
    router.push('/login')
  }
})

export default router
