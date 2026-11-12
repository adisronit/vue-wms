<template>
  <div class="space-y-6">
    <div class="flex flex-col xl:flex-row xl:items-center xl:justify-between gap-4">
      <div>
        <h1 class="text-2xl font-bold text-slate-800">Shipping</h1>
        <p class="text-slate-500">
          Manage packed orders, shipment confirmation and delivery dispatch
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
          @click="openShipping(null)"
          class="bg-orange-600 hover:bg-orange-700 text-white px-5 py-3 rounded-xl font-semibold"
        >
          + New Shipment
        </button>
      </div>
    </div>

    <div class="grid grid-cols-1 md:grid-cols-4 gap-5">
      <div class="summary-card">
        <p class="summary-label">Ready to Ship</p>
        <h2 class="summary-value">{{ readyCount }}</h2>
      </div>

      <div class="summary-card">
        <p class="summary-label">In Transit</p>
        <h2 class="summary-value">{{ inTransitCount }}</h2>
      </div>

      <div class="summary-card">
        <p class="summary-label">Delivered</p>
        <h2 class="summary-value">{{ deliveredCount }}</h2>
      </div>

      <div class="summary-card">
        <p class="summary-label">Delivery Issues</p>
        <h2 class="summary-value text-red-600">{{ issueCount }}</h2>
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
          placeholder="Search Shipment No, Order No, Customer..."
          class="input"
        />

        <select v-model="filters.status" class="input">
          <option value="">All Status</option>
          <option value="READY">Ready to Ship</option>
          <option value="DISPATCHED">Dispatched</option>
          <option value="IN_TRANSIT">In Transit</option>
          <option value="DELIVERED">Delivered</option>
          <option value="ISSUE">Issue</option>
          <option value="CANCELLED">Cancelled</option>
        </select>

        <select v-model="filters.carrier" class="input">
          <option value="">All Carriers</option>
          <option value="DHL">DHL</option>
          <option value="KERRY">Kerry Express</option>
          <option value="FLASH">Flash Express</option>
          <option value="INTERNAL">Internal Truck</option>
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
        <table class="min-w-[1250px] w-full text-sm">
          <thead class="bg-slate-50 text-slate-500">
            <tr>
              <th class="table-th">Shipment No.</th>
              <th class="table-th">Order No.</th>
              <th class="table-th">Customer</th>
              <th class="table-th">Carrier</th>
              <th class="table-th">Tracking No.</th>
              <th class="table-th">Packages</th>
              <th class="table-th">Weight</th>
              <th class="table-th">Ship Date</th>
              <th class="table-th">Status</th>
              <th class="table-th text-right">Action</th>
            </tr>
          </thead>

          <tbody>
            <tr v-if="loading">
              <td colspan="10" class="text-center py-10 text-slate-400">Loading shipments…</td>
            </tr>

            <tr v-else-if="shippingList.length === 0">
              <td colspan="10" class="text-center py-10 text-slate-400">No shipments found</td>
            </tr>

            <template v-else>
            <tr
              v-for="item in shippingList"
              :key="item.id"
              class="border-t hover:bg-orange-50"
            >
              <td class="table-td font-semibold text-slate-800">
                {{ item.shipmentNo }}
              </td>

              <td class="table-td">{{ item.orderNo }}</td>
              <td class="table-td">{{ item.customer }}</td>
              <td class="table-td">{{ item.carrier || '-' }}</td>
              <td class="table-td">{{ item.trackingNo || '-' }}</td>
              <td class="table-td">{{ item.packages }}</td>
              <td class="table-td">{{ item.weight != null ? item.weight + ' kg' : '-' }}</td>
              <td class="table-td">{{ item.shipDate || '-' }}</td>

              <td class="table-td">
                <span :class="getStatusClass(item.status)">
                  {{ item.statusText }}
                </span>
              </td>

              <td class="table-td text-right">
                <button
                  @click="openShipping(item)"
                  class="text-orange-600 hover:text-orange-700 font-semibold"
                >
                  Manage
                </button>
              </td>
            </tr>
            </template>
          </tbody>
        </table>
      </div>
    </div>

    <div
      v-if="selectedShipping"
      class="fixed inset-0 bg-black/40 flex justify-end z-50"
    >
      <div class="w-full max-w-3xl bg-white h-full p-6 overflow-y-auto">
        <div class="flex items-center justify-between mb-6">
          <div>
            <h2 class="text-xl font-bold text-slate-800">
              {{ isNewShipment ? 'New Shipment' : 'Shipping Detail' }}
            </h2>
            <p v-if="!isNewShipment" class="text-slate-500">
              {{ selectedShipping.shipmentNo }} / {{ selectedShipping.orderNo }}
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

        <!-- New shipment: source picking task selector -->
        <div v-if="isNewShipment" class="bg-slate-50 rounded-2xl p-5 mb-6">
          <label class="label">Source Picking Task</label>
          <select v-model="form.pickingTaskId" class="input">
            <option value="">Select a completed picking task</option>
            <option v-for="src in sources" :key="src.picking_task_id" :value="src.picking_task_id">
              {{ src.pick_no }} / {{ src.order_no }} ({{ src.item_count }} item{{ src.item_count === 1 ? '' : 's' }})
            </option>
          </select>
          <p v-if="sources.length === 0" class="text-xs text-slate-400 mt-2">
            No completed picking tasks are awaiting shipment.
          </p>
        </div>

        <!-- Existing shipment: read-only summary -->
        <div v-else class="bg-slate-50 rounded-2xl p-5 mb-6">
          <div class="grid grid-cols-2 md:grid-cols-4 gap-4">
            <div>
              <p class="detail-label">Customer</p>
              <p class="detail-value">{{ selectedShipping.customer }}</p>
            </div>

            <div>
              <p class="detail-label">Carrier</p>
              <p class="detail-value text-orange-600">{{ selectedShipping.carrier || '—' }}</p>
            </div>

            <div>
              <p class="detail-label">Packages</p>
              <p class="detail-value">{{ selectedShipping.packages }}</p>
            </div>

            <div>
              <p class="detail-label">Total Weight</p>
              <p class="detail-value">{{ selectedShipping.weight != null ? selectedShipping.weight + ' kg' : '—' }}</p>
            </div>
          </div>
        </div>

        <!-- Cancellation details (Phase 18) -->
        <div v-if="!isNewShipment && selectedShipping.status === 'CANCELLED'" class="bg-red-50 border border-red-200 rounded-2xl p-5 mb-6">
          <div class="grid grid-cols-2 md:grid-cols-3 gap-4">
            <div>
              <p class="detail-label">Cancelled At</p>
              <p class="detail-value">{{ formatDateTime(selectedShipping.cancelled_at) }}</p>
            </div>
            <div>
              <p class="detail-label">Cancelled By</p>
              <p class="detail-value">{{ selectedShipping.cancelled_by_name || '—' }}</p>
            </div>
            <div>
              <p class="detail-label">Cancellation Remark</p>
              <p class="detail-value">{{ selectedShipping.cancel_remark || '—' }}</p>
            </div>
          </div>
        </div>

        <div v-if="isNewShipment || canEditHeader && selectedShipping.status === 'READY'" class="space-y-4">
          <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div>
              <label class="label">Carrier</label>
              <select v-model="form.carrier" class="input">
                <option value="">Select carrier</option>
                <option value="DHL">DHL</option>
                <option value="KERRY">Kerry Express</option>
                <option value="FLASH">Flash Express</option>
                <option value="INTERNAL">Internal Truck</option>
              </select>
            </div>

            <div>
              <label class="label">Tracking No.</label>
              <input
                v-model="form.trackingNo"
                type="text"
                class="input"
                placeholder="Enter tracking number"
              />
            </div>
          </div>

          <div class="grid grid-cols-1 md:grid-cols-3 gap-4">
            <div>
              <label class="label">Package Count</label>
              <input
                v-model.number="form.packages"
                type="number"
                class="input"
                placeholder="Packages"
              />
            </div>

            <div>
              <label class="label">Total Weight</label>
              <input
                v-model.number="form.weight"
                type="number"
                class="input"
                placeholder="Weight"
              />
            </div>

            <div>
              <label class="label">Ship Date</label>
              <input
                v-model="form.shipDate"
                type="date"
                class="input"
              />
            </div>
          </div>

          <div>
            <label class="label">Delivery Address</label>
            <textarea
              v-model="form.address"
              rows="4"
              class="input"
              placeholder="Enter delivery address"
            ></textarea>
          </div>

          <div>
            <label class="label">Remark</label>
            <textarea
              v-model="form.remark"
              rows="3"
              class="input"
              placeholder="Enter remark"
            ></textarea>
          </div>

          <button
            v-if="isNewShipment"
            @click="saveNewShipment"
            :disabled="submitting"
            class="w-full bg-orange-600 hover:bg-orange-700 text-white py-3 rounded-xl font-semibold disabled:opacity-60"
          >
            {{ submitting ? 'Saving…' : 'Save Shipment' }}
          </button>
          <button
            v-else
            @click="saveHeaderChanges"
            :disabled="submitting"
            class="w-full bg-slate-800 hover:bg-slate-900 text-white py-3 rounded-xl font-semibold disabled:opacity-60"
          >
            {{ submitting ? 'Saving…' : 'Save Changes' }}
          </button>
        </div>

        <div v-if="!isNewShipment" class="mt-6">
          <h3 class="font-bold text-slate-800 mb-4">
            Package Items
          </h3>

          <p v-if="(selectedShipping.items || []).length === 0" class="text-sm text-slate-400">
            No items on this shipment.
          </p>

          <div
            v-for="product in selectedShipping.items"
            :key="product.id"
            class="border border-slate-200 rounded-2xl p-4 mb-3"
          >
            <div class="flex justify-between gap-4">
              <div>
                <p class="font-bold text-slate-800">{{ product.sku }}</p>
                <p class="text-sm text-slate-500">{{ product.product_name }}</p>
              </div>

              <div class="text-right">
                <p class="detail-label">Qty</p>
                <p class="detail-value">{{ product.qty }}</p>
              </div>
            </div>
          </div>
        </div>

        <div v-if="!isNewShipment && canConfirm" class="grid grid-cols-1 md:grid-cols-3 gap-3 mt-6">
          <button
            v-if="selectedShipping.status === 'READY'"
            @click="confirmDispatch"
            :disabled="submitting"
            class="bg-orange-600 hover:bg-orange-700 text-white py-3 rounded-xl font-semibold disabled:opacity-60"
          >
            Confirm Dispatch
          </button>

          <button
            v-if="selectedShipping.status === 'DISPATCHED'"
            @click="markDelivered"
            :disabled="submitting"
            class="bg-green-600 hover:bg-green-700 text-white py-3 rounded-xl font-semibold disabled:opacity-60"
          >
            Mark Delivered
          </button>

          <button
            v-if="selectedShipping.status === 'READY' || selectedShipping.status === 'DISPATCHED'"
            @click="reportIssue"
            :disabled="submitting"
            class="bg-red-600 hover:bg-red-700 text-white py-3 rounded-xl font-semibold disabled:opacity-60"
          >
            Report Issue
          </button>
        </div>
        <p v-else-if="!isNewShipment" class="text-sm text-slate-500 mt-6">
          Your role can view this shipment but not confirm dispatch, delivery, or issues.
        </p>

        <div v-if="!isNewShipment && canEditHeader && selectedShipping.status === 'READY'" class="mt-6 pt-6 border-t border-slate-200">
          <input
            v-model="cancelRemark"
            type="text"
            class="input mb-3"
            placeholder="Cancellation remark (optional)"
          />
          <button
            @click="cancelSelectedShipment"
            :disabled="submitting"
            class="w-full bg-slate-800 hover:bg-slate-900 text-white py-3 rounded-xl font-semibold disabled:opacity-60"
          >
            Cancel Shipment
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script>
import { listShipments, getShipment, listShippableSources, createShipment, updateShipment, confirmShipment, cancelShipment, newIdempotencyKey, exportShipments } from '../../services/shipping'
import { hasPermission } from '../../services/auth'

