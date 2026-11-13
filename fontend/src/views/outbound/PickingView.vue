<template>
  <div class="space-y-6">
    <div class="flex flex-col xl:flex-row xl:items-center xl:justify-between gap-4">
      <div>
        <h1 class="text-2xl font-bold text-slate-800">{{ $t('picking.title') }}</h1>
        <p class="text-slate-500">
          {{ $t('picking.subtitle') }}
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
          @click="openPicking(null)"
          class="bg-orange-600 hover:bg-orange-700 text-white px-5 py-3 rounded-xl font-semibold"
        >
          {{ $t('picking.newTaskButton') }}
        </button>
      </div>
    </div>

    <div class="grid grid-cols-1 md:grid-cols-4 gap-5">
      <div class="summary-card">
        <p class="summary-label">{{ $t('picking.summary.pendingPick') }}</p>
        <h2 class="summary-value">{{ pendingCount }}</h2>
      </div>

      <div class="summary-card">
        <p class="summary-label">{{ $t('picking.summary.inProgress') }}</p>
        <h2 class="summary-value">{{ inProgressCount }}</h2>
      </div>

      <div class="summary-card">
        <p class="summary-label">{{ $t('picking.summary.completed') }}</p>
        <h2 class="summary-value">{{ completedCount }}</h2>
      </div>

      <div class="summary-card">
        <p class="summary-label">{{ $t('picking.summary.shortPick') }}</p>
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
          :placeholder="$t('picking.filters.searchPlaceholder')"
          class="input"
        />

        <select v-model="filters.status" class="input">
          <option value="">{{ $t('picking.filters.allStatus') }}</option>
          <option value="PENDING">{{ $t('common.status.pending') }}</option>
          <option value="IN_PROGRESS">{{ $t('common.status.inProgress') }}</option>
          <option value="COMPLETED">{{ $t('common.status.completed') }}</option>
          <option value="SHORT_PICK">{{ $t('picking.filters.statusShortPick') }}</option>
          <option value="CANCELLED">{{ $t('common.status.cancelled') }}</option>
        </select>

        <select v-model="filters.priority" class="input">
          <option value="">{{ $t('picking.filters.allPriority') }}</option>
          <option value="HIGH">{{ $t('picking.priority.high') }}</option>
          <option value="NORMAL">{{ $t('picking.priority.normal') }}</option>
          <option value="LOW">{{ $t('picking.priority.low') }}</option>
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
        <table class="min-w-[1200px] w-full text-sm">
          <thead class="bg-slate-50 text-slate-500">
            <tr>
              <th class="table-th">{{ $t('picking.table.pickNo') }}</th>
              <th class="table-th">{{ $t('picking.table.orderNo') }}</th>
              <th class="table-th">{{ $t('picking.table.customer') }}</th>
              <th class="table-th">{{ $t('picking.table.warehouse') }}</th>
              <th class="table-th">{{ $t('picking.table.priority') }}</th>
              <th class="table-th">{{ $t('picking.table.items') }}</th>
              <th class="table-th">{{ $t('picking.table.progress') }}</th>
              <th class="table-th">{{ $t('picking.table.dueDate') }}</th>
              <th class="table-th">{{ $t('picking.table.status') }}</th>
              <th class="table-th text-right">{{ $t('picking.table.action') }}</th>
            </tr>
          </thead>

          <tbody>
            <tr v-if="loading">
              <td colspan="10" class="text-center py-10 text-slate-400">{{ $t('picking.loadingTasks') }}</td>
            </tr>

            <tr v-else-if="pickingList.length === 0">
              <td colspan="10" class="text-center py-10 text-slate-400">{{ $t('picking.noTasks') }}</td>
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
                  {{ priorityLabel(item.priority) }}
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
                  {{ statusText(item.status) }}
                </span>
              </td>

              <td class="table-td text-right">
                <button
                  @click="openPicking(item)"
                  class="text-orange-600 hover:text-orange-700 font-semibold"
                >
                  {{ $t('picking.table.startPick') }}
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
              {{ isNewTask ? $t('picking.drawer.newTaskTitle') : $t('picking.drawer.taskTitle') }}
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
              <label class="label">{{ $t('picking.drawer.orderNo') }}</label>
              <input v-model="newTaskForm.orderNo" type="text" class="input" :placeholder="$t('picking.drawer.orderNoPlaceholder')" />
            </div>
            <div>
              <label class="label">{{ $t('picking.drawer.customer') }}</label>
              <input v-model="newTaskForm.customer" type="text" class="input" :placeholder="$t('picking.drawer.customerPlaceholder')" />
            </div>
            <div>
              <label class="label">{{ $t('picking.drawer.warehouse') }}</label>
              <select v-model="newTaskForm.warehouseId" class="input" @change="onWarehouseChange">
                <option value="">{{ $t('picking.drawer.selectWarehouse') }}</option>
                <option v-for="w in warehouses" :key="w.id" :value="w.id">{{ w.code }} — {{ w.name }}</option>
              </select>
            </div>
            <div>
              <label class="label">{{ $t('picking.drawer.priority') }}</label>
              <select v-model="newTaskForm.priority" class="input">
                <option value="LOW">{{ $t('picking.priority.low') }}</option>
                <option value="NORMAL">{{ $t('picking.priority.normal') }}</option>
                <option value="HIGH">{{ $t('picking.priority.high') }}</option>
              </select>
            </div>
            <div>
              <label class="label">{{ $t('picking.drawer.dueDate') }}</label>
              <input v-model="newTaskForm.dueDate" type="date" class="input" />
            </div>
          </div>
        </div>

        <!-- Existing task summary -->
        <div v-else class="bg-slate-50 rounded-2xl p-5 mb-6">
          <div class="grid grid-cols-2 md:grid-cols-4 gap-4">
            <div>
              <p class="detail-label">{{ $t('picking.drawer.customer') }}</p>
              <p class="detail-value">{{ selectedPicking.customer }}</p>
            </div>

            <div>
              <p class="detail-label">{{ $t('picking.drawer.warehouse') }}</p>
              <p class="detail-value">{{ selectedPicking.warehouse }}</p>
            </div>

            <div>
              <p class="detail-label">{{ $t('picking.drawer.priority') }}</p>
              <p class="detail-value text-orange-600">
                {{ priorityLabel(selectedPicking.priority) }}
              </p>
            </div>

            <div>
              <p class="detail-label">{{ $t('picking.drawer.dueDate') }}</p>
              <p class="detail-value">{{ selectedPicking.dueDate || '—' }}</p>
            </div>
          </div>
        </div>

        <!-- Cancellation details (Phase 18) -->
        <div v-if="!isNewTask && selectedPicking.status === 'CANCELLED'" class="bg-red-50 border border-red-200 rounded-2xl p-5 mb-6">
          <div class="grid grid-cols-2 md:grid-cols-3 gap-4">
            <div>
              <p class="detail-label">{{ $t('picking.drawer.cancelledAt') }}</p>
              <p class="detail-value">{{ formatDateTime(selectedPicking.cancelled_at) }}</p>
            </div>
            <div>
              <p class="detail-label">{{ $t('picking.drawer.cancelledBy') }}</p>
              <p class="detail-value">{{ selectedPicking.cancelled_by_name || '—' }}</p>
            </div>
            <div>
              <p class="detail-label">{{ $t('picking.drawer.cancellationRemark') }}</p>
              <p class="detail-value">{{ selectedPicking.cancel_remark || '—' }}</p>
            </div>
          </div>
        </div>

        <!-- Add item form (new task only) -->
        <div v-if="isNewTask" class="bg-white border border-slate-200 rounded-2xl p-4 mb-6 space-y-3">
          <h3 class="font-bold text-slate-800 text-sm">{{ $t('picking.drawer.addItemTitle') }}</h3>
          <div class="grid grid-cols-2 md:grid-cols-4 gap-3">
            <input v-model="addItemForm.sku" type="text" class="input" :placeholder="$t('picking.drawer.skuBarcodePlaceholder')" />
            <select v-model="addItemForm.locationId" class="input" :disabled="!newTaskForm.warehouseId">
              <option value="">{{ $t('picking.drawer.locationOption') }}</option>
              <option v-for="l in drawerLocations" :key="l.id" :value="l.id">{{ l.code }}</option>
            </select>
            <input v-model="addItemForm.lotNo" type="text" class="input" :placeholder="$t('picking.drawer.lotNoOptionalPlaceholder')" />
            <input v-model.number="addItemForm.orderedQty" type="number" class="input" :placeholder="$t('picking.drawer.orderQtyPlaceholder')" />
          </div>
          <button
            @click="addItem"
            class="bg-slate-800 hover:bg-slate-900 text-white px-4 py-2 rounded-xl font-semibold text-sm"
          >
            {{ $t('picking.drawer.addItem') }}
          </button>
        </div>

        <div class="space-y-4">
          <h3 class="font-bold text-slate-800">
            {{ $t('picking.drawer.itemsTitle') }}
          </h3>

          <p v-if="displayItems.length === 0" class="text-sm text-slate-400">
            {{ $t('picking.drawer.noItemsYet') }}
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
                  {{ $t('picking.drawer.locationPrefix', { location: product.location }) }}
                </p>
                <p v-if="!isNewTask" class="text-xs mt-1">
                  <span :class="getItemStatusClass(product.status)">{{ product.statusText }}</span>
                </p>
              </div>

              <div class="grid grid-cols-3 gap-3 min-w-[280px]">
                <div>
                  <p class="detail-label">{{ $t('picking.drawer.order') }}</p>
                  <p class="detail-value">{{ product.orderQty }}</p>
                </div>

                <div>
                  <p class="detail-label">{{ $t('picking.drawer.picked') }}</p>
                  <p class="detail-value text-green-600">
                    {{ product.pickedQty }}
                  </p>
                </div>

                <div>
                  <p class="detail-label">{{ $t('picking.drawer.remain') }}</p>
                  <p class="detail-value text-red-600">
                    {{ product.orderQty - product.pickedQty }}
                  </p>
                </div>
              </div>
            </div>

            <!-- New task: just a remove control -->
            <div v-if="isNewTask" class="mt-3 text-right">
              <button @click="removeNewItem(product.key)" class="text-red-600 hover:text-red-700 text-sm font-semibold">
                {{ $t('picking.drawer.remove') }}
              </button>
            </div>

            <!-- Existing task, resolved line -->
            <p v-else-if="product.status === 'COMPLETED' || product.status === 'SHORT'" class="text-sm text-green-600 font-semibold mt-3">
              {{ $t('picking.drawer.lineResolved', { status: product.statusText }) }}
            </p>

            <!-- Existing task, workable line -->
            <template v-else-if="!isNewTask">
              <div v-if="canConfirm && isTaskWorkable" class="grid grid-cols-1 md:grid-cols-3 gap-3 mt-4">
                <input
                  v-model.number="product.inputQty"
                  type="number"
                  class="input"
                  :placeholder="$t('picking.drawer.pickQtyPlaceholder')"
                />

                <button
                  @click="confirmPick(product)"
                  :disabled="submitting"
                  class="bg-orange-600 hover:bg-orange-700 text-white rounded-xl font-semibold disabled:opacity-60"
                >
                  {{ $t('picking.drawer.confirmPick') }}
                </button>

                <button
                  @click="shortPick(product)"
                  :disabled="submitting"
                  class="bg-red-600 hover:bg-red-700 text-white rounded-xl font-semibold disabled:opacity-60"
                >
                  {{ $t('picking.drawer.shortPick') }}
                </button>
              </div>
              <div v-if="canConfirm && isTaskWorkable" class="mt-3">
                <input
                  v-model="product.remark"
                  type="text"
                  class="input"
                  :placeholder="$t('picking.drawer.remarkPlaceholder')"
                />
              </div>
              <p v-else-if="!canConfirm" class="text-sm text-slate-500 mt-3">
                {{ $t('picking.drawer.viewOnlyNote') }}
              </p>
              <p v-else-if="selectedPicking.status === 'CANCELLED'" class="text-sm text-red-600 font-semibold mt-3">
                {{ $t('picking.drawer.cancelledNote') }}
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
          {{ submitting ? $t('picking.drawer.saving') : $t('picking.drawer.saveTask') }}
        </button>
        <div v-else>
          <div v-if="canCancel && isTaskWorkable" class="mt-6">
            <input
              v-model="cancelRemark"
              type="text"
              class="input"
              :placeholder="$t('picking.drawer.cancelRemarkPlaceholder')"
            />
          </div>
          <div class="flex gap-3 mt-3">
            <button
              v-if="canCancel && isTaskWorkable"
              @click="cancelTask"
              :disabled="submitting"
              class="flex-1 bg-red-600 hover:bg-red-700 text-white py-3 rounded-xl font-semibold disabled:opacity-60"
            >
              {{ $t('picking.drawer.cancelTask') }}
            </button>
            <button
              @click="closeDrawer"
              class="flex-1 bg-green-600 hover:bg-green-700 text-white py-3 rounded-xl font-semibold"
            >
              {{ $t('picking.drawer.close') }}
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
import i18n from '../../i18n'

