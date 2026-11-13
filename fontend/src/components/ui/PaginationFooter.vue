<template>
  <div class="flex flex-col md:flex-row items-center justify-between gap-3 px-5 py-4 text-sm text-slate-500">
    <span>
      {{ $t('common.pagination.showing') }} {{ rangeStart }} {{ $t('common.pagination.to') }} {{ rangeEnd }}
      {{ $t('common.pagination.of') }} {{ total }} {{ $t('common.pagination.results') }}
    </span>

    <div class="flex items-center gap-4">
      <label class="flex items-center gap-2">
        {{ $t('common.pagination.rowsPerPage') }}
        <select v-model.number="limitModel" class="rows-per-page" :disabled="disabled">
          <option v-for="size in pageSizeOptions" :key="size" :value="size">{{ size }}</option>
        </select>
      </label>

      <div class="flex items-center gap-2">
        <button
          type="button"
          @click="$emit('update:page', 1)"
          :disabled="disabled || page <= 1"
          class="page-btn"
          :aria-label="$t('common.pagination.first')"
        >
          {{ $t('common.pagination.first') }}
        </button>
        <button
          type="button"
          @click="$emit('update:page', page - 1)"
          :disabled="disabled || page <= 1"
          class="page-btn"
        >
          {{ $t('common.pagination.previous') }}
        </button>
        <span class="whitespace-nowrap">{{ $t('common.pagination.page') }} {{ page }} {{ $t('common.pagination.of') }} {{ totalPages }}</span>
        <button
          type="button"
          @click="$emit('update:page', page + 1)"
          :disabled="disabled || page >= totalPages"
          class="page-btn"
        >
          {{ $t('common.pagination.next') }}
        </button>
        <button
          type="button"
          @click="$emit('update:page', totalPages)"
          :disabled="disabled || page >= totalPages"
          class="page-btn"
          :aria-label="$t('common.pagination.last')"
        >
          {{ $t('common.pagination.last') }}
        </button>
      </div>
    </div>
  </div>
</template>

<script>
// The standard pagination footer for every server-side-paginated list —
// "Showing X to Y of Z results", a rows-per-page select, and
// First/Previous/[page N of M]/Next/Last controls. Replaces the near-
// identical block every list view (ProductCategoryView, ProductMasterView,
// ...) previously hand-rolled. Pairs with the listPagination mixin: the
// host owns page/limit/total/totalPages state and refetches on
// update:page / update:limit, this component only renders and emits —
// it never clamps or fetches anything itself, so First/Previous/Next/
// Last are disabled exactly at the boundaries the host already knows.
export default {
  name: 'PaginationFooter',

  props: {
    page: { type: Number, required: true },
    totalPages: { type: Number, required: true },
    total: { type: Number, required: true },
    limit: { type: Number, required: true },
    pageSizeOptions: { type: Array, default: () => [10, 25, 50, 100] },
    disabled: { type: Boolean, default: false }
  },

  emits: ['update:page', 'update:limit'],

  computed: {
    rangeStart() {
      return this.total === 0 ? 0 : (this.page - 1) * this.limit + 1
    },
    rangeEnd() {
      return Math.min(this.page * this.limit, this.total)
    },
    limitModel: {
      get() {
        return this.limit
      },
      set(value) {
        this.$emit('update:limit', value)
      }
    }
  }
}
</script>

<style scoped>
.rows-per-page {
  border: 1px solid #cbd5e1;
  border-radius: 10px;
  padding: 6px 10px;
  outline: none;
  background: #fff;
  color: #334155;
}

.rows-per-page:focus {
  border-color: #ea580c;
  box-shadow: 0 0 0 2px #fed7aa;
}

.page-btn {
  padding: 6px 12px;
  border-radius: 8px;
  background: #f1f5f9;
  color: #334155;
  font-weight: 600;
}

.page-btn:hover:not(:disabled) {
  background: #e2e8f0;
}

.page-btn:focus-visible {
  outline: 2px solid #ea580c;
  outline-offset: 1px;
}

.page-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
</style>
