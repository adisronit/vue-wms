<template>
  <div class="space-y-6">
    <div class="flex flex-col md:flex-row md:items-center md:justify-between gap-4">
      <div>
        <h2 class="text-lg font-bold text-slate-800">{{ $t('platformAdmin.tenantUsers.title') }}</h2>
        <p class="text-slate-500 text-sm">{{ $t('platformAdmin.tenantUsers.subtitle') }}</p>
      </div>

      <div class="flex items-center gap-3">
        <button
          @click="showAssignExisting = true"
          class="bg-slate-100 hover:bg-slate-200 text-slate-700 px-4 py-2.5 rounded-xl font-semibold"
        >
          {{ $t('platformAdmin.tenantUsers.assignExistingButton') }}
        </button>
        <button
          @click="showForm = true"
          class="bg-orange-600 hover:bg-orange-700 text-white px-4 py-2.5 rounded-xl font-semibold"
        >
          {{ $t('platformAdmin.tenantUsers.addButton') }}
        </button>
      </div>
    </div>

    <FilterBar :cols="4">
      <input
        v-model="filters.search"
        type="text"
        :placeholder="$t('platformAdmin.tenantUsers.filters.searchPlaceholder')"
        class="input md:col-span-2"
      />

      <select v-model="filters.role" class="input">
        <option value="">{{ $t('platformAdmin.tenantUsers.filters.allRoles') }}</option>
        <option v-for="code in TENANT_ROLE_CODES" :key="code" :value="code">
          {{ $t('platformAdmin.tenantUsers.roles.' + roleKey(code)) }}
        </option>
      </select>

      <select v-model="filters.status" class="input">
        <option value="">{{ $t('platformAdmin.tenantUsers.filters.allStatus') }}</option>
        <option v-for="code in MEMBERSHIP_STATUSES" :key="code" :value="code">
          {{ $t('platformAdmin.tenantUsers.status.' + code.toLowerCase()) }}
        </option>
      </select>
    </FilterBar>

    <div v-if="listError" class="bg-red-50 border border-red-200 text-red-600 rounded-xl p-3 text-sm">
      {{ listError }}
    </div>

    <DataTable
      :loading="loading"
      :is-empty="members.length === 0"
      :loading-text="$t('platformAdmin.tenantUsers.table.loading')"
      :empty-text="$t('platformAdmin.tenantUsers.table.noData')"
      :colspan="5"
      min-width="900px"
    >
      <template #head>
        <tr>
          <th class="table-th">{{ $t('platformAdmin.tenantUsers.table.user') }}</th>
          <th class="table-th">{{ $t('platformAdmin.tenantUsers.table.role') }}</th>
          <th class="table-th">{{ $t('platformAdmin.tenantUsers.table.status') }}</th>
          <th class="table-th">{{ $t('platformAdmin.tenantUsers.table.joinedAt') }}</th>
          <th class="table-th text-right">{{ $t('platformAdmin.tenantUsers.table.action') }}</th>
        </tr>
      </template>

      <tr v-for="m in members" :key="m.user_id" class="border-t hover:bg-orange-50">
        <td class="table-td">
          <div class="font-semibold text-slate-800">{{ m.full_name }}</div>
          <div class="text-xs text-slate-400">{{ m.username }} &middot; {{ m.email }}</div>
        </td>
        <td class="table-td">{{ $t('platformAdmin.tenantUsers.roles.' + roleKey(m.role.code)) }}</td>
        <td class="table-td">
          <StatusBadge :tone="statusTone(m.status)">{{ $t('platformAdmin.tenantUsers.status.' + m.status.toLowerCase()) }}</StatusBadge>
        </td>
        <td class="table-td">{{ m.joined_at ? $d(new Date(m.joined_at), 'short') : '—' }}</td>
        <td class="table-td text-right space-x-3 whitespace-nowrap">
          <button @click="openChangeRole(m)" class="text-slate-600 hover:text-slate-800 font-semibold">
            {{ $t('platformAdmin.tenantUsers.table.changeRole') }}
          </button>
          <button @click="openChangeStatus(m)" class="text-slate-600 hover:text-slate-800 font-semibold">
            {{ m.status === 'DISABLED' ? $t('platformAdmin.tenantUsers.table.reactivate') : $t('platformAdmin.tenantUsers.table.suspend') }}
          </button>
        </td>
      </tr>

      <template v-if="!loading && members.length > 0" #footer>
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

    <TenantUserForm
      v-if="showForm"
      :tenant-id="tenantId"
      @close="showForm = false"
      @created="onCreated"
    />

    <AssignExistingUserDialog
      v-if="showAssignExisting"
      :tenant-id="tenantId"
      @close="showAssignExisting = false"
      @created="onCreated"
    />

    <ChangeRoleDialog
      v-if="roleTarget"
      :tenant-id="tenantId"
      :member="roleTarget"
      @close="roleTarget = null"
      @updated="onUpdated"
    />

    <MembershipStatusDialog
      v-if="statusTarget"
      :tenant-id="tenantId"
      :member="statusTarget"
      @close="statusTarget = null"
      @updated="onUpdated"
    />
  </div>
