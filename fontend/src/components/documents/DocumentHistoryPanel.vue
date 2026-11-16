<template>
  <div
    v-if="open"
    class="fixed inset-0 bg-black/40 flex items-center justify-center z-50 p-4"
    @keydown.esc="$emit('close')"
  >
    <div class="bg-white rounded-2xl shadow-xl w-full max-w-2xl p-6 space-y-4" role="dialog" aria-modal="true">
      <div class="flex items-center justify-between">
        <h2 class="text-lg font-bold text-slate-800">{{ title || $t('documentHistory.title') }}</h2>
        <button @click="$emit('close')" class="text-slate-500 hover:text-red-600 text-xl">✕</button>
      </div>

      <div v-if="loadError" class="bg-red-50 border border-red-200 text-red-600 rounded-xl p-3 text-sm">
        {{ loadError }}
      </div>
      <div v-else-if="loading" class="text-sm text-slate-400">{{ $t('documentHistory.loading') }}</div>
      <div v-else-if="rows.length === 0" class="text-sm text-slate-400">{{ $t('documentHistory.empty') }}</div>

      <div v-else class="max-h-96 overflow-y-auto divide-y divide-slate-100">
        <div
          v-for="row in rows"
          :key="row.id"
          class="py-3 flex items-center justify-between gap-3"
        >
          <div>
            <p class="font-semibold text-slate-800 text-sm">
              {{ formatDateTime(row.rendered_at) }}
              <span v-if="row.is_draft" class="ml-2 text-xs font-bold text-red-600 bg-red-50 px-2 py-0.5 rounded">
                {{ $t('documentHistory.draftBadge') }}
              </span>
            </p>
            <p class="text-xs text-slate-400">
              {{ $t('documentHistory.renderedBy', { name: row.rendered_by_name || $t('documentHistory.unknownUser') }) }}
            </p>
          </div>

          <div class="flex gap-2 shrink-0">
            <button
              @click="view(row)"
              class="bg-slate-100 hover:bg-slate-200 text-slate-700 text-xs px-3 py-2 rounded-lg font-semibold"
            >
              {{ $t('documentHistory.view') }}
            </button>
            <button
              @click="download(row)"
              class="bg-slate-100 hover:bg-slate-200 text-slate-700 text-xs px-3 py-2 rounded-lg font-semibold"
            >
              {{ $t('documentHistory.download') }}
            </button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script>
// Phase 33C.3 — Document History / Reprint panel. A small, generic
// modal reused from every document's existing Preview/Download/Print
// controls (Order detail, Picking task drawer, Picking Batch detail,
// Packing job detail) — one component, driven entirely by the
// documentType/sourceId props, rather than one bespoke history UI per
// screen. Fetches the history list only once opened (not on mount), so
// mounting the parent page never triggers a network call just because
// this component exists in its template.
import { getDocumentHistory, viewDocumentRender, downloadDocumentRender } from '../../services/documents'
import { showToast } from '../../services/toast'

export default {
  name: 'DocumentHistoryPanel',

  props: {
    open: { type: Boolean, default: false },
    documentType: { type: String, required: true },
    sourceId: { type: [String, Number], default: null },
    title: { type: String, default: '' }
  },

  emits: ['close'],

  data() {
    return {
      loading: false,
      loadError: '',
      rows: []
    }
  },

  watch: {
    open: {
      immediate: true,
      handler(isOpen) {
        if (isOpen) this.fetchHistory()
      }
    }
  },

  methods: {
    async fetchHistory() {
      if (!this.sourceId) return
      this.loading = true
      this.loadError = ''
      try {
        const res = await getDocumentHistory(this.documentType, this.sourceId)
        this.rows = res.data || []
      } catch (err) {
        this.loadError = err.message || this.$t('documentHistory.loadFailed')
      } finally {
        this.loading = false
      }
    },

    async view(row) {
      try {
        await viewDocumentRender(row.id)
      } catch (err) {
        showToast(err.message || this.$t('documentHistory.actionFailed'), 'error')
      }
    },

    async download(row) {
      try {
        await downloadDocumentRender(row.id, `${row.document_no || row.id}.pdf`)
      } catch (err) {
        showToast(err.message || this.$t('documentHistory.actionFailed'), 'error')
      }
    },

    formatDateTime(value) {
      if (!value) return ''
      const d = new Date(value)
      if (Number.isNaN(d.getTime())) return value
      return d.toLocaleString()
    }
  }
}
</script>
