<template>
  <div>
    <PageHeader :title="$t('reportsInventory.title')" :subtitle="$t('reports.sectionTitle')" />

    <div v-if="loadError" class="bg-red-50 border border-red-200 text-red-700 rounded-2xl p-4 mb-6">
      {{ loadError }}
    </div>

    <div class="flex gap-2 mb-6">
      <button type="button" class="tab-chip" :class="{ 'tab-chip-active': view === 'balance' }" @click="switchView('balance')">
        {{ $t('reportsInventory.tabs.balance') }}
      </button>
      <button type="button" class="tab-chip" :class="{ 'tab-chip-active': view === 'movement' }" @click="switchView('movement')">
        {{ $t('reportsInventory.tabs.movement') }}
      </button>
      <button type="button" class="tab-chip" :class="{ 'tab-chip-active': view === 'asof' }" @click="switchView('asof')">
        {{ $t('reportsInventory.tabs.asOf') }}
      </button>
    </div>

    <!-- Balance view -->
    <div v-if="view === 'balance'" class="space-y-6">
      <div class="bg-white rounded-2xl border border-slate-200 p-4 space-y-4">
        <div class="flex flex-wrap items-end gap-3">
          <label class="text-sm">
            <span class="block text-xs font-semibold text-slate-500 mb-1">{{ $t('common.labels.warehouse') }}</span>
            <select v-model="filters.warehouseId" class="field-select">
              <option value="">{{ $t('common.labels.selectOption') }}</option>
              <option v-for="wh in warehouses" :key="wh.id" :value="wh.id">{{ wh.name }}</option>
            </select>
          </label>
          <label class="text-sm">
            <span class="block text-xs font-semibold text-slate-500 mb-1">{{ $t('reportsReceiving.filters.search') }}</span>
            <input v-model="filters.search" type="text" class="field-input" :placeholder="$t('reportsInventory.filters.searchPlaceholder')" />
          </label>
          <button type="button" class="btn-primary" @click="applyFilters">{{ $t('reports.filterBar.apply') }}</button>
          <button type="button" class="btn-secondary" @click="resetFilters">{{ $t('reports.filterBar.reset') }}</button>
        </div>

        <div class="flex flex-wrap items-end gap-3 pt-3 border-t border-slate-100">
          <button type="button" class="preset-chip" :class="{ 'preset-chip-active': activeLens === 'none' }" @click="setLens('none')">
            {{ $t('reportsInventory.lenses.all') }}
          </button>
          <div class="flex items-end gap-2">
            <button type="button" class="preset-chip" :class="{ 'preset-chip-active': activeLens === 'lowStock' }" @click="setLens('lowStock')">
              {{ $t('reportsInventory.lenses.lowStock') }}
            </button>
            <input v-if="activeLens === 'lowStock'" v-model.number="lensParams.belowQty" type="number" min="0" class="field-input-sm" @change="applyFilters" />
          </div>
          <div class="flex items-end gap-2">
            <button type="button" class="preset-chip" :class="{ 'preset-chip-active': activeLens === 'nearExpiry' }" @click="setLens('nearExpiry')">
              {{ $t('reportsInventory.lenses.nearExpiry') }}
            </button>
            <input v-if="activeLens === 'nearExpiry'" v-model.number="lensParams.expiringWithinDays" type="number" min="0" class="field-input-sm" @change="applyFilters" />
          </div>
          <button type="button" class="preset-chip" :class="{ 'preset-chip-active': activeLens === 'expired' }" @click="setLens('expired')">
            {{ $t('reportsInventory.lenses.expired') }}
          </button>
          <div class="flex items-end gap-2">
            <button type="button" class="preset-chip" :class="{ 'preset-chip-active': activeLens === 'nonMoving' }" @click="setLens('nonMoving')">
              {{ $t('reportsInventory.lenses.nonMoving') }}
            </button>
            <input v-if="activeLens === 'nonMoving'" v-model.number="lensParams.inactiveDays" type="number" min="0" class="field-input-sm" @change="applyFilters" />
          </div>
        </div>
      </div>

      <ReportSummaryCards :cards="summaryCards" :loading="summaryLoading" />

      <ReportExportToolbar
        :generated-at="generatedAt"
        @preview-pdf="onPreviewPdf"
        @download-pdf="onDownloadPdf"
        @print="onPrint"
        @export-excel="onExportExcel"
        @export-csv="onExportCsv"
      />

      <ReportDataTable
        :loading="loading"
        :is-empty="!loading && rows.length === 0"
        :colspan="12"
        min-width="1500px"
        :page="page"
        :limit="limit"
        :total="total"
        @update:page="onPageChange"
      >
        <template #head>
          <tr>
            <th class="table-th">{{ $t('common.labels.warehouse') }}</th>
            <th class="table-th">{{ $t('common.labels.zone') }}</th>
            <th class="table-th">{{ $t('reportsStockCount.table.location') }}</th>
            <th class="table-th">{{ $t('reportsInventory.table.storageType') }}</th>
            <th class="table-th">{{ $t('reportsReceiving.table.sku') }}</th>
            <th class="table-th">{{ $t('reportsReceiving.table.lotNo') }}</th>
            <th class="table-th">{{ $t('reportsInventory.table.expiry') }}</th>
            <SortableTh column="on_hand" :sort-by="sortBy" :sort-dir="sortDir" @sort="toggleSort">{{ $t('reportsInventory.table.onHand') }}</SortableTh>
            <th class="table-th text-right">{{ $t('reportsInventory.table.reserved') }}</th>
            <th class="table-th text-right">{{ $t('reportsInventory.table.hold') }}</th>
            <SortableTh column="available" :sort-by="sortBy" :sort-dir="sortDir" @sort="toggleSort">{{ $t('reportsInventory.table.available') }}</SortableTh>
            <th class="table-th">{{ $t('reportsInventory.table.status') }}</th>
          </tr>
        </template>

        <tr v-for="row in rows" :key="row.inventory_id" class="hover:bg-slate-50">
          <td class="table-td">{{ row.warehouse_name }}</td>
          <td class="table-td">{{ row.zone_name }}</td>
          <td class="table-td">{{ row.location_code }}</td>
          <td class="table-td">{{ row.storage_type }}</td>
          <td class="table-td">{{ row.sku }}</td>
          <td class="table-td">{{ row.lot_no || '-' }}</td>
          <td class="table-td">{{ formatDate(row.expiry_date) }}</td>
          <td class="table-td text-right">{{ row.on_hand }} {{ row.uom }}</td>
          <td class="table-td text-right">{{ row.reserved }}</td>
          <td class="table-td text-right">{{ row.hold }}</td>
          <td class="table-td text-right font-semibold">{{ row.available }}</td>
          <td class="table-td"><StatusBadge :tone="statusTone(row.inventory_status)">{{ row.inventory_status }}</StatusBadge></td>
        </tr>
      </ReportDataTable>
    </div>

    <!-- Movement view -->
    <div v-else-if="view === 'movement'" class="space-y-6">
      <ReportFilterBar v-model="movementDateRange" :cols="2" :loading="movementLoading" @apply="applyMovementFilters" @reset="resetMovementFilters">
        <label class="text-sm">
          <span class="block text-xs font-semibold text-slate-500 mb-1">{{ $t('reportsInventory.filters.movementType') }}</span>
          <select v-model="movementFilters.transactionType" class="field-select">
            <option value="">{{ $t('common.labels.selectOption') }}</option>
            <option v-for="t in movementTypes" :key="t" :value="t">{{ t }}</option>
          </select>
        </label>
      </ReportFilterBar>

      <button type="button" class="export-btn" @click="onExportMovementsCsv">{{ $t('reports.export.exportCsv') }}</button>

      <ReportDataTable
        :loading="movementLoading"
        :is-empty="!movementLoading && movementRows.length === 0"
        :colspan="8"
        min-width="1300px"
        :page="movementPage"
        :limit="movementLimit"
        :total="movementTotal"
        @update:page="onMovementPageChange"
      >
        <template #head>
          <tr>
            <th class="table-th">{{ $t('reportsInventory.table.dateTime') }}</th>
            <th class="table-th">{{ $t('reportsInventory.filters.movementType') }}</th>
            <th class="table-th">{{ $t('common.labels.warehouse') }}</th>
            <th class="table-th">{{ $t('reportsStockCount.table.location') }}</th>
            <th class="table-th">{{ $t('reportsReceiving.table.sku') }}</th>
            <th class="table-th">{{ $t('reportsReceiving.table.lotNo') }}</th>
            <th class="table-th text-right">{{ $t('reportsInventory.table.quantity') }}</th>
            <th class="table-th">{{ $t('reportsInventory.table.reference') }}</th>
          </tr>
        </template>

        <tr v-for="row in movementRows" :key="row.transaction_id" class="hover:bg-slate-50">
          <td class="table-td">{{ formatDate(row.created_at) }}</td>
          <td class="table-td">{{ row.transaction_type }}</td>
          <td class="table-td">{{ row.warehouse_name }}</td>
          <td class="table-td">{{ row.location_code }}</td>
          <td class="table-td">{{ row.sku }}</td>
          <td class="table-td">{{ row.lot_no || '-' }}</td>
          <td class="table-td text-right" :class="row.quantity < 0 ? 'text-red-600' : 'text-blue-600'">{{ row.quantity }}</td>
          <td class="table-td">{{ row.reference_type ? `${row.reference_type} #${row.reference_id}` : '-' }}</td>
        </tr>
      </ReportDataTable>
    </div>

    <!-- As-of view -->
    <div v-else class="space-y-6">
      <div class="bg-orange-50 border border-orange-200 text-orange-800 rounded-2xl p-4">
        {{ $t('reportsInventory.asOfCaveat') }}
      </div>

      <div class="bg-white rounded-2xl border border-slate-200 p-4 flex flex-wrap items-end gap-3">
        <label class="text-sm">
          <span class="block text-xs font-semibold text-slate-500 mb-1">{{ $t('reportsInventory.filters.asOfDateTime') }}</span>
          <input v-model="asOfDateTime" type="datetime-local" class="field-input" />
        </label>
        <button type="button" class="btn-primary" @click="applyAsOf">{{ $t('reports.filterBar.apply') }}</button>
      </div>

      <ReportDataTable
        :loading="asOfLoading"
        :is-empty="!asOfLoading && asOfRows.length === 0"
        :colspan="6"
        min-width="1000px"
        :page="asOfPage"
        :limit="asOfLimit"
        :total="asOfTotal"
        @update:page="onAsOfPageChange"
      >
        <template #head>
          <tr>
            <th class="table-th">{{ $t('common.labels.warehouse') }}</th>
            <th class="table-th">{{ $t('reportsStockCount.table.location') }}</th>
            <th class="table-th">{{ $t('reportsReceiving.table.sku') }}</th>
            <th class="table-th">{{ $t('reportsReceiving.table.lotNo') }}</th>
            <th class="table-th text-right">{{ $t('reportsInventory.table.asOfQty') }}</th>
          </tr>
        </template>

        <tr v-for="(row, idx) in asOfRows" :key="idx" class="hover:bg-slate-50">
          <td class="table-td">{{ row.warehouse_name }}</td>
          <td class="table-td">{{ row.location_code }}</td>
          <td class="table-td">{{ row.sku }}</td>
          <td class="table-td">{{ row.lot_no || '-' }}</td>
          <td class="table-td text-right">{{ row.as_of_quantity }} {{ row.uom }}</td>
        </tr>
      </ReportDataTable>
    </div>

    <ReportPrintPreview v-model="showPreview" :pdf-url="previewUrl" />
  </div>
