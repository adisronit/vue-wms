<template>
  <div>
    <!-- Page Header -->
    <div class="flex flex-col md:flex-row md:items-center md:justify-between gap-4 mb-6">
      <div>
        <h1 class="text-2xl font-bold text-slate-800">
          Goods Receiving
        </h1>
        <p class="text-slate-500">
          Manage inbound purchase orders and receiving tasks
        </p>
      </div>

      <div class="flex gap-3">
        <button
          @click="exportCsv"
          :disabled="exporting"
          class="bg-slate-100 hover:bg-slate-200 text-slate-700 px-5 py-3 rounded-xl font-semibold disabled:opacity-60"
        >
          {{ exporting ? 'Exporting…' : '⬇ Export CSV' }}
        </button>

        <button
          v-if="canCreate"
          @click="$router.push('/inbound/receiving/new')"
          class="bg-orange-600 hover:bg-orange-700 text-white px-5 py-3 rounded-xl font-semibold"
        >
          + New Receiving
        </button>
      </div>
    </div>

    <!-- Summary Cards -->
    <div class="grid grid-cols-1 md:grid-cols-4 gap-5 mb-6">
      <div class="summary-card">
        <p class="summary-label">Waiting Receive</p>
        <h2 class="summary-value">{{ waitingCount }}</h2>
      </div>

      <!-- The backend consolidates "Receiving" and "Partial Received"
           into one PARTIALLY_RECEIVED status (see PROJECT_STATUS.md),
           so both cards intentionally read the same real count. -->
      <div class="summary-card">
        <p class="summary-label">Receiving</p>
        <h2 class="summary-value">{{ partiallyReceivedCount }}</h2>
      </div>

      <div class="summary-card">
        <p class="summary-label">Partial Received</p>
        <h2 class="summary-value">{{ partiallyReceivedCount }}</h2>
      </div>

      <div class="summary-card">
        <p class="summary-label">Completed</p>
        <h2 class="summary-value">{{ completedCount }}</h2>
      </div>
    </div>

    <div v-if="loadError" class="bg-red-50 border border-red-200 text-red-600 rounded-xl p-3 text-sm mb-6">
      {{ loadError }}
    </div>

    <!-- Filter -->
    <div class="bg-white rounded-2xl border border-slate-200 p-5 mb-6">
      <div class="grid grid-cols-1 md:grid-cols-4 gap-4">
        <input
          v-model="filters.keyword"
          type="text"
          placeholder="Search Document No, Reference No, Vendor..."
          class="input"
        />

        <select v-model="filters.status" class="input">
          <option value="">All Status</option>
          <option value="WAITING">Waiting</option>
          <option value="PARTIALLY_RECEIVED">Receiving</option>
          <option value="PARTIALLY_RECEIVED">Partial Received</option>
          <option value="COMPLETED">Completed</option>
        </select>

        <input
          v-model="filters.date"
          type="date"
          class="input"
        />

        <button
          @click="clearFilter"
          class="bg-slate-100 hover:bg-slate-200 text-slate-700 rounded-xl font-semibold"
        >
          Clear Filter
        </button>
      </div>
    </div>

    <!-- Receiving Table -->
    <div class="bg-white rounded-2xl border border-slate-200 shadow-sm overflow-hidden">
      <div class="w-full overflow-x-auto">
        <table class="min-w-[1100px] w-full text-sm">
          <thead class="bg-slate-50 text-slate-500">
            <tr>
              <th class="table-th">Document No.</th>
              <th class="table-th">ASN No.</th>
              <th class="table-th">Vendor</th>
              <th class="table-th">Warehouse</th>
              <th class="table-th">Expected Date</th>
              <th class="table-th">Items</th>
              <th class="table-th">Progress</th>
              <th class="table-th">Status</th>
              <th class="table-th text-right">Action</th>
            </tr>
          </thead>

          <tbody>
            <tr v-if="loading">
              <td colspan="9" class="text-center py-10 text-slate-400">Loading receiving documents…</td>
            </tr>

            <tr v-else-if="receivingList.length === 0">
              <td colspan="9" class="text-center py-10 text-slate-400">No receiving documents found</td>
            </tr>

            <template v-else>
            <tr
              v-for="item in receivingList"
              :key="item.id"
              class="border-t hover:bg-orange-50"
            >
              <td class="table-td font-semibold text-slate-800">
                {{ item.documentNo }}
              </td>

              <td class="table-td">
                {{ item.asnNo }}
              </td>

              <td class="table-td">
                {{ item.vendor }}
              </td>

              <td class="table-td">
                {{ item.warehouse }}
              </td>

              <td class="table-td">
                {{ item.expectedDate }}
              </td>

              <td class="table-td">
                {{ item.items }}
              </td>

              <td class="table-td">
                <div class="w-32">
                  <div class="h-2 bg-slate-200 rounded-full overflow-hidden">
                    <div
                      class="h-2 bg-orange-600 rounded-full"
                      :style="{ width: item.progress + '%' }"
                    ></div>
                  </div>
                  <p class="text-xs text-slate-500 mt-1">
                    {{ item.progress }}%
                  </p>
                </div>
              </td>

              <td class="table-td">
                <span :class="getStatusClass(item.status)">
                  {{ item.statusText }}
                </span>
              </td>

              <td class="table-td text-right">
                <button
                  @click="openReceiving(item)"
                  class="text-orange-600 hover:text-orange-700 font-semibold"
                >
                  Start Receive
                </button>
              </td>
            </tr>
            </template>
          </tbody>
        </table>
      </div>
    </div>

    <!-- Receiving Detail Panel -->
    <div
      v-if="selectedReceiving"
      class="fixed inset-0 bg-black/40 flex justify-end z-50"
    >
      <div class="w-full max-w-xl bg-white h-full p-6 overflow-y-auto">
        <div class="flex items-center justify-between mb-6">
          <div>
            <h2 class="text-xl font-bold text-slate-800">
              Receiving Detail
            </h2>
            <p class="text-slate-500">
              {{ selectedReceiving.documentNo }}
            </p>
          </div>

          <button
            @click="selectedReceiving = null"
            class="text-slate-500 hover:text-red-600"
          >
            ✕
          </button>
        </div>

        <div class="space-y-4">
          <div class="detail-row">
            <span>Vendor</span>
            <strong>{{ selectedReceiving.vendor }}</strong>
          </div>

          <div class="detail-row">
            <span>Warehouse</span>
            <strong>{{ selectedReceiving.warehouse }}</strong>
          </div>

          <div class="detail-row">
            <span>Expected Date</span>
            <strong>{{ selectedReceiving.expectedDate }}</strong>
          </div>

          <div class="detail-row">
            <span>Status</span>
            <span :class="getStatusClass(selectedReceiving.status)">{{ selectedReceiving.statusText }}</span>
          </div>
        </div>

        <!-- Cancellation details (Phase 18) -->
        <div v-if="selectedReceiving.status === 'CANCELLED'" class="mt-4 bg-red-50 border border-red-200 rounded-xl p-4 space-y-2">
          <div class="detail-row">
            <span>Cancelled At</span>
            <strong>{{ formatDateTime(selectedReceiving.cancelledAt) }}</strong>
          </div>
          <div class="detail-row">
            <span>Cancelled By</span>
            <strong>{{ selectedReceiving.cancelledByName || '—' }}</strong>
          </div>
          <div class="detail-row">
            <span>Cancellation Remark</span>
            <strong>{{ selectedReceiving.cancelRemark || '—' }}</strong>
          </div>
        </div>

        <div v-if="detailError" class="mt-4 bg-red-50 border border-red-200 text-red-600 rounded-xl p-3 text-sm">
          {{ detailError }}
        </div>

        <div class="mt-8">
          <h3 class="font-bold text-slate-800 mb-4">
            Receiving Items
          </h3>

          <p v-if="detailLoading" class="text-slate-400 text-sm">Loading items…</p>
          <p v-else-if="detailItems.length === 0" class="text-slate-400 text-sm">No lines on this receipt</p>

          <div
            v-for="line in detailItems"
            :key="line.itemId"
            class="border border-slate-200 rounded-xl p-4 mb-3"
          >
            <div class="flex justify-between">
              <div>
                <p class="font-semibold text-slate-800">
                  {{ line.sku }}
                </p>
                <p class="text-sm text-slate-500">
                  {{ line.name }}
                </p>
                <p class="text-xs text-slate-400 mt-1">
                  Received {{ line.receivedQty }} / {{ line.expectedQty || '—' }}
                </p>
              </div>

              <div class="text-right">
                <p class="text-sm text-slate-500">Expected Qty</p>
                <p class="font-bold">{{ line.expectedQty }}</p>
              </div>
            </div>

            <div v-if="line.status !== 'COMPLETED'" class="grid grid-cols-2 gap-3 mt-4">
              <input
                v-model.number="line.receiveQty"
                type="number"
                placeholder="Receive Qty"
                class="input"
              />

              <select v-model="line.locationId" class="input">
                <option value="">Select location</option>
                <option v-for="l in detailLocations" :key="l.id" :value="l.id">{{ l.code }}</option>
              </select>
            </div>
            <p v-else class="text-sm text-green-600 mt-3 font-semibold">Fully received</p>
          </div>

          <button
            v-if="canConfirm && detailItems.some((l) => l.status !== 'COMPLETED')"
            @click="confirmDetailReceiving"
            :disabled="confirmSubmitting"
            class="w-full bg-orange-600 hover:bg-orange-700 text-white py-3 rounded-xl font-semibold mt-4 disabled:opacity-60"
          >
            {{ confirmSubmitting ? 'Confirming…' : 'Confirm Receiving' }}
          </button>
          <p v-else-if="!canConfirm" class="text-sm text-slate-500 mt-4">
            Your role can view this receipt but not confirm receiving.
          </p>

          <div v-if="canCancelReceiving && isReceivingCancellable" class="mt-6 pt-6 border-t border-slate-200">
            <input
              v-model="cancelRemark"
              type="text"
              class="input mb-3"
              placeholder="Cancellation remark (optional)"
            />
            <button
              @click="cancelDetailReceiving"
              :disabled="confirmSubmitting"
              class="w-full bg-red-600 hover:bg-red-700 text-white py-3 rounded-xl font-semibold disabled:opacity-60"
            >
              Cancel Receipt
            </button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script>
