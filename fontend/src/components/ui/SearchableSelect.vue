<template>
  <div class="searchable-select" @keydown.esc="close">
    <input
      ref="inputEl"
      type="text"
      class="input"
      :class="{ 'input--disabled': disabled }"
      :placeholder="placeholder"
      :disabled="disabled"
      v-model="query"
      role="combobox"
      :aria-expanded="open"
      aria-autocomplete="list"
      @focus="onFocus"
      @input="onInput"
      @keydown.down.prevent="moveHighlight(1)"
      @keydown.up.prevent="moveHighlight(-1)"
      @keydown.enter.prevent="selectHighlighted"
      @blur="onBlur"
    />
    <button
      v-if="clearable && modelValue && !disabled"
      type="button"
      class="clear-btn"
      :aria-label="clearLabel"
      @mousedown.prevent="clear"
    >
      ✕
    </button>

    <ul v-if="open" class="options" role="listbox">
      <li v-if="loading" class="option option--empty">{{ loadingText }}</li>
      <li v-else-if="filteredOptions.length === 0" class="option option--empty">{{ emptyText }}</li>
      <li
        v-for="(opt, index) in filteredOptions"
        :key="opt.value"
        class="option"
        :class="{ 'option--highlighted': index === highlightedIndex }"
        role="option"
        :aria-selected="opt.value === modelValue"
        @mousedown.prevent="select(opt)"
        @mouseenter="highlightedIndex = index"
      >
        {{ opt.label }}
      </li>
    </ul>
  </div>
</template>

<script>
// A lightweight typeahead combobox, reused across the EWTA-redesigned
// Product Add/Edit form for every "pick one row from a tenant-scoped
// master list" field (Category, Brand, Family, Default Supplier,
// Preferred Warehouse/Zone) — replacing a plain <select> (which doesn't
// scale once a tenant has more rows than fit comfortably in a native
// dropdown, and can't search) without pulling in a UI library.
//
// Two modes, chosen by which prop the caller passes:
// - `fetch-options` (async (search) => [{value,label}]): debounced
//   remote search, for lists that can be large (Category/Brand/Family/
//   Supplier) and already have a search-capable list endpoint.
// - `options` (a plain array): client-side filtering only, for lists
//   that are always small and already fully loaded (e.g. a warehouse's
//   zones, fetched once after the warehouse is picked).
//
// The component never fetches "the label for an already-selected ID"
// itself — the caller passes `display-value` (resolved from whatever
// list it already has, e.g. an Edit drawer's existing product data) so
// opening the dropdown never requires an extra round trip just to show
// what's currently selected.
export default {
  name: 'SearchableSelect',

  props: {
    modelValue: { type: [String, Number, null], default: null },
    displayValue: { type: String, default: '' },
    fetchOptions: { type: Function, default: null },
    options: { type: Array, default: null },
    placeholder: { type: String, default: '' },
    disabled: { type: Boolean, default: false },
    clearable: { type: Boolean, default: true },
    clearLabel: { type: String, default: 'Clear' },
    loadingText: { type: String, default: 'Loading...' },
    emptyText: { type: String, default: 'No matches' },
    debounceMs: { type: Number, default: 300 }
  },

  emits: ['update:modelValue', 'update:displayValue', 'change'],

  data() {
    return {
      query: this.displayValue,
      open: false,
      loading: false,
      remoteOptions: [],
      highlightedIndex: -1,
      debounceHandle: null,
      // Tracks whether `query` currently reflects a real search the
      // user typed, vs. just mirroring displayValue — so a focus+blur
      // with no typing doesn't wipe the selection.
      dirty: false
    }
  },

  computed: {
    filteredOptions() {
      const source = this.options !== null ? this.options : this.remoteOptions
      if (this.options === null) return source // already server-filtered
      const q = this.query.trim().toLowerCase()
      if (!q) return source
      return source.filter((o) => o.label.toLowerCase().includes(q))
    }
  },

  watch: {
    displayValue(next) {
      if (!this.open) this.query = next
    }
  },

  methods: {
    onFocus() {
      this.open = true
      if (this.options !== null) return // static list, nothing to fetch
      if (this.remoteOptions.length === 0 && !this.loading) this.runFetch()
    },

    onInput() {
      this.dirty = true
      this.open = true
      this.highlightedIndex = -1
      if (this.options !== null) return
      clearTimeout(this.debounceHandle)
      this.debounceHandle = setTimeout(() => this.runFetch(), this.debounceMs)
    },

    async runFetch() {
      if (!this.fetchOptions) return
      this.loading = true
      try {
        this.remoteOptions = await this.fetchOptions(this.query.trim())
      } catch {
        this.remoteOptions = []
      } finally {
        this.loading = false
      }
    },

    moveHighlight(delta) {
      if (!this.open) {
        this.open = true
        return
      }
      const len = this.filteredOptions.length
      if (len === 0) return
      this.highlightedIndex = (this.highlightedIndex + delta + len) % len
    },

    selectHighlighted() {
      const opt = this.filteredOptions[this.highlightedIndex]
      if (opt) this.select(opt)
    },

    select(opt) {
      this.query = opt.label
      this.dirty = false
      this.open = false
      this.highlightedIndex = -1
      this.$emit('update:modelValue', opt.value)
      this.$emit('update:displayValue', opt.label)
      this.$emit('change', opt)
    },

    clear() {
      this.query = ''
      this.dirty = false
      this.open = false
      this.$emit('update:modelValue', null)
      this.$emit('update:displayValue', '')
      this.$emit('change', null)
    },

    onBlur() {
      // Deferred so a mousedown on an option (which itself prevents
      // default to avoid stealing focus before @click fires) still
      // completes its own select()/clear() before this closes the list.
      setTimeout(() => {
        this.open = false
        // Blurring without picking anything reverts the visible text
        // to the last real selection, rather than leaving a half-typed
        // search string that doesn't match modelValue.
        if (this.dirty) {
          this.query = this.displayValue
          this.dirty = false
        }
      }, 150)
    },

    close() {
      this.open = false
      this.$refs.inputEl.blur()
    }
  }
}
</script>