</template>

<script>
import PageHeader from '../../components/ui/PageHeader.vue'
import StatusBadge from '../../components/ui/StatusBadge.vue'
import SortableTh from '../../components/ui/SortableTh.vue'
import ReportFilterBar from '../../components/reports/ReportFilterBar.vue'
import ReportSummaryCards from '../../components/reports/ReportSummaryCards.vue'
import ReportDataTable from '../../components/reports/ReportDataTable.vue'
import ReportExportToolbar from '../../components/reports/ReportExportToolbar.vue'
import ReportPrintPreview from '../../components/reports/ReportPrintPreview.vue'
import { listWarehouses } from '../../services/warehouses'
import {
  listInventoryReport,
  getInventoryReportSummary,
  exportInventoryReportCsv,
  exportInventoryReportXlsx,
  previewInventoryReportPdf,
  downloadInventoryReportPdf,
  listInventoryMovements,
  exportInventoryMovementsCsv,
  listInventoryAsOf,
} from '../../services/reportsInventory'
import { resolveDateRange, toRangeISO, formatBangkokLocal, parseBangkokLocal } from '../../utils/reportDateRange'

const MOVEMENT_TYPES = ['RECEIVE', 'PUTAWAY', 'MOVE', 'PICK', 'SHIP', 'ADJUST', 'RETURN', 'COUNT', 'REJECT', 'TRANSFER_RECEIVE']