let searchDebounce = null

function toApiDate(dateStr) {
  return dateStr ? `${dateStr}T00:00:00Z` : null
}

// DISPATCHED is the only real state ShippingView.vue's actions ever
// produce, but the mock's own summary card is labeled "In Transit" (not
// "Dispatched") — mapped onto DISPATCHED for display, the same
// consolidation precedent Putaway's own "In Progress"/PARTIALLY_COMPLETED
// mapping already established. IN_TRANSIT itself is modeled in the
// schema but unreachable this phase (see internal/shipping's doc).
function statusText(status) {
  switch (status) {
    case 'READY': return 'Ready to Ship'
    case 'DISPATCHED': return 'In Transit'
    case 'IN_TRANSIT': return 'In Transit'
    case 'DELIVERED': return 'Delivered'
    case 'ISSUE': return 'Issue'
    case 'CANCELLED': return 'Cancelled'
    default: return status
  }
}

function mapShipmentRow(r) {
  return {
    id: r.id,
    shipmentNo: r.shipment_no,
    orderNo: r.order_no,
    customer: r.customer || '—',
    carrier: r.carrier,
    trackingNo: r.tracking_no,
    packages: r.packages,
    weight: r.weight,
    shipDate: r.ship_date ? r.ship_date.slice(0, 10) : '',
    status: r.status,
    statusText: statusText(r.status)
  }
}

