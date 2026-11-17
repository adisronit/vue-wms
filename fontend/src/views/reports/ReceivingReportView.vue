<template>
  <div>
    <PageHeader :title="$t('reportsReceiving.title')" :subtitle="$t('reports.sectionTitle')" />

    <div v-if="loadError" class="bg-red-50 border border-red-200 text-red-700 rounded-2xl p-4 mb-6">
      {{ loadError }}
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
          <span class="block text-xs font-semibold text-slate-500 mb-1">{{ $t('reportsReceiving.filters.supplier') }}</span>
          <select v-model="filters.supplierId" class="field-select">
            <option value="">{{ $t('common.labels.selectOption') }}</option>
            <option v-for="sup in suppliers" :key="sup.id" :value="sup.id">{{ sup.company_name }}</option>
          </select>
        </label>
        <label class="text-sm">
          <span class="block text-xs font-semibold text-slate-500 mb-1">{{ $t('common.labels.status') }}</span>
          <select v-model="filters.status" class="field-select">
            <option value="">{{ $t('common.labels.selectOption') }}</option>
            <option v-for="s in receiptStatuses" :key="s" :value="s">{{ s }}</option>
          </select>
        </label>
        <label class="text-sm">
          <span class="block text-xs font-semibold text-slate-500 mb-1">{{ $t('reportsReceiving.filters.search') }}</span>
          <input v-model="filters.search" type="text" class="field-input" :placeholder="$t('reportsReceiving.filters.searchPlaceholder')" />
        </label>
        <label class="text-sm">
          <span class="block text-xs font-semibold text-slate-500 mb-1">{{ $t('reportsReceiving.filters.lotNo') }}</span>
          <input v-model="filters.lotNo" type="text" class="field-input" />
        </label>
      </ReportFilterBar>

      <ReportSummaryCards :cards="summaryCards" :loading="summaryLoading" />

      <div class="grid grid-cols-1 lg:grid-cols-2 gap-4">
        <div class="bg-white rounded-2xl border border-slate-200 p-4">
          <h3 class="text-sm font-semibold text-slate-600 mb-2">{{ $t('reportsReceiving.charts.trend') }}</h3>
          <ReportChart type="line" :chart-data="trendChartData" />
        </div>
        <div class="bg-white rounded-2xl border border-slate-200 p-4">
          <h3 class="text-sm font-semibold text-slate-600 mb-2">{{ $t('reportsReceiving.charts.bySupplier') }}</h3>
          <ReportChart type="bar" :chart-data="bySupplierChartData" />
        </div>
        <div class="bg-white rounded-2xl border border-slate-200 p-4">
          <h3 class="text-sm font-semibold text-slate-600 mb-2">{{ $t('reportsReceiving.charts.byWarehouse') }}</h3>
          <ReportChart type="bar" :chart-data="byWarehouseChartData" />
        </div>
        <div class="bg-white rounded-2xl border border-slate-200 p-4">
          <h3 class="text-sm font-semibold text-slate-600 mb-2">{{ $t('reportsReceiving.charts.byOperator') }}</h3>
          <ReportChart type="bar" :chart-data="byOperatorChartData" />
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
        :colspan="12"
        min-width="1400px"
        :page="page"
        :limit="limit"
        :total="total"
        @update:page="onPageChange"
      >
        <template #head>
          <tr>
            <SortableTh column="receipt_no" :sort-by="sortBy" :sort-dir="sortDir" @sort="toggleSort">{{ $t('reportsReceiving.table.receiptNo') }}</SortableTh>
            <SortableTh column="receiving_date" :sort-by="sortBy" :sort-dir="sortDir" @sort="toggleSort">{{ $t('reportsReceiving.table.receivingDate') }}</SortableTh>
            <th class="table-th">{{ $t('reportsReceiving.filters.supplier') }}</th>
            <th class="table-th">{{ $t('common.labels.warehouse') }}</th>
            <th class="table-th">{{ $t('reportsReceiving.table.sku') }}</th>
            <th class="table-th text-right">{{ $t('reportsReceiving.table.expectedQty') }}</th>
            <th class="table-th text-right">{{ $t('reportsReceiving.table.receivedQty') }}</th>
            <th class="table-th text-right">{{ $t('reportsReceiving.table.shortOverQty') }}</th>
            <th class="table-th">{{ $t('reportsReceiving.table.lotNo') }}</th>
            <SortableTh column="status" :sort-by="sortBy" :sort-dir="sortDir" @sort="toggleSort">{{ $t('reportsReceiving.table.receivingStatus') }}</SortableTh>
            <th class="table-th">{{ $t('reportsReceiving.table.qcStatus') }}</th>
            <th class="table-th">{{ $t('reportsReceiving.table.putawayStatus') }}</th>
          </tr>
        </template>

        <tr v-for="row in rows" :key="`${row.receipt_item_id}-${row.split_id ?? 'none'}`" class="hover:bg-slate-50">
          <td class="table-td">{{ row.receipt_no }}</td>
          <td class="table-td">{{ formatDate(row.receiving_date) }}</td>
          <td class="table-td">{{ row.supplier_name || '-' }}</td>
          <td class="table-td">{{ row.warehouse_name }}</td>
          <td class="table-td">{{ row.sku }}</td>
          <td class="table-td text-right">{{ row.expected_qty }} {{ row.uom }}</td>
          <td class="table-td text-right">{{ row.received_qty }} {{ row.uom }}</td>
          <td class="table-td text-right" :class="row.short_over_qty < 0 ? 'text-red-600' : row.short_over_qty > 0 ? 'text-blue-600' : ''">
            {{ row.short_over_qty }}
          </td>
          <td class="table-td">{{ row.lot_no || '-' }}</td>
          <td class="table-td"><StatusBadge :tone="receivingTone(row.receiving_status)">{{ row.receiving_status }}</StatusBadge></td>
          <td class="table-td"><StatusBadge :tone="qcTone(row.qc_status)">{{ row.qc_status }}</StatusBadge></td>
          <td class="table-td"><StatusBadge :tone="putawayTone(row.putaway_status)">{{ row.putaway_status }}</StatusBadge></td>
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
  listReceivingReport,
  getReceivingReportSummary,
  exportReceivingReportCsv,
  exportReceivingReportXlsx,
  previewReceivingReportPdf,
  downloadReceivingReportPdf,
} from '../../services/reportsReceiving'
import { resolveDateRange, toRangeISO } from '../../utils/reportDateRange'

