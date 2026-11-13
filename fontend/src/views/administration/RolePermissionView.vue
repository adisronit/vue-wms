<template>
  <div class="space-y-6">
    <PageHeader :title="$t('rolePermission.title')" :subtitle="$t('rolePermission.subtitle')">
      <template #actions>
        <button
          @click="openNewRole"
          class="bg-orange-600 hover:bg-orange-700 text-white px-5 py-3 rounded-xl font-semibold"
        >
          {{ $t('rolePermission.newRoleButton') }}
        </button>
      </template>
    </PageHeader>

    <div v-if="loadError" class="bg-red-50 border border-red-200 text-red-600 rounded-xl p-3 text-sm">
      {{ loadError }}
    </div>

    <div class="grid grid-cols-1 xl:grid-cols-12 gap-6">
      <!-- Role List -->
      <div class="xl:col-span-4 bg-white rounded-2xl border border-slate-200 overflow-hidden">
        <div class="p-5 border-b border-slate-200">
          <input
            v-model="roleKeyword"
            type="text"
            class="input"
            :placeholder="$t('rolePermission.searchPlaceholder')"
          />
        </div>

        <div class="max-h-[650px] overflow-y-auto">
          <button
            v-for="role in filteredRoles"
            :key="role.id"
            @click="selectRole(role)"
            class="w-full text-left p-5 border-b border-slate-100 hover:bg-orange-50 transition"
            :class="selectedRole && selectedRole.id === role.id ? 'bg-orange-50' : ''"
          >
            <div class="flex items-center justify-between gap-3">
              <div>
                <p class="font-bold text-slate-800">
                  {{ role.roleName }}
                </p>
                <p class="text-sm text-slate-500">
                  {{ role.roleCode }}
                </p>
              </div>

              <StatusBadge :tone="statusTone(role.status)">
                {{ role.status === 'ACTIVE' ? $t('common.status.active') : $t('common.status.inactive') }}
              </StatusBadge>
            </div>

            <p class="text-sm text-slate-500 mt-3">
              {{ role.description }}
            </p>
          </button>
        </div>
      </div>

      <!-- Permission Panel -->
      <div class="xl:col-span-8 bg-white rounded-2xl border border-slate-200 overflow-hidden">
        <div v-if="selectedRole">
          <div class="p-6 border-b border-slate-200 flex flex-col md:flex-row md:items-center md:justify-between gap-4">
            <div>
              <h2 class="text-xl font-bold text-slate-800">
                {{ selectedRole.roleName }}
              </h2>
              <p class="text-slate-500">
                {{ $t('rolePermission.permissionConfigLabel', { code: selectedRole.roleCode }) }}
              </p>
            </div>

            <button
              @click="savePermission"
              :disabled="saving"
              class="bg-orange-600 hover:bg-orange-700 text-white px-5 py-3 rounded-xl font-semibold disabled:opacity-60"
            >
              {{ saving ? $t('rolePermission.saving') : $t('rolePermission.savePermissionsButton') }}
            </button>
          </div>

          <div v-if="saveError" class="mx-6 mt-4 bg-red-50 border border-red-200 text-red-600 rounded-xl p-3 text-sm">
            {{ saveError }}
          </div>

          <!-- Role Info -->
          <div class="p-6 border-b border-slate-200">
            <div class="grid grid-cols-1 md:grid-cols-3 gap-4">
              <div>
                <label class="label">{{ $t('rolePermission.fields.roleName') }}</label>
                <input
                  v-model="roleForm.roleName"
                  type="text"
                  class="input"
                />
              </div>

              <div>
                <label class="label">{{ $t('rolePermission.fields.roleCode') }}</label>
                <input
                  v-model="roleForm.roleCode"
                  type="text"
                  class="input"
                />
              </div>

              <div>
                <label class="label">{{ $t('rolePermission.fields.status') }}</label>
                <select v-model="roleForm.status" class="input">
                  <option value="ACTIVE">{{ $t('common.status.active') }}</option>
                  <option value="INACTIVE">{{ $t('common.status.inactive') }}</option>
                </select>
              </div>
            </div>

            <div class="mt-4">
              <label class="label">{{ $t('rolePermission.fields.description') }}</label>
              <textarea
                v-model="roleForm.description"
                rows="3"
                class="input"
              ></textarea>
            </div>
          </div>

          <!-- Warehouse Access -->
          <div class="p-6 border-b border-slate-200">
            <h3 class="font-bold text-slate-800 mb-4">
              {{ $t('rolePermission.warehouseAccess.title') }}
            </h3>

            <p v-if="warehousesLoading" class="text-sm text-slate-400">{{ $t('rolePermission.warehouseAccess.loading') }}</p>
            <p v-else-if="warehousesError" class="text-sm text-red-600">{{ warehousesError }}</p>
            <p v-else-if="warehouses.length === 0" class="text-sm text-slate-400">{{ $t('rolePermission.warehouseAccess.noWarehouses') }}</p>

            <div v-else class="grid grid-cols-1 md:grid-cols-3 gap-3">
              <label
                v-for="warehouse in warehouses"
                :key="warehouse.id"
                class="checkbox-card"
              >
                <input
                  type="checkbox"
                  :value="warehouse.id"
                  v-model="roleForm.warehouseAccess"
                />

                <div>
                  <p class="font-semibold text-slate-800">
                    {{ warehouse.code }}
                  </p>
                  <p class="text-sm text-slate-500">
                    {{ warehouse.name }}
                  </p>
                </div>
              </label>
            </div>
          </div>

          <!-- Permission Matrix -->
          <div class="p-6">
            <div class="flex items-center justify-between mb-4">
              <h3 class="font-bold text-slate-800">
                {{ $t('rolePermission.permissionMatrix.title') }}
              </h3>

              <button
                @click="toggleAllPermissions"
                class="text-orange-600 hover:text-orange-700 font-semibold text-sm"
              >
                {{ allChecked ? $t('rolePermission.permissionMatrix.uncheckAll') : $t('rolePermission.permissionMatrix.checkAll') }}
              </button>
            </div>

            <div class="overflow-x-auto border border-slate-200 rounded-2xl">
              <table class="min-w-[850px] w-full text-sm">
                <thead class="bg-slate-50 text-slate-500">
                  <tr>
                    <th class="table-th text-left">{{ $t('rolePermission.permissionMatrix.moduleColumn') }}</th>
                    <th
                      v-for="action in actions"
                      :key="action"
                      class="table-th text-center"
                    >
                      {{ actionLabel(action) }}
                    </th>
                  </tr>
                </thead>

                <tbody>
                  <tr
                    v-for="module in permissionModules"
                    :key="module.code"
                    class="border-t hover:bg-orange-50"
                  >
                    <td class="table-td font-semibold text-slate-800">
                      {{ moduleName(module.code) }}
                    </td>

                    <td
                      v-for="action in actions"
                      :key="action"
                      class="table-td text-center"
                    >
                      <input
                        v-if="module.actions.includes(action)"
                        type="checkbox"
                        v-model="roleForm.permissions[module.code][action]"
                        class="permission-checkbox"
                      />

                      <span v-else class="text-slate-300">-</span>
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>
        </div>

        <div
          v-else
          class="h-[500px] flex items-center justify-center text-slate-400"
        >
          {{ $t('rolePermission.selectRolePrompt') }}
        </div>
      </div>
    </div>
  </div>
