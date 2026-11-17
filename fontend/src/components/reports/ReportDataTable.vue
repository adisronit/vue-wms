<template>
  <DataTable
    :loading="loading"
    :is-empty="!loading && isEmpty"
    :loading-text="$t('reports.table.loading')"
    :empty-text="$t('reports.table.noData')"
    :colspan="colspan"
    :min-width="minWidth"
  >
    <template #head>
      <slot name="head" />
    </template>

    <slot />

    <template v-if="total !== null && !loading && !isEmpty" #footer>
      <PaginationFooter
        :page="page"
        :total-pages="totalPages"
        :total="total"
        :limit="limit"
        @update:page="$emit('update:page', $event)"
        @update:limit="$emit('update:limit', $event)"
      />
    </template>
  </DataTable>
</template>

<script>
import DataTable from '../ui/DataTable.vue'
import PaginationFooter from '../ui/PaginationFooter.vue'

// A Reports-flavored preset of the shared DataTable/PaginationFooter
// primitives every other list screen already uses (see
// ui/DataTable.vue, ui/PaginationFooter.vue) — same #head/default/
// #footer slot API, just with the Reports i18n strings baked in so
// each report page doesn't repeat loadingText/emptyText bindings.
// Deliberately does not reimplement sorting: SortableTh
// (ui/SortableTh.vue) still goes directly in the caller's #head slot
// the same way every other list view already uses it.
export default {
  name: 'ReportDataTable',

  components: { DataTable, PaginationFooter },

  props: {
    loading: { type: Boolean, default: false },
    isEmpty: { type: Boolean, default: false },
    colspan: { type: Number, default: 1 },
    minWidth: { type: String, default: '' },
    page: { type: Number, default: 1 },
    limit: { type: Number, default: 20 },
    total: { type: Number, default: null },
  },

  emits: ['update:page', 'update:limit'],

  computed: {
    totalPages() {
      if (!this.total || !this.limit) return 1
      return Math.max(1, Math.ceil(this.total / this.limit))
    },
  },
}
</script>
