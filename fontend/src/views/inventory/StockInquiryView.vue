<template>
  <div class="space-y-6">
    <!-- Page Header -->
    <div class="flex flex-col xl:flex-row xl:items-center xl:justify-between gap-4">
      <div>
        <h1 class="text-2xl font-bold text-slate-800">
          Stock Inquiry
        </h1>
        <p class="text-slate-500">
          Search and monitor warehouse inventory by SKU, lot, zone and location
        </p>
      </div>

      <div class="flex flex-col items-end gap-2">
        <div class="flex gap-3">
          <button
            @click="exportCsv"
            :disabled="exporting"
            class="bg-orange-600 hover:bg-orange-700 text-white px-5 py-3 rounded-xl font-semibold disabled:opacity-60"
          >
            {{ exporting ? 'Exporting…' : 'Export Stock' }}
          </button>

          <button
            v-if="canExportTransactions"
            @click="exportTransactionsAsync"
            :disabled="transactionJobStatus === 'PENDING' || transactionJobStatus === 'PROCESSING'"
            class="bg-slate-100 hover:bg-slate-200 text-slate-700 px-5 py-3 rounded-xl font-semibold disabled:opacity-60"
          >
            ⬇ Export Transaction History
          </button>
        </div>

        <!-- Phase 22 — the one async report; polls GET /reports/export-jobs/{id}
             until COMPLETED/FAILED rather than blocking the request. -->
        <p v-if="transactionJobStatus" class="text-sm" :class="transactionJobStatus === 'FAILED' ? 'text-red-600' : 'text-slate-500'">
          <span v-if="transactionJobStatus === 'PENDING' || transactionJobStatus === 'PROCESSING'">
            Generating transaction export…
          </span>
          <span v-else-if="transactionJobStatus === 'COMPLETED'">
            Transaction export ready — downloading…
          </span>
          <span v-else-if="transactionJobStatus === 'FAILED'">
            Transaction export failed: {{ transactionJobError }}
          </span>
        </p>
      </div>
    </div>

    <!-- Summary Cards -->
    <div class="grid grid-cols-1 md:grid-cols-4 gap-5">
      <div class="summary-card">
        <p class="summary-label">Total SKUs</p>
        <h2 class="summary-value">{{ summary.total_skus ?? 0 }}</h2>
      </div>

      <div class="summary-card">
        <p class="summary-label">Available Qty</p>
        <h2 class="summary-value">{{ summary.available_qty ?? 0 }}</h2>
      </div>

      <div class="summary-card">
        <p class="summary-label">Reserved Qty</p>
        <h2 class="summary-value">{{ summary.reserved_qty ?? 0 }}</h2>
      </div>

      <div class="summary-card">
        <p class="summary-label">Low Stock</p>
        <h2 class="summary-value text-red-600">{{ summary.low_stock_count ?? 0 }}</h2>
      </div>
    </div>

    <div v-if="listError" class="bg-red-50 border border-red-200 text-red-600 rounded-xl p-3 text-sm">
      {{ listError }}
    </div>

    <!-- Filter -->
    <div class="bg-white rounded-2xl border border-slate-200 p-5">
      <div class="grid grid-cols-1 md:grid-cols-5 gap-4">
        <input
          v-model="filters.keyword"
          type="text"
          placeholder="Search SKU, Product, Barcode..."
          class="input"
        />

        <select v-model="filters.warehouseId" class="input">
          <option value="">All Warehouses</option>
          <option v-for="w in warehouses" :key="w.id" :value="w.id">{{ w.code }} — {{ w.name }}</option>
        </select>

        <select v-model="filters.zoneId" class="input" :disabled="!filters.warehouseId">
          <option value="">All Zones</option>
          <option v-for="z in zones" :key="z.id" :value="z.id">Zone {{ z.code }}</option>
        </select>

        <select v-model="filters.stockStatus" class="input">
          <option value="">All Stock Status</option>
          <option value="AVAILABLE">Available</option>
          <option value="RESERVED">Reserved</option>
          <option value="HOLD">Hold</option>
          <option value="LOW_STOCK">Low Stock</option>
        </select>

        <button
          @click="clearFilter"
          class="bg-slate-100 hover:bg-slate-200 text-slate-700 rounded-xl font-semibold"
        >
          Clear Filter
        </button>
      </div>
    </div>

    <!-- Stock Table -->
    <div class="bg-white rounded-2xl border border-slate-200 shadow-sm overflow-hidden">
      <div class="w-full overflow-x-auto">
        <table class="min-w-[1250px] w-full text-sm">
          <thead class="bg-slate-50 text-slate-500">
            <tr>
              <th class="table-th">SKU</th>
              <th class="table-th">Product Name</th>
              <th class="table-th">Barcode</th>
              <th class="table-th">Warehouse</th>
              <th class="table-th">Location</th>
              <th class="table-th">Lot No.</th>
              <th class="table-th">Available</th>
              <th class="table-th">Reserved</th>
              <th class="table-th">On Hand</th>
              <th class="table-th">UOM</th>
              <th class="table-th">Status</th>
              <th class="table-th text-right">Action</th>
            </tr>
          </thead>

          <tbody>
            <tr v-if="loading">
              <td class="table-td" colspan="12">Loading inventory...</td>
            </tr>
            <tr v-else-if="stockList.length === 0">
              <td class="table-td" colspan="12">No inventory found.</td>
            </tr>
            <tr
              v-for="item in stockList"
              :key="item.id"
              class="border-t hover:bg-orange-50"
            >
              <td class="table-td font-semibold text-slate-800">
                {{ item.sku }}
              </td>

              <td class="table-td">
                {{ item.productName }}
              </td>

              <td class="table-td">
                {{ item.barcode }}
              </td>

              <td class="table-td">
                {{ item.warehouse }}
              </td>

              <td class="table-td">
                {{ item.location }}
              </td>

              <td class="table-td">
                {{ item.lotNo }}
              </td>

              <td class="table-td font-semibold text-green-600">
                {{ item.availableQty }}
              </td>

              <td class="table-td text-blue-600">
                {{ item.reservedQty }}
              </td>

              <td class="table-td font-semibold">
                {{ item.onHandQty }}
              </td>

              <td class="table-td">
                {{ item.uom }}
              </td>

              <td class="table-td">
                <span :class="getStatusClass(item.stockStatus)">
                  {{ item.stockStatusText }}
                </span>
              </td>

              <td class="table-td text-right">
                <button
                  @click="openStockDetail(item)"
                  class="text-orange-600 hover:text-orange-700 font-semibold"
                >
                  View Detail
                </button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- Stock Detail Drawer -->
    <div
      v-if="selectedStock"
      class="fixed inset-0 bg-black/40 flex justify-end z-50"
    >
      <div class="w-full max-w-2xl bg-white h-full p-6 overflow-y-auto">
        <div class="flex items-center justify-between mb-6">
          <div>
            <h2 class="text-xl font-bold text-slate-800">
              Stock Detail
            </h2>
            <p class="text-slate-500">
              {{ selectedStock.sku }} / {{ selectedStock.location }}
            </p>
          </div>

          <button
            @click="selectedStock = null"
            class="text-slate-500 hover:text-red-600 text-xl"
          >
            ✕
          </button>
        </div>

        <!-- Product Info -->
        <div class="bg-slate-50 rounded-2xl p-5 mb-6">
          <div class="grid grid-cols-2 gap-4">
            <div>
              <p class="detail-label">Product Name</p>
              <p class="detail-value">{{ selectedStock.productName }}</p>
            </div>

            <div>
              <p class="detail-label">Barcode</p>
              <p class="detail-value">{{ selectedStock.barcode }}</p>
            </div>

            <div>
              <p class="detail-label">Warehouse</p>
              <p class="detail-value">{{ selectedStock.warehouse }}</p>
            </div>

            <div>
              <p class="detail-label">Location</p>
              <p class="detail-value text-orange-600">
                {{ selectedStock.location }}
              </p>
            </div>

            <div>
              <p class="detail-label">Lot No.</p>
              <p class="detail-value">{{ selectedStock.lotNo }}</p>
            </div>

            <div>
              <p class="detail-label">Expiry Date</p>
              <p class="detail-value">{{ selectedStock.expiryDate }}</p>
            </div>
          </div>
        </div>

        <!-- Quantity Summary -->
        <div class="grid grid-cols-3 gap-4 mb-6">
          <div class="qty-card">
            <p class="summary-label">Available</p>
            <h3 class="qty-value text-green-600">
              {{ selectedStock.availableQty }}
            </h3>
          </div>

          <div class="qty-card">
            <p class="summary-label">Reserved</p>
            <h3 class="qty-value text-blue-600">
              {{ selectedStock.reservedQty }}
            </h3>
          </div>

          <div class="qty-card">
            <p class="summary-label">On Hand</p>
            <h3 class="qty-value">
              {{ selectedStock.onHandQty }}
            </h3>
          </div>
        </div>

        <!-- Recent Movement -->
        <div>
          <h3 class="font-bold text-slate-800 mb-4">
            Recent Movement
          </h3>

          <div v-if="movementHistory.length === 0" class="text-slate-400 text-sm">
            No movement recorded yet.
          </div>

          <div v-else class="space-y-3">
            <div
              v-for="movement in movementHistory"
              :key="movement.id"
              class="border border-slate-200 rounded-xl p-4"
            >
              <div class="flex justify-between">
                <div>
                  <p class="font-semibold text-slate-800">
                    {{ movement.transaction_type }}
                  </p>
                  <p class="text-sm text-slate-500">
                    {{ movement.reference_id || '—' }}
                  </p>
                </div>

                <div class="text-right">
                  <p
                    class="font-bold"
                    :class="movement.quantity > 0 ? 'text-green-600' : 'text-red-600'"
                  >
                    {{ movement.quantity > 0 ? '+' : '' }}{{ movement.quantity }}
                  </p>
                  <p class="text-xs text-slate-500">
                    {{ formatDate(movement.created_at) }}
                  </p>
                </div>
              </div>
            </div>
          </div>
        </div>

        <button
          class="w-full bg-orange-600 hover:bg-orange-700 text-white py-3 rounded-xl font-semibold mt-6"
        >
          Create Adjustment
        </button>
      </div>
    </div>
  </div>
