<template>
  <div class="space-y-6">
    <!-- Page Header -->
    <PageHeader :title="$t('warehouseSetup.title')" :subtitle="$t('warehouseSetup.subtitle')">
      <template #actions>
        <button
          @click="openWarehouse(null)"
          class="bg-orange-600 hover:bg-orange-700 text-white px-5 py-3 rounded-xl font-semibold"
        >
          {{ $t('warehouseSetup.newWarehouseButton') }}
        </button>
      </template>
    </PageHeader>

    <!-- Summary Cards -->
    <div class="grid grid-cols-1 md:grid-cols-4 gap-5">
      <div class="summary-card">
        <p class="summary-label">{{ $t('warehouseSetup.summary.totalWarehouses') }}</p>
        <h2 class="summary-value">{{ total }}</h2>
      </div>

      <div class="summary-card">
        <p class="summary-label">{{ $t('warehouseSetup.summary.totalZones') }}</p>
        <h2 class="summary-value">{{ totalZones }}</h2>
      </div>

      <div class="summary-card">
        <p class="summary-label">{{ $t('warehouseSetup.summary.totalLocations') }}</p>
        <h2 class="summary-value">{{ totalLocations }}</h2>
      </div>

      <div class="summary-card">
        <p class="summary-label">{{ $t('warehouseSetup.summary.avgUtilization') }}</p>
        <h2 class="summary-value text-orange-600">—</h2>
      </div>
    </div>

    <!-- Filter -->
    <FilterBar :cols="4">
      <input
        v-model="filters.keyword"
        type="text"
        :placeholder="$t('warehouseSetup.filters.searchPlaceholder')"
        class="input"
      />

      <select v-model="filters.type" class="input">
        <option value="">{{ $t('warehouseSetup.filters.allTypes') }}</option>
        <option value="Distribution Center">{{ $t('warehouseSetup.warehouseTypes.distributionCenter') }}</option>
        <option value="Factory Warehouse">{{ $t('warehouseSetup.warehouseTypes.factoryWarehouse') }}</option>
        <option value="Transit Warehouse">{{ $t('warehouseSetup.warehouseTypes.transitWarehouse') }}</option>
      </select>

      <select v-model="filters.status" class="input">
        <option value="">{{ $t('warehouseSetup.filters.allStatus') }}</option>
        <option value="ACTIVE">{{ $t('common.status.active') }}</option>
        <option value="INACTIVE">{{ $t('common.status.inactive') }}</option>
      </select>

      <button
        @click="clearFilter"
        class="bg-slate-100 hover:bg-slate-200 text-slate-700 rounded-xl font-semibold"
      >
        {{ $t('common.buttons.clearFilter') }}
      </button>
    </FilterBar>

    <!-- Warehouse Table -->
    <div v-if="listError" class="bg-red-50 border border-red-200 text-red-600 rounded-xl p-3 text-sm">
      {{ listError }}
    </div>

    <DataTable
      :loading="loading"
      :is-empty="warehouses.length === 0"
      :loading-text="$t('warehouseSetup.loadingWarehouses')"
      :empty-text="$t('warehouseSetup.noWarehouses')"
      :colspan="9"
      min-width="1150px"
    >
      <template #head>
        <tr>
          <SortableTh column="code" :sort-by="sortBy" :sort-dir="sortDir" @sort="toggleSort">
            {{ $t('warehouseSetup.table.code') }}
          </SortableTh>
          <SortableTh column="name" :sort-by="sortBy" :sort-dir="sortDir" @sort="toggleSort">
            {{ $t('warehouseSetup.table.name') }}
          </SortableTh>
          <SortableTh column="type" :sort-by="sortBy" :sort-dir="sortDir" @sort="toggleSort">
            {{ $t('warehouseSetup.table.type') }}
          </SortableTh>
          <th class="table-th">{{ $t('warehouseSetup.table.address') }}</th>
          <th class="table-th">{{ $t('warehouseSetup.table.zones') }}</th>
          <th class="table-th">{{ $t('warehouseSetup.table.locations') }}</th>
          <th class="table-th">{{ $t('warehouseSetup.table.utilization') }}</th>
          <SortableTh column="status" :sort-by="sortBy" :sort-dir="sortDir" @sort="toggleSort">
            {{ $t('warehouseSetup.table.status') }}
          </SortableTh>
          <th class="table-th text-right">{{ $t('warehouseSetup.table.action') }}</th>
        </tr>
      </template>

      <tr
        v-for="warehouse in warehouses"
        :key="warehouse.id"
        class="border-t hover:bg-orange-50"
      >
        <td class="table-td font-semibold text-slate-800">
          {{ warehouse.code }}
        </td>

        <td class="table-td">
          {{ warehouse.name }}
        </td>

        <td class="table-td">
          {{ warehouse.type }}
        </td>

        <td class="table-td">
          {{ warehouse.address }}
        </td>

        <td class="table-td">
          {{ warehouse.zones }}
        </td>

        <td class="table-td">
          {{ warehouse.locations }}
        </td>

        <td class="table-td">
          <div class="w-32">
            <div class="h-2 bg-slate-200 rounded-full overflow-hidden">
              <div
                class="h-2 bg-orange-600 rounded-full"
                :style="{ width: (warehouse.utilization ?? 0) + '%' }"
              ></div>
            </div>
            <p class="text-xs text-slate-500 mt-1">
              {{ warehouse.utilization ?? 0 }}%
            </p>
          </div>
        </td>

        <td class="table-td">
          <StatusBadge :tone="statusTone(warehouse.status)">
            {{ statusLabel(warehouse.status) }}
          </StatusBadge>
        </td>

        <td class="table-td text-right">
          <button
            @click="openWarehouse(warehouse)"
            class="text-orange-600 hover:text-orange-700 font-semibold"
          >
            {{ $t('warehouseSetup.manageButton') }}
          </button>
        </td>
      </tr>

      <template v-if="!loading && warehouses.length > 0" #footer>
        <PaginationFooter
          :page="page"
          :limit="limit"
          :total="total"
          :total-pages="totalPages"
          :page-size-options="pageSizeOptions"
          @update:page="goToPage"
          @update:limit="limit = $event"
        />
      </template>
    </DataTable>

    <!-- Warehouse Drawer -->
    <div
      v-if="selectedWarehouse"
      class="fixed inset-0 bg-black/40 flex justify-end z-50"
    >
      <div class="w-full max-w-4xl bg-white h-full p-6 overflow-y-auto">
        <div class="flex items-center justify-between mb-6">
          <div>
            <h2 class="text-xl font-bold text-slate-800">
              {{ selectedWarehouse.id ? $t('warehouseSetup.drawer.detailTitle') : $t('warehouseSetup.drawer.newTitle') }}
            </h2>
            <p class="text-slate-500">
              {{ $t('warehouseSetup.drawer.subtitle') }}
            </p>
          </div>

          <button
            @click="selectedWarehouse = null"
            class="text-slate-500 hover:text-red-600 text-xl"
          >
            ✕
          </button>
        </div>

        <!-- Tabs -->
        <div class="flex gap-2 mb-6 overflow-x-auto">
          <button
            v-for="tab in tabs"
            :key="tab.key"
            @click="activeTab = tab.key"
            class="tab-button"
            :class="activeTab === tab.key ? 'tab-active' : ''"
          >
            {{ $t(tab.labelKey) }}
          </button>
        </div>

        <!-- Warehouse Info -->
        <div v-if="activeTab === 'INFO'" class="space-y-4">
          <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div>
              <label class="label">{{ $t('warehouseSetup.infoTab.code') }}</label>
              <input v-model="warehouseForm.code" type="text" class="input" />
            </div>

            <div>
              <label class="label">{{ $t('warehouseSetup.infoTab.name') }}</label>
              <input v-model="warehouseForm.name" type="text" class="input" />
            </div>

            <div>
              <label class="label">{{ $t('warehouseSetup.infoTab.type') }}</label>
              <select v-model="warehouseForm.type" class="input">
                <option value="Distribution Center">{{ $t('warehouseSetup.warehouseTypes.distributionCenter') }}</option>
                <option value="Factory Warehouse">{{ $t('warehouseSetup.warehouseTypes.factoryWarehouse') }}</option>
                <option value="Transit Warehouse">{{ $t('warehouseSetup.warehouseTypes.transitWarehouse') }}</option>
              </select>
            </div>

            <div>
              <label class="label">{{ $t('warehouseSetup.infoTab.status') }}</label>
              <select v-model="warehouseForm.status" class="input">
                <option value="ACTIVE">{{ $t('common.status.active') }}</option>
                <option value="INACTIVE">{{ $t('common.status.inactive') }}</option>
              </select>
            </div>

            <div>
              <label class="label">{{ $t('warehouseSetup.infoTab.contactPerson') }}</label>
              <input v-model="warehouseForm.contactPerson" type="text" class="input" />
            </div>

            <div>
              <label class="label">{{ $t('warehouseSetup.infoTab.phone') }}</label>
              <input v-model="warehouseForm.phone" type="text" class="input" />
            </div>

            <div>
              <label class="label">{{ $t('warehouseSetup.infoTab.capacity') }}</label>
              <input v-model.number="warehouseForm.capacity" type="number" min="0" class="input" :placeholder="$t('warehouseSetup.infoTab.capacityPlaceholder')" />
            </div>
          </div>

          <div v-if="formError" class="bg-red-50 border border-red-200 text-red-600 rounded-xl p-3 text-sm">
            {{ formError }}
          </div>

          <div>
            <label class="label">{{ $t('warehouseSetup.infoTab.address') }}</label>
            <textarea v-model="warehouseForm.address" rows="4" class="input"></textarea>
          </div>

          <button
            @click="saveWarehouse"
            :disabled="saving"
            class="w-full bg-orange-600 hover:bg-orange-700 text-white py-3 rounded-xl font-semibold disabled:opacity-60"
          >
            {{ saving ? $t('warehouseSetup.saving') : $t('warehouseSetup.saveWarehouseButton') }}
          </button>
        </div>

        <!-- Zone Setup -->
        <div v-if="activeTab === 'ZONE'" class="space-y-4">
          <div v-if="!selectedWarehouse.id" class="text-slate-500">
            {{ $t('warehouseSetup.zoneTab.saveFirst') }}
          </div>

          <template v-else>
            <div class="flex justify-between items-center">
              <h3 class="font-bold text-slate-800">{{ $t('warehouseSetup.zoneTab.title') }}</h3>

              <button
                @click="addZone"
                class="bg-orange-600 hover:bg-orange-700 text-white px-4 py-2 rounded-xl font-semibold"
              >
                {{ $t('warehouseSetup.zoneTab.addZoneButton') }}
              </button>
            </div>

            <div
              v-for="zone in zones"
              :key="zone.id || zone.tempKey"
              class="border border-slate-200 rounded-2xl p-4"
            >
              <div class="grid grid-cols-1 md:grid-cols-4 gap-4">
                <input v-model="zone.code" class="input" :placeholder="$t('warehouseSetup.zoneTab.codePlaceholder')" />
                <input v-model="zone.name" class="input" :placeholder="$t('warehouseSetup.zoneTab.namePlaceholder')" />

                <select v-model="zone.type" class="input">
                  <option value="RECEIVING">{{ $t('warehouseSetup.zoneTypes.receiving') }}</option>
                  <option value="STORAGE">{{ $t('warehouseSetup.zoneTypes.storage') }}</option>
                  <option value="PICKING">{{ $t('warehouseSetup.zoneTypes.picking') }}</option>
                  <option value="PACKING">{{ $t('warehouseSetup.zoneTypes.packing') }}</option>
                  <option value="SHIPPING">{{ $t('warehouseSetup.zoneTypes.shipping') }}</option>
                  <option value="QC">{{ $t('warehouseSetup.zoneTypes.qc') }}</option>
                  <option value="HOLD">{{ $t('warehouseSetup.zoneTypes.hold') }}</option>
                </select>

                <select v-model="zone.status" class="input">
                  <option value="ACTIVE">{{ $t('common.status.active') }}</option>
                  <option value="INACTIVE">{{ $t('common.status.inactive') }}</option>
                </select>
              </div>
            </div>

            <div v-if="zoneError" class="bg-red-50 border border-red-200 text-red-600 rounded-xl p-3 text-sm">
              {{ zoneError }}
            </div>

            <button
              @click="saveZones"
              :disabled="savingZones"
              class="w-full bg-orange-600 hover:bg-orange-700 text-white py-3 rounded-xl font-semibold disabled:opacity-60"
            >
              {{ savingZones ? $t('warehouseSetup.saving') : $t('warehouseSetup.zoneTab.saveZonesButton') }}
            </button>
          </template>
        </div>

        <!-- Location Structure -->
        <div v-if="activeTab === 'LOCATION'" class="space-y-4">
          <div class="bg-slate-50 border border-slate-200 rounded-2xl p-5">
            <h3 class="font-bold text-slate-800 mb-4">{{ $t('warehouseSetup.locationTab.title') }}</h3>

            <div class="space-y-3">
              <div
                v-for="zone in locationTree"
                :key="zone.zoneCode"
                class="tree-zone"
              >
                <p class="font-bold text-orange-600">
                  {{ zone.zoneCode }} - {{ zone.zoneName }}
                </p>

                <div
                  v-for="aisle in zone.aisles"
                  :key="aisle.aisleCode"
                  class="ml-5 mt-3"
                >
                  <p class="font-semibold text-slate-700">
                    └─ {{ $t('warehouseSetup.locationTab.aislePrefix') }} {{ aisle.aisleCode }}
                  </p>

                  <div
                    v-for="rack in aisle.racks"
                    :key="rack.rackCode"
                    class="ml-5 mt-2"
                  >
                    <p class="text-slate-600">
                      └─ {{ $t('warehouseSetup.locationTab.rackPrefix') }} {{ rack.rackCode }}
                    </p>

                    <div class="ml-5 mt-2 grid grid-cols-2 md:grid-cols-4 gap-2">
                      <button
                        v-for="bin in rack.bins"
                        :key="bin"
                        class="bin-button"
                      >
                        {{ bin }}
                      </button>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </div>

          <button
            @click="generateLocations"
            class="bg-orange-600 hover:bg-orange-700 text-white px-5 py-3 rounded-xl font-semibold"
          >
            {{ $t('warehouseSetup.locationTab.generateButton') }}
          </button>
        </div>

        <!-- Capacity Dashboard -->
        <div v-if="activeTab === 'CAPACITY'" class="space-y-5">
          <div class="bg-orange-50 border border-orange-200 rounded-2xl p-6">
            <p class="text-slate-500">{{ $t('warehouseSetup.capacityTab.utilizationTitle') }}</p>
            <h2 class="text-3xl font-bold text-orange-600 mt-2 tabular-nums">
              {{ warehouseForm.utilization ?? 0 }}%
            </h2>

            <div class="h-3 bg-orange-200 rounded-full mt-5 overflow-hidden">
              <div
                class="h-3 bg-orange-600 rounded-full"
                :style="{ width: (warehouseForm.utilization ?? 0) + '%' }"
              ></div>
            </div>

            <!-- Occupied/Available require real inventory quantities, which
                 don't exist yet (see PROJECT_STATUS.md) — utilization is
                 not computable this phase, so it always shows 0% here. -->
          </div>

          <div class="grid grid-cols-1 md:grid-cols-3 gap-4">
            <div class="summary-card">
              <p class="summary-label">{{ $t('warehouseSetup.capacityTab.totalCapacity') }}</p>
              <h2 class="summary-value">{{ warehouseForm.capacity ?? '—' }}</h2>
            </div>

            <div class="summary-card">
              <p class="summary-label">{{ $t('warehouseSetup.capacityTab.occupied') }}</p>
              <h2 class="summary-value">—</h2>
            </div>

            <div class="summary-card">
              <p class="summary-label">{{ $t('warehouseSetup.capacityTab.available') }}</p>
              <h2 class="summary-value">—</h2>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script>
