<template>
  <div class="space-y-6">
    <PageHeader :title="$t('locationMaster.title')" :subtitle="$t('locationMaster.subtitle')">
      <template #actions>
        <button
          v-if="selectedZoneId"
          @click="openLocation(null)"
          class="bg-orange-600 hover:bg-orange-700 text-white px-5 py-3 rounded-xl font-semibold"
        >
          {{ $t('locationMaster.newButton') }}
        </button>
      </template>
    </PageHeader>

    <!-- Warehouse -> Zone cascading selection. The backend has no
         "list all locations" endpoint (zone_id is a required query
         param), so a zone must always be picked before anything else on
         this screen becomes usable. -->
    <FilterBar :cols="2">
      <select v-model="selectedWarehouseId" class="input">
        <option value="">{{ $t('locationMaster.selectors.warehousePlaceholder') }}</option>
        <option v-for="wh in warehouses" :key="wh.id" :value="wh.id">
          {{ wh.code }} - {{ wh.name }}
        </option>
      </select>

      <select v-model="selectedZoneId" class="input" :disabled="!selectedWarehouseId">
        <option value="">
          {{ selectedWarehouseId ? $t('locationMaster.selectors.zonePlaceholder') : $t('locationMaster.selectors.zoneDisabledPlaceholder') }}
        </option>
        <option v-for="z in zones" :key="z.id" :value="z.id">
          {{ z.code }} - {{ z.name }}
        </option>
      </select>
    </FilterBar>

    <div v-if="!selectedZoneId" class="empty-prompt">
      {{ selectedWarehouseId ? $t('locationMaster.prompts.selectZone') : $t('locationMaster.prompts.selectWarehouse') }}
    </div>

    <template v-else>
      <FilterBar :cols="3">
        <input
          v-model="filters.keyword"
          type="text"
          class="input"
          :placeholder="$t('locationMaster.filters.searchPlaceholder')"
        />

        <select v-model="filters.status" class="input">
          <option value="">{{ $t('locationMaster.filters.allStatus') }}</option>
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

      <div v-if="listError" class="bg-red-50 border border-red-200 text-red-600 rounded-xl p-3 text-sm">
        {{ listError }}
      </div>

      <DataTable
        :loading="loading"
        :is-empty="locations.length === 0"
        :loading-text="$t('locationMaster.loading')"
        :empty-text="$t('locationMaster.empty')"
        :colspan="7"
        min-width="1000px"
      >
        <template #head>
          <tr>
            <th class="table-th">{{ $t('locationMaster.table.code') }}</th>
            <th class="table-th">{{ $t('locationMaster.table.name') }}</th>
            <th class="table-th">{{ $t('locationMaster.table.position') }}</th>
            <th class="table-th">{{ $t('locationMaster.table.type') }}</th>
            <th class="table-th">{{ $t('locationMaster.table.capacity') }}</th>
            <th class="table-th">{{ $t('locationMaster.table.status') }}</th>
            <th class="table-th text-right">{{ $t('locationMaster.table.action') }}</th>
          </tr>
        </template>

        <tr v-for="loc in locations" :key="loc.id" class="border-t hover:bg-orange-50">
          <td class="table-td font-semibold text-slate-800">{{ loc.code }}</td>
          <td class="table-td">{{ loc.name || '—' }}</td>
          <td class="table-td">{{ positionLabel(loc) }}</td>
          <td class="table-td">{{ typeLabel(loc.type) }}</td>
          <td class="table-td">{{ loc.capacity ?? '—' }}</td>
          <td class="table-td">
            <StatusBadge :tone="statusTone(loc.status)">
              {{ statusLabel(loc.status) }}
            </StatusBadge>
          </td>
          <td class="table-td text-right space-x-3">
            <button
              @click="openLocation(loc)"
              class="text-orange-600 hover:text-orange-700 font-semibold"
            >
              {{ $t('common.buttons.edit') }}
            </button>
            <button
              @click="askDelete(loc)"
              class="text-red-600 hover:text-red-700 font-semibold"
            >
              {{ $t('common.buttons.delete') }}
            </button>
          </td>
        </tr>
      </DataTable>
    </template>

    <div
      v-if="selectedLocation"
      class="fixed inset-0 bg-black/40 flex justify-end z-50"
    >
      <div class="w-full max-w-2xl bg-white h-full p-6 overflow-y-auto">
        <div class="flex items-center justify-between mb-6">
          <div>
            <h2 class="text-xl font-bold text-slate-800">
              {{ selectedLocation.id ? $t('locationMaster.drawer.detailTitle') : $t('locationMaster.drawer.newTitle') }}
            </h2>
            <p class="text-slate-500">
              {{ $t('locationMaster.drawer.subtitle') }}
            </p>
          </div>

          <button
            @click="selectedLocation = null"
            class="text-slate-500 hover:text-red-600 text-xl"
          >
            ✕
          </button>
        </div>

        <FormSection>
          <div>
            <label class="label">{{ $t('locationMaster.fields.zone') }}</label>
            <input type="text" class="input" disabled :value="zoneDisplayLabel" />
          </div>

          <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div>
              <label class="label">{{ $t('locationMaster.fields.code') }}</label>
              <input v-model="locationForm.code" type="text" class="input" />
            </div>

            <div>
              <label class="label">{{ $t('locationMaster.fields.name') }}</label>
              <input v-model="locationForm.name" type="text" class="input" />
            </div>

            <div>
              <label class="label">{{ $t('locationMaster.fields.aisle') }}</label>
              <input v-model="locationForm.aisle" type="text" class="input" />
            </div>

            <div>
              <label class="label">{{ $t('locationMaster.fields.rack') }}</label>
              <input v-model="locationForm.rack" type="text" class="input" />
            </div>

            <div>
              <label class="label">{{ $t('locationMaster.fields.bin') }}</label>
              <input v-model="locationForm.bin" type="text" class="input" />
            </div>

            <div>
              <label class="label">{{ $t('locationMaster.fields.type') }}</label>
              <select v-model="locationForm.type" class="input">
                <option value="">{{ $t('locationMaster.fields.noTypeOption') }}</option>
                <option value="RECEIVING">{{ $t('warehouseSetup.zoneTypes.receiving') }}</option>
                <option value="STORAGE">{{ $t('warehouseSetup.zoneTypes.storage') }}</option>
                <option value="PICKING">{{ $t('warehouseSetup.zoneTypes.picking') }}</option>
                <option value="PACKING">{{ $t('warehouseSetup.zoneTypes.packing') }}</option>
                <option value="SHIPPING">{{ $t('warehouseSetup.zoneTypes.shipping') }}</option>
                <option value="QC">{{ $t('warehouseSetup.zoneTypes.qc') }}</option>
                <option value="HOLD">{{ $t('warehouseSetup.zoneTypes.hold') }}</option>
              </select>
            </div>

            <div>
              <label class="label">{{ $t('locationMaster.fields.capacity') }}</label>
              <input
                v-model="locationForm.capacity"
                type="number"
                min="0"
                class="input"
                :placeholder="$t('locationMaster.fields.capacityPlaceholder')"
              />
            </div>

            <div>
              <label class="label">{{ $t('locationMaster.fields.status') }}</label>
              <select v-model="locationForm.status" class="input">
                <option value="ACTIVE">{{ $t('common.status.active') }}</option>
                <option value="INACTIVE">{{ $t('common.status.inactive') }}</option>
              </select>
            </div>
          </div>
        </FormSection>

        <div v-if="formError" class="bg-red-50 border border-red-200 text-red-600 rounded-xl p-3 text-sm mb-6">
          {{ formError }}
        </div>

        <div class="flex flex-col md:flex-row justify-end gap-3">
          <button
            @click="selectedLocation = null"
            class="bg-white hover:bg-slate-100 text-slate-700 border border-slate-300 px-6 py-3 rounded-xl font-semibold"
          >
            {{ $t('common.buttons.cancel') }}
          </button>

          <button
            @click="saveLocation"
            :disabled="saving"
            class="bg-orange-600 hover:bg-orange-700 text-white px-6 py-3 rounded-xl font-semibold disabled:opacity-60"
          >
            {{ saving ? $t('common.buttons.submitting') : $t('common.buttons.save') }}
          </button>
        </div>
      </div>
    </div>

    <ConfirmDialog
      :open="!!locationToDelete"
      :title="$t('locationMaster.deleteTitle')"
      :message="$t('common.messages.deleteConfirm')"
      :confirm-label="$t('common.buttons.delete')"
      :cancel-label="$t('common.buttons.cancel')"
      :loading-label="$t('common.buttons.submitting')"
      danger
      :loading="deleting"
      :error-message="deleteError"
      @cancel="locationToDelete = null; deleteError = ''"
      @confirm="confirmDelete"
    />
  </div>
