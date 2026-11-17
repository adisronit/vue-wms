<template>
  <div class="flex flex-wrap items-center justify-between gap-3">
    <p v-if="generatedAt" class="text-xs text-slate-400">
      {{ $t('reports.export.generatedAt', { timestamp: generatedAt }) }}
    </p>
    <div class="flex flex-wrap gap-2 ml-auto">
      <button
        type="button"
        class="export-btn"
        :disabled="disabled"
        :title="disabled ? $t('reports.export.notAvailableYet') : ''"
        @click="$emit('preview-pdf')"
      >
        {{ $t('reports.export.previewPdf') }}
      </button>
      <button
        type="button"
        class="export-btn"
        :disabled="disabled"
        :title="disabled ? $t('reports.export.notAvailableYet') : ''"
        @click="$emit('download-pdf')"
      >
        {{ $t('reports.export.downloadPdf') }}
      </button>
      <button
        type="button"
        class="export-btn"
        :disabled="disabled"
        :title="disabled ? $t('reports.export.notAvailableYet') : ''"
        @click="$emit('print')"
      >
        {{ $t('reports.export.print') }}
      </button>
      <button
        type="button"
        class="export-btn"
        :disabled="disabled"
        :title="disabled ? $t('reports.export.notAvailableYet') : ''"
        @click="$emit('export-excel')"
      >
        {{ $t('reports.export.exportExcel') }}
      </button>
      <button
        type="button"
        class="export-btn"
        :disabled="disabled"
        :title="disabled ? $t('reports.export.notAvailableYet') : ''"
        @click="$emit('export-csv')"
      >
        {{ $t('reports.export.exportCsv') }}
      </button>
    </div>
  </div>
</template>

<script>
// Every report page wires these same five events to its own export
// endpoints once it has one (see services/receiving.js's own
// apiDownload/triggerDownload pattern) — this component only renders
// the shared control row, it never calls the network itself. `disabled`
// is for pages (like the Phase 34A coming-soon placeholder) that have
// no real export endpoint yet; it is not a loading/busy state.
export default {
  name: 'ReportExportToolbar',

  props: {
    disabled: { type: Boolean, default: false },
    generatedAt: { type: String, default: null },
  },

  emits: ['preview-pdf', 'download-pdf', 'print', 'export-excel', 'export-csv'],
}
</script>

<style scoped>
.export-btn {
  background: #f1f5f9;
  color: #334155;
  padding: 8px 14px;
  border-radius: 8px;
  font-size: 13px;
  font-weight: 500;
  transition: background-color 0.15s;
}
.export-btn:hover:not(:disabled) {
  background: #e2e8f0;
}
.export-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
</style>
