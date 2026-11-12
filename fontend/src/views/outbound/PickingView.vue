<template>
  <div class="space-y-6">
    <div class="flex flex-col xl:flex-row xl:items-center xl:justify-between gap-4">
      <div>
        <h1 class="text-2xl font-bold text-slate-800">Picking</h1>
        <p class="text-slate-500">
          Manage outbound picking tasks and confirm picked items
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
          @click="openPicking(null)"
          class="bg-orange-600 hover:bg-orange-700 text-white px-5 py-3 rounded-xl font-semibold"
        >
          + New Picking Task
        </button>
      </div>
    </div>

    <div class="grid grid-cols-1 md:grid-cols-4 gap-5">
      <div class="summary-card">
        <p class="summary-label">Pending Pick</p>
        <h2 class="summary-value">{{ pendingCount }}</h2>
      </div>

      <div class="summary-card">
        <p class="summary-label">In Progress</p>
        <h2 class="summary-value">{{ inProgressCount }}</h2>
      </div>

      <div class="summary-card">
        <p class="summary-label">Completed</p>
        <h2 class="summary-value">{{ completedCount }}</h2>
      </div>

      <div class="summary-card">
        <p class="summary-label">Short Pick</p>
        <h2 class="summary-value text-red-600">{{ shortPickCount }}</h2>
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
          placeholder="Search Pick No, Order No, SKU..."
          class="input"
        />

        <select v-model="filters.status" class="input">
          <option value="">All Status</option>
          <option value="PENDING">Pending</option>
          <option value="IN_PROGRESS">In Progress</option>
          <option value="COMPLETED">Completed</option>
          <option value="SHORT_PICK">Short Pick</option>
          <option value="CANCELLED">Cancelled</option>
        </select>

        <select v-model="filters.priority" class="input">
          <option value="">All Priority</option>
          <option value="HIGH">High</option>
          <option value="NORMAL">Normal</option>
          <option value="LOW">Low</option>
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
              <th class="table-th">Pick No.</th>
              <th class="table-th">Order No.</th>
              <th class="table-th">Customer</th>
              <th class="table-th">Warehouse</th>
              <th class="table-th">Priority</th>
              <th class="table-th">Items</th>
              <th class="table-th">Progress</th>
              <th class="table-th">Due Date</th>
              <th class="table-th">Status</th>
              <th class="table-th text-right">Action</th>
            </tr>
          </thead>

          <tbody>
            <tr v-if="loading">
              <td colspan="10" class="text-center py-10 text-slate-400">Loading picking tasks…</td>
            </tr>

            <tr v-else-if="pickingList.length === 0">
              <td colspan="10" class="text-center py-10 text-slate-400">No picking tasks found</td>
            </tr>

            <template v-else>
            <tr
              v-for="item in pickingList"
              :key="item.id"
              class="border-t hover:bg-orange-50"
            >
              <td class="table-td font-semibold text-slate-800">
                {{ item.pickNo }}
              </td>
              <td class="table-td">{{ item.orderNo }}</td>
              <td class="table-td">{{ item.customer }}</td>
              <td class="table-td">{{ item.warehouse }}</td>

              <td class="table-td">
                <span :class="getPriorityClass(item.priority)">
                  {{ item.priority }}
                </span>
              </td>

              <td class="table-td">{{ item.items }}</td>

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

              <td class="table-td">{{ item.dueDate || '—' }}</td>

              <td class="table-td">
                <span :class="getStatusClass(item.status)">
                  {{ item.statusText }}
                </span>
              </td>

              <td class="table-td text-right">
                <button
                  @click="openPicking(item)"
                  class="text-orange-600 hover:text-orange-700 font-semibold"
                >
                  Start Pick
                </button>
              </td>
            </tr>
            </template>
          </tbody>
        </table>
      </div>
    </div>

    <div
      v-if="selectedPicking"
      class="fixed inset-0 bg-black/40 flex justify-end z-50"
    >
      <div class="w-full max-w-3xl bg-white h-full p-6 overflow-y-auto">
        <div class="flex items-center justify-between mb-6">
          <div>
            <h2 class="text-xl font-bold text-slate-800">
              {{ isNewTask ? 'New Picking Task' : 'Picking Task' }}
            </h2>
            <p v-if="!isNewTask" class="text-slate-500">
              {{ selectedPicking.pickNo }} / {{ selectedPicking.orderNo }}
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

        <!-- New task header form -->
        <div v-if="isNewTask" class="bg-slate-50 rounded-2xl p-5 mb-6 space-y-4">
          <div class="grid grid-cols-2 gap-4">
            <div>
              <label class="label">Order No.</label>
              <input v-model="newTaskForm.orderNo" type="text" class="input" placeholder="SO-20260608-005" />
            </div>
            <div>
              <label class="label">Customer</label>
              <input v-model="newTaskForm.customer" type="text" class="input" placeholder="Customer name" />
            </div>
            <div>
              <label class="label">Warehouse</label>
              <select v-model="newTaskForm.warehouseId" class="input" @change="onWarehouseChange">
                <option value="">Select warehouse</option>
                <option v-for="w in warehouses" :key="w.id" :value="w.id">{{ w.code }} — {{ w.name }}</option>
              </select>
            </div>
            <div>
              <label class="label">Priority</label>
              <select v-model="newTaskForm.priority" class="input">
                <option value="LOW">Low</option>
                <option value="NORMAL">Normal</option>
                <option value="HIGH">High</option>
              </select>
            </div>
            <div>
              <label class="label">Due Date</label>
              <input v-model="newTaskForm.dueDate" type="date" class="input" />
            </div>
          </div>
        </div>

        <!-- Existing task summary -->
        <div v-else class="bg-slate-50 rounded-2xl p-5 mb-6">
          <div class="grid grid-cols-2 md:grid-cols-4 gap-4">
            <div>
              <p class="detail-label">Customer</p>
              <p class="detail-value">{{ selectedPicking.customer }}</p>
            </div>

            <div>
              <p class="detail-label">Warehouse</p>
              <p class="detail-value">{{ selectedPicking.warehouse }}</p>
            </div>

            <div>
              <p class="detail-label">Priority</p>
              <p class="detail-value text-orange-600">
                {{ selectedPicking.priority }}
              </p>
            </div>

            <div>
              <p class="detail-label">Due Date</p>
              <p class="detail-value">{{ selectedPicking.dueDate || '—' }}</p>
            </div>
          </div>
        </div>

        <!-- Cancellation details (Phase 18) -->
        <div v-if="!isNewTask && selectedPicking.status === 'CANCELLED'" class="bg-red-50 border border-red-200 rounded-2xl p-5 mb-6">
          <div class="grid grid-cols-2 md:grid-cols-3 gap-4">
            <div>
              <p class="detail-label">Cancelled At</p>
              <p class="detail-value">{{ formatDateTime(selectedPicking.cancelled_at) }}</p>
            </div>
            <div>
              <p class="detail-label">Cancelled By</p>
              <p class="detail-value">{{ selectedPicking.cancelled_by_name || '—' }}</p>
            </div>
            <div>
              <p class="detail-label">Cancellation Remark</p>
              <p class="detail-value">{{ selectedPicking.cancel_remark || '—' }}</p>
            </div>
          </div>
        </div>

        <!-- Add item form (new task only) -->
        <div v-if="isNewTask" class="bg-white border border-slate-200 rounded-2xl p-4 mb-6 space-y-3">
          <h3 class="font-bold text-slate-800 text-sm">Add Item</h3>
          <div class="grid grid-cols-2 md:grid-cols-4 gap-3">
            <input v-model="addItemForm.sku" type="text" class="input" placeholder="SKU / Barcode" />
            <select v-model="addItemForm.locationId" class="input" :disabled="!newTaskForm.warehouseId">
              <option value="">Location</option>
              <option v-for="l in drawerLocations" :key="l.id" :value="l.id">{{ l.code }}</option>
            </select>
            <input v-model="addItemForm.lotNo" type="text" class="input" placeholder="Lot No. (optional)" />
            <input v-model.number="addItemForm.orderedQty" type="number" class="input" placeholder="Order Qty" />
          </div>
          <button
            @click="addItem"
            class="bg-slate-800 hover:bg-slate-900 text-white px-4 py-2 rounded-xl font-semibold text-sm"
          >
            + Add Item
          </button>
        </div>

        <div class="space-y-4">
          <h3 class="font-bold text-slate-800">
            Picking Items
          </h3>

          <p v-if="displayItems.length === 0" class="text-sm text-slate-400">
            No items added yet.
          </p>

          <div
            v-for="product in displayItems"
            :key="product.key"
            class="border border-slate-200 rounded-2xl p-4"
          >
            <div class="flex flex-col md:flex-row md:items-start md:justify-between gap-4">
              <div>
                <p class="font-bold text-slate-800">
                  {{ product.sku }}
                </p>
                <p class="text-sm text-slate-500">
                  {{ product.productName }}
                </p>
                <p class="text-sm text-orange-600 font-semibold mt-2">
                  Location: {{ product.location }}
                </p>
                <p v-if="!isNewTask" class="text-xs mt-1">
                  <span :class="getItemStatusClass(product.status)">{{ product.statusText }}</span>
                </p>
              </div>

              <div class="grid grid-cols-3 gap-3 min-w-[280px]">
                <div>
                  <p class="detail-label">Order</p>
                  <p class="detail-value">{{ product.orderQty }}</p>
                </div>

                <div>
                  <p class="detail-label">Picked</p>
                  <p class="detail-value text-green-600">
                    {{ product.pickedQty }}
                  </p>
                </div>

                <div>
                  <p class="detail-label">Remain</p>
                  <p class="detail-value text-red-600">
                    {{ product.orderQty - product.pickedQty }}
                  </p>
                </div>
              </div>
            </div>

            <!-- New task: just a remove control -->
            <div v-if="isNewTask" class="mt-3 text-right">
              <button @click="removeNewItem(product.key)" class="text-red-600 hover:text-red-700 text-sm font-semibold">
                Remove
              </button>
            </div>

            <!-- Existing task, resolved line -->
            <p v-else-if="product.status === 'COMPLETED' || product.status === 'SHORT'" class="text-sm text-green-600 font-semibold mt-3">
              This line has already been resolved ({{ product.statusText }}).
            </p>

            <!-- Existing task, workable line -->
            <template v-else-if="!isNewTask">
              <div v-if="canConfirm && isTaskWorkable" class="grid grid-cols-1 md:grid-cols-3 gap-3 mt-4">
                <input
                  v-model.number="product.inputQty"
                  type="number"
                  class="input"
                  placeholder="Pick Qty"
                />

                <button
                  @click="confirmPick(product)"
                  :disabled="submitting"
                  class="bg-orange-600 hover:bg-orange-700 text-white rounded-xl font-semibold disabled:opacity-60"
                >
                  Confirm Pick
                </button>

                <button
                  @click="shortPick(product)"
                  :disabled="submitting"
                  class="bg-red-600 hover:bg-red-700 text-white rounded-xl font-semibold disabled:opacity-60"
                >
                  Short Pick
                </button>
              </div>
              <div v-if="canConfirm && isTaskWorkable" class="mt-3">
                <input
                  v-model="product.remark"
                  type="text"
                  class="input"
                  placeholder="Remark (required for Short Pick)"
                />
              </div>
              <p v-else-if="!canConfirm" class="text-sm text-slate-500 mt-3">
                Your role can view this task but not confirm picking.
              </p>
              <p v-else-if="selectedPicking.status === 'CANCELLED'" class="text-sm text-red-600 font-semibold mt-3">
                This task has been cancelled.
              </p>
            </template>
          </div>
        </div>

        <button
          v-if="isNewTask"
          @click="saveTask"
          :disabled="submitting"
          class="w-full bg-orange-600 hover:bg-orange-700 text-white py-3 rounded-xl font-semibold mt-6 disabled:opacity-60"
        >
          {{ submitting ? 'Saving…' : 'Save Picking Task' }}
        </button>
        <div v-else>
          <div v-if="canCancel && isTaskWorkable" class="mt-6">
            <input
              v-model="cancelRemark"
              type="text"
              class="input"
              placeholder="Cancellation remark (optional)"
            />
          </div>
          <div class="flex gap-3 mt-3">
            <button
              v-if="canCancel && isTaskWorkable"
              @click="cancelTask"
              :disabled="submitting"
              class="flex-1 bg-red-600 hover:bg-red-700 text-white py-3 rounded-xl font-semibold disabled:opacity-60"
            >
              Cancel Task
            </button>
            <button
              @click="closeDrawer"
              class="flex-1 bg-green-600 hover:bg-green-700 text-white py-3 rounded-xl font-semibold"
            >
              Close
            </button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script>
