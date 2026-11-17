<template>
  <div>
    <PageHeader :title="$t('reportsDelivery.title')" :subtitle="$t('reports.sectionTitle')" />

    <div v-if="loadError" class="bg-red-50 border border-red-200 text-red-700 rounded-2xl p-4 mb-6">
      {{ loadError }}
    </div>

    <div class="bg-orange-50 border border-orange-200 text-orange-800 rounded-2xl p-4 mb-6 text-sm">
      {{ $t('reportsDelivery.deliveryCaveat') }}
    </div>

    <div class="flex gap-2 mb-4">
      <button
        v-for="t in tabs" :key="t.key"
        @click="activeTab = t.key"
        :class="['px-4 py-2 rounded-full text-sm font-semibold', activeTab === t.key ? 'bg-orange-600 text-white' : 'bg-slate-100 text-slate-600']"
      >{{ t.label }}</button>
    </div>

    <div v-show="activeTab === 'detail'" class="space-y-6">
      <ReportFilterBar v-model="dateRange" :cols="5" :loading="loading" @apply="applyFilters" @reset="resetFilters">
        <label class="text-sm">
          <span class="block text-xs font-semibold text-slate-500 mb-1">{{ $t('common.labels.warehouse') }}</span>
          <select v-model="filters.warehouseId" class="field-select">
            <option value="">{{ $t('common.labels.selectOption') }}</option>
            <option v-for="wh in warehouses" :key="wh.id" :value="wh.id">{{ wh.name }}</option>
          </select>
        </label>
        <label class="text-sm">
          <span class="block text-xs font-semibold text-slate-500 mb-1">{{ $t('reportsDelivery.filters.carrier') }}</span>
          <select v-model="filters.carrier" class="field-select">
            <option value="">{{ $t('common.labels.selectOption') }}</option>
            <option v-for="c in carriers" :key="c" :value="c">{{ c }}</option>
          </select>
        </label>
        <label class="text-sm">
          <span class="block text-xs font-semibold text-slate-500 mb-1">{{ $t('common.labels.status') }}</span>
          <select v-model="filters.status" class="field-select">
            <option value="">{{ $t('common.labels.selectOption') }}</option>
            <option v-for="s in statuses" :key="s" :value="s">{{ s }}</option>
          </select>
        </label>
        <label class="text-sm">
          <span class="block text-xs font-semibold text-slate-500 mb-1">{{ $t('reportsDelivery.filters.orderType') }}</span>
          <select v-model="filters.orderType" class="field-select">
            <option value="">{{ $t('common.labels.selectOption') }}</option>
            <option v-for="t in orderTypes" :key="t" :value="t">{{ $t('reportsDelivery.orderTypes.' + t) }}</option>
          </select>
        </label>
        <label class="text-sm">
          <span class="block text-xs font-semibold text-slate-500 mb-1">{{ $t('reportsDelivery.filters.search') }}</span>
          <input v-model="filters.search" type="text" class="field-input" :placeholder="$t('reportsDelivery.filters.searchPlaceholder')" />
        </label>
      </ReportFilterBar>

      <ReportSummaryCards :cards="summaryCards" :loading="summaryLoading" />

      <div class="grid grid-cols-1 lg:grid-cols-2 gap-4">
        <div class="bg-white rounded-2xl border border-slate-200 p-4">
          <h3 class="text-sm font-semibold text-slate-600 mb-2">{{ $t('reportsDelivery.charts.trend') }}</h3>
          <ReportChart type="line" :chart-data="trendChartData" />
        </div>
        <div class="bg-white rounded-2xl border border-slate-200 p-4">
          <h3 class="text-sm font-semibold text-slate-600 mb-2">{{ $t('reportsDelivery.charts.byStatus') }}</h3>
          <ReportChart type="bar" :chart-data="byStatusChartData" />
        </div>
        <div class="bg-white rounded-2xl border border-slate-200 p-4">
          <h3 class="text-sm font-semibold text-slate-600 mb-2">{{ $t('reportsDelivery.charts.byCarrier') }}</h3>
          <ReportChart type="bar" :chart-data="byCarrierChartData" />
        </div>
        <div class="bg-white rounded-2xl border border-slate-200 p-4">
          <h3 class="text-sm font-semibold text-slate-600 mb-2">{{ $t('reportsDelivery.charts.byWarehouse') }}</h3>
          <ReportChart type="bar" :chart-data="byWarehouseChartData" />
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
        :colspan="13"
        min-width="1700px"
        :page="page"
        :limit="limit"
        :total="total"
        @update:page="onPageChange"
      >
        <template #head>
          <tr>
            <SortableTh column="ship_date" :sort-by="sortBy" :sort-dir="sortDir" @sort="toggleSort">{{ $t('reportsDelivery.table.shipmentNo') }}</SortableTh>
            <th class="table-th">{{ $t('reportsDelivery.table.orderNo') }}</th>
            <th class="table-th">{{ $t('reportsDelivery.table.orderType') }}</th>
            <th class="table-th">{{ $t('reportsDelivery.table.customer') }}</th>
            <th class="table-th">{{ $t('common.labels.warehouse') }}</th>
            <th class="table-th">{{ $t('reportsDelivery.table.carrier') }}</th>
            <th class="table-th">{{ $t('reportsDelivery.table.trackingNo') }}</th>
            <th class="table-th">{{ $t('reportsDelivery.table.sku') }}</th>
            <th class="table-th text-right">{{ $t('reportsDelivery.table.qty') }}</th>
            <SortableTh column="status" :sort-by="sortBy" :sort-dir="sortDir" @sort="toggleSort">{{ $t('reportsDelivery.table.status') }}</SortableTh>
            <th class="table-th">{{ $t('reportsDelivery.table.dispatchedAt') }}</th>
            <th class="table-th">{{ $t('reportsDelivery.table.deliveredAt') }}</th>
            <th class="table-th text-right">{{ $t('reportsDelivery.table.leadTime') }}</th>
          </tr>
        </template>

        <tr v-for="row in rows" :key="`${row.shipment_id}-${row.shipment_item_id}`" class="hover:bg-slate-50">
          <td class="table-td">{{ row.shipment_no }}</td>
          <td class="table-td">{{ row.order_no || '-' }}</td>
          <td class="table-td">{{ $t('reportsDelivery.orderTypes.' + row.order_type) }}</td>
          <td class="table-td">{{ row.customer_name || '-' }}</td>
          <td class="table-td">{{ row.warehouse_name || '-' }}</td>
          <td class="table-td">{{ row.carrier || '-' }}</td>
          <td class="table-td">{{ row.tracking_no || '-' }}</td>
          <td class="table-td">{{ row.sku }}</td>
          <td class="table-td text-right">{{ row.qty }} {{ row.uom }}</td>
          <td class="table-td">
            <StatusBadge :tone="statusTone(row.status)">{{ row.status }}</StatusBadge>
            <span v-if="row.is_exception" class="ml-1 text-xs text-red-600" :title="row.exception_remark || ''">⚠</span>
          </td>
          <td class="table-td">{{ formatDate(row.dispatched_at) }}</td>
          <td class="table-td">{{ formatDate(row.delivered_at) }}</td>
          <td class="table-td text-right">{{ row.delivery_lead_time_hours != null ? row.delivery_lead_time_hours.toFixed(1) : '-' }}</td>
        </tr>
      </ReportDataTable>
    </div>

    <div v-show="activeTab === 'trips'" class="space-y-4">
      <div class="bg-white rounded-2xl border border-slate-200 p-4 flex flex-wrap items-end gap-4">
        <label class="text-sm">
          <span class="block text-xs font-semibold text-slate-500 mb-1">{{ $t('reportsDelivery.filters.carrier') }}</span>
          <select v-model="filters.carrier" class="field-select">
            <option value="">{{ $t('common.labels.selectOption') }}</option>
            <option v-for="c in carriers" :key="c" :value="c">{{ c }}</option>
          </select>
        </label>
        <label class="text-sm">
          <span class="block text-xs font-semibold text-slate-500 mb-1">{{ $t('reportsDelivery.filters.search') }}</span>
          <input v-model="filters.search" type="text" class="field-input" :placeholder="$t('reportsDelivery.filters.tripSearchPlaceholder')" />
        </label>
        <button @click="onExportTripsCsv" class="px-4 py-2 rounded-lg bg-orange-600 text-white text-sm font-semibold">
          {{ $t('reports.export.exportCsv') }}
        </button>
      </div>

      <div class="bg-white rounded-2xl border border-slate-200 p-4">
        <h3 class="text-sm font-semibold text-slate-600 mb-2">{{ $t('reportsDelivery.kpi.totalTrips') }}: {{ summary ? summary.total_trips : '-' }} ({{ summary ? summary.completed_trips : '-' }} {{ $t('common.status.completed') }})</h3>
        <p class="text-sm text-slate-500">{{ $t('reportsDelivery.kpi.avgLoadingDuration') }}: {{ summary && summary.avg_loading_duration_hours != null ? summary.avg_loading_duration_hours.toFixed(2) + ' h' : $t('reportsDelivery.kpi.notAvailable') }}</p>
      </div>
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
import {
  listDeliveryReport,
  getDeliveryReportSummary,
  exportDeliveryReportCsv,
  exportDeliveryReportXlsx,
  previewDeliveryReportPdf,
  downloadDeliveryReportPdf,
  exportLoadingTripsCsv,
} from '../../services/reportsDelivery'
import { resolveDateRange, toRangeISO } from '../../utils/reportDateRange'

