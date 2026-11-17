<template>
  <div v-if="modelValue" class="fixed inset-0 bg-slate-900/60 flex items-center justify-center z-50 p-6">
    <div class="bg-white rounded-2xl shadow-xl w-full max-w-4xl h-[85vh] flex flex-col overflow-hidden">
      <div class="flex items-center justify-between px-5 py-3 border-b border-slate-200">
        <h3 class="font-bold text-slate-800">{{ $t('reports.printPreview.title') }}</h3>
        <div class="flex items-center gap-2">
          <button type="button" class="toolbar-btn" :disabled="!pdfUrl" @click="printNow">
            {{ $t('reports.printPreview.print') }}
          </button>
          <button type="button" class="toolbar-btn" @click="close">
            {{ $t('reports.printPreview.close') }}
          </button>
        </div>
      </div>

      <div class="flex-1 bg-slate-100">
        <iframe
          v-if="pdfUrl"
          ref="previewFrame"
          :src="pdfUrl"
          class="w-full h-full border-0"
          title="Report PDF preview"
        ></iframe>
        <div v-else class="h-full flex items-center justify-center text-sm text-slate-400">
          {{ $t('reports.table.loading') }}
        </div>
      </div>
    </div>
  </div>
</template>

<script>
// Generic modal shell for "Preview PDF": the caller fetches a PDF blob
// (via services/api.js's apiDownload) and passes a local
// URL.createObjectURL(blob) string as pdfUrl — this component never
// fetches anything itself. Print uses the iframe's own print(), the
// standard way to print embedded PDF content without a server round
// trip.
export default {
  name: 'ReportPrintPreview',

  props: {
    modelValue: { type: Boolean, default: false },
    pdfUrl: { type: String, default: null },
  },

  emits: ['update:modelValue'],

  methods: {
    close() {
      this.$emit('update:modelValue', false)
    },
    printNow() {
      const win = this.$refs.previewFrame?.contentWindow
      if (win) win.print()
    },
  },
}
</script>

<style scoped>
.toolbar-btn {
  background: #f1f5f9;
  color: #334155;
  padding: 6px 14px;
  border-radius: 8px;
  font-size: 13px;
  font-weight: 500;
  transition: background-color 0.15s;
}
.toolbar-btn:hover:not(:disabled) {
  background: #e2e8f0;
}
.toolbar-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
</style>
