<template>
  <div class="space-y-6">
    <PageHeader :title="$t('brands.title')" :subtitle="$t('brands.subtitle')">
      <template #actions>
        <button @click="openBrand(null)" class="bg-orange-600 hover:bg-orange-700 text-white px-5 py-3 rounded-xl font-semibold">
          {{ $t('brands.newButton') }}
        </button>
      </template>
    </PageHeader>

    <FilterBar :cols="3">
      <input v-model="filters.keyword" type="text" class="input" :placeholder="$t('brands.filters.searchPlaceholder')" />
      <select v-model="filters.status" class="input">
        <option value="">{{ $t('brands.filters.allStatus') }}</option>
        <option value="ACTIVE">{{ $t('common.status.active') }}</option>
        <option value="INACTIVE">{{ $t('common.status.inactive') }}</option>
      </select>
      <button @click="clearFilter" class="bg-slate-100 hover:bg-slate-200 text-slate-700 rounded-xl font-semibold">
        {{ $t('common.buttons.clearFilter') }}
      </button>
    </FilterBar>

    <div v-if="listError" class="bg-red-50 border border-red-200 text-red-600 rounded-xl p-3 text-sm">{{ listError }}</div>

    <DataTable :loading="loading" :is-empty="brands.length === 0" :loading-text="$t('brands.loading')" :empty-text="$t('brands.empty')" :colspan="4" min-width="700px">
      <template #head>
        <tr>
          <th class="table-th">{{ $t('brands.table.code') }}</th>
          <th class="table-th">{{ $t('brands.table.name') }}</th>
          <th class="table-th">{{ $t('brands.table.status') }}</th>
          <th class="table-th text-right">{{ $t('brands.table.action') }}</th>
        </tr>
      </template>

      <tr v-for="brand in brands" :key="brand.id" class="border-t hover:bg-orange-50">
        <td class="table-td font-semibold text-slate-800">{{ brand.code }}</td>
        <td class="table-td">{{ brand.name }}</td>
        <td class="table-td"><StatusBadge :tone="statusTone(brand.status)">{{ statusLabel(brand.status) }}</StatusBadge></td>
        <td class="table-td text-right space-x-3">
          <button @click="openBrand(brand)" class="text-orange-600 hover:text-orange-700 font-semibold">{{ $t('common.buttons.edit') }}</button>
          <button @click="askDelete(brand)" class="text-red-600 hover:text-red-700 font-semibold">{{ $t('common.buttons.delete') }}</button>
        </td>
      </tr>
    </DataTable>

    <div v-if="selectedBrand" class="fixed inset-0 bg-black/40 flex justify-end z-50">
      <div class="w-full max-w-md bg-white h-full p-6 overflow-y-auto">
        <div class="flex items-center justify-between mb-6">
          <div>
            <h2 class="text-xl font-bold text-slate-800">{{ selectedBrand.id ? $t('brands.drawer.detailTitle') : $t('brands.drawer.newTitle') }}</h2>
            <p class="text-slate-500">{{ $t('brands.drawer.subtitle') }}</p>
          </div>
          <button @click="selectedBrand = null" class="text-slate-500 hover:text-red-600 text-xl">✕</button>
        </div>

        <FormSection>
          <div>
            <label class="label">{{ $t('brands.fields.code') }}</label>
            <input v-model="brandForm.code" type="text" class="input" />
          </div>
          <div>
            <label class="label">{{ $t('brands.fields.name') }}</label>
            <input v-model="brandForm.name" type="text" class="input" />
          </div>
          <div>
            <label class="label">{{ $t('brands.fields.status') }}</label>
            <select v-model="brandForm.status" class="input">
              <option value="ACTIVE">{{ $t('common.status.active') }}</option>
              <option value="INACTIVE">{{ $t('common.status.inactive') }}</option>
            </select>
          </div>
        </FormSection>

        <div v-if="formError" class="bg-red-50 border border-red-200 text-red-600 rounded-xl p-3 text-sm mb-6">{{ formError }}</div>

        <div class="flex flex-col md:flex-row justify-end gap-3">
          <button @click="selectedBrand = null" class="bg-white hover:bg-slate-100 text-slate-700 border border-slate-300 px-6 py-3 rounded-xl font-semibold">
            {{ $t('common.buttons.cancel') }}
          </button>
          <button @click="saveBrand" :disabled="saving" class="bg-orange-600 hover:bg-orange-700 text-white px-6 py-3 rounded-xl font-semibold disabled:opacity-60">
            {{ saving ? $t('common.buttons.submitting') : $t('common.buttons.save') }}
          </button>
        </div>
      </div>
    </div>

    <ConfirmDialog
      :open="!!brandToDelete"
      :title="$t('brands.deleteTitle')"
      :message="$t('common.messages.deleteConfirm')"
      :confirm-label="$t('common.buttons.delete')"
      :cancel-label="$t('common.buttons.cancel')"
      :loading-label="$t('common.buttons.submitting')"
      danger
      :loading="deleting"
      :error-message="deleteError"
      @cancel="brandToDelete = null; deleteError = ''"
      @confirm="confirmDelete"
    />
  </div>
