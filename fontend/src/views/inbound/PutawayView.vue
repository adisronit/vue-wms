<template>
  <div class="space-y-6">
    <div class="flex flex-col xl:flex-row xl:items-center xl:justify-between gap-4">
      <div>
        <h1 class="text-2xl font-bold text-slate-800">{{ $t('putaway.title') }}</h1>
        <p class="text-slate-500">
          {{ $t('putaway.subtitle') }}
        </p>
      </div>

      <div class="flex gap-3">
        <button
          @click="exportCsv"
          :disabled="exporting"
          class="bg-slate-100 hover:bg-slate-200 text-slate-700 px-5 py-3 rounded-xl font-semibold disabled:opacity-60"
        >
          {{ exporting ? $t('common.buttons.exporting') : '⬇ ' + $t('common.buttons.exportCsv') }}
        </button>

        <button
          v-if="canCreate"
          @click="$router.push('/inbound/putaway/new')"
          class="bg-orange-600 hover:bg-orange-700 text-white px-5 py-3 rounded-xl font-semibold"
        >
          {{ $t('putaway.newButton') }}
        </button>
      </div>
    </div>

    <div class="grid grid-cols-1 md:grid-cols-4 gap-5">
      <div class="summary-card">
        <p class="summary-label">{{ $t('putaway.summary.pendingPutaway') }}</p>
        <h2 class="summary-value">{{ pendingCount }}</h2>
      </div>

      <div class="summary-card">
        <p class="summary-label">{{ $t('common.status.inProgress') }}</p>
        <h2 class="summary-value">{{ inProgressCount }}</h2>
      </div>

      <div class="summary-card">
        <p class="summary-label">{{ $t('common.status.completed') }}</p>
        <h2 class="summary-value">{{ completedCount }}</h2>
      </div>

      <div class="summary-card">
        <p class="summary-label">{{ $t('putaway.summary.locationIssues') }}</p>
        <h2 class="summary-value text-red-600">{{ issueCount }}</h2>
      </div>
    </div>

    <div v-if="loadError" class="bg-red-50 border border-red-200 text-red-600 rounded-xl p-3 text-sm">
      {{ loadError }}
    </div>

    <div class="bg-white rounded-2xl border border-slate-200 p-5">
      <div class="grid grid-cols-1 md:grid-cols-4 gap-4">
        <input
          v-model="filters.keyword"
          type="text"
          :placeholder="$t('putaway.filters.searchPlaceholder')"
          class="input"
        />

        <select v-model="filters.status" class="input">
          <option value="">{{ $t('putaway.filters.allStatus') }}</option>
          <option value="PENDING">{{ $t('common.status.pending') }}</option>
          <option value="PARTIALLY_COMPLETED">{{ $t('common.status.inProgress') }}</option>
          <option value="COMPLETED">{{ $t('common.status.completed') }}</option>
          <option value="ISSUE">{{ $t('putaway.statusLabels.issue') }}</option>
        </select>

        <select v-model="filters.zoneId" class="input">
          <option value="">{{ $t('putaway.filters.allZones') }}</option>
          <option v-for="z in zones" :key="z.id" :value="z.id">{{ z.code }}</option>
        </select>

        <button
          @click="clearFilter"
          class="bg-slate-100 hover:bg-slate-200 text-slate-700 rounded-xl font-semibold"
        >
          {{ $t('common.buttons.clearFilter') }}
        </button>
      </div>
    </div>

    <div class="bg-white rounded-2xl border border-slate-200 shadow-sm overflow-hidden">
      <div class="w-full overflow-x-auto">
        <table class="min-w-[1150px] w-full text-sm">
          <thead class="bg-slate-50 text-slate-500">
            <tr>
              <th class="table-th">{{ $t('putaway.table.taskNo') }}</th>
              <th class="table-th">{{ $t('putaway.table.receivingNo') }}</th>
              <th class="table-th">{{ $t('putaway.table.sku') }}</th>
              <th class="table-th">{{ $t('putaway.table.product') }}</th>
              <th class="table-th">{{ $t('putaway.table.qty') }}</th>
              <th class="table-th">{{ $t('putaway.table.fromLocation') }}</th>
              <th class="table-th">{{ $t('putaway.table.suggestedBin') }}</th>
              <th class="table-th">{{ $t('putaway.table.actualBin') }}</th>
              <th class="table-th">{{ $t('common.labels.status') }}</th>
              <th class="table-th text-right">{{ $t('putaway.table.action') }}</th>
            </tr>
          </thead>

          <tbody>
            <tr v-if="loading">
              <td colspan="10" class="text-center py-10 text-slate-400">{{ $t('putaway.loadingTasks') }}</td>
            </tr>

            <tr v-else-if="putawayList.length === 0">
              <td colspan="10" class="text-center py-10 text-slate-400">{{ $t('putaway.noTasks') }}</td>
            </tr>

            <template v-else>
            <tr
              v-for="item in putawayList"
              :key="item.id"
              class="border-t hover:bg-orange-50"
            >
              <td class="table-td font-semibold text-slate-800">
                {{ item.taskNo }}
              </td>
              <td class="table-td">{{ item.receivingNo }}</td>
              <td class="table-td">{{ item.sku }}</td>
              <td class="table-td">{{ item.productName }}</td>
              <td class="table-td">{{ item.qty }}</td>
              <td class="table-td">{{ item.fromLocation }}</td>
              <td class="table-td">{{ item.suggestedBin }}</td>
              <td class="table-td">{{ item.actualBin || '-' }}</td>
              <td class="table-td">
                <span :class="getStatusClass(item.status)">
                  {{ statusText(item.status) }}
                </span>
              </td>
              <td class="table-td text-right">
                <button
                  @click="openPutaway(item)"
                  class="text-orange-600 hover:text-orange-700 font-semibold"
                >
                  {{ $t('putaway.table.putawayAction') }}
                </button>
              </td>
            </tr>
            </template>
          </tbody>
        </table>
      </div>
    </div>

    <div
      v-if="selectedPutaway"
      class="fixed inset-0 bg-black/40 flex justify-end z-50"
    >
      <div class="w-full max-w-2xl bg-white h-full p-6 overflow-y-auto">
        <div class="flex items-center justify-between mb-6">
          <div>
            <h2 class="text-xl font-bold text-slate-800">
              {{ $t('putaway.drawer.title') }}
            </h2>
            <p class="text-slate-500">
              {{ selectedPutaway.taskNo }} / {{ selectedPutaway.sku }}
            </p>
          </div>

          <button
            @click="selectedPutaway = null"
            class="text-slate-500 hover:text-red-600 text-xl"
          >
            ✕
          </button>
        </div>

        <div class="bg-slate-50 rounded-2xl p-5 mb-6">
          <div class="grid grid-cols-2 gap-4">
            <div>
              <p class="detail-label">{{ $t('putaway.drawer.product') }}</p>
              <p class="detail-value">{{ selectedPutaway.productName }}</p>
            </div>

            <div>
              <p class="detail-label">{{ $t('putaway.drawer.quantity') }}</p>
              <p class="detail-value">{{ selectedPutaway.qty }}</p>
            </div>

            <div>
              <p class="detail-label">{{ $t('putaway.table.fromLocation') }}</p>
              <p class="detail-value">{{ selectedPutaway.fromLocation }}</p>
            </div>

            <div>
              <p class="detail-label">{{ $t('putaway.table.suggestedBin') }}</p>
              <p class="detail-value text-orange-600">
                {{ selectedPutaway.suggestedBin }}
              </p>
            </div>
          </div>
        </div>

        <!-- Cancellation details (Phase 18) — task-level, since a
             cancelled task's own status lives on the header, not this
             line's own item-level status (see openPutaway's taskStatus). -->
        <div v-if="selectedPutaway.taskStatus === 'CANCELLED'" class="bg-red-50 border border-red-200 rounded-2xl p-5 mb-6">
          <div class="grid grid-cols-2 gap-4">
            <div>
              <p class="detail-label">{{ $t('putaway.drawer.cancelledAt') }}</p>
              <p class="detail-value">{{ formatDateTime(selectedPutaway.cancelledAt) }}</p>
            </div>
            <div>
              <p class="detail-label">{{ $t('putaway.drawer.cancelledBy') }}</p>
              <p class="detail-value">{{ selectedPutaway.cancelledByName || '—' }}</p>
            </div>
            <div class="col-span-2">
              <p class="detail-label">{{ $t('putaway.drawer.cancellationRemark') }}</p>
              <p class="detail-value">{{ selectedPutaway.cancelRemark || '—' }}</p>
            </div>
          </div>
        </div>

        <div v-if="drawerError" class="bg-red-50 border border-red-200 text-red-600 rounded-xl p-3 text-sm mb-4">
          {{ drawerError }}
        </div>

        <p v-if="isDrawerItemResolved" class="text-sm text-green-600 font-semibold mb-4">
          {{ $t('putaway.drawer.alreadyResolved', { status: statusText(selectedPutaway.status) }) }}
        </p>

        <p v-else-if="isTaskTerminal" class="text-sm text-slate-500 font-semibold mb-4">
          {{ taskTerminalMessage }}
        </p>

        <template v-else-if="isTaskActionable">
          <div v-if="canConfirm" class="space-y-4">
            <div>
              <label class="label">{{ $t('putaway.drawer.scanSkuBarcode') }}</label>
              <input
                v-model="putawayForm.barcode"
                type="text"
                class="input"
                :placeholder="$t('putaway.drawer.scanSkuPlaceholder')"
              />
            </div>

            <div>
              <label class="label">{{ $t('putaway.drawer.actualBinLocation') }}</label>
              <input
                v-model="putawayForm.actualBin"
                type="text"
                class="input"
                :placeholder="$t('putaway.drawer.actualBinPlaceholder')"
              />
            </div>

            <div>
              <label class="label">{{ $t('putaway.drawer.putawayQty') }}</label>
              <input
                v-model="putawayForm.qty"
                type="number"
                class="input"
                :placeholder="$t('putaway.drawer.putawayQtyPlaceholder')"
              />
            </div>

            <div>
              <label class="label">{{ $t('common.labels.remark') }}</label>
              <textarea
                v-model="putawayForm.remark"
                rows="4"
                class="input"
                :placeholder="$t('putaway.drawer.remarkPlaceholder')"
              ></textarea>
            </div>

            <div class="grid grid-cols-1 md:grid-cols-2 gap-3 pt-4">
              <button
                @click="confirmPutaway"
                :disabled="submitting"
                class="bg-orange-600 hover:bg-orange-700 text-white py-3 rounded-xl font-semibold disabled:opacity-60"
              >
                {{ submitting ? $t('common.buttons.submitting') : $t('putaway.drawer.confirmButton') }}
              </button>

              <button
                @click="markIssue"
                :disabled="submitting"
                class="bg-red-600 hover:bg-red-700 text-white py-3 rounded-xl font-semibold disabled:opacity-60"
              >
                {{ $t('putaway.drawer.reportIssue') }}
              </button>
            </div>
          </div>
          <p v-else class="text-sm text-slate-500">
            {{ $t('putaway.drawer.viewOnlyNote') }}
          </p>

          <div v-if="canCancelPutaway" class="mt-6 pt-6 border-t border-slate-200">
            <input
              v-model="cancelRemark"
              type="text"
              class="input mb-3"
              :placeholder="$t('putaway.drawer.cancelRemarkPlaceholder')"
            />
            <button
              @click="cancelDrawerTask"
              :disabled="submitting"
              class="w-full bg-slate-800 hover:bg-slate-900 text-white py-3 rounded-xl font-semibold disabled:opacity-60"
            >
              {{ $t('putaway.drawer.cancelButton') }}
            </button>
          </div>
        </template>
      </div>
    </div>
  </div>