</template>

<script>
import {
  listWarehouses,
  listZones,
  listLocations,
  createLocation,
  updateLocation,
  deleteLocation
} from '../../services/warehouses'
import { showToast } from '../../services/toast'
import PageHeader from '../../components/ui/PageHeader.vue'
import FilterBar from '../../components/ui/FilterBar.vue'
import DataTable from '../../components/ui/DataTable.vue'
import StatusBadge from '../../components/ui/StatusBadge.vue'
import FormSection from '../../components/ui/FormSection.vue'
import ConfirmDialog from '../../components/ui/ConfirmDialog.vue'

let searchDebounce = null

// The 7 location types share the exact same vocabulary as warehouse
// zone types (RECEIVING/STORAGE/PICKING/PACKING/SHIPPING/QC/HOLD), so
// the option/display labels reuse warehouseSetup.zoneTypes.* rather
// than duplicating an identical set of translations under a new
// locationMaster.types.* namespace.
const TYPE_LABEL_KEYS = {
  RECEIVING: 'warehouseSetup.zoneTypes.receiving',
  STORAGE: 'warehouseSetup.zoneTypes.storage',
  PICKING: 'warehouseSetup.zoneTypes.picking',
  PACKING: 'warehouseSetup.zoneTypes.packing',
  SHIPPING: 'warehouseSetup.zoneTypes.shipping',
  QC: 'warehouseSetup.zoneTypes.qc',
  HOLD: 'warehouseSetup.zoneTypes.hold'
}

