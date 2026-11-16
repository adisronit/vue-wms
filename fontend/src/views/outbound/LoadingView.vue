<template>
  <div class="space-y-6">
    <PageHeader :title="$t('loading.title')" :subtitle="$t('loading.subtitle')">
      <template #actions>
        <button
          v-if="canCreate"
          @click="openCreateModal"
          class="bg-orange-600 hover:bg-orange-700 text-white px-5 py-3 rounded-xl font-semibold"
        >
          {{ $t('loading.newTripButton') }}
        </button>
      </template>
    </PageHeader>

    <div class="grid grid-cols-1 md:grid-cols-4 gap-5">
      <div class="summary-card">
        <p class="summary-label">{{ $t('loading.summary.planned') }}</p>
        <h2 class="summary-value">{{ plannedCount }}</h2>
      </div>
      <div class="summary-card">
        <p class="summary-label">{{ $t('loading.summary.loading') }}</p>
        <h2 class="summary-value text-orange-600">{{ loadingCount }}</h2>
      </div>
      <div class="summary-card">
        <p class="summary-label">{{ $t('loading.summary.completed') }}</p>
        <h2 class="summary-value text-green-600">{{ completedCount }}</h2>
      </div>
      <div class="summary-card">
        <p class="summary-label">{{ $t('loading.summary.cancelled') }}</p>
        <h2 class="summary-value">{{ cancelledCount }}</h2>
      </div>
    </div>

    <div v-if="loadError" class="bg-red-50 border border-red-200 text-red-600 rounded-xl p-3 text-sm">
      {{ loadError }}
    </div>

    <FilterBar :cols="3">
      <input v-model="filters.search" type="text" :placeholder="$t('loading.filters.searchPlaceholder')" class="input" />

      <select v-model="filters.status" class="input">
        <option value="">{{ $t('loading.filters.allStatus') }}</option>
        <option value="PLANNED">{{ $t('loading.statusLabels.planned') }}</option>
        <option value="LOADING">{{ $t('loading.statusLabels.loading') }}</option>
        <option value="COMPLETED">{{ $t('common.status.completed') }}</option>
        <option value="CANCELLED">{{ $t('common.status.cancelled') }}</option>
      </select>

      <button @click="clearFilter" class="bg-slate-100 hover:bg-slate-200 text-slate-700 rounded-xl font-semibold">
        {{ $t('common.buttons.clearFilter') }}
      </button>
    </FilterBar>

    <DataTable
      :loading="loading"
      :is-empty="trips.length === 0"
      :loading-text="$t('loading.loadingTrips')"
      :empty-text="$t('loading.noTrips')"
      :colspan="8"
      min-width="1000px"
    >
      <template #head>
        <tr>
          <th class="table-th">{{ $t('loading.table.tripNo') }}</th>
          <th class="table-th">{{ $t('loading.table.vehicleNo') }}</th>
          <th class="table-th">{{ $t('loading.table.driver') }}</th>
          <th class="table-th text-right">{{ $t('loading.table.shipments') }}</th>
          <th class="table-th">{{ $t('loading.table.progress') }}</th>
          <th class="table-th">{{ $t('common.labels.status') }}</th>
          <th class="table-th">{{ $t('loading.table.scheduled') }}</th>
          <th class="table-th text-right">{{ $t('packing.table.action') }}</th>
        </tr>
      </template>

      <tr v-for="trip in trips" :key="trip.id" class="border-t hover:bg-orange-50">
        <td class="table-td font-semibold text-slate-800">{{ trip.trip_no }}</td>
        <td class="table-td">{{ trip.vehicle_no || '—' }}</td>
        <td class="table-td">{{ trip.driver_name || '—' }}</td>
        <td class="table-td text-right">{{ trip.shipment_count }}</td>
        <td class="table-td">{{ trip.loaded_count }}/{{ trip.planned_count }}</td>
        <td class="table-td"><StatusBadge :tone="statusTone(trip.status)">{{ statusLabel(trip.status) }}</StatusBadge></td>
        <td class="table-td">{{ formatDateTime(trip.scheduled_loading_time) }}</td>
        <td class="table-td text-right">
          <button @click="$router.push(`/outbound/loading/${trip.id}`)" class="text-orange-600 hover:underline font-semibold">
            {{ $t('common.buttons.view') }}
          </button>
        </td>
      </tr>
    </DataTable>

    <!-- Create Loading Trip modal -->
    <div v-if="showCreateModal" class="fixed inset-0 bg-black/30 flex items-center justify-center z-50 p-4">
      <div class="bg-white rounded-2xl shadow-xl w-full max-w-xl p-6 space-y-4 max-h-[90vh] overflow-y-auto">
        <div class="flex items-center justify-between">
          <h2 class="text-lg font-bold text-slate-800">{{ $t('loading.create.title') }}</h2>
          <button @click="showCreateModal = false" class="text-slate-400 hover:text-slate-600">✕</button>
        </div>

        <p v-if="createError" class="bg-red-50 border border-red-200 text-red-600 rounded-xl p-3 text-sm">{{ createError }}</p>

        <div class="grid grid-cols-1 md:grid-cols-2 gap-3">
          <input v-model="createForm.vehicleNo" type="text" class="input" :placeholder="$t('loading.create.vehicleNoPlaceholder')" />
          <input v-model="createForm.driverName" type="text" class="input" :placeholder="$t('loading.create.driverNamePlaceholder')" />
          <select v-model="createForm.carrier" class="input">
            <option value="">{{ $t('loading.create.carrierPlaceholder') }}</option>
            <option value="DHL">DHL</option>
            <option value="KERRY">KERRY</option>
            <option value="FLASH">FLASH</option>
            <option value="INTERNAL">INTERNAL</option>
          </select>
          <input v-model="createForm.loadingDock" type="text" class="input" :placeholder="$t('loading.create.dockPlaceholder')" />
          <input v-model="createForm.route" type="text" class="input md:col-span-2" :placeholder="$t('loading.create.routePlaceholder')" />
          <input v-model="createForm.scheduledLoadingTime" type="datetime-local" class="input" />
          <input v-model="createForm.vehicleSeal" type="text" class="input" :placeholder="$t('loading.create.sealPlaceholder')" />
          <input v-model="createForm.remark" type="text" class="input md:col-span-2" :placeholder="$t('loading.create.remarkPlaceholder')" />
        </div>

        <div class="flex justify-end gap-3">
          <button @click="showCreateModal = false" class="px-4 py-2 rounded-xl bg-slate-100 text-slate-700 font-semibold">{{ $t('common.buttons.cancel') }}</button>
          <button
            @click="submitCreate"
            :disabled="creating"
            class="px-4 py-2 rounded-xl bg-orange-600 hover:bg-orange-700 text-white font-semibold disabled:opacity-60"
          >
            {{ creating ? $t('loading.create.creating') : $t('loading.create.createButton') }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script>
import { listLoadingTrips, createLoadingTrip } from '../../services/loading'
import { hasPermission } from '../../services/auth'
import PageHeader from '../../components/ui/PageHeader.vue'
import FilterBar from '../../components/ui/FilterBar.vue'
import DataTable from '../../components/ui/DataTable.vue'
import StatusBadge from '../../components/ui/StatusBadge.vue'

let searchDebounce = null

export default {
  name: 'LoadingView',

  components: { PageHeader, FilterBar, DataTable, StatusBadge },

  data() {
    return {
      loading: false,
      loadError: '',
      trips: [],
      allTrips: [],

      filters: { search: '', status: '' },

      showCreateModal: false,
      createForm: {
        vehicleNo: '', driverName: '', carrier: '', loadingDock: '', route: '',
        scheduledLoadingTime: '', vehicleSeal: '', remark: ''
      },
      createError: '',
      creating: false
    }
  },

  computed: {
    plannedCount() { return this.allTrips.filter((t) => t.status === 'PLANNED').length },
    loadingCount() { return this.allTrips.filter((t) => t.status === 'LOADING').length },
    completedCount() { return this.allTrips.filter((t) => t.status === 'COMPLETED').length },
    cancelledCount() { return this.allTrips.filter((t) => t.status === 'CANCELLED').length },
    canCreate() { return hasPermission('loading.create') }
  },

  watch: {
    'filters.search'() {
      clearTimeout(searchDebounce)
      searchDebounce = setTimeout(() => this.fetchTrips(), 300)
    },
    'filters.status'() { this.fetchTrips() }
  },

  async mounted() {
    await this.fetchTrips()
    await this.fetchSummary()
  },

  methods: {
    async fetchTrips() {
      this.loading = true
      this.loadError = ''
      try {
        const res = await listLoadingTrips({ search: this.filters.search, status: this.filters.status, limit: 100 })
        this.trips = res.data
      } catch (err) {
        this.loadError = err.message || this.$t('loading.messages.loadFailed')
      } finally {
        this.loading = false
      }
    },

    async fetchSummary() {
      try {
        const res = await listLoadingTrips({ limit: 200 })
        this.allTrips = res.data
      } catch {
        // Summary cards are best-effort — the main list's own error
        // banner already reports a real load failure.
      }
    },

    clearFilter() {
      this.filters = { search: '', status: '' }
      this.fetchTrips()
    },

    statusTone(status) {
      if (status === 'COMPLETED') return 'green'
      if (status === 'LOADING') return 'orange'
      if (status === 'CANCELLED') return 'red'
      return 'gray'
    },

    statusLabel(status) {
      switch (status) {
        case 'PLANNED': return this.$t('loading.statusLabels.planned')
        case 'LOADING': return this.$t('loading.statusLabels.loading')
        case 'COMPLETED': return this.$t('common.status.completed')
        case 'CANCELLED': return this.$t('common.status.cancelled')
        default: return status
      }
    },

    formatDateTime(isoString) {
      if (!isoString) return '—'
      return this.$d(new Date(isoString), 'long')
    },

    openCreateModal() {
      this.createError = ''
      this.createForm = {
        vehicleNo: '', driverName: '', carrier: '', loadingDock: '', route: '',
        scheduledLoadingTime: '', vehicleSeal: '', remark: ''
      }
      this.showCreateModal = true
    },

    async submitCreate() {
      this.createError = ''
      this.creating = true
      try {
        const res = await createLoadingTrip({
          vehicle_no: this.createForm.vehicleNo || undefined,
          driver_name: this.createForm.driverName || undefined,
          carrier: this.createForm.carrier || undefined,
          loading_dock: this.createForm.loadingDock || undefined,
          route: this.createForm.route || undefined,
          scheduled_loading_time: this.createForm.scheduledLoadingTime
            ? new Date(this.createForm.scheduledLoadingTime).toISOString() : undefined,
          vehicle_seal: this.createForm.vehicleSeal || undefined,
          remark: this.createForm.remark || undefined
        })
        this.showCreateModal = false
        this.$router.push(`/outbound/loading/${res.data.id}`)
      } catch (err) {
        this.createError = err.message || this.$t('loading.create.createFailed')
      } finally {
        this.creating = false
      }
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

.summary-card {
  background: #fff;
  border: 1px solid #e2e8f0;
  border-radius: 1rem;
  padding: 1.25rem;
}

.summary-label {
  color: #64748b;
  font-size: 0.85rem;
  font-weight: 600;
}

.summary-value {
  font-size: 1.75rem;
  font-weight: 700;
  color: #1e293b;
  margin-top: 0.25rem;
}
</style>
