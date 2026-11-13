<template>
  <div class="space-y-6">
    <PageHeader :title="$t('orders.title')" :subtitle="$t('orders.subtitle')">
      <template #actions>
        <button
          v-if="canCreate"
          @click="openNewOrder"
          class="bg-orange-600 hover:bg-orange-700 text-white px-5 py-3 rounded-xl font-semibold"
        >
          {{ $t('orders.newButton') }}
        </button>
      </template>
    </PageHeader>

    <div v-if="loadError" class="bg-red-50 border border-red-200 text-red-600 rounded-xl p-3 text-sm">
      {{ loadError }}
    </div>

    <FilterBar>
      <input v-model="filters.search" type="text" :placeholder="$t('orders.filters.searchPlaceholder')" class="input" />

      <select v-model="filters.status" class="input">
        <option value="">{{ $t('orders.filters.allStatus') }}</option>
        <option value="PENDING">{{ $t('common.status.pending') }}</option>
        <option value="PICKING">{{ $t('nav.picking') }}</option>
        <option value="CANCELLED">{{ $t('common.status.cancelled') }}</option>
      </select>

      <select v-model="filters.warehouseId" class="input">
        <option value="">{{ $t('orders.filters.allWarehouses') }}</option>
        <option v-for="w in warehouses" :key="w.id" :value="w.id">{{ w.code }}</option>
      </select>

      <button @click="clearFilter" class="bg-slate-100 hover:bg-slate-200 text-slate-700 rounded-xl font-semibold">
        {{ $t('common.buttons.clearFilter') }}
      </button>
    </FilterBar>

    <DataTable
      :loading="loading"
      :is-empty="orderList.length === 0"
      :loading-text="$t('orders.loadingOrders')"
      :empty-text="$t('orders.noOrders')"
      :colspan="9"
      min-width="1000px"
    >
      <template #head>
        <tr>
          <th class="table-th">{{ $t('orders.table.orderNo') }}</th>
          <th class="table-th">{{ $t('orders.table.externalRef') }}</th>
          <th class="table-th">{{ $t('orders.table.customer') }}</th>
          <th class="table-th">{{ $t('common.labels.warehouse') }}</th>
          <th class="table-th text-right">{{ $t('orders.table.items') }}</th>
          <th class="table-th">{{ $t('common.labels.status') }}</th>
          <th class="table-th">{{ $t('orders.table.fulfillment') }}</th>
          <th class="table-th">{{ $t('orders.table.createdAt') }}</th>
          <th class="table-th text-right">{{ $t('orders.table.action') }}</th>
        </tr>
      </template>

      <tr v-for="o in orderList" :key="o.id" class="border-t hover:bg-orange-50">
        <td class="table-td font-semibold text-slate-800">{{ o.order_no }}</td>
        <td class="table-td">{{ o.external_reference || '—' }}</td>
        <td class="table-td">{{ o.customer || '—' }}</td>
        <td class="table-td">{{ o.warehouse_code }}</td>
        <td class="table-td text-right">{{ o.item_count }}</td>
        <td class="table-td"><StatusBadge :tone="statusTone(o.status)">{{ statusLabel(o.status) }}</StatusBadge></td>
        <td class="table-td"><StatusBadge :tone="statusTone(o.fulfillment_status)">{{ statusLabel(o.fulfillment_status) }}</StatusBadge></td>
        <td class="table-td">{{ formatDate(o.created_at) }}</td>
        <td class="table-td text-right">
          <button @click="openDetail(o.id)" class="text-orange-600 hover:underline font-semibold">{{ $t('common.buttons.view') }}</button>
        </td>
      </tr>
    </DataTable>

    <!-- New/Edit Order panel -->
    <div v-if="showForm" class="fixed inset-0 bg-black/30 flex items-center justify-center z-50 p-4">
      <div class="bg-white rounded-2xl shadow-xl w-full max-w-2xl p-6 space-y-4 max-h-[90vh] overflow-y-auto">
        <div class="flex items-center justify-between">
          <h2 class="text-lg font-bold text-slate-800">{{ formMode === 'edit' ? $t('orders.form.editTitle') : $t('orders.form.newTitle') }}</h2>
          <button @click="showForm = false" class="text-slate-400 hover:text-slate-600">✕</button>
        </div>

        <p v-if="formError" class="bg-red-50 border border-red-200 text-red-600 rounded-xl p-3 text-sm">{{ formError }}</p>

        <div class="grid grid-cols-1 md:grid-cols-2 gap-3">
          <select v-model="form.warehouseId" class="input">
            <option value="">{{ $t('orders.form.selectWarehouse') }}</option>
            <option v-for="w in warehouses" :key="w.id" :value="w.id">{{ w.code }} — {{ w.name }}</option>
          </select>

          <input v-model="form.customer" type="text" :placeholder="$t('orders.form.customerPlaceholder')" class="input" />

          <input v-model="form.externalReference" type="text" :placeholder="$t('orders.form.externalReferencePlaceholder')" class="input" />

          <textarea v-model="form.remark" :placeholder="$t('orders.form.remarkPlaceholder')" class="input md:col-span-2" rows="2"></textarea>
        </div>

        <div class="border-t pt-3 space-y-3">
          <h3 class="font-semibold text-slate-700">{{ $t('orders.form.itemsTitle') }}</h3>

          <div class="grid grid-cols-1 md:grid-cols-4 gap-2">
            <input v-model="itemDraft.sku" type="text" :placeholder="$t('orders.form.skuPlaceholder')" class="input md:col-span-2" />
            <input v-model.number="itemDraft.orderedQty" type="number" min="0" step="0.01" :placeholder="$t('orders.form.qtyPlaceholder')" class="input" />
            <button @click="addItem" class="px-3 py-2 rounded-xl bg-slate-100 hover:bg-slate-200 text-slate-700 font-semibold">
              {{ $t('orders.form.addItem') }}
            </button>
          </div>

          <table v-if="form.items.length" class="min-w-full text-sm">
            <thead class="bg-slate-50 text-slate-500">
              <tr>
                <th class="table-th">{{ $t('orders.form.tableSku') }}</th>
                <th class="table-th">{{ $t('orders.form.tableProduct') }}</th>
                <th class="table-th">{{ $t('orders.form.tableQty') }}</th>
                <th class="table-th"></th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="(it, idx) in form.items" :key="idx" class="border-t">
                <td class="table-td">{{ it.sku }}</td>
                <td class="table-td">{{ it.productName }}</td>
                <td class="table-td">{{ it.orderedQty }}</td>
                <td class="table-td text-right">
                  <button @click="form.items.splice(idx, 1)" class="text-red-500 hover:underline">{{ $t('common.buttons.remove') }}</button>
                </td>
              </tr>
            </tbody>
          </table>
          <p v-else class="text-sm text-slate-400">{{ $t('orders.form.noItemsYet') }}</p>
        </div>

        <div class="flex justify-end gap-3">
          <button @click="showForm = false" class="px-4 py-2 rounded-xl bg-slate-100 text-slate-700 font-semibold">{{ $t('common.buttons.cancel') }}</button>
          <button @click="submitForm" :disabled="submitting" class="px-4 py-2 rounded-xl bg-orange-600 hover:bg-orange-700 text-white font-semibold disabled:opacity-60">
            {{ submitting ? $t('orders.form.saving') : (formMode === 'edit' ? $t('orders.form.saveChanges') : $t('orders.form.createOrder')) }}
          </button>
        </div>
      </div>
    </div>

    <!-- Detail drawer -->
    <div v-if="selectedOrder" class="fixed inset-0 bg-black/30 flex items-center justify-end z-50">
      <div class="bg-white h-full w-full max-w-lg p-6 space-y-4 overflow-y-auto">
        <div class="flex items-center justify-between">
          <h2 class="text-lg font-bold text-slate-800">{{ selectedOrder.order_no }}</h2>
          <button @click="closeDetail" class="text-slate-400 hover:text-slate-600">✕</button>
        </div>

        <p v-if="detailError" class="bg-red-50 border border-red-200 text-red-600 rounded-xl p-3 text-sm">{{ detailError }}</p>

        <div class="space-y-2 text-sm">
          <p><span class="text-slate-500">{{ $t('orders.detail.statusLabel') }}:</span> <StatusBadge :tone="statusTone(selectedOrder.status)">{{ statusLabel(selectedOrder.status) }}</StatusBadge></p>
          <p><span class="text-slate-500">{{ $t('orders.detail.fulfillmentLabel') }}:</span> <StatusBadge :tone="statusTone(selectedOrder.fulfillment_status)">{{ statusLabel(selectedOrder.fulfillment_status) }}</StatusBadge></p>
          <p><span class="text-slate-500">{{ $t('orders.detail.customerLabel') }}:</span> {{ selectedOrder.customer || '—' }}</p>
          <p><span class="text-slate-500">{{ $t('orders.detail.externalRefLabel') }}:</span> {{ selectedOrder.external_reference || '—' }}</p>
          <p><span class="text-slate-500">{{ $t('orders.detail.warehouseLabel') }}:</span> {{ selectedOrder.warehouse_code }}</p>
          <p v-if="selectedOrder.pick_no"><span class="text-slate-500">{{ $t('orders.detail.pickingTaskLabel') }}:</span> {{ selectedOrder.pick_no }}</p>
          <p v-if="selectedOrder.remark"><span class="text-slate-500">{{ $t('orders.detail.remarkLabel') }}:</span> {{ selectedOrder.remark }}</p>
          <p v-if="selectedOrder.cancel_remark" class="text-slate-600"><span class="text-slate-500">{{ $t('orders.detail.cancelRemarkLabel') }}:</span> {{ selectedOrder.cancel_remark }}</p>
          <p><span class="text-slate-500">{{ $t('orders.detail.createdByLabel') }}:</span> {{ selectedOrder.created_by_name || '—' }}</p>
        </div>

        <div class="border-t pt-3">
          <h3 class="font-semibold text-slate-700 mb-2">{{ $t('orders.detail.itemsTitle') }}</h3>
          <table class="min-w-full text-sm">
            <thead class="bg-slate-50 text-slate-500">
              <tr>
                <th class="table-th">{{ $t('orders.detail.tableSku') }}</th>
                <th class="table-th">{{ $t('orders.detail.tableProduct') }}</th>
                <th class="table-th">{{ $t('orders.detail.tableQty') }}</th>
                <th class="table-th">{{ $t('orders.detail.tableUom') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="it in selectedOrder.items" :key="it.id" class="border-t">
                <td class="table-td">{{ it.sku }}</td>
                <td class="table-td">{{ it.product_name }}</td>
                <td class="table-td">{{ it.ordered_qty }}</td>
                <td class="table-td">{{ it.uom }}</td>
              </tr>
            </tbody>
          </table>
        </div>

        <div v-if="canUpdate && selectedOrder.status === 'PENDING'" class="flex gap-3 border-t pt-3">
          <button @click="openEditOrder" class="px-4 py-2 rounded-xl bg-slate-100 text-slate-700 font-semibold">
            {{ $t('common.buttons.edit') }}
          </button>
          <button @click="openReleasePanel" class="px-4 py-2 rounded-xl bg-orange-600 hover:bg-orange-700 text-white font-semibold">
            {{ $t('orders.detail.createPickingTask') }}
          </button>
        </div>

        <div v-if="canApprove && selectedOrder.status === 'PENDING'" class="space-y-2 pt-2 border-t">
          <textarea v-model="cancelRemark" :placeholder="$t('orders.detail.cancelRemarkPlaceholder')" class="input" rows="2"></textarea>
          <button @click="doCancel" :disabled="actionBusy" class="w-full px-4 py-2 rounded-xl bg-slate-100 text-slate-700 font-semibold disabled:opacity-60">
            {{ $t('orders.detail.cancelOrder') }}
          </button>
        </div>

        <p v-if="selectedOrder.status === 'PICKING'" class="text-xs text-slate-400 pt-2 border-t">
          {{ $t('orders.detail.pickingTaskNote') }}
        </p>
      </div>
    </div>

    <!-- Create Picking Task (release) panel -->
    <div v-if="showReleasePanel" class="fixed inset-0 bg-black/30 flex items-center justify-center z-50 p-4">
      <div class="bg-white rounded-2xl shadow-xl w-full max-w-2xl p-6 space-y-4 max-h-[90vh] overflow-y-auto">
        <div class="flex items-center justify-between">
          <h2 class="text-lg font-bold text-slate-800">{{ $t('orders.release.title', { orderNo: selectedOrder.order_no }) }}</h2>
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
          <button @click="submitRelease" :disabled="releaseBusy" class="px-4 py-2 rounded-xl bg-orange-600 hover:bg-orange-700 text-white font-semibold disabled:opacity-60">
            {{ releaseBusy ? $t('orders.release.creating') : $t('orders.detail.createPickingTask') }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script>
import {
  listOrders, getOrder, createOrder, updateOrder, createOrderPickingTask, cancelOrder
} from '../../services/orders'
import { listWarehouses, listZones, listLocations } from '../../services/warehouses'
import { listProducts } from '../../services/products'
import { hasPermission } from '../../services/auth'
import PageHeader from '../../components/ui/PageHeader.vue'
import FilterBar from '../../components/ui/FilterBar.vue'
import DataTable from '../../components/ui/DataTable.vue'
import StatusBadge from '../../components/ui/StatusBadge.vue'

export default {
  name: 'OrderPage',
  components: { PageHeader, FilterBar, DataTable, StatusBadge },
  data() {
    return {
      loading: false,
      loadError: '',
      orderList: [],
      warehouses: [],
      filters: { search: '', status: '', warehouseId: '' },

      showForm: false,
      formMode: 'create',
      editingOrderId: null,
      formError: '',
      submitting: false,
      form: { warehouseId: '', customer: '', externalReference: '', remark: '', items: [] },
      itemDraft: { sku: '', orderedQty: null },

      selectedOrder: null,
      detailError: '',
      actionBusy: false,
      cancelRemark: '',

      showReleasePanel: false,
      releaseError: '',
      releaseBusy: false,
      releaseForm: { priority: 'NORMAL', dueDate: '', assignments: [] },
      releaseLocations: []
    }
  },
  computed: {
    canCreate() { return hasPermission('order.create') },
    canUpdate() { return hasPermission('order.update') },
    canApprove() { return hasPermission('order.approve') }
  },
  async mounted() {
    await this.fetchWarehouses()
    await this.fetchOrders()
  },
  methods: {
    async fetchWarehouses() {
      try {
        const res = await listWarehouses({ limit: 100 })
        this.warehouses = res.data
      } catch (err) {
        this.loadError = err.message || this.$t('orders.messages.loadWarehousesFailed')
      }
    },

    async fetchOrders() {
      this.loading = true
      this.loadError = ''
      try {
        const res = await listOrders(this.filters)
        this.orderList = res.data
      } catch (err) {
        this.loadError = err.message || this.$t('orders.messages.loadOrdersFailed')
      } finally {
        this.loading = false
      }
    },

    clearFilter() {
      this.filters = { search: '', status: '', warehouseId: '' }
      this.fetchOrders()
    },

    formatDate(value) {
      if (!value) return '—'
      return this.$d(new Date(value), 'long')
    },

    statusTone(status) {
      if (status === 'DELIVERED' || status === 'PICKED' || status === 'COMPLETED') return 'green'
      if (status === 'PICKING' || status === 'SHIPPING') return 'orange'
      if (status === 'SHIP_ISSUE') return 'red'
      return 'gray'
    },

    // Covers both order.status (PENDING/PICKING/CANCELLED) and the
    // richer, derived fulfillment_status (adds PICKING_CANCELLED/
    // PICKED/SHIPPING/SHIP_ISSUE/DELIVERED) — see backend internal/
    // order's dto.go FulfillmentStatus. Called from the template, not
    // cached at map time, so the label stays reactive to the locale.
    statusLabel(status) {
      switch (status) {
        case 'PENDING': return this.$t('orders.statusLabels.pending')
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

    openNewOrder() {
      this.formMode = 'create'
      this.editingOrderId = null
      this.formError = ''
      this.form = { warehouseId: '', customer: '', externalReference: '', remark: '', items: [] }
      this.itemDraft = { sku: '', orderedQty: null }
      this.showForm = true
    },

    openEditOrder() {
      this.formMode = 'edit'
      this.editingOrderId = this.selectedOrder.id
      this.formError = ''
      this.form = {
        warehouseId: this.selectedOrder.warehouse_id,
        customer: this.selectedOrder.customer || '',
        externalReference: this.selectedOrder.external_reference || '',
        remark: this.selectedOrder.remark || '',
        items: this.selectedOrder.items.map((it) => ({
          productId: it.product_id, sku: it.sku, productName: it.product_name,
          orderedQty: it.ordered_qty, uom: it.uom
        }))
      }
      this.itemDraft = { sku: '', orderedQty: null }
      this.showForm = true
    },

    async addItem() {
      const sku = (this.itemDraft.sku || '').trim()
      if (!sku || !this.itemDraft.orderedQty) {
        this.formError = this.$t('orders.messages.skuQtyRequired')
        return
      }
      try {
        const res = await listProducts({ search: sku, limit: 20 })
        const product = res.data.find((p) => p.sku === sku || p.barcode === sku)
        if (!product) {
          this.formError = this.$t('orders.messages.noProductFound', { value: sku })
          return
        }
        this.form.items.push({
          productId: product.id, sku: product.sku, productName: product.name,
          orderedQty: this.itemDraft.orderedQty, uom: product.base_uom || ''
        })
        this.itemDraft = { sku: '', orderedQty: null }
        this.formError = ''
      } catch (err) {
        this.formError = err.message || this.$t('orders.messages.lookupProductFailed')
      }
    },

    async submitForm() {
      this.formError = ''
      if (!this.form.warehouseId) {
        this.formError = this.$t('orders.messages.warehouseRequired')
        return
      }
      if (this.form.items.length === 0) {
        this.formError = this.$t('orders.messages.atLeastOneItem')
        return
      }

      const payload = {
        warehouse_id: this.form.warehouseId,
        customer: this.form.customer || undefined,
        external_reference: this.form.externalReference || undefined,
        remark: this.form.remark || undefined,
        items: this.form.items.map((it) => ({
          product_id: it.productId, ordered_qty: it.orderedQty, uom: it.uom || undefined
        }))
      }

      this.submitting = true
      try {
        if (this.formMode === 'edit') {
          await updateOrder(this.editingOrderId, payload)
        } else {
          await createOrder(payload)
        }
        this.showForm = false
        await this.fetchOrders()
        if (this.formMode === 'edit' && this.selectedOrder) {
          await this.openDetail(this.editingOrderId)
        }
      } catch (err) {
        this.formError = err.message || this.$t('orders.messages.saveOrderFailed')
      } finally {
        this.submitting = false
      }
    },

    async openDetail(id) {
      this.detailError = ''
      this.cancelRemark = ''
      try {
        const res = await getOrder(id)
        this.selectedOrder = res.data
      } catch (err) {
        this.loadError = err.message || this.$t('orders.messages.loadOrderDetailFailed')
      }
    },

    closeDetail() {
      this.selectedOrder = null
    },

    async doCancel() {
      this.actionBusy = true
      this.detailError = ''
      try {
        const res = await cancelOrder(this.selectedOrder.id, { remark: this.cancelRemark })
        this.selectedOrder = res.data
        await this.fetchOrders()
      } catch (err) {
        this.detailError = err.message
      } finally {
        this.actionBusy = false
      }
    },

    async openReleasePanel() {
      this.releaseError = ''
      this.releaseForm = {
        priority: 'NORMAL', dueDate: '',
        assignments: this.selectedOrder.items.map((it) => ({
          orderItemId: it.id, sku: it.sku, productName: it.product_name, orderedQty: it.ordered_qty,
          locationId: '', lotNo: ''
        }))
      }
      this.releaseLocations = []
      try {
        const zonesRes = await listZones(this.selectedOrder.warehouse_id)
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

    async submitRelease() {
      this.releaseError = ''
      for (const a of this.releaseForm.assignments) {
        if (!a.locationId) {
          this.releaseError = this.$t('orders.messages.lineNeedsLocation')
          return
        }
      }

      this.releaseBusy = true
      try {
        const res = await createOrderPickingTask(this.selectedOrder.id, {
          priority: this.releaseForm.priority,
          due_date: this.releaseForm.dueDate ? new Date(this.releaseForm.dueDate).toISOString() : undefined,
          items: this.releaseForm.assignments.map((a) => ({
            order_item_id: a.orderItemId, location_id: a.locationId, lot_no: a.lotNo || undefined
          }))
        })
        this.selectedOrder = res.data
        this.showReleasePanel = false
        await this.fetchOrders()
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
