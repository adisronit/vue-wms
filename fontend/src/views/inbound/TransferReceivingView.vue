<template>
  <div class="space-y-6">
    <PageHeader :title="$t('transferReceiving.title')" :subtitle="$t('transferReceiving.subtitle')">
      <template #actions>
        <button
          v-if="canCreate"
          @click="openReceipt(null)"
          :disabled="!filters.destinationWarehouseId"
          class="bg-orange-600 hover:bg-orange-700 text-white px-5 py-3 rounded-xl font-semibold disabled:opacity-60"
          :title="!filters.destinationWarehouseId ? $t('transferReceiving.selectWarehouseFirst') : ''"
        >
          {{ $t('transferReceiving.newButton') }}
        </button>
      </template>
    </PageHeader>

    <div class="grid grid-cols-1 md:grid-cols-3 gap-5">
      <div class="summary-card">
        <p class="summary-label">{{ $t('common.status.draft') }}</p>
        <h2 class="summary-value">{{ draftCount }}</h2>
      </div>

      <div class="summary-card">
        <p class="summary-label">{{ $t('common.status.completed') }}</p>
        <h2 class="summary-value text-green-600">{{ completedCount }}</h2>
      </div>

      <div class="summary-card">
        <p class="summary-label">{{ $t('common.status.cancelled') }}</p>
        <h2 class="summary-value">{{ cancelledCount }}</h2>
      </div>
    </div>

    <div v-if="loadError" class="bg-red-50 border border-red-200 text-red-600 rounded-xl p-3 text-sm">
      {{ loadError }}
    </div>

    <FilterBar>
      <input
        v-model="filters.keyword"
        type="text"
        :placeholder="$t('transferReceiving.filters.searchPlaceholder')"
        class="input"
      />

      <select v-model="filters.status" class="input">
        <option value="">{{ $t('transferReceiving.filters.allStatus') }}</option>
        <option value="DRAFT">{{ $t('common.status.draft') }}</option>
        <option value="COMPLETED">{{ $t('common.status.completed') }}</option>
        <option value="CANCELLED">{{ $t('common.status.cancelled') }}</option>
      </select>

      <select v-model="filters.destinationWarehouseId" class="input">
        <option value="">{{ $t('transferReceiving.filters.selectDestinationWarehouse') }}</option>
        <option v-for="w in warehouses" :key="w.id" :value="w.id">{{ w.code }}</option>
      </select>

      <label class="flex items-center gap-2 text-sm text-slate-600 whitespace-nowrap">
        <input type="checkbox" v-model="filters.hasDiscrepancy" />
        {{ $t('transferReceiving.filters.hasDiscrepancyOnly') }}
      </label>

      <button
        @click="clearFilter"
        class="bg-slate-100 hover:bg-slate-200 text-slate-700 rounded-xl font-semibold"
      >
        {{ $t('common.buttons.clearFilter') }}
      </button>
    </FilterBar>

    <p v-if="!filters.destinationWarehouseId" class="text-sm text-slate-500">
      {{ $t('transferReceiving.selectWarehouseHint') }}
    </p>

    <DataTable
      v-else
      :loading="loading"
      :is-empty="receiptList.length === 0"
      :loading-text="$t('transferReceiving.loadingReceipts')"
      :empty-text="$t('transferReceiving.noReceipts')"
      :colspan="7"
      min-width="1100px"
    >
      <template #head>
        <tr>
          <th class="table-th">{{ $t('transferReceiving.table.receiptNo') }}</th>
          <th class="table-th">{{ $t('transferReceiving.table.shipmentNo') }}</th>
          <th class="table-th">{{ $t('transferReceiving.table.orderNo') }}</th>
          <th class="table-th">{{ $t('transferReceiving.table.transferReference') }}</th>
          <th class="table-th">{{ $t('transferReceiving.table.destinationWarehouse') }}</th>
          <th class="table-th">{{ $t('common.labels.status') }}</th>
          <th class="table-th text-right">{{ $t('transferReceiving.table.action') }}</th>
        </tr>
      </template>

      <tr
        v-for="item in receiptList"
        :key="item.id"
        class="border-t hover:bg-orange-50"
      >
        <td class="table-td font-semibold text-slate-800">{{ item.receipt_no }}</td>
        <td class="table-td">{{ item.shipment_no }}</td>
        <td class="table-td">{{ item.order_no }}</td>
        <td class="table-td">{{ item.transfer_reference || '—' }}</td>
        <td class="table-td">{{ item.destination_warehouse_code }}</td>
        <td class="table-td">
          <StatusBadge :tone="statusTone(item.status)">{{ statusText(item.status) }}</StatusBadge>
        </td>
        <td class="table-td text-right">
          <button @click="openReceipt(item)" class="text-orange-600 hover:text-orange-700 font-semibold">
            {{ $t('common.buttons.view') }}
          </button>
        </td>
      </tr>
    </DataTable>

    <div v-if="selectedReceipt" class="fixed inset-0 bg-black/40 flex justify-end z-50">
      <div class="w-full max-w-3xl bg-white h-full p-6 overflow-y-auto">
        <div class="flex items-center justify-between mb-6">
          <div>
            <h2 class="text-xl font-bold text-slate-800">
              {{ isNewReceipt ? $t('transferReceiving.drawer.newReceiptTitle') : $t('transferReceiving.drawer.detailTitle') }}
            </h2>
            <p v-if="!isNewReceipt" class="text-slate-500">
              {{ selectedReceipt.receipt_no }} · {{ selectedReceipt.destination_warehouse_code }}
            </p>
          </div>

          <div v-if="!isNewReceipt" class="flex gap-2 items-center">
            <button @click="viewTransferNotePdf" class="bg-slate-100 hover:bg-slate-200 text-slate-700 px-3 py-2 rounded-lg text-sm font-semibold">
              {{ $t('transferReceiving.drawer.viewTransferNote') }}
            </button>
            <button @click="downloadTransferNotePdf" class="bg-slate-100 hover:bg-slate-200 text-slate-700 px-3 py-2 rounded-lg text-sm font-semibold">
              {{ $t('transferReceiving.drawer.downloadTransferNote') }}
            </button>
            <button @click="showHistory = true" class="bg-slate-100 hover:bg-slate-200 text-slate-700 px-3 py-2 rounded-lg text-sm font-semibold">
              📜 {{ $t('transferReceiving.drawer.historyButton') }}
            </button>
          </div>

          <button @click="closeDrawer" class="text-slate-500 hover:text-red-600 text-xl">✕</button>
        </div>

        <DocumentHistoryPanel
          v-if="!isNewReceipt"
          :open="showHistory"
          document-type="TRANSFER_NOTE"
          :source-id="selectedReceipt.id"
          :title="$t('transferReceiving.drawer.historyButton')"
          @close="showHistory = false"
        />

        <div v-if="!isNewReceipt" class="mb-6">
          <StatusBadge :tone="statusTone(selectedReceipt.status)">{{ statusText(selectedReceipt.status) }}</StatusBadge>
        </div>

        <div v-if="!isNewReceipt && reconciliation" class="bg-slate-50 rounded-2xl p-5 mb-6">
          <div class="flex items-center justify-between mb-3">
            <h3 class="font-bold text-slate-800">{{ $t('transferReceiving.reconciliation.title') }}</h3>
            <StatusBadge :tone="reconciliationStatusTone(reconciliation.reconciliation_status)">
              {{ reconciliationStatusText(reconciliation.reconciliation_status) }}
            </StatusBadge>
          </div>
          <div class="grid grid-cols-3 gap-4">
            <div>
              <p class="detail-label">{{ $t('transferReceiving.reconciliation.totalDispatched') }}</p>
              <p class="detail-value">{{ reconciliation.total_dispatched_qty }}</p>
            </div>
            <div>
              <p class="detail-label">{{ $t('transferReceiving.reconciliation.totalReceived') }}</p>
              <p class="detail-value">{{ reconciliation.total_received_qty }}</p>
            </div>
            <div>
              <p class="detail-label">{{ $t('transferReceiving.reconciliation.totalDiscrepancy') }}</p>
              <p class="detail-value" :class="{ 'text-red-600': reconciliation.total_discrepancy_qty > 0 }">
                {{ reconciliation.total_discrepancy_qty }}
              </p>
            </div>
          </div>
        </div>

        <div v-if="drawerError" class="bg-red-50 border border-red-200 text-red-600 rounded-xl p-3 text-sm mb-4">
          {{ drawerError }}
        </div>

        <!-- New receipt: eligible shipment selector -->
        <div v-if="isNewReceipt" class="bg-slate-50 rounded-2xl p-5 mb-6">
          <label class="label">{{ $t('transferReceiving.drawer.sourceShipment') }}</label>
          <select v-model="form.shipmentId" class="input">
            <option value="">{{ $t('transferReceiving.drawer.selectSourceShipment') }}</option>
            <option v-for="src in eligibleShipments" :key="src.shipment_id" :value="src.shipment_id">
              {{ src.shipment_no }} / {{ src.order_no }}
              <template v-if="src.transfer_reference">({{ src.transfer_reference }})</template>
              — {{ src.source_warehouse_code }} → {{ $t('transferReceiving.drawer.itemCountSuffix', { count: src.item_count }) }}
            </option>
          </select>
          <p v-if="eligibleShipments.length === 0" class="text-xs text-slate-400 mt-2">
            {{ $t('transferReceiving.drawer.noSourceShipments') }}
          </p>

          <label class="label mt-4">{{ $t('common.labels.remark') }}</label>
          <textarea v-model="form.remark" rows="2" class="input"></textarea>

          <button
            @click="createReceipt"
            :disabled="submitting"
            class="mt-4 w-full bg-orange-600 hover:bg-orange-700 text-white py-3 rounded-xl font-semibold disabled:opacity-60"
          >
            {{ submitting ? $t('common.buttons.submitting') : $t('transferReceiving.drawer.createButton') }}
          </button>
        </div>

        <!-- Existing receipt: read-only header summary -->
        <template v-else>
          <div class="bg-slate-50 rounded-2xl p-5 mb-6">
            <div class="grid grid-cols-2 gap-4">
              <div>
                <p class="detail-label">{{ $t('transferReceiving.table.shipmentNo') }}</p>
                <p class="detail-value">{{ selectedReceipt.shipment_no }}</p>
              </div>
              <div>
                <p class="detail-label">{{ $t('transferReceiving.table.orderNo') }}</p>
                <p class="detail-value">{{ selectedReceipt.order_no }}</p>
              </div>
              <div v-if="selectedReceipt.transfer_reference">
                <p class="detail-label">{{ $t('transferReceiving.table.transferReference') }}</p>
                <p class="detail-value">{{ selectedReceipt.transfer_reference }}</p>
              </div>
              <div>
                <p class="detail-label">{{ $t('transferReceiving.table.destinationWarehouse') }}</p>
                <p class="detail-value">{{ selectedReceipt.destination_warehouse_code }}</p>
              </div>
              <div v-if="selectedReceipt.received_at">
                <p class="detail-label">{{ $t('transferReceiving.drawer.receivedAt') }}</p>
                <p class="detail-value">{{ formatDateTime(selectedReceipt.received_at) }}</p>
              </div>
              <div v-if="selectedReceipt.received_by_name">
                <p class="detail-label">{{ $t('transferReceiving.drawer.receivedBy') }}</p>
                <p class="detail-value">{{ selectedReceipt.received_by_name }}</p>
              </div>
              <div class="col-span-2">
                <p class="detail-label">{{ $t('common.labels.remark') }}</p>
                <p class="detail-value">{{ selectedReceipt.remark || '—' }}</p>
              </div>
            </div>
          </div>

          <div v-if="selectedReceipt.status === 'CANCELLED'" class="bg-red-50 border border-red-200 rounded-2xl p-5 mb-6">
            <div class="grid grid-cols-2 gap-4">
              <div>
                <p class="detail-label">{{ $t('transferReceiving.drawer.cancelledAt') }}</p>
                <p class="detail-value">{{ formatDateTime(selectedReceipt.cancelled_at) }}</p>
              </div>
              <div>
                <p class="detail-label">{{ $t('transferReceiving.drawer.cancelledBy') }}</p>
                <p class="detail-value">{{ selectedReceipt.cancelled_by_name || '—' }}</p>
              </div>
              <div class="col-span-2">
                <p class="detail-label">{{ $t('transferReceiving.drawer.cancellationRemark') }}</p>
                <p class="detail-value">{{ selectedReceipt.cancel_remark || '—' }}</p>
              </div>
            </div>
          </div>

          <h3 class="font-bold text-slate-800 mb-3">{{ $t('transferReceiving.drawer.lines') }}</h3>
          <div class="space-y-3 mb-6">
            <div
              v-for="line in selectedReceipt.items"
              :key="line.id"
              class="border border-slate-200 rounded-2xl p-4"
              :class="lineRowClass(line)"
            >
              <div class="flex items-center justify-between mb-2">
                <p class="font-semibold text-slate-800">{{ line.sku }} — {{ line.product_name }}</p>
                <div class="flex items-center gap-2">
                  <span
                    v-if="reconciliationLineMap[line.id]"
                    class="text-xs font-bold px-2 py-0.5 rounded"
                    :class="reconciliationLineMap[line.id].fully_reconciled ? 'text-green-700 bg-green-50' : 'text-red-700 bg-red-50'"
                  >
                    {{ reconciliationLineMap[line.id].fully_reconciled
                      ? $t('transferReceiving.reconciliation.fullyReconciled')
                      : $t('transferReceiving.reconciliation.notFullyReconciled') }}
                  </span>
                  <StatusBadge :tone="lineStatusTone(line.status)">{{ lineStatusText(line.status) }}</StatusBadge>
                </div>
              </div>
              <p class="text-sm text-slate-500 mb-3">
                {{ $t('transferReceiving.drawer.dispatchedReceived') }}: {{ line.received_qty }} / {{ line.dispatched_qty }} {{ line.uom }}
                <span v-if="line.lot_no"> · {{ $t('transferReceiving.table.lotNo') }}: {{ line.lot_no }}</span>
                <span v-if="line.location_code"> · {{ $t('common.labels.location') }}: {{ line.location_code }}</span>
              </p>
              <p v-if="line.discrepancy_qty" class="text-sm text-red-600 mb-3">
                {{ $t('transferReceiving.drawer.discrepancy') }}: {{ line.discrepancy_qty }} {{ line.uom }} — {{ line.discrepancy_reason }}
              </p>

              <div v-if="lineCanReceive(line)" class="grid grid-cols-1 md:grid-cols-2 gap-3">
                <select v-model="receiveForms[line.id].locationId" class="input">
                  <option value="">{{ $t('transferReceiving.drawer.selectStagingLocation') }}</option>
                  <option v-for="loc in destinationLocations" :key="loc.id" :value="loc.id">{{ loc.code }}</option>
                </select>
                <input
                  v-model.number="receiveForms[line.id].quantity"
                  type="number"
                  class="input"
                  :placeholder="$t('transferReceiving.drawer.quantityPlaceholder')"
                />

                <label class="flex items-center gap-2 text-sm text-slate-600 md:col-span-2">
                  <input type="checkbox" v-model="receiveForms[line.id].closeShort" />
                  {{ $t('transferReceiving.drawer.closeShortLabel') }}
                </label>
                <textarea
                  v-if="receiveForms[line.id].closeShort"
                  v-model="receiveForms[line.id].discrepancyReason"
                  rows="2"
                  class="input md:col-span-2"
                  :placeholder="$t('transferReceiving.drawer.discrepancyReasonPlaceholder')"
                ></textarea>

                <button
                  @click="receiveLine(line)"
                  :disabled="submitting"
                  class="md:col-span-2 bg-orange-600 hover:bg-orange-700 text-white rounded-xl font-semibold disabled:opacity-60 py-2"
                >
                  {{ $t('transferReceiving.drawer.receiveButton') }}
                </button>
              </div>
            </div>
          </div>

          <div class="flex flex-col md:flex-row gap-3">
            <button
              v-if="canConfirmAction && selectedReceipt.status === 'DRAFT'"
              @click="confirmReceipt"
              :disabled="submitting || !allLinesSettled"
              class="flex-1 bg-slate-800 hover:bg-slate-900 text-white py-3 rounded-xl font-semibold disabled:opacity-60"
              :title="!allLinesSettled ? $t('transferReceiving.drawer.notReadyToConfirm') : ''"
            >
              {{ $t('transferReceiving.drawer.confirmButton') }}
            </button>

            <button
              v-if="canCancelAction && selectedReceipt.status === 'DRAFT'"
              @click="showCancelConfirm = true"
              :disabled="submitting"
              class="flex-1 bg-white border border-slate-300 hover:bg-slate-100 text-slate-700 py-3 rounded-xl font-semibold disabled:opacity-60"
            >
              {{ $t('transferReceiving.drawer.cancelButton') }}
            </button>
          </div>
        </template>
      </div>
    </div>

    <ConfirmDialog
      :open="showCancelConfirm"
      :title="$t('transferReceiving.drawer.cancelButton')"
      :message="$t('transferReceiving.messages.confirmCancelReceipt')"
      :confirm-label="$t('transferReceiving.drawer.cancelButton')"
      :cancel-label="$t('common.buttons.cancel')"
      :loading-label="$t('common.buttons.submitting')"
      danger
      :loading="submitting"
      @cancel="showCancelConfirm = false"
      @confirm="cancelDrawerReceipt"
    />
  </div>
