<template>
  <div class="space-y-6">
    <div class="flex flex-col xl:flex-row xl:items-center xl:justify-between gap-4">
      <div>
        <h1 class="text-2xl font-bold text-slate-800">{{ $t('userManagement.title') }}</h1>
        <p class="text-slate-500">
          {{ $t('userManagement.subtitle') }}
        </p>
      </div>

      <button
        @click="openUser(null)"
        class="bg-orange-600 hover:bg-orange-700 text-white px-5 py-3 rounded-xl font-semibold"
      >
        {{ $t('userManagement.newUserButton') }}
      </button>
    </div>

    <div class="bg-white rounded-2xl border border-slate-200 p-5">
      <div class="grid grid-cols-1 md:grid-cols-4 gap-4">
        <input v-model="filters.keyword" type="text" :placeholder="$t('userManagement.filters.searchPlaceholder')" class="input" />

        <select v-model="filters.role" class="input">
          <option value="">{{ $t('userManagement.filters.allRoles') }}</option>
          <option v-for="role in roles" :key="role.code" :value="role.code">{{ role.name }}</option>
        </select>

        <select v-model="filters.status" class="input">
          <option value="">{{ $t('userManagement.filters.allStatus') }}</option>
          <option value="ACTIVE">{{ $t('common.status.active') }}</option>
          <option value="INACTIVE">{{ $t('common.status.inactive') }}</option>
          <option value="LOCKED">{{ $t('userManagement.status.locked') }}</option>
        </select>

        <button @click="clearFilter" class="bg-slate-100 hover:bg-slate-200 text-slate-700 rounded-xl font-semibold">
          {{ $t('common.buttons.clearFilter') }}
        </button>
      </div>
    </div>

    <div v-if="listError" class="bg-red-50 border border-red-200 text-red-600 rounded-xl p-3 text-sm">
      {{ listError }}
    </div>

    <div class="bg-white rounded-2xl border border-slate-200 shadow-sm overflow-hidden">
      <div class="w-full overflow-x-auto">
        <table class="min-w-[1000px] w-full text-sm">
          <thead class="bg-slate-50 text-slate-500">
            <tr>
              <th class="table-th">{{ $t('userManagement.table.user') }}</th>
              <th class="table-th">{{ $t('userManagement.table.email') }}</th>
              <th class="table-th">{{ $t('userManagement.table.role') }}</th>
              <th class="table-th">{{ $t('userManagement.table.warehouse') }}</th>
              <th class="table-th">{{ $t('userManagement.table.lastLogin') }}</th>
              <th class="table-th">{{ $t('userManagement.table.status') }}</th>
              <th class="table-th text-right">{{ $t('userManagement.table.action') }}</th>
            </tr>
          </thead>

          <tbody>
            <tr v-if="loading">
              <td class="table-td" colspan="7">{{ $t('userManagement.loadingUsers') }}</td>
            </tr>
            <tr v-else-if="users.length === 0">
              <td class="table-td" colspan="7">{{ $t('userManagement.noUsers') }}</td>
            </tr>
            <tr v-for="user in users" :key="user.id" class="border-t hover:bg-orange-50">
              <td class="table-td font-semibold text-slate-800">{{ user.full_name }}</td>
              <td class="table-td">{{ user.email }}</td>
              <td class="table-td">{{ user.role.name }}</td>
              <td class="table-td">{{ formatWarehouse(user) }}</td>
              <td class="table-td">{{ formatLastLogin(user.last_login_at) }}</td>
              <td class="table-td">
                <span :class="getStatusClass(user.status)">
                  {{ statusLabel(user.status) }}
                </span>
              </td>
              <td class="table-td text-right">
                <button @click="openUser(user)" class="text-orange-600 hover:text-orange-700 font-semibold">
                  {{ $t('common.buttons.edit') }}
                </button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <div v-if="selectedUser" class="fixed inset-0 bg-black/40 flex justify-end z-50">
      <div class="w-full max-w-2xl bg-white h-full p-6 overflow-y-auto">
        <div class="flex items-center justify-between mb-6">
          <div>
            <h2 class="text-xl font-bold text-slate-800">
              {{ selectedUser.id ? $t('userManagement.drawer.editTitle') : $t('userManagement.drawer.newTitle') }}
            </h2>
            <p class="text-slate-500">{{ $t('userManagement.drawer.subtitle') }}</p>
          </div>

          <button @click="selectedUser = null" class="text-slate-500 hover:text-red-600 text-xl">
            ✕
          </button>
        </div>

        <div class="space-y-4">
          <div>
            <label class="label">{{ $t('userManagement.fields.fullName') }}</label>
            <input v-model="userForm.fullName" type="text" class="input" :placeholder="$t('userManagement.fields.fullNamePlaceholder')" />
          </div>

          <div>
            <label class="label">{{ $t('userManagement.fields.email') }}</label>
            <input v-model="userForm.email" type="email" class="input" :placeholder="$t('userManagement.fields.emailPlaceholder')" />
          </div>

          <div>
            <label class="label">{{ $t('userManagement.fields.username') }}</label>
            <input v-model="userForm.username" type="text" class="input" :placeholder="$t('userManagement.fields.usernamePlaceholder')" :disabled="!!selectedUser.id" />
          </div>

          <!-- Only collected on create: the backend has no separate
               password-reset endpoint yet, and editing an existing user
               never requires re-entering it. -->
          <div v-if="!selectedUser.id">
            <label class="label">{{ $t('userManagement.fields.password') }}</label>
            <input v-model="userForm.password" type="password" class="input" :placeholder="$t('userManagement.fields.passwordPlaceholder')" />
          </div>

          <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div>
              <label class="label">{{ $t('userManagement.fields.role') }}</label>
              <select v-model="userForm.role" class="input">
                <option value="">{{ $t('userManagement.fields.selectRole') }}</option>
                <option v-for="role in roles" :key="role.code" :value="role.code">{{ role.name }}</option>
              </select>
            </div>

            <div>
              <label class="label">{{ $t('userManagement.fields.warehouse') }}</label>
              <select v-model="userForm.warehouseId" class="input">
                <option value="">{{ $t('userManagement.fields.unassigned') }}</option>
                <option v-for="wh in warehouses" :key="wh.id" :value="wh.id">{{ wh.code }} - {{ wh.name }}</option>
              </select>
            </div>
          </div>

          <div>
            <label class="label">{{ $t('userManagement.fields.status') }}</label>
            <select v-model="userForm.status" class="input">
              <option value="ACTIVE">{{ $t('common.status.active') }}</option>
              <option value="INACTIVE">{{ $t('common.status.inactive') }}</option>
              <option value="LOCKED">{{ $t('userManagement.status.locked') }}</option>
            </select>
          </div>

          <div v-if="formError" class="bg-red-50 border border-red-200 text-red-600 rounded-xl p-3 text-sm">
            {{ formError }}
          </div>

          <button
            @click="saveUser"
            :disabled="saving"
            class="w-full bg-orange-600 hover:bg-orange-700 text-white py-3 rounded-xl font-semibold mt-4 disabled:opacity-60"
          >
            {{ saving ? $t('userManagement.saving') : $t('userManagement.saveButton') }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script>
import { listUsers, createUser, updateUser } from '../../services/users'
import { listRoles } from '../../services/roles'
import { listWarehouses } from '../../services/warehouses'

let searchDebounce = null

export default {
  name: 'UserManagementView',

  data() {
    return {
      selectedUser: null,
      loading: false,
      saving: false,
      listError: '',
      formError: '',
      roles: [],
      warehouses: [],
      users: [],
      filters: {
        keyword: '',
        role: '',
        status: ''
      },
      userForm: {
        fullName: '',
        email: '',
        username: '',
        password: '',
        role: '',
        warehouseId: '',
        status: 'ACTIVE'
      }
    }
  },

  watch: {
    'filters.role'() {
      this.fetchUsers()
    },
    'filters.status'() {
      this.fetchUsers()
    },
    'filters.keyword'() {
      clearTimeout(searchDebounce)
      searchDebounce = setTimeout(() => this.fetchUsers(), 300)
    }
  },

  async mounted() {
    await this.fetchRoles()
    await this.fetchWarehouses()
    await this.fetchUsers()
  },

  methods: {
    async fetchRoles() {
      try {
        const res = await listRoles()
        this.roles = res.data
      } catch (err) {
        this.listError = err.message || this.$t('userManagement.messages.loadRolesFailed')
      }
    },

    async fetchWarehouses() {
      try {
        const res = await listWarehouses({ status: 'ACTIVE', limit: 100 })
        this.warehouses = res.data
      } catch (err) {
        this.listError = err.message || this.$t('userManagement.messages.loadWarehousesFailed')
      }
    },

    async fetchUsers() {
      this.loading = true
      this.listError = ''
      try {
        const res = await listUsers({
          search: this.filters.keyword,
          role: this.filters.role,
          status: this.filters.status,
          limit: 100
        })
        this.users = res.data
      } catch (err) {
        this.listError = err.message || this.$t('userManagement.messages.loadUsersFailed')
      } finally {
        this.loading = false
      }
    },

    openUser(user) {
      this.formError = ''
      this.selectedUser = user || {}

      this.userForm = user
        ? {
            fullName: user.full_name,
            email: user.email,
            username: user.username,
            password: '',
            role: user.role.code,
            warehouseId: user.warehouse_id || '',
            status: user.status
          }
        : {
            fullName: '',
            email: '',
            username: '',
            password: '',
            role: '',
            warehouseId: '',
            status: 'ACTIVE'
          }
    },

    clearFilter() {
      this.filters.keyword = ''
      this.filters.role = ''
      this.filters.status = ''
    },

    async saveUser() {
      this.formError = ''
      this.saving = true

      try {
        if (this.selectedUser.id) {
          await updateUser(this.selectedUser.id, {
            full_name: this.userForm.fullName,
            email: this.userForm.email,
            role_code: this.userForm.role,
            warehouse_id: this.userForm.warehouseId || null,
            status: this.userForm.status,
            ...(this.userForm.password ? { password: this.userForm.password } : {})
          })
        } else {
          await createUser({
            username: this.userForm.username,
            password: this.userForm.password,
            full_name: this.userForm.fullName,
            email: this.userForm.email,
            role_code: this.userForm.role,
            warehouse_id: this.userForm.warehouseId || null,
            status: this.userForm.status
          })
        }

        this.selectedUser = null
        await this.fetchUsers()
      } catch (err) {
        this.formError = err.message || this.$t('userManagement.messages.saveFailed')
      } finally {
        this.saving = false
      }
    },

    // "-" for no warehouse: no warehouse-scoped authorization exists in
    // the backend yet (see migrations/0039's own doc), so a NULL
    // warehouse_id means "unassigned," not "all warehouses" — showing
    // "All Warehouses" here would imply a capability that isn't real.
    formatWarehouse(user) {
      if (!user.warehouse_code) return '-'
      return `${user.warehouse_code} - ${user.warehouse_name}`
    },

    formatLastLogin(isoString) {
      if (!isoString) return this.$t('userManagement.messages.neverLoggedIn')
      return this.$d(new Date(isoString), 'long')
    },

    statusLabel(status) {
      if (status === 'ACTIVE') return this.$t('common.status.active')
      if (status === 'INACTIVE') return this.$t('common.status.inactive')
      if (status === 'LOCKED') return this.$t('userManagement.status.locked')
      return status
    },

    getStatusClass(status) {
      if (status === 'ACTIVE') return 'badge-green'
      if (status === 'INACTIVE') return 'badge-gray'
      if (status === 'LOCKED') return 'badge-red'
      return 'badge-gray'
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
  font-weight: 600;
  font-size: 14px;
}

.table-th {
  padding: 14px 16px;
  font-weight: 700;
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

.badge-red {
  background: #fee2e2;
  color: #dc2626;
  padding: 5px 10px;
  border-radius: 999px;
  font-weight: 600;
  font-size: 12px;
}
</style>