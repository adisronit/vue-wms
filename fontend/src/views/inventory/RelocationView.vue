<template>
  <div class="space-y-6">
    <PageHeader :title="$t('relocation.title')" :subtitle="$t('relocation.subtitle')">
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
          @click="$router.push('/inventory/relocation/new')"
          class="bg-orange-600 hover:bg-orange-700 text-white px-5 py-3 rounded-xl font-semibold"
        >
          {{ $t('relocation.newButton') }}
        </button>
      </template>
    </PageHeader>

    <div class="grid grid-cols-1 md:grid-cols-4 gap-5">
      <div class="summary-card">
        <p class="summary-label">{{ $t('common.status.draft') }}</p>
        <h2 class="summary-value">{{ draftCount }}</h2>
      </div>

      <div class="summary-card">
        <p class="summary-label">{{ $t('relocation.statusLabels.ready') }}</p>
        <h2 class="summary-value">{{ readyCount }}</h2>
      </div>

      <div class="summary-card">
        <p class="summary-label">{{ $t('common.status.inProgress') }}</p>
        <h2 class="summary-value">{{ inProgressCount }}</h2>
      </div>

      <div class="summary-card">
        <p class="summary-label">{{ $t('common.status.completed') }}</p>
        <h2 class="summary-value text-green-600">{{ completedCount }}</h2>
      </div>
    </div>

    <div v-if="loadError" class="bg-red-50 border border-red-200 text-red-600 rounded-xl p-3 text-sm">
      {{ loadError }}
    </div>

    <FilterBar>
      <input
        v-model="filters.keyword"
        type="text"
        :placeholder="$t('relocation.filters.searchPlaceholder')"
        class="input"
      />

      <select v-model="filters.taskStatus" class="input">
        <option value="">{{ $t('relocation.filters.allStatus') }}</option>
        <option value="DRAFT">{{ $t('common.status.draft') }}</option>
        <option value="READY">{{ $t('relocation.statusLabels.ready') }}</option>
        <option value="IN_PROGRESS">{{ $t('common.status.inProgress') }}</option>
        <option value="COMPLETED">{{ $t('common.status.completed') }}</option>
        <option value="CANCELLED">{{ $t('common.status.cancelled') }}</option>
      </select>

      <select v-model="filters.warehouseId" class="input">
        <option value="">{{ $t('relocation.filters.allWarehouses') }}</option>
        <option v-for="w in warehouses" :key="w.id" :value="w.id">{{ w.code }}</option>
      </select>

      <button
        @click="clearFilter"
        class="bg-slate-100 hover:bg-slate-200 text-slate-700 rounded-xl font-semibold"
      >
        {{ $t('common.buttons.clearFilter') }}
      </button>
    </FilterBar>

    <DataTable
      :loading="loading"
      :is-empty="relocationList.length === 0"
      :loading-text="$t('relocation.loadingTasks')"
      :empty-text="$t('relocation.noTasks')"
      :colspan="9"
      min-width="1150px"
    >
      <template #head>
        <tr>
          <th class="table-th">{{ $t('relocation.table.taskNo') }}</th>
          <th class="table-th">{{ $t('relocation.table.sku') }}</th>
          <th class="table-th">{{ $t('relocation.table.product') }}</th>
          <th class="table-th">{{ $t('relocation.table.lotNo') }}</th>
          <th class="table-th text-right">{{ $t('relocation.table.qty') }}</th>
          <th class="table-th">{{ $t('relocation.table.fromLocation') }}</th>
          <th class="table-th">{{ $t('relocation.table.toLocation') }}</th>
          <th class="table-th">{{ $t('common.labels.status') }}</th>
          <th class="table-th text-right">{{ $t('relocation.table.action') }}</th>
        </tr>
      </template>

      <tr
        v-for="item in relocationList"
        :key="item.id"
        class="border-t hover:bg-orange-50"
      >
        <td class="table-td font-semibold text-slate-800">{{ item.taskNo }}</td>
        <td class="table-td">{{ item.sku }}</td>
        <td class="table-td">{{ item.productName }}</td>
        <td class="table-td">{{ item.lotNo || '—' }}</td>
        <td class="table-td text-right">{{ item.movedQty }} / {{ item.qty }}</td>
        <td class="table-td">{{ item.fromLocation }}</td>
        <td class="table-td">{{ item.toLocation }}</td>
        <td class="table-td">
          <StatusBadge :tone="statusTone(item.taskStatus)">{{ statusText(item.taskStatus) }}</StatusBadge>
        </td>
        <td class="table-td text-right">
          <button @click="openRelocation(item)" class="text-orange-600 hover:text-orange-700 font-semibold">
            {{ $t('common.buttons.view') }}
          </button>
        </td>
      </tr>
    </DataTable>

    <div v-if="selectedTask" class="fixed inset-0 bg-black/40 flex justify-end z-50">
      <div class="w-full max-w-3xl bg-white h-full p-6 overflow-y-auto">
        <div class="flex items-center justify-between mb-6">
          <div>
            <h2 class="text-xl font-bold text-slate-800">{{ $t('relocation.drawer.title') }}</h2>
            <p class="text-slate-500">{{ selectedTask.relocation_no }} · {{ selectedTask.warehouse_code }}</p>
          </div>

          <button @click="closeDrawer" class="text-slate-500 hover:text-red-600 text-xl">✕</button>
        </div>

        <div class="mb-6 flex items-center gap-3">
          <button
            @click="viewTransferNote"
            class="bg-slate-100 hover:bg-slate-200 text-slate-700 text-sm px-3 py-2 rounded-lg font-semibold"
          >
            📄 {{ $t('relocation.documents.viewTransferNote') }}
          </button>
          <StatusBadge :tone="statusTone(selectedTask.status)">{{ statusText(selectedTask.status) }}</StatusBadge>
        </div>

        <div class="bg-slate-50 rounded-2xl p-5 mb-6">
          <div class="grid grid-cols-2 gap-4">
            <div>
              <p class="detail-label">{{ $t('common.labels.warehouse') }}</p>
              <p class="detail-value">{{ selectedTask.warehouse_code }}</p>
            </div>
            <div>
              <p class="detail-label">{{ $t('common.labels.createdAt') }}</p>
              <p class="detail-value">{{ formatDateTime(selectedTask.created_at) }}</p>
            </div>
            <div class="col-span-2">
              <p class="detail-label">{{ $t('common.labels.remark') }}</p>
              <p class="detail-value">{{ selectedTask.remark || '—' }}</p>
            </div>
          </div>
        </div>

        <div v-if="selectedTask.status === 'CANCELLED'" class="bg-red-50 border border-red-200 rounded-2xl p-5 mb-6">
          <div class="grid grid-cols-2 gap-4">
            <div>
              <p class="detail-label">{{ $t('relocation.drawer.cancelledAt') }}</p>
              <p class="detail-value">{{ formatDateTime(selectedTask.cancelled_at) }}</p>
            </div>
            <div>
              <p class="detail-label">{{ $t('relocation.drawer.cancelledBy') }}</p>
              <p class="detail-value">{{ selectedTask.cancelled_by_name || '—' }}</p>
            </div>
            <div class="col-span-2">
              <p class="detail-label">{{ $t('relocation.drawer.cancellationRemark') }}</p>
              <p class="detail-value">{{ selectedTask.cancel_remark || '—' }}</p>
            </div>
          </div>
        </div>

        <div v-if="drawerError" class="bg-red-50 border border-red-200 text-red-600 rounded-xl p-3 text-sm mb-4">
          {{ drawerError }}
        </div>

        <h3 class="font-bold text-slate-800 mb-3">{{ $t('relocation.drawer.lines') }}</h3>
        <div class="space-y-3 mb-6">
          <div
            v-for="line in selectedTask.items"
            :key="line.id"
            class="border border-slate-200 rounded-2xl p-4"
            :class="line.status === 'COMPLETED' ? 'bg-green-50' : ''"
          >
            <div class="flex items-center justify-between mb-2">
              <p class="font-semibold text-slate-800">{{ line.sku }} — {{ line.product_name }}</p>
              <StatusBadge :tone="statusTone(line.status)">{{ statusText(line.status) }}</StatusBadge>
            </div>
            <p class="text-sm text-slate-500 mb-3">
              {{ line.source_location_code }} → {{ line.destination_location_code }}
              <span v-if="line.lot_no"> · {{ $t('relocation.table.lotNo') }}: {{ line.lot_no }}</span>
              <span v-if="line.lpn"> · LPN: {{ line.lpn }}</span>
              · {{ $t('relocation.drawer.moved') }}: {{ line.moved_qty }} / {{ line.requested_qty }} {{ line.uom }}
            </p>

            <div v-if="canConfirmLine(line)" class="grid grid-cols-1 md:grid-cols-3 gap-3">
              <input
                v-model.number="lineForms[line.id].quantity"
                type="number"
                class="input"
                :placeholder="$t('relocation.drawer.confirmQtyPlaceholder')"
              />
              <input
                v-model="lineForms[line.id].destinationCode"
                type="text"
                class="input"
                :placeholder="$t('relocation.drawer.overrideDestinationPlaceholder')"
              />
              <button
                @click="confirmLine(line)"
                :disabled="submitting"
                class="bg-orange-600 hover:bg-orange-700 text-white rounded-xl font-semibold disabled:opacity-60"
              >
                {{ $t('relocation.drawer.confirmLineButton') }}
              </button>
            </div>
          </div>
        </div>

        <div class="flex flex-col md:flex-row gap-3">
          <button
            v-if="canReady && selectedTask.status === 'DRAFT'"
            @click="markReady"
            :disabled="submitting"
            class="flex-1 bg-slate-800 hover:bg-slate-900 text-white py-3 rounded-xl font-semibold disabled:opacity-60"
          >
            {{ $t('relocation.drawer.markReadyButton') }}
          </button>

          <button
            v-if="canCancelTask"
            @click="showCancelConfirm = true"
            :disabled="submitting"
            class="flex-1 bg-white border border-slate-300 hover:bg-slate-100 text-slate-700 py-3 rounded-xl font-semibold disabled:opacity-60"
          >
            {{ $t('relocation.drawer.cancelButton') }}
          </button>
        </div>
      </div>
    </div>

    <ConfirmDialog
      :open="showCancelConfirm"
      :title="$t('relocation.drawer.cancelButton')"
      :message="$t('relocation.messages.confirmCancelTask')"
      :confirm-label="$t('relocation.drawer.cancelButton')"
      :cancel-label="$t('common.buttons.cancel')"
      :loading-label="$t('common.buttons.submitting')"
      danger
      :loading="submitting"
      @cancel="showCancelConfirm = false"
      @confirm="cancelDrawerTask"
    />
  </div>
