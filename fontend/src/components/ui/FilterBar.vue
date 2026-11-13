<template>
  <div class="bg-white rounded-2xl border border-slate-200 p-5 mb-6">
    <div class="grid grid-cols-1 gap-4" :class="gridColsClass">
      <slot />
    </div>
  </div>
</template>

<script>
// Standard filter-row container (search/select/date inputs + a clear
// button) — the same bg-white/rounded-2xl/border/p-5/grid wrapper every
// list screen previously re-declared around its own filter inputs.
// Callers still own the actual <input>/<select> markup and v-model
// bindings; this only standardizes the container and column count.
export default {
  name: 'FilterBar',

  props: {
    // How many columns the filter grid uses at the md breakpoint —
    // matches whatever number of filter controls (including the clear
    // button) a given screen has.
    cols: {
      type: Number,
      default: 4
    }
  },

  computed: {
    gridColsClass() {
      const map = {
        2: 'md:grid-cols-2',
        3: 'md:grid-cols-3',
        4: 'md:grid-cols-4',
        5: 'md:grid-cols-5',
        6: 'md:grid-cols-6'
      }
      return map[this.cols] || 'md:grid-cols-4'
    }
  }
}
</script>