const CARRIERS = ['DHL', 'KERRY', 'FLASH', 'INTERNAL']
const STATUSES = ['READY', 'DISPATCHED', 'IN_TRANSIT', 'DELIVERED', 'ISSUE', 'CANCELLED']
const ORDER_TYPES = ['SALES_ORDER', 'INTERNAL_TRANSFER']

export default {
  name: 'DeliveryReportView',

  components: {
    PageHeader, StatusBadge, SortableTh,
    ReportFilterBar, ReportSummaryCards, ReportDataTable, ReportChart, ReportExportToolbar, ReportPrintPreview,
  },

  data() {
    return {
      activeTab: 'detail',
      tabs: [
        { key: 'detail', label: this.$t('reportsDelivery.tabs.detail') },
        { key: 'trips', label: this.$t('reportsDelivery.tabs.trips') },
      ],

      dateRange: { preset: 'last30Days', from: null, to: null },
      filters: { warehouseId: '', carrier: '', status: '', orderType: '', search: '' },
      warehouses: [],
      carriers: CARRIERS,
      statuses: STATUSES,
      orderTypes: ORDER_TYPES,

      rows: [],
      total: 0,
      page: 1,
      limit: 20,
      sortBy: 'ship_date',
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
        carrier: this.filters.carrier || undefined,
        status: this.filters.status || undefined,
        orderType: this.filters.orderType || undefined,
        search: this.filters.search || undefined,
      }
    },
    summaryCards() {
      if (!this.summary) return []
      const s = this.summary
      const na = this.$t('reportsDelivery.kpi.notAvailable')
      const pct = (v) => (v != null ? `${v.toFixed(1)}%` : na)
      const num = (v) => (v != null ? v.toFixed(2) : na)
      return [
        { key: 'total', label: this.$t('reportsDelivery.kpi.totalShipments'), value: String(s.total_shipments) },
        { key: 'dispatch', label: this.$t('reportsDelivery.kpi.dispatchRate'), value: pct(s.dispatch_rate.rate_pct), hint: `${s.dispatch_rate.numerator}/${s.dispatch_rate.denominator}` },
        { key: 'delivery', label: this.$t('reportsDelivery.kpi.deliveryConfirmationRate'), value: pct(s.delivery_confirmation_rate.rate_pct), hint: `${s.delivery_confirmation_rate.numerator}/${s.delivery_confirmation_rate.denominator}` },
        { key: 'exception', label: this.$t('reportsDelivery.kpi.exceptionRate'), value: pct(s.exception_rate.rate_pct), hint: `${s.exception_rate.numerator}/${s.exception_rate.denominator}` },
        { key: 'leadTime', label: this.$t('reportsDelivery.kpi.avgLeadTime'), value: num(s.avg_delivery_lead_time_hours) + ' h' },
      ]
    },
    trendChartData() {
      const trend = this.summary?.charts?.trend || []
      return { labels: trend.map((t) => t.date), datasets: [{ label: this.$t('reportsDelivery.kpi.totalShipments'), data: trend.map((t) => t.shipment_count), borderColor: '#ea580c', backgroundColor: '#fed7aa' }] }
    },
    byStatusChartData() {
      return this.namedCountChartData(this.summary?.charts?.by_status)
    },
    byCarrierChartData() {
      return this.namedCountChartData(this.summary?.charts?.by_carrier)
    },
    byWarehouseChartData() {
      return this.namedCountChartData(this.summary?.charts?.by_warehouse)
    },
  },

  async mounted() {
    await this.fetchWarehouses()
    await this.applyFilters()
  },

  methods: {
    namedCountChartData(buckets) {
      const list = buckets || []
      return { labels: list.map((b) => b.name), datasets: [{ label: this.$t('reportsDelivery.kpi.totalShipments'), data: list.map((b) => b.count), backgroundColor: '#fb923c' }] }
    },
    formatDate(value) {
      if (!value) return '-'
      return new Date(value).toLocaleString()
    },
    statusTone(status) {
      if (status === 'DISPATCHED' || status === 'IN_TRANSIT') return 'blue'
      if (status === 'DELIVERED') return 'green'
      if (status === 'ISSUE' || status === 'CANCELLED') return 'red'
      return 'gray'
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
      this.dateRange = { preset: 'last30Days', from: null, to: null }
      this.filters = { warehouseId: '', carrier: '', status: '', orderType: '', search: '' }
      this.applyFilters()
    },

    async fetchRows() {
      this.loading = true
      this.loadError = ''
      try {
        const res = await listDeliveryReport(this.appliedQuery, {
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
        const res = await getDeliveryReportSummary(this.appliedQuery)
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
      await exportDeliveryReportCsv(this.appliedQuery)
    },
    async onExportExcel() {
      await exportDeliveryReportXlsx(this.appliedQuery)
    },
    async onDownloadPdf() {
      await downloadDeliveryReportPdf(this.appliedQuery)
    },
    async onPreviewPdf() {
      const blob = await previewDeliveryReportPdf(this.appliedQuery)
      this.previewUrl = URL.createObjectURL(blob)
      this.showPreview = true
    },
    onPrint() {
      window.print()
    },
    async onExportTripsCsv() {
      await exportLoadingTripsCsv(this.appliedQuery)
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
