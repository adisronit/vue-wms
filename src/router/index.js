import { createRouter, createWebHistory } from 'vue-router'
import LgoinView from '../views/auth/LoginView.vue'
import MainLayout from '../layouts/MainLayout.vue'
import DashboardView from '../views/dashboard/DashboardView.vue'
import ReceiveView from '../views/inbound/ReceiveView.vue'
import QualityCheckView from '../views/inbound/QualityCheckView.vue'
import PutawayView from '../views/inbound/PutawayView.vue'
import StockInquiryView from '../views/inventory/StockInquiryView.vue'
import StockAdjustmentView from '../views/inventory/StockAdjustmentView.vue'
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
      }
    ]
  }
]

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes: routes,
})

export default router
