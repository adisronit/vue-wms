<template>
  <div class="space-y-6">
    <PageHeader :title="$t('customers.title')" :subtitle="$t('customers.subtitle')">
      <template #actions>
        <button
          @click="openCustomer(null)"
          class="bg-orange-600 hover:bg-orange-700 text-white px-5 py-3 rounded-xl font-semibold"
        >
          {{ $t('customers.newButton') }}
        </button>
      </template>
    </PageHeader>

    <FilterBar :cols="3">
      <input
        v-model="filters.keyword"
        type="text"
        class="input"
        :placeholder="$t('customers.filters.searchPlaceholder')"
      />

      <select v-model="filters.status" class="input">
        <option value="">{{ $t('customers.filters.allStatus') }}</option>
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
      :is-empty="customers.length === 0"
      :loading-text="$t('customers.loading')"
      :empty-text="$t('customers.empty')"
      :colspan="6"
      min-width="1050px"
    >
      <template #head>
        <tr>
          <th class="table-th">{{ $t('customers.table.code') }}</th>
          <th class="table-th">{{ $t('customers.table.companyName') }}</th>
          <th class="table-th">{{ $t('customers.table.taxId') }}</th>
          <th class="table-th">{{ $t('customers.table.contactPerson') }}</th>
          <th class="table-th">{{ $t('customers.table.status') }}</th>
          <th class="table-th text-right">{{ $t('customers.table.action') }}</th>
        </tr>
      </template>

      <tr
        v-for="customer in customers"
        :key="customer.id"
        class="border-t hover:bg-orange-50"
      >
        <td class="table-td font-semibold text-slate-800">
          {{ customer.code }}
        </td>

        <td class="table-td">
          {{ customer.company_name }}
        </td>

        <td class="table-td">
          {{ customer.tax_id || '—' }}
        </td>

        <td class="table-td">
          {{ customer.contact_person || '—' }}
        </td>

        <td class="table-td">
          <StatusBadge :tone="statusTone(customer.status)">
            {{ statusLabel(customer.status) }}
          </StatusBadge>
        </td>

        <td class="table-td text-right space-x-3">
          <button
            @click="openCustomer(customer)"
            class="text-orange-600 hover:text-orange-700 font-semibold"
          >
            {{ $t('common.buttons.edit') }}
          </button>
          <button
            @click="askDelete(customer)"
            class="text-red-600 hover:text-red-700 font-semibold"
          >
            {{ $t('common.buttons.delete') }}
          </button>
        </td>
      </tr>
    </DataTable>

    <div
      v-if="selectedCustomer"
      class="fixed inset-0 bg-black/40 flex justify-end z-50"
    >
      <div class="w-full max-w-2xl bg-white h-full p-6 overflow-y-auto">
        <div class="flex items-center justify-between mb-6">
          <div>
            <h2 class="text-xl font-bold text-slate-800">
              {{ selectedCustomer.id ? $t('customers.drawer.detailTitle') : $t('customers.drawer.newTitle') }}
            </h2>
            <p class="text-slate-500">
              {{ $t('customers.drawer.subtitle') }}
            </p>
          </div>

          <button
            @click="selectedCustomer = null"
            class="text-slate-500 hover:text-red-600 text-xl"
          >
            ✕
          </button>
        </div>

        <FormSection>
          <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div>
              <label class="label">{{ $t('customers.fields.code') }}</label>
              <input v-model="customerForm.code" type="text" class="input" />
            </div>

            <div>
              <label class="label">{{ $t('customers.fields.companyName') }}</label>
              <input v-model="customerForm.companyName" type="text" class="input" />
            </div>

            <div>
              <label class="label">{{ $t('customers.fields.taxId') }}</label>
              <input v-model="customerForm.taxId" type="text" class="input" />
            </div>

            <div>
              <label class="label">{{ $t('customers.fields.contactPerson') }}</label>
              <input v-model="customerForm.contactPerson" type="text" class="input" />
            </div>

            <div>
              <label class="label">{{ $t('customers.fields.contactPhone') }}</label>
              <input v-model="customerForm.contactPhone" type="text" class="input" />
            </div>

            <div>
              <label class="label">{{ $t('customers.fields.email') }}</label>
              <input v-model="customerForm.email" type="email" class="input" />
            </div>

            <div>
              <label class="label">{{ $t('customers.fields.phone') }}</label>
              <input v-model="customerForm.phone" type="text" class="input" />
            </div>

            <div>
              <label class="label">{{ $t('customers.fields.status') }}</label>
              <select v-model="customerForm.status" class="input">
                <option value="ACTIVE">{{ $t('common.status.active') }}</option>
                <option value="INACTIVE">{{ $t('common.status.inactive') }}</option>
              </select>
            </div>
          </div>

          <div>
            <label class="label">{{ $t('customers.fields.address') }}</label>
            <textarea v-model="customerForm.address" rows="3" class="input"></textarea>
          </div>
        </FormSection>

        <div v-if="formError" class="bg-red-50 border border-red-200 text-red-600 rounded-xl p-3 text-sm mb-6">
          {{ formError }}
        </div>

        <div class="flex flex-col md:flex-row justify-end gap-3">
          <button
            @click="selectedCustomer = null"
            class="bg-white hover:bg-slate-100 text-slate-700 border border-slate-300 px-6 py-3 rounded-xl font-semibold"
          >
            {{ $t('common.buttons.cancel') }}
          </button>

          <button
            @click="saveCustomer"
            :disabled="saving"
            class="bg-orange-600 hover:bg-orange-700 text-white px-6 py-3 rounded-xl font-semibold disabled:opacity-60"
          >
            {{ saving ? $t('common.buttons.submitting') : $t('common.buttons.save') }}
          </button>
        </div>
      </div>
    </div>

    <ConfirmDialog
      :open="!!customerToDelete"
      :title="$t('customers.deleteTitle')"
      :message="$t('common.messages.deleteConfirm')"
      :confirm-label="$t('common.buttons.delete')"
      :cancel-label="$t('common.buttons.cancel')"
      :loading-label="$t('common.buttons.submitting')"
      danger
      :loading="deleting"
      :error-message="deleteError"
      @cancel="customerToDelete = null; deleteError = ''"
      @confirm="confirmDelete"
    />
  </div>