</template>

<script>
import {
  listInventory, getInventorySummary, getInventoryTransactions,
  exportInventory, createInventoryTransactionsExport, getExportJob, downloadExportJob
} from '../../services/inventory'
import { listWarehouses, listZones } from '../../services/warehouses'
import { hasPermission } from '../../services/auth'

let searchDebounce = null

// Maps the backend's snake_case joined/computed response to the shape
// this component (and its template) already used, the same pattern as
// Phase 4's ProductMasterView field mapping.
function mapItemFromApi(item) {
  return {
    id: item.id,
    sku: item.sku,
    productName: item.product_name,
    barcode: item.barcode || '',
    warehouseId: item.warehouse_id,
    warehouse: item.warehouse_code,
    zoneId: item.zone_id,
    zone: item.zone_code,
    location: item.location_code,
    lotNo: item.lot_no,
    expiryDate: item.expiry_date ? item.expiry_date.slice(0, 10) : '-',
    availableQty: item.available_qty,
    reservedQty: item.reserved_qty,
    onHandQty: item.on_hand_qty,
    uom: item.uom,
    stockStatus: item.status,
    stockStatusText: statusLabel(item.status)
  }
}

function statusLabel(status) {
  if (status === 'AVAILABLE') return 'Available'
  if (status === 'RESERVED') return 'Reserved'
  if (status === 'HOLD') return 'Hold'
  if (status === 'LOW_STOCK') return 'Low Stock'
  return status
}