function emptyForm() {
  return {
    pickingTaskId: '',
    carrier: '',
    trackingNo: '',
    packages: 1,
    weight: null,
    shipDate: '',
    address: '',
    remark: ''
  }
}

export default {
  name: 'ShippingView',

  data() {
    return {
      loading: false,
      loadError: '',
      exporting: false,
      shippingList: [],
      allShipments: [],
      sources: [],

      selectedShipping: null,
      isNewShipment: false,
      drawerError: '',
      submitting: false,
      cancelRemark: '',

      form: emptyForm(),

      filters: {
        keyword: '',
        status: '',
        carrier: '',
        date: ''
      }
    }
  },

  computed: {
    readyCount() {
      return this.allShipments.filter((s) => s.status === 'READY').length
    },
    inTransitCount() {
      return this.allShipments.filter((s) => s.status === 'DISPATCHED' || s.status === 'IN_TRANSIT').length
    },
    deliveredCount() {
      return this.allShipments.filter((s) => s.status === 'DELIVERED').length
    },
    issueCount() {
      return this.allShipments.filter((s) => s.status === 'ISSUE').length
    },
    canCreate() {
      return hasPermission('shipping.create')
    },
    canEditHeader() {
      return hasPermission('shipping.update')
    },
    // Confirm (dispatch/deliver/issue) is gated on shipping.approve on
    // the backend, the same significant-action permission Receiving/
    // Putaway's own Confirm uses — see internal/shipping/handler.go's doc.
    canConfirm() {
      return hasPermission('shipping.approve')
    }
  },

  watch: {
    'filters.keyword'() {
      clearTimeout(searchDebounce)
      searchDebounce = setTimeout(() => this.fetchShipments(), 300)
    },
    'filters.status'() {
      this.fetchShipments()
    },
    'filters.carrier'() {
      this.fetchShipments()
    },
    'filters.date'() {
      this.fetchShipments()
    }
  },

  mounted() {
    this.fetchShipments()
    this.fetchSummary()
  },

  methods: {
    async fetchShipments() {
      this.loading = true
      this.loadError = ''
      try {
        const res = await listShipments({
          search: this.filters.keyword,
          status: this.filters.status,
          carrier: this.filters.carrier,
          date: this.filters.date,
          limit: 100
        })
        this.shippingList = res.data.map(mapShipmentRow)
      } catch (err) {
        this.loadError = err.message
      } finally {
        this.loading = false
      }
    },

    // A separate, unfiltered fetch backing the summary cards — no
    // dedicated stats endpoint this phase, the same approach
    // Receiving/Putaway/Picking use.
    async fetchSummary() {
      try {
        const res = await listShipments({ limit: 200 })
        this.allShipments = res.data
      } catch {
        // Non-critical: the table's own error banner already surfaces API failures.
      }
    },

    clearFilter() {
      this.filters.keyword = ''
      this.filters.status = ''
      this.filters.carrier = ''
      this.filters.date = ''
      this.fetchShipments()
    },

    async exportCsv() {
      this.exporting = true
      try {
        await exportShipments({
          search: this.filters.keyword, status: this.filters.status,
          carrier: this.filters.carrier, date: this.filters.date
        })
      } catch (err) {
        this.loadError = err.message
      } finally {
        this.exporting = false
      }
    },

    async openShipping(item) {
      this.drawerError = ''
      this.submitting = false
      this.cancelRemark = ''

      if (!item) {
        this.isNewShipment = true
        this.selectedShipping = {}
        this.form = emptyForm()
        try {
          const res = await listShippableSources()
          this.sources = res.data
        } catch (err) {
          this.drawerError = err.message
        }
        return
      }

      this.isNewShipment = false
      try {
        const res = await getShipment(item.id)
        this.selectedShipping = {
          ...res.data,
          shipDate: res.data.ship_date ? res.data.ship_date.slice(0, 10) : ''
        }
        this.form = {
          pickingTaskId: '',
          carrier: res.data.carrier || '',
          trackingNo: res.data.tracking_no || '',
          packages: res.data.packages,
          weight: res.data.weight,
          shipDate: this.selectedShipping.shipDate,
          address: res.data.address || '',
          remark: res.data.remark || ''
        }
      } catch (err) {
        this.drawerError = err.message
        this.selectedShipping = null
      }
    },

    closeDrawer() {
      this.selectedShipping = null
      this.isNewShipment = false
    },

    async saveNewShipment() {
      this.drawerError = ''
      if (!this.form.pickingTaskId) {
        this.drawerError = 'Please select a source picking task'
        return
      }

      this.submitting = true
      try {
        await createShipment({
          picking_task_id: this.form.pickingTaskId,
          carrier: this.form.carrier || null,
          tracking_no: this.form.trackingNo || null,
          packages: this.form.packages || 1,
          weight: this.form.weight,
          ship_date: toApiDate(this.form.shipDate),
          address: this.form.address || null,
          remark: this.form.remark || null
        })
        alert('Shipment created successfully')
        this.closeDrawer()
        await this.fetchShipments()
        await this.fetchSummary()
      } catch (err) {
        this.drawerError = err.message
      } finally {
        this.submitting = false
      }
    },

    async saveHeaderChanges() {
      this.drawerError = ''
      this.submitting = true
      try {
        await updateShipment(this.selectedShipping.id, {
          carrier: this.form.carrier || null,
          tracking_no: this.form.trackingNo || null,
          packages: this.form.packages || 1,
          weight: this.form.weight,
          ship_date: toApiDate(this.form.shipDate),
          address: this.form.address || null,
          remark: this.form.remark || null
        })
        await this.openShipping({ id: this.selectedShipping.id })
        await this.fetchShipments()
      } catch (err) {
        this.drawerError = err.message
      } finally {
        this.submitting = false
      }
    },

    async confirmDispatch() {
      await this.submitConfirm('DISPATCH')
    },

    async markDelivered() {
      await this.submitConfirm('DELIVER')
    },

    async reportIssue() {
      if (!this.form.remark) {
        this.drawerError = 'A remark is required to report an issue'
        return
      }
      await this.submitConfirm('ISSUE', this.form.remark)
    },

    async submitConfirm(action, remark) {
      this.drawerError = ''
      this.submitting = true
      try {
        const payload = { idempotency_key: newIdempotencyKey(), action }
        if (remark) payload.remark = remark

        await confirmShipment(this.selectedShipping.id, payload)
        alert(
          action === 'DISPATCH' ? 'Shipment dispatched successfully' :
          action === 'DELIVER' ? 'Shipment marked as delivered' :
          'Shipping issue has been reported'
        )
        await this.openShipping({ id: this.selectedShipping.id })
        await this.fetchShipments()
        await this.fetchSummary()
      } catch (err) {
        this.drawerError = err.message
      } finally {
        this.submitting = false
      }
    },

    async cancelSelectedShipment() {
      this.drawerError = ''
      this.submitting = true
      try {
        await cancelShipment(this.selectedShipping.id, {
          remark: this.cancelRemark || undefined
        })
        alert('Shipment cancelled')
        this.closeDrawer()
        await this.fetchShipments()
        await this.fetchSummary()
      } catch (err) {
        this.drawerError = err.message
      } finally {
        this.submitting = false
      }
    },

    formatDateTime(isoString) {
      if (!isoString) return '—'
      return new Date(isoString).toLocaleString()
    },

    getStatusClass(status) {
      if (status === 'READY') return 'badge-gray'
      if (status === 'DISPATCHED') return 'badge-blue'
      if (status === 'IN_TRANSIT') return 'badge-blue'
      if (status === 'DELIVERED') return 'badge-green'
      if (status === 'ISSUE') return 'badge-red'
      if (status === 'CANCELLED') return 'badge-red'
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
