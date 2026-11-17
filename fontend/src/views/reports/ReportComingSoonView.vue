<template>
  <div>
    <PageHeader :title="$t(meta.titleKey)" :subtitle="$t('reports.sectionTitle')" />

    <div class="bg-orange-50 border border-orange-200 text-orange-800 rounded-2xl p-4 mb-6">
      <p class="font-semibold">{{ $t('reports.comingSoon.title') }} — {{ meta.phase }}</p>
      <p class="text-sm mt-1">{{ $t('reports.comingSoon.message') }}</p>
      <p class="text-sm mt-1">{{ $t('reports.comingSoon.frameworkNote') }}</p>
    </div>

    <div class="space-y-6">
      <ReportFilterBar v-model="dateRange" :cols="1" :loading="false" @apply="applyFilters" @reset="resetFilters">
        <label class="text-sm">
          <span class="block text-xs font-semibold text-slate-500 mb-1">{{ $t('common.labels.warehouse') }}</span>
          <select v-model="warehouseId" class="field-select">
            <option value="">{{ $t('common.labels.selectOption') }}</option>
            <option v-for="wh in warehouses" :key="wh.id" :value="wh.id">{{ wh.name }}</option>
          </select>
        </label>
      </ReportFilterBar>

      <p v-if="appliedSummary" class="text-xs text-slate-500">{{ appliedSummary }}</p>

      <ReportSummaryCards :cards="[]" :loading="false" />

      <div class="bg-white rounded-2xl border border-slate-200 p-4">
        <ReportChart type="bar" :chart-data="null" />
      </div>

      <ReportExportToolbar disabled />

      <ReportDataTable :loading="false" :is-empty="true" :colspan="1" :page="1" :limit="20" :total="0">
        <template #head>
          <tr>
            <th class="table-th">{{ $t(meta.titleKey) }}</th>
          </tr>
        </template>
      </ReportDataTable>
    </div>
  </div>
</template>

<script>
import PageHeader from '../../components/ui/PageHeader.vue'
import ReportFilterBar from '../../components/reports/ReportFilterBar.vue'
import ReportSummaryCards from '../../components/reports/ReportSummaryCards.vue'
import ReportDataTable from '../../components/reports/ReportDataTable.vue'
import ReportChart from '../../components/reports/ReportChart.vue'
import ReportExportToolbar from '../../components/reports/ReportExportToolbar.vue'
import { listWarehouses } from '../../services/warehouses'
import { resolveDateRange, toRangeISO } from '../../utils/reportDateRange'

// Phase 34A ships the framework (menu, filter/date-range/table/chart/
// export shell), not report content — each report family gets its own
// data source, columns and KPIs in its own later Phase 34 batch (see
// REPORT_META below). This view demos the shared components with real
// warehouse master data for the one generic filter every report needs,
// but never fabricates report rows/metrics/chart series: Apply only
// resolves and displays the chosen filters, it does not call a
// not-yet-existing report endpoint.
const REPORT_META = {
  receiving: { titleKey: 'nav.reportsReceiving', phase: 'Phase 34B' },
  stockCount: { titleKey: 'nav.reportsStockCount', phase: 'Phase 34C' },
  pickingIssue: { titleKey: 'nav.reportsPickingIssue', phase: 'Phase 34D' },
  inventory: { titleKey: 'nav.reportsInventory', phase: 'Phase 34E' },
  pickingKpi: { titleKey: 'nav.reportsPickingKpi', phase: 'Phase 34F' },
  qc: { titleKey: 'nav.reportsQc', phase: 'Phase 34G' },
  delivery: { titleKey: 'nav.reportsDelivery', phase: 'Phase 34H' },
  warehouseLayout: { titleKey: 'nav.reportsWarehouseLayout', phase: 'Phase 34I' },
}

export default {
  name: 'ReportComingSoonView',

  components: {
    PageHeader,
    ReportFilterBar,
    ReportSummaryCards,
    ReportDataTable,
    ReportChart,
    ReportExportToolbar,
  },

  props: {
    reportKey: { type: String, required: true, validator: (v) => !!REPORT_META[v] },
  },

  data() {
    return {
      dateRange: { preset: 'today', from: null, to: null },
      warehouseId: '',
      warehouses: [],
      appliedSummary: '',
    }
  },

  computed: {
    meta() {
      return REPORT_META[this.reportKey]
    },
  },

  async mounted() {
    try {
      const res = await listWarehouses({ status: 'ACTIVE', limit: 100 })
      this.warehouses = res.data
    } catch {
      // Non-critical for a placeholder page: the warehouse filter just stays empty.
      this.warehouses = []
    }
  },

  methods: {
    applyFilters() {
      const { from, to } = toRangeISO(resolveDateRange(this.dateRange.preset, this.dateRange))
      const warehouseName = this.warehouses.find((w) => w.id === this.warehouseId)?.name
      this.appliedSummary = [
        `${this.$t('reports.dateRange.from')}: ${from ?? '—'}`,
        `${this.$t('reports.dateRange.to')}: ${to ?? '—'}`,
        warehouseName ? `${this.$t('common.labels.warehouse')}: ${warehouseName}` : null,
      ]
        .filter(Boolean)
        .join('  ·  ')
    },
    resetFilters() {
      this.dateRange = { preset: 'today', from: null, to: null }
      this.warehouseId = ''
      this.appliedSummary = ''
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
  min-width: 200px;
}
</style>