</template>

<script>
import {
  listTransferReceipts,
  listEligibleTransferShipments,
  getTransferReceipt,
  createTransferReceipt,
  receiveTransferReceiptItem,
  confirmTransferReceipt,
  cancelTransferReceipt,
  newIdempotencyKey,
  getTransferReceiptReconciliation,
  viewTransferNote,
  downloadTransferNote,
} from '../../services/transferReceipts'
import { listWarehouses, listLocations, listZones } from '../../services/warehouses'
import { hasPermission } from '../../services/auth'
import { showToast } from '../../services/toast'
import PageHeader from '../../components/ui/PageHeader.vue'
import FilterBar from '../../components/ui/FilterBar.vue'
import DataTable from '../../components/ui/DataTable.vue'
import StatusBadge from '../../components/ui/StatusBadge.vue'
import ConfirmDialog from '../../components/ui/ConfirmDialog.vue'
import DocumentHistoryPanel from '../../components/documents/DocumentHistoryPanel.vue'
import i18n from '../../i18n'

const { t } = i18n.global
let searchDebounce = null

function statusText(status) {
  switch (status) {
    case 'DRAFT': return t('common.status.draft')
    case 'COMPLETED': return t('common.status.completed')
    case 'CANCELLED': return t('common.status.cancelled')
    default: return status
  }
}

