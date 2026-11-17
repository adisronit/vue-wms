<template>
  <div class="space-y-3">
    <div class="flex flex-wrap gap-2">
      <button
        v-for="preset in presets"
        :key="preset"
        type="button"
        class="preset-chip"
        :class="{ 'preset-chip-active': modelValue.preset === preset }"
        @click="selectPreset(preset)"
      >
        {{ $t(`reports.dateRange.${preset}`) }}
      </button>
    </div>

    <div v-if="modelValue.preset === 'custom'" class="flex flex-wrap items-end gap-3">
      <label class="text-sm">
        <span class="block text-xs font-semibold text-slate-500 mb-1">{{ $t('reports.dateRange.from') }}</span>
        <input
          type="datetime-local"
          class="field-input"
          :value="modelValue.from || ''"
          @change="emitCustom('from', $event.target.value)"
        />
      </label>
      <label class="text-sm">
        <span class="block text-xs font-semibold text-slate-500 mb-1">{{ $t('reports.dateRange.to') }}</span>
        <input
          type="datetime-local"
          class="field-input"
          :value="modelValue.to || ''"
          @change="emitCustom('to', $event.target.value)"
        />
      </label>
    </div>

    <p class="text-xs text-slate-400">{{ $t('reports.dateRange.timezoneNote') }}</p>
  </div>
</template>

<script>
import { DATE_RANGE_PRESETS } from '../../utils/reportDateRange'

// Dumb preset/custom-range picker — resolving a preset or custom pair
// into actual UTC boundaries (see utils/reportDateRange.js) happens in
// the page that owns the filter, at Apply time, not here on every
// keystroke.
export default {
  name: 'ReportDateTimeRangePicker',

  props: {
    modelValue: {
      type: Object,
      required: true,
      // Shape: { preset: 'today' | ... | 'custom', from: string|null, to: string|null }
      // from/to are only used for preset === 'custom', as
      // datetime-local ("YYYY-MM-DDTHH:mm") Bangkok-local strings.
    },
  },

  emits: ['update:modelValue'],

  data() {
    return {
      presets: DATE_RANGE_PRESETS,
    }
  },

  methods: {
    selectPreset(preset) {
      this.$emit('update:modelValue', { preset, from: this.modelValue.from, to: this.modelValue.to })
    },
    emitCustom(field, value) {
      this.$emit('update:modelValue', { ...this.modelValue, [field]: value })
    },
  },
}
</script>

<style scoped>
.preset-chip {
  padding: 6px 12px;
  border-radius: 999px;
  font-size: 13px;
  font-weight: 500;
  background: #f1f5f9;
  color: #475569;
  border: 1px solid transparent;
  transition: background-color 0.15s, color 0.15s;
}
.preset-chip:hover {
  background: #e2e8f0;
}
.preset-chip-active {
  background: #fff7ed;
  color: #c2410c;
  border-color: #fdba74;
  font-weight: 600;
}
.field-input {
  border: 1px solid #cbd5e1;
  border-radius: 8px;
  padding: 6px 10px;
  font-size: 14px;
}
</style>
