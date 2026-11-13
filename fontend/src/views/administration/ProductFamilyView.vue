<template>
  <div class="space-y-6">
    <PageHeader :title="$t('productFamilies.title')" :subtitle="$t('productFamilies.subtitle')">
      <template #actions>
        <button @click="openFamily(null)" class="bg-orange-600 hover:bg-orange-700 text-white px-5 py-3 rounded-xl font-semibold">
          {{ $t('productFamilies.newButton') }}
        </button>
      </template>
    </PageHeader>

    <FilterBar :cols="3">
      <input v-model="filters.keyword" type="text" class="input" :placeholder="$t('productFamilies.filters.searchPlaceholder')" />
      <select v-model="filters.status" class="input">
        <option value="">{{ $t('productFamilies.filters.allStatus') }}</option>
        <option value="ACTIVE">{{ $t('common.status.active') }}</option>
        <option value="INACTIVE">{{ $t('common.status.inactive') }}</option>
      </select>
      <button @click="clearFilter" class="bg-slate-100 hover:bg-slate-200 text-slate-700 rounded-xl font-semibold">
        {{ $t('common.buttons.clearFilter') }}
      </button>
    </FilterBar>

    <div v-if="listError" class="bg-red-50 border border-red-200 text-red-600 rounded-xl p-3 text-sm">{{ listError }}</div>

    <DataTable :loading="loading" :is-empty="families.length === 0" :loading-text="$t('productFamilies.loading')" :empty-text="$t('productFamilies.empty')" :colspan="4" min-width="700px">
      <template #head>
        <tr>
          <th class="table-th">{{ $t('productFamilies.table.code') }}</th>
          <th class="table-th">{{ $t('productFamilies.table.name') }}</th>
          <th class="table-th">{{ $t('productFamilies.table.status') }}</th>
          <th class="table-th text-right">{{ $t('productFamilies.table.action') }}</th>
        </tr>
      </template>

      <tr v-for="family in families" :key="family.id" class="border-t hover:bg-orange-50">
        <td class="table-td font-semibold text-slate-800">{{ family.code }}</td>
        <td class="table-td">{{ family.name }}</td>
        <td class="table-td"><StatusBadge :tone="statusTone(family.status)">{{ statusLabel(family.status) }}</StatusBadge></td>
        <td class="table-td text-right space-x-3">
          <button @click="openFamily(family)" class="text-orange-600 hover:text-orange-700 font-semibold">{{ $t('common.buttons.edit') }}</button>
          <button @click="askDelete(family)" class="text-red-600 hover:text-red-700 font-semibold">{{ $t('common.buttons.delete') }}</button>
        </td>
      </tr>
    </DataTable>

    <div v-if="selectedFamily" class="fixed inset-0 bg-black/40 flex justify-end z-50">
      <div class="w-full max-w-md bg-white h-full p-6 overflow-y-auto">
        <div class="flex items-center justify-between mb-6">
          <div>
            <h2 class="text-xl font-bold text-slate-800">{{ selectedFamily.id ? $t('productFamilies.drawer.detailTitle') : $t('productFamilies.drawer.newTitle') }}</h2>
            <p class="text-slate-500">{{ $t('productFamilies.drawer.subtitle') }}</p>
          </div>
          <button @click="selectedFamily = null" class="text-slate-500 hover:text-red-600 text-xl">✕</button>
        </div>

        <FormSection>
          <div>
            <label class="label">{{ $t('productFamilies.fields.code') }}</label>
            <input v-model="familyForm.code" type="text" class="input" />
          </div>
          <div>
            <label class="label">{{ $t('productFamilies.fields.name') }}</label>
            <input v-model="familyForm.name" type="text" class="input" />
          </div>
          <div>
            <label class="label">{{ $t('productFamilies.fields.status') }}</label>
            <select v-model="familyForm.status" class="input">
              <option value="ACTIVE">{{ $t('common.status.active') }}</option>
              <option value="INACTIVE">{{ $t('common.status.inactive') }}</option>
            </select>
          </div>
        </FormSection>

        <div v-if="formError" class="bg-red-50 border border-red-200 text-red-600 rounded-xl p-3 text-sm mb-6">{{ formError }}</div>

        <div class="flex flex-col md:flex-row justify-end gap-3">
          <button @click="selectedFamily = null" class="bg-white hover:bg-slate-100 text-slate-700 border border-slate-300 px-6 py-3 rounded-xl font-semibold">
            {{ $t('common.buttons.cancel') }}
          </button>
          <button @click="saveFamily" :disabled="saving" class="bg-orange-600 hover:bg-orange-700 text-white px-6 py-3 rounded-xl font-semibold disabled:opacity-60">
            {{ saving ? $t('common.buttons.submitting') : $t('common.buttons.save') }}
          </button>
        </div>
      </div>
    </div>

    <ConfirmDialog
      :open="!!familyToDelete"
      :title="$t('productFamilies.deleteTitle')"
      :message="$t('common.messages.deleteConfirm')"
      :confirm-label="$t('common.buttons.delete')"
      :cancel-label="$t('common.buttons.cancel')"
      :loading-label="$t('common.buttons.submitting')"
      danger
      :loading="deleting"
      :error-message="deleteError"
      @cancel="familyToDelete = null; deleteError = ''"
      @confirm="confirmDelete"
    />
  </div>
