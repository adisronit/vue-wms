<template>
  <th
    class="table-th sortable"
    :class="align === 'right' ? 'text-right' : ''"
    :aria-sort="ariaSort"
    tabindex="0"
    role="columnheader button"
    @click="$emit('sort', column)"
    @keydown.enter="$emit('sort', column)"
    @keydown.space.prevent="$emit('sort', column)"
  >
    <slot />
    <span class="sort-indicator" :class="{ 'sort-indicator--active': isActive }">{{ icon }}</span>
  </th>
</template>

<script>
// A single sortable column header — click or Enter/Space (when focused)
// to sort by `column`, toggling direction on repeat clicks. Pairs with
// the listPagination mixin's toggleSort(column): this component is
// deliberately "dumb" (no sort state of its own) so every list view's
// head row stays plain markup — <SortableTh column="code" :sort-by="sortBy"
// :sort-dir="sortDir" @sort="toggleSort">{{ $t('...') }}</SortableTh> —
// instead of each view re-implementing the same aria-sort/icon logic
// that already existed, duplicated, in ProductCategoryView.vue and others.
export default {
  name: 'SortableTh',

  props: {
    column: { type: String, required: true },
    sortBy: { type: String, default: '' },
    sortDir: { type: String, default: 'asc' },
    align: { type: String, default: 'left' }
  },

  emits: ['sort'],

  computed: {
    isActive() {
      return this.sortBy === this.column
    },
    icon() {
      if (!this.isActive) return '⇅'
      return this.sortDir === 'asc' ? '▲' : '▼'
    },
    ariaSort() {
      if (!this.isActive) return 'none'
      return this.sortDir === 'asc' ? 'ascending' : 'descending'
    }
  }
}
</script>

<style scoped>
.table-th {
  padding: 14px 16px;
  font-weight: var(--fw-table-header);
  font-size: var(--fs-table-header);
  background: #f8fafc;
  border-bottom: 1px solid #e2e8f0;
  color: #475569;
  white-space: nowrap;
}

.sortable {
  cursor: pointer;
  user-select: none;
}

.sortable:hover {
  color: #ea580c;
}

.sortable:focus-visible {
  outline: 2px solid #ea580c;
  outline-offset: -2px;
}

.sort-indicator {
  display: inline-block;
  margin-left: 4px;
  font-size: 0.7rem;
  color: #cbd5e1;
}

.sort-indicator--active {
  color: #ea580c;
}
</style>
