<template>
  <div>
    <PageHeader :title="$t('reportsPicking.title')" :subtitle="$t('reports.sectionTitle')" />

    <div v-if="loadError" class="bg-red-50 border border-red-200 text-red-700 rounded-2xl p-4 mb-6">
      {{ loadError }}
    </div>

    <div class="flex gap-2 mb-6">
      <button
        type="button"
        class="tab-chip"
        :class="{ 'tab-chip-active': view === 'detail' }"
        @click="switchView('detail')"
      >
        {{ $t('reportsPicking.tabs.detail') }}
      </button>
      <button
        type="button"
        class="tab-chip"
        :class="{ 'tab-chip-active': view === 'batch' }"
        @click="switchView('batch')"
      >
        {{ $t('reportsPicking.tabs.batchAllocation') }}
      </button>
    </div>

    <div v-if="view === 'detail'" class="space-y-6">
      <ReportFilterBar v-model="dateRange" :cols="5" :loading="loading" @apply="applyFilters" @reset="resetFilters">
        <label class="text-sm">
          <span class="block text-xs font-semibold text-slate-500 mb-1">{{ $t('common.labels.warehouse') }}</span>
          <select v-model="filters.warehouseId" class="field-select">
            <option value="">{{ $t('common.labels.selectOption') }}</option>
            <option v-for="wh in warehouses" :key="wh.id" :value="wh.id">{{ wh.name }}</option>
          </select>
        </label>
        <label class="text-sm">
          <span class="block text-xs font-semibold text-slate-500 mb-1">{{ $t('common.labels.status') }}</span>
          <select v-model="filters.status" class="field-select">
            <option value="">{{ $t('common.labels.selectOption') }}</option>
            <option v-for="s in taskStatuses" :key="s" :value="s">{{ s }}</option>
          </select>
        </label>
        <label class="text-sm">
          <span class="block text-xs font-semibold text-slate-500 mb-1">{{ $t('reportsReceiving.filters.search') }}</span>
          <input v-model="filters.search" type="text" class="field-input" :placeholder="$t('reportsPicking.filters.searchPlaceholder')" />
        </label>
      </ReportFilterBar>

      <div class="flex flex-wrap gap-2">
        <button
          type="button"
          class="preset-chip"
          :class="{ 'preset-chip-active': filters.shortOnly }"
          @click="toggleShortOnly"
        >
          {{ $t('reportsPicking.presets.shortOnly') }}
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
        min-width="1600px"
        :page="page"
        :limit="limit"
        :total="total"
        @update:page="onPageChange"
      >
        <template #head>
          <tr>
            <SortableTh column="pick_no" :sort-by="sortBy" :sort-dir="sortDir" @sort="toggleSort">{{ $t('reportsPicking.table.pickNo') }}</SortableTh>
            <th class="table-th">{{ $t('reportsPicking.table.orderNo') }}</th>
            <th class="table-th">{{ $t('reportsPicking.table.pickingMode') }}</th>
            <th class="table-th">{{ $t('reportsPicking.table.customer') }}</th>
            <th class="table-th">{{ $t('common.labels.warehouse') }}</th>
            <th class="table-th">{{ $t('reportsReceiving.table.sku') }}</th>
            <th class="table-th">{{ $t('reportsStockCount.table.location') }}</th>
            <th class="table-th text-right">{{ $t('reportsPicking.table.requestedQty') }}</th>
            <th class="table-th text-right">{{ $t('reportsPicking.table.pickedQty') }}</th>
            <th class="table-th text-right">{{ $t('reportsPicking.table.shortQty') }}</th>
            <th class="table-th text-right">{{ $t('reportsPicking.table.dispatchedQty') }}</th>
            <SortableTh column="status" :sort-by="sortBy" :sort-dir="sortDir" @sort="toggleSort">{{ $t('reportsPicking.table.status') }}</SortableTh>
            <th class="table-th">{{ $t('reportsPicking.table.picker') }}</th>
          </tr>
        </template>

        <tr v-for="row in rows" :key="row.task_item_id" class="hover:bg-slate-50">
          <td class="table-td">{{ row.pick_no }}</td>
          <td class="table-td">{{ row.order_no || '-' }}</td>
          <td class="table-td">
            {{ row.picking_mode }}<span v-if="row.batch_no"> ({{ row.batch_no }})</span>
          </td>
          <td class="table-td">{{ row.customer_name || '-' }}</td>
          <td class="table-td">{{ row.warehouse_name }}</td>
          <td class="table-td">{{ row.sku }}</td>
          <td class="table-td">{{ row.location_code }}</td>
          <td class="table-td text-right">{{ row.requested_qty }} {{ row.uom }}</td>
          <td class="table-td text-right">{{ row.picked_qty }} {{ row.uom }}</td>
          <td class="table-td text-right" :class="row.short_qty ? 'text-red-600' : ''">{{ row.short_qty ?? '-' }}</td>
          <td class="table-td text-right">{{ row.dispatched_qty ?? $t('reportsReceiving.kpi.notAvailable') }}</td>
          <td class="table-td"><StatusBadge :tone="itemStatusTone(row.item_status)">{{ row.item_status }}</StatusBadge></td>
          <td class="table-td">{{ row.assigned_picker || '-' }}</td>
        </tr>
      </ReportDataTable>
    </div>

    <div v-else class="space-y-6">
      <p class="text-sm text-slate-500 bg-slate-50 border border-slate-200 rounded-xl p-3">
        {{ $t('reportsPicking.batchAllocationNote') }}
      </p>

      <ReportFilterBar v-model="batchDateRange" :cols="2" :loading="batchLoading" @apply="applyBatchFilters" @reset="resetBatchFilters">
        <label class="text-sm">
          <span class="block text-xs font-semibold text-slate-500 mb-1">{{ $t('reportsReceiving.filters.search') }}</span>
          <input v-model="batchFilters.search" type="text" class="field-input" :placeholder="$t('reportsPicking.filters.batchSearchPlaceholder')" />
        </label>
      </ReportFilterBar>

      <button type="button" class="export-btn" @click="onExportBatchCsv">{{ $t('reports.export.exportCsv') }}</button>

      <ReportDataTable
        :loading="batchLoading"
        :is-empty="!batchLoading && batchRows.length === 0"
        :colspan="9"
        min-width="1300px"
        :page="batchPage"
        :limit="batchLimit"
        :total="batchTotal"
        @update:page="onBatchPageChange"
      >
        <template #head>
          <tr>
            <th class="table-th">{{ $t('reportsPicking.table.batchNo') }}</th>
            <th class="table-th">{{ $t('reportsPicking.table.orderNo') }}</th>
            <th class="table-th">{{ $t('reportsPicking.table.customer') }}</th>
            <th class="table-th">{{ $t('common.labels.warehouse') }}</th>
            <th class="table-th">{{ $t('reportsReceiving.table.sku') }}</th>
            <th class="table-th text-right">{{ $t('reportsPicking.table.allocatedQty') }}</th>
            <th class="table-th text-right">{{ $t('reportsPicking.table.sortedQty') }}</th>
            <th class="table-th text-right">{{ $t('reportsPicking.table.shortageQty') }}</th>
            <th class="table-th text-right">{{ $t('reportsPicking.table.sharedPickedQty') }}</th>
          </tr>
        </template>

        <tr v-for="(row, idx) in batchRows" :key="idx" class="hover:bg-slate-50">
          <td class="table-td">{{ row.batch_no }}</td>
          <td class="table-td">{{ row.order_no }}</td>
          <td class="table-td">{{ row.customer_name || '-' }}</td>
          <td class="table-td">{{ row.warehouse_name }}</td>
          <td class="table-td">{{ row.sku }}</td>
          <td class="table-td text-right">{{ row.allocated_qty }} {{ row.uom }}</td>
          <td class="table-td text-right">{{ row.sorted_qty }} {{ row.uom }}</td>
          <td class="table-td text-right">{{ row.shortage_qty ?? '-' }}</td>
          <td class="table-td text-right">{{ row.shared_task_picked_qty ?? '-' }}</td>
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
  listPickingReport,
  getPickingReportSummary,
  exportPickingReportCsv,
  exportPickingReportXlsx,
  previewPickingReportPdf,
  downloadPickingReportPdf,
  listPickingBatchAllocations,
  exportPickingBatchAllocationsCsv,
} from '../../services/reportsPicking'
import { resolveDateRange, toRangeISO } from '../../utils/reportDateRange'

