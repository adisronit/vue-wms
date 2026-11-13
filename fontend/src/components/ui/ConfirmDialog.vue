<template>
  <div
    v-if="open"
    class="fixed inset-0 bg-black/40 flex items-center justify-center z-50 p-4"
    @keydown.esc="$emit('cancel')"
  >
    <div class="bg-white rounded-2xl shadow-xl w-full max-w-md p-6 space-y-4" role="alertdialog" aria-modal="true">
      <h2 class="text-lg font-bold text-slate-800">{{ title }}</h2>

      <div class="text-sm text-slate-600">
        <slot>{{ message }}</slot>
      </div>

      <div v-if="errorMessage" class="bg-red-50 border border-red-200 text-red-600 rounded-xl p-3 text-sm">
        {{ errorMessage }}
      </div>

      <div class="flex gap-3">
        <button
          @click="$emit('cancel')"
          class="flex-1 bg-slate-100 hover:bg-slate-200 text-slate-700 py-3 rounded-xl font-semibold"
        >
          {{ cancelLabel }}
        </button>
        <button
          @click="$emit('confirm')"
          :disabled="loading"
          class="flex-1 text-white py-3 rounded-xl font-semibold disabled:opacity-60"
          :class="danger ? 'bg-red-600 hover:bg-red-700' : 'bg-orange-600 hover:bg-orange-700'"
        >
          {{ loading ? loadingLabel : confirmLabel }}
        </button>
      </div>
    </div>
  </div>
</template>

<script>
// Generic confirmation modal for any consequential action (cancel a
// document, suspend a membership, delete a row). Generalizes the
// confirm-step pattern already used ad hoc in TenantStatusModal/
// MembershipStatusDialog so other destructive actions (e.g. cancelling
// a receipt, which previously ran immediately with no confirmation at
// all) can reuse the same, consistent step instead of either skipping
// confirmation or re-implementing this markup per screen.
export default {
  name: 'ConfirmDialog',

  props: {
    open: { type: Boolean, default: false },
    title: { type: String, required: true },
    message: { type: String, default: '' },
    confirmLabel: { type: String, default: 'Confirm' },
    cancelLabel: { type: String, default: 'Cancel' },
    loadingLabel: { type: String, default: 'Please wait...' },
    danger: { type: Boolean, default: false },
    loading: { type: Boolean, default: false },
    errorMessage: { type: String, default: '' }
  },

  emits: ['confirm', 'cancel']
}
</script>