<style scoped>
.searchable-select {
  position: relative;
}

.input {
  width: 100%;
  border: 1px solid #cbd5e1;
  border-radius: 12px;
  padding: 11px 36px 11px 14px;
  outline: none;
}

.input:focus {
  border-color: #ea580c;
  box-shadow: 0 0 0 2px #fed7aa;
}

.input--disabled {
  background: #f1f5f9;
  color: #94a3b8;
  cursor: not-allowed;
}

.clear-btn {
  position: absolute;
  right: 10px;
  top: 50%;
  transform: translateY(-50%);
  border: none;
  background: transparent;
  color: #94a3b8;
  cursor: pointer;
  font-size: 0.85rem;
  line-height: 1;
  padding: 4px;
}

.clear-btn:hover {
  color: #ea580c;
}

.options {
  position: absolute;
  z-index: 30;
  top: calc(100% + 4px);
  left: 0;
  right: 0;
  max-height: 220px;
  overflow-y: auto;
  background: #fff;
  border: 1px solid #e2e8f0;
  border-radius: 12px;
  box-shadow: 0 8px 24px rgba(15, 23, 42, 0.12);
  list-style: none;
  margin: 0;
  padding: 6px;
}

.option {
  padding: 9px 10px;
  border-radius: 8px;
  cursor: pointer;
  font-size: 0.9rem;
  color: #334155;
}

.option:hover,
.option--highlighted {
  background: #fff7ed;
  color: #ea580c;
}

.option--empty {
  color: #94a3b8;
  cursor: default;
}

.option--empty:hover {
  background: transparent;
  color: #94a3b8;
}
</style>
