<template>
  <div class="space-y-6">
    <div class="flex flex-col xl:flex-row xl:items-center xl:justify-between gap-4">
      <div>
        <h1 class="text-2xl font-bold text-slate-800">Orders</h1>
        <p class="text-slate-500">
          Order intake — orchestrates Picking once locations/lots are assigned
        </p>
      </div>

      <div class="flex gap-3">
        <button
          v-if="canCreate"
          @click="openNewOrder"
          class="bg-orange-600 hover:bg-orange-700 text-white px-5 py-3 rounded-xl font-semibold"
        >
          + New Order
        </button>
      </div>
    </div>

    <div v-if="loadError" class="bg-red-50 border border-red-200 text-red-600 rounded-xl p-3 text-sm">
      {{ loadError }}
    </div>

    <div class="bg-white rounded-2xl border border-slate-200 p-5">
      <div class="grid grid-cols-1 md:grid-cols-4 gap-4">
        <input v-model="filters.search" type="text" placeholder="Search Order No, External Ref, Customer..." class="input" />

        <select v-model="filters.status" class="input">
          <option value="">All Status</option>
          <option value="PENDING">Pending</option>
          <option value="PICKING">Picking</option>
          <option value="CANCELLED">Cancelled</option>
        </select>

        <select v-model="filters.warehouseId" class="input">
          <option value="">All Warehouses</option>
          <option v-for="w in warehouses" :key="w.id" :value="w.id">{{ w.code }}</option>
        </select>

        <button @click="clearFilter" class="bg-slate-100 hover:bg-slate-200 text-slate-700 rounded-xl font-semibold">
          Clear Filter
        </button>
      </div>
    </div>

    <div class="bg-white rounded-2xl border border-slate-200 shadow-sm overflow-hidden">
      <div class="w-full overflow-x-auto">
        <table class="min-w-[1000px] w-full text-sm">
          <thead class="bg-slate-50 text-slate-500">
            <tr>
              <th class="table-th">Order No.</th>
              <th class="table-th">External Ref.</th>
              <th class="table-th">Customer</th>
              <th class="table-th">Warehouse</th>
              <th class="table-th">Items</th>
              <th class="table-th">Status</th>
              <th class="table-th">Fulfillment</th>
              <th class="table-th">Created At</th>
              <th class="table-th text-right">Action</th>
            </tr>
          </thead>
          <tbody>
            <tr v-if="loading">
              <td colspan="9" class="text-center py-10 text-slate-400">Loading orders…</td>
            </tr>
            <tr v-else-if="orderList.length === 0">
              <td colspan="9" class="text-center py-10 text-slate-400">No orders found</td>
            </tr>
            <template v-else>
              <tr v-for="o in orderList" :key="o.id" class="border-t hover:bg-orange-50">
                <td class="table-td font-semibold text-slate-800">{{ o.order_no }}</td>
                <td class="table-td">{{ o.external_reference || '—' }}</td>
                <td class="table-td">{{ o.customer || '—' }}</td>
                <td class="table-td">{{ o.warehouse_code }}</td>
                <td class="table-td">{{ o.item_count }}</td>
                <td class="table-td"><span :class="statusBadgeClass(o.status)">{{ o.status }}</span></td>
                <td class="table-td"><span :class="statusBadgeClass(o.fulfillment_status)">{{ o.fulfillment_status }}</span></td>
                <td class="table-td">{{ formatDate(o.created_at) }}</td>
                <td class="table-td text-right">
                  <button @click="openDetail(o.id)" class="text-orange-600 hover:underline font-semibold">View</button>
                </td>
              </tr>
            </template>
          </tbody>
        </table>
      </div>
    </div>

    <!-- New/Edit Order panel -->
    <div v-if="showForm" class="fixed inset-0 bg-black/30 flex items-center justify-center z-50 p-4">
      <div class="bg-white rounded-2xl shadow-xl w-full max-w-2xl p-6 space-y-4 max-h-[90vh] overflow-y-auto">
        <div class="flex items-center justify-between">
          <h2 class="text-lg font-bold text-slate-800">{{ formMode === 'edit' ? 'Edit Order' : 'New Order' }}</h2>
          <button @click="showForm = false" class="text-slate-400 hover:text-slate-600">✕</button>
        </div>

        <p v-if="formError" class="bg-red-50 border border-red-200 text-red-600 rounded-xl p-3 text-sm">{{ formError }}</p>

        <div class="grid grid-cols-1 md:grid-cols-2 gap-3">
          <select v-model="form.warehouseId" class="input">
            <option value="">Select Warehouse</option>
            <option v-for="w in warehouses" :key="w.id" :value="w.id">{{ w.code }} — {{ w.name }}</option>
          </select>

          <input v-model="form.customer" type="text" placeholder="Customer (optional)" class="input" />

          <input v-model="form.externalReference" type="text" placeholder="External Reference (optional)" class="input" />

          <textarea v-model="form.remark" placeholder="Remark (optional)" class="input md:col-span-2" rows="2"></textarea>
        </div>

        <div class="border-t pt-3 space-y-3">
          <h3 class="font-semibold text-slate-700">Items</h3>

          <div class="grid grid-cols-1 md:grid-cols-4 gap-2">
            <input v-model="itemDraft.sku" type="text" placeholder="Product SKU" class="input md:col-span-2" />
            <input v-model.number="itemDraft.orderedQty" type="number" min="0" step="0.01" placeholder="Qty" class="input" />
            <button @click="addItem" class="px-3 py-2 rounded-xl bg-slate-100 hover:bg-slate-200 text-slate-700 font-semibold">
              Add Item
            </button>
          </div>

          <table v-if="form.items.length" class="min-w-full text-sm">
            <thead class="bg-slate-50 text-slate-500">
              <tr>
                <th class="table-th">SKU</th>
                <th class="table-th">Product</th>
                <th class="table-th">Qty</th>
                <th class="table-th"></th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="(it, idx) in form.items" :key="idx" class="border-t">
                <td class="table-td">{{ it.sku }}</td>
                <td class="table-td">{{ it.productName }}</td>
                <td class="table-td">{{ it.orderedQty }}</td>
                <td class="table-td text-right">
                  <button @click="form.items.splice(idx, 1)" class="text-red-500 hover:underline">Remove</button>
                </td>
              </tr>
            </tbody>
          </table>
          <p v-else class="text-sm text-slate-400">No items added yet</p>
        </div>

        <div class="flex justify-end gap-3">
          <button @click="showForm = false" class="px-4 py-2 rounded-xl bg-slate-100 text-slate-700 font-semibold">Cancel</button>
          <button @click="submitForm" :disabled="submitting" class="px-4 py-2 rounded-xl bg-orange-600 hover:bg-orange-700 text-white font-semibold disabled:opacity-60">
            {{ submitting ? 'Saving…' : (formMode === 'edit' ? 'Save Changes' : 'Create Order') }}
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
          <p><span class="text-slate-500">Status:</span> <span :class="statusBadgeClass(selectedOrder.status)">{{ selectedOrder.status }}</span></p>
          <p><span class="text-slate-500">Fulfillment:</span> <span :class="statusBadgeClass(selectedOrder.fulfillment_status)">{{ selectedOrder.fulfillment_status }}</span></p>
          <p><span class="text-slate-500">Customer:</span> {{ selectedOrder.customer || '—' }}</p>
          <p><span class="text-slate-500">External Ref:</span> {{ selectedOrder.external_reference || '—' }}</p>
          <p><span class="text-slate-500">Warehouse:</span> {{ selectedOrder.warehouse_code }}</p>
          <p v-if="selectedOrder.pick_no"><span class="text-slate-500">Picking Task:</span> {{ selectedOrder.pick_no }}</p>
          <p v-if="selectedOrder.remark"><span class="text-slate-500">Remark:</span> {{ selectedOrder.remark }}</p>
          <p v-if="selectedOrder.cancel_remark" class="text-slate-600"><span class="text-slate-500">Cancel Remark:</span> {{ selectedOrder.cancel_remark }}</p>
          <p><span class="text-slate-500">Created By:</span> {{ selectedOrder.created_by_name || '—' }}</p>
        </div>

        <div class="border-t pt-3">
          <h3 class="font-semibold text-slate-700 mb-2">Items</h3>
          <table class="min-w-full text-sm">
            <thead class="bg-slate-50 text-slate-500">
              <tr>
                <th class="table-th">SKU</th>
                <th class="table-th">Product</th>
                <th class="table-th">Qty</th>
                <th class="table-th">UOM</th>
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
            Edit
          </button>
          <button @click="openReleasePanel" class="px-4 py-2 rounded-xl bg-orange-600 hover:bg-orange-700 text-white font-semibold">
            Create Picking Task
          </button>
        </div>

        <div v-if="canApprove && selectedOrder.status === 'PENDING'" class="space-y-2 pt-2 border-t">
          <textarea v-model="cancelRemark" placeholder="Cancel remark (optional)" class="input" rows="2"></textarea>
          <button @click="doCancel" :disabled="actionBusy" class="w-full px-4 py-2 rounded-xl bg-slate-100 text-slate-700 font-semibold disabled:opacity-60">
            Cancel Order
          </button>
        </div>

        <p v-if="selectedOrder.status === 'PICKING'" class="text-xs text-slate-400 pt-2 border-t">
          A picking task has already been created for this order — cancel it directly from Picking if needed.
        </p>
      </div>
    </div>

    <!-- Create Picking Task (release) panel -->
    <div v-if="showReleasePanel" class="fixed inset-0 bg-black/30 flex items-center justify-center z-50 p-4">
      <div class="bg-white rounded-2xl shadow-xl w-full max-w-2xl p-6 space-y-4 max-h-[90vh] overflow-y-auto">
        <div class="flex items-center justify-between">
          <h2 class="text-lg font-bold text-slate-800">Create Picking Task — {{ selectedOrder.order_no }}</h2>
          <button @click="showReleasePanel = false" class="text-slate-400 hover:text-slate-600">✕</button>
        </div>

        <p v-if="releaseError" class="bg-red-50 border border-red-200 text-red-600 rounded-xl p-3 text-sm">{{ releaseError }}</p>

        <div class="grid grid-cols-1 md:grid-cols-2 gap-3">
          <select v-model="releaseForm.priority" class="input">
            <option value="NORMAL">Priority: Normal</option>
            <option value="LOW">Priority: Low</option>
            <option value="HIGH">Priority: High</option>
          </select>
          <input v-model="releaseForm.dueDate" type="date" class="input" />
        </div>

        <div class="space-y-3">
          <h3 class="font-semibold text-slate-700">Assign a location/lot to each line</h3>
          <div v-for="a in releaseForm.assignments" :key="a.orderItemId" class="grid grid-cols-1 md:grid-cols-3 gap-2 items-center border-b pb-2">
            <div class="text-sm text-slate-600 md:col-span-1">{{ a.sku }} — {{ a.productName }} ({{ a.orderedQty }})</div>
            <select v-model="a.locationId" class="input">
              <option value="">Select Location</option>
              <option v-for="l in releaseLocations" :key="l.id" :value="l.id">{{ l.code }}</option>
            </select>
            <input v-model="a.lotNo" type="text" placeholder="Lot No (optional)" class="input" />
          </div>
        </div>

        <div class="flex justify-end gap-3">
          <button @click="showReleasePanel = false" class="px-4 py-2 rounded-xl bg-slate-100 text-slate-700 font-semibold">Cancel</button>
          <button @click="submitRelease" :disabled="releaseBusy" class="px-4 py-2 rounded-xl bg-orange-600 hover:bg-orange-700 text-white font-semibold disabled:opacity-60">
            {{ releaseBusy ? 'Creating…' : 'Create Picking Task' }}
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

