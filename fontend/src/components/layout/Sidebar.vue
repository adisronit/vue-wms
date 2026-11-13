<template>
  <aside class="w-64 shrink-0 bg-slate-900 text-white hidden lg:flex flex-col">
    <div class="h-16 flex items-center px-5 border-b border-slate-700 shrink-0">
      <div class="w-9 h-9 shrink-0 bg-orange-600 rounded-lg flex items-center justify-center text-base font-bold">
        W
      </div>

      <div class="ml-3 min-w-0">
        <h1 class="text-sm font-bold truncate leading-tight">{{ $t('common.appName') }}</h1>
        <p class="text-xs text-slate-400 truncate leading-tight">{{ $t('common.appTagline') }}</p>
      </div>
    </div>

    <nav class="flex-1 px-3 py-4 space-y-0.5 overflow-y-auto sidebar-scroll" aria-label="Primary">
      <router-link to="/dashboard" class="menu-item" active-class="menu-active">
        {{ $t('nav.dashboard') }}
      </router-link>

      <p class="menu-title">{{ $t('nav.sectionInbound') }}</p>
      <router-link to="/inbound/receiving" class="menu-item" active-class="menu-active">
        {{ $t('nav.receiving') }}
      </router-link>
      <router-link to="/inbound/quality-check" class="menu-item" active-class="menu-active">
        {{ $t('nav.qualityCheck') }}
      </router-link>
      <router-link to="/inbound/put-away" class="menu-item" active-class="menu-active">
        {{ $t('nav.putaway') }}
      </router-link>

      <p class="menu-title">{{ $t('nav.sectionInventory') }}</p>
      <router-link to="/inventory/stock-inquiry" class="menu-item" active-class="menu-active">
        {{ $t('nav.stockInquiry') }}
      </router-link>
      <router-link to="/inventory/stock-adjustment" class="menu-item" active-class="menu-active">
        {{ $t('nav.stockAdjustment') }}
      </router-link>
      <router-link to="/inventory/check-stock" class="menu-item" active-class="menu-active">
        {{ $t('nav.checkStock') }}
      </router-link>
      <router-link v-if="canViewReplenishment" to="/inventory/replenishment" class="menu-item" active-class="menu-active">
        {{ $t('nav.replenishment') }}
      </router-link>

      <p class="menu-title">{{ $t('nav.sectionOutbound') }}</p>
      <router-link to="/outbound/orders" class="menu-item" active-class="menu-active">
        {{ $t('nav.orders') }}
      </router-link>
      <router-link to="/outbound/picking" class="menu-item" active-class="menu-active">
        {{ $t('nav.picking') }}
      </router-link>
      <router-link to="/outbound/shipping" class="menu-item" active-class="menu-active">
        {{ $t('nav.shipping') }}
      </router-link>

      <template v-if="canViewWarehouses || canViewProducts">
        <p class="menu-title">{{ $t('nav.sectionMasterData') }}</p>
        <router-link v-if="canViewWarehouses" to="/administration/warehouse" class="menu-item" active-class="menu-active">
          {{ $t('nav.warehouse') }}
        </router-link>
        <router-link v-if="canViewProducts" to="/administration/products" class="menu-item" active-class="menu-active">
          {{ $t('nav.productMaster') }}
        </router-link>
      </template>

      <template v-if="canViewUsers || canViewRoles">
        <p class="menu-title">{{ $t('nav.sectionAdministration') }}</p>
        <router-link v-if="canViewUsers" to="/administration/user-management" class="menu-item" active-class="menu-active">
          {{ $t('nav.userManagement') }}
        </router-link>
        <router-link v-if="canViewRoles" to="/administration/roles" class="menu-item" active-class="menu-active">
          {{ $t('nav.roles') }}
        </router-link>
      </template>

      <template v-if="canViewPlatformAdmin">
        <p class="menu-title">{{ $t('nav.sectionPlatformAdmin') }}</p>
        <router-link to="/platform/tenants" class="menu-item" active-class="menu-active">
          {{ $t('nav.platformTenants') }}
        </router-link>
      </template>
    </nav>

    <div class="p-3 border-t border-slate-700">
      <button @click="$emit('logout')" class="logout-button">
        {{ $t('nav.logout') }}
      </button>
    </div>
  </aside>
</template>

<script>
// Primary navigation, grouped by workflow (Dashboard / Inbound /
// Inventory / Outbound / Master Data / Administration / Platform
// Administration) rather than the previous flat "Administration"
// bucket that mixed Warehouse/Product Master in with Users/Roles.
// Deliberately text-only, no per-item decorative icon: a left-accent
// background on the active item (see .menu-active) is enough to orient
// the eye without the inconsistent emoji glyphs the previous sidebar
// used (rendering differently per OS/font and reading as informal
// rather than professional).
export default {
  name: 'Sidebar',

  props: {
    canViewUsers: { type: Boolean, default: false },
    canViewRoles: { type: Boolean, default: false },
    canViewWarehouses: { type: Boolean, default: false },
    canViewProducts: { type: Boolean, default: false },
    canViewReplenishment: { type: Boolean, default: false },
    canViewPlatformAdmin: { type: Boolean, default: false }
  },

  emits: ['logout']
}
</script>

<style scoped>
.menu-item {
  display: block;
  padding: 10px 12px;
  border-radius: 8px;
  color: #cbd5e1;
  font-size: 14px;
  font-weight: 500;
  text-decoration: none;
  border-left: 3px solid transparent;
  transition: background-color 0.15s, color 0.15s;
}

.menu-item:hover {
  background: #1e293b;
  color: white;
}

.menu-active {
  background: #1e293b;
  color: white;
  border-left-color: #ea580c;
  font-weight: 600;
}

.menu-title {
  margin: 16px 12px 6px;
  color: #64748b;
  font-size: 11px;
  text-transform: uppercase;
  letter-spacing: 0.5px;
  font-weight: 600;
}

.logout-button {
  width: 100%;
  background: #1e293b;
  color: white;
  padding: 10px;
  border-radius: 8px;
  font-size: 14px;
  font-weight: 500;
  transition: background-color 0.15s;
}

.logout-button:hover {
  background: #dc2626;
}

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
