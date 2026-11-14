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
          <SortableTh column="code" :sort-by="sortBy" :sort-dir="sortDir" @sort="toggleSort">
            {{ $t('customers.table.code') }}
          </SortableTh>
          <SortableTh column="companyName" :sort-by="sortBy" :sort-dir="sortDir" @sort="toggleSort">
            {{ $t('customers.table.companyName') }}
          </SortableTh>
          <th class="table-th">{{ $t('customers.table.taxId') }}</th>
          <th class="table-th">{{ $t('customers.table.contactPerson') }}</th>
          <SortableTh column="status" :sort-by="sortBy" :sort-dir="sortDir" @sort="toggleSort">
            {{ $t('customers.table.status') }}
          </SortableTh>
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

      <template v-if="!loading && customers.length > 0" #footer>
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

    <!-- Customer drawer: Overview + Delivery Locations tabs -->
    <div v-if="selectedCustomer" class="fixed inset-0 bg-black/40 flex justify-end z-50" @keydown.esc="attemptClose">
      <div class="drawer-panel bg-white h-full flex flex-col">
        <div class="flex items-center justify-between px-6 py-5 border-b border-slate-200">
          <div>
            <h2 class="text-xl font-bold text-slate-800">
              {{ customerForm.id ? $t('customers.drawer.detailTitle') : $t('customers.drawer.newTitle') }}
            </h2>
            <p class="text-slate-500 text-sm">{{ $t('customers.drawer.subtitle') }}</p>
          </div>
          <button @click="attemptClose" class="text-slate-500 hover:text-red-600 text-xl" :aria-label="$t('common.buttons.cancel')">✕</button>
        </div>

        <div class="flex gap-2 px-6 pt-4 overflow-x-auto">
          <button
            v-for="tab in tabs"
            :key="tab.key"
            @click="activeTab = tab.key"
            class="tab-button"
            :class="activeTab === tab.key ? 'tab-active' : ''"
          >
            {{ $t(tab.labelKey) }}
          </button>
        </div>

        <div class="flex-1 overflow-y-auto px-6 py-6 space-y-6">
          <!-- Overview tab -->
          <template v-if="activeTab === 'OVERVIEW'">
            <div v-if="formError" class="bg-red-50 border border-red-200 text-red-600 rounded-xl p-3 text-sm">
              {{ formError }}
            </div>

            <FormSection :title="$t('customers.sections.basic')">
              <div class="field-grid">
                <div class="field">
                  <label class="label">{{ $t('customers.fields.code') }}*</label>
                  <input v-model="customerForm.code" class="input" :class="{ 'input--error': fieldErrors.code }" />
                  <p v-if="fieldErrors.code" class="field-error">{{ fieldErrors.code }}</p>
                </div>

                <div class="field">
                  <label class="label">{{ $t('customers.fields.status') }}</label>
                  <select v-model="customerForm.status" class="input">
                    <option value="ACTIVE">{{ $t('common.status.active') }}</option>
                    <option value="INACTIVE">{{ $t('common.status.inactive') }}</option>
                  </select>
                </div>

                <div class="field">
                  <label class="label">{{ $t('customers.fields.legalNameEn') }}*</label>
                  <input v-model="customerForm.companyName" class="input" :class="{ 'input--error': fieldErrors.companyName }" />
                  <p v-if="fieldErrors.companyName" class="field-error">{{ fieldErrors.companyName }}</p>
                </div>

                <div class="field">
                  <label class="label">{{ $t('customers.fields.legalNameTh') }}</label>
                  <input v-model="customerForm.legalNameTh" class="input" />
                </div>
              </div>
            </FormSection>

            <FormSection :title="$t('customers.sections.contact')">
              <div class="field-grid">
                <div class="field">
                  <label class="label">{{ $t('customers.fields.contactPerson') }}</label>
                  <input v-model="customerForm.contactPerson" class="input" />
                </div>

                <div class="field">
                  <label class="label">{{ $t('customers.fields.contactPhone') }}</label>
                  <input v-model="customerForm.contactPhone" class="input" />
                </div>

                <div class="field">
                  <label class="label">{{ $t('customers.fields.email') }}</label>
                  <input v-model="customerForm.email" type="email" class="input" :class="{ 'input--error': fieldErrors.email }" />
                  <p v-if="fieldErrors.email" class="field-error">{{ fieldErrors.email }}</p>
                </div>

                <div class="field">
                  <label class="label">{{ $t('customers.fields.phone') }}</label>
                  <input v-model="customerForm.phone" class="input" />
                </div>
              </div>
            </FormSection>

            <FormSection :title="$t('customers.sections.billing')">
              <div class="field">
                <label class="label">{{ $t('customers.fields.billingAddress') }}</label>
                <textarea v-model="customerForm.address" rows="3" class="input"></textarea>
              </div>
            </FormSection>

            <FormSection>
              <template #default>
                <button type="button" class="advanced-toggle" @click="moreDetailsExpanded = !moreDetailsExpanded">
                  <span>{{ $t('customers.sections.more') }}</span>
                  <span class="advanced-toggle__chevron" :class="{ 'advanced-toggle__chevron--open': moreDetailsExpanded }">▾</span>
                </button>
                <p class="advanced-hint">{{ $t('customers.sections.moreHint') }}</p>

                <div v-show="moreDetailsExpanded" class="field-grid mt-4">
                  <div class="field">
                    <label class="label">{{ $t('customers.fields.customerType') }}</label>
                    <input v-model="customerForm.customerType" class="input" :placeholder="$t('customers.fields.customerTypePlaceholder')" />
                  </div>

                  <div class="field">
                    <label class="label">{{ $t('customers.fields.customerGroup') }}</label>
                    <input v-model="customerForm.customerGroup" class="input" :placeholder="$t('customers.fields.customerGroupPlaceholder')" />
                  </div>

                  <div class="field">
                    <label class="label">{{ $t('customers.fields.taxId') }}</label>
                    <input v-model="customerForm.taxId" class="input" />
                  </div>

                  <div class="field">
                    <label class="label">{{ $t('customers.fields.branchNo') }}</label>
                    <input v-model="customerForm.branchNo" class="input" :placeholder="$t('customers.fields.branchNoPlaceholder')" />
                  </div>
                </div>
              </template>
            </FormSection>
          </template>

          <!-- Delivery Locations tab -->
          <template v-else-if="activeTab === 'LOCATIONS'">
            <div v-if="!customerForm.id" class="text-slate-500">
              {{ $t('customers.locationsTab.saveFirst') }}
            </div>

            <template v-else>
              <div v-if="locationFormOpen" class="border border-slate-200 rounded-2xl p-5 space-y-4">
                <h3 class="font-bold text-slate-800">
                  {{ editingLocationId ? $t('customers.locationsTab.editTitle') : $t('customers.locationsTab.addTitle') }}
                </h3>

                <div v-if="locationFormError" class="bg-red-50 border border-red-200 text-red-600 rounded-xl p-3 text-sm">
                  {{ locationFormError }}
                </div>

                <div class="field-grid">
                  <div class="field">
                    <label class="label">{{ $t('customers.locationsTab.fields.code') }}*</label>
                    <input v-model="locationForm.code" class="input" />
                  </div>

                  <div class="field">
                    <label class="label">{{ $t('customers.locationsTab.fields.name') }}*</label>
                    <input v-model="locationForm.name" class="input" />
                  </div>

                  <div class="field">
                    <label class="label">{{ $t('customers.locationsTab.fields.type') }}</label>
                    <select v-model="locationForm.type" class="input">
                      <option value="">—</option>
                      <option value="BRANCH">{{ $t('customers.locationsTab.types.branch') }}</option>
                      <option value="DISTRIBUTION_CENTER">{{ $t('customers.locationsTab.types.distributionCenter') }}</option>
                      <option value="DELIVERY_ADDRESS">{{ $t('customers.locationsTab.types.deliveryAddress') }}</option>
                    </select>
                  </div>

                  <div class="field">
                    <label class="label">{{ $t('customers.locationsTab.fields.status') }}</label>
                    <select v-model="locationForm.status" class="input">
                      <option value="ACTIVE">{{ $t('common.status.active') }}</option>
                      <option value="INACTIVE">{{ $t('common.status.inactive') }}</option>
                    </select>
                  </div>

                  <div class="field field--span2">
                    <label class="label">{{ $t('customers.locationsTab.fields.address') }}</label>
                    <textarea v-model="locationForm.address" rows="2" class="input"></textarea>
                  </div>

                  <div class="field">
                    <label class="label">{{ $t('customers.locationsTab.fields.province') }}</label>
                    <input v-model="locationForm.province" class="input" />
                  </div>

                  <div class="field">
                    <label class="label">{{ $t('customers.locationsTab.fields.district') }}</label>
                    <input v-model="locationForm.district" class="input" />
                  </div>

                  <div class="field">
                    <label class="label">{{ $t('customers.locationsTab.fields.postalCode') }}</label>
                    <input v-model="locationForm.postalCode" class="input" />
                  </div>

                  <div class="field">
                    <label class="label">{{ $t('customers.locationsTab.fields.contactPerson') }}</label>
                    <input v-model="locationForm.contactPerson" class="input" />
                  </div>

                  <div class="field">
                    <label class="label">{{ $t('customers.locationsTab.fields.contactPhone') }}</label>
                    <input v-model="locationForm.contactPhone" class="input" />
                  </div>

                  <div class="field">
                    <label class="label">{{ $t('customers.locationsTab.fields.receivingWindow') }}</label>
                    <input v-model="locationForm.receivingWindow" class="input" :placeholder="$t('customers.locationsTab.fields.receivingWindowPlaceholder')" />
                  </div>

                  <div class="field field--span2">
                    <label class="label">{{ $t('customers.locationsTab.fields.instructions') }}</label>
                    <textarea v-model="locationForm.instructions" rows="2" class="input"></textarea>
                  </div>

                  <div class="field field--span2">
                    <label class="checkbox-label">
                      <input type="checkbox" v-model="locationForm.isDefault" />
                      {{ $t('customers.locationsTab.fields.isDefault') }}
                    </label>
                  </div>
                </div>

                <div class="flex justify-end gap-3">
                  <button @click="closeLocationForm" class="footer-btn footer-btn--secondary">
                    {{ $t('common.buttons.cancel') }}
                  </button>
                  <button @click="saveLocation" :disabled="savingLocation" class="footer-btn footer-btn--primary">
                    {{ savingLocation ? $t('common.buttons.submitting') : $t('common.buttons.save') }}
                  </button>
                </div>
              </div>

              <template v-else>
                <div class="flex flex-col md:flex-row justify-between gap-3">
                  <input
                    v-model="locationFilters.keyword"
                    type="text"
                    class="input md:max-w-xs"
                    :placeholder="$t('customers.locationsTab.searchPlaceholder')"
                  />
                  <select v-model="locationFilters.status" class="input md:max-w-xs">
                    <option value="">{{ $t('customers.filters.allStatus') }}</option>
                    <option value="ACTIVE">{{ $t('common.status.active') }}</option>
                    <option value="INACTIVE">{{ $t('common.status.inactive') }}</option>
                  </select>
                  <button @click="openLocationForm(null)" class="bg-orange-600 hover:bg-orange-700 text-white px-4 py-2 rounded-xl font-semibold whitespace-nowrap">
                    {{ $t('customers.locationsTab.addButton') }}
                  </button>
                </div>

                <div v-if="locationListError" class="bg-red-50 border border-red-200 text-red-600 rounded-xl p-3 text-sm">
                  {{ locationListError }}
                </div>

                <DataTable
                  :loading="locationsLoading"
                  :is-empty="locations.length === 0"
                  :loading-text="$t('customers.locationsTab.loading')"
                  :empty-text="$t('customers.locationsTab.empty')"
                  :colspan="7"
                  min-width="900px"
                >
                  <template #head>
                    <tr>
                      <th class="table-th">{{ $t('customers.locationsTab.fields.code') }}</th>
                      <th class="table-th">{{ $t('customers.locationsTab.fields.name') }}</th>
                      <th class="table-th">{{ $t('customers.locationsTab.fields.type') }}</th>
                      <th class="table-th">{{ $t('customers.locationsTab.fields.province') }}</th>
                      <th class="table-th">{{ $t('customers.locationsTab.fields.contactPerson') }}</th>
                      <th class="table-th">{{ $t('customers.locationsTab.fields.status') }}</th>
                      <th class="table-th text-right">{{ $t('customers.table.action') }}</th>
                    </tr>
                  </template>

                  <tr v-for="loc in locations" :key="loc.id" class="border-t hover:bg-orange-50">
                    <td class="table-td font-semibold text-slate-800">
                      {{ loc.code }}
                      <StatusBadge v-if="loc.is_default" tone="orange">{{ $t('customers.locationsTab.defaultBadge') }}</StatusBadge>
                    </td>
                    <td class="table-td">{{ loc.name }}</td>
                    <td class="table-td">{{ typeLabel(loc.type) }}</td>
                    <td class="table-td">{{ loc.province || '—' }}</td>
                    <td class="table-td">{{ loc.contact_person || '—' }}</td>
                    <td class="table-td">
                      <StatusBadge :tone="statusTone(loc.status)">{{ statusLabel(loc.status) }}</StatusBadge>
                    </td>
                    <td class="table-td text-right space-x-3">
                      <button @click="openLocationForm(loc)" class="text-orange-600 hover:text-orange-700 font-semibold">
                        {{ $t('common.buttons.edit') }}
                      </button>
                      <button
                        v-if="loc.status === 'ACTIVE'"
                        @click="askDeactivateLocation(loc)"
                        class="text-red-600 hover:text-red-700 font-semibold"
                      >
                        {{ $t('customers.locationsTab.deactivateButton') }}
                      </button>
                    </td>
                  </tr>
                </DataTable>
              </template>
            </template>
          </template>
        </div>

        <div v-if="activeTab === 'OVERVIEW'" class="drawer-footer">
          <button @click="attemptClose" class="footer-btn footer-btn--secondary">
            {{ $t('common.buttons.cancel') }}
          </button>
          <button @click="saveCustomer" :disabled="saving" class="footer-btn footer-btn--primary">
            {{ saving ? $t('common.buttons.submitting') : $t('common.buttons.save') }}
          </button>
        </div>
      </div>
    </div>

    <ConfirmDialog
      :open="showUnsavedConfirm"
      :title="$t('customers.unsavedChanges.title')"
      :message="$t('customers.unsavedChanges.message')"
      :confirm-label="$t('customers.unsavedChanges.confirmButton')"
      :cancel-label="$t('customers.unsavedChanges.cancelButton')"
      danger
      @cancel="showUnsavedConfirm = false"
      @confirm="forceClose"
    />

    <ConfirmDialog
      :open="!!locationToDeactivate"
      :title="$t('customers.locationsTab.deactivateTitle')"
      :message="$t('customers.locationsTab.deactivateConfirm')"
      :confirm-label="$t('customers.locationsTab.deactivateButton')"
      :cancel-label="$t('common.buttons.cancel')"
      :loading-label="$t('common.buttons.submitting')"
      danger
      :loading="deactivatingLocation"
      :error-message="deactivateLocationError"
      @cancel="locationToDeactivate = null; deactivateLocationError = ''"
      @confirm="confirmDeactivateLocation"
    />

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
import {
  listCustomerDeliveryLocations,
  createCustomerDeliveryLocation,
  updateCustomerDeliveryLocation
} from '../../services/customerDeliveryLocations'
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
let locationSearchDebounce = null
const EMAIL_PATTERN = /^[^\s@]+@[^\s@]+\.[^\s@]+$/