const RECEIPT_STATUSES = ['DRAFT', 'WAITING', 'PARTIALLY_RECEIVED', 'COMPLETED', 'CANCELLED']

export default {
  name: 'ReceivingReportView',

  components: {
    PageHeader, StatusBadge, SortableTh,
    ReportFilterBar, ReportSummaryCards, ReportDataTable, ReportChart, ReportExportToolbar, ReportPrintPreview,
  },

  data() {
    return {
      dateRange: { preset: 'last30Days', from: null, to: null },
      filters: { warehouseId: '', supplierId: '', status: '', search: '', lotNo: '' },
      warehouses: [],
      suppliers: [],
      receiptStatuses: RECEIPT_STATUSES,

      rows: [],
      total: 0,
      page: 1,
      limit: 20,
      sortBy: 'receiving_date',
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
        status: this.filters.status || undefined,
        search: this.filters.search || undefined,
        lotNo: this.filters.lotNo || undefined,
      }
    },
    summaryCards() {
      if (!this.summary) return []
      const s = this.summary
      const uomLine = (s.total_received_qty_by_uom || []).map((u) => `${u.qty} ${u.uom}`).join(', ') || '0'
      return [
        { key: 'total', label: this.$t('reportsReceiving.kpi.totalReceipts'), value: String(s.total_receipts) },
        { key: 'qty', label: this.$t('reportsReceiving.kpi.totalReceivedQty'), value: uomLine },
        { key: 'completion', label: this.$t('reportsReceiving.kpi.completionRate'), value: s.completion_rate != null ? `${s.completion_rate.toFixed(1)}%` : this.$t('reportsReceiving.kpi.notAvailable') },
        { key: 'lead', label: this.$t('reportsReceiving.kpi.leadTime'), value: s.avg_receiving_lead_time_hours != null ? `${s.avg_receiving_lead_time_hours.toFixed(1)} h` : this.$t('reportsReceiving.kpi.notAvailable') },
        { key: 'pendingQc', label: this.$t('reportsReceiving.kpi.pendingQc'), value: String(s.pending_qc_qty) },
        { key: 'pendingPutaway', label: this.$t('reportsReceiving.kpi.pendingPutaway'), value: String(s.pending_putaway_qty) },
      ]
    },
    trendChartData() {
      const trend = this.summary?.charts?.trend || []
      return { labels: trend.map((t) => t.date), datasets: [{ label: this.$t('reportsReceiving.kpi.totalReceivedQty'), data: trend.map((t) => t.received_qty), borderColor: '#ea580c', backgroundColor: '#fed7aa' }] }
    },
    bySupplierChartData() {
      return this.namedCountChartData(this.summary?.charts?.by_supplier)
    },
    byWarehouseChartData() {
      return this.namedCountChartData(this.summary?.charts?.by_warehouse)
    },
    byOperatorChartData() {
      return this.namedCountChartData(this.summary?.charts?.by_operator)
    },
  },

  async mounted() {
    await Promise.all([this.fetchWarehouses(), this.fetchSuppliers()])
    await this.applyFilters()
  },

  methods: {
    namedCountChartData(buckets) {
      const list = buckets || []
      return { labels: list.map((b) => b.name), datasets: [{ label: this.$t('reportsReceiving.kpi.totalReceivedQty'), data: list.map((b) => b.received_qty), backgroundColor: '#fb923c' }] }
    },
    formatDate(value) {
      if (!value) return '-'
      return new Date(value).toLocaleString()
    },
    receivingTone(status) {
      return { DRAFT: 'gray', WAITING: 'blue', PARTIALLY_RECEIVED: 'orange', COMPLETED: 'green', CANCELLED: 'red' }[status] || 'gray'
    },
    qcTone(status) {
      return { NOT_REQUIRED: 'gray', PENDING: 'blue', PARTIAL: 'orange', PASSED: 'green', REJECTED: 'red', MIXED: 'orange' }[status] || 'gray'
    },
    putawayTone(status) {
      return { NOT_STARTED: 'gray', PENDING: 'blue', IN_PROGRESS: 'orange', COMPLETED: 'green', ISSUE: 'red' }[status] || 'gray'
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
      this.filters = { warehouseId: '', supplierId: '', status: '', search: '', lotNo: '' }
      this.applyFilters()
    },

    async fetchRows() {
      this.loading = true
      this.loadError = ''
      try {
        const res = await listReceivingReport(this.appliedQuery, {
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
        const res = await getReceivingReportSummary(this.appliedQuery)
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
      await exportReceivingReportCsv(this.appliedQuery)
    },
    async onExportExcel() {
      await exportReceivingReportXlsx(this.appliedQuery)
    },
    async onDownloadPdf() {
      await downloadReceivingReportPdf(this.appliedQuery)
    },
    async onPreviewPdf() {
      const blob = await previewReceivingReportPdf(this.appliedQuery)
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
