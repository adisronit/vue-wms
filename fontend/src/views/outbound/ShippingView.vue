<template>
  <div class="space-y-6">
    <div class="flex flex-col xl:flex-row xl:items-center xl:justify-between gap-4">
      <div>
        <h1 class="text-2xl font-bold text-slate-800">{{ $t('shipping.title') }}</h1>
        <p class="text-slate-500">
          {{ $t('shipping.subtitle') }}
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
          @click="openShipping(null)"
          class="bg-orange-600 hover:bg-orange-700 text-white px-5 py-3 rounded-xl font-semibold"
        >
          {{ $t('shipping.newShipmentButton') }}
        </button>
      </div>
    </div>

    <div class="grid grid-cols-1 md:grid-cols-4 gap-5">
      <div class="summary-card">
        <p class="summary-label">{{ $t('shipping.summary.readyToShip') }}</p>
        <h2 class="summary-value">{{ readyCount }}</h2>
      </div>

      <div class="summary-card">
        <p class="summary-label">{{ $t('shipping.summary.inTransit') }}</p>
        <h2 class="summary-value">{{ inTransitCount }}</h2>
      </div>

      <div class="summary-card">
        <p class="summary-label">{{ $t('shipping.summary.delivered') }}</p>
        <h2 class="summary-value">{{ deliveredCount }}</h2>
      </div>

      <div class="summary-card">
        <p class="summary-label">{{ $t('shipping.summary.deliveryIssues') }}</p>
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
          :placeholder="$t('shipping.filters.searchPlaceholder')"
          class="input"
        />

        <select v-model="filters.status" class="input">
          <option value="">{{ $t('shipping.filters.allStatus') }}</option>
          <option value="READY">{{ $t('shipping.filters.statusReady') }}</option>
          <option value="DISPATCHED">{{ $t('shipping.filters.statusDispatched') }}</option>
          <option value="IN_TRANSIT">{{ $t('shipping.filters.statusInTransit') }}</option>
          <option value="DELIVERED">{{ $t('shipping.filters.statusDelivered') }}</option>
          <option value="ISSUE">{{ $t('shipping.filters.statusIssue') }}</option>
          <option value="CANCELLED">{{ $t('common.status.cancelled') }}</option>
        </select>

        <!-- Carrier option labels (DHL, Kerry Express, Flash Express) are
             real-world carrier brand names / business data, left
             untranslated per "preserve original business values". -->
        <select v-model="filters.carrier" class="input">
          <option value="">{{ $t('shipping.filters.allCarriers') }}</option>
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
          {{ $t('common.buttons.clearFilter') }}
        </button>
      </div>
    </div>

    <div class="bg-white rounded-2xl border border-slate-200 shadow-sm overflow-hidden">
      <div class="w-full overflow-x-auto">
        <table class="min-w-[1250px] w-full text-sm">
          <thead class="bg-slate-50 text-slate-500">
            <tr>
              <th class="table-th">{{ $t('shipping.table.shipmentNo') }}</th>
              <th class="table-th">{{ $t('shipping.table.orderNo') }}</th>
              <th class="table-th">{{ $t('shipping.table.customer') }}</th>
              <th class="table-th">{{ $t('shipping.table.carrier') }}</th>
              <th class="table-th">{{ $t('shipping.table.trackingNo') }}</th>
              <th class="table-th">{{ $t('shipping.table.packages') }}</th>
              <th class="table-th">{{ $t('shipping.table.weight') }}</th>
              <th class="table-th">{{ $t('shipping.table.shipDate') }}</th>
              <th class="table-th">{{ $t('shipping.table.status') }}</th>
              <th class="table-th text-right">{{ $t('shipping.table.action') }}</th>
            </tr>
          </thead>

          <tbody>
            <tr v-if="loading">
              <td colspan="10" class="text-center py-10 text-slate-400">{{ $t('shipping.loadingShipments') }}</td>
            </tr>

            <tr v-else-if="shippingList.length === 0">
              <td colspan="10" class="text-center py-10 text-slate-400">{{ $t('shipping.noShipments') }}</td>
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
                  {{ statusText(item.status) }}
                </span>
              </td>

              <td class="table-td text-right">
                <button
                  @click="openShipping(item)"
                  class="text-orange-600 hover:text-orange-700 font-semibold"
                >
                  {{ $t('shipping.table.manage') }}
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
              {{ isNewShipment ? $t('shipping.drawer.newShipmentTitle') : $t('shipping.drawer.detailTitle') }}
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
          <label class="label">{{ $t('shipping.drawer.sourcePickingTask') }}</label>
          <select v-model="form.pickingTaskId" class="input">
            <option value="">{{ $t('shipping.drawer.selectSourceTask') }}</option>
            <option v-for="src in sources" :key="src.picking_task_id" :value="src.picking_task_id">
              {{ src.pick_no }} / {{ src.order_no }} {{ $t('shipping.drawer.itemCountSuffix', { count: src.item_count, plural: src.item_count === 1 ? '' : 's' }) }}
            </option>
          </select>
          <p v-if="sources.length === 0" class="text-xs text-slate-400 mt-2">
            {{ $t('shipping.drawer.noSourceTasks') }}
          </p>
        </div>

        <!-- Existing shipment: read-only summary -->
        <div v-else class="bg-slate-50 rounded-2xl p-5 mb-6">
          <div class="grid grid-cols-2 md:grid-cols-4 gap-4">
            <div>
              <p class="detail-label">{{ $t('shipping.drawer.customer') }}</p>
              <p class="detail-value">{{ selectedShipping.customer }}</p>
            </div>

            <div>
              <p class="detail-label">{{ $t('shipping.drawer.carrierLabel') }}</p>
              <p class="detail-value text-orange-600">{{ selectedShipping.carrier || '—' }}</p>
            </div>

            <div>
              <p class="detail-label">{{ $t('shipping.drawer.packages') }}</p>
              <p class="detail-value">{{ selectedShipping.packages }}</p>
            </div>

            <div>
              <p class="detail-label">{{ $t('shipping.drawer.totalWeight') }}</p>
              <p class="detail-value">{{ selectedShipping.weight != null ? selectedShipping.weight + ' kg' : '—' }}</p>
            </div>
          </div>
        </div>

        <!-- Cancellation details (Phase 18) -->
        <div v-if="!isNewShipment && selectedShipping.status === 'CANCELLED'" class="bg-red-50 border border-red-200 rounded-2xl p-5 mb-6">
          <div class="grid grid-cols-2 md:grid-cols-3 gap-4">
            <div>
              <p class="detail-label">{{ $t('shipping.drawer.cancelledAt') }}</p>
              <p class="detail-value">{{ formatDateTime(selectedShipping.cancelled_at) }}</p>
            </div>
            <div>
              <p class="detail-label">{{ $t('shipping.drawer.cancelledBy') }}</p>
              <p class="detail-value">{{ selectedShipping.cancelled_by_name || '—' }}</p>
            </div>
            <div>
              <p class="detail-label">{{ $t('shipping.drawer.cancellationRemark') }}</p>
              <p class="detail-value">{{ selectedShipping.cancel_remark || '—' }}</p>
            </div>
          </div>
        </div>

        <div v-if="isNewShipment || canEditHeader && selectedShipping.status === 'READY'" class="space-y-4">
          <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div>
              <label class="label">{{ $t('shipping.drawer.carrierLabel') }}</label>
              <!-- Carrier option labels are real-world carrier brand
                   names / business data, left untranslated. -->
              <select v-model="form.carrier" class="input">
                <option value="">{{ $t('shipping.drawer.selectCarrier') }}</option>
                <option value="DHL">DHL</option>
                <option value="KERRY">Kerry Express</option>
                <option value="FLASH">Flash Express</option>
                <option value="INTERNAL">Internal Truck</option>
              </select>
            </div>

            <div>
              <label class="label">{{ $t('shipping.drawer.trackingNo') }}</label>
              <input
                v-model="form.trackingNo"
                type="text"
                class="input"
                :placeholder="$t('shipping.drawer.trackingNoPlaceholder')"
              />
            </div>
          </div>

          <div class="grid grid-cols-1 md:grid-cols-3 gap-4">
            <div>
              <label class="label">{{ $t('shipping.drawer.packageCount') }}</label>
              <input
                v-model.number="form.packages"
                type="number"
                class="input"
                :placeholder="$t('shipping.drawer.packagesPlaceholder')"
              />
            </div>

            <div>
              <label class="label">{{ $t('shipping.drawer.totalWeightField') }}</label>
              <input
                v-model.number="form.weight"
                type="number"
                class="input"
                :placeholder="$t('shipping.drawer.weightPlaceholder')"
              />
            </div>

            <div>
              <label class="label">{{ $t('shipping.drawer.shipDate') }}</label>
              <input
                v-model="form.shipDate"
                type="date"
                class="input"
              />
            </div>
          </div>

          <div>
            <label class="label">{{ $t('shipping.drawer.deliveryAddress') }}</label>
            <textarea
              v-model="form.address"
              rows="4"
              class="input"
              :placeholder="$t('shipping.drawer.addressPlaceholder')"
            ></textarea>
          </div>

          <div>
            <label class="label">{{ $t('shipping.drawer.remark') }}</label>
            <textarea
              v-model="form.remark"
              rows="3"
              class="input"
              :placeholder="$t('shipping.drawer.remarkPlaceholder')"
            ></textarea>
          </div>

          <button
            v-if="isNewShipment"
            @click="saveNewShipment"
            :disabled="submitting"
            class="w-full bg-orange-600 hover:bg-orange-700 text-white py-3 rounded-xl font-semibold disabled:opacity-60"
          >
            {{ submitting ? $t('shipping.drawer.saving') : $t('shipping.drawer.saveShipment') }}
          </button>
          <button
            v-else
            @click="saveHeaderChanges"
            :disabled="submitting"
            class="w-full bg-slate-800 hover:bg-slate-900 text-white py-3 rounded-xl font-semibold disabled:opacity-60"
          >
            {{ submitting ? $t('shipping.drawer.saving') : $t('shipping.drawer.saveChanges') }}
          </button>
        </div>

        <div v-if="!isNewShipment" class="mt-6">
          <h3 class="font-bold text-slate-800 mb-4">
            {{ $t('shipping.drawer.packageItemsTitle') }}
          </h3>

          <p v-if="(selectedShipping.items || []).length === 0" class="text-sm text-slate-400">
            {{ $t('shipping.drawer.noItemsOnShipment') }}
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
                <p class="detail-label">{{ $t('shipping.drawer.qty') }}</p>
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
            {{ $t('shipping.drawer.confirmDispatch') }}
          </button>

          <button
            v-if="selectedShipping.status === 'DISPATCHED'"
            @click="markDelivered"
            :disabled="submitting"
            class="bg-green-600 hover:bg-green-700 text-white py-3 rounded-xl font-semibold disabled:opacity-60"
          >
            {{ $t('shipping.drawer.markDelivered') }}
          </button>

          <button
            v-if="selectedShipping.status === 'READY' || selectedShipping.status === 'DISPATCHED'"
            @click="reportIssue"
            :disabled="submitting"
            class="bg-red-600 hover:bg-red-700 text-white py-3 rounded-xl font-semibold disabled:opacity-60"
          >
            {{ $t('shipping.drawer.reportIssue') }}
          </button>
        </div>
        <p v-else-if="!isNewShipment" class="text-sm text-slate-500 mt-6">
          {{ $t('shipping.drawer.viewOnlyNote') }}
        </p>

        <div v-if="!isNewShipment && canEditHeader && selectedShipping.status === 'READY'" class="mt-6 pt-6 border-t border-slate-200">
          <input
            v-model="cancelRemark"
            type="text"
            class="input mb-3"
            :placeholder="$t('shipping.drawer.cancelRemarkPlaceholder')"
          />
          <button
            @click="cancelSelectedShipment"
            :disabled="submitting"
            class="w-full bg-slate-800 hover:bg-slate-900 text-white py-3 rounded-xl font-semibold disabled:opacity-60"
          >
            {{ $t('shipping.drawer.cancelShipment') }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script>
import { listShipments, getShipment, listShippableSources, createShipment, updateShipment, confirmShipment, cancelShipment, newIdempotencyKey, exportShipments } from '../../services/shipping'
import { hasPermission } from '../../services/auth'
import i18n from '../../i18n'

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
// Called from the template (methods.statusText below), not cached onto
// a row at map time, so the displayed label stays reactive to the
// active locale — switching language must not require re-fetching.
function statusText(status) {
  const t = i18n.global.t
  switch (status) {
    case 'READY': return t('shipping.statusLabels.readyToShip')
    case 'DISPATCHED': return t('shipping.statusLabels.inTransit')
    case 'IN_TRANSIT': return t('shipping.statusLabels.inTransit')
    case 'DELIVERED': return t('shipping.statusLabels.delivered')
    case 'ISSUE': return t('shipping.statusLabels.issue')
    case 'CANCELLED': return t('common.status.cancelled')
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
    status: r.status
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
    statusText,

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
        this.drawerError = this.$t('shipping.messages.selectSourceRequired')
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
        alert(this.$t('shipping.messages.shipmentCreated'))
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
        this.drawerError = this.$t('shipping.messages.remarkRequiredForIssue')
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
          action === 'DISPATCH' ? this.$t('shipping.messages.dispatchedSuccess') :
          action === 'DELIVER' ? this.$t('shipping.messages.deliveredSuccess') :
          this.$t('shipping.messages.issueReported')
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
        alert(this.$t('shipping.messages.shipmentCancelled'))
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
      return this.$d(new Date(isoString), 'long')
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
  font-weight: var(--fw-label);
  font-size: var(--fs-label);
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
