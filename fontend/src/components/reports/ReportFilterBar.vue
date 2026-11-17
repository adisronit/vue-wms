<template>
  <FilterBar :cols="cols">
    <div class="md:col-span-full">
      <ReportDateTimeRangePicker
        :model-value="modelValue"
        @update:model-value="$emit('update:modelValue', $event)"
      />
    </div>

    <slot />

    <div class="md:col-span-full flex items-center justify-end gap-2">
      <button type="button" class="btn-secondary" :disabled="loading" @click="$emit('reset')">
        {{ $t('reports.filterBar.reset') }}
      </button>
      <button type="button" class="btn-primary" :disabled="loading" @click="$emit('apply')">
        {{ loading ? $t('common.buttons.loading') : $t('reports.filterBar.apply') }}
      </button>
    </div>
  </FilterBar>
</template>

<script>
import FilterBar from '../ui/FilterBar.vue'
import ReportDateTimeRangePicker from './ReportDateTimeRangePicker.vue'

// Reuses the shared FilterBar container (ui/FilterBar.vue) every other
// list screen's filter row already uses, adding the one thing every
// report needs on top of it — the shared date range picker plus
// Apply/Reset — while leaving report-specific extra fields (warehouse,
// SKU, supplier, ...) to the caller's default slot, since those differ
// per report family (see the Phase 34 spec's own per-report filter
// lists).
export default {
  name: 'ReportFilterBar',

  components: { FilterBar, ReportDateTimeRangePicker },

  props: {
    modelValue: { type: Object, required: true },
    loading: { type: Boolean, default: false },
    // Total filter-grid columns at the md breakpoint, matching
    // FilterBar's own cols prop — size it to however many extra fields
    // the caller puts in the default slot.
    cols: { type: Number, default: 4 },
  },

  emits: ['update:modelValue', 'apply', 'reset'],
}
</script>

<style scoped>
.btn-primary {
  background: #ea580c;
  color: white;
  padding: 8px 16px;
  border-radius: 8px;
  font-size: 14px;
  font-weight: 600;
  transition: background-color 0.15s;
}
.btn-primary:hover:not(:disabled) {
  background: #c2410c;
}
.btn-primary:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}
.btn-secondary {
  background: #f1f5f9;
  color: #334155;
  padding: 8px 16px;
  border-radius: 8px;
  font-size: 14px;
  font-weight: 500;
  transition: background-color 0.15s;
}
.btn-secondary:hover:not(:disabled) {
  background: #e2e8f0;
}
.btn-secondary:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}
</style>
