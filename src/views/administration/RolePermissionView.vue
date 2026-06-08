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
              class="bg-orange-600 hover:bg-orange-700 text-white px-5 py-3 rounded-xl font-semibold"
            >
              Save Permissions
            </button>
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

            <div class="grid grid-cols-1 md:grid-cols-3 gap-3">
              <label
                v-for="warehouse in warehouses"
                :key="warehouse.code"
                class="checkbox-card"
              >
                <input
                  type="checkbox"
                  :value="warehouse.code"
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
export default {
  name: 'RolePermissionView',

  data() {
    return {
      roleKeyword: '',
      selectedRole: null,
      allChecked: false,

      warehouses: [
        { code: 'WH-01', name: 'Bangkok Warehouse' },
        { code: 'WH-02', name: 'Rayong Warehouse' },
        { code: 'WH-03', name: 'Chonburi Warehouse' }
      ],

      actions: ['View', 'Create', 'Edit', 'Delete', 'Approve', 'Export'],

      permissionModules: [
        {
          code: 'DASHBOARD',
          name: 'Dashboard',
          actions: ['View', 'Export']
        },
        {
          code: 'RECEIVING',
          name: 'Receiving',
          actions: ['View', 'Create', 'Edit', 'Delete', 'Approve']
        },
        {
          code: 'QUALITY_CHECK',
          name: 'Quality Check',
          actions: ['View', 'Create', 'Edit', 'Approve']
        },
        {
          code: 'PUTAWAY',
          name: 'Putaway',
          actions: ['View', 'Create', 'Edit', 'Approve']
        },
        {
          code: 'STOCK_INQUIRY',
          name: 'Stock Inquiry',
          actions: ['View', 'Export']
        },
        {
          code: 'STOCK_ADJUSTMENT',
          name: 'Stock Adjustment',
          actions: ['View', 'Create', 'Edit', 'Delete', 'Approve']
        },
        {
          code: 'PICKING',
          name: 'Picking',
          actions: ['View', 'Create', 'Edit', 'Approve']
        },
        {
          code: 'SHIPPING',
          name: 'Shipping',
          actions: ['View', 'Create', 'Edit', 'Approve']
        },
        {
          code: 'USER_MANAGEMENT',
          name: 'User Management',
          actions: ['View', 'Create', 'Edit', 'Delete']
        },
        {
          code: 'ROLE_PERMISSION',
          name: 'Roles & Permissions',
          actions: ['View', 'Create', 'Edit', 'Delete', 'Approve']
        }
      ],

      roles: [
        {
          id: 1,
          roleCode: 'ADMIN',
          roleName: 'System Administrator',
          description: 'Full access to all modules and system configuration',
          status: 'ACTIVE',
          warehouseAccess: ['WH-01', 'WH-02', 'WH-03']
        },
        {
          id: 2,
          roleCode: 'WAREHOUSE_MANAGER',
          roleName: 'Warehouse Manager',
          description: 'Manage warehouse operations and approval tasks',
          status: 'ACTIVE',
          warehouseAccess: ['WH-01', 'WH-02']
        },
        {
          id: 3,
          roleCode: 'RECEIVING',
          roleName: 'Receiving Staff',
          description: 'Receive goods and update inbound transactions',
          status: 'ACTIVE',
          warehouseAccess: ['WH-01']
        },
        {
          id: 4,
          roleCode: 'PICKING',
          roleName: 'Picking Staff',
          description: 'Execute outbound picking tasks',
          status: 'ACTIVE',
          warehouseAccess: ['WH-01']
        }
      ],

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

  mounted() {
    this.selectRole(this.roles[0])
  },

  methods: {
    selectRole(role) {
      this.selectedRole = role

      this.roleForm = {
        roleName: role.roleName,
        roleCode: role.roleCode,
        description: role.description,
        status: role.status,
        warehouseAccess: [...role.warehouseAccess],
        permissions: this.createDefaultPermissions(role.roleCode)
      }

      this.updateAllCheckedState()
    },

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

    createDefaultPermissions(roleCode) {
      const permissions = this.createEmptyPermissions()

      if (roleCode === 'ADMIN') {
        this.permissionModules.forEach((module) => {
          module.actions.forEach((action) => {
            permissions[module.code][action] = true
          })
        })
      }

      if (roleCode === 'WAREHOUSE_MANAGER') {
        this.permissionModules.forEach((module) => {
          module.actions.forEach((action) => {
            if (['View', 'Create', 'Edit', 'Approve', 'Export'].includes(action)) {
              permissions[module.code][action] = true
            }
          })
        })

        permissions.USER_MANAGEMENT.Delete = false
        permissions.ROLE_PERMISSION.Delete = false
      }

      if (roleCode === 'RECEIVING') {
        permissions.DASHBOARD.View = true
        permissions.RECEIVING.View = true
        permissions.RECEIVING.Create = true
        permissions.RECEIVING.Edit = true
        permissions.STOCK_INQUIRY.View = true
      }

      if (roleCode === 'PICKING') {
        permissions.DASHBOARD.View = true
        permissions.PICKING.View = true
        permissions.PICKING.Edit = true
        permissions.STOCK_INQUIRY.View = true
      }

      return permissions
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

      this.allChecked = availablePermissions.every((item) => item === true)
    },

    savePermission() {
      alert('Permissions saved successfully')
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