const TASK_STATUSES = ['PENDING', 'IN_PROGRESS', 'COMPLETED', 'SHORT_PICK', 'CANCELLED']

export default {
  name: 'PickingReportView',

  components: {
    PageHeader, StatusBadge, SortableTh,
    ReportFilterBar, ReportSummaryCards, ReportDataTable, ReportExportToolbar, ReportPrintPreview,
  },

  data() {
    return {
      view: 'detail',
      dateRange: { preset: 'last30Days', from: null, to: null },
      filters: { warehouseId: '', status: '', search: '', shortOnly: false },
      warehouses: [],
      taskStatuses: TASK_STATUSES,

      rows: [],
      total: 0,
      page: 1,
      limit: 20,
      sortBy: 'created_at',
      sortDir: 'desc',
      loading: false,

      summary: null,
      summaryLoading: false,

      loadError: '',
      generatedAt: '',

      showPreview: false,
      previewUrl: null,

      batchDateRange: { preset: 'last30Days', from: null, to: null },
      batchFilters: { search: '' },
      batchRows: [],
      batchTotal: 0,
      batchPage: 1,
      batchLimit: 20,
      batchLoading: false,
    }
  },

  computed: {
    appliedQuery() {
      const { from, to } = toRangeISO(resolveDateRange(this.dateRange.preset, this.dateRange))
      return {
        from, to,
        warehouseId: this.filters.warehouseId || undefined,
        status: this.filters.status || undefined,
        search: this.filters.search || undefined,
        shortOnly: this.filters.shortOnly || undefined,
      }
    },
    batchAppliedQuery() {
      const { from, to } = toRangeISO(resolveDateRange(this.batchDateRange.preset, this.batchDateRange))
      return { from, to, search: this.batchFilters.search || undefined }
    },
    summaryCards() {
      if (!this.summary) return []
      const s = this.summary
      return [
        { key: 'tasks', label: this.$t('reportsPicking.kpi.totalTasks'), value: String(s.total_tasks) },
        { key: 'completed', label: this.$t('reportsPicking.kpi.completedTasks'), value: String(s.completed_tasks) },
        { key: 'requested', label: this.$t('reportsPicking.kpi.totalRequested'), value: String(s.total_requested_qty) },
        { key: 'picked', label: this.$t('reportsPicking.kpi.totalPicked'), value: String(s.total_picked_qty) },
        { key: 'short', label: this.$t('reportsPicking.kpi.totalShort'), value: String(s.total_short_qty) },
        { key: 'shortRate', label: this.$t('reportsPicking.kpi.shortPickRate'), value: s.short_pick_rate != null ? `${s.short_pick_rate.toFixed(1)}%` : this.$t('reportsReceiving.kpi.notAvailable') },
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
      if (view === 'batch' && this.batchRows.length === 0) {
        this.applyBatchFilters()
      }
    },
    itemStatusTone(status) {
      return { PENDING: 'gray', PARTIALLY_PICKED: 'blue', COMPLETED: 'green', SHORT: 'red' }[status] || 'gray'
    },
    toggleShortOnly() {
      this.filters.shortOnly = !this.filters.shortOnly
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
      this.dateRange = { preset: 'last30Days', from: null, to: null }
      this.filters = { warehouseId: '', status: '', search: '', shortOnly: false }
      this.applyFilters()
    },

    async fetchRows() {
      this.loading = true
      this.loadError = ''
      try {
        const res = await listPickingReport(this.appliedQuery, {
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
        const res = await getPickingReportSummary(this.appliedQuery)
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
      await exportPickingReportCsv(this.appliedQuery)
    },
    async onExportExcel() {
      await exportPickingReportXlsx(this.appliedQuery)
    },
    async onDownloadPdf() {
      await downloadPickingReportPdf(this.appliedQuery)
    },
    async onPreviewPdf() {
      const blob = await previewPickingReportPdf(this.appliedQuery)
      this.previewUrl = URL.createObjectURL(blob)
      this.showPreview = true
    },
    onPrint() {
      window.print()
    },

    async applyBatchFilters() {
      this.batchPage = 1
      await this.fetchBatchRows()
    },
    resetBatchFilters() {
      this.batchDateRange = { preset: 'last30Days', from: null, to: null }
      this.batchFilters = { search: '' }
      this.applyBatchFilters()
    },
    async fetchBatchRows() {
      this.batchLoading = true
      try {
        const res = await listPickingBatchAllocations(this.batchAppliedQuery, { page: this.batchPage, limit: this.batchLimit })
        this.batchRows = res.data
        this.batchTotal = res.meta.total
      } catch (err) {
        this.loadError = err.message || this.$t('reports.table.noData')
        this.batchRows = []
        this.batchTotal = 0
      } finally {
        this.batchLoading = false
      }
    },
    onBatchPageChange(page) {
      this.batchPage = page
      this.fetchBatchRows()
    },
    async onExportBatchCsv() {
      await exportPickingBatchAllocationsCsv(this.batchAppliedQuery)
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
