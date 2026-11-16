<template>
  <div class="space-y-6">
    <PageHeader :title="order ? order.order_no : $t('orders.title')" :subtitle="$t('orders.detailPage.subtitle')">
      <template #actions>
        <button
          @click="$router.push('/outbound/orders')"
          class="bg-slate-100 hover:bg-slate-200 text-slate-700 px-5 py-3 rounded-xl font-semibold"
        >
          {{ $t('newRelocation.backButton') }}
        </button>
        <button v-if="order" @click="viewPdf" class="bg-slate-100 hover:bg-slate-200 text-slate-700 px-5 py-3 rounded-xl font-semibold">
          {{ $t('orders.detailPage.viewPdf') }}
        </button>
        <button v-if="order" @click="downloadPdf" class="bg-slate-100 hover:bg-slate-200 text-slate-700 px-5 py-3 rounded-xl font-semibold">
          {{ $t('orders.detailPage.downloadPdf') }}
        </button>
      </template>
    </PageHeader>

    <div v-if="loadError" class="bg-red-50 border border-red-200 text-red-600 rounded-xl p-3 text-sm">{{ loadError }}</div>
    <div v-else-if="loading" class="text-sm text-slate-400">{{ $t('orders.loadingOrders') }}</div>

    <template v-else-if="order">
      <FormSection :title="$t('orders.detail.statusLabel')">
        <div class="flex flex-wrap gap-4 items-center mb-4">
          <StatusBadge :tone="statusTone(order.status)">{{ statusLabel(order.status) }}</StatusBadge>
          <StatusBadge :tone="statusTone(order.fulfillment_status)">{{ statusLabel(order.fulfillment_status) }}</StatusBadge>
          <span v-if="order.shortage_approved" class="text-sm text-amber-600 font-semibold">{{ $t('orders.review.shortageApprovedBadge') }}</span>
        </div>

        <div class="grid grid-cols-1 md:grid-cols-3 gap-3 text-sm">
          <div><span class="field-label">{{ $t('orders.form.orderTypeLabel') }}:</span> {{ order.order_type }}</div>
          <div><span class="field-label">{{ $t('orders.form.priorityLabel') }}:</span> {{ order.priority }}</div>
          <div><span class="field-label">{{ $t('orders.detail.warehouseLabel') }}:</span> {{ order.warehouse_code }}</div>
          <div><span class="field-label">{{ $t('orders.detail.customerLabel') }}:</span> {{ order.customer || '—' }}</div>
          <div v-if="order.ship_to_name"><span class="field-label">{{ $t('orders.detail.shipToLabel') }}:</span> {{ order.ship_to_name }}</div>
          <div v-if="order.ship_to_address"><span class="field-label">{{ $t('orders.detail.shipToAddressLabel') }}:</span> {{ order.ship_to_address }}</div>
          <div><span class="field-label">{{ $t('orders.detail.externalRefLabel') }}:</span> {{ order.external_reference || '—' }}</div>
          <div v-if="order.required_shipping_date"><span class="field-label">{{ $t('orders.form.requiredShippingDateLabel') }}:</span> {{ formatDate(order.required_shipping_date) }}</div>
          <div v-if="order.required_delivery_date"><span class="field-label">{{ $t('orders.form.requiredDeliveryDateLabel') }}:</span> {{ formatDate(order.required_delivery_date) }}</div>
          <div v-if="order.carrier"><span class="field-label">{{ $t('orders.form.carrierPlaceholder') }}:</span> {{ order.carrier }}</div>
          <div v-if="order.shipping_method"><span class="field-label">{{ $t('orders.form.shippingMethodPlaceholder') }}:</span> {{ order.shipping_method }}</div>
          <div v-if="order.prepared_by"><span class="field-label">{{ $t('orders.form.preparedByPlaceholder') }}:</span> {{ order.prepared_by }}</div>
          <div v-if="order.approved_by"><span class="field-label">{{ $t('orders.form.approvedByPlaceholder') }}:</span> {{ order.approved_by }}</div>
          <div v-if="order.pick_no"><span class="field-label">{{ $t('orders.detail.pickingTaskLabel') }}:</span> {{ order.pick_no }}</div>
          <div><span class="field-label">{{ $t('orders.detail.createdByLabel') }}:</span> {{ order.created_by_name || '—' }}</div>
        </div>
        <p v-if="order.delivery_instructions" class="mt-3 text-sm"><span class="field-label">{{ $t('orders.form.deliveryInstructionsPlaceholder') }}:</span> {{ order.delivery_instructions }}</p>
        <p v-if="order.remark" class="mt-2 text-sm"><span class="field-label">{{ $t('orders.detail.remarkLabel') }}:</span> {{ order.remark }}</p>
        <p v-if="order.cancel_remark" class="mt-2 text-sm text-slate-600"><span class="field-label">{{ $t('orders.detail.cancelRemarkLabel') }}:</span> {{ order.cancel_remark }}</p>
      </FormSection>

      <FormSection :title="$t('orders.detail.itemsTitle')">
        <DataTable :is-empty="order.items.length === 0" :empty-text="$t('orders.form.noItemsYet')" :colspan="7" min-width="1000px">
          <template #head>
            <tr>
              <th class="table-th">{{ $t('orders.detail.tableSku') }}</th>
              <th class="table-th">{{ $t('orders.detail.tableProduct') }}</th>
              <th class="table-th text-right">{{ $t('orders.detail.tableQty') }}</th>
              <th class="table-th">{{ $t('orders.detail.tableUom') }}</th>
              <th class="table-th">{{ $t('orders.form.lotHintColumn') }}</th>
              <th class="table-th">{{ $t('orders.review.shortage') }}</th>
              <th class="table-th">{{ $t('orders.form.lineNotesColumn') }}</th>
            </tr>
          </template>
          <tr v-for="it in order.items" :key="it.id" class="border-t">
            <td class="table-td">{{ it.sku }}</td>
            <td class="table-td">{{ it.product_name }}</td>
            <td class="table-td text-right">{{ it.ordered_qty }}</td>
            <td class="table-td">{{ it.uom }}</td>
            <td class="table-td">{{ it.preferred_lot_no || '—' }}</td>
            <td class="table-td">
              <span v-if="it.shortage_flag" class="text-red-600 font-semibold">{{ $t('orders.review.shortfallOf', { qty: it.shortfall_qty }) }}</span>
              <span v-else>—</span>
            </td>
            <td class="table-td">{{ it.line_notes || '—' }}</td>
          </tr>
        </DataTable>
      </FormSection>

      <FormSection>
        <div class="flex flex-wrap gap-3">
          <button v-if="canUpdate && order.status === 'PENDING'" @click="$router.push(`/outbound/orders/${order.id}/edit`)" class="px-5 py-3 rounded-xl bg-slate-100 text-slate-700 font-semibold">
            {{ $t('common.buttons.edit') }}
          </button>
          <button v-if="canConfirm && order.status === 'PENDING'" @click="$router.push(`/outbound/orders/${order.id}/review`)" class="px-5 py-3 rounded-xl bg-orange-600 hover:bg-orange-700 text-white font-semibold">
            {{ $t('orders.detailPage.reviewAndRelease') }}
          </button>
          <button v-if="canUpdate && order.status === 'RELEASED'" @click="openReleasePanel" class="px-5 py-3 rounded-xl bg-orange-600 hover:bg-orange-700 text-white font-semibold">
            {{ $t('orders.detail.createPickingTask') }}
          </button>
          <button v-if="canApprove && (order.status === 'PENDING' || order.status === 'RELEASED')" @click="showCancelConfirm = true" class="px-5 py-3 rounded-xl bg-slate-100 text-slate-700 font-semibold">
            {{ $t('orders.detail.cancelOrder') }}
          </button>
        </div>
        <p v-if="order.status === 'PICKING'" class="text-xs text-slate-400 mt-3">{{ $t('orders.detail.pickingTaskNote') }}</p>
      </FormSection>
    </template>

    <!-- Create Picking Task (location/lot assignment) panel -->
    <div v-if="showReleasePanel" class="fixed inset-0 bg-black/30 flex items-center justify-center z-50 p-4">
      <div class="bg-white rounded-2xl shadow-xl w-full max-w-2xl p-6 space-y-4 max-h-[90vh] overflow-y-auto">
        <div class="flex items-center justify-between">
          <h2 class="text-lg font-bold text-slate-800">{{ $t('orders.release.title', { orderNo: order.order_no }) }}</h2>
          <button @click="showReleasePanel = false" class="text-slate-400 hover:text-slate-600">✕</button>
        </div>

        <p v-if="releaseError" class="bg-red-50 border border-red-200 text-red-600 rounded-xl p-3 text-sm">{{ releaseError }}</p>

        <div class="grid grid-cols-1 md:grid-cols-2 gap-3">
          <select v-model="releaseForm.priority" class="input">
            <option value="NORMAL">{{ $t('orders.release.priorityNormal') }}</option>
            <option value="LOW">{{ $t('orders.release.priorityLow') }}</option>
            <option value="HIGH">{{ $t('orders.release.priorityHigh') }}</option>
          </select>
          <input v-model="releaseForm.dueDate" type="date" class="input" />
        </div>

        <div class="space-y-3">
          <h3 class="font-semibold text-slate-700">{{ $t('orders.release.assignLocationTitle') }}</h3>
          <div v-for="a in releaseForm.assignments" :key="a.orderItemId" class="grid grid-cols-1 md:grid-cols-3 gap-2 items-center border-b pb-2">
            <div class="text-sm text-slate-600 md:col-span-1">{{ a.sku }} — {{ a.productName }} ({{ a.orderedQty }})</div>
            <select v-model="a.locationId" class="input">
              <option value="">{{ $t('orders.release.selectLocation') }}</option>
              <option v-for="l in releaseLocations" :key="l.id" :value="l.id">{{ l.code }}</option>
            </select>
            <input v-model="a.lotNo" type="text" :placeholder="$t('orders.release.lotNoPlaceholder')" class="input" />
          </div>
        </div>

        <div class="flex justify-end gap-3">
          <button @click="showReleasePanel = false" class="px-4 py-2 rounded-xl bg-slate-100 text-slate-700 font-semibold">{{ $t('common.buttons.cancel') }}</button>
          <button @click="submitCreatePickingTask" :disabled="releaseBusy" class="px-4 py-2 rounded-xl bg-orange-600 hover:bg-orange-700 text-white font-semibold disabled:opacity-60">
            {{ releaseBusy ? $t('orders.release.creating') : $t('orders.detail.createPickingTask') }}
          </button>
        </div>
      </div>
    </div>

    <ConfirmDialog
      :open="showCancelConfirm"
      :title="$t('orders.detail.cancelOrder')"
      :message="$t('orders.detail.cancelConfirmMessage')"
      :confirm-label="$t('orders.detail.cancelOrder')"
      :cancel-label="$t('common.buttons.cancel')"
      :loading-label="$t('common.buttons.submitting')"
      :loading="actionBusy"
      :error-message="cancelError"
      danger
      @cancel="showCancelConfirm = false"
      @confirm="doCancel"
    />
  </div>
