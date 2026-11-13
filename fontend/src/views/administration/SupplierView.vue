<template>
  <div class="space-y-6">
    <PageHeader :title="$t('suppliers.title')" :subtitle="$t('suppliers.subtitle')">
      <template #actions>
        <button
          @click="openSupplier(null)"
          class="bg-orange-600 hover:bg-orange-700 text-white px-5 py-3 rounded-xl font-semibold"
        >
          {{ $t('suppliers.newButton') }}
        </button>
      </template>
    </PageHeader>

    <FilterBar :cols="3">
      <input
        v-model="filters.keyword"
        type="text"
        class="input"
        :placeholder="$t('suppliers.filters.searchPlaceholder')"
      />

      <select v-model="filters.status" class="input">
        <option value="">{{ $t('suppliers.filters.allStatus') }}</option>
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
      :is-empty="suppliers.length === 0"
      :loading-text="$t('suppliers.loading')"
      :empty-text="$t('suppliers.empty')"
      :colspan="6"
      min-width="1050px"
    >
      <template #head>
        <tr>
          <SortableTh column="code" :sort-by="sortBy" :sort-dir="sortDir" @sort="toggleSort">
            {{ $t('suppliers.table.code') }}
          </SortableTh>
          <SortableTh column="companyName" :sort-by="sortBy" :sort-dir="sortDir" @sort="toggleSort">
            {{ $t('suppliers.table.companyName') }}
          </SortableTh>
          <th class="table-th">{{ $t('suppliers.table.taxId') }}</th>
          <th class="table-th">{{ $t('suppliers.table.contactPerson') }}</th>
          <SortableTh column="status" :sort-by="sortBy" :sort-dir="sortDir" @sort="toggleSort">
            {{ $t('suppliers.table.status') }}
          </SortableTh>
          <th class="table-th text-right">{{ $t('suppliers.table.action') }}</th>
        </tr>
      </template>

      <tr
        v-for="supplier in suppliers"
        :key="supplier.id"
        class="border-t hover:bg-orange-50"
      >
        <td class="table-td font-semibold text-slate-800">
          {{ supplier.code }}
        </td>

        <td class="table-td">
          {{ supplier.company_name }}
        </td>

        <td class="table-td">
          {{ supplier.tax_id || '—' }}
        </td>

        <td class="table-td">
          {{ supplier.contact_person || '—' }}
        </td>

        <td class="table-td">
          <StatusBadge :tone="statusTone(supplier.status)">
            {{ statusLabel(supplier.status) }}
          </StatusBadge>
        </td>

        <td class="table-td text-right space-x-3">
          <button
            @click="openSupplier(supplier)"
            class="text-orange-600 hover:text-orange-700 font-semibold"
          >
            {{ $t('common.buttons.edit') }}
          </button>
          <button
            @click="askDelete(supplier)"
            class="text-red-600 hover:text-red-700 font-semibold"
          >
            {{ $t('common.buttons.delete') }}
          </button>
        </td>
      </tr>

      <template v-if="!loading && suppliers.length > 0" #footer>
        <PaginationFooter
          :page="page"
          :limit="limit"
          :total="total"
          :total-pages="totalPages"
          :page-size-options="pageSizeOptions"
          @update:page="goToPage"
          @update:limit="limit = $event"
        />
      </template>
    </DataTable>

    <div
      v-if="selectedSupplier"
      class="fixed inset-0 bg-black/40 flex justify-end z-50"
    >
      <div class="w-full max-w-2xl bg-white h-full p-6 overflow-y-auto">
        <div class="flex items-center justify-between mb-6">
          <div>
            <h2 class="text-xl font-bold text-slate-800">
              {{ selectedSupplier.id ? $t('suppliers.drawer.detailTitle') : $t('suppliers.drawer.newTitle') }}
            </h2>
            <p class="text-slate-500">
              {{ $t('suppliers.drawer.subtitle') }}
            </p>
          </div>

          <button
            @click="selectedSupplier = null"
            class="text-slate-500 hover:text-red-600 text-xl"
          >
            ✕
          </button>
        </div>

        <FormSection>
          <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div>
              <label class="label">{{ $t('suppliers.fields.code') }}</label>
              <input v-model="supplierForm.code" type="text" class="input" />
            </div>

            <div>
              <label class="label">{{ $t('suppliers.fields.companyName') }}</label>
              <input v-model="supplierForm.companyName" type="text" class="input" />
            </div>

            <div>
              <label class="label">{{ $t('suppliers.fields.taxId') }}</label>
              <input v-model="supplierForm.taxId" type="text" class="input" />
            </div>

            <div>
              <label class="label">{{ $t('suppliers.fields.contactPerson') }}</label>
              <input v-model="supplierForm.contactPerson" type="text" class="input" />
            </div>

            <div>
              <label class="label">{{ $t('suppliers.fields.contactPhone') }}</label>
              <input v-model="supplierForm.contactPhone" type="text" class="input" />
            </div>

            <div>
              <label class="label">{{ $t('suppliers.fields.email') }}</label>
              <input v-model="supplierForm.email" type="email" class="input" />
            </div>

            <div>
              <label class="label">{{ $t('suppliers.fields.phone') }}</label>
              <input v-model="supplierForm.phone" type="text" class="input" />
            </div>

            <div>
              <label class="label">{{ $t('suppliers.fields.status') }}</label>
              <select v-model="supplierForm.status" class="input">
                <option value="ACTIVE">{{ $t('common.status.active') }}</option>
                <option value="INACTIVE">{{ $t('common.status.inactive') }}</option>
              </select>
            </div>
          </div>

          <div>
            <label class="label">{{ $t('suppliers.fields.address') }}</label>
            <textarea v-model="supplierForm.address" rows="3" class="input"></textarea>
          </div>
        </FormSection>

        <div v-if="formError" class="bg-red-50 border border-red-200 text-red-600 rounded-xl p-3 text-sm mb-6">
          {{ formError }}
        </div>

        <div class="flex flex-col md:flex-row justify-end gap-3">
          <button
            @click="selectedSupplier = null"
            class="bg-white hover:bg-slate-100 text-slate-700 border border-slate-300 px-6 py-3 rounded-xl font-semibold"
          >
            {{ $t('common.buttons.cancel') }}
          </button>

          <button
            @click="saveSupplier"
            :disabled="saving"
            class="bg-orange-600 hover:bg-orange-700 text-white px-6 py-3 rounded-xl font-semibold disabled:opacity-60"
          >
            {{ saving ? $t('common.buttons.submitting') : $t('common.buttons.save') }}
          </button>
        </div>
      </div>
    </div>

    <ConfirmDialog
      :open="!!supplierToDelete"
      :title="$t('suppliers.deleteTitle')"
      :message="$t('common.messages.deleteConfirm')"
      :confirm-label="$t('common.buttons.delete')"
      :cancel-label="$t('common.buttons.cancel')"
      :loading-label="$t('common.buttons.submitting')"
      danger
      :loading="deleting"
      :error-message="deleteError"
      @cancel="supplierToDelete = null; deleteError = ''"
      @confirm="confirmDelete"
    />
  </div>