</template>

<script>
import { listPutaway, getPutawayTask, confirmPutawayTask, cancelPutawayTask, newIdempotencyKey, exportPutaway } from '../../services/putaway'
import { listWarehouses, listZones, listLocations } from '../../services/warehouses'
import { hasPermission } from '../../services/auth'
import i18n from '../../i18n'

const { t } = i18n.global
let searchDebounce = null

// Called from the template (methods.statusText below), not cached onto
// a row at map time, so the displayed label stays reactive to the
// active locale — switching language must not require re-fetching.
function statusText(status) {
  switch (status) {
    case 'PENDING': return t('common.status.pending')
    case 'PARTIALLY_COMPLETED': return t('common.status.inProgress')
    case 'COMPLETED': return t('common.status.completed')
    case 'ISSUE': return t('putaway.statusLabels.issue')
    default: return status
  }
}

// Real backend field names mapped onto the mock's existing display
// field names so the template needs no further changes. There is no
// bin-suggestion algorithm in this codebase (see PROJECT_STATUS.md),
// so suggestedBin is always "—"; destination_location_code is what the
// mock's "Actual Bin" column shows.
function mapPutawayRow(r) {
  return {
    id: r.item_id,
    taskId: r.task_id,
    taskNo: r.putaway_no,
    receivingNo: r.receipt_no || '—',
    sku: r.sku,
    productName: r.product_name,
    qty: r.requested_qty,
    fromLocation: r.source_location_code,
    suggestedBin: '—',
    actualBin: r.destination_location_code,
    status: r.status
  }
}

