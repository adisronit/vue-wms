<template>
  <div class="space-y-6">
    <PageHeader :title="$t('platformAdmin.tenants.title')" :subtitle="$t('platformAdmin.tenants.subtitle')">
      <template #actions>
        <button
          @click="openCreate"
          class="bg-orange-600 hover:bg-orange-700 text-white px-5 py-3 rounded-xl font-semibold"
        >
          {{ $t('platformAdmin.tenants.addButton') }}
        </button>
      </template>
    </PageHeader>

    <FilterBar :cols="3">
      <input
        v-model="filters.search"
        type="text"
        :placeholder="$t('platformAdmin.tenants.filters.searchPlaceholder')"
        class="input"
      />

      <select v-model="filters.status" class="input">
        <option value="">{{ $t('platformAdmin.tenants.filters.allStatus') }}</option>
        <option v-for="code in TENANT_STATUSES" :key="code" :value="code">
          {{ $t('platformAdmin.status.' + code.toLowerCase()) }}
        </option>
      </select>

      <button @click="clearFilter" class="bg-slate-100 hover:bg-slate-200 text-slate-700 rounded-xl font-semibold">
        {{ $t('common.buttons.clearFilter') }}
      </button>
    </FilterBar>

    <div v-if="listError" class="bg-red-50 border border-red-200 text-red-600 rounded-xl p-3 text-sm">
      {{ listError }}
    </div>

    <DataTable
      :loading="loading"
      :is-empty="tenants.length === 0"
      :loading-text="$t('platformAdmin.tenants.table.loading')"
      :empty-text="$t('platformAdmin.tenants.table.noData')"
      :colspan="5"
      min-width="800px"
    >
      <template #head>
        <tr>
          <th class="table-th">{{ $t('platformAdmin.tenants.table.name') }}</th>
          <th class="table-th">{{ $t('platformAdmin.tenants.table.slug') }}</th>
          <th class="table-th">{{ $t('platformAdmin.tenants.table.status') }}</th>
          <th class="table-th">{{ $t('platformAdmin.tenants.table.createdAt') }}</th>
          <th class="table-th text-right">{{ $t('platformAdmin.tenants.table.action') }}</th>
        </tr>
      </template>

      <tr v-for="t in tenants" :key="t.id" class="border-t hover:bg-orange-50">
        <td class="table-td font-semibold text-slate-800">{{ t.name }}</td>
        <td class="table-td">{{ t.slug }}</td>
        <td class="table-td">
          <StatusBadge :tone="statusTone(t.status)">{{ $t('platformAdmin.status.' + t.status.toLowerCase()) }}</StatusBadge>
        </td>
        <td class="table-td">{{ $d(new Date(t.created_at), 'short') }}</td>
        <td class="table-td text-right space-x-3 whitespace-nowrap">
          <router-link :to="`/platform/tenants/${t.id}`" class="text-orange-600 hover:text-orange-700 font-semibold">
            {{ $t('platformAdmin.tenants.table.view') }}
          </router-link>
          <button @click="openEdit(t)" class="text-slate-600 hover:text-slate-800 font-semibold">
            {{ $t('platformAdmin.tenants.table.edit') }}
          </button>
          <button @click="openStatus(t)" class="text-slate-600 hover:text-slate-800 font-semibold">
            {{ $t('platformAdmin.tenants.table.changeStatus') }}
          </button>
        </td>
      </tr>

      <template v-if="!loading && tenants.length > 0" #footer>
        <div class="flex items-center justify-between px-5 py-4 text-sm text-slate-500">
          <span>
            {{ $t('common.pagination.showing') }} {{ rangeStart }} {{ $t('common.pagination.to') }} {{ rangeEnd }}
            {{ $t('common.pagination.of') }} {{ total }} {{ $t('common.pagination.results') }}
          </span>

          <div class="flex items-center gap-3">
            <button
              @click="page > 1 && goToPage(page - 1)"
              :disabled="page <= 1"
              class="px-3 py-1.5 rounded-lg bg-slate-100 hover:bg-slate-200 font-semibold disabled:opacity-50"
            >
              {{ $t('common.pagination.previous') }}
            </button>
            <span>{{ $t('common.pagination.page') }} {{ page }} {{ $t('common.pagination.of') }} {{ totalPages }}</span>
            <button
              @click="page < totalPages && goToPage(page + 1)"
              :disabled="page >= totalPages"
              class="px-3 py-1.5 rounded-lg bg-slate-100 hover:bg-slate-200 font-semibold disabled:opacity-50"
            >
              {{ $t('common.pagination.next') }}
            </button>
          </div>
        </div>
      </template>
    </DataTable>

    <TenantFormDrawer
      v-if="showForm"
      :tenant="editingTenant"
      @close="showForm = false"
      @saved="onSaved"
    />

    <TenantStatusModal
      v-if="statusTenant"
      :tenant="statusTenant"
      @close="statusTenant = null"
      @updated="onStatusUpdated"
    />
  </div>