</template>

<script>
import { getOrder, cancelOrder, createOrderPickingTask, viewOrderDocument, downloadOrderDocument } from '../../services/orders'
import { listZones, listLocations } from '../../services/warehouses'
import { hasPermission } from '../../services/auth'
import { showToast } from '../../services/toast'
import PageHeader from '../../components/ui/PageHeader.vue'
import FormSection from '../../components/ui/FormSection.vue'
import DataTable from '../../components/ui/DataTable.vue'
import StatusBadge from '../../components/ui/StatusBadge.vue'
import ConfirmDialog from '../../components/ui/ConfirmDialog.vue'

export default {
  name: 'OrderDetailView',
  components: { PageHeader, FormSection, DataTable, StatusBadge, ConfirmDialog },
  props: { id: { type: [String, Number], required: true } },
  data() {
    return {
      loading: false,
      loadError: '',
      order: null,

      showCancelConfirm: false,
      actionBusy: false,
      cancelError: '',

      showReleasePanel: false,
      releaseError: '',
      releaseBusy: false,
      releaseForm: { priority: 'NORMAL', dueDate: '', assignments: [] },
      releaseLocations: []
    }
  },
  computed: {
    canUpdate() { return hasPermission('order.update') },
    canConfirm() { return hasPermission('order.confirm') },
    canApprove() { return hasPermission('order.approve') }
  },
  async mounted() {
    await this.load()
  },
  methods: {
    async load() {
      this.loading = true
      this.loadError = ''
      try {
        const res = await getOrder(this.id)
        this.order = res.data
      } catch (err) {
        this.loadError = err.message || this.$t('orders.messages.loadOrderDetailFailed')
      } finally {
        this.loading = false
      }
    },

    formatDate(value) {
      if (!value) return '—'
      return this.$d(new Date(value), 'long')
    },

    statusTone(status) {
      if (status === 'DELIVERED' || status === 'PICKED' || status === 'COMPLETED') return 'green'
      if (status === 'PICKING' || status === 'SHIPPING' || status === 'RELEASED') return 'orange'
      if (status === 'SHIP_ISSUE') return 'red'
      return 'gray'
    },

    statusLabel(status) {
      switch (status) {
        case 'PENDING': return this.$t('orders.statusLabels.pending')
        case 'RELEASED': return this.$t('orders.statusLabels.released')
        case 'PICKING': return this.$t('orders.statusLabels.picking')
        case 'PICKING_CANCELLED': return this.$t('orders.statusLabels.pickingCancelled')
        case 'PICKED': return this.$t('orders.statusLabels.picked')
        case 'SHIPPING': return this.$t('orders.statusLabels.shipping')
        case 'SHIP_ISSUE': return this.$t('orders.statusLabels.shipIssue')
        case 'DELIVERED': return this.$t('orders.statusLabels.delivered')
        case 'CANCELLED': return this.$t('orders.statusLabels.cancelled')
        default: return status
      }
    },

    async viewPdf() {
      try {
        await viewOrderDocument(this.order.id)
      } catch (err) {
        showToast(err.message || this.$t('orders.detailPage.pdfFailed'), 'error')
      }
    },
    async downloadPdf() {
      try {
        await downloadOrderDocument(this.order.id)
      } catch (err) {
        showToast(err.message || this.$t('orders.detailPage.pdfFailed'), 'error')
      }
    },

    async doCancel() {
      this.actionBusy = true
      this.cancelError = ''
      try {
        const res = await cancelOrder(this.order.id, {})
        this.order = res.data
        this.showCancelConfirm = false
        showToast(this.$t('orders.detailPage.cancelSuccess'), 'success')
      } catch (err) {
        this.cancelError = err.message
      } finally {
        this.actionBusy = false
      }
    },

    async openReleasePanel() {
      this.releaseError = ''
      this.releaseForm = {
        priority: 'NORMAL', dueDate: '',
        assignments: this.order.items.map((it) => ({
          orderItemId: it.id, sku: it.sku, productName: it.product_name, orderedQty: it.ordered_qty,
          locationId: '', lotNo: it.preferred_lot_no || ''
        }))
      }
      this.releaseLocations = []
      try {
        const zonesRes = await listZones(this.order.warehouse_id)
        const allLocations = []
        for (const zone of zonesRes.data) {
          const locRes = await listLocations(zone.id)
          allLocations.push(...locRes.data)
        }
        this.releaseLocations = allLocations
      } catch (err) {
        this.releaseError = err.message || this.$t('orders.messages.loadLocationsFailed')
      }
      this.showReleasePanel = true
    },

    async submitCreatePickingTask() {
      this.releaseError = ''
      for (const a of this.releaseForm.assignments) {
        if (!a.locationId) {
          this.releaseError = this.$t('orders.messages.lineNeedsLocation')
          return
        }
      }

      this.releaseBusy = true
      try {
        const res = await createOrderPickingTask(this.order.id, {
          priority: this.releaseForm.priority,
          due_date: this.releaseForm.dueDate ? new Date(this.releaseForm.dueDate).toISOString() : undefined,
          items: this.releaseForm.assignments.map((a) => ({
            order_item_id: a.orderItemId, location_id: a.locationId, lot_no: a.lotNo || undefined
          }))
        })
        this.order = res.data
        this.showReleasePanel = false
        showToast(this.$t('orders.detailPage.pickingTaskCreated'), 'success')
      } catch (err) {
        this.releaseError = err.message || this.$t('orders.messages.createPickingTaskFailed')
      } finally {
        this.releaseBusy = false
      }
    }
  }
}
</script>

<style scoped>
.field-label {
  color: #64748b;
  font-weight: 600;
}

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
  padding: 10px 12px;
  color: #475569;
  white-space: nowrap;
}
</style>
