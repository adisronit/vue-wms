<template>
  <div class="h-screen flex bg-slate-100 overflow-hidden">
    <Sidebar
      :can-view-users="canViewUsers"
      :can-view-roles="canViewRoles"
      :can-view-warehouses="canViewWarehouses"
      :can-view-products="canViewProducts"
      :can-view-product-categories="canViewProductCategories"
      :can-view-customers="canViewCustomers"
      :can-view-suppliers="canViewSuppliers"
      :can-view-replenishment="canViewReplenishment"
      :can-view-platform-admin="canViewPlatformAdmin"
      @logout="logout"
    />

    <!-- Main Area -->
    <div class="flex-1 min-w-0 flex flex-col">
      <!-- Topbar -->
      <header class="h-16 shrink-0 bg-white border-b border-slate-200 flex items-center justify-between px-6">
        <div class="min-w-0">
          <h2 class="text-lg font-bold text-slate-800 truncate">
            {{ $t('layout.headerTitle') }}
          </h2>
          <p class="text-xs text-slate-500 truncate">
            {{ $t('layout.headerSubtitle') }}
          </p>
        </div>

        <div class="flex items-center gap-4 shrink-0">
          <LanguageSwitcher />

          <div class="relative">
            <button
              @click="toggleNotifications"
              class="relative w-10 h-10 rounded-full bg-slate-100 hover:bg-slate-200"
            >
              🔔
              <span v-if="unreadCount > 0" class="absolute top-1 right-1 w-2 h-2 bg-orange-500 rounded-full"></span>
            </button>

            <div
              v-if="showNotifications"
              class="absolute right-0 mt-2 w-96 max-h-[28rem] overflow-y-auto bg-white rounded-2xl border border-slate-200 shadow-lg z-50"
            >
              <div class="px-4 py-3 border-b border-slate-200 flex items-center justify-between">
                <h3 class="font-bold text-slate-800">{{ $t('layout.notifications.title') }}</h3>
                <span class="text-xs text-slate-500">{{ unreadCount }} {{ $t('layout.notifications.unread') }}</span>
              </div>

              <div v-if="notificationsLoading" class="p-6 text-center text-slate-400 text-sm">
                {{ $t('layout.notifications.loading') }}
              </div>
              <div v-else-if="notifications.length === 0" class="p-6 text-center text-slate-400 text-sm">
                {{ $t('layout.notifications.empty') }}
              </div>
              <button
                v-else
                v-for="n in notifications"
                :key="n.id"
                @click="openNotification(n)"
                class="w-full text-left px-4 py-3 border-b border-slate-100 hover:bg-slate-50 flex gap-3"
                :class="n.is_read ? 'opacity-60' : ''"
              >
                <span class="mt-1 w-2 h-2 rounded-full shrink-0" :class="n.is_read ? 'bg-transparent' : 'bg-orange-500'"></span>
                <span class="min-w-0">
                  <span class="block text-sm font-semibold text-slate-800 truncate">{{ n.title }}</span>
                  <span class="block text-xs text-slate-500">{{ n.message }}</span>
                </span>
              </button>
            </div>
          </div>

          <div class="flex items-center gap-3">
            <div class="text-right hidden sm:block">
              <p class="text-sm font-semibold text-slate-800">{{ user?.full_name || $t('layout.userFallback') }}</p>
              <p class="text-xs text-slate-500">{{ user?.role?.name || '' }}</p>
            </div>

            <div class="w-11 h-11 bg-orange-600 text-white rounded-full flex items-center justify-center font-bold">
              {{ userInitial }}
            </div>
          </div>
        </div>
      </header>

      <!-- Page Content -->
      <main class="flex-1 min-w-0 overflow-y-auto overflow-x-hidden p-6">
        <router-view />
      </main>
    </div>

    <ToastHost />
  </div>
</template>

<script>
import { currentUser, hasPermission, logout as clearSession, checkPlatformAdminStatus, isPlatformAdmin } from '../services/auth'
import { listNotifications, markNotificationRead } from '../services/notifications'
import LanguageSwitcher from '../components/LanguageSwitcher.vue'
import Sidebar from '../components/layout/Sidebar.vue'
import ToastHost from '../components/ui/ToastHost.vue'

let notificationPoll = null

export default {
  components: { LanguageSwitcher, Sidebar, ToastHost },
  data() {
    return {
      user: currentUser(),
      notifications: [],
      unreadCount: 0,
      notificationsLoading: false,
      showNotifications: false,
      // Resolved asynchronously below (checkPlatformAdminStatus), not
      // read synchronously from isPlatformAdmin() at data() time — the
      // very first render, before that check has resolved, must show
      // the nav item hidden (fail closed), not stale-true from a
      // previous session.
      platformAdmin: false
    }
  },

  computed: {
    userInitial() {
      return this.user?.full_name?.charAt(0)?.toUpperCase() || 'U'
    },
    canViewUsers() {
      return hasPermission('users.view')
    },
    canViewRoles() {
      return hasPermission('roles.view')
    },
    canViewWarehouses() {
      return hasPermission('warehouse.view')
    },
    canViewProducts() {
      return hasPermission('product.view')
    },
    canViewProductCategories() {
      return hasPermission('product_category.view')
    },
    canViewCustomers() {
      return hasPermission('customer.view')
    },
    canViewSuppliers() {
      return hasPermission('supplier.view')
    },
    canViewReplenishment() {
      return hasPermission('replenishment.view')
    },
    canViewPlatformAdmin() {
      return this.platformAdmin
    }
  },

  mounted() {
    // MainLayout mounts once per session on a full load/reload (e.g.
    // the user refreshes while already logged in) — LoginView.vue
    // already resolves this right after a fresh login, so this call is
    // usually a cheap, already-cached re-read, not a second real check.
    checkPlatformAdminStatus().then(() => {
      this.platformAdmin = isPlatformAdmin()
    })
    this.fetchUnreadCount()
    // Light periodic refresh so the badge doesn't need a page reload to
    // notice a new reminder — matches the reminder scanner's own
    // periodic-not-realtime shape, no websocket/SSE introduced for this.
    notificationPoll = setInterval(this.fetchUnreadCount, 60000)
  },

  beforeUnmount() {
    if (notificationPoll) clearInterval(notificationPoll)
  },

  methods: {
    logout() {
      clearSession()
      this.$router.push('/login')
    },

    async fetchUnreadCount() {
      try {
        const res = await listNotifications({ isRead: false, limit: 100 })
        this.unreadCount = res.meta.total
      } catch {
        // Non-critical: the bell simply doesn't update this cycle.
      }
    },

    async toggleNotifications() {
      this.showNotifications = !this.showNotifications
      if (!this.showNotifications) return

      this.notificationsLoading = true
      try {
        const res = await listNotifications({ limit: 10 })
        this.notifications = res.data
      } catch {
        this.notifications = []
      } finally {
        this.notificationsLoading = false
      }
    },

    async openNotification(n) {
      if (!n.is_read) {
        try {
          await markNotificationRead(n.id)
          n.is_read = true
          this.unreadCount = Math.max(0, this.unreadCount - 1)
        } catch {
          // Non-critical: the notification just stays marked unread for now.
        }
      }

      this.showNotifications = false

      if (n.reference_type === 'STOCK_ADJUSTMENT' && n.reference_id) {
        this.$router.push(`/inventory/stock-adjustment?adjustment_id=${n.reference_id}`)
      }
    }
  }
}
</script>
