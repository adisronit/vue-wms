<template>
  <div class="space-y-6">
    <div class="flex flex-col xl:flex-row xl:items-center xl:justify-between gap-4">
      <div>
        <h1 class="text-2xl font-bold text-slate-800">Stock Adjustment</h1>
        <p class="text-slate-500">
          Create and review inventory adjustment transactions
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
          @click="openNewAdjustment"
          class="bg-orange-600 hover:bg-orange-700 text-white px-5 py-3 rounded-xl font-semibold"
        >
          + New Adjustment
        </button>
      </div>
    </div>

    <div class="grid grid-cols-1 md:grid-cols-4 gap-5">
      <div class="summary-card">
        <p class="summary-label">Pending Approval</p>
        <h2 class="summary-value">{{ pendingCount }}</h2>
      </div>

      <div class="summary-card">
        <p class="summary-label">Approved Today</p>
        <h2 class="summary-value">{{ approvedTodayCount }}</h2>
      </div>

      <div class="summary-card">
        <p class="summary-label">Rejected</p>
        <h2 class="summary-value text-red-600">{{ rejectedCount }}</h2>
      </div>

      <div class="summary-card">
        <p class="summary-label">Total Variance</p>
        <h2 class="summary-value text-orange-600">{{ totalVariance > 0 ? '+' : '' }}{{ totalVariance }}</h2>
      </div>
    </div>

    <div v-if="loadError" class="bg-red-50 border border-red-200 text-red-600 rounded-xl p-3 text-sm">
      {{ loadError }}
    </div>

    <div class="bg-white rounded-2xl border border-slate-200 p-5">
      <div class="grid grid-cols-1 md:grid-cols-5 gap-4">
        <input
          v-model="filters.keyword"
          type="text"
          placeholder="Search Adj No, SKU, Reason..."
          class="input"
        />

        <select v-model="filters.type" class="input">
          <option value="">All Types</option>
          <option value="INCREASE">Increase</option>
          <option value="DECREASE">Decrease</option>
        </select>

        <select v-model="filters.status" class="input">
          <option value="">All Status</option>
          <option value="DRAFT">Draft</option>
          <option value="PENDING">Pending Approval</option>
          <option value="APPROVED">Approved</option>
          <option value="REJECTED">Rejected</option>
          <option value="OVERDUE">Overdue (Pending)</option>
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

    <div class="bg-white rounded-2xl border border-slate-200 shadow-sm overflow-hidden">
      <div class="w-full overflow-x-auto">
        <table class="min-w-[1200px] w-full text-sm">
          <thead class="bg-slate-50 text-slate-500">
            <tr>
              <th class="table-th">Adjustment No.</th>
              <th class="table-th">Date</th>
              <th class="table-th">SKU</th>
              <th class="table-th">Product</th>
              <th class="table-th">Location</th>
              <th class="table-th">Current Qty</th>
              <th class="table-th">Adjust Qty</th>
              <th class="table-th">New Qty</th>
              <th class="table-th">Reason</th>
              <th class="table-th">Pending Age</th>
              <th class="table-th">Status</th>
              <th class="table-th text-right">Action</th>
            </tr>
          </thead>

          <tbody>
            <tr v-if="loading">
              <td colspan="12" class="text-center py-10 text-slate-400">Loading adjustments…</td>
            </tr>

            <tr v-else-if="adjustmentList.length === 0">
              <td colspan="12" class="text-center py-10 text-slate-400">No stock adjustments found</td>
            </tr>

            <template v-else>
            <tr
              v-for="item in adjustmentList"
              :key="item.id"
              class="border-t hover:bg-orange-50"
            >
              <td class="table-td font-semibold text-slate-800">
                {{ item.adjustmentNo }}
              </td>
              <td class="table-td">{{ item.date }}</td>
              <td class="table-td">{{ item.sku }}</td>
              <td class="table-td">{{ item.productName }}</td>
              <td class="table-td">{{ item.location }}</td>
              <td class="table-td">{{ item.currentQty != null ? item.currentQty : '—' }}</td>

              <td
                class="table-td font-semibold"
                :class="item.adjustQty > 0 ? 'text-green-600' : 'text-red-600'"
              >
                {{ item.adjustQty > 0 ? '+' : '' }}{{ item.adjustQty }}
              </td>

              <td class="table-td font-semibold">{{ item.newQty != null ? item.newQty : '—' }}</td>
              <td class="table-td">{{ item.reason }}</td>

              <td class="table-td">
                <span v-if="item.ageHours != null">
                  {{ formatAge(item.ageHours) }}
                  <span v-if="item.isOverdue" class="badge-red ml-1">Overdue</span>
                </span>
                <span v-else>—</span>
              </td>

              <td class="table-td">
                <span :class="getStatusClass(item.status)">
                  {{ item.statusText }}
                </span>
              </td>

              <td class="table-td text-right">
                <button
                  @click="openDetail(item)"
                  class="text-orange-600 hover:text-orange-700 font-semibold"
                >
                  View Detail
                </button>
              </td>
            </tr>
            </template>
          </tbody>
        </table>
      </div>
    </div>

    <div
      v-if="showDrawer"
      class="fixed inset-0 bg-black/40 flex justify-end z-50"
    >
      <div class="w-full max-w-2xl bg-white h-full p-6 overflow-y-auto">
        <div class="flex items-center justify-between mb-6">
          <div>
            <h2 class="text-xl font-bold text-slate-800">
              {{ isNew ? 'New Stock Adjustment' : 'Adjustment Detail' }}
            </h2>
            <p class="text-slate-500">
              Adjust inventory quantity with approval workflow
            </p>
          </div>

          <button
            @click="closeDrawer"
            class="text-slate-500 hover:text-red-600 text-xl"
          >
            ✕
          </button>
        </div>

        <div v-if="drawerError" class="bg-red-50 border border-red-200 text-red-600 rounded-xl p-3 text-sm mb-4">
          {{ drawerError }}
        </div>

        <div class="space-y-4">
          <template v-if="isNew">
            <div>
              <label class="label">SKU / Barcode</label>
              <input
                v-model="form.sku"
                type="text"
                class="input"
                placeholder="Scan or enter SKU"
                @blur="resolveProductAndQty"
                @keyup.enter="resolveProductAndQty"
              />
            </div>

            <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
              <div>
                <label class="label">Warehouse</label>
                <select v-model="form.warehouseId" class="input" @change="onWarehouseChange">
                  <option value="">Select warehouse</option>
                  <option v-for="w in warehouses" :key="w.id" :value="w.id">{{ w.code }} — {{ w.name }}</option>
                </select>
              </div>

              <div>
                <label class="label">Location</label>
                <select v-model="form.locationId" class="input" :disabled="!form.warehouseId" @change="onLotNoChange">
                  <option value="">Select location</option>
                  <option v-for="l in drawerLocations" :key="l.id" :value="l.id">{{ l.code }}</option>
                </select>
              </div>
            </div>

            <div>
              <label class="label">Lot No. (optional)</label>
              <input
                v-model="form.lotNo"
                type="text"
                class="input"
                placeholder="Leave blank for non-lot-tracked stock"
                @change="onLotNoChange"
              />
            </div>
          </template>

          <template v-else>
            <div>
              <label class="label">SKU</label>
              <input :value="form.sku + ' — ' + form.productName" type="text" class="input bg-slate-50" readonly />
            </div>

            <div>
              <label class="label">Location</label>
              <input :value="form.locationCode" type="text" class="input bg-slate-50" readonly />
            </div>
          </template>

          <!-- Created by/at + pending-age follow-up (Phase 20) -->
          <div v-if="!isNew" class="bg-slate-50 rounded-2xl p-4 grid grid-cols-2 gap-4">
            <div>
              <p class="detail-label">Created By</p>
              <p class="detail-value">{{ form.createdByName || '—' }}</p>
            </div>
            <div>
              <p class="detail-label">Created At</p>
              <p class="detail-value">{{ formatDateTime(form.createdAt) }}</p>
            </div>
          </div>

          <div v-if="form.status === 'PENDING' && form.ageHours != null"
               class="rounded-2xl p-4 border"
               :class="form.isOverdue ? 'bg-red-50 border-red-200' : 'bg-orange-50 border-orange-200'"
          >
            <p class="text-sm font-semibold" :class="form.isOverdue ? 'text-red-600' : 'text-orange-600'">
              Pending for {{ formatAge(form.ageHours) }}
              <span v-if="form.isOverdue">— overdue (threshold: {{ form.pendingWarningHours }}h)</span>
            </p>
          </div>

          <div class="grid grid-cols-1 md:grid-cols-3 gap-4">
            <div>
              <label class="label">Current Qty</label>
              <input
                :value="form.currentQty != null ? form.currentQty : '—'"
                type="text"
                class="input bg-slate-50"
                readonly
              />
            </div>

            <div>
              <label class="label">Adjustment Type</label>
              <select v-model="form.type" class="input" :disabled="!canEditFields">
                <option value="INCREASE">Increase</option>
                <option value="DECREASE">Decrease</option>
              </select>
            </div>

            <div>
              <label class="label">Adjust Qty</label>
              <input
                v-model.number="form.adjustQty"
                type="number"
                class="input"
                placeholder="Qty"
                :readonly="!canEditFields"
              />
            </div>
          </div>

          <div class="bg-orange-50 border border-orange-200 rounded-2xl p-4">
            <p class="text-sm text-slate-500">{{ isTerminal ? 'Resulting Quantity' : 'New Quantity (estimated)' }}</p>
            <h3 class="text-3xl font-bold text-orange-600">
              {{ isTerminal ? (form.afterQty != null ? form.afterQty : '—') : calculatedNewQty }}
            </h3>
          </div>

          <div>
            <label class="label">Reason</label>
            <select v-model="form.reason" class="input" :disabled="!canEditFields">
              <option value="">Select reason</option>
              <option value="Cycle Count Variance">Cycle Count Variance</option>
              <option value="Damaged Goods">Damaged Goods</option>
              <option value="Lost Goods">Lost Goods</option>
              <option value="Found Goods">Found Goods</option>
              <option value="System Correction">System Correction</option>
            </select>
          </div>

          <div>
            <label class="label">Remark</label>
            <textarea
              v-model="form.remark"
              rows="4"
              class="input"
              placeholder="Enter remark"
              :readonly="!canEditFields && !isPendingForApprover"
            ></textarea>
          </div>

          <div v-if="canEditFields" class="grid grid-cols-1 md:grid-cols-2 gap-3 pt-4">
            <button
              @click="submitAdjustment"
              :disabled="submitting"
              class="bg-orange-600 hover:bg-orange-700 text-white py-3 rounded-xl font-semibold disabled:opacity-60"
            >
              Submit for Approval
            </button>

            <button
              @click="saveDraft"
              :disabled="submitting"
              class="bg-slate-100 hover:bg-slate-200 text-slate-700 py-3 rounded-xl font-semibold disabled:opacity-60"
            >
              Save Draft
            </button>
          </div>

          <div v-else-if="isPendingForApprover" class="grid grid-cols-1 md:grid-cols-2 gap-3 pt-4">
            <button
              @click="approveAdjustment"
              :disabled="submitting"
              class="bg-green-600 hover:bg-green-700 text-white py-3 rounded-xl font-semibold disabled:opacity-60"
            >
              Approve
            </button>

            <button
              @click="rejectAdjustment"
              :disabled="submitting"
              class="bg-red-600 hover:bg-red-700 text-white py-3 rounded-xl font-semibold disabled:opacity-60"
            >
              Reject
            </button>
          </div>

          <p v-else-if="!isNew && form.status === 'PENDING'" class="text-sm text-slate-500 pt-2">
            Your role can view this adjustment but not approve or reject it.
          </p>
        </div>
      </div>
    </div>
  </div>
