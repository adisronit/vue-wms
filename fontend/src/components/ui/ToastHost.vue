<template>
  <div class="fixed top-5 right-5 z-[100] w-full max-w-sm space-y-2" aria-live="polite" role="status">
    <div
      v-for="t in toasts"
      :key="t.id"
      class="rounded-xl shadow-lg border px-4 py-3 text-sm font-medium flex items-start justify-between gap-3"
      :class="toneClass(t.type)"
    >
      <span>{{ t.message }}</span>
      <button
        @click="dismissToast(t.id)"
        class="opacity-60 hover:opacity-100 shrink-0"
        :aria-label="$t('common.buttons.cancel')"
      >
        ✕
      </button>
    </div>
  </div>
</template>

<script>
import { toastState, dismissToast } from '../../services/toast'

const TONE_CLASSES = {
  success: 'bg-green-50 border-green-200 text-green-700',
  error: 'bg-red-50 border-red-200 text-red-700',
  info: 'bg-blue-50 border-blue-200 text-blue-700'
}

export default {
  name: 'ToastHost',

  computed: {
    toasts() {
      return toastState().toasts
    }
  },

  methods: {
    dismissToast,
    toneClass(type) {
      return TONE_CLASSES[type] || TONE_CLASSES.info
    }
  }
}
</script>