import { listReceipts, getReceipt, confirmReceipt, cancelReceipt, newIdempotencyKey, exportReceipts } from '../../services/receiving'
import { listZones, listLocations } from '../../services/warehouses'
import { hasPermission } from '../../services/auth'

let searchDebounce = null

function statusText(status) {
  switch (status) {
    case 'DRAFT': return 'Draft'
    case 'WAITING': return 'Waiting'
    case 'PARTIALLY_RECEIVED': return 'Partially Received'
    case 'COMPLETED': return 'Completed'
    case 'CANCELLED': return 'Cancelled'
    default: return status
  }
}

// The real backend's field names (receipt_no, reference_no, ...) are
// mapped onto the mock's existing display field names (documentNo,
// asnNo, ...) so the template needs no further changes.
function mapReceiptListFromApi(r) {
  return {
    id: r.id,
    documentNo: r.receipt_no,
    asnNo: r.reference_no || '—',
    vendor: r.vendor || '—',
    warehouse: r.warehouse_code,
    expectedDate: r.receiving_date ? r.receiving_date.slice(0, 10) : '—',
    items: r.item_count,
    progress: Math.round(r.progress_pct),
    status: r.status,
    statusText: statusText(r.status)
  }
}

function mapDetailItem(item) {
  return {
    itemId: item.id,
    productId: item.product_id,
    sku: item.sku,
    name: item.product_name,
    expectedQty: item.expected_qty,
    receivedQty: item.received_qty,
    uom: item.uom,
    lotNo: item.lot_no,
    expiryDate: item.expiry_date ? item.expiry_date.slice(0, 10) : '',
    locationId: item.location_id,
    status: item.status,
    receiveQty: ''
  }
}