</template>

<script>
import {
  listCustomers,
  createCustomer,
  updateCustomer,
  deleteCustomer
} from '../../services/customers'
import { showToast } from '../../services/toast'
import PageHeader from '../../components/ui/PageHeader.vue'
import FilterBar from '../../components/ui/FilterBar.vue'
import DataTable from '../../components/ui/DataTable.vue'
import StatusBadge from '../../components/ui/StatusBadge.vue'
import FormSection from '../../components/ui/FormSection.vue'
import ConfirmDialog from '../../components/ui/ConfirmDialog.vue'

let searchDebounce = null

export default {
  name: 'CustomerView',

  components: { PageHeader, FilterBar, DataTable, StatusBadge, FormSection, ConfirmDialog },

  data() {
    return {
      customers: [],
      loading: false,
      listError: '',
      saving: false,
      formError: '',
      selectedCustomer: null,
      customerForm: this.createEmptyForm(),
      customerToDelete: null,
      deleting: false,
      deleteError: '',
      filters: {
        keyword: '',
        status: ''
      }
    }
  },

  watch: {
    'filters.status'() {
      this.fetchCustomers()
    },
    'filters.keyword'() {
      clearTimeout(searchDebounce)
      searchDebounce = setTimeout(() => this.fetchCustomers(), 300)
    }
  },

  mounted() {
    this.fetchCustomers()
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

    async fetchCustomers() {
      this.loading = true
      this.listError = ''
      try {
        const res = await listCustomers({
          search: this.filters.keyword,
          status: this.filters.status,
          limit: 100
        })
        this.customers = res.data
      } catch (err) {
        this.listError = err.message || this.$t('customers.messages.loadFailed')
      } finally {
        this.loading = false
      }
    },

    openCustomer(customer) {
      this.formError = ''
      this.selectedCustomer = customer || {}

      this.customerForm = customer
        ? {
            code: customer.code,
            companyName: customer.company_name,
            taxId: customer.tax_id || '',
            contactPerson: customer.contact_person || '',
            contactPhone: customer.contact_phone || '',
            address: customer.address || '',
            email: customer.email || '',
            phone: customer.phone || '',
            status: customer.status
          }
        : this.createEmptyForm()
    },

    clearFilter() {
      this.filters.keyword = ''
      this.filters.status = ''
    },

    async saveCustomer() {
      this.formError = ''

      if (!this.customerForm.code || !this.customerForm.companyName) {
        this.formError = this.$t('common.messages.requiredField')
        return
      }

      this.saving = true
      const payload = {
        code: this.customerForm.code,
        company_name: this.customerForm.companyName,
        tax_id: this.customerForm.taxId || null,
        contact_person: this.customerForm.contactPerson || null,
        contact_phone: this.customerForm.contactPhone || null,
        address: this.customerForm.address || null,
        email: this.customerForm.email || null,
        phone: this.customerForm.phone || null,
        status: this.customerForm.status
      }

      try {
        if (this.selectedCustomer.id) {
          await updateCustomer(this.selectedCustomer.id, payload)
        } else {
          await createCustomer(payload)
        }

        this.selectedCustomer = null
        showToast(this.$t('common.messages.saveSuccess'), 'success')
        await this.fetchCustomers()
      } catch (err) {
        this.formError = err.message || this.$t('common.messages.saveFailed')
      } finally {
        this.saving = false
      }
    },

    askDelete(customer) {
      this.deleteError = ''
      this.customerToDelete = customer
    },

    async confirmDelete() {
      if (!this.customerToDelete) return
      this.deleting = true
      this.deleteError = ''

      try {
        await deleteCustomer(this.customerToDelete.id)
        this.customerToDelete = null
        showToast(this.$t('common.messages.deleteSuccess'), 'success')
        await this.fetchCustomers()
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