export default {
  name: 'CustomerView',

  components: { PageHeader, FilterBar, DataTable, StatusBadge, FormSection, ConfirmDialog, SortableTh, PaginationFooter },

  mixins: [listPagination],

  data() {
    return {
      customers: [],
      loading: false,
      listError: '',
      saving: false,
      formError: '',
      fieldErrors: {},
      selectedCustomer: null,
      customerForm: this.createEmptyForm(),
      formSnapshot: '',
      showUnsavedConfirm: false,
      moreDetailsExpanded: false,
      activeTab: 'OVERVIEW',
      tabs: [
        { key: 'OVERVIEW', labelKey: 'customers.tabs.overview' },
        { key: 'LOCATIONS', labelKey: 'customers.tabs.locations' }
      ],
      customerToDelete: null,
      deleting: false,
      deleteError: '',
      filters: {
        keyword: '',
        status: ''
      },
      sortBy: 'code',
      sortDir: 'asc',

      locations: [],
      locationsLoading: false,
      locationListError: '',
      locationFilters: { keyword: '', status: '' },
      locationFormOpen: false,
      editingLocationId: null,
      locationForm: this.createEmptyLocationForm(),
      locationFormError: '',
      savingLocation: false,
      locationToDeactivate: null,
      deactivatingLocation: false,
      deactivateLocationError: ''
    }
  },

  watch: {
    'filters.status'() {
      this.resetToFirstPage()
    },
    'filters.keyword'() {
      clearTimeout(searchDebounce)
      searchDebounce = setTimeout(() => this.resetToFirstPage(), 300)
    },
    activeTab(tab) {
      if (tab === 'LOCATIONS' && this.customerForm.id) this.fetchLocations()
    },
    'locationFilters.status'() {
      this.fetchLocations()
    },
    'locationFilters.keyword'() {
      clearTimeout(locationSearchDebounce)
      locationSearchDebounce = setTimeout(() => this.fetchLocations(), 300)
    }
  },

  mounted() {
    this.fetchCustomers()
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
        customerType: '',
        customerGroup: '',
        taxId: '',
        branchNo: '',
        contactPerson: '',
        contactPhone: '',
        address: '',
        email: '',
        phone: '',
        status: 'ACTIVE'
      }
    },

    createEmptyLocationForm() {
      return {
        code: '',
        name: '',
        type: '',
        address: '',
        province: '',
        district: '',
        postalCode: '',
        contactPerson: '',
        contactPhone: '',
        receivingWindow: '',
        instructions: '',
        isDefault: false,
        status: 'ACTIVE'
      }
    },

    // fetchList() is the listPagination mixin's required hook —
    // goToPage/resetToFirstPage/the limit watcher all call it. Kept as
    // a thin alias over fetchCustomers() (the pre-existing public name
    // other methods here already call directly) rather than renaming
    // every call site.
    fetchList() {
      return this.fetchCustomers()
    },

    async fetchCustomers() {
      this.loading = true
      this.listError = ''
      try {
        const res = await listCustomers({
          search: this.filters.keyword,
          status: this.filters.status,
          page: this.page,
          limit: this.limit,
          sortBy: this.sortBy,
          sortDir: this.sortDir
        })
        this.customers = res.data
        this.total = res.meta.total
        this.totalPages = res.meta.total_pages
      } catch (err) {
        this.listError = err.message || this.$t('customers.messages.loadFailed')
      } finally {
        this.loading = false
      }
    },

    openCustomer(customer) {
      this.formError = ''
      this.fieldErrors = {}
      this.moreDetailsExpanded = false
      this.activeTab = 'OVERVIEW'
      this.locations = []
      this.locationFormOpen = false
      this.selectedCustomer = customer || {}

      this.customerForm = customer
        ? {
            id: customer.id,
            code: customer.code,
            companyName: customer.company_name,
            legalNameTh: customer.legal_name_th || '',
            customerType: customer.customer_type || '',
            customerGroup: customer.customer_group || '',
            taxId: customer.tax_id || '',
            branchNo: customer.branch_no || '',
            contactPerson: customer.contact_person || '',
            contactPhone: customer.contact_phone || '',
            address: customer.address || '',
            email: customer.email || '',
            phone: customer.phone || '',
            status: customer.status
          }
        : this.createEmptyForm()

      this.formSnapshot = JSON.stringify(this.customerForm)
      window.addEventListener('beforeunload', this.handleBeforeUnload)
    },

    clearFilter() {
      this.filters.keyword = ''
      this.filters.status = ''
    },

    isDirty() {
      return JSON.stringify(this.customerForm) !== this.formSnapshot
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
      this.selectedCustomer = null
      window.removeEventListener('beforeunload', this.handleBeforeUnload)
    },

    handleBeforeUnload(event) {
      if (!this.isDirty()) return
      event.preventDefault()
      event.returnValue = ''
    },

    validateForm() {
      const errors = {}
      if (!this.customerForm.code) errors.code = this.$t('common.messages.requiredField')
      if (!this.customerForm.companyName) errors.companyName = this.$t('common.messages.requiredField')
      if (this.customerForm.email && !EMAIL_PATTERN.test(this.customerForm.email)) {
        errors.email = this.$t('customers.messages.emailInvalid')
      }
      this.fieldErrors = errors
      return Object.keys(errors).length === 0
    },

    async saveCustomer() {
      this.formError = ''

      if (!this.validateForm()) {
        this.formError = this.$t('common.messages.requiredField')
        return
      }

      this.saving = true
      const payload = {
        code: this.customerForm.code,
        company_name: this.customerForm.companyName,
        legal_name_th: this.customerForm.legalNameTh || null,
        customer_type: this.customerForm.customerType || null,
        customer_group: this.customerForm.customerGroup || null,
        tax_id: this.customerForm.taxId || null,
        branch_no: this.customerForm.branchNo || null,
        contact_person: this.customerForm.contactPerson || null,
        contact_phone: this.customerForm.contactPhone || null,
        address: this.customerForm.address || null,
        email: this.customerForm.email || null,
        phone: this.customerForm.phone || null,
        status: this.customerForm.status
      }

      try {
        let saved
        if (this.customerForm.id) {
          saved = await updateCustomer(this.customerForm.id, payload)
        } else {
          saved = await createCustomer(payload)
        }

        this.customerForm.id = saved.data.id
        this.formSnapshot = JSON.stringify(this.customerForm)
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
    },

    typeLabel(type) {
      if (type === 'BRANCH') return this.$t('customers.locationsTab.types.branch')
      if (type === 'DISTRIBUTION_CENTER') return this.$t('customers.locationsTab.types.distributionCenter')
      if (type === 'DELIVERY_ADDRESS') return this.$t('customers.locationsTab.types.deliveryAddress')
      return '—'
    },

    async fetchLocations() {
      if (!this.customerForm.id) return
      this.locationsLoading = true
      this.locationListError = ''
      try {
        const res = await listCustomerDeliveryLocations(this.customerForm.id, {
          search: this.locationFilters.keyword,
          status: this.locationFilters.status
        })
        this.locations = res.data
      } catch (err) {
        this.locationListError = err.message || this.$t('customers.locationsTab.loadFailed')
      } finally {
        this.locationsLoading = false
      }
    },

    openLocationForm(loc) {
      this.locationFormError = ''
      this.editingLocationId = loc ? loc.id : null
      this.locationForm = loc
        ? {
            code: loc.code,
            name: loc.name,
            type: loc.type || '',
            address: loc.address || '',
            province: loc.province || '',
            district: loc.district || '',
            postalCode: loc.postal_code || '',
            contactPerson: loc.contact_person || '',
            contactPhone: loc.contact_phone || '',
            receivingWindow: loc.receiving_window || '',
            instructions: loc.instructions || '',
            isDefault: loc.is_default,
            status: loc.status
          }
        : this.createEmptyLocationForm()
      this.locationFormOpen = true
    },

    closeLocationForm() {
      this.locationFormOpen = false
      this.editingLocationId = null
    },

    async saveLocation() {
      this.locationFormError = ''
      if (!this.locationForm.code || !this.locationForm.name) {
        this.locationFormError = this.$t('common.messages.requiredField')
        return
      }

      this.savingLocation = true
      const payload = {
        code: this.locationForm.code,
        name: this.locationForm.name,
        type: this.locationForm.type || null,
        address: this.locationForm.address || null,
        province: this.locationForm.province || null,
        district: this.locationForm.district || null,
        postal_code: this.locationForm.postalCode || null,
        contact_person: this.locationForm.contactPerson || null,
        contact_phone: this.locationForm.contactPhone || null,
        receiving_window: this.locationForm.receivingWindow || null,
        instructions: this.locationForm.instructions || null,
        is_default: this.locationForm.isDefault,
        status: this.locationForm.status
      }

      try {
        if (this.editingLocationId) {
          await updateCustomerDeliveryLocation(this.editingLocationId, payload)
        } else {
          await createCustomerDeliveryLocation({ customer_id: this.customerForm.id, ...payload })
        }
        this.closeLocationForm()
        showToast(this.$t('common.messages.saveSuccess'), 'success')
        await this.fetchLocations()
      } catch (err) {
        this.locationFormError = err.message || this.$t('common.messages.saveFailed')
      } finally {
        this.savingLocation = false
      }
    },

    askDeactivateLocation(loc) {
      this.deactivateLocationError = ''
      this.locationToDeactivate = loc
    },

    async confirmDeactivateLocation() {
      if (!this.locationToDeactivate) return
      this.deactivatingLocation = true
      this.deactivateLocationError = ''

      const loc = this.locationToDeactivate
      try {
        await updateCustomerDeliveryLocation(loc.id, {
          code: loc.code,
          name: loc.name,
          type: loc.type || null,
          address: loc.address || null,
          province: loc.province || null,
          district: loc.district || null,
          postal_code: loc.postal_code || null,
          contact_person: loc.contact_person || null,
          contact_phone: loc.contact_phone || null,
          receiving_window: loc.receiving_window || null,
          instructions: loc.instructions || null,
          is_default: loc.is_default,
          status: 'INACTIVE'
        })
        this.locationToDeactivate = null
        showToast(this.$t('common.messages.saveSuccess'), 'success')
        await this.fetchLocations()
      } catch (err) {
        this.deactivateLocationError = err.message || this.$t('common.messages.saveFailed')
      } finally {
        this.deactivatingLocation = false
      }
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

.checkbox-label {
  display: flex;
  align-items: center;
  gap: 8px;
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

.tab-button {
  padding: 10px 16px;
  border-radius: 12px;
  background: #f1f5f9;
  color: #475569;
  font-weight: 600;
  white-space: nowrap;
}

.tab-active {
  background: #ea580c;
  color: white;
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
