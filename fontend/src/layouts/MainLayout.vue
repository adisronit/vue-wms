<template>
  <div class="h-screen flex bg-slate-100 overflow-hidden">
    <!-- Sidebar -->
    <aside class="w-72 shrink-0 bg-slate-900 text-white hidden lg:flex flex-col">
      <div class="h-20 flex items-center px-6 border-b border-slate-700 shrink-0">
        <div class="w-11 h-11 shrink-0 bg-orange-600 rounded-xl flex items-center justify-center text-xl">
          📦
        </div>

        <div class="ml-3 min-w-0">
          <h1 class="text-lg font-bold truncate">{{ $t('common.appName') }}</h1>
          <p class="text-xs text-slate-400 truncate">{{ $t('common.appTagline') }}</p>
        </div>
      </div>

      <nav class="flex-1 px-4 py-6 space-y-2 overflow-y-auto sidebar-scroll">
        <router-link
          to="/dashboard"
          class="menu-item"
          active-class="menu-active"
        >
          <span class="menu-icon">🏠</span>
          <span class="menu-text">{{ $t('nav.dashboard') }}</span>
        </router-link>

        <div class="menu-title">{{ $t('nav.sectionInbound') }}</div>

        <router-link
          to="/inbound/receiving"
          class="menu-item"
          active-class="menu-active"
        >
          <span class="menu-icon">📥</span>
          <span class="menu-text">{{ $t('nav.receiving') }}</span>
        </router-link>

        <router-link
          to="/inbound/quality-check"
          class="menu-item"
          active-class="menu-active"
        >
          <span class="menu-icon">✅</span>
          <span class="menu-text">{{ $t('nav.qualityCheck') }}</span>
        </router-link>

        <router-link
          to="/inbound/put-away"
          class="menu-item"
          active-class="menu-active"
        >
          <span class="menu-icon">📍</span>
          <span class="menu-text">{{ $t('nav.putaway') }}</span>
        </router-link>

        <div class="menu-title">{{ $t('nav.sectionInventory') }}</div>

        <router-link
          to="/inventory/stock-inquiry"
          class="menu-item"
          active-class="menu-active"
        >
          <span class="menu-icon">📦</span>
          <span class="menu-text">{{ $t('nav.stockInquiry') }}</span>
        </router-link>

        <router-link
          to="/inventory/stock-adjustment"
          class="menu-item"
          active-class="menu-active"
        >
          <span class="menu-icon">🔄</span>
          <span class="menu-text">{{ $t('nav.stockAdjustment') }}</span>
        </router-link>

        <router-link
          to="/inventory/check-stock"
          class="menu-item"
          active-class="menu-active"
        >
          <span class="menu-icon">🧾</span>
          <span class="menu-text">{{ $t('nav.checkStock') }}</span>
        </router-link>

        <router-link
          v-if="canViewReplenishment"
          to="/inventory/replenishment"
          class="menu-item"
          active-class="menu-active"
        >
          <span class="menu-icon">🚛</span>
          <span class="menu-text">{{ $t('nav.replenishment') }}</span>
        </router-link>

        <div class="menu-title">{{ $t('nav.sectionOutbound') }}</div>

        <router-link
          to="/outbound/orders"
          class="menu-item"
          active-class="menu-active"
        >
          <span class="menu-icon">🧾</span>
          <span class="menu-text">{{ $t('nav.orders') }}</span>
        </router-link>

        <router-link
          to="/outbound/picking"
          class="menu-item"
          active-class="menu-active"
        >
          <span class="menu-icon">📤</span>
          <span class="menu-text">{{ $t('nav.picking') }}</span>
        </router-link>

        <router-link
          to="/outbound/shipping"
          class="menu-item"
          active-class="menu-active"
        >
          <span class="menu-icon">🚚</span>
          <span class="menu-text">{{ $t('nav.shipping') }}</span>
        </router-link>

        <div class="menu-title">{{ $t('nav.sectionAdministration') }}</div>

          <router-link
            v-if="canViewUsers"
            to="/administration/user-management"
            class="menu-item"
            active-class="menu-active"
          >
            <span class="menu-icon">👤</span>
            <span class="menu-text">{{ $t('nav.userManagement') }}</span>
          </router-link>

          <router-link
            v-if="canViewRoles"
            to="/administration/roles"
            class="menu-item"
            active-class="menu-active"
          >
            <span class="menu-icon">🔐</span>
            <span class="menu-text">{{ $t('nav.roles') }}</span>
          </router-link>

          <router-link
            v-if="canViewWarehouses"
            to="/administration/warehouse"
            class="menu-item"
            active-class="menu-active"
          >
            <span class="menu-icon">🏭</span>
            <span class="menu-text">{{ $t('nav.warehouse') }}</span>
          </router-link>

          <router-link
            v-if="canViewProducts"
            to="/administration/products"
            class="menu-item"
            active-class="menu-active"
          >
            <span class="menu-icon">📦</span>
            <span class="menu-text">{{ $t('nav.productMaster') }}</span>
          </router-link>

          <template v-if="canViewPlatformAdmin">
            <div class="menu-title">{{ $t('nav.sectionPlatformAdmin') }}</div>

            <router-link
              to="/platform/tenants"
              class="menu-item"
              active-class="menu-active"
            >
              <span class="menu-icon">🏢</span>
              <span class="menu-text">{{ $t('nav.platformTenants') }}</span>
            </router-link>
          </template>
      </nav>

      <div class="p-4 border-t border-slate-700">
        <button @click="logout"
          class="w-full bg-slate-800 hover:bg-red-600 text-white py-3 rounded-xl transition"
        >
          {{ $t('nav.logout') }}
        </button>
      </div>
    </aside>

    <!-- Main Area -->
    <div class="flex-1 min-w-0 flex flex-col">
      <!-- Topbar -->
      <header class="h-20 shrink-0 bg-white border-b border-slate-200 flex items-center justify-between px-6">
        <div class="min-w-0">
          <h2 class="text-xl font-bold text-slate-800 truncate">
            {{ $t('layout.headerTitle') }}
          </h2>
          <p class="text-sm text-slate-500 truncate">
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
  </div>
</template>

<script>
import { currentUser, hasPermission, logout as clearSession, checkPlatformAdminStatus, isPlatformAdmin } from '../services/auth'
import { listNotifications, markNotificationRead } from '../services/notifications'
import LanguageSwitcher from '../components/LanguageSwitcher.vue'

let notificationPoll = null

export default {
  components: { LanguageSwitcher },
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

<style scoped>

.menu-item {
  display: flex;
  align-items: center;
  gap: 10px;

  padding: 12px 14px;

  border-radius: 12px;

  color: #cbd5e1;

  transition: all .2s;

  text-decoration: none;
}

.menu-item:hover {
  background: #1e293b;
}

.menu-active {
  background: #ea580c;
  color: white;
}

.menu-title {
  margin-top: 20px;
  margin-bottom: 8px;

  color: #64748b;

  font-size: 11px;

  text-transform: uppercase;

  letter-spacing: 1px;

  font-weight: 700;
}

/* Custom Scrollbar */

.sidebar-scroll::-webkit-scrollbar {
  width: 6px;
}

.sidebar-scroll::-webkit-scrollbar-track {
  background: transparent;
}

.sidebar-scroll::-webkit-scrollbar-thumb {
  background: #475569;
  border-radius: 10px;
}

.sidebar-scroll::-webkit-scrollbar-thumb:hover {
  background: #64748b;
}

</style>