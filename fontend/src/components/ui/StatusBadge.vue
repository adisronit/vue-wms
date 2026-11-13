<template>
  <span class="status-badge" :class="'status-badge--' + tone">
    <slot />
  </span>
</template>

<script>
// Replaces the ~20 copies of .badge-gray/.badge-orange/.badge-blue/
// .badge-green(/.badge-red) each view previously defined identically in
// its own <style scoped> block. Callers keep their own domain-specific
// status -> tone mapping (e.g. "COMPLETED" -> "green") — this component
// only owns the visual rendering of a tone, not any business status
// vocabulary.
export default {
  name: 'StatusBadge',

  props: {
    // One of: gray | orange | blue | green | red
    tone: {
      type: String,
      default: 'gray',
      validator: (v) => ['gray', 'orange', 'blue', 'green', 'red'].includes(v)
    }
  }
}
</script>

<style scoped>
.status-badge {
  display: inline-block;
  padding: 5px 10px;
  border-radius: 999px;
  font-weight: 600;
  font-size: var(--fs-table-header);
  white-space: nowrap;
}

.status-badge--gray {
  background: #f1f5f9;
  color: #475569;
}

.status-badge--orange {
  background: #ffedd5;
  color: #ea580c;
}

.status-badge--blue {
  background: #dbeafe;
  color: #2563eb;
}

.status-badge--green {
  background: #dcfce7;
  color: #16a34a;
}

.status-badge--red {
  background: #fee2e2;
  color: #dc2626;
}
</style>