export default {
  name: 'InventoryReportView',

  components: {
    PageHeader, StatusBadge, SortableTh,
    ReportFilterBar, ReportSummaryCards, ReportDataTable, ReportExportToolbar, ReportPrintPreview,
  },

  data() {
    return {
      view: 'balance',
      warehouses: [],
      filters: { warehouseId: '', search: '' },
      activeLens: 'none',
      lensParams: { belowQty: 10, expiringWithinDays: 30, inactiveDays: 90 },

      rows: [],
      total: 0,
      page: 1,
      limit: 20,
      sortBy: 'on_hand',
      sortDir: 'desc',
      loading: false,

      summary: null,
      summaryLoading: false,

      loadError: '',
      generatedAt: '',
      showPreview: false,
      previewUrl: null,

      movementTypes: MOVEMENT_TYPES,
      movementDateRange: { preset: 'last30Days', from: null, to: null },
      movementFilters: { transactionType: '' },
      movementRows: [],
      movementTotal: 0,
      movementPage: 1,
      movementLimit: 20,
      movementLoading: false,

      asOfDateTime: formatBangkokLocal(new Date()),
      asOfRows: [],
      asOfTotal: 0,
      asOfPage: 1,
      asOfLimit: 20,
      asOfLoading: false,
    }
  },

  computed: {
    appliedQuery() {
      const q = { warehouseId: this.filters.warehouseId || undefined, search: this.filters.search || undefined }
      if (this.activeLens === 'lowStock') q.belowQty = this.lensParams.belowQty
      if (this.activeLens === 'nearExpiry') q.expiringWithinDays = this.lensParams.expiringWithinDays
      if (this.activeLens === 'expired') q.expiredOnly = true
      if (this.activeLens === 'nonMoving') q.inactiveDays = this.lensParams.inactiveDays
      return q
    },
    movementAppliedQuery() {
      const { from, to } = toRangeISO(resolveDateRange(this.movementDateRange.preset, this.movementDateRange))
      return { from, to, transactionType: this.movementFilters.transactionType || undefined }
    },
    summaryCards() {
      if (!this.summary) return []
      const na = this.$t('reportsReceiving.kpi.notAvailable')
      const uomLine = (list) => (list || []).map((u) => `${u.qty} ${u.uom}`).join(', ') || '0'
      return [
        { key: 'onHand', label: this.$t('reportsInventory.kpi.totalOnHand'), value: uomLine(this.summary.total_on_hand_by_uom) },
        { key: 'available', label: this.$t('reportsInventory.kpi.totalAvailable'), value: uomLine(this.summary.total_available_by_uom) },
        { key: 'lowStock', label: this.$t('reportsInventory.kpi.lowStockCount'), value: this.summary.low_stock_count ?? na },
        { key: 'nearExpiry', label: this.$t('reportsInventory.kpi.nearExpiryCount'), value: this.summary.near_expiry_count ?? na },
        { key: 'expired', label: this.$t('reportsInventory.kpi.expiredCount'), value: this.summary.expired_count ?? na },
        { key: 'nonMoving', label: this.$t('reportsInventory.kpi.nonMovingCount'), value: this.summary.non_moving_count ?? na },
      ]
    },
  },

  async mounted() {
    await this.fetchWarehouses()
    await this.applyFilters()
  },

  methods: {
    switchView(view) {
      this.view = view
      if (view === 'movement' && this.movementRows.length === 0) this.applyMovementFilters()
      if (view === 'asof' && this.asOfRows.length === 0) this.applyAsOf()
    },
    formatDate(value) {
      if (!value) return '-'
      return new Date(value).toLocaleString()
    },
    statusTone(status) {
      return { AVAILABLE: 'green', RESERVED: 'blue', ON_HOLD: 'orange', ZERO: 'gray' }[status] || 'gray'
    },
    setLens(lens) {
      this.activeLens = lens
      this.applyFilters()
    },

    async fetchWarehouses() {
      try {
        const res = await listWarehouses({ status: 'ACTIVE', limit: 100 })
        this.warehouses = res.data
      } catch {
        this.warehouses = []
      }
    },

    async applyFilters() {
      this.page = 1
      await Promise.all([this.fetchRows(), this.fetchSummary()])
    },
    resetFilters() {
      this.filters = { warehouseId: '', search: '' }
      this.activeLens = 'none'
      this.applyFilters()
    },
    async fetchRows() {
      this.loading = true
      this.loadError = ''
      try {
        const res = await listInventoryReport(this.appliedQuery, { page: this.page, limit: this.limit, sortBy: this.sortBy, sortDir: this.sortDir })
        this.rows = res.data
        this.total = res.meta.total
        this.generatedAt = new Date().toLocaleString()
      } catch (err) {
        this.loadError = err.message || this.$t('reports.table.noData')
        this.rows = []
        this.total = 0
      } finally {
        this.loading = false
      }
    },
    async fetchSummary() {
      this.summaryLoading = true
      try {
        const res = await getInventoryReportSummary(this.appliedQuery)
        this.summary = res.data
      } catch {
        this.summary = null
      } finally {
        this.summaryLoading = false
      }
    },
    onPageChange(page) {
      this.page = page
      this.fetchRows()
    },
    toggleSort(column) {
      this.sortDir = this.sortBy === column && this.sortDir === 'asc' ? 'desc' : 'asc'
      this.sortBy = column
      this.page = 1
      this.fetchRows()
    },
    async onExportCsv() {
      await exportInventoryReportCsv(this.appliedQuery)
    },
    async onExportExcel() {
      await exportInventoryReportXlsx(this.appliedQuery)
    },
    async onDownloadPdf() {
      await downloadInventoryReportPdf(this.appliedQuery)
    },
    async onPreviewPdf() {
      const blob = await previewInventoryReportPdf(this.appliedQuery)
      this.previewUrl = URL.createObjectURL(blob)
      this.showPreview = true
    },
    onPrint() {
      window.print()
    },

    async applyMovementFilters() {
      this.movementPage = 1
      await this.fetchMovements()
    },
    resetMovementFilters() {
      this.movementDateRange = { preset: 'last30Days', from: null, to: null }
      this.movementFilters = { transactionType: '' }
      this.applyMovementFilters()
    },
    async fetchMovements() {
      this.movementLoading = true
      try {
        const res = await listInventoryMovements(this.movementAppliedQuery, { page: this.movementPage, limit: this.movementLimit })
        this.movementRows = res.data
        this.movementTotal = res.meta.total
      } catch (err) {
        this.loadError = err.message || this.$t('reports.table.noData')
        this.movementRows = []
        this.movementTotal = 0
      } finally {
        this.movementLoading = false
      }
    },
    onMovementPageChange(page) {
      this.movementPage = page
      this.fetchMovements()
    },
    async onExportMovementsCsv() {
      await exportInventoryMovementsCsv(this.movementAppliedQuery)
    },

    async applyAsOf() {
      this.asOfPage = 1
      await this.fetchAsOf()
    },
    async fetchAsOf() {
      this.asOfLoading = true
      try {
        const asOfISO = parseBangkokLocal(this.asOfDateTime).toISOString()
        const res = await listInventoryAsOf(asOfISO, { warehouseId: this.filters.warehouseId }, { page: this.asOfPage, limit: this.asOfLimit })
        this.asOfRows = res.data
        this.asOfTotal = res.meta.total
      } catch (err) {
        this.loadError = err.message || this.$t('reports.table.noData')
        this.asOfRows = []
        this.asOfTotal = 0
      } finally {
        this.asOfLoading = false
      }
    },
    onAsOfPageChange(page) {
      this.asOfPage = page
      this.fetchAsOf()
    },
  },
}
</script>

