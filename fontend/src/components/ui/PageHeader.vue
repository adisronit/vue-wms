<template>
  <div class="flex flex-col md:flex-row md:items-center md:justify-between gap-4 mb-6">
    <div class="min-w-0">
      <h1 class="text-2xl font-bold text-slate-800">
        <slot name="title">{{ title }}</slot>
      </h1>
      <p v-if="subtitle || $slots.subtitle" class="text-slate-500">
        <slot name="subtitle">{{ subtitle }}</slot>
      </p>
    </div>

    <div v-if="$slots.actions" class="flex gap-3 flex-wrap shrink-0">
      <slot name="actions" />
    </div>
  </div>
</template>

<script>
// Standard page title + subtitle + action-buttons row, used at the top
// of every routed screen. Pulled out of ~20 near-identical copies
// (each view previously hardcoded the same text-2xl/font-bold/mb-6
// markup) so the page title hierarchy stays visually consistent
// app-wide and only needs to change in one place.
export default {
  name: 'PageHeader',

  props: {
    title: { type: String, default: '' },
    subtitle: { type: String, default: '' }
  }
}
</script>
