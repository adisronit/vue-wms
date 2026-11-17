import { createRouter, createWebHistory } from 'vue-router'
import { isAuthenticated, checkPlatformAdminStatus } from '../services/auth'
import { setUnauthorizedHandler } from '../services/api'
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
import ReportComingSoonView from '../views/reports/ReportComingSoonView.vue'
import ReceivingReportView from '../views/reports/ReceivingReportView.vue'
import StockCountReportView from '../views/reports/StockCountReportView.vue'
import PickingReportView from '../views/reports/PickingReportView.vue'
import InventoryReportView from '../views/reports/InventoryReportView.vue'
import PickingKpiView from '../views/reports/PickingKpiView.vue'
import QCReportView from '../views/reports/QCReportView.vue'

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
      },
      {
        path: 'inbound/receiving',
        name: 'Receiving',
        component: ReceiveView,
      },
      {
        path: 'inbound/quality-check',
        name: 'QualityCheck',
        component: QualityCheckView,
      },
      {
        path: 'inbound/put-away',
        name: 'PutAway',
        component: PutawayView,
      },
      {
        path: 'inbound/transfer-receiving',
        name: 'TransferReceiving',
        component: TransferReceivingView,
      },
      {
        path: 'inventory/stock-inquiry',
        name: 'StockInquiry',
        component: StockInquiryView,
      },
      {
        path: 'inventory/stock-adjustment',
        name: 'StockAdjustment',
        component: StockAdjustmentView,
      },
      {
        path: 'outbound/orders',
        name: 'Orders',
        component: OrderView,
      },
      {
        path: 'outbound/orders/new',
        name: 'NewOrder',
        component: OrderFormView,
      },
      {
        path: 'outbound/orders/:id/edit',
        name: 'EditOrder',
        component: OrderFormView,
        props: true,
      },
      {
        path: 'outbound/orders/:id/review',
        name: 'OrderReview',
        component: OrderReviewView,
        props: true,
      },
      {
        path: 'outbound/orders/:id',
        name: 'OrderDetail',
        component: OrderDetailView,
        props: true,
      },
      {
        path: 'outbound/picking',
        name: 'Picking',
        component: PickingView,
      },
      {
        path: 'outbound/picking-batches',
        name: 'PickingBatches',
        component: PickingBatchView,
      },
      {
        path: 'outbound/picking-batches/new',
        name: 'NewPickingBatch',
        component: NewPickingBatchView,
      },
      {
        path: 'outbound/picking-batches/:id',
        name: 'PickingBatchDetail',
        component: PickingBatchDetailView,
        props: true,
      },
      {
        path: 'outbound/picking-batches/:id/sortation',
        name: 'Sortation',
        component: SortationView,
        props: true,
      },
      {
        path: 'outbound/packing',
        name: 'Packing',
        component: PackingView,
      },
      {
        path: 'outbound/packing/:id',
        name: 'PackingDetail',
        component: PackingDetailView,
        props: true,
      },
      {
        path: 'outbound/shipping',
        name: 'Shipping',
        component: ShippingView,
      },
      {
        path: 'outbound/loading',
        name: 'Loading',
        component: LoadingView,
      },
      {
        path: 'outbound/loading/:id',
        name: 'LoadingTripDetail',
        component: LoadingTripDetailView,
        props: true,
      },
      {
        path: 'administration/user-management',
        name: 'UserManagement',
        component: UserManagementView,
      },
      {
        path: 'administration/roles',
        name: 'Roles',
        component: RolePermissionView,
      },
      {
        path: 'administration/warehouse',
        name: 'Warehouse',
        component: WarehouseView,
      },
      {
        path: 'administration/locations',
        name: 'Locations',
        component: LocationView,
      },
      {
        path: 'inbound/receiving/new',
        name: 'NewReceiving',
        component: NewReceivingView
      },
      {
        path: 'inbound/quality-check/new',
        name: 'NewQualityCheck',
        component: NewQualityCheckView
      },
      {
        path: 'inbound/putaway/new',
        name: 'NewPutAway',
        component: NewPutawayView
      },
      {
        path: 'inventory/check-stock',
        name: 'CheckStock',
        component: CheckStockView
      },
      {
        path: 'administration/products',
        name: 'ProductMaster',
        component: ProductMasterView
      },
      {
        path: 'administration/product-categories',
        name: 'ProductCategories',
        component: ProductCategoryView
      },
      {
        path: 'administration/customers',
        name: 'Customers',
        component: CustomerView
      },
      {
        path: 'administration/suppliers',
        name: 'Suppliers',
        component: SupplierView
      },
      {
        path: 'administration/brands',
        name: 'Brands',
        component: BrandView
      },
      {
        path: 'administration/product-families',
        name: 'ProductFamilies',
        component: ProductFamilyView
      },
      {
        path: 'inventory/replenishment',
        name: 'Replenishment',
        component: ReplenishmentView
      },
      {
        path: 'inventory/relocation',
        name: 'Relocation',
        component: RelocationView
      },
      {
        path: 'inventory/relocation/new',
        name: 'NewRelocation',
        component: NewRelocationView
      },
      {
        path: 'platform/tenants',
        name: 'PlatformTenants',
        component: TenantManagementView,
        meta: { requiresPlatformAdmin: true }
      },
      {
        path: 'platform/tenants/:id',
        name: 'PlatformTenantDetail',
        component: TenantDetailView,
        meta: { requiresPlatformAdmin: true }
      },
      // Phase 34A: every Reports & Analytics submenu routes to the same
      // framework-demo placeholder, parameterized by reportKey — each
      // gets replaced with its own real view in its own later Phase 34
      // batch (see ReportComingSoonView.vue's REPORT_META).
      {
        path: 'reports/receiving',
        name: 'ReportsReceiving',
        component: ReceivingReportView
      },
      {
        path: 'reports/stock-count',
        name: 'ReportsStockCount',
        component: StockCountReportView
      },
      {
        path: 'reports/picking-issue',
        name: 'ReportsPickingIssue',
        component: PickingReportView
      },
      {
        path: 'reports/inventory',
        name: 'ReportsInventory',
        component: InventoryReportView
      },
      {
        path: 'reports/picking-kpi',
        name: 'ReportsPickingKpi',
        component: PickingKpiView
      },
      {
        path: 'reports/qc',
        name: 'ReportsQc',
        component: QCReportView
      },
      {
        path: 'reports/delivery',
        name: 'ReportsDelivery',
        component: ReportComingSoonView,
        props: { reportKey: 'delivery' }
      },
      {
        path: 'reports/warehouse-layout',
        name: 'ReportsWarehouseLayout',
        component: ReportComingSoonView,
        props: { reportKey: 'warehouseLayout' }
      }
    ]
  }
]

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes: routes,
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

  return true
})

// A 401 from any API call (expired/invalidated token) sends the user
// back to /login the same way the guard above does.
setUnauthorizedHandler(() => {
  if (router.currentRoute.value.path !== '/login') {
    router.push('/login')
  }
})

export default router