</template>

<script>
import {
  listRelocations,
  getRelocationTask,
  confirmRelocationTask,
  cancelRelocationTask,
  readyRelocationTask,
  newIdempotencyKey,
  exportRelocations,
  viewRelocationTransferNote,
} from '../../services/relocation'
import { listWarehouses, listLocations, listZones } from '../../services/warehouses'
import { hasPermission } from '../../services/auth'
import { showToast } from '../../services/toast'
import PageHeader from '../../components/ui/PageHeader.vue'
import FilterBar from '../../components/ui/FilterBar.vue'
import DataTable from '../../components/ui/DataTable.vue'
import StatusBadge from '../../components/ui/StatusBadge.vue'
import ConfirmDialog from '../../components/ui/ConfirmDialog.vue'
import i18n from '../../i18n'

const { t } = i18n.global
let searchDebounce = null

function statusText(status) {
  switch (status) {
    case 'DRAFT': return t('common.status.draft')
    case 'READY': return t('relocation.statusLabels.ready')
    case 'PENDING': return t('common.status.pending')
    case 'PARTIALLY_COMPLETED':
    case 'IN_PROGRESS': return t('common.status.inProgress')
    case 'COMPLETED': return t('common.status.completed')
    case 'CANCELLED': return t('common.status.cancelled')
    default: return status
  }
}