import {
  listWarehouses, createWarehouse, updateWarehouse,
  listZones, createZone, updateZone,
  listLocations
} from '../../services/warehouses'
import { showToast } from '../../services/toast.js'
import i18n from '../../i18n'
import PageHeader from '../../components/ui/PageHeader.vue'
import FilterBar from '../../components/ui/FilterBar.vue'
import DataTable from '../../components/ui/DataTable.vue'
import StatusBadge from '../../components/ui/StatusBadge.vue'
import SortableTh from '../../components/ui/SortableTh.vue'
import PaginationFooter from '../../components/ui/PaginationFooter.vue'
import listPagination from '../../mixins/listPagination'

let searchDebounce = null
let zoneTempCounter = 0

export default {
  name: 'WarehouseView',

  components: { PageHeader, FilterBar, DataTable, StatusBadge, SortableTh, PaginationFooter },

  mixins: [listPagination],

  data() {
    return {
      selectedWarehouse: null,
      activeTab: 'INFO',
      loading: false,
      listError: '',
      saving: false,
      formError: '',
      savingZones: false,
      zoneError: '',

      filters: {
        keyword: '',
        type: '',
        status: ''
      },

      sortBy: 'code',
      sortDir: 'asc',

      tabs: [
        { key: 'INFO', labelKey: 'warehouseSetup.tabs.info' },
        { key: 'ZONE', labelKey: 'warehouseSetup.tabs.zone' },
        { key: 'LOCATION', labelKey: 'warehouseSetup.tabs.location' },
        { key: 'CAPACITY', labelKey: 'warehouseSetup.tabs.capacity' }
      ],

      warehouseForm: {
        code: '',
        name: '',
        type: 'Distribution Center',
        address: '',
        contactPerson: '',
        phone: '',
        capacity: null,
        status: 'ACTIVE',
        utilization: 0
      },

      warehouses: [],
      zones: [],
      locationTree: []
    }
  },

  watch: {
    'filters.type'() {
      this.resetToFirstPage()
    },
    'filters.status'() {
      this.resetToFirstPage()
    },
    'filters.keyword'() {
      clearTimeout(searchDebounce)
      searchDebounce = setTimeout(() => this.resetToFirstPage(), 300)
    },
    activeTab(tab) {
      if (!this.selectedWarehouse?.id) return
      if (tab === 'ZONE') this.fetchZones()
      if (tab === 'LOCATION') this.loadLocationTree()
    }
  },

  computed: {
    totalZones() {
      return this.warehouses.reduce((sum, w) => sum + (w.zones || 0), 0)
    },
    totalLocations() {
      return this.warehouses.reduce((sum, w) => sum + (w.locations || 0), 0)
    }
  },

  mounted() {
    this.fetchWarehouses()
  },

  methods: {
    // fetchList() is the listPagination mixin's required hook —
    // goToPage/resetToFirstPage/the limit watcher all call it. Kept as
    // a thin alias over fetchWarehouses() (the pre-existing public name
    // other methods here already call directly) rather than renaming
    // every call site.
    fetchList() {
      return this.fetchWarehouses()
    },

    async fetchWarehouses() {
      this.loading = true
      this.listError = ''
      try {
        const res = await listWarehouses({
          search: this.filters.keyword,
          type: this.filters.type,
          status: this.filters.status,
          page: this.page,
          limit: this.limit,
          sortBy: this.sortBy,
          sortDir: this.sortDir
        })
        this.warehouses = res.data
        this.total = res.meta.total
        this.totalPages = res.meta.total_pages
      } catch (err) {
        this.listError = err.message || this.$t('warehouseSetup.messages.loadFailed')
      } finally {
        this.loading = false
      }
    },

    openWarehouse(warehouse) {
      this.formError = ''
      this.zoneError = ''
      this.zones = []
      this.locationTree = []
      this.selectedWarehouse = warehouse || { id: null }
      this.activeTab = 'INFO'

      this.warehouseForm = warehouse
        ? {
            code: warehouse.code,
            name: warehouse.name,
            type: warehouse.type,
            address: warehouse.address,
            contactPerson: warehouse.contact_person || '',
            phone: warehouse.phone || '',
            capacity: warehouse.capacity,
            status: warehouse.status,
            utilization: warehouse.utilization ?? 0
          }
        : {
            code: '',
            name: '',
            type: 'Distribution Center',
            address: '',
            contactPerson: '',
            phone: '',
            capacity: null,
            status: 'ACTIVE',
            utilization: 0
          }
    },

    clearFilter() {
      this.filters.keyword = ''
      this.filters.type = ''
      this.filters.status = ''
    },

    async saveWarehouse() {
      this.formError = ''
      this.saving = true

      const payload = {
        code: this.warehouseForm.code,
        name: this.warehouseForm.name,
        type: this.warehouseForm.type,
        address: this.warehouseForm.address,
        contact_person: this.warehouseForm.contactPerson,
        phone: this.warehouseForm.phone,
        capacity: this.warehouseForm.capacity,
        status: this.warehouseForm.status
      }

      try {
        if (this.selectedWarehouse.id) {
          await updateWarehouse(this.selectedWarehouse.id, payload)
        } else {
          await createWarehouse(payload)
        }

        this.selectedWarehouse = null
        await this.fetchWarehouses()
      } catch (err) {
        this.formError = err.message || this.$t('warehouseSetup.messages.saveFailed')
      } finally {
        this.saving = false
      }
    },

    async fetchZones() {
      this.zoneError = ''
      try {
        const res = await listZones(this.selectedWarehouse.id)
        this.zones = res.data
      } catch (err) {
        this.zoneError = err.message || this.$t('warehouseSetup.messages.loadZonesFailed')
      }
    },

    addZone() {
      zoneTempCounter += 1
      this.zones.push({
        id: null,
        tempKey: `new-${zoneTempCounter}`,
        code: '',
        name: '',
        type: 'STORAGE',
        status: 'ACTIVE'
      })
    },

    // Saves every row in this.zones — new rows (no id) via POST,
    // existing rows via PUT. The Zone Setup tab has no per-row save
    // affordance of its own (the mock never persisted zones at all), so
    // one "Save Zones" button covers the whole list, the same way
    // "Save Warehouse" covers the whole Info tab.
    async saveZones() {
      this.zoneError = ''
      this.savingZones = true

      try {
        for (const z of this.zones) {
          const payload = { code: z.code, name: z.name, type: z.type, status: z.status }
          if (z.id) {
            await updateZone(z.id, payload)
          } else {
            const res = await createZone({ warehouse_id: this.selectedWarehouse.id, ...payload })
            Object.assign(z, res.data)
          }
        }
        await this.fetchWarehouses()
      } catch (err) {
        this.zoneError = err.message || this.$t('warehouseSetup.messages.saveZonesFailed')
      } finally {
        this.savingZones = false
      }
    },

    // Builds the zone -> aisle -> rack -> bin tree from real locations.
    // Bulk-generating locations from an aisle/rack/bin count (the
    // "Generate Bin Locations" button below) is not implemented this
    // phase — see PROJECT_STATUS.md — so the tree only shows locations
    // created individually via the API.
    async loadLocationTree() {
      if (!this.selectedWarehouse?.id) return
      if (this.zones.length === 0) await this.fetchZones()

      const tree = []
      for (const z of this.zones) {
        const res = await listLocations(z.id)
        const aislesMap = {}

        res.data.forEach((loc) => {
          const aisleKey = loc.aisle || '—'
          const rackKey = loc.rack || '—'
          if (!aislesMap[aisleKey]) aislesMap[aisleKey] = {}
          if (!aislesMap[aisleKey][rackKey]) aislesMap[aisleKey][rackKey] = []
          aislesMap[aisleKey][rackKey].push(loc.code)
        })

        const aisles = Object.keys(aislesMap).sort().map((aisleCode) => ({
          aisleCode,
          racks: Object.keys(aislesMap[aisleCode]).sort().map((rackCode) => ({
            rackCode,
            bins: aislesMap[aisleCode][rackCode]
          }))
        }))

        tree.push({ zoneCode: z.code, zoneName: z.name, aisles })
      }

      this.locationTree = tree
    },

    generateLocations() {
      showToast(this.$t('warehouseSetup.messages.binsGenerated'), 'success')
    },

    statusTone(status) {
      if (status === 'ACTIVE') return 'green'
      if (status === 'INACTIVE') return 'gray'
      return 'gray'
    },

    statusLabel(status) {
      if (status === 'ACTIVE') return this.$t('common.status.active')
      if (status === 'INACTIVE') return this.$t('common.status.inactive')
      return status
    }
  }
}
</script>

