<template>
  <div>
    <PageHeader :title="$t('reportsQc.title')" :subtitle="$t('reports.sectionTitle')" />

    <div v-if="loadError" class="bg-red-50 border border-red-200 text-red-700 rounded-2xl p-4 mb-6">
      {{ loadError }}
    </div>

    <div class="bg-orange-50 border border-orange-200 text-orange-800 rounded-2xl p-4 mb-6 text-sm">
      {{ $t('reportsQc.grainNote') }}
    </div>

    <div class="space-y-6">
      <ReportFilterBar v-model="dateRange" :cols="5" :loading="loading" @apply="applyFilters" @reset="resetFilters">
        <label class="text-sm">
          <span class="block text-xs font-semibold text-slate-500 mb-1">{{ $t('common.labels.warehouse') }}</span>
          <select v-model="filters.warehouseId" class="field-select">
            <option value="">{{ $t('common.labels.selectOption') }}</option>
            <option v-for="wh in warehouses" :key="wh.id" :value="wh.id">{{ wh.name }}</option>
          </select>
        </label>
        <label class="text-sm">
          <span class="block text-xs font-semibold text-slate-500 mb-1">{{ $t('reportsQc.filters.supplier') }}</span>
          <select v-model="filters.supplierId" class="field-select">
            <option value="">{{ $t('common.labels.selectOption') }}</option>
            <option v-for="sup in suppliers" :key="sup.id" :value="sup.id">{{ sup.company_name }}</option>
          </select>
        </label>
        <label class="text-sm">
          <span class="block text-xs font-semibold text-slate-500 mb-1">{{ $t('reportsQc.filters.qcType') }}</span>
          <select v-model="filters.qcType" class="field-select">
            <option value="">{{ $t('common.labels.selectOption') }}</option>
            <option v-for="t in qcTypes" :key="t" :value="t">{{ t }}</option>
          </select>
        </label>
        <label class="text-sm">
          <span class="block text-xs font-semibold text-slate-500 mb-1">{{ $t('reportsQc.filters.resolutionStatus') }}</span>
          <select v-model="filters.resolutionStatus" class="field-select">
            <option value="">{{ $t('common.labels.selectOption') }}</option>
            <option v-for="s in resolutionStatuses" :key="s" :value="s">{{ s }}</option>
          </select>
        </label>
        <label class="text-sm">
          <span class="block text-xs font-semibold text-slate-500 mb-1">{{ $t('reportsQc.filters.search') }}</span>
          <input v-model="filters.search" type="text" class="field-input" :placeholder="$t('reportsQc.filters.searchPlaceholder')" />
        </label>
        <label class="text-sm">
          <span class="block text-xs font-semibold text-slate-500 mb-1">{{ $t('reportsQc.filters.defectCode') }}</span>
          <input v-model="filters.defectCode" type="text" class="field-input" />
        </label>
      </ReportFilterBar>

      <ReportSummaryCards :cards="summaryCards" :loading="summaryLoading" />

      <div class="grid grid-cols-1 lg:grid-cols-2 gap-4">
        <div class="bg-white rounded-2xl border border-slate-200 p-4">
          <h3 class="text-sm font-semibold text-slate-600 mb-2">{{ $t('reportsQc.charts.topDefects') }}</h3>
          <ReportChart type="bar" :chart-data="topDefectsChartData" />
        </div>
        <div class="bg-white rounded-2xl border border-slate-200 p-4">
          <h3 class="text-sm font-semibold text-slate-600 mb-2">{{ $t('reportsQc.charts.supplierRejectionTrend') }}</h3>
          <ReportChart type="line" :chart-data="supplierRejectionTrendChartData" />
        </div>
        <div class="bg-white rounded-2xl border border-slate-200 p-4">
          <h3 class="text-sm font-semibold text-slate-600 mb-2">{{ $t('reportsQc.charts.byQcType') }}</h3>
          <ReportChart type="bar" :chart-data="byQcTypeChartData" />
        </div>
      </div>

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
        :colspan="14"
        min-width="1700px"
        :page="page"
        :limit="limit"
        :total="total"
        @update:page="onPageChange"
      >
        <template #head>
          <tr>
            <SortableTh column="inspection_date" :sort-by="sortBy" :sort-dir="sortDir" @sort="toggleSort">{{ $t('reportsQc.table.inspectionNo') }}</SortableTh>
            <th class="table-th">{{ $t('reportsQc.table.receiptNo') }}</th>
            <th class="table-th">{{ $t('common.labels.warehouse') }}</th>
            <th class="table-th">{{ $t('reportsQc.filters.supplier') }}</th>
            <th class="table-th">{{ $t('reportsQc.table.sku') }}</th>
            <th class="table-th">{{ $t('reportsQc.table.lotNo') }}</th>
            <th class="table-th">{{ $t('reportsQc.table.location') }}</th>
            <th class="table-th text-right">{{ $t('reportsQc.table.heldQty') }}</th>
            <th class="table-th text-right">{{ $t('reportsQc.table.passQty') }}</th>
            <th class="table-th text-right">{{ $t('reportsQc.table.rejectQty') }}</th>
            <th class="table-th text-right">{{ $t('reportsQc.table.remainingHold') }}</th>
            <SortableTh column="resolution_status" :sort-by="sortBy" :sort-dir="sortDir" @sort="toggleSort">{{ $t('reportsQc.table.resolutionStatus') }}</SortableTh>
            <th class="table-th">{{ $t('reportsQc.table.defectCode') }}</th>
            <th class="table-th text-right">{{ $t('reportsQc.table.turnaround') }} / {{ $t('reportsQc.table.holdAging') }}</th>
          </tr>
        </template>

        <tr v-for="row in rows" :key="row.inspection_item_id" class="hover:bg-slate-50">
          <td class="table-td">{{ row.inspection_no }}</td>
          <td class="table-td">{{ row.receipt_no }}</td>
          <td class="table-td">{{ row.warehouse_name }}</td>
          <td class="table-td">{{ row.supplier_name || '-' }}</td>
          <td class="table-td">{{ row.sku }}</td>
          <td class="table-td">{{ row.lot_no || '-' }}</td>
          <td class="table-td">{{ row.location_code || '-' }}</td>
          <td class="table-td text-right">{{ row.held_qty_snapshot }}</td>
          <td class="table-td text-right">{{ row.pass_qty }}</td>
          <td class="table-td text-right">{{ row.reject_qty }}</td>
          <td class="table-td text-right">{{ row.remaining_hold_qty }}</td>
          <td class="table-td"><StatusBadge :tone="resolutionTone(row.resolution_status)">{{ row.resolution_status }}</StatusBadge></td>
          <td class="table-td">{{ row.defect_code || '-' }}</td>
          <td class="table-td text-right">
            <span v-if="row.qc_turnaround_hours != null">{{ row.qc_turnaround_hours.toFixed(1) }}</span>
            <span v-else-if="row.hold_aging_hours != null">{{ row.hold_aging_hours.toFixed(1) }}</span>
            <span v-else>-</span>
          </td>
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
import ReportChart from '../../components/reports/ReportChart.vue'
import ReportExportToolbar from '../../components/reports/ReportExportToolbar.vue'
import ReportPrintPreview from '../../components/reports/ReportPrintPreview.vue'
import { listWarehouses } from '../../services/warehouses'
import { listSuppliers } from '../../services/suppliers'
import {
  listQCReport,
  getQCReportSummary,
  exportQCReportCsv,
  exportQCReportXlsx,
  previewQCReportPdf,
  downloadQCReportPdf,
} from '../../services/reportsQC'
import { resolveDateRange, toRangeISO } from '../../utils/reportDateRange'

