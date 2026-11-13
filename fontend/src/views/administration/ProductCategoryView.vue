<template>
  <div class="space-y-6">
    <PageHeader :title="$t('productCategory.title')" :subtitle="$t('productCategory.subtitle')">
      <template #actions>
        <button
          @click="openCategory(null)"
          class="bg-orange-600 hover:bg-orange-700 text-white px-5 py-3 rounded-xl font-semibold"
        >
          {{ $t('productCategory.newButton') }}
        </button>
      </template>
    </PageHeader>

    <FilterBar :cols="4">
      <input
        v-model="filters.keyword"
        type="text"
        class="input"
        :placeholder="$t('productCategory.filters.searchPlaceholder')"
      />

      <select v-model="filters.parentId" class="input">
        <option value="">{{ $t('productCategory.filters.allParents') }}</option>
        <option v-for="cat in allCategories" :key="cat.id" :value="cat.id">
          {{ cat.code }} - {{ cat.name }}
        </option>
      </select>

      <select v-model="filters.status" class="input">
        <option value="">{{ $t('productCategory.filters.allStatus') }}</option>
        <option value="ACTIVE">{{ $t('common.status.active') }}</option>
        <option value="INACTIVE">{{ $t('common.status.inactive') }}</option>
      </select>

      <button
        @click="clearFilter"
        class="bg-slate-100 hover:bg-slate-200 text-slate-700 rounded-xl font-semibold"
      >
        {{ $t('common.buttons.clearFilter') }}
      </button>
    </FilterBar>

    <div v-if="listError" class="bg-red-50 border border-red-200 text-red-600 rounded-xl p-3 text-sm">
      {{ listError }}
    </div>

    <DataTable
      :loading="loading"
      :is-empty="categories.length === 0"
      :loading-text="$t('productCategory.loading')"
      :empty-text="$t('productCategory.empty')"
      :colspan="6"
      min-width="950px"
    >
      <template #head>
        <tr>
          <th class="table-th">{{ $t('productCategory.table.code') }}</th>
          <th class="table-th">{{ $t('productCategory.table.name') }}</th>
          <th class="table-th">{{ $t('productCategory.table.parent') }}</th>
          <th class="table-th">{{ $t('productCategory.table.description') }}</th>
          <th class="table-th">{{ $t('productCategory.table.status') }}</th>
          <th class="table-th text-right">{{ $t('productCategory.table.action') }}</th>
        </tr>
      </template>

      <tr
        v-for="category in categories"
        :key="category.id"
        class="border-t hover:bg-orange-50"
      >
        <td class="table-td font-semibold text-slate-800">
          {{ category.code }}
        </td>

        <td class="table-td">
          {{ category.name }}
        </td>

        <td class="table-td">
          {{ parentLabel(category.parent_id) }}
        </td>

        <td class="table-td">
          {{ category.description || '—' }}
        </td>

        <td class="table-td">
          <StatusBadge :tone="statusTone(category.status)">
            {{ statusLabel(category.status) }}
          </StatusBadge>
        </td>

        <td class="table-td text-right space-x-3">
          <button
            @click="openCategory(category)"
            class="text-orange-600 hover:text-orange-700 font-semibold"
          >
            {{ $t('common.buttons.edit') }}
          </button>
          <button
            @click="askDelete(category)"
            class="text-red-600 hover:text-red-700 font-semibold"
          >
            {{ $t('common.buttons.delete') }}
          </button>
        </td>
      </tr>
    </DataTable>

    <div
      v-if="selectedCategory"
      class="fixed inset-0 bg-black/40 flex justify-end z-50"
    >
      <div class="w-full max-w-2xl bg-white h-full p-6 overflow-y-auto">
        <div class="flex items-center justify-between mb-6">
          <div>
            <h2 class="text-xl font-bold text-slate-800">
              {{ selectedCategory.id ? $t('productCategory.drawer.detailTitle') : $t('productCategory.drawer.newTitle') }}
            </h2>
            <p class="text-slate-500">
              {{ $t('productCategory.drawer.subtitle') }}
            </p>
          </div>

          <button
            @click="selectedCategory = null"
            class="text-slate-500 hover:text-red-600 text-xl"
          >
            ✕
          </button>
        </div>

        <FormSection>
          <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div>
              <label class="label">{{ $t('productCategory.fields.code') }}</label>
              <input v-model="categoryForm.code" type="text" class="input" />
            </div>

            <div>
              <label class="label">{{ $t('productCategory.fields.name') }}</label>
              <input v-model="categoryForm.name" type="text" class="input" />
            </div>

            <div>
              <label class="label">{{ $t('productCategory.fields.parent') }}</label>
              <select v-model="categoryForm.parentId" class="input">
                <option value="">{{ $t('productCategory.fields.noParentOption') }}</option>
                <option v-for="cat in availableParents" :key="cat.id" :value="cat.id">
                  {{ cat.code }} - {{ cat.name }}
                </option>
              </select>
            </div>

            <div>
              <label class="label">{{ $t('productCategory.fields.status') }}</label>
              <select v-model="categoryForm.status" class="input">
                <option value="ACTIVE">{{ $t('common.status.active') }}</option>
                <option value="INACTIVE">{{ $t('common.status.inactive') }}</option>
              </select>
            </div>
          </div>

          <div>
            <label class="label">{{ $t('productCategory.fields.description') }}</label>
            <textarea v-model="categoryForm.description" rows="3" class="input"></textarea>
          </div>
        </FormSection>

        <div v-if="formError" class="bg-red-50 border border-red-200 text-red-600 rounded-xl p-3 text-sm mb-6">
          {{ formError }}
        </div>

        <div class="flex flex-col md:flex-row justify-end gap-3">
          <button
            @click="selectedCategory = null"
            class="bg-white hover:bg-slate-100 text-slate-700 border border-slate-300 px-6 py-3 rounded-xl font-semibold"
          >
            {{ $t('common.buttons.cancel') }}
          </button>

          <button
            @click="saveCategory"
            :disabled="saving"
            class="bg-orange-600 hover:bg-orange-700 text-white px-6 py-3 rounded-xl font-semibold disabled:opacity-60"
          >
            {{ saving ? $t('common.buttons.submitting') : $t('common.buttons.save') }}
          </button>
        </div>
      </div>
    </div>

    <ConfirmDialog
      :open="!!categoryToDelete"
      :title="$t('productCategory.deleteTitle')"
      :message="$t('common.messages.deleteConfirm')"
      :confirm-label="$t('common.buttons.delete')"
      :cancel-label="$t('common.buttons.cancel')"
      :loading-label="$t('common.buttons.submitting')"
      danger
      :loading="deleting"
      :error-message="deleteError"
      @cancel="categoryToDelete = null; deleteError = ''"
      @confirm="confirmDelete"
    />
  </div>
