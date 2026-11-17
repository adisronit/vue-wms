<template>
  <div class="relative" :style="{ height: height + 'px' }">
    <component :is="chartComponent" v-if="hasData" :data="chartData" :options="mergedOptions" />
    <div v-else class="h-full flex items-center justify-center text-sm text-slate-400">
      {{ $t('reports.chart.noData') }}
    </div>
  </div>
</template>

<script>
import { Chart as ChartJS, registerables } from 'chart.js'
import { Bar, Line, Pie, Doughnut } from 'vue-chartjs'

ChartJS.register(...registerables)

const CHART_COMPONENTS = { bar: Bar, line: Line, pie: Pie, doughnut: Doughnut }

// Thin, generic wrapper so every report page configures chart.js the
// same way instead of each one re-registering components/options.
// Never fabricates a placeholder series when chartData is empty — it
// shows the shared "no data" state instead (see CLAUDE.md's "never
// fabricate operational data").
export default {
  name: 'ReportChart',

  props: {
    type: {
      type: String,
      default: 'bar',
      validator: (v) => Object.keys(CHART_COMPONENTS).includes(v),
    },
    chartData: {
      type: Object,
      default: null,
      // Chart.js data shape: { labels: [...], datasets: [...] }
    },
    chartOptions: { type: Object, default: () => ({}) },
    height: { type: Number, default: 280 },
  },

  computed: {
    chartComponent() {
      return CHART_COMPONENTS[this.type]
    },
    hasData() {
      return !!this.chartData?.labels?.length && !!this.chartData?.datasets?.length
    },
    mergedOptions() {
      return {
        responsive: true,
        maintainAspectRatio: false,
        ...this.chartOptions,
      }
    },
  },
}
</script>
