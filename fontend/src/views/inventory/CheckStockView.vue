<template>
  <div class="space-y-6">
    <div class="flex flex-col xl:flex-row xl:items-center xl:justify-between gap-4">
      <div>
        <h1 class="text-2xl font-bold text-slate-800">{{ $t('checkStock.title') }}</h1>
        <p class="text-slate-500">
          {{ $t('checkStock.subtitle') }}
        </p>
      </div>

      <button
        v-if="!countId"
        @click="createCountTask"
        :disabled="submitting"
        class="bg-orange-600 hover:bg-orange-700 text-white px-5 py-3 rounded-xl font-semibold disabled:opacity-60"
      >
        {{ $t('checkStock.newCountTask') }}
      </button>
      <span v-else class="text-sm text-slate-500 font-semibold">
        {{ countNo }} — {{ statusText }}
      </span>
    </div>

    <div v-if="pageError" class="bg-red-50 border border-red-200 text-red-600 rounded-xl p-3 text-sm">
      {{ pageError }}
    </div>

    <div class="bg-white rounded-2xl border border-slate-200 p-6">
      <div class="flex flex-col xl:flex-row xl:items-center xl:justify-between gap-4 mb-5">
        <div>
          <h2 class="text-lg font-bold text-slate-800">{{ $t('checkStock.history.title') }}</h2>
          <p class="text-sm text-slate-500">
            {{ $t('checkStock.history.subtitle') }}
          </p>
        </div>
        <button
          v-if="countId"
          @click="resetWorkspace"
          class="text-orange-600 hover:text-orange-700 font-semibold text-sm"
        >
          {{ $t('checkStock.startDifferentCount') }}
        </button>
      </div>

      <div class="grid grid-cols-1 md:grid-cols-5 gap-4 mb-5">
        <input
          v-model="historyFilters.search"
          type="text"
          :placeholder="$t('checkStock.history.searchPlaceholder')"
          class="input"
        />

        <select v-model="historyFilters.status" class="input">
          <option value="">{{ $t('checkStock.history.allStatus') }}</option>
          <option value="DRAFT">{{ $t('checkStock.status.draft') }}</option>
          <option value="CONFIRMED">{{ $t('checkStock.status.confirmed') }}</option>
        </select>

        <select v-model="historyFilters.warehouseId" class="input">
          <option value="">{{ $t('checkStock.history.allWarehouses') }}</option>
          <option v-for="w in warehouses" :key="w.id" :value="w.id">{{ w.code }}</option>
        </select>

        <input v-model="historyFilters.date" type="date" class="input" />

        <button
          @click="clearHistoryFilters"
          class="bg-slate-100 hover:bg-slate-200 text-slate-700 rounded-xl font-semibold"
        >
          {{ $t('common.buttons.clearFilter') }}
        </button>
      </div>

      <div class="flex justify-end mb-5">
        <button
          @click="exportHistoryCsv"
          :disabled="exportingHistory"
          class="bg-slate-100 hover:bg-slate-200 text-slate-700 px-4 py-2 rounded-xl font-semibold text-sm disabled:opacity-60"
        >
          {{ exportingHistory ? $t('common.buttons.exporting') : '⬇ ' + $t('common.buttons.exportCsv') }}
        </button>
      </div>

      <div v-if="historyError" class="bg-red-50 border border-red-200 text-red-600 rounded-xl p-3 text-sm mb-4">
        {{ historyError }}
      </div>

      <div class="border border-slate-200 rounded-2xl overflow-hidden">
        <div class="w-full overflow-x-auto">
          <table class="min-w-[1100px] w-full text-sm">
            <thead class="bg-slate-50 text-slate-500">
              <tr>
                <th class="table-th">{{ $t('checkStock.history.table.countNo') }}</th>
                <th class="table-th">{{ $t('checkStock.history.table.type') }}</th>
                <th class="table-th">{{ $t('checkStock.history.table.warehouse') }}</th>
                <th class="table-th">{{ $t('checkStock.history.table.zone') }}</th>
                <th class="table-th">{{ $t('checkStock.history.table.createdDate') }}</th>
                <th class="table-th">{{ $t('checkStock.history.table.createdBy') }}</th>
                <th class="table-th text-right">{{ $t('checkStock.history.table.items') }}</th>
                <th class="table-th text-right">{{ $t('checkStock.history.table.varianceLines') }}</th>
                <th class="table-th">{{ $t('checkStock.history.table.status') }}</th>
                <th class="table-th text-right">{{ $t('checkStock.history.table.action') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-if="historyLoading">
                <td colspan="10" class="text-center py-10 text-slate-400">{{ $t('checkStock.history.table.loading') }}</td>
              </tr>
              <tr v-else-if="historyList.length === 0">
                <td colspan="10" class="text-center py-10 text-slate-400">{{ $t('checkStock.history.table.noData') }}</td>
              </tr>
              <template v-else>
                <tr
                  v-for="row in historyList"
                  :key="row.id"
                  class="border-t hover:bg-orange-50"
                  :class="row.id === countId ? 'bg-orange-50' : ''"
                >
                  <td class="table-td font-semibold text-slate-800">{{ row.countNo }}</td>
                  <td class="table-td">{{ countTypeText(row.countType) }}</td>
                  <td class="table-td">{{ row.warehouseCode }}</td>
                  <td class="table-td">{{ row.zoneCode }}</td>
                  <td class="table-td">{{ row.createdAt }}</td>
                  <td class="table-td">{{ row.createdByName }}</td>
                  <td class="table-td text-right">{{ row.itemCount }}</td>
                  <td class="table-td text-right">
                    <span :class="row.varianceLines > 0 ? 'text-red-600 font-semibold' : ''">{{ row.varianceLines }}</span>
                  </td>
                  <td class="table-td">
                    <span :class="historyStatusClass(row.status)">
                      {{ historyStatusText(row.status) }}
                    </span>
                  </td>
                  <td class="table-td text-right">
                    <button
                      @click="viewCount(row)"
                      class="text-orange-600 hover:text-orange-700 font-semibold"
                    >
                      {{ $t('checkStock.history.table.view') }}
                    </button>
                  </td>
                </tr>
              </template>
            </tbody>
          </table>
        </div>
      </div>
    </div>

    <div class="bg-white rounded-2xl border border-slate-200 p-6">
      <h2 class="text-lg font-bold text-slate-800 mb-5">{{ $t('checkStock.countInfo.title') }}</h2>

      <div class="grid grid-cols-1 md:grid-cols-4 gap-4">
        <div>
          <label class="label">{{ $t('checkStock.countInfo.countType') }}</label>
          <select v-model="form.countType" class="input" :disabled="!!countId">
            <option value="CYCLE_COUNT">{{ $t('checkStock.countInfo.cycleCount') }}</option>
            <option value="FULL_COUNT">{{ $t('checkStock.countInfo.fullCount') }}</option>
            <option value="LOCATION_COUNT">{{ $t('checkStock.countInfo.locationCount') }}</option>
            <option value="SKU_COUNT">{{ $t('checkStock.countInfo.skuCount') }}</option>
          </select>
        </div>

        <div>
          <label class="label">{{ $t('checkStock.countInfo.warehouse') }}</label>
          <select v-model="form.warehouseId" class="input" :disabled="!!countId" @change="onWarehouseChange">
            <option value="">{{ $t('checkStock.countInfo.selectWarehouse') }}</option>
            <option v-for="w in warehouses" :key="w.id" :value="w.id">{{ w.code }} — {{ w.name }}</option>
          </select>
        </div>

        <div>
          <label class="label">{{ $t('checkStock.countInfo.zone') }}</label>
          <select v-model="form.zoneId" class="input" :disabled="!!countId">
            <option value="">{{ $t('checkStock.countInfo.allZone') }}</option>
            <option v-for="z in zones" :key="z.id" :value="z.id">{{ z.code }}</option>
          </select>
        </div>

        <div>
          <label class="label">{{ $t('checkStock.countInfo.countDate') }}</label>
          <input v-model="form.countDate" type="date" class="input" :disabled="!!countId" />
        </div>
      </div>
    </div>

    <div v-if="countId" class="bg-orange-50 border border-orange-200 rounded-2xl p-5">
      <div class="flex flex-col xl:flex-row xl:items-center xl:justify-between gap-4 mb-4">
        <div>
          <h2 class="text-lg font-bold text-slate-800">{{ $t('checkStock.scan.title') }}</h2>
          <p class="text-sm text-slate-500">
            {{ $t('checkStock.scan.subtitle') }}
          </p>
        </div>

        <span class="badge-orange">{{ $t('checkStock.scan.scannerReady') }}</span>
      </div>

      <div class="grid grid-cols-1 md:grid-cols-5 gap-4">
        <input
          v-model="scanForm.sku"
          type="text"
          class="input bg-white"
          :placeholder="$t('checkStock.scan.skuPlaceholder')"
          @keyup.enter="focusCountQty"
          ref="skuInput"
          :disabled="!canEdit"
        />

        <input
          v-model="scanForm.location"
          type="text"
          class="input bg-white"
          :placeholder="$t('checkStock.scan.locationPlaceholder')"
          :disabled="!canEdit"
        />

        <input
          v-model.number="scanForm.countQty"
          type="number"
          class="input bg-white input-number"
          :placeholder="$t('checkStock.scan.countQtyPlaceholder')"
          @keyup.enter="confirmScan"
          ref="countQtyInput"
          :disabled="!canEdit"
        />

        <select v-model="scanForm.condition" class="input bg-white" :disabled="!canEdit">
          <option value="GOOD">{{ $t('checkStock.scan.good') }}</option>
          <option value="DAMAGED">{{ $t('checkStock.scan.damaged') }}</option>
          <option value="HOLD">{{ $t('checkStock.scan.hold') }}</option>
        </select>

        <button
          @click="confirmScan"
          :disabled="!canEdit || submitting"
          class="bg-orange-600 hover:bg-orange-700 text-white rounded-xl font-semibold disabled:opacity-60"
        >
          {{ $t('checkStock.scan.confirmCount') }}
        </button>
      </div>
    </div>

    <div v-if="countId" class="bg-white rounded-2xl border border-slate-200 p-6">
      <div class="flex flex-col xl:flex-row xl:items-center xl:justify-between gap-4 mb-5">
        <div>
          <h2 class="text-lg font-bold text-slate-800">{{ $t('checkStock.items.title') }}</h2>
          <p class="text-sm text-slate-500">
            {{ $t('checkStock.items.subtitle') }}
          </p>
        </div>
      </div>

      <div v-if="canEdit" class="grid grid-cols-1 md:grid-cols-4 gap-3 mb-5 bg-slate-50 rounded-2xl p-4">
        <input v-model="addItemForm.sku" type="text" class="input" :placeholder="$t('checkStock.items.skuPlaceholder')" />
        <select v-model="addItemForm.locationId" class="input">
          <option value="">{{ $t('checkStock.items.locationPlaceholder') }}</option>
          <option v-for="l in drawerLocations" :key="l.id" :value="l.id">{{ l.code }}</option>
        </select>
        <input v-model="addItemForm.lotNo" type="text" class="input" :placeholder="$t('checkStock.items.lotNoPlaceholder')" />
        <button @click="addItem" :disabled="submitting" class="bg-slate-800 hover:bg-slate-900 text-white rounded-xl font-semibold disabled:opacity-60">
          {{ $t('checkStock.items.addItem') }}
        </button>
      </div>

      <div class="border border-slate-200 rounded-2xl overflow-hidden">
        <div class="w-full overflow-x-auto">
          <table class="min-w-[1400px] w-full text-sm">
            <thead class="bg-slate-50 text-slate-500">
              <tr>
                <th class="table-th">{{ $t('checkStock.items.table.sku') }}</th>
                <th class="table-th">{{ $t('checkStock.items.table.productName') }}</th>
                <th class="table-th">{{ $t('checkStock.items.table.location') }}</th>
                <th class="table-th">{{ $t('checkStock.items.table.lotNo') }}</th>
                <th class="table-th text-right">{{ $t('checkStock.items.table.systemQty') }}</th>
                <th class="table-th text-right">{{ $t('checkStock.items.table.countQty') }}</th>
                <th class="table-th text-right">{{ $t('checkStock.items.table.variance') }}</th>
                <th class="table-th">{{ $t('checkStock.items.table.uom') }}</th>
                <th class="table-th">{{ $t('checkStock.items.table.condition') }}</th>
                <th class="table-th">{{ $t('checkStock.items.table.status') }}</th>
                <th class="table-th">{{ $t('checkStock.items.table.remark') }}</th>
                <th class="table-th text-right">{{ $t('checkStock.items.table.action') }}</th>
              </tr>
            </thead>

            <tbody>
              <tr
                v-for="item in items"
                :key="item.id"
                class="border-t hover:bg-orange-50"
                :class="rowVariance(item) !== 0 ? 'bg-red-50' : ''"
              >
                <td class="table-td">{{ item.sku }}</td>
                <td class="table-td min-w-[220px]">{{ item.productName }}</td>
                <td class="table-td">{{ item.locationCode }}</td>
                <td class="table-td">{{ item.lotNo || '—' }}</td>

                <td class="table-td text-right">{{ item.systemQty }}</td>

                <td class="table-td">
                  <input
                    v-model.number="item.editCountQty"
                    type="number"
                    class="table-input input-number"
                    :readonly="!canEdit"
                  />
                </td>

                <td class="table-td text-right font-bold">
                  <span :class="rowVariance(item) === 0 ? 'text-slate-600' : 'text-red-600'">
                    {{ rowVariance(item) > 0 ? '+' : '' }}{{ rowVariance(item) }}
                  </span>
                </td>

                <td class="table-td">{{ item.uom }}</td>

                <td class="table-td">
                  <select v-model="item.editCondition" class="table-input min-w-[130px]" :disabled="!canEdit">
                    <option value="GOOD">{{ $t('checkStock.scan.good') }}</option>
                    <option value="DAMAGED">{{ $t('checkStock.scan.damaged') }}</option>
                    <option value="HOLD">{{ $t('checkStock.scan.hold') }}</option>
                  </select>
                </td>

                <td class="table-td">
                  <span :class="getItemStatusClass(item.status)">{{ $t('checkStock.itemStatus.' + item.status.toLowerCase()) }}</span>
                </td>

                <td class="table-td">
                  <input
                    v-model="item.editRemark"
                    class="table-input min-w-[220px]"
                    :placeholder="$t('checkStock.items.table.remarkPlaceholder')"
                    :readonly="!canEdit"
                  />
                </td>

                <td class="table-td text-right">
                  <button
                    v-if="canEdit"
                    @click="saveItem(item)"
                    :disabled="submitting"
                    class="text-orange-600 hover:text-orange-700 font-semibold disabled:opacity-60"
                  >
                    {{ $t('checkStock.items.table.save') }}
                  </button>
                </td>
              </tr>

              <tr v-if="items.length === 0">
                <td colspan="12" class="text-center py-10 text-slate-400">
                  {{ $t('checkStock.items.table.noData') }}
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </div>

    <div v-if="countId" class="bg-white rounded-2xl border border-slate-200 p-6">
      <h2 class="text-lg font-bold text-slate-800 mb-5">{{ $t('checkStock.summary.title') }}</h2>

      <div class="grid grid-cols-1 md:grid-cols-4 gap-5">
        <div class="summary-card">
          <p class="summary-label">{{ $t('checkStock.summary.totalLines') }}</p>
          <h2 class="summary-value text-number">{{ items.length }}</h2>
        </div>

        <div class="summary-card">
          <p class="summary-label">{{ $t('checkStock.summary.matchedLines') }}</p>
          <h2 class="summary-value text-green-600 text-number">{{ matchedLines }}</h2>
        </div>

        <div class="summary-card">
          <p class="summary-label">{{ $t('checkStock.summary.varianceLines') }}</p>
          <h2 class="summary-value text-red-600 text-number">{{ varianceLines }}</h2>
        </div>

        <div class="summary-card">
          <p class="summary-label">{{ $t('checkStock.summary.netVariance') }}</p>
          <h2
            class="summary-value text-number"
            :class="netVariance === 0 ? 'text-green-600' : 'text-orange-600'"
          >
            {{ netVariance > 0 ? '+' : '' }}{{ netVariance }}
          </h2>
        </div>
      </div>
    </div>

    <div v-if="countId && generatedAdjustments.length > 0" class="bg-white rounded-2xl border border-slate-200 p-6">
      <div class="flex flex-col md:flex-row md:items-center md:justify-between gap-3 mb-4">
        <div>
          <h2 class="text-lg font-bold text-slate-800">{{ $t('checkStock.generatedAdjustments.title') }}</h2>
          <p class="text-sm text-slate-500">
            {{ $t('checkStock.generatedAdjustments.subtitle') }}
          </p>
        </div>
        <button
          @click="$router.push('/inventory/stock-adjustment')"
          class="text-orange-600 hover:text-orange-700 font-semibold text-sm"
        >
          {{ $t('checkStock.generatedAdjustments.viewLink') }}
        </button>
      </div>

      <div class="border border-slate-200 rounded-2xl overflow-hidden">
        <table class="min-w-full w-full text-sm">
          <thead class="bg-slate-50 text-slate-500">
            <tr>
              <th class="table-th">{{ $t('checkStock.generatedAdjustments.table.adjustmentNo') }}</th>
              <th class="table-th">{{ $t('checkStock.generatedAdjustments.table.reason') }}</th>
              <th class="table-th">{{ $t('checkStock.generatedAdjustments.table.status') }}</th>
              <th class="table-th">{{ $t('checkStock.generatedAdjustments.table.created') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="adj in generatedAdjustments" :key="adj.id" class="border-t">
              <td class="table-td font-semibold text-slate-800">{{ adj.adjustment_no }}</td>
              <td class="table-td">{{ adj.reason }}</td>
              <td class="table-td">
                <span :class="adjustmentStatusClass(adj.status)">{{ adjustmentStatusLabel(adj.status) }}</span>
              </td>
              <td class="table-td">{{ (adj.created_at || '').slice(0, 10) }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <div v-if="countId && canEdit" class="bg-white border border-slate-200 rounded-2xl p-6">
      <p v-if="varianceLines > 0" class="text-xs text-slate-500 mb-3 text-right">
        {{ $t('checkStock.actions.chooseOneNote') }}
      </p>
      <div class="flex flex-col md:flex-row justify-end gap-3">
        <button
          @click="saveDraft"
          :disabled="submitting"
          class="bg-white hover:bg-slate-100 text-slate-700 border border-slate-300 px-6 py-3 rounded-xl font-semibold disabled:opacity-60"
        >
          {{ $t('checkStock.actions.saveDraft') }}
        </button>

        <button
          @click="createAdjustment"
          :disabled="submitting"
          class="bg-slate-800 hover:bg-slate-900 text-white px-6 py-3 rounded-xl font-semibold disabled:opacity-60"
        >
          {{ $t('checkStock.actions.createAdjustment') }}
        </button>

        <button
          @click="confirmCount"
          :disabled="submitting"
          class="bg-orange-600 hover:bg-orange-700 text-white px-6 py-3 rounded-xl font-semibold disabled:opacity-60"
        >
          {{ $t('checkStock.actions.confirmCount') }}
        </button>
      </div>
    </div>
  </div>
</template>

<script>
import { listStockCounts, getStockCount, createStockCount, addStockCountItem, recordStockCount, confirmStockCount, createAdjustmentsFromCount, newIdempotencyKey, exportStockCounts } from '../../services/stockCount'
import { listWarehouses, listZones, listLocations } from '../../services/warehouses'
import { listProducts } from '../../services/products'
import i18n from '../../i18n'

let historySearchDebounce = null

function todayDate() {
  return new Date().toISOString().slice(0, 10)
}

function countTypeText(type) {
  const t = i18n.global.t
  switch (type) {
    case 'CYCLE_COUNT': return t('checkStock.countInfo.cycleCount')
    case 'FULL_COUNT': return t('checkStock.countInfo.fullCount')
    case 'LOCATION_COUNT': return t('checkStock.countInfo.locationCount')
    case 'SKU_COUNT': return t('checkStock.countInfo.skuCount')
    default: return type
  }
}

// Phase 16 — ADJUSTMENT_CREATED is the new terminal state "Create
// Adjustment" claims a count into (mutually exclusive with CONFIRMED,
// which "Confirm Count" claims). Both are non-editable (canEdit is
// DRAFT-only, unchanged), but need their own distinct label so a count
// routed through the adjustment bridge doesn't display as "Draft".
function historyStatusText(status) {
  const t = i18n.global.t
  if (status === 'CONFIRMED') return t('checkStock.status.confirmed')
  if (status === 'ADJUSTMENT_CREATED') return t('checkStock.status.adjustmentCreated')
  return t('checkStock.status.draft')
}

function historyStatusClass(status) {
  if (status === 'CONFIRMED') return 'badge-green'
  if (status === 'ADJUSTMENT_CREATED') return 'badge-orange'
  return 'badge-gray'
}

// Phase 17 — status badge for a generated Stock Adjustment document
// (DRAFT/PENDING/APPROVED/REJECTED, StockAdjustmentView.vue's own
// vocabulary — distinct from this page's own Status/ItemStatus values,
// so reusing getItemStatusClass here would be wrong).
function adjustmentStatusClass(status) {
  if (status === 'APPROVED') return 'badge-green'
  if (status === 'PENDING') return 'badge-orange'
  if (status === 'REJECTED') return 'badge-red'
  return 'badge-gray'
}

// Reuses stockAdjustment.status.* (StockAdjustmentView.vue's own
// vocabulary for this exact DRAFT/PENDING/APPROVED/REJECTED set) rather
// than duplicating the labels under checkStock.*. Called from the
// template, not cached at map time, so it stays reactive to the locale.
function adjustmentStatusLabel(status) {
  const t = i18n.global.t
  switch (status) {
    case 'DRAFT': return t('stockAdjustment.status.draft')
    case 'PENDING': return t('stockAdjustment.status.pending')
    case 'APPROVED': return t('stockAdjustment.status.approved')
    case 'REJECTED': return t('stockAdjustment.status.rejected')
    default: return status
  }
}

function mapHistoryRow(r) {
  return {
    id: r.id,
    countNo: r.count_no,
    countType: r.count_type,
    warehouseCode: r.warehouse_code,
    zoneCode: r.zone_code || '—',
    countDate: r.count_date ? r.count_date.slice(0, 10) : '—',
    itemCount: r.item_count,
    varianceLines: r.variance_lines,
    status: r.status,
    createdByName: r.created_by_name || '—',
    createdAt: r.created_at ? r.created_at.slice(0, 10) : '—'
  }
}

function mapItem(it) {
  return {
    id: it.id,
    sku: it.sku,
    productName: it.product_name,
    locationCode: it.location_code,
    lotNo: it.lot_no,
    uom: it.uom,
    systemQty: it.system_qty,
    countQty: it.count_qty,
    variance: it.variance_qty,
    status: it.status,
    editCountQty: it.count_qty != null ? it.count_qty : 0,
    editCondition: it.condition,
    editRemark: it.remark || ''
  }
}

export default {
  name: 'CheckStockView',

  data() {
    return {
      pageError: '',
      submitting: false,
      exportingHistory: false,

      historyList: [],
      historyLoading: false,
      historyError: '',
      historyFilters: {
        search: '',
        status: '',
        warehouseId: '',
        date: ''
      },

      countId: null,
      countNo: '',
      status: 'DRAFT',

      form: {
        countType: 'CYCLE_COUNT',
        warehouseId: '',
        zoneId: '',
        countDate: todayDate()
      },

      warehouses: [],
      zones: [],
      drawerLocations: [],

      scanForm: { sku: '', location: '', countQty: 1, condition: 'GOOD' },
      addItemForm: { sku: '', locationId: '', lotNo: '' },

      items: [],
      generatedAdjustments: []
    }
  },

  computed: {
    canEdit() {
      return this.status === 'DRAFT'
    },
    statusText() {
      return historyStatusText(this.status)
    },
    matchedLines() {
      return this.items.filter((item) => this.rowVariance(item) === 0).length
    },
    varianceLines() {
      return this.items.filter((item) => this.rowVariance(item) !== 0).length
    },
    netVariance() {
      return this.items.reduce((sum, item) => sum + this.rowVariance(item), 0)
    }
  },

  watch: {
    'historyFilters.search'() {
      clearTimeout(historySearchDebounce)
      historySearchDebounce = setTimeout(() => this.fetchHistory(), 300)
    },
    'historyFilters.status'() {
      this.fetchHistory()
    },
    'historyFilters.warehouseId'() {
      this.fetchHistory()
    },
    'historyFilters.date'() {
      this.fetchHistory()
    }
  },

  mounted() {
    this.fetchWarehouses()
    this.fetchHistory()
  },

  methods: {
    rowVariance(item) {
      return Number(item.editCountQty || 0) - Number(item.systemQty || 0)
    },

    countTypeText,
    historyStatusText,
    historyStatusClass,
    adjustmentStatusClass,
    adjustmentStatusLabel,

    async fetchHistory() {
      this.historyLoading = true
      this.historyError = ''
      try {
        const res = await listStockCounts({
          search: this.historyFilters.search,
          status: this.historyFilters.status,
          warehouseId: this.historyFilters.warehouseId,
          date: this.historyFilters.date,
          limit: 100
        })
        this.historyList = res.data.map(mapHistoryRow)
      } catch (err) {
        this.historyError = err.message
      } finally {
        this.historyLoading = false
      }
    },

    clearHistoryFilters() {
      this.historyFilters = { search: '', status: '', warehouseId: '', date: '' }
      this.fetchHistory()
    },

    async exportHistoryCsv() {
      this.exportingHistory = true
      try {
        await exportStockCounts({
          search: this.historyFilters.search, status: this.historyFilters.status,
          warehouseId: this.historyFilters.warehouseId, date: this.historyFilters.date
        })
      } catch (err) {
        this.historyError = err.message
      } finally {
        this.exportingHistory = false
      }
    },

    // Loads a past or in-progress count into the same workspace sections
    // "+ New Count Task" populates — canEdit (derived from the loaded
    // count's own status) then naturally makes a CONFIRMED count
    // read-only (inputs already gate on it) without any extra branching.
    async viewCount(row) {
      this.pageError = ''
      try {
        const res = await getStockCount(row.id)
        this.countId = res.data.id
        this.countNo = res.data.count_no
        this.status = res.data.status
        this.items = (res.data.items || []).map(mapItem)
        this.generatedAdjustments = res.data.generated_adjustments || []
        this.form = {
          countType: res.data.count_type,
          warehouseId: res.data.warehouse_id,
          zoneId: res.data.zone_id || '',
          countDate: res.data.count_date ? res.data.count_date.slice(0, 10) : ''
        }
        this.zones = []
        this.drawerLocations = []
        if (res.data.warehouse_id) {
          const zonesRes = await listZones(res.data.warehouse_id)
          this.zones = zonesRes.data
          const allLocations = []
          for (const zone of zonesRes.data) {
            const locRes = await listLocations(zone.id)
            allLocations.push(...locRes.data)
          }
          this.drawerLocations = allLocations
        }
      } catch (err) {
        this.pageError = err.message
      }
    },

    resetWorkspace() {
      this.pageError = ''
      this.countId = null
      this.countNo = ''
      this.status = 'DRAFT'
      this.items = []
      this.generatedAdjustments = []
      this.form = { countType: 'CYCLE_COUNT', warehouseId: '', zoneId: '', countDate: todayDate() }
      this.zones = []
      this.drawerLocations = []
      this.scanForm = { sku: '', location: '', countQty: 1, condition: 'GOOD' }
      this.addItemForm = { sku: '', locationId: '', lotNo: '' }
    },

    async fetchWarehouses() {
      try {
        const res = await listWarehouses({ limit: 100 })
        this.warehouses = res.data
      } catch (err) {
        this.pageError = err.message
      }
    },

    async onWarehouseChange() {
      this.form.zoneId = ''
      this.zones = []
      this.drawerLocations = []
      if (!this.form.warehouseId) return
      try {
        const res = await listZones(this.form.warehouseId)
        this.zones = res.data
        const allLocations = []
        for (const zone of res.data) {
          const locRes = await listLocations(zone.id)
          allLocations.push(...locRes.data)
        }
        this.drawerLocations = allLocations
      } catch (err) {
        this.pageError = err.message
      }
    },

    async createCountTask() {
      this.pageError = ''
      if (!this.form.warehouseId) {
        this.pageError = this.$t('checkStock.messages.warehouseRequired')
        return
      }

      this.submitting = true
      try {
        const res = await createStockCount({
          count_type: this.form.countType,
          warehouse_id: this.form.warehouseId,
          zone_id: this.form.zoneId || null,
          count_date: this.form.countDate ? `${this.form.countDate}T00:00:00Z` : null
        })
        this.countId = res.data.id
        this.countNo = res.data.count_no
        this.status = res.data.status
        this.items = (res.data.items || []).map(mapItem)
        this.generatedAdjustments = res.data.generated_adjustments || []
        await this.fetchHistory()
      } catch (err) {
        this.pageError = err.message
      } finally {
        this.submitting = false
      }
    },

    async addItem() {
      this.pageError = ''
      if (!this.addItemForm.sku) {
        this.pageError = this.$t('checkStock.messages.skuBarcodeRequired')
        return
      }
      if (!this.addItemForm.locationId) {
        this.pageError = this.$t('checkStock.messages.locationRequired')
        return
      }

      const trimmed = this.addItemForm.sku.trim()
      let product
      try {
        const res = await listProducts({ search: trimmed, limit: 20 })
        product = res.data.find((p) => p.sku === trimmed || p.barcode === trimmed)
      } catch (err) {
        this.pageError = err.message
        return
      }
      if (!product) {
        this.pageError = this.$t('checkStock.messages.noProductFound', { value: trimmed })
        return
      }

      this.submitting = true
      try {
        const res = await addStockCountItem(this.countId, {
          product_id: product.id, location_id: this.addItemForm.locationId, lot_no: this.addItemForm.lotNo || '',
          uom: product.base_uom
        })
        this.items = (res.data.items || []).map(mapItem)
        this.addItemForm = { sku: '', locationId: this.addItemForm.locationId, lotNo: '' }
      } catch (err) {
        this.pageError = err.message
      } finally {
        this.submitting = false
      }
    },

    focusCountQty() {
      if (this.$refs.countQtyInput) this.$refs.countQtyInput.focus()
    },

    async confirmScan() {
      this.pageError = ''
      if (!this.scanForm.sku) {
        this.pageError = this.$t('checkStock.messages.skuOrBarcodeRequired')
        return
      }

      const item = this.items.find((it) => it.sku === this.scanForm.sku &&
        (!this.scanForm.location || it.locationCode === this.scanForm.location))
      if (!item) {
        this.pageError = this.$t('checkStock.messages.itemNotFound')
        return
      }

      item.editCountQty = Number(this.scanForm.countQty || 0)
      item.editCondition = this.scanForm.condition
      await this.saveItem(item)

      this.scanForm = { sku: '', location: '', countQty: 1, condition: 'GOOD' }
      this.$nextTick(() => {
        if (this.$refs.skuInput) this.$refs.skuInput.focus()
      })
    },

    async saveItem(item) {
      this.submitting = true
      try {
        const res = await recordStockCount(this.countId, item.id, {
          count_qty: Number(item.editCountQty || 0), condition: item.editCondition,
          status: 'COUNTED', remark: item.editRemark || null
        })
        this.items = (res.data.items || []).map(mapItem)
      } catch (err) {
        this.pageError = err.message
      } finally {
        this.submitting = false
      }
    },

    async saveDraft() {
      this.submitting = true
      try {
        for (const item of this.items) {
          await recordStockCount(this.countId, item.id, {
            count_qty: Number(item.editCountQty || 0), condition: item.editCondition,
            status: item.editCountQty ? 'COUNTED' : 'PENDING', remark: item.editRemark || null
          })
        }
        const res = await getStockCount(this.countId)
        this.items = (res.data.items || []).map(mapItem)
        this.generatedAdjustments = res.data.generated_adjustments || []
        alert(this.$t('checkStock.messages.draftSaved'))
      } catch (err) {
        this.pageError = err.message
      } finally {
        this.submitting = false
      }
    },

    // Phase 17: stays on this page and refreshes the count afterward
    // (rather than immediately redirecting to Stock Adjustment, as
    // before) so the newly generated document(s) actually render in the
    // "Generated Adjustments" panel — satisfying "show the resulting
    // adjustment document(s) clearly after success" instead of a bare
    // alert() the user had no time to read before being navigated away.
    // Reusing the same target route the old redirect already used for
    // the "View in Stock Adjustments" link — this codebase's routing has
    // no per-record deep link on that page (list+drawer only, the same
    // as every other module), so this can only open the list, not the
    // specific document; the user identifies it there by adjustment_no,
    // now visible in the panel.
    async createAdjustment() {
      this.pageError = ''
      if (this.varianceLines === 0) {
        this.pageError = this.$t('checkStock.messages.noVarianceFound')
        return
      }

      this.submitting = true
      try {
        await this.saveDraft()
        await createAdjustmentsFromCount(this.countId)
        const res = await getStockCount(this.countId)
        this.status = res.data.status
        this.items = (res.data.items || []).map(mapItem)
        this.generatedAdjustments = res.data.generated_adjustments || []
        await this.fetchHistory()
      } catch (err) {
        this.pageError = err.message
      } finally {
        this.submitting = false
      }
    },

    getItemStatusClass(status) {
      if (status === 'PENDING') return 'badge-gray'
      if (status === 'COUNTED') return 'badge-green'
      if (status === 'RECOUNT') return 'badge-red'
      return 'badge-gray'
    },

    async confirmCount() {
      this.pageError = ''
      const pendingItem = this.items.find((item) => !item.editCountQty && item.editCountQty !== 0)
      if (pendingItem) {
        this.pageError = this.$t('checkStock.messages.someItemsPending')
        return
      }

      this.submitting = true
      try {
        await this.saveDraft()
        const res = await confirmStockCount(this.countId, { idempotency_key: newIdempotencyKey() })
        this.status = res.data.status
        this.items = (res.data.items || []).map(mapItem)
        // Confirm never creates an adjustment (Phase 16), so this is
        // always [] here — refreshed anyway to stay in sync rather than
        // assume, matching Get's own "always fresh from the database"
        // guarantee (Phase 17).
        this.generatedAdjustments = res.data.generated_adjustments || []
        await this.fetchHistory()
        alert(this.$t('checkStock.messages.countConfirmed'))
      } catch (err) {
        this.pageError = err.message
      } finally {
        this.submitting = false
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

.label {
  display: block;
  margin-bottom: 8px;
  color: #334155;
  font-weight: 600;
  font-size: 14px;
}

.table-th {
  padding: 14px 16px;
  font-weight: 700;
  white-space: nowrap;
}

.table-td {
  padding: 10px 12px;
  color: #475569;
  white-space: nowrap;
}

.table-input {
  width: 100%;
  min-width: 120px;
  border: 1px solid #cbd5e1;
  border-radius: 10px;
  padding: 9px 10px;
  outline: none;
}

.table-input:focus {
  border-color: #ea580c;
  box-shadow: 0 0 0 2px #fed7aa;
}

.input-number,
.text-number {
  text-align: right;
  font-variant-numeric: tabular-nums;
}

.summary-card {
  background: #f8fafc;
  border: 1px solid #e2e8f0;
  border-radius: 18px;
  padding: 20px;
}

.summary-label {
  color: #64748b;
  font-size: 14px;
}

.summary-value {
  font-size: 28px;
  font-weight: 800;
  color: #0f172a;
  margin-top: 8px;
}

.badge-orange {
  background: #ffedd5;
  color: #ea580c;
  padding: 6px 12px;
  border-radius: 999px;
  font-weight: 700;
  font-size: 12px;
}

.badge-gray {
  background: #f1f5f9;
  color: #475569;
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