</template>

<script>
import {
  listProductCategories,
  createProductCategory,
  updateProductCategory,
  deleteProductCategory
} from '../../services/productCategories'
import { showToast } from '../../services/toast'
import PageHeader from '../../components/ui/PageHeader.vue'
import FilterBar from '../../components/ui/FilterBar.vue'
import DataTable from '../../components/ui/DataTable.vue'
import StatusBadge from '../../components/ui/StatusBadge.vue'
import FormSection from '../../components/ui/FormSection.vue'
import ConfirmDialog from '../../components/ui/ConfirmDialog.vue'

let searchDebounce = null

export default {
  name: 'ProductCategoryView',

  components: { PageHeader, FilterBar, DataTable, StatusBadge, FormSection, ConfirmDialog },

  data() {
    return {
      categories: [],
      // Fetched once, unfiltered — used to resolve parent_id -> label
      // for the table and to populate the parent dropdowns, since there
      // is no separate "get parent name" endpoint and the main list
      // above may be narrowed by search/status/parent filters.
      allCategories: [],
      loading: false,
      listError: '',
      saving: false,
      formError: '',
      selectedCategory: null,
      categoryForm: this.createEmptyForm(),
      categoryToDelete: null,
      deleting: false,
      deleteError: '',
      filters: {
        keyword: '',
        parentId: '',
        status: ''
      }
    }
  },

  computed: {
    // Excludes the category being edited and any of its own descendants
    // from the parent dropdown — selecting either would be an obviously
    // doomed circular request. The backend is still the real enforcer
    // (INVALID_PARENT), this is just a UX nicety.
    availableParents() {
      if (!this.selectedCategory || !this.selectedCategory.id) return this.allCategories
      const blocked = this.descendantIds(this.selectedCategory.id)
      blocked.add(this.selectedCategory.id)
      return this.allCategories.filter((c) => !blocked.has(c.id))
    }
  },

  watch: {
    'filters.parentId'() {
      this.fetchCategories()
    },
    'filters.status'() {
      this.fetchCategories()
    },
    'filters.keyword'() {
      clearTimeout(searchDebounce)
      searchDebounce = setTimeout(() => this.fetchCategories(), 300)
    }
  },

  mounted() {
    this.fetchAllCategories()
    this.fetchCategories()
  },

  methods: {
    createEmptyForm() {
      return {
        code: '',
        name: '',
        parentId: '',
        description: '',
        status: 'ACTIVE'
      }
    },

    async fetchCategories() {
      this.loading = true
      this.listError = ''
      try {
        const res = await listProductCategories({
          search: this.filters.keyword,
          parentId: this.filters.parentId,
          status: this.filters.status,
          limit: 100
        })
        this.categories = res.data
      } catch (err) {
        this.listError = err.message || this.$t('productCategory.messages.loadFailed')
      } finally {
        this.loading = false
      }
    },

    // Unfiltered fetch for parent-name resolution and dropdown options —
    // independent of whatever search/status/parent filters are active
    // on the main list above.
    async fetchAllCategories() {
      try {
        const res = await listProductCategories({ limit: 200 })
        this.allCategories = res.data
      } catch (err) {
        this.listError = err.message || this.$t('productCategory.messages.loadFailed')
      }
    },

    descendantIds(id) {
      const result = new Set()
      const queue = [id]
      while (queue.length) {
        const current = queue.pop()
        this.allCategories.forEach((c) => {
          if (c.parent_id === current && !result.has(c.id)) {
            result.add(c.id)
            queue.push(c.id)
          }
        })
      }
      return result
    },

    parentLabel(parentId) {
      if (!parentId) return this.$t('productCategory.noParentLabel')
      const parent = this.allCategories.find((c) => c.id === parentId)
      return parent ? `${parent.code} - ${parent.name}` : this.$t('productCategory.noParentLabel')
    },

    openCategory(category) {
      this.formError = ''
      this.selectedCategory = category || {}

      this.categoryForm = category
        ? {
            code: category.code,
            name: category.name,
            parentId: category.parent_id || '',
            description: category.description || '',
            status: category.status
          }
        : this.createEmptyForm()
    },

    clearFilter() {
      this.filters.keyword = ''
      this.filters.parentId = ''
      this.filters.status = ''
    },

    async saveCategory() {
      this.formError = ''

      if (!this.categoryForm.code) {
        this.formError = this.$t('common.messages.requiredField')
        return
      }
      if (!this.categoryForm.name) {
        this.formError = this.$t('common.messages.requiredField')
        return
      }

      this.saving = true
      const payload = {
        code: this.categoryForm.code,
        name: this.categoryForm.name,
        parent_id: this.categoryForm.parentId || null,
        description: this.categoryForm.description || null,
        status: this.categoryForm.status
      }

      try {
        if (this.selectedCategory.id) {
          await updateProductCategory(this.selectedCategory.id, payload)
        } else {
          await createProductCategory(payload)
        }

        this.selectedCategory = null
        showToast(this.$t('common.messages.saveSuccess'), 'success')
        await Promise.all([this.fetchCategories(), this.fetchAllCategories()])
      } catch (err) {
        this.formError = err.message || this.$t('common.messages.saveFailed')
      } finally {
        this.saving = false
      }
    },

    askDelete(category) {
      this.deleteError = ''
      this.categoryToDelete = category
    },

    async confirmDelete() {
      if (!this.categoryToDelete) return
      this.deleting = true
      this.deleteError = ''

      try {
        await deleteProductCategory(this.categoryToDelete.id)
        this.categoryToDelete = null
        showToast(this.$t('common.messages.deleteSuccess'), 'success')
        await Promise.all([this.fetchCategories(), this.fetchAllCategories()])
      } catch (err) {
        // 409 HAS_CHILDREN / HAS_PRODUCTS land here — the server's own
        // message is shown as-is rather than retried automatically.
        this.deleteError = err.message || this.$t('common.messages.deleteFailed')
      } finally {
        this.deleting = false
      }
    },

    statusTone(status) {
      if (status === 'ACTIVE') return 'green'
      if (status === 'INACTIVE') return 'gray'
      return 'gray'
    },

    statusLabel(status) {
      if (status === 'ACTIVE') return this.$t('common.status.active')
      if (status === 'INACTIVE') return this.$t('common.status.inactive')
      return status
    }
  }
}
</script>

<style scoped>
.input {
  width: 100%;
  border: 1px solid #cbd5e1;
  border-radius: 12px;
  padding: 11px 14px;
  outline: none;
}

.input:focus {
  border-color: #ea580c;
  box-shadow: 0 0 0 2px #fed7aa;
}

.label {
  display: block;
  margin-bottom: 8px;
  color: #334155;
  font-weight: var(--fw-label);
  font-size: var(--fs-label);
}

.table-th {
  padding: 14px 16px;
  font-weight: var(--fw-table-header);
  font-size: var(--fs-table-header);
  background: #f8fafc;
  border-bottom: 1px solid #e2e8f0;
  color: #475569;
  white-space: nowrap;
}

.table-td {
  padding: 14px 16px;
  border-bottom: 1px solid #f1f5f9;
  vertical-align: middle;
  color: #475569;
}
</style>
