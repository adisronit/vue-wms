import { createRouter, createWebHistory } from 'vue-router'
import { isAuthenticated } from '../services/auth'
import { setUnauthorizedHandler } from '../services/api'
import LgoinView from '../views/auth/LoginView.vue'
import MainLayout from '../layouts/MainLayout.vue'
import DashboardView from '../views/dashboard/DashboardView.vue'
import ReceiveView from '../views/inbound/ReceiveView.vue'
import QualityCheckView from '../views/inbound/QualityCheckView.vue'
import PutawayView from '../views/inbound/PutawayView.vue'
import StockInquiryView from '../views/inventory/StockInquiryView.vue'
import StockAdjustmentView from '../views/inventory/StockAdjustmentView.vue'
import OrderView from '../views/outbound/OrderView.vue'
import PickingView from '../views/outbound/PickingView.vue'
import ShippingView from '../views/outbound/ShippingView.vue'
import UserManagementView from '../views/administration/UserManagementView.vue'
import RolePermissionView from '../views/administration/RolePermissionView.vue'
import WarehouseView from '../views/administration/WarehouseView.vue'
import NewReceivingView from '../views/inbound/NewReceivingView.vue'
import NewQualityCheckView from '../views/inbound/NewQualityCheckView.vue'
import NewPutawayView from '../views/inbound/NewPutawayView.vue'
import CheckStockView from '../views/inventory/CheckStockView.vue'
import ProductMasterView from '../views/administration/ProductMasterView.vue'
import ReplenishmentView from '../views/inventory/ReplenishmentView.vue'

const routes = [
  {
    path: '/',
    redirect: '/login',
  },
  {
    path: '/login',
    name: 'Login',
    component: LgoinView,
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
        path: 'outbound/picking',
        name: 'Picking',
        component: PickingView,
      },
      {
        path: 'outbound/shipping',
        name: 'Shipping',
        component: ShippingView,
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
        path: 'inventory/replenishment',
        name: 'Replenishment',
        component: ReplenishmentView
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
router.beforeEach((to) => {
  const authed = isAuthenticated()

  if (to.path !== '/login' && !authed) {
    return '/login'
  }

  if (to.path === '/login' && authed) {
    return '/dashboard'
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