</template>

<script>
import { listBrands, createBrand, updateBrand, deleteBrand } from '../../services/brands'
import { showToast } from '../../services/toast'
import PageHeader from '../../components/ui/PageHeader.vue'
import FilterBar from '../../components/ui/FilterBar.vue'
import DataTable from '../../components/ui/DataTable.vue'
import StatusBadge from '../../components/ui/StatusBadge.vue'
import FormSection from '../../components/ui/FormSection.vue'
import ConfirmDialog from '../../components/ui/ConfirmDialog.vue'

let searchDebounce = null

// Mirrors SupplierView.vue's own shape exactly (same minimal master-
// data CRUD pattern), just a narrower field set (code/name/status —
// no contact/address fields, since nothing in the EWTA requirements
// asks for them on a Brand).
export default {
  name: 'BrandView',

  components: { PageHeader, FilterBar, DataTable, StatusBadge, FormSection, ConfirmDialog },

  data() {
    return {
      brands: [],
      loading: false,
      listError: '',
      saving: false,
      formError: '',
      selectedBrand: null,
      brandForm: this.createEmptyForm(),
      brandToDelete: null,
      deleting: false,
      deleteError: '',
      filters: { keyword: '', status: '' }
    }
  },

  watch: {
    'filters.status'() {
      this.fetchBrands()
    },
    'filters.keyword'() {
      clearTimeout(searchDebounce)
      searchDebounce = setTimeout(() => this.fetchBrands(), 300)
    }
  },

  mounted() {
    this.fetchBrands()
  },

  methods: {
    createEmptyForm() {
      return { code: '', name: '', status: 'ACTIVE' }
    },

    async fetchBrands() {
      this.loading = true
      this.listError = ''
      try {
        const res = await listBrands({ search: this.filters.keyword, status: this.filters.status, limit: 100 })
        this.brands = res.data
      } catch (err) {
        this.listError = err.message || this.$t('brands.messages.loadFailed')
      } finally {
        this.loading = false
      }
    },

    openBrand(brand) {
      this.formError = ''
      this.selectedBrand = brand || {}
      this.brandForm = brand ? { code: brand.code, name: brand.name, status: brand.status } : this.createEmptyForm()
    },

    clearFilter() {
      this.filters.keyword = ''
      this.filters.status = ''
    },

    async saveBrand() {
      this.formError = ''
      if (!this.brandForm.code || !this.brandForm.name) {
        this.formError = this.$t('common.messages.requiredField')
        return
      }

      this.saving = true
      const payload = { code: this.brandForm.code, name: this.brandForm.name, status: this.brandForm.status }

      try {
        if (this.selectedBrand.id) {
          await updateBrand(this.selectedBrand.id, payload)
        } else {
          await createBrand(payload)
        }
        this.selectedBrand = null
        showToast(this.$t('common.messages.saveSuccess'), 'success')
        await this.fetchBrands()
      } catch (err) {
        this.formError = err.message || this.$t('common.messages.saveFailed')
      } finally {
        this.saving = false
      }
    },

    askDelete(brand) {
      this.deleteError = ''
      this.brandToDelete = brand
    },

    async confirmDelete() {
      if (!this.brandToDelete) return
      this.deleting = true
      this.deleteError = ''
      try {
        await deleteBrand(this.brandToDelete.id)
        this.brandToDelete = null
        showToast(this.$t('common.messages.deleteSuccess'), 'success')
        await this.fetchBrands()
      } catch (err) {
        this.deleteError = err.message || this.$t('common.messages.deleteFailed')
      } finally {
        this.deleting = false
      }
    },

    statusTone(status) {
      return status === 'ACTIVE' ? 'green' : 'gray'
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