export default {
  name: 'PutawayView',

  data() {
    return {
      loading: false,
      loadError: '',
      exporting: false,
      putawayList: [],
      allItems: [],
      zones: [],

      selectedPutaway: null,
      drawerError: '',
      drawerTaskId: null,
      drawerLocations: [],
      submitting: false,
      cancelRemark: '',

      filters: {
        keyword: '',
        status: '',
        zoneId: ''
      },

      putawayForm: {
        barcode: '',
        actualBin: '',
        qty: '',
        remark: ''
      }
    }
  },

  computed: {
    pendingCount() {
      return this.allItems.filter((r) => r.status === 'PENDING').length
    },
    inProgressCount() {
      return this.allItems.filter((r) => r.status === 'PARTIALLY_COMPLETED').length
    },
    completedCount() {
      return this.allItems.filter((r) => r.status === 'COMPLETED').length
    },
    issueCount() {
      return this.allItems.filter((r) => r.status === 'ISSUE').length
    },
    canCreate() {
      return hasPermission('putaway.create')
    },
    canConfirm() {
      return hasPermission('putaway.approve')
    },
    // Cancel is gated on putaway.update, not putaway.approve — see the
    // backend's own internal/putaway/handler.go doc: Service.Cancel's
    // own per-line MovedQty check is the real safety gate, so the
    // permission sits at editing's authority level.
    canCancelPutaway() {
      return hasPermission('putaway.update')
    },
    isDrawerItemResolved() {
      return this.selectedPutaway && (this.selectedPutaway.status === 'COMPLETED' || this.selectedPutaway.status === 'ISSUE')
    },
    // Phase 19: the backend task header status (fetched by openPutaway,
    // not the per-line item status isDrawerItemResolved already checks
    // above) is the source of truth for whether Confirm/Report Issue/
    // Cancel may run at all — mirrors internal/putaway/model.go's
    // Status.CanConfirm()/Status.CanCancel(), which both only allow
    // PENDING/IN_PROGRESS (COMPLETED/ISSUE/CANCELLED are all terminal).
    // taskStatus is undefined until openPutaway's async fetch resolves,
    // so this stays false (not yet actionable) until we actually know.
    isTaskActionable() {
      const status = this.selectedPutaway && this.selectedPutaway.taskStatus
      return status === 'PENDING' || status === 'IN_PROGRESS'
    },
    isTaskTerminal() {
      const status = this.selectedPutaway && this.selectedPutaway.taskStatus
      return !!status && !this.isTaskActionable
    },
    taskTerminalMessage() {
      const status = this.selectedPutaway && this.selectedPutaway.taskStatus
      if (status === 'CANCELLED') {
        return this.$t('putaway.drawer.terminalCancelled')
      }
      return this.$t('putaway.drawer.terminalMessage', { status: statusText(status) })
    }
  },

  watch: {
    'filters.keyword'() {
      clearTimeout(searchDebounce)
      searchDebounce = setTimeout(() => this.fetchPutaway(), 300)
    },
    'filters.status'() {
      this.fetchPutaway()
    },
    'filters.zoneId'() {
      this.fetchPutaway()
    }
  },

  mounted() {
    this.fetchPutaway()
    this.fetchSummary()
    this.fetchZones()
  },

  methods: {
    statusText,

    async fetchPutaway() {
      this.loading = true
      this.loadError = ''
      try {
        const res = await listPutaway({
          search: this.filters.keyword,
          status: this.filters.status,
          zoneId: this.filters.zoneId,
          limit: 100
        })
        this.putawayList = res.data.map(mapPutawayRow)
      } catch (err) {
        this.loadError = err.message
      } finally {
        this.loading = false
      }
    },

    // A separate, unfiltered fetch backing the summary cards — no
    // dedicated stats endpoint this phase, the same approach
    // ReceiveView.vue uses.
    async fetchSummary() {
      try {
        const res = await listPutaway({ limit: 200 })
        this.allItems = res.data
      } catch {
        // Non-critical: the table's own error banner already surfaces API failures.
      }
    },

    // No dedicated "all zones" endpoint (listZones is always scoped to
    // one warehouse) — the Zone filter fetches every warehouse's zones
    // and flattens them, real data replacing the mock's hardcoded A/B/C.
    async fetchZones() {
      try {
        const whRes = await listWarehouses({ limit: 100 })
        const allZones = []
        for (const wh of whRes.data) {
          const zRes = await listZones(wh.id)
          allZones.push(...zRes.data)
        }
        this.zones = allZones
      } catch {
        // Non-critical: the zone filter simply stays empty.
      }
    },

    clearFilter() {
      this.filters.keyword = ''
      this.filters.status = ''
      this.filters.zoneId = ''
      this.fetchPutaway()
    },

    async exportCsv() {
      this.exporting = true
      try {
        await exportPutaway({ search: this.filters.keyword, status: this.filters.status, zoneId: this.filters.zoneId })
      } catch (err) {
        this.loadError = err.message
      } finally {
        this.exporting = false
      }
    },

    async openPutaway(item) {
      this.selectedPutaway = item
      this.drawerTaskId = item.taskId
      this.drawerError = ''
      this.drawerLocations = []
      this.cancelRemark = ''
      this.putawayForm = {
        barcode: item.sku,
        actualBin: item.actualBin !== '—' ? item.actualBin : '',
        qty: item.qty,
        remark: ''
      }

      try {
        const res = await getPutawayTask(item.taskId)
        await this.fetchLocationsForWarehouse(res.data.warehouse_id)
        this.selectedPutaway = {
          ...item,
          taskStatus: res.data.status,
          cancelledAt: res.data.cancelled_at,
          cancelledByName: res.data.cancelled_by_name,
          cancelRemark: res.data.cancel_remark
        }
      } catch (err) {
        this.drawerError = err.message
      }
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

    async confirmPutaway() {
      if (!this.putawayForm.barcode) {
        this.drawerError = this.$t('putaway.messages.barcodeRequired')
        return
      }
      if (!this.putawayForm.actualBin) {
        this.drawerError = this.$t('putaway.messages.actualBinRequired')
        return
      }
      if (!this.putawayForm.qty || this.putawayForm.qty <= 0) {
        this.drawerError = this.$t('putaway.messages.putawayQtyRequired')
        return
      }

      const destination = this.drawerLocations.find((l) => l.code === this.putawayForm.actualBin.trim())
      if (!destination) {
        this.drawerError = this.$t('putaway.messages.noLocationFound', { value: this.putawayForm.actualBin })
        return
      }

      await this.submitConfirm({
        item_id: this.selectedPutaway.id,
        action: 'CONFIRMED',
        quantity: Number(this.putawayForm.qty),
        destination_location_id: destination.id,
        remark: this.putawayForm.remark || ''
      })
    },

    async markIssue() {
      if (!this.putawayForm.remark) {
        this.drawerError = this.$t('putaway.messages.remarkRequiredForIssue')
        return
      }

      await this.submitConfirm({
        item_id: this.selectedPutaway.id,
        action: 'ISSUE',
        remark: this.putawayForm.remark
      })
    },

    async submitConfirm(confirmItem) {
      this.submitting = true
      this.drawerError = ''
      try {
        await confirmPutawayTask(this.drawerTaskId, {
          idempotency_key: newIdempotencyKey(),
          items: [confirmItem]
        })
        alert(confirmItem.action === 'ISSUE' ? this.$t('putaway.messages.issueReported') : this.$t('putaway.messages.putawayConfirmed'))
        this.selectedPutaway = null
        await this.fetchPutaway()
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
        await cancelPutawayTask(this.drawerTaskId, {
          remark: this.cancelRemark || undefined
        })
        alert(this.$t('putaway.messages.taskCancelled'))
        this.selectedPutaway = null
        await this.fetchPutaway()
        await this.fetchSummary()
      } catch (err) {
        this.drawerError = err.message
      } finally {
        this.submitting = false
      }
    },

    getStatusClass(status) {
      if (status === 'PARTIALLY_COMPLETED') return 'badge-orange'
      if (status === 'COMPLETED') return 'badge-green'
      if (status === 'ISSUE') return 'badge-red'
      return 'badge-gray'
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

.label {
  display: block;
  margin-bottom: 8px;
  color: #334155;
  font-weight: 600;
  font-size: 14px;
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
  font-size: 30px;
  font-weight: 800;
  color: #0f172a;
  margin-top: 8px;
}

.table-th {
  padding: 14px 16px;
  font-weight: 700;
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

.badge-gray {
  background: #f1f5f9;
  color: #475569;
  padding: 5px 10px;
  border-radius: 999px;
  font-weight: 600;
  font-size: 12px;
}

.badge-orange {
  background: #ffedd5;
  color: #ea580c;
  padding: 5px 10px;
  border-radius: 999px;
  font-weight: 600;
  font-size: 12px;
}

.badge-green {
  background: #dcfce7;
  color: #16a34a;
  padding: 5px 10px;
  border-radius: 999px;
  font-weight: 600;
  font-size: 12px;
}

.badge-red {
  background: #fee2e2;
  color: #dc2626;
  padding: 5px 10px;
  border-radius: 999px;
  font-weight: 600;
  font-size: 12px;
}
</style>