let searchDebounce = null

function toApiDate(dateStr) {
  return dateStr ? `${dateStr}T00:00:00Z` : null
}

// Called from the template (methods.statusText below), not cached onto
// a row at map time, so the displayed label stays reactive to the
// active locale — switching language must not require re-fetching.
function statusText(status) {
  const t = i18n.global.t
  switch (status) {
    case 'PENDING': return t('common.status.pending')
    case 'IN_PROGRESS': return t('common.status.inProgress')
    case 'COMPLETED': return t('common.status.completed')
    case 'SHORT_PICK': return t('picking.filters.statusShortPick')
    case 'CANCELLED': return t('common.status.cancelled')
    default: return status
  }
}

function itemStatusText(status) {
  const t = i18n.global.t
  switch (status) {
    case 'PENDING': return t('picking.itemStatus.pending')
    case 'PARTIALLY_PICKED': return t('picking.itemStatus.partiallyPicked')
    case 'COMPLETED': return t('picking.itemStatus.completed')
    case 'SHORT': return t('picking.itemStatus.short')
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
    status: r.status
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
    statusText,

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
        this.drawerError = this.$t('picking.messages.skuBarcodeRequired')
        return
      }
      if (!this.addItemForm.locationId) {
        this.drawerError = this.$t('picking.messages.locationRequired')
        return
      }
      if (!this.addItemForm.orderedQty || this.addItemForm.orderedQty <= 0) {
        this.drawerError = this.$t('picking.messages.orderQtyPositive')
        return
      }

      const product = await this.findProduct(this.addItemForm.sku)
      if (!product) {
        this.drawerError = this.$t('picking.messages.noProductFound', { value: this.addItemForm.sku })
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
        this.drawerError = this.$t('picking.messages.orderNoRequired')
        return
      }
      if (!this.newTaskForm.warehouseId) {
        this.drawerError = this.$t('picking.messages.warehouseRequired')
        return
      }
      if (this.newItems.length === 0) {
        this.drawerError = this.$t('picking.messages.atLeastOneItem')
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
        alert(this.$t('picking.messages.taskCreated'))
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
        this.drawerError = this.$t('picking.messages.pickQtyPositive')
        return
      }
      const remainQty = product.orderQty - product.pickedQty
      if (product.inputQty > remainQty) {
        this.drawerError = this.$t('picking.messages.pickQtyExceedsRemaining')
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
        this.drawerError = this.$t('picking.messages.remarkRequiredForShortPick')
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
      return this.$d(new Date(isoString), 'long')
    },

    priorityLabel(priority) {
      if (priority === 'HIGH') return this.$t('picking.priority.high')
      if (priority === 'NORMAL') return this.$t('picking.priority.normal')
      if (priority === 'LOW') return this.$t('picking.priority.low')
      return priority
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