export default {
  name: 'ReceivingPage',

  data() {
    return {
      loading: false,
      loadError: '',
      exporting: false,
      receivingList: [],
      allReceipts: [],

      selectedReceiving: null,
      detailItems: [],
      detailLocations: [],
      detailLoading: false,
      detailError: '',
      confirmSubmitting: false,
      cancelRemark: '',

      filters: {
        keyword: '',
        status: '',
        date: ''
      }
    }
  },

  computed: {
    waitingCount() {
      return this.allReceipts.filter((r) => r.status === 'WAITING').length
    },

    partiallyReceivedCount() {
      return this.allReceipts.filter((r) => r.status === 'PARTIALLY_RECEIVED').length
    },

    completedCount() {
      return this.allReceipts.filter((r) => r.status === 'COMPLETED').length
    },

    canCreate() {
      return hasPermission('receiving.create')
    },

    canConfirm() {
      return hasPermission('receiving.approve')
    },

    // Cancel is gated on receiving.update, not receiving.approve — see
    // the backend's own internal/receiving/handler.go doc: it only ever
    // runs before any stock has been received, the same authority level
    // as editing the receipt.
    canCancelReceiving() {
      return hasPermission('receiving.update')
    },

    isReceivingCancellable() {
      return !!this.selectedReceiving &&
        (this.selectedReceiving.status === 'DRAFT' || this.selectedReceiving.status === 'WAITING')
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
    'filters.date'() {
      this.fetchReceipts()
    }
  },

  mounted() {
    this.fetchReceipts()
    this.fetchSummary()
  },

  methods: {
    async fetchReceipts() {
      this.loading = true
      this.loadError = ''
      try {
        const res = await listReceipts({
          search: this.filters.keyword,
          status: this.filters.status,
          date: this.filters.date,
          limit: 100
        })
        this.receivingList = res.data.map(mapReceiptListFromApi)
      } catch (err) {
        this.loadError = err.message
      } finally {
        this.loading = false
      }
    },

    // A separate, unfiltered fetch backing the summary cards — there is
    // no dedicated stats endpoint this phase, so the cards read from the
    // same list API with a larger page size instead of the current
    // (possibly filtered) table data.
    async fetchSummary() {
      try {
        const res = await listReceipts({ limit: 200 })
        this.allReceipts = res.data
      } catch {
        // Non-critical: the table's own error banner already surfaces API failures.
      }
    },

    clearFilter() {
      this.filters.keyword = ''
      this.filters.status = ''
      this.filters.date = ''
      this.fetchReceipts()
    },

    async exportCsv() {
      this.exporting = true
      try {
        await exportReceipts({ search: this.filters.keyword, status: this.filters.status, date: this.filters.date })
      } catch (err) {
        this.loadError = err.message
      } finally {
        this.exporting = false
      }
    },

    async openReceiving(item) {
      this.selectedReceiving = item
      this.detailItems = []
      this.detailLocations = []
      this.detailError = ''
      this.cancelRemark = ''
      this.detailLoading = true
      try {
        const res = await getReceipt(item.id)
        this.detailItems = res.data.items.map(mapDetailItem)
        await this.fetchDetailLocations(res.data.warehouse_id)
        this.selectedReceiving = {
          ...item,
          cancelledAt: res.data.cancelled_at,
          cancelledByName: res.data.cancelled_by_name,
          cancelRemark: res.data.cancel_remark
        }
      } catch (err) {
        this.detailError = err.message
      } finally {
        this.detailLoading = false
      }
    },

    async fetchDetailLocations(warehouseId) {
      if (!warehouseId) return
      const zonesRes = await listZones(warehouseId)
      const allLocations = []
      for (const zone of zonesRes.data) {
        const locRes = await listLocations(zone.id)
        allLocations.push(...locRes.data)
      }
      this.detailLocations = allLocations
    },

    async confirmDetailReceiving() {
      const linesToConfirm = this.detailItems
        .filter((line) => Number(line.receiveQty || 0) > 0)
        .map((line) => ({
          item_id: line.itemId,
          quantity: Number(line.receiveQty),
          lot_no: line.lotNo || '',
          expiry_date: line.expiryDate ? `${line.expiryDate}T00:00:00Z` : null,
          location_id: line.locationId
        }))

      if (linesToConfirm.length === 0) {
        this.detailError = 'Enter at least one receive quantity to confirm receiving'
        return
      }
      if (linesToConfirm.some((l) => !l.location_id)) {
        this.detailError = 'Every confirmed line must have a location'
        return
      }

      this.confirmSubmitting = true
      this.detailError = ''
      try {
        await confirmReceipt(this.selectedReceiving.id, {
          idempotency_key: newIdempotencyKey(),
          items: linesToConfirm
        })
        alert('Receiving confirmed successfully')
        this.selectedReceiving = null
        await this.fetchReceipts()
        await this.fetchSummary()
      } catch (err) {
        this.detailError = err.message
      } finally {
        this.confirmSubmitting = false
      }
    },

    async cancelDetailReceiving() {
      this.confirmSubmitting = true
      this.detailError = ''
      try {
        await cancelReceipt(this.selectedReceiving.id, {
          remark: this.cancelRemark || undefined
        })
        alert('Receipt cancelled')
        this.selectedReceiving = null
        await this.fetchReceipts()
        await this.fetchSummary()
      } catch (err) {
        this.detailError = err.message
      } finally {
        this.confirmSubmitting = false
      }
    },

    getStatusClass(status) {
      if (status === 'PARTIALLY_RECEIVED') return 'badge-orange'
      if (status === 'COMPLETED') return 'badge-green'
      return 'badge-gray'
    },

    formatDateTime(isoString) {
      if (!isoString) return '—'
      return new Date(isoString).toLocaleString()
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

.badge-blue {
  background: #dbeafe;
  color: #2563eb;
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

.detail-row {
  display: flex;
  justify-content: space-between;
  border-bottom: 1px solid #e2e8f0;
  padding-bottom: 12px;
  color: #475569;
}
</style>