export default {
  name: 'LocationView',

  components: { PageHeader, FilterBar, DataTable, StatusBadge, FormSection, ConfirmDialog },

  data() {
    return {
      warehouses: [],
      zones: [],
      selectedWarehouseId: '',
      selectedZoneId: '',
      locations: [],
      loading: false,
      listError: '',
      saving: false,
      formError: '',
      selectedLocation: null,
      locationForm: this.createEmptyForm(),
      locationToDelete: null,
      deleting: false,
      deleteError: '',
      filters: {
        keyword: '',
        status: ''
      }
    }
  },

  computed: {
    // All locations visible at any time belong to selectedZoneId, so the
    // same warehouse/zone pair is valid for both the "new" form and any
    // row being edited.
    zoneDisplayLabel() {
      const wh = this.warehouses.find((w) => w.id === this.selectedWarehouseId)
      const zone = this.zones.find((z) => z.id === this.selectedZoneId)
      if (!wh || !zone) return ''
      return `${wh.code} - ${wh.name} / ${zone.code} - ${zone.name}`
    }
  },

  watch: {
    selectedWarehouseId() {
      this.selectedZoneId = ''
      this.zones = []
      this.locations = []
      if (this.selectedWarehouseId) this.fetchZones()
    },
    selectedZoneId() {
      this.locations = []
      if (this.selectedZoneId) this.fetchLocations()
    },
    'filters.status'() {
      if (this.selectedZoneId) this.fetchLocations()
    },
    'filters.keyword'() {
      clearTimeout(searchDebounce)
      searchDebounce = setTimeout(() => {
        if (this.selectedZoneId) this.fetchLocations()
      }, 300)
    }
  },

  mounted() {
    this.fetchWarehouses()
  },

  methods: {
    createEmptyForm() {
      return {
        code: '',
        name: '',
        aisle: '',
        rack: '',
        bin: '',
        type: '',
        capacity: '',
        status: 'ACTIVE'
      }
    },

    async fetchWarehouses() {
      try {
        const res = await listWarehouses({ limit: 200 })
        this.warehouses = res.data
      } catch (err) {
        this.listError = err.message || this.$t('locationMaster.messages.loadWarehousesFailed')
      }
    },

    async fetchZones() {
      try {
        const res = await listZones(this.selectedWarehouseId)
        this.zones = res.data
      } catch (err) {
        this.listError = err.message || this.$t('locationMaster.messages.loadZonesFailed')
      }
    },

    async fetchLocations() {
      this.loading = true
      this.listError = ''
      try {
        const res = await listLocations(this.selectedZoneId, {
          search: this.filters.keyword,
          status: this.filters.status,
          limit: 500
        })
        this.locations = res.data
      } catch (err) {
        this.listError = err.message || this.$t('locationMaster.messages.loadFailed')
      } finally {
        this.loading = false
      }
    },

    openLocation(location) {
      this.formError = ''
      this.selectedLocation = location || {}

      this.locationForm = location
        ? {
            code: location.code,
            name: location.name || '',
            aisle: location.aisle || '',
            rack: location.rack || '',
            bin: location.bin || '',
            type: location.type || '',
            capacity: location.capacity === null || location.capacity === undefined ? '' : location.capacity,
            status: location.status
          }
        : this.createEmptyForm()
    },

    clearFilter() {
      this.filters.keyword = ''
      this.filters.status = ''
    },

    async saveLocation() {
      this.formError = ''

      if (!this.locationForm.code) {
        this.formError = this.$t('common.messages.requiredField')
        return
      }

      this.saving = true
      const payload = {
        code: this.locationForm.code,
        name: this.locationForm.name || '',
        aisle: this.locationForm.aisle || '',
        rack: this.locationForm.rack || '',
        bin: this.locationForm.bin || '',
        type: this.locationForm.type || '',
        capacity:
          this.locationForm.capacity === '' || this.locationForm.capacity === null
            ? null
            : Number(this.locationForm.capacity),
        status: this.locationForm.status
      }

      try {
        if (this.selectedLocation.id) {
          // zone_id is deliberately omitted — the backend contract does
          // not allow moving a location to another zone via update.
          await updateLocation(this.selectedLocation.id, payload)
        } else {
          await createLocation({ zone_id: this.selectedZoneId, ...payload })
        }

        this.selectedLocation = null
        showToast(this.$t('common.messages.saveSuccess'), 'success')
        await this.fetchLocations()
      } catch (err) {
        this.formError = err.message || this.$t('locationMaster.messages.saveFailed')
      } finally {
        this.saving = false
      }
    },

    askDelete(location) {
      this.deleteError = ''
      this.locationToDelete = location
    },

    async confirmDelete() {
      if (!this.locationToDelete) return
      this.deleting = true
      this.deleteError = ''

      try {
        await deleteLocation(this.locationToDelete.id)
        this.locationToDelete = null
        showToast(this.$t('common.messages.deleteSuccess'), 'success')
        await this.fetchLocations()
      } catch (err) {
        // 409 HAS_STOCK lands here — the server's own message is shown
        // as-is rather than retried automatically.
        this.deleteError = err.message || this.$t('locationMaster.messages.deleteFailed')
      } finally {
        this.deleting = false
      }
    },

    positionLabel(location) {
      const parts = [location.aisle, location.rack, location.bin].filter(Boolean)
      return parts.length ? parts.join(' / ') : '—'
    },

    typeLabel(type) {
      const key = TYPE_LABEL_KEYS[type]
      return key ? this.$t(key) : '—'
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

.input:disabled {
  background: #f8fafc;
  color: #64748b;
}

.label {
  display: block;
  margin-bottom: 8px;
  color: #334155;
  font-weight: var(--fw-label);
  font-size: var(--fs-label);
}

.table-th {
  padding: 14px 16px;
  font-weight: var(--fw-table-header);
  font-size: var(--fs-table-header);
  background: #f8fafc;
  border-bottom: 1px solid #e2e8f0;
  color: #475569;
  white-space: nowrap;
}

.table-td {
  padding: 14px 16px;
  border-bottom: 1px solid #f1f5f9;
  vertical-align: middle;
  color: #475569;
}

.empty-prompt {
  background: #fff;
  border: 1px solid #e2e8f0;
  border-radius: 16px;
  padding: 48px 24px;
  text-align: center;
  color: #94a3b8;
}
</style>