</template>

<script>
import {
  listSuppliers,
  createSupplier,
  updateSupplier,
  deleteSupplier
} from '../../services/suppliers'
import { showToast } from '../../services/toast'
import PageHeader from '../../components/ui/PageHeader.vue'
import FilterBar from '../../components/ui/FilterBar.vue'
import DataTable from '../../components/ui/DataTable.vue'
import StatusBadge from '../../components/ui/StatusBadge.vue'
import FormSection from '../../components/ui/FormSection.vue'
import ConfirmDialog from '../../components/ui/ConfirmDialog.vue'
import SortableTh from '../../components/ui/SortableTh.vue'
import PaginationFooter from '../../components/ui/PaginationFooter.vue'
import listPagination from '../../mixins/listPagination'

let searchDebounce = null

export default {
  name: 'SupplierView',

  components: { PageHeader, FilterBar, DataTable, StatusBadge, FormSection, ConfirmDialog, SortableTh, PaginationFooter },

  mixins: [listPagination],

  data() {
    return {
      suppliers: [],
      loading: false,
      listError: '',
      saving: false,
      formError: '',
      selectedSupplier: null,
      supplierForm: this.createEmptyForm(),
      supplierToDelete: null,
      deleting: false,
      deleteError: '',
      filters: {
        keyword: '',
        status: ''
      },
      sortBy: 'code',
      sortDir: 'asc'
    }
  },

  watch: {
    'filters.status'() {
      this.resetToFirstPage()
    },
    'filters.keyword'() {
      clearTimeout(searchDebounce)
      searchDebounce = setTimeout(() => this.resetToFirstPage(), 300)
    }
  },

  mounted() {
    this.fetchSuppliers()
  },

  methods: {
    createEmptyForm() {
      return {
        code: '',
        companyName: '',
        taxId: '',
        contactPerson: '',
        contactPhone: '',
        address: '',
        email: '',
        phone: '',
        status: 'ACTIVE'
      }
    },

    // fetchList() is the listPagination mixin's required hook —
    // resetToFirstPage()/goToPage()/toggleSort() all call it. Kept as a
    // thin alias so the public method name every other handler in this
    // file already calls (fetchSuppliers) stays unchanged.
    fetchList() {
      return this.fetchSuppliers()
    },

    async fetchSuppliers() {
      this.loading = true
      this.listError = ''
      try {
        const res = await listSuppliers({
          search: this.filters.keyword,
          status: this.filters.status,
          page: this.page,
          limit: this.limit,
          sortBy: this.sortBy,
          sortDir: this.sortDir
        })
        this.suppliers = res.data
        this.total = res.meta.total
        this.totalPages = res.meta.total_pages
      } catch (err) {
        this.listError = err.message || this.$t('suppliers.messages.loadFailed')
      } finally {
        this.loading = false
      }
    },

    openSupplier(supplier) {
      this.formError = ''
      this.selectedSupplier = supplier || {}

      this.supplierForm = supplier
        ? {
            code: supplier.code,
            companyName: supplier.company_name,
            taxId: supplier.tax_id || '',
            contactPerson: supplier.contact_person || '',
            contactPhone: supplier.contact_phone || '',
            address: supplier.address || '',
            email: supplier.email || '',
            phone: supplier.phone || '',
            status: supplier.status
          }
        : this.createEmptyForm()
    },

    clearFilter() {
      this.filters.keyword = ''
      this.filters.status = ''
    },

    async saveSupplier() {
      this.formError = ''

      if (!this.supplierForm.code || !this.supplierForm.companyName) {
        this.formError = this.$t('common.messages.requiredField')
        return
      }

      this.saving = true
      const payload = {
        code: this.supplierForm.code,
        company_name: this.supplierForm.companyName,
        tax_id: this.supplierForm.taxId || null,
        contact_person: this.supplierForm.contactPerson || null,
        contact_phone: this.supplierForm.contactPhone || null,
        address: this.supplierForm.address || null,
        email: this.supplierForm.email || null,
        phone: this.supplierForm.phone || null,
        status: this.supplierForm.status
      }

      try {
        if (this.selectedSupplier.id) {
          await updateSupplier(this.selectedSupplier.id, payload)
        } else {
          await createSupplier(payload)
        }

        this.selectedSupplier = null
        showToast(this.$t('common.messages.saveSuccess'), 'success')
        await this.fetchSuppliers()
      } catch (err) {
        this.formError = err.message || this.$t('common.messages.saveFailed')
      } finally {
        this.saving = false
      }
    },

    askDelete(supplier) {
      this.deleteError = ''
      this.supplierToDelete = supplier
    },

    async confirmDelete() {
      if (!this.supplierToDelete) return
      this.deleting = true
      this.deleteError = ''

      try {
        await deleteSupplier(this.supplierToDelete.id)
        this.supplierToDelete = null
        showToast(this.$t('common.messages.deleteSuccess'), 'success')
        await this.fetchSuppliers()
      } catch (err) {
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