function statusTone(status) {
  if (status === 'DRAFT') return 'gray'
  if (status === 'COMPLETED') return 'green'
  if (status === 'CANCELLED') return 'red'
  return 'gray'
}

function lineStatusText(status) {
  switch (status) {
    case 'PENDING': return t('common.status.pending')
    case 'PARTIALLY_RECEIVED': return t('common.status.inProgress')
    case 'COMPLETED': return t('common.status.completed')
    case 'DISCREPANCY': return t('transferReceiving.lineStatus.discrepancy')
    default: return status
  }
}

function lineStatusTone(status) {
  if (status === 'PENDING') return 'gray'
  if (status === 'PARTIALLY_RECEIVED') return 'orange'
  if (status === 'COMPLETED') return 'green'
  if (status === 'DISCREPANCY') return 'red'
  return 'gray'
}

// Phase 33E.3 — the reconciliation summary's own overall verdict
// (PENDING/RECONCILED/DISCREPANCY, see Service.Reconciliation's own
// doc).
function reconciliationStatusText(status) {
  switch (status) {
    case 'PENDING': return t('transferReceiving.reconciliation.status.pending')
    case 'RECONCILED': return t('transferReceiving.reconciliation.status.reconciled')
    case 'DISCREPANCY': return t('transferReceiving.reconciliation.status.discrepancy')
    default: return status
  }
}