const QC_TYPES = ['NORMAL', 'FULL', 'RANDOM', 'DAMAGE']
const RESOLUTION_STATUSES = ['PENDING', 'PARTIAL', 'MIXED', 'PASSED', 'REJECTED']
const CHART_COLORS = ['#ea580c', '#2563eb', '#16a34a', '#db2777', '#7c3aed', '#0891b2']

export default {
  name: 'QCReportView',

  components: {
    PageHeader, StatusBadge, SortableTh,
    ReportFilterBar, ReportSummaryCards, ReportDataTable, ReportChart, ReportExportToolbar, ReportPrintPreview,
  },

  data() {
    return {
      dateRange: { preset: 'last30Days', from: null, to: null },
      filters: { warehouseId: '', supplierId: '', qcType: '', resolutionStatus: '', search: '', defectCode: '' },
      warehouses: [],
      suppliers: [],
      qcTypes: QC_TYPES,
      resolutionStatuses: RESOLUTION_STATUSES,

      rows: [],
      total: 0,
      page: 1,
      limit: 20,
      sortBy: 'inspection_date',
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
        supplierId: this.filters.supplierId || undefined,
        qcType: this.filters.qcType || undefined,
        resolutionStatus: this.filters.resolutionStatus || undefined,
        search: this.filters.search || undefined,
        defectCode: this.filters.defectCode || undefined,
      }
    },
    summaryCards() {
      if (!this.summary) return []
      const s = this.summary
      const na = this.$t('reportsQc.kpi.notAvailable')
      const pct = (v) => (v != null ? `${v.toFixed(1)}%` : na)
      const num = (v) => (v != null ? v.toFixed(2) : na)
      return [
        { key: 'total', label: this.$t('reportsQc.kpi.totalItems'), value: String(s.total_inspection_items) },
        { key: 'pass', label: this.$t('reportsQc.kpi.passRate'), value: pct(s.pass_rate.rate_pct), hint: `${s.pass_rate.numerator}/${s.pass_rate.denominator}` },
        { key: 'reject', label: this.$t('reportsQc.kpi.rejectionRate'), value: pct(s.rejection_rate.rate_pct), hint: `${s.rejection_rate.numerator}/${s.rejection_rate.denominator}` },
        { key: 'hold', label: this.$t('reportsQc.kpi.holdRate'), value: pct(s.hold_rate.rate_pct), hint: `${s.hold_rate.numerator}/${s.hold_rate.denominator}` },
        { key: 'pendingNotStarted', label: this.$t('reportsQc.kpi.pendingNotStarted'), value: `${s.pending_qc_not_started_count} / ${s.pending_qc_not_started_qty}` },
        { key: 'pendingAwaiting', label: this.$t('reportsQc.kpi.pendingAwaitingDecision'), value: `${s.pending_qc_awaiting_decision_count} / ${s.pending_qc_awaiting_decision_qty}` },
        { key: 'turnaround', label: this.$t('reportsQc.kpi.avgTurnaround'), value: num(s.avg_turnaround_hours) + ' h' },
        { key: 'holdAging', label: this.$t('reportsQc.kpi.avgHoldAging'), value: num(s.avg_hold_aging_hours) + ' h' },
        { key: 'maxHoldAging', label: this.$t('reportsQc.kpi.maxHoldAging'), value: num(s.max_hold_aging_hours) + ' h' },
      ]
    },
    topDefectsChartData() {
      const list = this.summary?.charts?.top_defects || []
      return { labels: list.map((d) => d.defect_code), datasets: [{ label: this.$t('reportsQc.charts.topDefects'), data: list.map((d) => d.qty), backgroundColor: '#fb923c' }] }
    },
    byQcTypeChartData() {
      const list = this.summary?.charts?.by_qc_type || []
      return { labels: list.map((b) => b.name), datasets: [{ label: this.$t('reportsQc.charts.byQcType'), data: list.map((b) => b.count), backgroundColor: '#60a5fa' }] }
    },
    supplierRejectionTrendChartData() {
      const points = this.summary?.charts?.supplier_rejection_trend || []
      const dates = [...new Set(points.map((p) => p.date))].sort()
      const suppliers = [...new Set(points.map((p) => p.supplier))]
      const datasets = suppliers.map((supplier, i) => {
        const bySupplier = new Map(points.filter((p) => p.supplier === supplier).map((p) => [p.date, p.reject_qty]))
        return {
          label: supplier,
          data: dates.map((d) => bySupplier.get(d) ?? 0),
          borderColor: CHART_COLORS[i % CHART_COLORS.length],
          backgroundColor: CHART_COLORS[i % CHART_COLORS.length],
        }
      })
      return { labels: dates, datasets }
    },
  },

  async mounted() {
    await Promise.all([this.fetchWarehouses(), this.fetchSuppliers()])
    await this.applyFilters()
  },

  methods: {
    resolutionTone(status) {
      return { PENDING: 'blue', PARTIAL: 'orange', MIXED: 'orange', PASSED: 'green', REJECTED: 'red' }[status] || 'gray'
    },

    async fetchWarehouses() {
      try {
        const res = await listWarehouses({ status: 'ACTIVE', limit: 100 })
        this.warehouses = res.data
      } catch {
        this.warehouses = []
      }
    },
    async fetchSuppliers() {
      try {
        const res = await listSuppliers({ status: 'ACTIVE', limit: 100 })
        this.suppliers = res.data
      } catch {
        this.suppliers = []
      }
    },

    async applyFilters() {
      this.page = 1
      await Promise.all([this.fetchRows(), this.fetchSummary()])
    },
    resetFilters() {
      this.dateRange = { preset: 'last30Days', from: null, to: null }
      this.filters = { warehouseId: '', supplierId: '', qcType: '', resolutionStatus: '', search: '', defectCode: '' }
      this.applyFilters()
    },

    async fetchRows() {
      this.loading = true
      this.loadError = ''
      try {
        const res = await listQCReport(this.appliedQuery, {
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
        const res = await getQCReportSummary(this.appliedQuery)
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
      await exportQCReportCsv(this.appliedQuery)
    },
    async onExportExcel() {
      await exportQCReportXlsx(this.appliedQuery)
    },
    async onDownloadPdf() {
      await downloadQCReportPdf(this.appliedQuery)
    },
    async onPreviewPdf() {
      const blob = await previewQCReportPdf(this.appliedQuery)
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
.table-td {
  padding: 10px 16px;
  font-size: 13px;
  color: #334155;
  white-space: nowrap;
  border-bottom: 1px solid #f1f5f9;
}
</style>
