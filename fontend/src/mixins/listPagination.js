// Shared page/limit/sort bookkeeping for a server-side-paginated list
// screen using DataTable + SortableTh + PaginationFooter — the
// standardized contract every "primary listing" view in the app
// follows (see ProductCategoryView.vue for the original, hand-rolled
// version of this exact logic, which this mixin now centralizes).
//
// Contract the host component must satisfy:
//   - implement fetchList() — the actual API call, reading
//     this.page/this.limit/this.sortBy/this.sortDir (and whatever
//     filters) and setting this.total/this.totalPages from the
//     response's meta on success.
//   - declare sortBy/sortDir in its own data() with the view's
//     intended default column/direction (code/name ASC for master
//     data, created_at/date DESC for transactional listings) — this
//     mixin's toggleSort() only mutates them, it doesn't own the
//     default.
//   - call resetToFirstPage() (not fetchList() directly) from filter
//     watchers, so every filter change and every page-size change
//     goes through the same "reset to page 1" path.
export const PAGE_SIZE_OPTIONS = [10, 25, 50, 100]
export const DEFAULT_PAGE_LIMIT = 25

export default {
  data() {
    return {
      page: 1,
      limit: DEFAULT_PAGE_LIMIT,
      total: 0,
      totalPages: 1,
      pageSizeOptions: PAGE_SIZE_OPTIONS
    }
  },

  watch: {
    limit() {
      this.resetToFirstPage()
    }
  },

  methods: {
    goToPage(targetPage) {
      const clamped = Math.min(Math.max(1, targetPage), Math.max(1, this.totalPages))
      if (clamped === this.page) return
      this.page = clamped
      this.fetchList()
    },

    resetToFirstPage() {
      this.page = 1
      this.fetchList()
    },

    // Toggles direction when re-clicking the active column, otherwise
    // switches to the new column at defaultDir — matching the
    // established per-view behavior this replaces. A sort change always
    // resets to page 1: staying on, say, page 3 after the order
    // underneath it just changed would show an arbitrary slice of the
    // newly-sorted list, not what the user actually asked for.
    toggleSort(column, defaultDir = 'asc') {
      if (this.sortBy === column) {
        this.sortDir = this.sortDir === 'asc' ? 'desc' : 'asc'
      } else {
        this.sortBy = column
        this.sortDir = defaultDir
      }
      this.resetToFirstPage()
    }
  }
}