function reconciliationStatusTone(status) {
  if (status === 'PENDING') return 'gray'
  if (status === 'RECONCILED') return 'green'
  if (status === 'DISCREPANCY') return 'red'
  return 'gray'
}

export default {
  name: 'TransferReceivingView',

  components: { PageHeader, FilterBar, DataTable, StatusBadge, ConfirmDialog, DocumentHistoryPanel },

  data() {
    return {
      loading: false,
      loadError: '',
      receiptList: [],
      allReceipts: [],
      warehouses: [],

      selectedReceipt: null,
      isNewReceipt: false,
      drawerError: '',
      eligibleShipments: [],
      destinationLocations: [],
      form: { shipmentId: '', remark: '' },
      receiveForms: {},
      submitting: false,
      showCancelConfirm: false,
      showHistory: false,
      // Phase 33E.3 — the reconciliation summary for the currently open
      // receipt, re-fetched every time the receipt is (re)loaded so it
      // always reflects the latest Receive/Confirm action.
      reconciliation: null,

      filters: {
        keyword: '',
        status: '',
        destinationWarehouseId: '',
        hasDiscrepancy: false
      }
    }
  },

  computed: {
    draftCount() {
      return this.allReceipts.filter((r) => r.status === 'DRAFT').length
    },
    completedCount() {
      return this.allReceipts.filter((r) => r.status === 'COMPLETED').length
    },
    cancelledCount() {
      return this.allReceipts.filter((r) => r.status === 'CANCELLED').length
    },
    canCreate() {
      return hasPermission('transfer_receipt.create')
    },
    canConfirmAction() {
      return hasPermission('transfer_receipt.confirm')
    },
    canCancelAction() {
      return hasPermission('transfer_receipt.approve')
    },
    allLinesSettled() {
      const items = (this.selectedReceipt && this.selectedReceipt.items) || []
      return items.length > 0 && items.every((it) => it.status === 'COMPLETED' || it.status === 'DISCREPANCY')
    },
    // Phase 33E.3 — item_id -> reconciliation line, for the per-line
    // "Fully Reconciled" badge rendered alongside each line's existing
    // status badge.
    reconciliationLineMap() {
      const map = {}
      const lines = (this.reconciliation && this.reconciliation.lines) || []
      for (const line of lines) {
        map[line.item_id] = line
      }
      return map
    }
  },

  watch: {
    'filters.keyword'() {
      clearTimeout(searchDebounce)
      searchDebounce = setTimeout(() => this.fetchReceipts(), 300)
    },
    'filters.status'() {
      this.fetchReceipts()
    },
    'filters.hasDiscrepancy'() {
      this.fetchReceipts()
    },
    'filters.destinationWarehouseId'() {
      this.fetchReceipts()
      this.fetchSummary()
    }
  },

  mounted() {
    this.fetchWarehouses()
  },

  methods: {
    statusText,
    statusTone,
    lineStatusText,
    lineStatusTone,
    reconciliationStatusText,
    reconciliationStatusTone,

    async fetchReceipts() {
      if (!this.filters.destinationWarehouseId) {
        this.receiptList = []
        return
      }
      this.loading = true
      this.loadError = ''
      try {
        const res = await listTransferReceipts({
          search: this.filters.keyword,
          status: this.filters.status,
          destinationWarehouseId: this.filters.destinationWarehouseId,
          hasDiscrepancy: this.filters.hasDiscrepancy,
          limit: 100
        })
        this.receiptList = res.data
      } catch (err) {
        this.loadError = err.message
      } finally {
        this.loading = false
      }
    },

    async fetchSummary() {
      if (!this.filters.destinationWarehouseId) {
        this.allReceipts = []
        return
      }
      try {
        const res = await listTransferReceipts({ destinationWarehouseId: this.filters.destinationWarehouseId, limit: 200 })
        this.allReceipts = res.data
      } catch {
        // Non-critical: the summary cards simply stay at zero.
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
      this.filters.status = ''
      this.filters.destinationWarehouseId = ''
      this.filters.hasDiscrepancy = false
      this.receiptList = []
      this.allReceipts = []
    },

    lineCanReceive(line) {
      return this.selectedReceipt.status === 'DRAFT' && line.status !== 'COMPLETED' && line.status !== 'DISCREPANCY'
    },

    lineRowClass(line) {
      if (line.status === 'COMPLETED') return 'bg-green-50'
      if (line.status === 'DISCREPANCY') return 'bg-red-50'
      return ''
    },

    async openReceipt(item) {
      this.drawerError = ''
      this.submitting = false

      if (!item) {
        this.isNewReceipt = true
        this.selectedReceipt = {}
        this.form = { shipmentId: '', remark: '' }
        try {
          const res = await listEligibleTransferShipments(this.filters.destinationWarehouseId)
          this.eligibleShipments = res.data
        } catch (err) {
          this.drawerError = err.message
        }
        return
      }

      this.isNewReceipt = false
      try {
        const res = await getTransferReceipt(item.id)
        this.selectedReceipt = res.data
        this.receiveForms = {}
        for (const line of res.data.items) {
          this.receiveForms[line.id] = { locationId: '', quantity: line.dispatched_qty - line.received_qty, closeShort: false, discrepancyReason: '' }
        }
        await this.fetchLocationsForWarehouse(res.data.destination_warehouse_id)
        await this.fetchReconciliation(item.id)
      } catch (err) {
        this.drawerError = err.message
        this.selectedReceipt = null
      }
    },

    // Phase 33E.3 — read-only reconciliation summary for the currently
    // open receipt. Non-critical: a lookup failure here must never
    // block viewing/working the receipt itself, so it simply leaves the
    // reconciliation panel hidden.
    async fetchReconciliation(id) {
      this.reconciliation = null
      try {
        const res = await getTransferReceiptReconciliation(id)
        this.reconciliation = res.data
      } catch {
        // Non-critical — see this method's own doc above.
      }
    },

    closeDrawer() {
      this.selectedReceipt = null
      this.isNewReceipt = false
      this.showHistory = false
      this.reconciliation = null
    },

    async fetchLocationsForWarehouse(warehouseId) {
      this.destinationLocations = []
      if (!warehouseId) return
      try {
        const zonesRes = await listZones(warehouseId)
        const receivingZones = zonesRes.data.filter((z) => z.type === 'RECEIVING')
        const allLocations = []
        for (const zone of receivingZones) {
          const locRes = await listLocations(zone.id)
          allLocations.push(...locRes.data)
        }
        this.destinationLocations = allLocations
      } catch {
        // Non-critical: the location <select> simply stays empty.
      }
    },

    async createReceipt() {
      this.drawerError = ''
      if (!this.form.shipmentId) {
        this.drawerError = this.$t('transferReceiving.messages.selectSourceRequired')
        return
      }

      this.submitting = true
      try {
        const res = await createTransferReceipt({
          shipment_id: this.form.shipmentId,
          remark: this.form.remark || null
        })
        showToast(this.$t('transferReceiving.messages.receiptCreated'), 'success')
        await this.openReceipt({ id: res.data.id })
        await this.fetchReceipts()
        await this.fetchSummary()
      } catch (err) {
        this.drawerError = err.message
      } finally {
        this.submitting = false
      }
    },

    async receiveLine(line) {
      const form = this.receiveForms[line.id]
      if (!form.locationId) {
        this.drawerError = this.$t('transferReceiving.messages.locationRequired')
        return
      }
      if (form.closeShort && !form.discrepancyReason) {
        this.drawerError = this.$t('transferReceiving.messages.discrepancyReasonRequired')
        return
      }
      if (!form.closeShort && (!form.quantity || form.quantity <= 0)) {
        this.drawerError = this.$t('transferReceiving.messages.quantityRequired')
        return
      }

      this.submitting = true
      this.drawerError = ''
      try {
        await receiveTransferReceiptItem(this.selectedReceipt.id, line.id, {
          idempotency_key: newIdempotencyKey(),
          location_id: form.locationId,
          quantity: Number(form.quantity) || 0,
          close_short: form.closeShort,
          discrepancy_reason: form.closeShort ? form.discrepancyReason : ''
        })
        showToast(this.$t('transferReceiving.messages.itemReceived'), 'success')
        await this.openReceipt({ id: this.selectedReceipt.id })
        await this.fetchReceipts()
        await this.fetchSummary()
      } catch (err) {
        this.drawerError = err.message
      } finally {
        this.submitting = false
      }
    },

    async confirmReceipt() {
      this.submitting = true
      this.drawerError = ''
      try {
        await confirmTransferReceipt(this.selectedReceipt.id, { idempotency_key: newIdempotencyKey() })
        showToast(this.$t('transferReceiving.messages.receiptConfirmed'), 'success')
        await this.openReceipt({ id: this.selectedReceipt.id })
        await this.fetchReceipts()
        await this.fetchSummary()
      } catch (err) {
        this.drawerError = err.message
      } finally {
        this.submitting = false
      }
    },

    async cancelDrawerReceipt() {
      this.submitting = true
      this.drawerError = ''
      try {
        await cancelTransferReceipt(this.selectedReceipt.id, {})
        this.showCancelConfirm = false
        showToast(this.$t('transferReceiving.messages.receiptCancelled'), 'success')
        this.closeDrawer()
        await this.fetchReceipts()
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
    },

    // Phase 33E.3 — Inter-Warehouse Transfer Note PDF. Mirrors
    // ShippingView.vue's own viewDeliveryNotePdf/downloadDeliveryNotePdf
    // shape exactly.
    async viewTransferNotePdf() {
      try {
        await viewTransferNote(this.selectedReceipt.id)
      } catch (err) {
        showToast(err.message || this.$t('transferReceiving.messages.pdfFailed'), 'error')
      }
    },

    async downloadTransferNotePdf() {
      try {
        await downloadTransferNote(this.selectedReceipt.id)
      } catch (err) {
        showToast(err.message || this.$t('transferReceiving.messages.pdfFailed'), 'error')
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
  font-size: 13px;
  font-weight: 600;
  color: #475569;
  margin-bottom: 6px;
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
