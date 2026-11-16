<template>
  <div class="space-y-6">
    <PageHeader :title="$t('packing.title')" :subtitle="$t('packing.subtitle')">
      <template #actions>
        <button
          @click="exportCsv"
          :disabled="exporting"
          class="bg-slate-100 hover:bg-slate-200 text-slate-700 px-5 py-3 rounded-xl font-semibold disabled:opacity-60"
        >
          {{ exporting ? $t('common.buttons.exporting') : '⬇ ' + $t('common.buttons.exportCsv') }}
        </button>

        <button
          v-if="canCreate"
          @click="openCreateModal"
          class="bg-orange-600 hover:bg-orange-700 text-white px-5 py-3 rounded-xl font-semibold"
        >
          {{ $t('packing.newJobButton') }}
        </button>
      </template>
    </PageHeader>

    <div class="grid grid-cols-1 md:grid-cols-4 gap-5">
      <div class="summary-card">
        <p class="summary-label">{{ $t('packing.summary.draft') }}</p>
        <h2 class="summary-value">{{ draftCount }}</h2>
      </div>
      <div class="summary-card">
        <p class="summary-label">{{ $t('packing.summary.inProgress') }}</p>
        <h2 class="summary-value">{{ inProgressCount }}</h2>
      </div>
      <div class="summary-card">
        <p class="summary-label">{{ $t('packing.summary.completed') }}</p>
        <h2 class="summary-value text-green-600">{{ completedCount }}</h2>
      </div>
      <div class="summary-card">
        <p class="summary-label">{{ $t('packing.summary.cancelled') }}</p>
        <h2 class="summary-value">{{ cancelledCount }}</h2>
      </div>
    </div>

    <div v-if="loadError" class="bg-red-50 border border-red-200 text-red-600 rounded-xl p-3 text-sm">
      {{ loadError }}
    </div>

    <FilterBar :cols="4">
      <input v-model="filters.search" type="text" :placeholder="$t('packing.filters.searchPlaceholder')" class="input" />

      <select v-model="filters.status" class="input">
        <option value="">{{ $t('packing.filters.allStatus') }}</option>
        <option value="DRAFT">{{ $t('packing.statusLabels.draft') }}</option>
        <option value="IN_PROGRESS">{{ $t('common.status.inProgress') }}</option>
        <option value="COMPLETED">{{ $t('common.status.completed') }}</option>
        <option value="CANCELLED">{{ $t('common.status.cancelled') }}</option>
      </select>

      <select v-model="filters.warehouseId" class="input">
        <option value="">{{ $t('orders.filters.allWarehouses') }}</option>
        <option v-for="w in warehouses" :key="w.id" :value="w.id">{{ w.code }}</option>
      </select>

      <button @click="clearFilter" class="bg-slate-100 hover:bg-slate-200 text-slate-700 rounded-xl font-semibold">
        {{ $t('common.buttons.clearFilter') }}
      </button>
    </FilterBar>

    <DataTable
      :loading="loading"
      :is-empty="packingList.length === 0"
      :loading-text="$t('packing.loadingJobs')"
      :empty-text="$t('packing.noJobs')"
      :colspan="9"
      min-width="1100px"
    >
      <template #head>
        <tr>
          <th class="table-th">{{ $t('packing.table.jobNo') }}</th>
          <th class="table-th">{{ $t('packing.table.orderNo') }}</th>
          <th class="table-th">{{ $t('packing.table.customer') }}</th>
          <th class="table-th">{{ $t('packing.table.warehouse') }}</th>
          <th class="table-th text-right">{{ $t('packing.table.items') }}</th>
          <th class="table-th">{{ $t('packing.table.progress') }}</th>
          <th class="table-th">{{ $t('packing.table.assignedPacker') }}</th>
          <th class="table-th">{{ $t('common.labels.status') }}</th>
          <th class="table-th text-right">{{ $t('packing.table.action') }}</th>
        </tr>
      </template>

      <tr v-for="job in packingList" :key="job.id" class="border-t hover:bg-orange-50">
        <td class="table-td font-semibold text-slate-800">{{ job.job_no }}</td>
        <td class="table-td">{{ job.order_no }}</td>
        <td class="table-td">{{ job.customer || '—' }}</td>
        <td class="table-td">{{ job.warehouse_code }}</td>
        <td class="table-td text-right">{{ job.item_count }}</td>
        <td class="table-td">{{ Math.round(job.progress_pct) }}%</td>
        <td class="table-td">{{ job.assigned_packer_name || '—' }}</td>
        <td class="table-td"><StatusBadge :tone="statusTone(job.status)">{{ statusLabel(job.status) }}</StatusBadge></td>
        <td class="table-td text-right">
          <button @click="$router.push(`/outbound/packing/${job.id}`)" class="text-orange-600 hover:underline font-semibold">
            {{ $t('common.buttons.view') }}
          </button>
        </td>
      </tr>
    </DataTable>

    <!-- Create Packing Job modal -->
    <div v-if="showCreateModal" class="fixed inset-0 bg-black/30 flex items-center justify-center z-50 p-4">
      <div class="bg-white rounded-2xl shadow-xl w-full max-w-xl p-6 space-y-4 max-h-[90vh] overflow-y-auto">
        <div class="flex items-center justify-between">
          <h2 class="text-lg font-bold text-slate-800">{{ $t('packing.create.title') }}</h2>
          <button @click="showCreateModal = false" class="text-slate-400 hover:text-slate-600">✕</button>
        </div>

        <p v-if="createError" class="bg-red-50 border border-red-200 text-red-600 rounded-xl p-3 text-sm">{{ createError }}</p>

        <div v-if="loadingSources" class="text-sm text-slate-400">{{ $t('packing.create.loadingSources') }}</div>
        <p v-else-if="sources.length === 0" class="text-sm text-slate-500">{{ $t('packing.create.noSources') }}</p>

        <div v-else class="space-y-3">
          <label class="label">{{ $t('packing.create.selectPickingTask') }}</label>
          <select v-model="createForm.pickingTaskId" class="input" @change="onSourceChange">
            <option value="">{{ $t('packing.create.selectPlaceholder') }}</option>
            <option v-for="s in sources" :key="s.picking_task_id" :value="s.picking_task_id">
              {{ s.pick_no }} — {{ s.order_no }} ({{ s.customer || '—' }}, {{ s.item_count }} {{ $t('packing.create.linesSuffix') }})
            </option>
          </select>

          <!-- Phase 33C.2: a batch-sourced source task fans out into one
               packing job PER ORDER — the caller must pick which one of
               the batch's own still-pending orders this job is for. -->
          <template v-if="selectedSource && selectedSource.is_batch">
            <p class="text-xs text-orange-600">{{ $t('packing.create.batchSourcedNote') }}</p>
            <label class="label">{{ $t('packing.create.selectOrderLabel') }}</label>
            <select v-model="createForm.orderId" class="input">
              <option value="">{{ $t('packing.create.selectOrderPlaceholder') }}</option>
              <option v-for="o in selectedSource.pending_orders" :key="o.order_id" :value="o.order_id">
                {{ o.order_no }} ({{ o.customer || '—' }})
              </option>
            </select>
          </template>

          <input v-model="createForm.remark" type="text" class="input" :placeholder="$t('packing.create.remarkPlaceholder')" />
        </div>

        <div class="flex justify-end gap-3">
          <button @click="showCreateModal = false" class="px-4 py-2 rounded-xl bg-slate-100 text-slate-700 font-semibold">{{ $t('common.buttons.cancel') }}</button>
          <button
            @click="submitCreate"
            :disabled="creating || !createForm.pickingTaskId || (selectedSource && selectedSource.is_batch && !createForm.orderId)"
            class="px-4 py-2 rounded-xl bg-orange-600 hover:bg-orange-700 text-white font-semibold disabled:opacity-60"
          >
            {{ creating ? $t('packing.create.creating') : $t('packing.create.createButton') }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script>
import { listPacking, exportPacking, listPackableSources, createPackingJob } from '../../services/packing'
import { listWarehouses } from '../../services/warehouses'
import { hasPermission } from '../../services/auth'
import { showToast } from '../../services/toast'
import PageHeader from '../../components/ui/PageHeader.vue'
import FilterBar from '../../components/ui/FilterBar.vue'
import DataTable from '../../components/ui/DataTable.vue'
import StatusBadge from '../../components/ui/StatusBadge.vue'

let searchDebounce = null

export default {
  name: 'PackingView',

  components: { PageHeader, FilterBar, DataTable, StatusBadge },

  data() {
    return {
      loading: false,
      loadError: '',
      exporting: false,
      packingList: [],
      allJobs: [],
      warehouses: [],

      filters: { search: '', status: '', warehouseId: '' },

      showCreateModal: false,
      loadingSources: false,
      sources: [],
      createForm: { pickingTaskId: '', orderId: '', remark: '' },
      createError: '',
      creating: false
    }
  },

  computed: {
    draftCount() { return this.allJobs.filter((j) => j.status === 'DRAFT').length },
    inProgressCount() { return this.allJobs.filter((j) => j.status === 'IN_PROGRESS').length },
    completedCount() { return this.allJobs.filter((j) => j.status === 'COMPLETED').length },
    cancelledCount() { return this.allJobs.filter((j) => j.status === 'CANCELLED').length },
    canCreate() { return hasPermission('packing.create') },
    // selectedSource (Phase 33C.2) resolves the chosen picking task's
    // own source row — is_batch/pending_orders drive the order picker.
    selectedSource() {
      if (!this.createForm.pickingTaskId) return null
      return this.sources.find((s) => s.picking_task_id === this.createForm.pickingTaskId) || null
    }
  },

  watch: {
    'filters.search'() {
      clearTimeout(searchDebounce)
      searchDebounce = setTimeout(() => this.fetchPacking(), 300)
    },
    'filters.status'() { this.fetchPacking() },
    'filters.warehouseId'() { this.fetchPacking() }
  },

  async mounted() {
    await this.fetchWarehouses()
    await this.fetchPacking()
    await this.fetchSummary()
  },

  methods: {
    async fetchWarehouses() {
      try {
        const res = await listWarehouses({ limit: 100 })
        this.warehouses = res.data
      } catch (err) {
        this.loadError = err.message || this.$t('orders.messages.loadWarehousesFailed')
      }
    },

    async fetchPacking() {
      this.loading = true
      this.loadError = ''
      try {
        const res = await listPacking({
          search: this.filters.search, status: this.filters.status, warehouseId: this.filters.warehouseId, limit: 100
        })
        this.packingList = res.data
      } catch (err) {
        this.loadError = err.message || this.$t('packing.messages.loadFailed')
      } finally {
        this.loading = false
      }
    },

    async fetchSummary() {
      try {
        const res = await listPacking({ limit: 200 })
        this.allJobs = res.data
      } catch {
        // Summary cards are best-effort — the main list's own error
        // banner already reports a real load failure.
      }
    },

    clearFilter() {
      this.filters = { search: '', status: '', warehouseId: '' }
      this.fetchPacking()
    },

    async exportCsv() {
      this.exporting = true
      try {
        await exportPacking(this.filters)
      } catch (err) {
        showToast(err.message || this.$t('packing.messages.exportFailed'), 'error')
      } finally {
        this.exporting = false
      }
    },

    statusTone(status) {
      if (status === 'COMPLETED') return 'green'
      if (status === 'IN_PROGRESS') return 'orange'
      if (status === 'CANCELLED') return 'red'
      return 'gray'
    },

    statusLabel(status) {
      switch (status) {
        case 'DRAFT': return this.$t('packing.statusLabels.draft')
        case 'IN_PROGRESS': return this.$t('common.status.inProgress')
        case 'COMPLETED': return this.$t('common.status.completed')
        case 'CANCELLED': return this.$t('common.status.cancelled')
        default: return status
      }
    },

    async openCreateModal() {
      this.createError = ''
      this.createForm = { pickingTaskId: '', orderId: '', remark: '' }
      this.showCreateModal = true
      this.loadingSources = true
      try {
        const res = await listPackableSources()
        this.sources = res.data
      } catch (err) {
        this.createError = err.message || this.$t('packing.create.loadSourcesFailed')
      } finally {
        this.loadingSources = false
      }
    },

    // onSourceChange (Phase 33C.2) resets any previously-selected
    // order_id whenever the picking task selection changes — a stale
    // order_id from a different batch must never carry over.
    onSourceChange() {
      this.createForm.orderId = ''
    },

    async submitCreate() {
      this.createError = ''
      this.creating = true
      try {
        const res = await createPackingJob({
          picking_task_id: this.createForm.pickingTaskId,
          order_id: (this.selectedSource && this.selectedSource.is_batch) ? this.createForm.orderId : undefined,
          remark: this.createForm.remark || undefined
        })
        this.showCreateModal = false
        showToast(this.$t('packing.create.createSuccess'), 'success')
        this.$router.push(`/outbound/packing/${res.data.id}`)
      } catch (err) {
        this.createError = err.message || this.$t('packing.create.createFailed')
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

.label {
  display: block;
  font-size: 0.8rem;
  font-weight: 600;
  color: #64748b;
  margin-bottom: 4px;
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