<style scoped>
.input {
  width: 100%;
  border: 1px solid #cbd5e1;
  border-radius: 12px;
  padding: 11px 14px;
  outline: none;
}

.input:focus {
  border-color: #ea580c;
  box-shadow: 0 0 0 2px #fed7aa;
}

.label {
  display: block;
  margin-bottom: 8px;
  color: #334155;
  font-weight: var(--fw-label);
  font-size: var(--fs-label);
}

.summary-card {
  background: white;
  border: 1px solid #e2e8f0;
  border-radius: 18px;
  padding: 20px;
}

.summary-label {
  color: #64748b;
  font-size: 14px;
}

.summary-value {
  font-size: var(--fs-kpi);
  font-weight: var(--fw-kpi);
  color: #0f172a;
  margin-top: 8px;
  font-variant-numeric: tabular-nums;
}

.table-th {
  padding: 14px 16px;
  font-weight: var(--fw-table-header);
  font-size: var(--fs-table-header);
  white-space: nowrap;
}

.table-td {
  padding: 14px 16px;
  color: #475569;
  white-space: nowrap;
}

.tab-button {
  padding: 10px 16px;
  border-radius: 12px;
  background: #f1f5f9;
  color: #475569;
  font-weight: 600;
  white-space: nowrap;
}

.tab-active {
  background: #ea580c;
  color: white;
}

.tree-zone {
  background: white;
  border: 1px solid #e2e8f0;
  border-radius: 16px;
  padding: 16px;
}

.bin-button {
  background: #fff7ed;
  color: #ea580c;
  border: 1px solid #fed7aa;
  padding: 8px 10px;
  border-radius: 10px;
  font-weight: 600;
  font-size: 12px;
}

.bin-button:hover {
  background: #ea580c;
  color: white;
}
</style>