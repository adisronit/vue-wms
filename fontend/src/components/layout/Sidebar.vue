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

    <div class="px-3 py-3 border-b border-slate-700 shrink-0">
      <div class="relative">
        <svg
          class="pointer-events-none absolute left-2.5 top-1/2 -translate-y-1/2 w-4 h-4 text-slate-500"
          viewBox="0 0 20 20"
          fill="none"
          stroke="currentColor"
          stroke-width="1.75"
          aria-hidden="true"
        >
          <circle cx="8.5" cy="8.5" r="5.5" />
          <line x1="12.8" y1="12.8" x2="17" y2="17" stroke-linecap="round" />
        </svg>
        <input
          ref="searchInput"
          v-model="searchQuery"
          type="text"
          role="searchbox"
          class="search-input w-full bg-slate-800 text-white placeholder-slate-500 text-sm rounded-lg pl-8 pr-8 py-2 border border-slate-700 focus:outline-none focus:ring-2 focus:ring-orange-500 focus:border-transparent"
          :placeholder="$t('nav.searchPlaceholder')"
          :aria-label="$t('nav.searchPlaceholder')"
          @keydown.esc="clearSearch"
        />
        <button
          v-if="searchQuery"
          type="button"
          class="clear-btn absolute right-2 top-1/2 -translate-y-1/2 text-slate-400 hover:text-white"
          :aria-label="$t('nav.searchClear')"
          @mousedown.prevent="clearSearch"
        >
          ✕
        </button>
      </div>
    </div>

    <nav class="flex-1 px-3 py-4 space-y-0.5 overflow-y-auto sidebar-scroll" aria-label="Primary">
      <template v-if="filteredSections.length > 0">
        <template v-for="section in filteredSections" :key="section.key">
          <p v-if="section.titleKey" class="menu-title">{{ $t(section.titleKey) }}</p>
          <router-link
            v-for="item in section.items"
            :key="item.to"
            :to="item.to"
            class="menu-item"
            active-class="menu-active"
          >
            {{ $t(item.labelKey) }}
          </router-link>
        </template>
      </template>
      <p v-else class="menu-empty">{{ $t('nav.searchNoResults') }}</p>
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
    canViewProductCategories: { type: Boolean, default: false },
    canViewCustomers: { type: Boolean, default: false },
    canViewSuppliers: { type: Boolean, default: false },
    canViewBrands: { type: Boolean, default: false },
    canViewProductFamilies: { type: Boolean, default: false },
    canViewReplenishment: { type: Boolean, default: false },
    canViewRelocation: { type: Boolean, default: false },
    canViewTransferReceipt: { type: Boolean, default: false },
    canViewPacking: { type: Boolean, default: false },
    canViewLoading: { type: Boolean, default: false },
    canViewPickingBatches: { type: Boolean, default: false },
    canViewPlatformAdmin: { type: Boolean, default: false }
  },

  emits: ['logout'],

  data() {
    return {
      searchQuery: ''
    }
  },

  computed: {
    searchTerm() {
      return this.searchQuery.trim().toLowerCase()
    },

    // Single source of truth for the nav tree, mirroring the previous
    // hardcoded template 1:1 (same groups, same items, same per-item
    // permission gate) so both the normal render and the search filter
    // read from one place instead of duplicating the menu twice.
    menuSections() {
      return [
        {
          key: 'dashboard',
          titleKey: null,
          items: [
            { labelKey: 'nav.dashboard', to: '/dashboard', visible: true }
          ]
        },
        {
          key: 'inbound',
          titleKey: 'nav.sectionInbound',
          items: [
            { labelKey: 'nav.receiving', to: '/inbound/receiving', visible: true },
            { labelKey: 'nav.qualityCheck', to: '/inbound/quality-check', visible: true },
            { labelKey: 'nav.putaway', to: '/inbound/put-away', visible: true },
            { labelKey: 'nav.transferReceiving', to: '/inbound/transfer-receiving', visible: this.canViewTransferReceipt }
          ]
        },
        {
          key: 'inventory',
          titleKey: 'nav.sectionInventory',
          items: [
            { labelKey: 'nav.stockInquiry', to: '/inventory/stock-inquiry', visible: true },
            { labelKey: 'nav.stockAdjustment', to: '/inventory/stock-adjustment', visible: true },
            { labelKey: 'nav.checkStock', to: '/inventory/check-stock', visible: true },
            { labelKey: 'nav.replenishment', to: '/inventory/replenishment', visible: this.canViewReplenishment },
            { labelKey: 'nav.relocation', to: '/inventory/relocation', visible: this.canViewRelocation }
          ]
        },
        {
          key: 'outbound',
          titleKey: 'nav.sectionOutbound',
          items: [
            { labelKey: 'nav.orders', to: '/outbound/orders', visible: true },
            { labelKey: 'nav.picking', to: '/outbound/picking', visible: true },
            { labelKey: 'nav.pickingBatches', to: '/outbound/picking-batches', visible: this.canViewPickingBatches },
            { labelKey: 'nav.packing', to: '/outbound/packing', visible: this.canViewPacking },
            { labelKey: 'nav.shipping', to: '/outbound/shipping', visible: true },
            { labelKey: 'nav.loading', to: '/outbound/loading', visible: this.canViewLoading }
          ]
        },
        {
          key: 'masterData',
          titleKey: 'nav.sectionMasterData',
          items: [
            { labelKey: 'nav.warehouse', to: '/administration/warehouse', visible: this.canViewWarehouses },
            { labelKey: 'nav.locations', to: '/administration/locations', visible: this.canViewWarehouses },
            { labelKey: 'nav.productMaster', to: '/administration/products', visible: this.canViewProducts },
            { labelKey: 'nav.productCategories', to: '/administration/product-categories', visible: this.canViewProductCategories },
            { labelKey: 'nav.brands', to: '/administration/brands', visible: this.canViewBrands },
            { labelKey: 'nav.productFamilies', to: '/administration/product-families', visible: this.canViewProductFamilies },
            { labelKey: 'nav.customers', to: '/administration/customers', visible: this.canViewCustomers },
            { labelKey: 'nav.suppliers', to: '/administration/suppliers', visible: this.canViewSuppliers }
          ]
        },
        {
          key: 'administration',
          titleKey: 'nav.sectionAdministration',
          items: [
            { labelKey: 'nav.userManagement', to: '/administration/user-management', visible: this.canViewUsers },
            { labelKey: 'nav.roles', to: '/administration/roles', visible: this.canViewRoles }
          ]
        },
        {
          key: 'platformAdmin',
          titleKey: 'nav.sectionPlatformAdmin',
          items: [
            { labelKey: 'nav.platformTenants', to: '/platform/tenants', visible: this.canViewPlatformAdmin }
          ]
        }
      ]
    },

    // RBAC gating happens here, before any search filtering is applied —
    // an item or section the caller is not authorized for is dropped
    // from this list entirely, so no search term can ever bring it back.
    authorizedSections() {
      return this.menuSections
        .map(section => ({ ...section, items: section.items.filter(item => item.visible) }))
        .filter(section => section.items.length > 0)
    },

    filteredSections() {
      const term = this.searchTerm
      if (!term) return this.authorizedSections

      const sections = []
      for (const section of this.authorizedSections) {
        const titleMatches = Boolean(section.titleKey) && this.$t(section.titleKey).toLowerCase().includes(term)
        const items = titleMatches
          ? section.items
          : section.items.filter(item => this.$t(item.labelKey).toLowerCase().includes(term))
        if (items.length > 0) sections.push({ ...section, items })
      }
      return sections
    }
  },

  methods: {
    clearSearch() {
      this.searchQuery = ''
      this.$nextTick(() => this.$refs.searchInput && this.$refs.searchInput.focus())
    }
  }
}
</script>

<style scoped>
.search-input::-ms-clear {
  display: none;
}

.clear-btn {
  font-size: 13px;
  line-height: 1;
  background: none;
  border: none;
  cursor: pointer;
  padding: 2px;
}

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

.menu-empty {
  margin: 16px 12px;
  color: #64748b;
  font-size: 13px;
  text-align: center;
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