</template>

<script>
import {
  listRoles,
  listPermissions,
  getRolePermissions,
  setRolePermissions,
  getRoleWarehouseAccess,
  setRoleWarehouseAccess
} from '../../services/roles'
import { listWarehouses } from '../../services/warehouses'
import { showToast } from '../../services/toast'
import PageHeader from '../../components/ui/PageHeader.vue'
import StatusBadge from '../../components/ui/StatusBadge.vue'

// Module display order/names and the action label<->backend action code
// mapping. The frontend previously hardcoded the full permission matrix;
// now only these labels are local — which modules/actions actually exist
// and which are granted per role both come from the API.
//
// MODULE_ORDER/ACTION_CODES stay plain English keys — they are object
// keys used for lookups (roleForm.permissions[module.code][action],
// ACTION_CODES[action]) and must never change with locale. Only the
// *display* string (MODULE_NAMES values, actionLabel() below) is
// translated.
const MODULE_ORDER = [
  'dashboard', 'receiving', 'quality_check', 'putaway', 'stock_inquiry',
  'stock_adjustment', 'order', 'picking', 'shipping', 'users', 'roles', 'warehouse', 'product',
  'tenant_users'
]
// Maps each module code to its rolePermission.modules.* key suffix.
// Display names are looked up through this at render time (see the
// moduleName() method), never cached onto permissionModules, so a
// locale switch updates the matrix immediately without a re-fetch.
const MODULE_NAME_KEYS = {
  dashboard: 'dashboard',
  receiving: 'receiving',
  quality_check: 'qualityCheck',
  putaway: 'putaway',
  stock_inquiry: 'stockInquiry',
  stock_adjustment: 'stockAdjustment',
  order: 'order',
  picking: 'picking',
  shipping: 'shipping',
  users: 'users',
  roles: 'roles',
  warehouse: 'warehouse',
  product: 'product',
  tenant_users: 'tenantUsers'
}
const ACTION_CODES = { View: 'view', Create: 'create', Edit: 'update', Delete: 'delete', Approve: 'approve', Export: 'export', Suspend: 'suspend' }
const ACTION_LABEL_KEYS = { View: 'view', Create: 'create', Edit: 'edit', Delete: 'delete', Approve: 'approve', Export: 'export', Suspend: 'suspend' }