export default {
  name: 'OrderPage',
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
        this.loadError = err.message || 'Failed to load warehouses'
      }
    },

    async fetchOrders() {
      this.loading = true
      this.loadError = ''
      try {
        const res = await listOrders(this.filters)
        this.orderList = res.data
      } catch (err) {
        this.loadError = err.message || 'Failed to load orders'
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
      return new Date(value).toLocaleString()
    },

    statusBadgeClass(status) {
      if (status === 'DELIVERED' || status === 'PICKED' || status === 'COMPLETED') return 'badge-green'
      if (status === 'PICKING' || status === 'SHIPPING') return 'badge-orange'
      if (status === 'SHIP_ISSUE' || status === 'PICKING_CANCELLED' || status === 'CANCELLED') return 'badge-gray'
      return 'badge-gray'
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
        this.formError = 'SKU and quantity are required to add an item'
        return
      }
      try {
        const res = await listProducts({ search: sku, limit: 20 })
        const product = res.data.find((p) => p.sku === sku || p.barcode === sku)
        if (!product) {
          this.formError = `No product found for SKU/barcode "${sku}"`
          return
        }
        this.form.items.push({
          productId: product.id, sku: product.sku, productName: product.name,
          orderedQty: this.itemDraft.orderedQty, uom: product.base_uom || ''
        })
        this.itemDraft = { sku: '', orderedQty: null }
        this.formError = ''
      } catch (err) {
        this.formError = err.message || 'Failed to look up product'
      }
    },

    async submitForm() {
      this.formError = ''
      if (!this.form.warehouseId) {
        this.formError = 'Warehouse is required'
        return
      }
      if (this.form.items.length === 0) {
        this.formError = 'At least one item is required'
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
        this.formError = err.message || 'Failed to save order'
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
        this.loadError = err.message || 'Failed to load order detail'
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
        this.releaseError = err.message || 'Failed to load locations'
      }
      this.showReleasePanel = true
    },

    async submitRelease() {
      this.releaseError = ''
      for (const a of this.releaseForm.assignments) {
        if (!a.locationId) {
          this.releaseError = 'Every line needs a location assigned'
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
        this.releaseError = err.message || 'Failed to create picking task'
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
</style>
