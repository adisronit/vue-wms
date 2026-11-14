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

    <FilterBar :cols="4">
      <input
        v-model="filters.keyword"
        type="text"
        class="input"
        :placeholder="$t('suppliers.filters.searchPlaceholder')"
      />

      <select v-model="filters.supplierType" class="input">
        <option value="">{{ $t('suppliers.filters.allTypes') }}</option>
        <option value="DOMESTIC">{{ $t('suppliers.fields.typeDomestic') }}</option>
        <option value="OVERSEAS">{{ $t('suppliers.fields.typeOverseas') }}</option>
      </select>

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
      :colspan="7"
      min-width="1100px"
    >
      <template #head>
        <tr>
          <SortableTh column="code" :sort-by="sortBy" :sort-dir="sortDir" @sort="toggleSort">
            {{ $t('suppliers.table.code') }}
          </SortableTh>
          <SortableTh column="companyName" :sort-by="sortBy" :sort-dir="sortDir" @sort="toggleSort">
            {{ $t('suppliers.table.companyName') }}
          </SortableTh>
          <th class="table-th">{{ $t('suppliers.table.type') }}</th>
          <th class="table-th">{{ $t('suppliers.table.country') }}</th>
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
          <StatusBadge :tone="typeTone(supplier.supplier_type)">
            {{ typeLabel(supplier.supplier_type) }}
          </StatusBadge>
        </td>

        <td class="table-td">
          {{ supplier.country || '—' }}
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

    <!-- Full-width responsive drawer with a sticky Save/Cancel footer -->
    <div v-if="selectedSupplier" class="fixed inset-0 bg-black/40 flex justify-end z-50" @keydown.esc="attemptClose">
      <div class="drawer-panel bg-white h-full flex flex-col">
        <div class="flex items-center justify-between px-6 py-5 border-b border-slate-200">
          <div>
            <h2 class="text-xl font-bold text-slate-800">
              {{ supplierForm.id ? $t('suppliers.drawer.detailTitle') : $t('suppliers.drawer.newTitle') }}
            </h2>
            <p class="text-slate-500 text-sm">{{ $t('suppliers.drawer.subtitle') }}</p>
          </div>
          <button @click="attemptClose" class="text-slate-500 hover:text-red-600 text-xl" :aria-label="$t('common.buttons.cancel')">✕</button>
        </div>

        <div class="flex-1 overflow-y-auto px-6 py-6 space-y-6">
          <div v-if="formError" class="bg-red-50 border border-red-200 text-red-600 rounded-xl p-3 text-sm">
            {{ formError }}
          </div>

          <FormSection :title="$t('suppliers.sections.basic')">
            <div class="field-grid">
              <div class="field">
                <label class="label">{{ $t('suppliers.fields.code') }}*</label>
                <input v-model="supplierForm.code" class="input" :class="{ 'input--error': fieldErrors.code }" />
                <p v-if="fieldErrors.code" class="field-error">{{ fieldErrors.code }}</p>
              </div>

              <div class="field">
                <label class="label">{{ $t('suppliers.fields.status') }}</label>
                <select v-model="supplierForm.status" class="input">
                  <option value="ACTIVE">{{ $t('common.status.active') }}</option>
                  <option value="INACTIVE">{{ $t('common.status.inactive') }}</option>
                </select>
              </div>

              <div class="field">
                <label class="label">{{ $t('suppliers.fields.legalNameEn') }}*</label>
                <input v-model="supplierForm.companyName" class="input" :class="{ 'input--error': fieldErrors.companyName }" />
                <p v-if="fieldErrors.companyName" class="field-error">{{ fieldErrors.companyName }}</p>
              </div>

              <div class="field">
                <label class="label">{{ $t('suppliers.fields.legalNameTh') }}</label>
                <input v-model="supplierForm.legalNameTh" class="input" />
              </div>

              <div class="field field--span2">
                <label class="label">{{ $t('suppliers.fields.supplierType') }}*</label>
                <div class="type-toggle">
                  <button
                    type="button"
                    class="type-toggle__option"
                    :class="{ 'type-toggle__option--active': supplierForm.supplierType === 'DOMESTIC' }"
                    @click="supplierForm.supplierType = 'DOMESTIC'"
                  >
                    {{ $t('suppliers.fields.typeDomestic') }}
                  </button>
                  <button
                    type="button"
                    class="type-toggle__option"
                    :class="{ 'type-toggle__option--active': supplierForm.supplierType === 'OVERSEAS' }"
                    @click="supplierForm.supplierType = 'OVERSEAS'"
                  >
                    {{ $t('suppliers.fields.typeOverseas') }}
                  </button>
                </div>
              </div>

              <div class="field">
                <label class="label">{{ $t('suppliers.fields.country') }}</label>
                <SearchableSelect
                  v-model="supplierForm.country"
                  v-model:display-value="supplierForm.country"
                  :options="countryOptions"
                  :placeholder="$t('suppliers.fields.countryPlaceholder')"
                  :clear-label="$t('common.buttons.clearFilter')"
                  :empty-text="$t('suppliers.fields.noMatches')"
                />
              </div>

              <div class="field">
                <label class="label">{{ $t('suppliers.fields.taxId') }}</label>
                <input v-model="supplierForm.taxId" class="input" />
              </div>
            </div>
          </FormSection>

          <FormSection :title="$t('suppliers.sections.contact')">
            <div class="field-grid">
              <div class="field">
                <label class="label">{{ $t('suppliers.fields.contactPerson') }}</label>
                <input v-model="supplierForm.contactPerson" class="input" />
              </div>

              <div class="field">
                <label class="label">{{ $t('suppliers.fields.contactPhone') }}</label>
                <input v-model="supplierForm.contactPhone" class="input" />
              </div>

              <div class="field">
                <label class="label">{{ $t('suppliers.fields.email') }}</label>
                <input v-model="supplierForm.email" type="email" class="input" :class="{ 'input--error': fieldErrors.email }" />
                <p v-if="fieldErrors.email" class="field-error">{{ fieldErrors.email }}</p>
              </div>

              <div class="field">
                <label class="label">{{ $t('suppliers.fields.phone') }}</label>
                <input v-model="supplierForm.phone" class="input" />
              </div>

              <div class="field field--span2">
                <label class="label">{{ $t('suppliers.fields.address') }}</label>
                <textarea v-model="supplierForm.address" rows="3" class="input"></textarea>
              </div>
            </div>
          </FormSection>

          <FormSection>
            <template #default>
              <button type="button" class="advanced-toggle" @click="tradeDetailsExpanded = !tradeDetailsExpanded">
                <span>{{ $t('suppliers.sections.trade') }}</span>
                <span class="advanced-toggle__chevron" :class="{ 'advanced-toggle__chevron--open': tradeDetailsExpanded }">▾</span>
              </button>
              <p class="advanced-hint">
                {{ supplierForm.supplierType === 'OVERSEAS' ? $t('suppliers.sections.tradeHintOverseas') : $t('suppliers.sections.tradeHintDomestic') }}
              </p>

              <div v-show="tradeDetailsExpanded" class="field-grid mt-4">
                <template v-if="supplierForm.supplierType === 'OVERSEAS'">
                  <div class="field">
                    <label class="label">{{ $t('suppliers.fields.currency') }}</label>
                    <SearchableSelect
                      v-model="supplierForm.currency"
                      v-model:display-value="supplierForm.currencyLabel"
                      :options="currencyOptions"
                      :placeholder="$t('suppliers.fields.currencyPlaceholder')"
                      :clear-label="$t('common.buttons.clearFilter')"
                      :empty-text="$t('suppliers.fields.noMatches')"
                    />
                  </div>

                  <div class="field">
                    <label class="label">{{ $t('suppliers.fields.defaultIncoterm') }}</label>
                    <SearchableSelect
                      v-model="supplierForm.defaultIncoterm"
                      v-model:display-value="supplierForm.defaultIncotermLabel"
                      :options="incotermOptions"
                      :placeholder="$t('suppliers.fields.defaultIncotermPlaceholder')"
                      :clear-label="$t('common.buttons.clearFilter')"
                      :empty-text="$t('suppliers.fields.noMatches')"
                    />
                  </div>

                  <div class="field">
                    <label class="label">{{ $t('suppliers.fields.incotermNamedPlace') }}</label>
                    <input v-model="supplierForm.incotermNamedPlace" class="input" :placeholder="$t('suppliers.fields.incotermNamedPlacePlaceholder')" />
                  </div>

                  <div class="field">
                    <label class="label">{{ $t('suppliers.fields.leadTimeDays') }}</label>
                    <input
                      v-model.number="supplierForm.leadTimeDays"
                      type="number"
                      min="0"
                      class="input input-number"
                      :class="{ 'input--error': fieldErrors.leadTimeDays }"
                    />
                    <p v-if="fieldErrors.leadTimeDays" class="field-error">{{ fieldErrors.leadTimeDays }}</p>
                  </div>

                  <div class="field">
                    <label class="label">{{ $t('suppliers.fields.paymentTerms') }}</label>
                    <input v-model="supplierForm.paymentTerms" class="input" :placeholder="$t('suppliers.fields.paymentTermsPlaceholder')" />
                  </div>

                  <div class="field">
                    <label class="label">{{ $t('suppliers.fields.shippingMethod') }}</label>
                    <select v-model="supplierForm.shippingMethod" class="input">
                      <option value="">—</option>
                      <option v-for="m in shippingMethodOptions" :key="m" :value="m">{{ m }}</option>
                    </select>
                  </div>
                </template>

                <template v-else>
                  <div class="field">
                    <label class="label">{{ $t('suppliers.fields.thaiTaxId') }}</label>
                    <input v-model="supplierForm.thaiTaxId" class="input" />
                  </div>

                  <div class="field">
                    <label class="label">{{ $t('suppliers.fields.vatRegistration') }}</label>
                    <input v-model="supplierForm.vatRegistration" class="input" />
                  </div>

                  <div class="field">
                    <label class="label">{{ $t('suppliers.fields.province') }}</label>
                    <input v-model="supplierForm.province" class="input" />
                  </div>

                  <div class="field">
                    <label class="label">{{ $t('suppliers.fields.postalCode') }}</label>
                    <input v-model="supplierForm.postalCode" class="input" />
                  </div>
                </template>
              </div>
            </template>
          </FormSection>
        </div>

        <div class="drawer-footer">
          <button @click="attemptClose" class="footer-btn footer-btn--secondary">
            {{ $t('common.buttons.cancel') }}
          </button>
          <button @click="saveSupplier" :disabled="saving" class="footer-btn footer-btn--primary">
            {{ saving ? $t('common.buttons.submitting') : $t('common.buttons.save') }}
          </button>
        </div>
      </div>
    </div>

    <ConfirmDialog
      :open="showUnsavedConfirm"
      :title="$t('suppliers.unsavedChanges.title')"
      :message="$t('suppliers.unsavedChanges.message')"
      :confirm-label="$t('suppliers.unsavedChanges.confirmButton')"
      :cancel-label="$t('suppliers.unsavedChanges.cancelButton')"
      danger
      @cancel="showUnsavedConfirm = false"
      @confirm="forceClose"
    />

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
import { COUNTRIES, CURRENCIES, INCOTERMS, SHIPPING_METHODS } from '../../constants/tradeReference'
import PageHeader from '../../components/ui/PageHeader.vue'
import FilterBar from '../../components/ui/FilterBar.vue'
import DataTable from '../../components/ui/DataTable.vue'
import StatusBadge from '../../components/ui/StatusBadge.vue'
import FormSection from '../../components/ui/FormSection.vue'
import ConfirmDialog from '../../components/ui/ConfirmDialog.vue'
import SearchableSelect from '../../components/ui/SearchableSelect.vue'
import SortableTh from '../../components/ui/SortableTh.vue'
import PaginationFooter from '../../components/ui/PaginationFooter.vue'
import listPagination from '../../mixins/listPagination'