</template>

<script>
import { listStockAdjustments, getStockAdjustment, createStockAdjustment, updateStockAdjustment, confirmStockAdjustment, newIdempotencyKey, exportStockAdjustments } from '../../services/stockAdjustment'
import { listWarehouses, listZones, listLocations } from '../../services/warehouses'
import { listProducts } from '../../services/products'
import { listInventory } from '../../services/inventory'
import { hasPermission } from '../../services/auth'

let searchDebounce = null

function statusText(status) {
  switch (status) {
    case 'DRAFT': return 'Draft'
    case 'PENDING': return 'Pending Approval'
    case 'APPROVED': return 'Approved'
    case 'REJECTED': return 'Rejected'
    default: return status
  }
}

function mapAdjustmentRow(r) {
  return {
    id: r.id,
    adjustmentNo: r.adjustment_no,
    date: (r.date || '').slice(0, 10),
    sku: r.sku,
    productName: r.product_name,
    location: r.location_code,
    currentQty: r.before_qty,
    adjustQty: r.adjust_type === 'DECREASE' ? -r.adjust_qty : r.adjust_qty,
    newQty: r.after_qty,
    reason: r.reason,
    status: r.status,
    statusText: statusText(r.status),
    // Phase 20 — server-computed (never derived from raw timestamps in
    // the browser); null for anything that isn't currently PENDING.
    ageHours: r.age_hours,
    isOverdue: r.is_overdue
  }
}