</template>

<script>
import { listProductFamilies, createProductFamily, updateProductFamily, deleteProductFamily } from '../../services/productFamilies'
import { showToast } from '../../services/toast'
import PageHeader from '../../components/ui/PageHeader.vue'
import FilterBar from '../../components/ui/FilterBar.vue'
import DataTable from '../../components/ui/DataTable.vue'
import StatusBadge from '../../components/ui/StatusBadge.vue'
import FormSection from '../../components/ui/FormSection.vue'
import ConfirmDialog from '../../components/ui/ConfirmDialog.vue'

let searchDebounce = null

// Mirrors BrandView.vue's own shape exactly — same minimal master-data
// CRUD pattern (code/name/status), for the Family that groups sellable
// SKUs/variants (e.g. "Cheddar Block" family with 150g/300g variants).
export default {
  name: 'ProductFamilyView',

  components: { PageHeader, FilterBar, DataTable, StatusBadge, FormSection, ConfirmDialog },

  data() {
    return {
      families: [],
      loading: false,
      listError: '',
      saving: false,
      formError: '',
      selectedFamily: null,
      familyForm: this.createEmptyForm(),
      familyToDelete: null,
      deleting: false,
      deleteError: '',
      filters: { keyword: '', status: '' }
    }
  },

  watch: {
    'filters.status'() {
      this.fetchFamilies()
    },
    'filters.keyword'() {
      clearTimeout(searchDebounce)
      searchDebounce = setTimeout(() => this.fetchFamilies(), 300)
    }
  },

  mounted() {
    this.fetchFamilies()
  },

  methods: {
    createEmptyForm() {
      return { code: '', name: '', status: 'ACTIVE' }
    },

    async fetchFamilies() {
      this.loading = true
      this.listError = ''
      try {
        const res = await listProductFamilies({ search: this.filters.keyword, status: this.filters.status, limit: 100 })
        this.families = res.data
      } catch (err) {
        this.listError = err.message || this.$t('productFamilies.messages.loadFailed')
      } finally {
        this.loading = false
      }
    },

    openFamily(family) {
      this.formError = ''
      this.selectedFamily = family || {}
      this.familyForm = family ? { code: family.code, name: family.name, status: family.status } : this.createEmptyForm()
    },

    clearFilter() {
      this.filters.keyword = ''
      this.filters.status = ''
    },

    async saveFamily() {
      this.formError = ''
      if (!this.familyForm.code || !this.familyForm.name) {
        this.formError = this.$t('common.messages.requiredField')
        return
      }

      this.saving = true
      const payload = { code: this.familyForm.code, name: this.familyForm.name, status: this.familyForm.status }

      try {
        if (this.selectedFamily.id) {
          await updateProductFamily(this.selectedFamily.id, payload)
        } else {
          await createProductFamily(payload)
        }
        this.selectedFamily = null
        showToast(this.$t('common.messages.saveSuccess'), 'success')
        await this.fetchFamilies()
      } catch (err) {
        this.formError = err.message || this.$t('common.messages.saveFailed')
      } finally {
        this.saving = false
      }
    },

    askDelete(family) {
      this.deleteError = ''
      this.familyToDelete = family
    },

    async confirmDelete() {
      if (!this.familyToDelete) return
      this.deleting = true
      this.deleteError = ''
      try {
        await deleteProductFamily(this.familyToDelete.id)
        this.familyToDelete = null
        showToast(this.$t('common.messages.deleteSuccess'), 'success')
        await this.fetchFamilies()
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
