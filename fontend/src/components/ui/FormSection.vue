<template>
  <div class="bg-white rounded-2xl border border-slate-200 p-6 mb-6">
    <div v-if="title || $slots.title" class="mb-4">
      <h2 class="text-lg font-bold text-slate-800">
        <slot name="title">{{ title }}</slot>
      </h2>
      <p v-if="description" class="text-sm text-slate-500 mt-1">{{ description }}</p>
    </div>

    <div class="space-y-4">
      <slot />
    </div>
  </div>
</template>

<script>
// A titled card wrapper for one logical group of form fields — the
// bg-white/rounded-2xl/border/p-6 card every "New ..." form screen
// previously repeated by hand around each of its sections (header
// info, line items, confirmation), now with one consistent heading
// style instead of each screen inventing its own.
export default {
  name: 'FormSection',

  props: {
    title: { type: String, default: '' },
    description: { type: String, default: '' }
  }
}
</script>