export default {
  name: 'RolePermissionView',

  components: { PageHeader, StatusBadge },

  data() {
    return {
      roleKeyword: '',
      selectedRole: null,
      allChecked: false,
      loadError: '',
      saveError: '',
      saving: false,

      // Warehouse Access: available options load from the real warehouse
      // master (GET /warehouses); a role's assigned access loads from
      // GET /roles/{id}/warehouse-access when it's selected below.
      warehouses: [],
      warehousesLoading: false,
      warehousesError: '',

      actions: ['View', 'Create', 'Edit', 'Delete', 'Approve', 'Export', 'Suspend'],
      permissionModules: [],
      roles: [],

      roleForm: {
        roleName: '',
        roleCode: '',
        description: '',
        status: 'ACTIVE',
        warehouseAccess: [],
        permissions: {}
      }
    }
  },

  computed: {
    filteredRoles() {
      return this.roles.filter((role) => {
        const keyword = this.roleKeyword.toLowerCase()

        return (
          role.roleName.toLowerCase().includes(keyword) ||
          role.roleCode.toLowerCase().includes(keyword)
        )
      })
    }
  },

  async mounted() {
    await Promise.all([this.fetchCatalog(), this.fetchWarehouses()])
    if (this.roles.length) {
      await this.selectRole(this.roles[0])
    }
  },

  methods: {
    moduleName(code) {
      return this.$t('rolePermission.modules.' + (MODULE_NAME_KEYS[code] || code))
    },

    // Only active warehouses — matches Warehouse Setup's own precedent
    // for a picker (e.g. User Management's Warehouse select) rather than
    // Warehouse Setup's own management screen, which shows every status.
    async fetchWarehouses() {
      this.warehousesLoading = true
      this.warehousesError = ''
      try {
        const res = await listWarehouses({ status: 'ACTIVE', limit: 100 })
        this.warehouses = res.data
      } catch (err) {
        this.warehousesError = err.message || this.$t('rolePermission.messages.loadWarehousesFailed')
      } finally {
        this.warehousesLoading = false
      }
    },

    async fetchCatalog() {
      this.loadError = ''
      try {
        const [rolesRes, permissionsRes] = await Promise.all([listRoles(), listPermissions()])

        this.roles = rolesRes.data.map((r) => ({
          id: r.id,
          roleCode: r.code,
          roleName: r.name,
          description: r.description,
          status: r.is_active ? 'ACTIVE' : 'INACTIVE'
        }))

        const actionsByModule = {}
        permissionsRes.data.forEach((p) => {
          if (!actionsByModule[p.module]) actionsByModule[p.module] = new Set()
          actionsByModule[p.module].add(p.action)
        })

        this.permissionModules = MODULE_ORDER
          .filter((code) => actionsByModule[code])
          .map((code) => ({
            code,
            actions: this.actions.filter((label) => actionsByModule[code].has(ACTION_CODES[label]))
          }))
      } catch (err) {
        this.loadError = err.message || this.$t('rolePermission.messages.loadFailed')
      }
    },

    actionLabel(action) {
      return this.$t(`rolePermission.actions.${ACTION_LABEL_KEYS[action]}`)
    },

    createEmptyPermissions() {
      const permissions = {}

      this.permissionModules.forEach((module) => {
        permissions[module.code] = {}

        this.actions.forEach((action) => {
          permissions[module.code][action] = false
        })
      })

      return permissions
    },

    async selectRole(role) {
      this.selectedRole = role
      this.saveError = ''

      const permissions = this.createEmptyPermissions()

      try {
        const res = await getRolePermissions(role.id)
        res.data.permission_codes.forEach((code) => {
          const [moduleCode, actionCode] = code.split('.')
          const label = Object.keys(ACTION_CODES).find((k) => ACTION_CODES[k] === actionCode)
          if (permissions[moduleCode] && label) {
            permissions[moduleCode][label] = true
          }
        })
      } catch (err) {
        this.loadError = err.message || this.$t('rolePermission.messages.loadPermissionsFailed')
      }

      let warehouseAccess = []
      try {
        const res = await getRoleWarehouseAccess(role.id)
        warehouseAccess = res.data.warehouse_access.map((w) => w.id)
      } catch (err) {
        this.loadError = err.message || this.$t('rolePermission.messages.loadAccessFailed')
      }

      this.roleForm = {
        roleName: role.roleName,
        roleCode: role.roleCode,
        description: role.description,
        status: role.status,
        warehouseAccess,
        permissions
      }

      this.updateAllCheckedState()
    },

    // "+ New Role" stays a local-only draft: the backend intentionally
    // does not implement role create/delete this phase (see
    // PROJECT_STATUS.md) because the frontend never actually persisted
    // it either. savePermission() below refuses to save this draft
    // instead of pretending it succeeded.
    openNewRole() {
      this.selectedRole = {
        id: null,
        roleCode: 'NEW_ROLE',
        roleName: this.$t('rolePermission.newRoleDefaults.name'),
        description: '',
        status: 'ACTIVE',
        warehouseAccess: []
      }

      this.roleForm = {
        roleName: this.$t('rolePermission.newRoleDefaults.name'),
        roleCode: 'NEW_ROLE',
        description: '',
        status: 'ACTIVE',
        warehouseAccess: [],
        permissions: this.createEmptyPermissions()
      }

      this.updateAllCheckedState()
    },

    toggleAllPermissions() {
      this.allChecked = !this.allChecked

      this.permissionModules.forEach((module) => {
        module.actions.forEach((action) => {
          this.roleForm.permissions[module.code][action] = this.allChecked
        })
      })
    },

    updateAllCheckedState() {
      const availablePermissions = []

      this.permissionModules.forEach((module) => {
        module.actions.forEach((action) => {
          availablePermissions.push(this.roleForm.permissions[module.code][action])
        })
      })

      this.allChecked = availablePermissions.length > 0 && availablePermissions.every((item) => item === true)
    },

    async savePermission() {
      this.saveError = ''

      if (!this.selectedRole || !this.selectedRole.id) {
        this.saveError = this.$t('rolePermission.messages.createNotSupported')
        return
      }

      const codes = []
      this.permissionModules.forEach((module) => {
        module.actions.forEach((action) => {
          if (this.roleForm.permissions[module.code][action]) {
            codes.push(`${module.code}.${ACTION_CODES[action]}`)
          }
        })
      })

      // Warehouse checkbox values come from warehouse.id, which — like
      // every other ID this API returns — is JSON-encoded as a string
      // (see WarehouseResponse's id,string convention); the backend's
      // warehouse_ids field is a plain []int64, so these must be
      // converted to real numbers before being sent.
      const warehouseIds = this.roleForm.warehouseAccess.map((id) => Number(id))

      this.saving = true
      try {
        await Promise.all([
          setRolePermissions(this.selectedRole.id, codes),
          setRoleWarehouseAccess(this.selectedRole.id, warehouseIds)
        ])
        showToast(this.$t('rolePermission.messages.saveSuccess'), 'success')
      } catch (err) {
        this.saveError = err.message || this.$t('rolePermission.messages.saveFailed')
      } finally {
        this.saving = false
      }
    },

    statusTone(status) {
      if (status === 'ACTIVE') return 'green'
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

.label {
  display: block;
  margin-bottom: 8px;
  color: #334155;
  font-weight: var(--fw-label);
  font-size: var(--fs-label);
}

.checkbox-card {
  display: flex;
  gap: 12px;
  align-items: flex-start;
  border: 1px solid #e2e8f0;
  border-radius: 14px;
  padding: 14px;
  cursor: pointer;
}

.checkbox-card:hover {
  background: #fff7ed;
  border-color: #fb923c;
}

.permission-checkbox {
  width: 18px;
  height: 18px;
  accent-color: #ea580c;
  cursor: pointer;
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