import { listPicking, getPickingTask, createPickingTask, confirmPickingTask, cancelPickingTask, newIdempotencyKey, exportPicking } from '../../services/picking'
import { listWarehouses, listZones, listLocations } from '../../services/warehouses'
import { listProducts } from '../../services/products'
import { hasPermission } from '../../services/auth'

let searchDebounce = null

function toApiDate(dateStr) {
  return dateStr ? `${dateStr}T00:00:00Z` : null
}

function statusText(status) {
  switch (status) {
    case 'PENDING': return 'Pending'
    case 'IN_PROGRESS': return 'In Progress'
    case 'COMPLETED': return 'Completed'
    case 'SHORT_PICK': return 'Short Pick'
    case 'CANCELLED': return 'Cancelled'
    default: return status
  }
}

function itemStatusText(status) {
  switch (status) {
    case 'PENDING': return 'Pending'
    case 'PARTIALLY_PICKED': return 'Partially Picked'
    case 'COMPLETED': return 'Completed'
    case 'SHORT': return 'Short'
    default: return status
  }
}

// Real backend field names mapped onto the mock's existing display field
// names so the template needs minimal changes.
function mapPickingRow(r) {
  return {
    id: r.id,
    pickNo: r.pick_no,
    orderNo: r.order_no,
    customer: r.customer || '—',
    warehouse: r.warehouse_code,
    priority: r.priority,
    items: r.item_count,
    progress: Math.round(r.progress_pct),
    dueDate: r.due_date ? r.due_date.slice(0, 10) : '',
    status: r.status,
    statusText: statusText(r.status)
  }
}