export default {
  name: 'StockInquiryView',

  data() {
    return {
      selectedStock: null,
      loading: false,
      listError: '',

      filters: {
        keyword: '',
        warehouseId: '',
        zoneId: '',
        stockStatus: ''
      },

      summary: {},
      warehouses: [],
      zones: [],
      stockList: [],
      movementHistory: [],

      exporting: false,
      transactionJobStatus: '',
      transactionJobError: '',
      transactionJobPoll: null
    }
  },

  computed: {
    canExportTransactions() {
      return hasPermission('stock_inquiry.export')
    }
  },

  watch: {
    'filters.warehouseId'() {
      this.filters.zoneId = ''
      this.fetchZones()
      this.fetchStock()
    },
    'filters.zoneId'() {
      this.fetchStock()
    },
    'filters.stockStatus'() {
      this.fetchStock()
    },
    'filters.keyword'() {
      clearTimeout(searchDebounce)
      searchDebounce = setTimeout(() => this.fetchStock(), 300)
    }
  },

  async mounted() {
    await this.fetchWarehouses()
    await Promise.all([this.fetchStock(), this.fetchSummary()])
  },

  beforeUnmount() {
    if (this.transactionJobPoll) clearInterval(this.transactionJobPoll)
  },

  methods: {
    async exportCsv() {
      this.exporting = true
      try {
        await exportInventory({
          search: this.filters.keyword, warehouseId: this.filters.warehouseId,
          zoneId: this.filters.zoneId, status: this.filters.stockStatus
        })
      } catch (err) {
        this.listError = err.message
      } finally {
        this.exporting = false
      }
    },

    // Phase 22's one async report: create the job, then poll its status
    // every 2s (rather than blocking the request) until it reaches a
    // terminal state, auto-downloading once COMPLETED.
    async exportTransactionsAsync() {
      this.transactionJobError = ''
      try {
        const res = await createInventoryTransactionsExport({
          warehouseId: this.filters.warehouseId
        })
        const jobId = res.data.job_id
        this.transactionJobStatus = 'PENDING'
        this.pollTransactionJob(jobId)
      } catch (err) {
        this.transactionJobStatus = 'FAILED'
        this.transactionJobError = err.message
      }
    },

    pollTransactionJob(jobId) {
      if (this.transactionJobPoll) clearInterval(this.transactionJobPoll)
      this.transactionJobPoll = setInterval(async () => {
        try {
          const res = await getExportJob(jobId)
          this.transactionJobStatus = res.data.status
          if (res.data.status === 'COMPLETED') {
            clearInterval(this.transactionJobPoll)
            await downloadExportJob(jobId)
            setTimeout(() => { this.transactionJobStatus = '' }, 4000)
          } else if (res.data.status === 'FAILED') {
            clearInterval(this.transactionJobPoll)
            this.transactionJobError = res.data.error_message || 'unknown error'
          }
        } catch (err) {
          clearInterval(this.transactionJobPoll)
          this.transactionJobStatus = 'FAILED'
          this.transactionJobError = err.message
        }
      }, 2000)
    },
    async fetchWarehouses() {
      try {
        const res = await listWarehouses({ limit: 100 })
        this.warehouses = res.data
      } catch (err) {
        this.listError = err.message || 'Failed to load warehouses'
      }
    },

    async fetchZones() {
      this.zones = []
      if (!this.filters.warehouseId) return
      try {
        const res = await listZones(this.filters.warehouseId)
        this.zones = res.data
      } catch (err) {
        this.listError = err.message || 'Failed to load zones'
      }
    },

    async fetchSummary() {
      try {
        const res = await getInventorySummary()
        this.summary = res.data
      } catch (err) {
        this.listError = err.message || 'Failed to load inventory summary'
      }
    },

    async fetchStock() {
      this.loading = true
      this.listError = ''
      try {
        const res = await listInventory({
          search: this.filters.keyword,
          warehouseId: this.filters.warehouseId,
          zoneId: this.filters.zoneId,
          status: this.filters.stockStatus,
          limit: 100
        })
        this.stockList = res.data.map(mapItemFromApi)
      } catch (err) {
        this.listError = err.message || 'Failed to load inventory'
      } finally {
        this.loading = false
      }
    },

    async openStockDetail(item) {
      this.selectedStock = item
      this.movementHistory = []
      try {
        const res = await getInventoryTransactions(item.id)
        this.movementHistory = res.data
      } catch (err) {
        this.listError = err.message || 'Failed to load movement history'
      }
    },

    formatDate(isoString) {
      if (!isoString) return '-'
      return isoString.slice(0, 10)
    },

    clearFilter() {
      this.filters.keyword = ''
      this.filters.warehouseId = ''
      this.filters.zoneId = ''
      this.filters.stockStatus = ''
    },

    getStatusClass(status) {
      if (status === 'AVAILABLE') return 'badge-green'
      if (status === 'RESERVED') return 'badge-blue'
      if (status === 'HOLD') return 'badge-orange'
      if (status === 'LOW_STOCK') return 'badge-red'
      return 'badge-gray'
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

.summary-card,
.qty-card {
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

.qty-value {
  font-size: 24px;
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

.badge-red {
  background: #fee2e2;
  color: #dc2626;
  padding: 5px 10px;
  border-radius: 999px;
  font-weight: 600;
  font-size: 12px;
}
</style>