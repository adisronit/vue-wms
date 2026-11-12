<template>
  <div class="space-y-6">
    <div class="flex flex-col xl:flex-row xl:items-center xl:justify-between gap-4">
      <div>
        <h1 class="text-2xl font-bold text-slate-800">Roles & Permissions</h1>
        <p class="text-slate-500">
          Manage role-based access control for warehouse operations
        </p>
      </div>

      <button
        @click="openNewRole"
        class="bg-orange-600 hover:bg-orange-700 text-white px-5 py-3 rounded-xl font-semibold"
      >
        + New Role
      </button>
    </div>

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
            placeholder="Search role..."
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

              <span :class="getStatusClass(role.status)">
                {{ role.status }}
              </span>
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
                {{ selectedRole.roleCode }} permission configuration
              </p>
            </div>

            <button
              @click="savePermission"
              :disabled="saving"
              class="bg-orange-600 hover:bg-orange-700 text-white px-5 py-3 rounded-xl font-semibold disabled:opacity-60"
            >
              {{ saving ? 'Saving...' : 'Save Permissions' }}
            </button>
          </div>

          <div v-if="saveError" class="mx-6 mt-4 bg-red-50 border border-red-200 text-red-600 rounded-xl p-3 text-sm">
            {{ saveError }}
          </div>

          <!-- Role Info -->
          <div class="p-6 border-b border-slate-200">
            <div class="grid grid-cols-1 md:grid-cols-3 gap-4">
              <div>
                <label class="label">Role Name</label>
                <input
                  v-model="roleForm.roleName"
                  type="text"
                  class="input"
                />
              </div>

              <div>
                <label class="label">Role Code</label>
                <input
                  v-model="roleForm.roleCode"
                  type="text"
                  class="input"
                />
              </div>

              <div>
                <label class="label">Status</label>
                <select v-model="roleForm.status" class="input">
                  <option value="ACTIVE">ACTIVE</option>
                  <option value="INACTIVE">INACTIVE</option>
                </select>
              </div>
            </div>

            <div class="mt-4">
              <label class="label">Description</label>
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
              Warehouse Access
            </h3>

            <p v-if="warehousesLoading" class="text-sm text-slate-400">Loading warehouses...</p>
            <p v-else-if="warehousesError" class="text-sm text-red-600">{{ warehousesError }}</p>
            <p v-else-if="warehouses.length === 0" class="text-sm text-slate-400">No active warehouses found.</p>

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
                Permission Matrix
              </h3>

              <button
                @click="toggleAllPermissions"
                class="text-orange-600 hover:text-orange-700 font-semibold text-sm"
              >
                {{ allChecked ? 'Uncheck All' : 'Check All' }}
              </button>
            </div>

            <div class="overflow-x-auto border border-slate-200 rounded-2xl">
              <table class="min-w-[850px] w-full text-sm">
                <thead class="bg-slate-50 text-slate-500">
                  <tr>
                    <th class="table-th text-left">Module</th>
                    <th
                      v-for="action in actions"
                      :key="action"
                      class="table-th text-center"
                    >
                      {{ action }}
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
                      {{ module.name }}
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
          Select a role to configure permissions
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

// Module display order/names and the action label<->backend action code
// mapping. The frontend previously hardcoded the full permission matrix;
// now only these labels are local — which modules/actions actually exist
// and which are granted per role both come from the API.
const MODULE_ORDER = [
  'dashboard', 'receiving', 'quality_check', 'putaway', 'stock_inquiry',
  'stock_adjustment', 'order', 'picking', 'shipping', 'users', 'roles', 'warehouse', 'product'
]
const MODULE_NAMES = {
  dashboard: 'Dashboard',
  receiving: 'Receiving',
  quality_check: 'Quality Check',
  putaway: 'Putaway',
  stock_inquiry: 'Stock Inquiry',
  stock_adjustment: 'Stock Adjustment',
  order: 'Order Management',
  picking: 'Picking',
  shipping: 'Shipping',
  users: 'User Management',
  roles: 'Roles & Permissions',
  warehouse: 'Warehouse Setup',
  product: 'Product Master'
}
const ACTION_CODES = { View: 'view', Create: 'create', Edit: 'update', Delete: 'delete', Approve: 'approve', Export: 'export' }

export default {
  name: 'RolePermissionView',

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

      actions: ['View', 'Create', 'Edit', 'Delete', 'Approve', 'Export'],
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
        this.warehousesError = err.message || 'Failed to load warehouses'
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
            name: MODULE_NAMES[code] || code,
            actions: this.actions.filter((label) => actionsByModule[code].has(ACTION_CODES[label]))
          }))
      } catch (err) {
        this.loadError = err.message || 'Failed to load roles and permissions'
      }
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
        this.loadError = err.message || 'Failed to load role permissions'
      }

      let warehouseAccess = []
      try {
        const res = await getRoleWarehouseAccess(role.id)
        warehouseAccess = res.data.warehouse_access.map((w) => w.id)
      } catch (err) {
        this.loadError = err.message || 'Failed to load role warehouse access'
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
        roleName: 'New Role',
        description: '',
        status: 'ACTIVE',
        warehouseAccess: []
      }

      this.roleForm = {
        roleName: 'New Role',
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
        this.saveError = 'Creating new roles is not supported yet.'
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
        alert('Permissions saved successfully')
      } catch (err) {
        this.saveError = err.message || 'Failed to save permissions'
      } finally {
        this.saving = false
      }
    },

    getStatusClass(status) {
      if (status === 'ACTIVE') return 'badge-green'
      if (status === 'INACTIVE') return 'badge-gray'
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
</style>