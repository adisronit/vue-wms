<template>
  <div>
    <PageHeader :title="$t('reportsPickingKpi.title')" :subtitle="$t('reports.sectionTitle')" />

    <div v-if="loadError" class="bg-red-50 border border-red-200 text-red-700 rounded-2xl p-4 mb-6">
      {{ loadError }}
    </div>

    <div class="bg-orange-50 border border-orange-200 text-orange-800 rounded-2xl p-4 mb-6 text-sm">
      {{ $t('reportsPickingKpi.unsupportedNote') }}
    </div>

    <div class="space-y-6">
      <ReportFilterBar v-model="dateRange" :cols="2" :loading="loading" @apply="applyFilters" @reset="resetFilters">
        <label class="text-sm">
          <span class="block text-xs font-semibold text-slate-500 mb-1">{{ $t('common.labels.warehouse') }}</span>
          <select v-model="filters.warehouseId" class="field-select">
            <option value="">{{ $t('common.labels.selectOption') }}</option>
            <option v-for="wh in warehouses" :key="wh.id" :value="wh.id">{{ wh.name }}</option>
          </select>
        </label>
      </ReportFilterBar>

      <ReportSummaryCards :cards="summaryCards" :loading="loading" />

      <div class="grid grid-cols-1 lg:grid-cols-2 gap-4">
        <div class="bg-white rounded-2xl border border-slate-200 p-4">
          <h3 class="text-sm font-semibold text-slate-600 mb-2">{{ $t('reportsPickingKpi.charts.dailyTrend') }}</h3>
          <ReportChart type="line" :chart-data="dailyTrendChartData" />
        </div>
        <div class="bg-white rounded-2xl border border-slate-200 p-4">
          <h3 class="text-sm font-semibold text-slate-600 mb-2">{{ $t('reportsPickingKpi.charts.cycleTimeDistribution') }}</h3>
          <ReportChart type="bar" :chart-data="cycleTimeChartData" />
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

      <div class="bg-white rounded-2xl border border-slate-200 overflow-hidden">
        <h3 class="text-sm font-semibold text-slate-600 px-4 pt-4">{{ $t('reportsPickingKpi.tables.byOperator') }}</h3>
        <table class="w-full text-sm mt-2">
          <thead class="bg-slate-50 text-slate-500 text-xs uppercase">
            <tr>
              <th class="table-th">{{ $t('reportsPickingKpi.table.operator') }}</th>
              <th class="table-th text-right">{{ $t('reportsPicking.kpi.totalTasks') }}</th>
              <th class="table-th text-right">{{ $t('reportsPicking.kpi.completedTasks') }}</th>
              <th class="table-th text-right">{{ $t('reportsPickingKpi.table.linesPerHour') }}</th>
              <th class="table-th text-right">{{ $t('reportsPickingKpi.table.unitsPerHour') }}</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-slate-100">
            <tr v-if="!summary || summary.by_operator.length === 0">
              <td class="table-td" colspan="5">{{ $t('reports.table.noData') }}</td>
            </tr>
            <tr v-for="row in summary ? summary.by_operator : []" :key="row.key" class="hover:bg-slate-50">
              <td class="table-td">{{ row.key }}</td>
              <td class="table-td text-right">{{ row.task_count }}</td>
              <td class="table-td text-right">{{ row.completed_task_count }}</td>
              <td class="table-td text-right">{{ row.lines_per_hour != null ? row.lines_per_hour.toFixed(2) : $t('reportsReceiving.kpi.notAvailable') }}</td>
              <td class="table-td text-right">{{ row.units_per_hour != null ? row.units_per_hour.toFixed(2) : $t('reportsReceiving.kpi.notAvailable') }}</td>
            </tr>
          </tbody>
        </table>
      </div>

      <div class="bg-white rounded-2xl border border-slate-200 overflow-hidden">
        <h3 class="text-sm font-semibold text-slate-600 px-4 pt-4">{{ $t('reportsPickingKpi.tables.byPickingMode') }}</h3>
        <table class="w-full text-sm mt-2">
          <thead class="bg-slate-50 text-slate-500 text-xs uppercase">
            <tr>
              <th class="table-th">{{ $t('reportsPicking.table.pickingMode') }}</th>
              <th class="table-th text-right">{{ $t('reportsPicking.kpi.totalTasks') }}</th>
              <th class="table-th text-right">{{ $t('reportsPickingKpi.table.completionRate') }}</th>
              <th class="table-th text-right">{{ $t('reportsPickingKpi.table.avgCycleTime') }}</th>
              <th class="table-th text-right">{{ $t('reportsPicking.kpi.shortPickRate') }}</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-slate-100">
            <tr v-if="!summary || summary.by_picking_mode.length === 0">
              <td class="table-td" colspan="5">{{ $t('reports.table.noData') }}</td>
            </tr>
            <tr v-for="row in summary ? summary.by_picking_mode : []" :key="row.picking_mode" class="hover:bg-slate-50">
              <td class="table-td">{{ row.picking_mode }}</td>
              <td class="table-td text-right">{{ row.task_count }}</td>
              <td class="table-td text-right">{{ row.completion_rate_pct != null ? row.completion_rate_pct.toFixed(1) + '%' : $t('reportsReceiving.kpi.notAvailable') }}</td>
              <td class="table-td text-right">{{ row.avg_cycle_time_hours != null ? row.avg_cycle_time_hours.toFixed(2) + ' h' : $t('reportsReceiving.kpi.notAvailable') }}</td>
              <td class="table-td text-right">{{ row.short_pick_rate_pct != null ? row.short_pick_rate_pct.toFixed(1) + '%' : $t('reportsReceiving.kpi.notAvailable') }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <ReportPrintPreview v-model="showPreview" :pdf-url="previewUrl" />
  </div>
</template>

<script>
import PageHeader from '../../components/ui/PageHeader.vue'
import ReportFilterBar from '../../components/reports/ReportFilterBar.vue'
import ReportSummaryCards from '../../components/reports/ReportSummaryCards.vue'
import ReportChart from '../../components/reports/ReportChart.vue'
import ReportExportToolbar from '../../components/reports/ReportExportToolbar.vue'
import ReportPrintPreview from '../../components/reports/ReportPrintPreview.vue'
import { listWarehouses } from '../../services/warehouses'
import {
  getPickingKpiSummary,
  exportPickingKpiCsv,
  exportPickingKpiXlsx,
  previewPickingKpiPdf,
  downloadPickingKpiPdf,
} from '../../services/reportsPickingKpi'
import { resolveDateRange, toRangeISO } from '../../utils/reportDateRange'

export default {
  name: 'PickingKpiView',

  components: { PageHeader, ReportFilterBar, ReportSummaryCards, ReportChart, ReportExportToolbar, ReportPrintPreview },

  data() {
    return {
      dateRange: { preset: 'last30Days', from: null, to: null },
      filters: { warehouseId: '' },
      warehouses: [],

      summary: null,
      loading: false,
      loadError: '',
      generatedAt: '',

      showPreview: false,
      previewUrl: null,
    }
  },

  computed: {
    appliedQuery() {
      const { from, to } = toRangeISO(resolveDateRange(this.dateRange.preset, this.dateRange))
      return { from, to, warehouseId: this.filters.warehouseId || undefined }
    },
    summaryCards() {
      if (!this.summary) return []
      const s = this.summary
      const na = this.$t('reportsReceiving.kpi.notAvailable')
      const pct = (v) => (v != null ? `${v.toFixed(1)}%` : na)
      const num = (v) => (v != null ? v.toFixed(2) : na)
      return [
        { key: 'completion', label: this.$t('reportsPicking.kpi.totalTasks'), value: `${s.total_tasks} (${s.completed_tasks} ${this.$t('reportsPicking.kpi.completedTasks').toLowerCase()})` },
        { key: 'completionRate', label: this.$t('reportsPickingKpi.table.completionRate'), value: pct(s.completion_rate_pct) },
        { key: 'linesPerHour', label: this.$t('reportsPickingKpi.kpi.linesPerHour'), value: num(s.lines_per_hour) },
        { key: 'unitsPerHour', label: this.$t('reportsPickingKpi.kpi.unitsPerHour'), value: num(s.units_per_hour) },
        { key: 'accuracy', label: this.$t('reportsPickingKpi.kpi.accuracy'), value: pct(s.accuracy.rate_pct), hint: `${s.accuracy.numerator}/${s.accuracy.denominator} ${this.$t('reportsPickingKpi.kpi.scansLabel')}` },
        { key: 'onTime', label: this.$t('reportsPickingKpi.kpi.onTimeRate'), value: pct(s.on_time.rate_pct), hint: `${s.on_time.numerator}/${s.on_time.denominator}, ${s.tasks_without_due_date} ${this.$t('reportsPickingKpi.kpi.noDueDateLabel')}` },
        { key: 'cycleTime', label: this.$t('reportsPickingKpi.kpi.avgCycleTime'), value: num(s.avg_cycle_time_hours) + ' h' },
        { key: 'shortRate', label: this.$t('reportsPicking.kpi.shortPickRate'), value: pct(s.short_pick_rate_pct) },
        { key: 'repickRate', label: this.$t('reportsPickingKpi.kpi.repickRate'), value: pct(s.repick_rate_pct) },
      ]
    },
    dailyTrendChartData() {
      const trend = this.summary?.charts?.daily_trend || []
      return {
        labels: trend.map((t) => t.date),
        datasets: [
          { label: this.$t('reportsPickingKpi.table.linesPerHour'), data: trend.map((t) => t.picked_lines), borderColor: '#ea580c', backgroundColor: '#fed7aa' },
        ],
      }
    },
    cycleTimeChartData() {
      const buckets = this.summary?.charts?.cycle_time_distribution || []
      return { labels: buckets.map((b) => b.label), datasets: [{ label: this.$t('reportsPickingKpi.charts.cycleTimeDistribution'), data: buckets.map((b) => b.count), backgroundColor: '#fb923c' }] }
    },
  },

  async mounted() {
    await this.fetchWarehouses()
    await this.applyFilters()
  },

  methods: {
    async fetchWarehouses() {
      try {
        const res = await listWarehouses({ status: 'ACTIVE', limit: 100 })
        this.warehouses = res.data
      } catch {
        this.warehouses = []
      }
    },
    async applyFilters() {
      this.loading = true
      this.loadError = ''
      try {
        const res = await getPickingKpiSummary(this.appliedQuery)
        this.summary = res.data
        this.generatedAt = new Date().toLocaleString()
      } catch (err) {
        this.loadError = err.message || this.$t('reports.table.noData')
        this.summary = null
      } finally {
        this.loading = false
      }
    },
    resetFilters() {
      this.dateRange = { preset: 'last30Days', from: null, to: null }
      this.filters = { warehouseId: '' }
      this.applyFilters()
    },
    async onExportCsv() {
      await exportPickingKpiCsv(this.appliedQuery)
    },
    async onExportExcel() {
      await exportPickingKpiXlsx(this.appliedQuery)
    },
    async onDownloadPdf() {
      await downloadPickingKpiPdf(this.appliedQuery)
    },
    async onPreviewPdf() {
      const blob = await previewPickingKpiPdf(this.appliedQuery)
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
.field-select {
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