<style scoped>
.field-select, .field-input {
  border: 1px solid #cbd5e1;
  border-radius: 8px;
  padding: 6px 10px;
  font-size: 14px;
  min-width: 160px;
}
.field-input-sm {
  border: 1px solid #cbd5e1;
  border-radius: 8px;
  padding: 6px 8px;
  font-size: 14px;
  width: 90px;
}
.table-td {
  padding: 10px 16px;
  font-size: 13px;
  color: #334155;
  white-space: nowrap;
  border-bottom: 1px solid #f1f5f9;
}
.tab-chip, .preset-chip {
  padding: 8px 16px;
  border-radius: 999px;
  font-size: 13px;
  font-weight: 600;
  background: #f1f5f9;
  color: #475569;
  border: 1px solid transparent;
  transition: background-color 0.15s, color 0.15s;
}
.tab-chip:hover, .preset-chip:hover {
  background: #e2e8f0;
}
.tab-chip-active, .preset-chip-active {
  background: #fff7ed;
  color: #c2410c;
  border-color: #fdba74;
}
.btn-primary {
  background: #ea580c;
  color: white;
  padding: 8px 16px;
  border-radius: 8px;
  font-size: 14px;
  font-weight: 600;
}
.btn-primary:hover {
  background: #c2410c;
}
.btn-secondary {
  background: #f1f5f9;
  color: #334155;
  padding: 8px 16px;
  border-radius: 8px;
  font-size: 14px;
  font-weight: 500;
}
.btn-secondary:hover {
  background: #e2e8f0;
}
.export-btn {
  background: #f1f5f9;
  color: #334155;
  padding: 8px 14px;
  border-radius: 8px;
  font-size: 13px;
  font-weight: 500;
}
.export-btn:hover {
  background: #e2e8f0;
}
</style>
