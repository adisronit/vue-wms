<template>
  <div>
    <PageHeader :title="$t('reportsStockCount.title')" :subtitle="$t('reports.sectionTitle')" />

    <div v-if="loadError" class="bg-red-50 border border-red-200 text-red-700 rounded-2xl p-4 mb-6">
      {{ loadError }}
    </div>

    <div class="space-y-6">
      <ReportFilterBar v-model="dateRange" :cols="5" :loading="loading" @apply="applyFilters" @reset="resetFilters">
        <label class="text-sm">
          <span class="block text-xs font-semibold text-slate-500 mb-1">{{ $t('common.labels.warehouse') }}</span>
          <select v-model="filters.warehouseId" class="field-select" @change="onWarehouseChange">
            <option value="">{{ $t('common.labels.selectOption') }}</option>
            <option v-for="wh in warehouses" :key="wh.id" :value="wh.id">{{ wh.name }}</option>
          </select>
        </label>
        <label class="text-sm">
          <span class="block text-xs font-semibold text-slate-500 mb-1">{{ $t('common.labels.zone') }}</span>
          <select v-model="filters.zoneId" class="field-select" :disabled="!filters.warehouseId">
            <option value="">{{ $t('common.labels.selectOption') }}</option>
            <option v-for="z in zones" :key="z.id" :value="z.id">{{ z.name }}</option>
          </select>
        </label>
        <label class="text-sm">
          <span class="block text-xs font-semibold text-slate-500 mb-1">{{ $t('common.labels.status') }}</span>
          <select v-model="filters.status" class="field-select">
            <option value="">{{ $t('common.labels.selectOption') }}</option>
            <option v-for="s in countStatuses" :key="s" :value="s">{{ s }}</option>
          </select>
        </label>
        <label class="text-sm">
          <span class="block text-xs font-semibold text-slate-500 mb-1">{{ $t('reportsReceiving.filters.search') }}</span>
          <input v-model="filters.search" type="text" class="field-input" :placeholder="$t('reportsStockCount.filters.searchPlaceholder')" />
        </label>
      </ReportFilterBar>

      <div class="flex flex-wrap gap-2">
        <button
          v-for="preset in viewPresets"
          :key="preset.key"
          type="button"
          class="preset-chip"
          :class="{ 'preset-chip-active': activePreset === preset.key }"
          @click="applyPreset(preset.key)"
        >
          {{ $t(`reportsStockCount.presets.${preset.key}`) }}
        </button>
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
        :colspan="13"
        min-width="1500px"
        :page="page"
        :limit="limit"
        :total="total"
        @update:page="onPageChange"
      >
        <template #head>
          <tr>
            <SortableTh column="count_no" :sort-by="sortBy" :sort-dir="sortDir" @sort="toggleSort">{{ $t('reportsStockCount.table.countNo') }}</SortableTh>
            <SortableTh column="count_date" :sort-by="sortBy" :sort-dir="sortDir" @sort="toggleSort">{{ $t('reportsStockCount.table.countDate') }}</SortableTh>
            <th class="table-th">{{ $t('common.labels.warehouse') }}</th>
            <th class="table-th">{{ $t('common.labels.zone') }}</th>
            <th class="table-th">{{ $t('reportsStockCount.table.location') }}</th>
            <th class="table-th">{{ $t('reportsReceiving.table.sku') }}</th>
            <th class="table-th text-right">{{ $t('reportsStockCount.table.systemQty') }}</th>
            <th class="table-th text-right">{{ $t('reportsStockCount.table.countedQty') }}</th>
            <th class="table-th text-right">{{ $t('reportsStockCount.table.variance') }}</th>
            <th class="table-th">{{ $t('reportsStockCount.table.itemStatus') }}</th>
            <th class="table-th">{{ $t('reportsStockCount.table.countedBy') }}</th>
            <th class="table-th">{{ $t('reportsStockCount.table.approvedBy') }}</th>
            <th class="table-th">{{ $t('reportsStockCount.table.adjustmentRef') }}</th>
          </tr>
        </template>

        <tr v-for="row in rows" :key="row.count_item_id" class="hover:bg-slate-50">
          <td class="table-td">{{ row.count_no }}</td>
          <td class="table-td">{{ formatDate(row.count_date_time) }}</td>
          <td class="table-td">{{ row.warehouse_name }}</td>
          <td class="table-td">{{ row.zone_name || '-' }}</td>
          <td class="table-td">{{ row.location_code }}</td>
          <td class="table-td">{{ row.sku }}</td>
          <td class="table-td text-right">{{ row.system_qty }} {{ row.uom }}</td>
          <td class="table-td text-right">{{ row.counted_qty ?? '-' }}</td>
          <td class="table-td text-right" :class="varianceClass(row.variance_qty)">
            {{ row.variance_qty ?? '-' }}<span v-if="row.variance_pct != null"> ({{ row.variance_pct.toFixed(1) }}%)</span>
          </td>
          <td class="table-td"><StatusBadge :tone="itemStatusTone(row.item_status)">{{ row.item_status }}</StatusBadge></td>
          <td class="table-td">{{ row.counted_by || '-' }}</td>
          <td class="table-td">{{ row.approved_by || $t('reportsReceiving.kpi.notAvailable') }}</td>
          <td class="table-td">{{ row.adjustment_no || '-' }}</td>
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
import { listWarehouses, listZones } from '../../services/warehouses'
import {
  listStockCountReport,
  getStockCountReportSummary,
  exportStockCountReportCsv,
  exportStockCountReportXlsx,
  previewStockCountReportPdf,
  downloadStockCountReportPdf,
} from '../../services/reportsStockCount'
import { resolveDateRange, toRangeISO } from '../../utils/reportDateRange'