let searchDebounce = null
const EMAIL_PATTERN = /^[^\s@]+@[^\s@]+\.[^\s@]+$/

function labelFor(options, value) {
  const match = options.find((o) => o.value === value)
  return match ? match.label : value || ''
}

export default {
  name: 'SupplierView',

  components: { PageHeader, FilterBar, DataTable, StatusBadge, FormSection, ConfirmDialog, SearchableSelect, SortableTh, PaginationFooter },

  mixins: [listPagination],

  data() {
    return {
      suppliers: [],
      loading: false,
      listError: '',
      saving: false,
      formError: '',
      fieldErrors: {},
      selectedSupplier: null,
      supplierForm: this.createEmptyForm(),
      formSnapshot: '',
      showUnsavedConfirm: false,
      tradeDetailsExpanded: false,
      supplierToDelete: null,
      deleting: false,
      deleteError: '',
      filters: {
        keyword: '',
        status: '',
        supplierType: ''
      },
      sortBy: 'code',
      sortDir: 'asc',
      countryOptions: COUNTRIES,
      currencyOptions: CURRENCIES,
      incotermOptions: INCOTERMS,
      shippingMethodOptions: SHIPPING_METHODS
    }
  },

  watch: {
    'filters.status'() {
      this.resetToFirstPage()
    },
    'filters.supplierType'() {
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

  beforeUnmount() {
    window.removeEventListener('beforeunload', this.handleBeforeUnload)
  },

  methods: {
    createEmptyForm() {
      return {
        id: null,
        code: '',
        companyName: '',
        legalNameTh: '',
        supplierType: 'DOMESTIC',
        country: '',
        taxId: '',
        contactPerson: '',
        contactPhone: '',
        address: '',
        email: '',
        phone: '',
        status: 'ACTIVE',
        currency: '',
        currencyLabel: '',
        defaultIncoterm: '',
        defaultIncotermLabel: '',
        incotermNamedPlace: '',
        leadTimeDays: null,
        paymentTerms: '',
        shippingMethod: '',
        thaiTaxId: '',
        vatRegistration: '',
        province: '',
        postalCode: ''
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
          supplierType: this.filters.supplierType,
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
      this.fieldErrors = {}
      this.tradeDetailsExpanded = false
      this.selectedSupplier = supplier || {}

      this.supplierForm = supplier
        ? {
            id: supplier.id,
            code: supplier.code,
            companyName: supplier.company_name,
            legalNameTh: supplier.legal_name_th || '',
            supplierType: supplier.supplier_type || 'DOMESTIC',
            country: supplier.country || '',
            taxId: supplier.tax_id || '',
            contactPerson: supplier.contact_person || '',
            contactPhone: supplier.contact_phone || '',
            address: supplier.address || '',
            email: supplier.email || '',
            phone: supplier.phone || '',
            status: supplier.status,
            currency: supplier.currency || '',
            currencyLabel: labelFor(this.currencyOptions, supplier.currency),
            defaultIncoterm: supplier.default_incoterm || '',
            defaultIncotermLabel: labelFor(this.incotermOptions, supplier.default_incoterm),
            incotermNamedPlace: supplier.incoterm_named_place || '',
            leadTimeDays: supplier.lead_time_days ?? null,
            paymentTerms: supplier.payment_terms || '',
            shippingMethod: supplier.shipping_method || '',
            thaiTaxId: supplier.thai_tax_id || '',
            vatRegistration: supplier.vat_registration || '',
            province: supplier.province || '',
            postalCode: supplier.postal_code || ''
          }
        : this.createEmptyForm()

      this.formSnapshot = JSON.stringify(this.supplierForm)
      window.addEventListener('beforeunload', this.handleBeforeUnload)
    },

    clearFilter() {
      this.filters.keyword = ''
      this.filters.status = ''
      this.filters.supplierType = ''
    },

    isDirty() {
      return JSON.stringify(this.supplierForm) !== this.formSnapshot
    },

    attemptClose() {
      if (this.isDirty()) {
        this.showUnsavedConfirm = true
        return
      }
      this.forceClose()
    },

    forceClose() {
      this.showUnsavedConfirm = false
      this.selectedSupplier = null
      window.removeEventListener('beforeunload', this.handleBeforeUnload)
    },

    handleBeforeUnload(event) {
      if (!this.isDirty()) return
      event.preventDefault()
      event.returnValue = ''
    },

    validateForm() {
      const errors = {}
      if (!this.supplierForm.code) errors.code = this.$t('common.messages.requiredField')
      if (!this.supplierForm.companyName) errors.companyName = this.$t('common.messages.requiredField')
      if (this.supplierForm.email && !EMAIL_PATTERN.test(this.supplierForm.email)) {
        errors.email = this.$t('suppliers.messages.emailInvalid')
      }
      if (this.supplierForm.leadTimeDays !== null && this.supplierForm.leadTimeDays !== '' && this.supplierForm.leadTimeDays < 0) {
        errors.leadTimeDays = this.$t('suppliers.messages.leadTimeInvalid')
      }
      this.fieldErrors = errors
      return Object.keys(errors).length === 0
    },

    async saveSupplier() {
      this.formError = ''

      if (!this.validateForm()) {
        this.formError = this.$t('common.messages.requiredField')
        return
      }

      this.saving = true
      const payload = {
        code: this.supplierForm.code,
        company_name: this.supplierForm.companyName,
        legal_name_th: this.supplierForm.legalNameTh || null,
        supplier_type: this.supplierForm.supplierType,
        country: this.supplierForm.country || null,
        tax_id: this.supplierForm.taxId || null,
        contact_person: this.supplierForm.contactPerson || null,
        contact_phone: this.supplierForm.contactPhone || null,
        address: this.supplierForm.address || null,
        email: this.supplierForm.email || null,
        phone: this.supplierForm.phone || null,
        status: this.supplierForm.status,
        currency: this.supplierForm.currency || null,
        default_incoterm: this.supplierForm.defaultIncoterm || null,
        incoterm_named_place: this.supplierForm.incotermNamedPlace || null,
        lead_time_days: this.supplierForm.leadTimeDays === '' ? null : this.supplierForm.leadTimeDays,
        payment_terms: this.supplierForm.paymentTerms || null,
        shipping_method: this.supplierForm.shippingMethod || null,
        thai_tax_id: this.supplierForm.thaiTaxId || null,
        vat_registration: this.supplierForm.vatRegistration || null,
        province: this.supplierForm.province || null,
        postal_code: this.supplierForm.postalCode || null
      }

      try {
        if (this.supplierForm.id) {
          await updateSupplier(this.supplierForm.id, payload)
        } else {
          await createSupplier(payload)
        }

        this.forceClose()
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
    },

    typeTone(type) {
      return type === 'OVERSEAS' ? 'blue' : 'gray'
    },

    typeLabel(type) {
      if (type === 'OVERSEAS') return this.$t('suppliers.fields.typeOverseas')
      if (type === 'DOMESTIC') return this.$t('suppliers.fields.typeDomestic')
      return type || '—'
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

.input--error {
  border-color: #ef4444;
}

.label {
  display: block;
  margin-bottom: 8px;
  color: #334155;
  font-weight: var(--fw-label);
  font-size: var(--fs-label);
}

.field-error {
  color: #ef4444;
  font-size: 0.78rem;
  margin-top: -6px;
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

/* Balanced 2-column field layout — mirrors ProductMasterView's own
   field-grid/field--span2 so every Enterprise Add/Edit drawer in this
   app lays fields out identically. */
.field-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 16px;
}

.field--span2 {
  grid-column: 1 / -1;
}

@media (max-width: 640px) {
  .field-grid {
    grid-template-columns: 1fr;
  }
  .field--span2 {
    grid-column: auto;
  }
}

.type-toggle {
  display: flex;
  gap: 8px;
}

.type-toggle__option {
  flex: 1;
  padding: 10px 14px;
  border-radius: 12px;
  border: 1px solid #cbd5e1;
  background: #fff;
  color: #475569;
  font-weight: 600;
  cursor: pointer;
}

.type-toggle__option--active {
  background: #fff7ed;
  border-color: #ea580c;
  color: #ea580c;
}

.advanced-toggle {
  display: flex;
  align-items: center;
  gap: 8px;
  background: none;
  border: none;
  cursor: pointer;
  font-weight: 700;
  font-size: 1rem;
  color: #0f172a;
  padding: 0;
}

.advanced-toggle__chevron {
  transition: transform 0.15s ease;
  color: #ea580c;
}

.advanced-toggle__chevron--open {
  transform: rotate(180deg);
}

.advanced-hint {
  color: #94a3b8;
  font-size: 0.8rem;
  margin-top: 4px;
}

/* Wide, responsive drawer with a sticky footer — the middle content
   area scrolls, the header/footer never do. */
.drawer-panel {
  width: 100%;
  max-width: 1000px;
}

@media (max-width: 768px) {
  .drawer-panel {
    max-width: 100%;
  }
}

.drawer-footer {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
  padding: 16px 24px;
  border-top: 1px solid #e2e8f0;
  background: #fff;
  flex-shrink: 0;
}

.footer-btn {
  padding: 12px 24px;
  border-radius: 12px;
  font-weight: 600;
  cursor: pointer;
}

.footer-btn--secondary {
  background: #fff;
  color: #334155;
  border: 1px solid #cbd5e1;
}

.footer-btn--secondary:hover {
  background: #f1f5f9;
}

.footer-btn--primary {
  background: #ea580c;
  color: #fff;
  border: none;
}

.footer-btn--primary:hover:not(:disabled) {
  background: #c2410c;
}

.footer-btn--primary:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}
</style>