</template>

<script>
import { listTenants, TENANT_STATUSES } from '../../services/platformTenants'
import TenantFormDrawer from './TenantFormDrawer.vue'
import TenantStatusModal from './TenantStatusModal.vue'
import PageHeader from '../../components/ui/PageHeader.vue'
import FilterBar from '../../components/ui/FilterBar.vue'
import DataTable from '../../components/ui/DataTable.vue'
import StatusBadge from '../../components/ui/StatusBadge.vue'

let searchDebounce = null
const PAGE_LIMIT = 20

export default {
  name: 'TenantManagementView',

  components: { TenantFormDrawer, TenantStatusModal, PageHeader, FilterBar, DataTable, StatusBadge },

  data() {
    return {
      TENANT_STATUSES,
      loading: false,
      listError: '',
      tenants: [],
      filters: { search: '', status: '' },
      page: 1,
      total: 0,
      totalPages: 1,
      showForm: false,
      editingTenant: null,
      statusTenant: null
    }
  },

  computed: {
    rangeStart() {
      return this.total === 0 ? 0 : (this.page - 1) * PAGE_LIMIT + 1
    },
    rangeEnd() {
      return Math.min(this.page * PAGE_LIMIT, this.total)
    }
  },

  watch: {
    'filters.status'() {
      this.page = 1
      this.fetchTenants()
    },
    'filters.search'() {
      clearTimeout(searchDebounce)
      searchDebounce = setTimeout(() => {
        this.page = 1
        this.fetchTenants()
      }, 300)
    }
  },

  mounted() {
    this.fetchTenants()
  },

  methods: {
    async fetchTenants() {
      this.loading = true
      this.listError = ''
      try {
        const res = await listTenants({
          page: this.page,
          limit: PAGE_LIMIT,
          search: this.filters.search,
          status: this.filters.status
        })
        this.tenants = res.data
        this.total = res.meta.total
        this.totalPages = res.meta.total_pages
      } catch (err) {
        this.listError = err.message || this.$t('platformAdmin.tenants.messages.loadFailed')
      } finally {
        this.loading = false
      }
    },

    goToPage(page) {
      this.page = page
      this.fetchTenants()
    },

    clearFilter() {
      this.filters.search = ''
      this.filters.status = ''
    },

    openCreate() {
      this.editingTenant = null
      this.showForm = true
    },

    openEdit(tenant) {
      this.editingTenant = tenant
      this.showForm = true
    },

    openStatus(tenant) {
      this.statusTenant = tenant
    },

    onSaved(savedTenant) {
      this.showForm = false
      const wasCreate = !this.editingTenant
      this.editingTenant = null
      if (wasCreate) {
        // Phase 6: after a successful creation, navigate to the saved
        // tenant rather than just refreshing the list.
        this.$router.push(`/platform/tenants/${savedTenant.id}`)
        return
      }
      this.fetchTenants()
    },

    onStatusUpdated() {
      this.statusTenant = null
      this.fetchTenants()
    },

    statusTone(status) {
      if (status === 'ACTIVE') return 'green'
      if (status === 'TRIAL') return 'orange'
      if (status === 'CANCELLED') return 'red'
      return 'gray'
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

.table-th {
  padding: 14px 16px;
  font-weight: var(--fw-table-header);
  font-size: var(--fs-table-header);
  white-space: nowrap;
}

.table-td {
  padding: 14px 16px;
  color: #475569;
  white-space: nowrap;
}

.badge-gray {
  background: #f1f5f9;
  color: #475569;
  padding: 5px 10px;
  border-radius: 999px;
  font-weight: 600;
  font-size: 12px;
}

.badge-green {
  background: #dcfce7;
  color: #16a34a;
  padding: 5px 10px;
  border-radius: 999px;
  font-weight: 600;
  font-size: 12px;
}

.badge-orange {
  background: #ffedd5;
  color: #ea580c;
  padding: 5px 10px;
  border-radius: 999px;
  font-weight: 600;
  font-size: 12px;
}

.badge-red {
  background: #fee2e2;
  color: #dc2626;
  padding: 5px 10px;
  border-radius: 999px;
  font-weight: 600;
  font-size: 12px;
}
</style>