const COUNT_STATUSES = ['DRAFT', 'CONFIRMED', 'ADJUSTMENT_CREATED']

// The Phase 34C spec's 6 report "views" (Summary/Detail/Variance/
// Recount/Count Sheet/Adjustment Reference) are one underlying
// list+summary endpoint with different default filters — not 6
// separate endpoints, since they all query the same stock_count_items
// grain.
const VIEW_PRESETS = [
  { key: 'detail', filters: {} },
  { key: 'varianceOnly', filters: { varianceOnly: true } },
  { key: 'recount', filters: { itemStatus: 'RECOUNT' } },
  { key: 'countSheet', filters: { itemStatus: 'PENDING' } },
  { key: 'adjustmentReference', filters: { hasAdjustment: 'true' } },
]

export default {
  name: 'StockCountReportView',

  components: {
    PageHeader, StatusBadge, SortableTh,
    ReportFilterBar, ReportSummaryCards, ReportDataTable, ReportExportToolbar, ReportPrintPreview,
  },

  data() {
    return {
      dateRange: { preset: 'last30Days', from: null, to: null },
      filters: { warehouseId: '', zoneId: '', status: '', search: '', varianceOnly: false, itemStatus: '', hasAdjustment: undefined },
      warehouses: [],
      zones: [],
      countStatuses: COUNT_STATUSES,
      viewPresets: VIEW_PRESETS,
      activePreset: 'detail',

      rows: [],
      total: 0,
      page: 1,
      limit: 20,
      sortBy: 'count_date',
      sortDir: 'desc',
      loading: false,

      summary: null,
      summaryLoading: false,

      loadError: '',
      generatedAt: '',

      showPreview: false,
      previewUrl: null,
    }
  },

  computed: {
    appliedQuery() {
      const { from, to } = toRangeISO(resolveDateRange(this.dateRange.preset, this.dateRange))
      return {
        from, to,
        warehouseId: this.filters.warehouseId || undefined,
        zoneId: this.filters.zoneId || undefined,
        status: this.filters.status || undefined,
        search: this.filters.search || undefined,
        varianceOnly: this.filters.varianceOnly || undefined,
        itemStatus: this.filters.itemStatus || undefined,
        hasAdjustment: this.filters.hasAdjustment,
      }
    },
    summaryCards() {
      if (!this.summary) return []
      const s = this.summary
      return [
        { key: 'sessions', label: this.$t('reportsStockCount.kpi.totalSessions'), value: String(s.total_count_sessions) },
        { key: 'locations', label: this.$t('reportsStockCount.kpi.countedLocations'), value: String(s.counted_locations) },
        { key: 'skus', label: this.$t('reportsStockCount.kpi.countedSkus'), value: String(s.counted_skus) },
        { key: 'accuracy', label: this.$t('reportsStockCount.kpi.accuracy'), value: s.stock_count_accuracy != null ? `${s.stock_count_accuracy.toFixed(1)}%` : this.$t('reportsReceiving.kpi.notAvailable') },
        { key: 'variance', label: this.$t('reportsStockCount.kpi.quantityVariance'), value: String(s.quantity_variance) },
        { key: 'recount', label: this.$t('reportsStockCount.kpi.recountRate'), value: s.recount_rate != null ? `${s.recount_rate.toFixed(1)}%` : this.$t('reportsReceiving.kpi.notAvailable') },
        { key: 'pending', label: this.$t('reportsStockCount.kpi.pendingApproval'), value: String(s.pending_approval) },
      ]
    },
  },

  async mounted() {
    await this.fetchWarehouses()
    await this.applyFilters()
  },

  methods: {
    formatDate(value) {
      if (!value) return '-'
      return new Date(value).toLocaleString()
    },
    varianceClass(v) {
      if (v == null || v === 0) return ''
      return v < 0 ? 'text-red-600' : 'text-blue-600'
    },
    itemStatusTone(status) {
      return { PENDING: 'gray', COUNTED: 'green', RECOUNT: 'orange' }[status] || 'gray'
    },

    async fetchWarehouses() {
      try {
        const res = await listWarehouses({ status: 'ACTIVE', limit: 100 })
        this.warehouses = res.data
      } catch {
        this.warehouses = []
      }
    },
    async onWarehouseChange() {
      this.filters.zoneId = ''
      this.zones = []
      if (!this.filters.warehouseId) return
      try {
        const res = await listZones(this.filters.warehouseId)
        this.zones = res.data
      } catch {
        this.zones = []
      }
    },

    applyPreset(key) {
      this.activePreset = key
      const preset = VIEW_PRESETS.find((p) => p.key === key)
      this.filters.varianceOnly = !!preset.filters.varianceOnly
      this.filters.itemStatus = preset.filters.itemStatus || ''
      this.filters.hasAdjustment = preset.filters.hasAdjustment
      this.applyFilters()
    },

    async applyFilters() {
      this.page = 1
      await Promise.all([this.fetchRows(), this.fetchSummary()])
    },
    resetFilters() {
      this.dateRange = { preset: 'last30Days', from: null, to: null }
      this.filters = { warehouseId: '', zoneId: '', status: '', search: '', varianceOnly: false, itemStatus: '', hasAdjustment: undefined }
      this.zones = []
      this.activePreset = 'detail'
      this.applyFilters()
    },

    async fetchRows() {
      this.loading = true
      this.loadError = ''
      try {
        const res = await listStockCountReport(this.appliedQuery, {
          page: this.page, limit: this.limit, sortBy: this.sortBy, sortDir: this.sortDir,
        })
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
        const res = await getStockCountReportSummary(this.appliedQuery)
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
      await exportStockCountReportCsv(this.appliedQuery)
    },
    async onExportExcel() {
      await exportStockCountReportXlsx(this.appliedQuery)
    },
    async onDownloadPdf() {
      await downloadStockCountReportPdf(this.appliedQuery)
    },
    async onPreviewPdf() {
      const blob = await previewStockCountReportPdf(this.appliedQuery)
      this.previewUrl = URL.createObjectURL(blob)
      this.showPreview = true
    },
    onPrint() {
      window.print()
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
.field-select:disabled {
  background: #f1f5f9;
  cursor: not-allowed;
}
.table-td {
  padding: 10px 16px;
  font-size: 13px;
  color: #334155;
  white-space: nowrap;
  border-bottom: 1px solid #f1f5f9;
}
.preset-chip {
  padding: 6px 12px;
  border-radius: 999px;
  font-size: 13px;
  font-weight: 500;
  background: #f1f5f9;
  color: #475569;
  border: 1px solid transparent;
  transition: background-color 0.15s, color 0.15s;
}
.preset-chip:hover {
  background: #e2e8f0;
}
.preset-chip-active {
  background: #fff7ed;
  color: #c2410c;
  border-color: #fdba74;
  font-weight: 600;
}
</style>