</template>

<script>
import { listTenantUsers, TENANT_ROLE_CODES, MEMBERSHIP_STATUSES } from '../../services/platformTenantUsers'
import TenantUserForm from './TenantUserForm.vue'
import AssignExistingUserDialog from './AssignExistingUserDialog.vue'
import ChangeRoleDialog from './ChangeRoleDialog.vue'
import MembershipStatusDialog from './MembershipStatusDialog.vue'
import FilterBar from '../../components/ui/FilterBar.vue'
import DataTable from '../../components/ui/DataTable.vue'
import StatusBadge from '../../components/ui/StatusBadge.vue'

let searchDebounce = null
const PAGE_LIMIT = 20

// ROLE_NAME_KEYS maps a role code to its i18n key suffix — plain
// English keys used only for lookups, never shown directly (mirrors
// RolePermissionView.vue's own MODULE_NAME_KEYS convention).
const ROLE_NAME_KEYS = {
  ADMIN: 'admin',
  WAREHOUSE_MANAGER: 'warehouseManager',
  RECEIVING: 'receiving',
  QC: 'qc',
  PUTAWAY: 'putaway',
  PICKING: 'picking',
  SHIPPING: 'shipping'
}

export default {
  name: 'TenantUserList',

  components: {
    TenantUserForm,
    AssignExistingUserDialog,
    ChangeRoleDialog,
    MembershipStatusDialog,
    FilterBar,
    DataTable,
    StatusBadge
  },

  props: {
    tenantId: {
      type: [String, Number],
      required: true
    }
  },

  data() {
    return {
      TENANT_ROLE_CODES,
      MEMBERSHIP_STATUSES,
      loading: false,
      listError: '',
      members: [],
      filters: { search: '', role: '', status: '' },
      page: 1,
      total: 0,
      totalPages: 1,
      showForm: false,
      showAssignExisting: false,
      roleTarget: null,
      statusTarget: null
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
    'filters.role'() {
      this.page = 1
      this.fetchMembers()
    },
    'filters.status'() {
      this.page = 1
      this.fetchMembers()
    },
    'filters.search'() {
      clearTimeout(searchDebounce)
      searchDebounce = setTimeout(() => {
        this.page = 1
        this.fetchMembers()
      }, 300)
    }
  },

  mounted() {
    this.fetchMembers()
  },

  methods: {
    roleKey(code) {
      return ROLE_NAME_KEYS[code] || code
    },

    async fetchMembers() {
      this.loading = true
      this.listError = ''
      try {
        const res = await listTenantUsers(this.tenantId, {
          page: this.page,
          limit: PAGE_LIMIT,
          search: this.filters.search,
          role: this.filters.role,
          status: this.filters.status
        })
        this.members = res.data
        this.total = res.meta.total
        this.totalPages = res.meta.total_pages
      } catch (err) {
        this.listError = err.message || this.$t('platformAdmin.tenantUsers.messages.loadFailed')
      } finally {
        this.loading = false
      }
    },

    goToPage(page) {
      this.page = page
      this.fetchMembers()
    },

    openChangeRole(member) {
      this.roleTarget = member
    },

    openChangeStatus(member) {
      this.statusTarget = member
    },

    onCreated() {
      this.showForm = false
      this.showAssignExisting = false
      this.page = 1
      this.fetchMembers()
    },

    onUpdated() {
      this.roleTarget = null
      this.statusTarget = null
      this.fetchMembers()
    },

    statusTone(status) {
      if (status === 'ACTIVE') return 'green'
      if (status === 'INVITED') return 'orange'
      if (status === 'DISABLED') return 'gray'
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
</style>