function emptyForm() {
  return {
    sku: '', productId: null, productName: '',
    warehouseId: '', locationId: '', locationCode: '', lotNo: '',
    currentQty: null, type: 'INCREASE', adjustQty: 0,
    afterQty: null, reason: '', remark: '', status: 'DRAFT',
    createdByName: null, createdAt: null,
    ageHours: null, isOverdue: false, pendingWarningHours: null
  }
}

export default {
  name: 'StockAdjustmentView',

  data() {
    return {
      loading: false,
      loadError: '',
      exporting: false,
      adjustmentList: [],
      allAdjustments: [],
      warehouses: [],
      drawerLocations: [],

      showDrawer: false,
      isNew: false,
      selectedId: null,
      drawerError: '',
      submitting: false,

      filters: { keyword: '', type: '', status: '', date: '' },
      form: emptyForm()
    }
  },

  computed: {
    calculatedNewQty() {
      const qty = Number(this.form.adjustQty || 0)
      const current = Number(this.form.currentQty || 0)
      return this.form.type === 'INCREASE' ? current + qty : current - qty
    },
    isTerminal() {
      return this.form.status === 'APPROVED' || this.form.status === 'REJECTED'
    },
    canCreate() {
      return hasPermission('stock_adjustment.create')
    },
    canEditFields() {
      return (this.isNew || this.form.status === 'DRAFT') && hasPermission(this.isNew ? 'stock_adjustment.create' : 'stock_adjustment.update')
    },
    isPendingForApprover() {
      return !this.isNew && this.form.status === 'PENDING' && hasPermission('stock_adjustment.approve')
    },
    pendingCount() {
      return this.allAdjustments.filter((a) => a.status === 'PENDING').length
    },
    approvedTodayCount() {
      const today = new Date().toISOString().slice(0, 10)
      return this.allAdjustments.filter((a) => a.status === 'APPROVED' && (a.date || '').slice(0, 10) === today).length
    },
    rejectedCount() {
      return this.allAdjustments.filter((a) => a.status === 'REJECTED').length
    },
    totalVariance() {
      return this.allAdjustments
        .filter((a) => a.status === 'APPROVED')
        .reduce((sum, a) => sum + (a.adjust_type === 'DECREASE' ? -a.adjust_qty : a.adjust_qty), 0)
    }
  },

  watch: {
    'filters.keyword'() {
      clearTimeout(searchDebounce)
      searchDebounce = setTimeout(() => this.fetchAdjustments(), 300)
    },
    'filters.type'() { this.fetchAdjustments() },
    'filters.status'() { this.fetchAdjustments() },
    'filters.date'() { this.fetchAdjustments() }
  },

  mounted() {
    this.fetchAdjustments()
    this.fetchSummary()

    // Phase 21 — deep link from a notification (MainLayout's bell
    // dropdown navigates here with ?adjustment_id=<id>). Opens the
    // detail drawer directly rather than requiring the user to find the
    // row in the list themselves.
    const adjustmentId = this.$route.query.adjustment_id
    if (adjustmentId) {
      this.openDetail({ id: adjustmentId })
    }
  },

  methods: {
    async fetchAdjustments() {
      this.loading = true
      this.loadError = ''
      try {
        // "Overdue" isn't a real backend status — it's Phase 20's
        // overdue=true filter, which the API itself scopes to PENDING
        // (see internal/stockadjustment/repository.go's List). Selecting
        // it here sends overdue instead of status=OVERDUE.
        const isOverdueFilter = this.filters.status === 'OVERDUE'
        const res = await listStockAdjustments({
          search: this.filters.keyword, type: this.filters.type,
          status: isOverdueFilter ? '' : this.filters.status,
          overdue: isOverdueFilter,
          date: this.filters.date, limit: 100
        })
        this.adjustmentList = res.data.map(mapAdjustmentRow)
      } catch (err) {
        this.loadError = err.message
      } finally {
        this.loading = false
      }
    },

    async fetchSummary() {
      try {
        const res = await listStockAdjustments({ limit: 200 })
        this.allAdjustments = res.data
      } catch {
        // Non-critical: the table's own error banner already surfaces API failures.
      }
    },

    async fetchWarehouses() {
      if (this.warehouses.length > 0) return
      try {
        const res = await listWarehouses({ limit: 100 })
        this.warehouses = res.data
      } catch (err) {
        this.drawerError = err.message
      }
    },

    async onWarehouseChange() {
      this.form.locationId = ''
      this.drawerLocations = []
      if (!this.form.warehouseId) return
      try {
        const zonesRes = await listZones(this.form.warehouseId)
        const allLocations = []
        for (const zone of zonesRes.data) {
          const locRes = await listLocations(zone.id)
          allLocations.push(...locRes.data)
        }
        this.drawerLocations = allLocations
      } catch (err) {
        this.drawerError = err.message
      }
    },

    clearFilter() {
      this.filters = { keyword: '', type: '', status: '', date: '' }
      this.fetchAdjustments()
    },

    async exportCsv() {
      this.exporting = true
      try {
        const isOverdueFilter = this.filters.status === 'OVERDUE'
        await exportStockAdjustments({
          search: this.filters.keyword, type: this.filters.type,
          status: isOverdueFilter ? '' : this.filters.status, date: this.filters.date,
          overdue: isOverdueFilter
        })
      } catch (err) {
        this.loadError = err.message
      } finally {
        this.exporting = false
      }
    },

    async openNewAdjustment() {
      this.isNew = true
      this.selectedId = null
      this.drawerError = ''
      this.form = emptyForm()
      this.drawerLocations = []
      this.showDrawer = true
      await this.fetchWarehouses()
    },

    async openDetail(item) {
      this.isNew = false
      this.selectedId = item.id
      this.drawerError = ''
      this.showDrawer = true
      try {
        const res = await getStockAdjustment(item.id)
        this.form = {
          sku: res.data.sku, productId: res.data.product_id, productName: res.data.product_name,
          warehouseId: '', locationId: res.data.location_id, locationCode: res.data.location_code,
          lotNo: res.data.lot_no, currentQty: res.data.before_qty, type: res.data.adjust_type,
          adjustQty: res.data.adjust_qty, afterQty: res.data.after_qty, reason: res.data.reason,
          remark: res.data.remark || '', status: res.data.status,
          createdByName: res.data.created_by_name, createdAt: res.data.created_at,
          // Phase 20 — server-computed, null/false for anything not PENDING.
          ageHours: res.data.age_hours, isOverdue: res.data.is_overdue,
          pendingWarningHours: res.data.pending_warning_hours
        }
      } catch (err) {
        this.drawerError = err.message
      }
    },

    closeDrawer() {
      this.showDrawer = false
    },

    async resolveProductAndQty() {
      const trimmed = (this.form.sku || '').trim()
      if (!trimmed) {
        this.drawerError = 'SKU is required'
        return null
      }
      const res = await listProducts({ search: trimmed, limit: 20 })
      const p = res.data.find((x) => x.sku === trimmed || x.barcode === trimmed)
      if (!p) {
        this.drawerError = `No product found for SKU/barcode "${trimmed}"`
        return null
      }
      this.form.productId = p.id
      this.form.productName = p.name

      await this.refreshCurrentQtyPreview()
      return p.id
    },

    // Inventory identity is (product, location, lot) — a preview that
    // ignored lot_no could show a different lot's balance (or none at
    // all) than the one this adjustment will actually touch, so this is
    // re-run whenever product, location, or lot changes.
    async refreshCurrentQtyPreview() {
      if (!this.form.productId || !this.form.locationId) return
      try {
        const invRes = await listInventory({
          productId: this.form.productId, locationId: this.form.locationId,
          lotNo: this.form.lotNo || '', limit: 5
        })
        const match = invRes.data.find((row) => (row.lot_no || '') === (this.form.lotNo || ''))
        this.form.currentQty = match ? match.on_hand_qty : 0
      } catch {
        this.form.currentQty = null
      }
    },

    onLotNoChange() {
      this.refreshCurrentQtyPreview()
    },

    async submitAdjustment() {
      await this.save('PENDING')
    },

    async saveDraft() {
      await this.save('DRAFT')
    },

    async save(status) {
      this.drawerError = ''

      if (this.isNew) {
        const productId = await this.resolveProductAndQty()
        if (!productId) return
        if (!this.form.locationId) {
          this.drawerError = 'Location is required'
          return
        }
      }
      if (!this.form.adjustQty || this.form.adjustQty <= 0) {
        this.drawerError = 'Adjust Qty must be greater than zero'
        return
      }
      if (!this.form.reason) {
        this.drawerError = 'Reason is required'
        return
      }

      this.submitting = true
      try {
        if (this.isNew) {
          await createStockAdjustment({
            product_id: this.form.productId, location_id: this.form.locationId, lot_no: this.form.lotNo || '',
            adjust_type: this.form.type, adjust_qty: this.form.adjustQty, reason: this.form.reason,
            remark: this.form.remark || null, status
          })
          alert(status === 'PENDING' ? 'Stock adjustment submitted for approval' : 'Draft saved successfully')
        } else {
          await updateStockAdjustment(this.selectedId, {
            lot_no: this.form.lotNo || '', adjust_type: this.form.type, adjust_qty: this.form.adjustQty,
            reason: this.form.reason, remark: this.form.remark || null, status
          })
          alert(status === 'PENDING' ? 'Stock adjustment submitted for approval' : 'Draft saved successfully')
        }
        this.closeDrawer()
        await this.fetchAdjustments()
        await this.fetchSummary()
      } catch (err) {
        this.drawerError = err.message
      } finally {
        this.submitting = false
      }
    },

    async approveAdjustment() {
      await this.confirm('APPROVE')
    },

    async rejectAdjustment() {
      if (!this.form.remark) {
        this.drawerError = 'A remark is required to reject an adjustment'
        return
      }
      await this.confirm('REJECT')
    },

    async confirm(action) {
      this.drawerError = ''
      this.submitting = true
      try {
        await confirmStockAdjustment(this.selectedId, {
          idempotency_key: newIdempotencyKey(), action, remark: this.form.remark || ''
        })
        alert(action === 'APPROVE' ? 'Stock adjustment approved' : 'Stock adjustment rejected')
        this.closeDrawer()
        await this.fetchAdjustments()
        await this.fetchSummary()
      } catch (err) {
        this.drawerError = err.message
      } finally {
        this.submitting = false
      }
    },

    getStatusClass(status) {
      if (status === 'DRAFT') return 'badge-gray'
      if (status === 'PENDING') return 'badge-orange'
      if (status === 'APPROVED') return 'badge-green'
      if (status === 'REJECTED') return 'badge-red'
      return 'badge-gray'
    },

    // Phase 20 — display formatting only; the age_hours value itself
    // and the overdue threshold it's compared against both come from
    // the API (internal/stockadjustment's pendingAging), never computed
    // here from raw timestamps.
    formatAge(ageHours) {
      if (ageHours == null) return '—'
      const days = Math.floor(ageHours / 24)
      const hours = ageHours % 24
      if (days > 0) return `${days}d ${hours}h`
      return `${hours}h`
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
