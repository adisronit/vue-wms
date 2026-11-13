<template>
  <div class="space-y-6">
    <PageHeader :title="$t('replenishment.title')" :subtitle="$t('replenishment.subtitle')">
      <template #actions>
        <button
          v-if="canCreate"
          @click="openRecommendations"
          class="bg-slate-100 hover:bg-slate-200 text-slate-700 px-5 py-3 rounded-xl font-semibold"
        >
          {{ $t('replenishment.recommendations') }}
        </button>

        <button
          v-if="canCreate"
          @click="openNewTask"
          class="bg-orange-600 hover:bg-orange-700 text-white px-5 py-3 rounded-xl font-semibold"
        >
          {{ $t('replenishment.newTask') }}
        </button>
      </template>
    </PageHeader>

    <div v-if="loadError" class="bg-red-50 border border-red-200 text-red-600 rounded-xl p-3 text-sm">
      {{ loadError }}
    </div>

    <FilterBar :cols="4">
      <input v-model="filters.search" type="text" :placeholder="$t('replenishment.filters.searchPlaceholder')" class="input" />

      <select v-model="filters.status" class="input">
        <option value="">{{ $t('replenishment.filters.allStatus') }}</option>
        <option value="PENDING">{{ $t('replenishment.status.pending') }}</option>
        <option value="IN_PROGRESS">{{ $t('replenishment.status.inProgress') }}</option>
        <option value="COMPLETED">{{ $t('replenishment.status.completed') }}</option>
        <option value="CANCELLED">{{ $t('replenishment.status.cancelled') }}</option>
        <option value="ISSUE">{{ $t('replenishment.status.issue') }}</option>
      </select>

      <select v-model="filters.warehouseId" class="input">
        <option value="">{{ $t('replenishment.filters.allWarehouses') }}</option>
        <option v-for="w in warehouses" :key="w.id" :value="w.id">{{ w.code }}</option>
      </select>

      <button @click="clearFilter" class="bg-slate-100 hover:bg-slate-200 text-slate-700 rounded-xl font-semibold">
        {{ $t('common.buttons.clearFilter') }}
      </button>
    </FilterBar>

    <DataTable
      :loading="loading"
      :is-empty="taskList.length === 0"
      :loading-text="$t('replenishment.table.loading')"
      :empty-text="$t('replenishment.table.noData')"
      :colspan="9"
      min-width="1000px"
    >
      <template #head>
        <tr>
          <th class="table-th">{{ $t('replenishment.table.taskNo') }}</th>
          <th class="table-th">{{ $t('replenishment.table.sku') }}</th>
          <th class="table-th">{{ $t('replenishment.table.product') }}</th>
          <th class="table-th">{{ $t('replenishment.table.from') }}</th>
          <th class="table-th">{{ $t('replenishment.table.to') }}</th>
          <th class="table-th">{{ $t('replenishment.table.qty') }}</th>
          <th class="table-th">{{ $t('replenishment.table.warehouse') }}</th>
          <th class="table-th">{{ $t('replenishment.table.status') }}</th>
          <th class="table-th text-right">{{ $t('replenishment.table.action') }}</th>
        </tr>
      </template>

      <tr v-for="t in taskList" :key="t.id" class="border-t hover:bg-orange-50">
        <td class="table-td font-semibold text-slate-800">{{ t.task_no }}</td>
        <td class="table-td">{{ t.sku }}</td>
        <td class="table-td">{{ t.product_name }}</td>
        <td class="table-td">{{ t.from_location_code }}</td>
        <td class="table-td">{{ t.to_location_code }}</td>
        <td class="table-td">{{ t.quantity }}</td>
        <td class="table-td">{{ t.warehouse_code }}</td>
        <td class="table-td"><StatusBadge :tone="statusTone(t.status)">{{ statusText(t.status) }}</StatusBadge></td>
        <td class="table-td text-right">
          <button @click="openDetail(t.id)" class="text-orange-600 hover:underline font-semibold">{{ $t('replenishment.table.view') }}</button>
        </td>
      </tr>
    </DataTable>

    <!-- New Task panel -->
    <div v-if="showNewForm" class="fixed inset-0 bg-black/30 flex items-center justify-center z-50 p-4">
      <div class="bg-white rounded-2xl shadow-xl w-full max-w-lg p-6 space-y-4">
        <div class="flex items-center justify-between">
          <h2 class="text-lg font-bold text-slate-800">{{ $t('replenishment.newTaskForm.title') }}</h2>
          <button @click="showNewForm = false" class="text-slate-400 hover:text-slate-600">✕</button>
        </div>

        <p v-if="formError" class="bg-red-50 border border-red-200 text-red-600 rounded-xl p-3 text-sm">{{ formError }}</p>

        <div class="grid grid-cols-1 gap-3">
          <select v-model="form.warehouseId" @change="onFormWarehouseChange" class="input">
            <option value="">{{ $t('replenishment.newTaskForm.selectWarehouse') }}</option>
            <option v-for="w in warehouses" :key="w.id" :value="w.id">{{ w.code }} — {{ w.name }}</option>
          </select>

          <input v-model="form.sku" type="text" :placeholder="$t('replenishment.newTaskForm.skuPlaceholder')" class="input" />

          <input v-model="form.lotNo" type="text" :placeholder="$t('replenishment.newTaskForm.lotNoPlaceholder')" class="input" />

          <select v-model="form.fromLocationId" class="input">
            <option value="">{{ $t('replenishment.newTaskForm.fromLocation') }}</option>
            <option v-for="l in formLocations" :key="l.id" :value="l.id">{{ l.code }}</option>
          </select>

          <select v-model="form.toLocationId" class="input">
            <option value="">{{ $t('replenishment.newTaskForm.toLocation') }}</option>
            <option v-for="l in formLocations" :key="l.id" :value="l.id">{{ l.code }}</option>
          </select>

          <input v-model.number="form.quantity" type="number" min="0" step="0.01" :placeholder="$t('replenishment.newTaskForm.quantityPlaceholder')" class="input" />

          <textarea v-model="form.remark" :placeholder="$t('replenishment.newTaskForm.remarkPlaceholder')" class="input" rows="2"></textarea>
        </div>

        <div class="flex justify-end gap-3">
          <button @click="showNewForm = false" class="px-4 py-2 rounded-xl bg-slate-100 text-slate-700 font-semibold">{{ $t('replenishment.newTaskForm.cancel') }}</button>
          <button @click="submitNewTask" :disabled="submitting" class="px-4 py-2 rounded-xl bg-orange-600 hover:bg-orange-700 text-white font-semibold disabled:opacity-60">
            {{ submitting ? $t('replenishment.newTaskForm.creating') : $t('replenishment.newTaskForm.createTask') }}
          </button>
        </div>
      </div>
    </div>

    <!-- Recommendations panel -->
    <div v-if="showRecommendations" class="fixed inset-0 bg-black/30 flex items-center justify-center z-50 p-4">
      <div class="bg-white rounded-2xl shadow-xl w-full max-w-3xl p-6 space-y-4">
        <div class="flex items-center justify-between">
          <h2 class="text-lg font-bold text-slate-800">{{ $t('replenishment.recommendationsPanel.title') }}</h2>
          <button @click="showRecommendations = false" class="text-slate-400 hover:text-slate-600">✕</button>
        </div>

        <div class="flex gap-3">
          <select v-model="recommendWarehouseId" class="input">
            <option value="">{{ $t('replenishment.recommendationsPanel.selectWarehouse') }}</option>
            <option v-for="w in warehouses" :key="w.id" :value="w.id">{{ w.code }} — {{ w.name }}</option>
          </select>
          <button @click="loadRecommendations" :disabled="!recommendWarehouseId || recommendLoading" class="px-4 py-2 rounded-xl bg-slate-100 text-slate-700 font-semibold disabled:opacity-60">
            {{ recommendLoading ? $t('replenishment.recommendationsPanel.loading') : $t('replenishment.recommendationsPanel.load') }}
          </button>
        </div>

        <p v-if="recommendError" class="bg-red-50 border border-red-200 text-red-600 rounded-xl p-3 text-sm">{{ recommendError }}</p>

        <div class="overflow-x-auto max-h-96 overflow-y-auto">
          <table class="min-w-[700px] w-full text-sm">
            <thead class="bg-slate-50 text-slate-500">
              <tr>
                <th class="table-th">{{ $t('replenishment.recommendationsPanel.table.sku') }}</th>
                <th class="table-th">{{ $t('replenishment.recommendationsPanel.table.product') }}</th>
                <th class="table-th">{{ $t('replenishment.recommendationsPanel.table.from') }}</th>
                <th class="table-th">{{ $t('replenishment.recommendationsPanel.table.to') }}</th>
                <th class="table-th">{{ $t('replenishment.recommendationsPanel.table.suggestedQty') }}</th>
                <th class="table-th text-right">{{ $t('replenishment.recommendationsPanel.table.action') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-if="recommendations.length === 0">
                <td colspan="6" class="text-center py-8 text-slate-400">{{ $t('replenishment.recommendationsPanel.table.noData') }}</td>
              </tr>
              <tr v-for="(r, i) in recommendations" :key="i" class="border-t">
                <td class="table-td">{{ r.sku }}</td>
                <td class="table-td">{{ r.product_name }}</td>
                <td class="table-td">{{ r.from_location_code }}</td>
                <td class="table-td">{{ r.to_location_code }}</td>
                <td class="table-td">{{ r.suggested_qty }}</td>
                <td class="table-td text-right">
                  <button @click="useRecommendation(r)" class="text-orange-600 hover:underline font-semibold">{{ $t('replenishment.recommendationsPanel.table.use') }}</button>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </div>

    <!-- Detail drawer -->
    <div v-if="selectedTask" class="fixed inset-0 bg-black/30 flex items-center justify-end z-50">
      <div class="bg-white h-full w-full max-w-md p-6 space-y-4 overflow-y-auto">
        <div class="flex items-center justify-between">
          <h2 class="text-lg font-bold text-slate-800">{{ selectedTask.task_no }}</h2>
          <button @click="selectedTask = null" class="text-slate-400 hover:text-slate-600">✕</button>
        </div>

        <p v-if="detailError" class="bg-red-50 border border-red-200 text-red-600 rounded-xl p-3 text-sm">{{ detailError }}</p>

        <div class="space-y-2 text-sm">
          <p><span class="text-slate-500">{{ $t('replenishment.detail.status') }}</span> <StatusBadge :tone="statusTone(selectedTask.status)">{{ statusText(selectedTask.status) }}</StatusBadge></p>
          <p><span class="text-slate-500">{{ $t('replenishment.detail.product') }}</span> {{ selectedTask.sku }} — {{ selectedTask.product_name }}</p>
          <p><span class="text-slate-500">{{ $t('replenishment.detail.lot') }}</span> {{ selectedTask.lot_no || '—' }}</p>
          <p><span class="text-slate-500">{{ $t('replenishment.detail.from') }}</span> {{ selectedTask.from_location_code }}</p>
          <p><span class="text-slate-500">{{ $t('replenishment.detail.to') }}</span> {{ selectedTask.to_location_code }}</p>
          <p><span class="text-slate-500">{{ $t('replenishment.detail.quantity') }}</span> {{ selectedTask.quantity }}</p>
          <p><span class="text-slate-500">{{ $t('replenishment.detail.warehouse') }}</span> {{ selectedTask.warehouse_code }}</p>
          <p v-if="selectedTask.remark"><span class="text-slate-500">{{ $t('replenishment.detail.remark') }}</span> {{ selectedTask.remark }}</p>
          <p v-if="selectedTask.issue_remark" class="text-red-600"><span class="text-slate-500">{{ $t('replenishment.detail.issue') }}</span> {{ selectedTask.issue_remark }}</p>
          <p v-if="selectedTask.cancel_remark" class="text-slate-600"><span class="text-slate-500">{{ $t('replenishment.detail.cancelRemark') }}</span> {{ selectedTask.cancel_remark }}</p>
          <p><span class="text-slate-500">{{ $t('replenishment.detail.createdBy') }}</span> {{ selectedTask.created_by_name || '—' }}</p>
        </div>

        <div v-if="canUpdate && selectedTask.status === 'PENDING'" class="flex gap-3">
          <button @click="doStart" :disabled="actionBusy" class="px-4 py-2 rounded-xl bg-slate-100 text-slate-700 font-semibold disabled:opacity-60">
            {{ $t('replenishment.detail.start') }}
          </button>
        </div>

        <div v-if="canUpdate && (selectedTask.status === 'PENDING' || selectedTask.status === 'IN_PROGRESS')">
          <button @click="doConfirm" :disabled="actionBusy" class="w-full px-4 py-2 rounded-xl bg-orange-600 hover:bg-orange-700 text-white font-semibold disabled:opacity-60">
            {{ actionBusy ? $t('replenishment.detail.confirming') : $t('replenishment.detail.confirmMove') }}
          </button>
        </div>

        <div v-if="canApprove && (selectedTask.status === 'PENDING' || selectedTask.status === 'IN_PROGRESS')" class="space-y-2 pt-2 border-t">
          <textarea v-model="cancelRemark" :placeholder="$t('replenishment.detail.cancelRemarkPlaceholder')" class="input" rows="2"></textarea>
          <button @click="doCancel" :disabled="actionBusy" class="w-full px-4 py-2 rounded-xl bg-slate-100 text-slate-700 font-semibold disabled:opacity-60">
            {{ $t('replenishment.detail.cancelTask') }}
          </button>

          <textarea v-model="issueRemark" :placeholder="$t('replenishment.detail.issueRemarkPlaceholder')" class="input" rows="2"></textarea>
          <button @click="doReportIssue" :disabled="actionBusy" class="w-full px-4 py-2 rounded-xl bg-red-50 hover:bg-red-100 text-red-600 font-semibold disabled:opacity-60">
            {{ $t('replenishment.detail.reportIssue') }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script>
import {
  listReplenishment, getReplenishmentTask, createReplenishmentTask,
  startReplenishmentTask, confirmReplenishmentTask, cancelReplenishmentTask,
  reportReplenishmentIssue, getReplenishmentRecommendations, newIdempotencyKey
} from '../../services/replenishment'
import { listWarehouses, listZones, listLocations } from '../../services/warehouses'
import { listProducts } from '../../services/products'
import { hasPermission } from '../../services/auth'
import PageHeader from '../../components/ui/PageHeader.vue'
import FilterBar from '../../components/ui/FilterBar.vue'
import DataTable from '../../components/ui/DataTable.vue'
import StatusBadge from '../../components/ui/StatusBadge.vue'

export default {
  name: 'ReplenishmentPage',
  components: { PageHeader, FilterBar, DataTable, StatusBadge },
  data() {
    return {
      loading: false,
      loadError: '',
      taskList: [],
      warehouses: [],
      filters: { search: '', status: '', warehouseId: '' },

      showNewForm: false,
      formError: '',
      submitting: false,
      form: { warehouseId: '', sku: '', lotNo: '', fromLocationId: '', toLocationId: '', quantity: null, remark: '' },
      formLocations: [],

      showRecommendations: false,
      recommendWarehouseId: '',
      recommendLoading: false,
      recommendError: '',
      recommendations: [],

      selectedTask: null,
      detailError: '',
      actionBusy: false,
      cancelRemark: '',
      issueRemark: ''
    }
  },
  computed: {
    canCreate() { return hasPermission('replenishment.create') },
    canUpdate() { return hasPermission('replenishment.update') },
    canApprove() { return hasPermission('replenishment.approve') }
  },
  async mounted() {
    await this.fetchWarehouses()
    await this.fetchTasks()
  },
  methods: {
    // Called from the template, not cached onto a row at map/load time,
    // so the displayed label stays reactive to the active locale —
    // switching language must not require re-fetching.
    statusText(status) {
      switch (status) {
        case 'PENDING': return this.$t('replenishment.status.pending')
        case 'IN_PROGRESS': return this.$t('replenishment.status.inProgress')
        case 'COMPLETED': return this.$t('replenishment.status.completed')
        case 'CANCELLED': return this.$t('replenishment.status.cancelled')
        case 'ISSUE': return this.$t('replenishment.status.issue')
        default: return status
      }
    },

    async fetchWarehouses() {
      try {
        const res = await listWarehouses({ limit: 100 })
        this.warehouses = res.data
      } catch (err) {
        this.loadError = err.message || this.$t('replenishment.messages.loadWarehousesFailed')
      }
    },

    async fetchTasks() {
      this.loading = true
      this.loadError = ''
      try {
        const res = await listReplenishment(this.filters)
        this.taskList = res.data
      } catch (err) {
        this.loadError = err.message || this.$t('replenishment.messages.loadTasksFailed')
      } finally {
        this.loading = false
      }
    },

    clearFilter() {
      this.filters = { search: '', status: '', warehouseId: '' }
      this.fetchTasks()
    },

    statusTone(status) {
      if (status === 'COMPLETED') return 'green'
      if (status === 'IN_PROGRESS') return 'orange'
      if (status === 'ISSUE' || status === 'CANCELLED') return 'gray'
      return 'gray'
    },

    openNewTask() {
      this.formError = ''
      this.form = { warehouseId: '', sku: '', lotNo: '', fromLocationId: '', toLocationId: '', quantity: null, remark: '' }
      this.formLocations = []
      this.showNewForm = true
    },

    async onFormWarehouseChange() {
      this.formLocations = []
      if (!this.form.warehouseId) return
      try {
        const zonesRes = await listZones(this.form.warehouseId)
        const allLocations = []
        for (const zone of zonesRes.data) {
          const locRes = await listLocations(zone.id)
          allLocations.push(...locRes.data)
        }
        this.formLocations = allLocations
      } catch (err) {
        this.formError = err.message || this.$t('replenishment.messages.loadLocationsFailed')
      }
    },

    async submitNewTask() {
      this.formError = ''
      if (!this.form.warehouseId || !this.form.fromLocationId || !this.form.toLocationId || !this.form.quantity) {
        this.formError = this.$t('replenishment.messages.requiredFields')
        return
      }
      const trimmedSku = (this.form.sku || '').trim()
      if (!trimmedSku) {
        this.formError = this.$t('replenishment.messages.skuRequired')
        return
      }

      this.submitting = true
      try {
        const productRes = await listProducts({ search: trimmedSku, limit: 20 })
        const product = productRes.data.find((p) => p.sku === trimmedSku || p.barcode === trimmedSku)
        if (!product) {
          this.formError = this.$t('replenishment.messages.noProductFound', { value: trimmedSku })
          return
        }

        await createReplenishmentTask({
          product_id: product.id,
          from_location_id: this.form.fromLocationId,
          to_location_id: this.form.toLocationId,
          lot_no: this.form.lotNo || '',
          quantity: this.form.quantity,
          warehouse_id: this.form.warehouseId,
          remark: this.form.remark || undefined
        })

        this.showNewForm = false
        await this.fetchTasks()
      } catch (err) {
        this.formError = err.message || this.$t('replenishment.messages.createTaskFailed')
      } finally {
        this.submitting = false
      }
    },

    openRecommendations() {
      this.recommendError = ''
      this.recommendations = []
      this.showRecommendations = true
    },

    async loadRecommendations() {
      if (!this.recommendWarehouseId) return
      this.recommendLoading = true
      this.recommendError = ''
      try {
        const res = await getReplenishmentRecommendations(this.recommendWarehouseId)
        this.recommendations = res.data
      } catch (err) {
        this.recommendError = err.message || this.$t('replenishment.messages.loadRecommendationsFailed')
      } finally {
        this.recommendLoading = false
      }
    },

    async useRecommendation(r) {
      this.showRecommendations = false
      this.formError = ''
      this.form = {
        warehouseId: this.recommendWarehouseId, sku: r.sku, lotNo: r.lot_no,
        fromLocationId: r.from_location_id, toLocationId: r.to_location_id,
        quantity: r.suggested_qty, remark: ''
      }
      await this.onFormWarehouseChange()
      this.showNewForm = true
    },

    async openDetail(id) {
      this.detailError = ''
      this.cancelRemark = ''
      this.issueRemark = ''
      try {
        const res = await getReplenishmentTask(id)
        this.selectedTask = res.data
      } catch (err) {
        this.loadError = err.message || this.$t('replenishment.messages.loadDetailFailed')
      }
    },

    async doStart() {
      this.actionBusy = true
      this.detailError = ''
      try {
        const res = await startReplenishmentTask(this.selectedTask.id)
        this.selectedTask = res.data
        await this.fetchTasks()
      } catch (err) {
        this.detailError = err.message
      } finally {
        this.actionBusy = false
      }
    },

    async doConfirm() {
      this.actionBusy = true
      this.detailError = ''
      try {
        const res = await confirmReplenishmentTask(this.selectedTask.id, { idempotency_key: newIdempotencyKey() })
        this.selectedTask = res.data
        await this.fetchTasks()
      } catch (err) {
        this.detailError = err.message
      } finally {
        this.actionBusy = false
      }
    },

    async doCancel() {
      this.actionBusy = true
      this.detailError = ''
      try {
        const res = await cancelReplenishmentTask(this.selectedTask.id, { remark: this.cancelRemark })
        this.selectedTask = res.data
        await this.fetchTasks()
      } catch (err) {
        this.detailError = err.message
      } finally {
        this.actionBusy = false
      }
    },

    async doReportIssue() {
      if (!this.issueRemark.trim()) {
        this.detailError = this.$t('replenishment.messages.issueRemarkRequired')
        return
      }
      this.actionBusy = true
      this.detailError = ''
      try {
        const res = await reportReplenishmentIssue(this.selectedTask.id, { remark: this.issueRemark })
        this.selectedTask = res.data
        await this.fetchTasks()
      } catch (err) {
        this.detailError = err.message
      } finally {
        this.actionBusy = false
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
</style>