export default {
  name: 'PickingView',

  data() {
    return {
      loading: false,
      loadError: '',
      exporting: false,
      pickingList: [],
      allTasks: [],
      warehouses: [],

      selectedPicking: null,
      isNewTask: false,
      drawerError: '',
      drawerLocations: [],
      submitting: false,
      cancelRemark: '',

      newTaskForm: {
        orderNo: '',
        customer: '',
        warehouseId: '',
        priority: 'NORMAL',
        dueDate: ''
      },
      addItemForm: {
        sku: '',
        locationId: '',
        lotNo: '',
        orderedQty: null
      },
      newItems: [],

      filters: {
        keyword: '',
        status: '',
        priority: '',
        date: ''
      }
    }
  },

  computed: {
    pendingCount() {
      return this.allTasks.filter((t) => t.status === 'PENDING').length
    },
    inProgressCount() {
      return this.allTasks.filter((t) => t.status === 'IN_PROGRESS').length
    },
    completedCount() {
      return this.allTasks.filter((t) => t.status === 'COMPLETED').length
    },
    shortPickCount() {
      return this.allTasks.filter((t) => t.status === 'SHORT_PICK').length
    },
    canCreate() {
      return hasPermission('picking.create')
    },
    // Confirm (per-line pick/short-pick) is gated on picking.update on
    // the backend, not picking.approve — see internal/picking/handler.go's
    // doc: the PICKING role's own Phase 2 grants only ever included
    // view/update, never create/approve.
    canConfirm() {
      return hasPermission('picking.update')
    },
    // Cancel is gated on picking.approve, not picking.update — see the
    // backend's own internal/picking/handler.go doc: cancelling ends the
    // whole task and releases its reservation, a more consequential,
    // planning-level action than confirming one line, so it sits at the
    // same authority level as Create rather than picking staff's own
    // execution-level permission.
    canCancel() {
      return hasPermission('picking.approve')
    },
    // Pick/Short-pick lines only make sense while the task itself is
    // still workable — once it's been Cancelled (Phase 13) the backend
    // rejects Confirm outright (409), so hide the controls here too
    // rather than let a user hit that error needlessly.
    isTaskWorkable() {
      return !this.isNewTask && this.selectedPicking &&
        (this.selectedPicking.status === 'PENDING' || this.selectedPicking.status === 'IN_PROGRESS')
    },
    displayItems() {
      if (this.isNewTask) {
        return this.newItems.map((it, idx) => ({
          key: `new-${idx}`,
          sku: it.sku,
          productName: it.productName,
          location: it.locationCode,
          orderQty: it.orderedQty,
          pickedQty: 0
        }))
      }
      if (!this.selectedPicking) return []
      return (this.selectedPicking.items || []).map((it) => ({
        key: it.id,
        id: it.id,
        sku: it.sku,
        productName: it.product_name,
        location: it.location_code,
        orderQty: it.ordered_qty,
        pickedQty: it.picked_qty,
        status: it.status,
        statusText: itemStatusText(it.status),
        inputQty: it.inputQty || 0,
        remark: it.remark || ''
      }))
    }
  },

  watch: {
    'filters.keyword'() {
      clearTimeout(searchDebounce)
      searchDebounce = setTimeout(() => this.fetchPicking(), 300)
    },
    'filters.status'() {
      this.fetchPicking()
    },
    'filters.priority'() {
      this.fetchPicking()
    },
    'filters.date'() {
      this.fetchPicking()
    }
  },

  mounted() {
    this.fetchPicking()
    this.fetchSummary()
  },

  methods: {
    async fetchPicking() {
      this.loading = true
      this.loadError = ''
      try {
        const res = await listPicking({
          search: this.filters.keyword,
          status: this.filters.status,
          priority: this.filters.priority,
          date: this.filters.date,
          limit: 100
        })
        this.pickingList = res.data.map(mapPickingRow)
      } catch (err) {
        this.loadError = err.message
      } finally {
        this.loading = false
      }
    },

    // A separate, unfiltered fetch backing the summary cards — no
    // dedicated stats endpoint this phase, the same approach
    // ReceiveView.vue/PutawayView.vue use.
    async fetchSummary() {
      try {
        const res = await listPicking({ limit: 200 })
        this.allTasks = res.data
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
      this.addItemForm.locationId = ''
      this.drawerLocations = []
      if (!this.newTaskForm.warehouseId) return
      try {
        const zonesRes = await listZones(this.newTaskForm.warehouseId)
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
      this.filters.keyword = ''
      this.filters.status = ''
      this.filters.priority = ''
      this.filters.date = ''
      this.fetchPicking()
    },

    async exportCsv() {
      this.exporting = true
      try {
        await exportPicking({
          search: this.filters.keyword, status: this.filters.status,
          priority: this.filters.priority, date: this.filters.date
        })
      } catch (err) {
        this.loadError = err.message
      } finally {
        this.exporting = false
      }
    },

    async openPicking(item) {
      this.drawerError = ''
      this.submitting = false
      this.cancelRemark = ''

      if (!item) {
        this.isNewTask = true
        this.selectedPicking = {}
        this.newTaskForm = { orderNo: '', customer: '', warehouseId: '', priority: 'NORMAL', dueDate: '' }
        this.addItemForm = { sku: '', locationId: '', lotNo: '', orderedQty: null }
        this.newItems = []
        this.drawerLocations = []
        await this.fetchWarehouses()
        return
      }

      this.isNewTask = false
      try {
        const res = await getPickingTask(item.id)
        this.selectedPicking = {
          ...res.data,
          warehouse: res.data.warehouse_code,
          dueDate: res.data.due_date ? res.data.due_date.slice(0, 10) : ''
        }
      } catch (err) {
        this.drawerError = err.message
        this.selectedPicking = null
      }
    },

    closeDrawer() {
      this.selectedPicking = null
      this.isNewTask = false
    },

    async findProduct(text) {
      const trimmed = (text || '').trim()
      if (!trimmed) return null
      const res = await listProducts({ search: trimmed, limit: 20 })
      return res.data.find((p) => p.sku === trimmed || p.barcode === trimmed) || null
    },

    async addItem() {
      this.drawerError = ''
      if (!this.addItemForm.sku) {
        this.drawerError = 'SKU / Barcode is required'
        return
      }
      if (!this.addItemForm.locationId) {
        this.drawerError = 'Location is required'
        return
      }
      if (!this.addItemForm.orderedQty || this.addItemForm.orderedQty <= 0) {
        this.drawerError = 'Order Qty must be greater than zero'
        return
      }

      const product = await this.findProduct(this.addItemForm.sku)
      if (!product) {
        this.drawerError = `No product found for SKU/barcode "${this.addItemForm.sku}"`
        return
      }

      // Location IDs from the API are JSON-encoded as strings (see
      // backend `,string` int64 convention); compare as strings rather
      // than coercing with Number(), which silently fails to match here.
      const location = this.drawerLocations.find((l) => String(l.id) === String(this.addItemForm.locationId))

      this.newItems.push({
        productId: product.id,
        sku: product.sku,
        productName: product.name,
        uom: product.base_uom,
        lotNo: this.addItemForm.lotNo || '',
        locationId: this.addItemForm.locationId,
        locationCode: location ? location.code : '',
        orderedQty: Number(this.addItemForm.orderedQty)
      })

      this.addItemForm = { sku: '', locationId: this.addItemForm.locationId, lotNo: '', orderedQty: null }
    },

    removeNewItem(key) {
      const idx = Number(String(key).replace('new-', ''))
      this.newItems.splice(idx, 1)
    },

    async saveTask() {
      this.drawerError = ''
      if (!this.newTaskForm.orderNo) {
        this.drawerError = 'Order No. is required'
        return
      }
      if (!this.newTaskForm.warehouseId) {
        this.drawerError = 'Warehouse is required'
        return
      }
      if (this.newItems.length === 0) {
        this.drawerError = 'At least one item is required'
        return
      }

      this.submitting = true
      try {
        await createPickingTask({
          order_no: this.newTaskForm.orderNo,
          customer: this.newTaskForm.customer || null,
          warehouse_id: this.newTaskForm.warehouseId,
          priority: this.newTaskForm.priority,
          due_date: toApiDate(this.newTaskForm.dueDate),
          items: this.newItems.map((it) => ({
            product_id: it.productId,
            lot_no: it.lotNo,
            location_id: it.locationId,
            ordered_qty: it.orderedQty,
            uom: it.uom
          }))
        })
        alert('Picking task created successfully')
        this.closeDrawer()
        await this.fetchPicking()
        await this.fetchSummary()
      } catch (err) {
        this.drawerError = err.message
      } finally {
        this.submitting = false
      }
    },

    async confirmPick(product) {
      this.drawerError = ''
      if (!product.inputQty || product.inputQty <= 0) {
        this.drawerError = 'Pick Qty must be greater than zero'
        return
      }
      const remainQty = product.orderQty - product.pickedQty
      if (product.inputQty > remainQty) {
        this.drawerError = 'Pick Qty cannot exceed remaining quantity'
        return
      }

      await this.submitConfirm({
        item_id: product.id,
        action: 'PICKED',
        quantity: Number(product.inputQty)
      })
    },

    async shortPick(product) {
      this.drawerError = ''
      if (!product.remark) {
        this.drawerError = 'A remark is required to report a short pick'
        return
      }

      await this.submitConfirm({
        item_id: product.id,
        action: 'SHORT',
        remark: product.remark
      })
    },

    async submitConfirm(confirmItem) {
      this.submitting = true
      try {
        const res = await confirmPickingTask(this.selectedPicking.id, {
          idempotency_key: newIdempotencyKey(),
          items: [confirmItem]
        })
        this.selectedPicking = {
          ...res.data,
          warehouse: res.data.warehouse_code,
          dueDate: res.data.due_date ? res.data.due_date.slice(0, 10) : ''
        }
        await this.fetchPicking()
        await this.fetchSummary()
      } catch (err) {
        this.drawerError = err.message
      } finally {
        this.submitting = false
      }
    },

    async cancelTask() {
      this.drawerError = ''
      this.submitting = true
      try {
        const res = await cancelPickingTask(this.selectedPicking.id, {
          remark: this.cancelRemark || undefined
        })
        this.selectedPicking = {
          ...res.data,
          warehouse: res.data.warehouse_code,
          dueDate: res.data.due_date ? res.data.due_date.slice(0, 10) : ''
        }
        await this.fetchPicking()
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
      if (status === 'PENDING') return 'badge-gray'
      if (status === 'IN_PROGRESS') return 'badge-orange'
      if (status === 'COMPLETED') return 'badge-green'
      if (status === 'SHORT_PICK') return 'badge-red'
      if (status === 'CANCELLED') return 'badge-red'
      return 'badge-gray'
    },

    getItemStatusClass(status) {
      if (status === 'PENDING') return 'badge-gray'
      if (status === 'PARTIALLY_PICKED') return 'badge-orange'
      if (status === 'COMPLETED') return 'badge-green'
      if (status === 'SHORT') return 'badge-red'
      return 'badge-gray'
    },

    getPriorityClass(priority) {
      if (priority === 'HIGH') return 'badge-red'
      if (priority === 'NORMAL') return 'badge-blue'
      if (priority === 'LOW') return 'badge-gray'
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