function statusTone(status) {
  if (status === 'DRAFT') return 'gray'
  if (status === 'READY') return 'blue'
  if (status === 'PARTIALLY_COMPLETED' || status === 'IN_PROGRESS') return 'orange'
  if (status === 'COMPLETED') return 'green'
  if (status === 'CANCELLED') return 'red'
  return 'gray'
}

function mapRelocationRow(r) {
  return {
    id: r.item_id,
    taskId: r.task_id,
    taskNo: r.relocation_no,
    sku: r.sku,
    productName: r.product_name,
    lotNo: r.lot_no,
    qty: r.requested_qty,
    movedQty: r.moved_qty,
    fromLocation: r.source_location_code,
    toLocation: r.destination_location_code,
    status: r.status,
    taskStatus: r.task_status
  }
}

export default {
  name: 'RelocationView',

  components: { PageHeader, FilterBar, DataTable, StatusBadge, ConfirmDialog },

  data() {
    return {
      loading: false,
      loadError: '',
      exporting: false,
      relocationList: [],
      allItems: [],
      warehouses: [],

      selectedTask: null,
      drawerError: '',
      drawerTaskId: null,
      drawerLocations: [],
      lineForms: {},
      submitting: false,
      showCancelConfirm: false,

      filters: {
        keyword: '',
        taskStatus: '',
        warehouseId: ''
      }
    }
  },

  computed: {
    draftCount() {
      return this.allItems.filter((r) => r.task_status === 'DRAFT').length
    },
    readyCount() {
      return this.allItems.filter((r) => r.task_status === 'READY').length
    },
    inProgressCount() {
      return this.allItems.filter((r) => r.task_status === 'IN_PROGRESS').length
    },
    completedCount() {
      return this.allItems.filter((r) => r.task_status === 'COMPLETED').length
    },
    canCreate() {
      return hasPermission('relocation.create')
    },
    canReady() {
      return hasPermission('relocation.approve')
    },
    canConfirm() {
      return hasPermission('relocation.confirm')
    },
    canCancelTask() {
      const status = this.selectedTask && this.selectedTask.status
      return hasPermission('relocation.update') && (status === 'DRAFT' || status === 'READY' || status === 'IN_PROGRESS')
    }
  },

  watch: {
    'filters.keyword'() {
      clearTimeout(searchDebounce)
      searchDebounce = setTimeout(() => this.fetchRelocations(), 300)
    },
    'filters.taskStatus'() {
      this.fetchRelocations()
    },
    'filters.warehouseId'() {
      this.fetchRelocations()
    }
  },

  mounted() {
    this.fetchRelocations()
    this.fetchSummary()
    this.fetchWarehouses()
  },

  methods: {
    statusText,
    statusTone,

    async fetchRelocations() {
      this.loading = true
      this.loadError = ''
      try {
        const res = await listRelocations({
          search: this.filters.keyword,
          taskStatus: this.filters.taskStatus,
          warehouseId: this.filters.warehouseId,
          limit: 100
        })
        this.relocationList = res.data.map(mapRelocationRow)
      } catch (err) {
        this.loadError = err.message
      } finally {
        this.loading = false
      }
    },

    async fetchSummary() {
      try {
        const res = await listRelocations({ limit: 200 })
        this.allItems = res.data
      } catch {
        // Non-critical: the table's own error banner already surfaces API failures.
      }
    },

    async fetchWarehouses() {
      try {
        const res = await listWarehouses({ limit: 100 })
        this.warehouses = res.data
      } catch {
        // Non-critical: the warehouse filter simply stays empty.
      }
    },

    clearFilter() {
      this.filters.keyword = ''
      this.filters.taskStatus = ''
      this.filters.warehouseId = ''
      this.fetchRelocations()
    },

    async exportCsv() {
      this.exporting = true
      try {
        await exportRelocations({ search: this.filters.keyword, taskStatus: this.filters.taskStatus, warehouseId: this.filters.warehouseId })
      } catch (err) {
        this.loadError = err.message
      } finally {
        this.exporting = false
      }
    },

    async viewTransferNote() {
      try {
        await viewRelocationTransferNote(this.drawerTaskId)
      } catch (err) {
        showToast(err.message, 'error')
      }
    },

    canConfirmLine(line) {
      const taskStatus = this.selectedTask && this.selectedTask.status
      return this.canConfirm && (taskStatus === 'READY' || taskStatus === 'IN_PROGRESS') && line.status !== 'COMPLETED'
    },

    async openRelocation(item) {
      this.drawerTaskId = item.taskId
      this.drawerError = ''
      this.drawerLocations = []
      try {
        const res = await getRelocationTask(item.taskId)
        this.selectedTask = res.data
        this.lineForms = {}
        for (const line of res.data.items) {
          this.lineForms[line.id] = { quantity: line.requested_qty - line.moved_qty, destinationCode: '' }
        }
        await this.fetchLocationsForWarehouse(res.data.warehouse_id)
      } catch (err) {
        this.drawerError = err.message
      }
    },

    closeDrawer() {
      this.selectedTask = null
      this.drawerTaskId = null
    },

    async fetchLocationsForWarehouse(warehouseId) {
      if (!warehouseId) return
      const zonesRes = await listZones(warehouseId)
      const allLocations = []
      for (const zone of zonesRes.data) {
        const locRes = await listLocations(zone.id)
        allLocations.push(...locRes.data)
      }
      this.drawerLocations = allLocations
    },

    async markReady() {
      this.submitting = true
      this.drawerError = ''
      try {
        await readyRelocationTask(this.drawerTaskId)
        showToast(this.$t('relocation.messages.markedReady'), 'success')
        await this.openRelocation({ taskId: this.drawerTaskId })
        await this.fetchRelocations()
        await this.fetchSummary()
      } catch (err) {
        this.drawerError = err.message
      } finally {
        this.submitting = false
      }
    },

    async confirmLine(line) {
      const form = this.lineForms[line.id]
      if (!form.quantity || form.quantity <= 0) {
        this.drawerError = this.$t('relocation.messages.confirmQtyRequired')
        return
      }

      let destinationLocationId
      if (form.destinationCode) {
        const destination = this.drawerLocations.find((l) => l.code === form.destinationCode.trim())
        if (!destination) {
          this.drawerError = this.$t('relocation.messages.noLocationFound', { value: form.destinationCode })
          return
        }
        destinationLocationId = destination.id
      }

      this.submitting = true
      this.drawerError = ''
      try {
        await confirmRelocationTask(this.drawerTaskId, {
          idempotency_key: newIdempotencyKey(),
          items: [{
            item_id: line.id,
            quantity: Number(form.quantity),
            destination_location_id: destinationLocationId
          }]
        })
        showToast(this.$t('relocation.messages.relocationConfirmed'), 'success')
        await this.openRelocation({ taskId: this.drawerTaskId })
        await this.fetchRelocations()
        await this.fetchSummary()
      } catch (err) {
        this.drawerError = err.message
      } finally {
        this.submitting = false
      }
    },

    async cancelDrawerTask() {
      this.submitting = true
      this.drawerError = ''
      try {
        await cancelRelocationTask(this.drawerTaskId, {})
        this.showCancelConfirm = false
        showToast(this.$t('relocation.messages.taskCancelled'), 'success')
        this.closeDrawer()
        await this.fetchRelocations()
        await this.fetchSummary()
      } catch (err) {
        this.drawerError = err.message
        this.showCancelConfirm = false
      } finally {
        this.submitting = false
      }
    },

    formatDateTime(isoString) {
      if (!isoString) return '—'
      return this.$d(new Date(isoString), 'long')
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

.detail-label {
  color: #64748b;
  font-size: 13px;
}

.detail-value {
  color: #0f172a;
  font-weight: 700;
  margin-top: 4px;
}
</style>
