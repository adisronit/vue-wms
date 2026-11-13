<template>
  <div class="bg-white rounded-2xl border border-slate-200 shadow-sm overflow-hidden">
    <div class="w-full overflow-x-auto">
      <table class="w-full text-sm" :style="minWidth ? { minWidth } : {}">
        <thead class="bg-slate-50 text-slate-500">
          <slot name="head" />
        </thead>
        <tbody>
          <tr v-if="loading">
            <td :colspan="colspan" class="text-center py-10 text-slate-400">
              {{ loadingText }}
            </td>
          </tr>
          <tr v-else-if="isEmpty">
            <td :colspan="colspan" class="text-center py-10 text-slate-400">
              {{ emptyText }}
            </td>
          </tr>
          <slot v-else />
        </tbody>
      </table>
    </div>

    <div v-if="$slots.footer" class="border-t border-slate-200">
      <slot name="footer" />
    </div>
  </div>
</template>

<script>
// Standard table shell: the bg-white/rounded-2xl/overflow-x-auto wrapper
// plus the loading-row / empty-row handling every list screen
// previously re-implemented by hand (<tr v-if="loading">...<tr
// v-else-if="list.length === 0">...). Callers keep full control of
// columns — they provide the <tr><th>...</th></tr> head row and their
// own <tr v-for> data rows as slot content; this component only owns
// the surrounding chrome and the three loading/empty/data states, plus
// an optional pagination footer slot.
export default {
  name: 'DataTable',

  props: {
    loading: { type: Boolean, default: false },
    isEmpty: { type: Boolean, default: false },
    loadingText: { type: String, default: 'Loading...' },
    emptyText: { type: String, default: 'No data found' },
    // Must match the number of <th> columns in the head slot, so the
    // loading/empty row's single <td colspan> spans the full table
    // width instead of leaving a ragged partial row.
    colspan: { type: Number, default: 1 },
    // e.g. '1100px' for a wide table that needs horizontal scroll on
    // narrower viewports rather than crushing its columns.
    minWidth: { type: String, default: '' }
  }
}
</script>
