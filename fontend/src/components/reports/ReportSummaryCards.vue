<template>
  <div v-if="loading" class="grid gap-3" :style="gridStyle">
    <div v-for="n in skeletonCount" :key="n" class="summary-card animate-pulse">
      <div class="h-3 w-20 bg-slate-200 rounded mb-3"></div>
      <div class="h-6 w-16 bg-slate-200 rounded"></div>
    </div>
  </div>

  <div v-else-if="cards.length === 0" class="text-sm text-slate-400 py-6 text-center">
    {{ $t('reports.summaryCards.empty') }}
  </div>

  <div v-else class="grid gap-3" :style="gridStyle">
    <div v-for="card in cards" :key="card.key" class="summary-card">
      <p class="text-xs font-semibold text-slate-500 truncate">{{ card.label }}</p>
      <p class="text-2xl font-bold text-slate-800 mt-1">{{ card.value }}</p>
      <p v-if="card.hint" class="text-xs text-slate-400 mt-1 truncate">{{ card.hint }}</p>
    </div>
  </div>
</template>

<script>
// Generic KPI/stat-card grid. `cards` values are pre-formatted strings
// by the caller (e.g. "1,234" or "N/A") — this component never
// computes or infers a metric itself, matching CLAUDE.md's "show N/A
// when a metric cannot be reliably calculated" rule, which only the
// report's own service layer can actually determine.
export default {
  name: 'ReportSummaryCards',

  props: {
    cards: {
      type: Array,
      default: () => [],
      // [{ key, label, value, hint? }]
    },
    loading: { type: Boolean, default: false },
    minColumnWidth: { type: String, default: '180px' },
  },

  computed: {
    gridStyle() {
      return { gridTemplateColumns: `repeat(auto-fill, minmax(${this.minColumnWidth}, 1fr))` }
    },
    skeletonCount() {
      return this.cards.length || 4
    },
  },
}
</script>

<style scoped>
.summary-card {
  background: white;
  border: 1px solid #e2e8f0;
  border-radius: 16px;
  padding: 16px;
}
</style>
