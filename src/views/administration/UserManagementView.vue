<template>
  <div class="space-y-6">
    <div class="flex flex-col xl:flex-row xl:items-center xl:justify-between gap-4">
      <div>
        <h1 class="text-2xl font-bold text-slate-800">User Management</h1>
        <p class="text-slate-500">
          Manage system users, roles and warehouse access
        </p>
      </div>

      <button
        @click="openUser(null)"
        class="bg-orange-600 hover:bg-orange-700 text-white px-5 py-3 rounded-xl font-semibold"
      >
        + New User
      </button>
    </div>

    <div class="bg-white rounded-2xl border border-slate-200 p-5">
      <div class="grid grid-cols-1 md:grid-cols-4 gap-4">
        <input v-model="filters.keyword" type="text" placeholder="Search user, email, role..." class="input" />

        <select v-model="filters.role" class="input">
          <option value="">All Roles</option>
          <option value="ADMIN">Admin</option>
          <option value="WAREHOUSE_MANAGER">Warehouse Manager</option>
          <option value="RECEIVING">Receiving</option>
          <option value="PICKING">Picking</option>
        </select>

        <select v-model="filters.status" class="input">
          <option value="">All Status</option>
          <option value="ACTIVE">Active</option>
          <option value="INACTIVE">Inactive</option>
          <option value="LOCKED">Locked</option>
        </select>

        <button @click="clearFilter" class="bg-slate-100 hover:bg-slate-200 text-slate-700 rounded-xl font-semibold">
          Clear Filter
        </button>
      </div>
    </div>

    <div class="bg-white rounded-2xl border border-slate-200 shadow-sm overflow-hidden">
      <div class="w-full overflow-x-auto">
        <table class="min-w-[1000px] w-full text-sm">
          <thead class="bg-slate-50 text-slate-500">
            <tr>
              <th class="table-th">User</th>
              <th class="table-th">Email</th>
              <th class="table-th">Role</th>
              <th class="table-th">Warehouse</th>
              <th class="table-th">Last Login</th>
              <th class="table-th">Status</th>
              <th class="table-th text-right">Action</th>
            </tr>
          </thead>

          <tbody>
            <tr v-for="user in filteredUsers" :key="user.id" class="border-t hover:bg-orange-50">
              <td class="table-td font-semibold text-slate-800">{{ user.fullName }}</td>
              <td class="table-td">{{ user.email }}</td>
              <td class="table-td">{{ user.roleText }}</td>
              <td class="table-td">{{ user.warehouse }}</td>
              <td class="table-td">{{ user.lastLogin }}</td>
              <td class="table-td">
                <span :class="getStatusClass(user.status)">
                  {{ user.statusText }}
                </span>
              </td>
              <td class="table-td text-right">
                <button @click="openUser(user)" class="text-orange-600 hover:text-orange-700 font-semibold">
                  Edit
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
              {{ selectedUser.id ? 'Edit User' : 'New User' }}
            </h2>
            <p class="text-slate-500">User profile and access control</p>
          </div>

          <button @click="selectedUser = null" class="text-slate-500 hover:text-red-600 text-xl">
            ✕
          </button>
        </div>

        <div class="space-y-4">
          <div>
            <label class="label">Full Name</label>
            <input v-model="userForm.fullName" type="text" class="input" placeholder="Enter full name" />
          </div>

          <div>
            <label class="label">Email</label>
            <input v-model="userForm.email" type="email" class="input" placeholder="Enter email" />
          </div>

          <div>
            <label class="label">Username</label>
            <input v-model="userForm.username" type="text" class="input" placeholder="Enter username" />
          </div>

          <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div>
              <label class="label">Role</label>
              <select v-model="userForm.role" class="input">
                <option value="">Select role</option>
                <option value="ADMIN">Admin</option>
                <option value="WAREHOUSE_MANAGER">Warehouse Manager</option>
                <option value="RECEIVING">Receiving</option>
                <option value="QC">Quality Check</option>
                <option value="PUTAWAY">Putaway</option>
                <option value="PICKING">Picking</option>
                <option value="SHIPPING">Shipping</option>
              </select>
            </div>

            <div>
              <label class="label">Warehouse</label>
              <select v-model="userForm.warehouse" class="input">
                <option value="">Select warehouse</option>
                <option value="WH-01">WH-01</option>
                <option value="WH-02">WH-02</option>
                <option value="ALL">All Warehouses</option>
              </select>
            </div>
          </div>

          <div>
            <label class="label">Status</label>
            <select v-model="userForm.status" class="input">
              <option value="ACTIVE">Active</option>
              <option value="INACTIVE">Inactive</option>
              <option value="LOCKED">Locked</option>
            </select>
          </div>

          <button
            @click="saveUser"
            class="w-full bg-orange-600 hover:bg-orange-700 text-white py-3 rounded-xl font-semibold mt-4"
          >
            Save User
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script>
export default {
  name: 'UserManagementView',

  data() {
    return {
      selectedUser: null,
      filters: {
        keyword: '',
        role: '',
        status: ''
      },
      userForm: {
        fullName: '',
        email: '',
        username: '',
        role: '',
        warehouse: '',
        status: 'ACTIVE'
      },
      users: [
        {
          id: 1,
          fullName: 'Admin User',
          email: 'admin@smartwms.com',
          username: 'admin',
          role: 'ADMIN',
          roleText: 'Admin',
          warehouse: 'ALL',
          status: 'ACTIVE',
          statusText: 'Active',
          lastLogin: '2026-06-08 09:30'
        },
        {
          id: 2,
          fullName: 'Receiving Staff',
          email: 'receiving@smartwms.com',
          username: 'receiving01',
          role: 'RECEIVING',
          roleText: 'Receiving',
          warehouse: 'WH-01',
          status: 'ACTIVE',
          statusText: 'Active',
          lastLogin: '2026-06-08 08:15'
        }
      ]
    }
  },

  computed: {
    filteredUsers() {
      return this.users.filter((user) => {
        const keyword = this.filters.keyword.toLowerCase()

        const matchKeyword =
          user.fullName.toLowerCase().includes(keyword) ||
          user.email.toLowerCase().includes(keyword) ||
          user.username.toLowerCase().includes(keyword) ||
          user.roleText.toLowerCase().includes(keyword)

        const matchRole = !this.filters.role || user.role === this.filters.role
        const matchStatus = !this.filters.status || user.status === this.filters.status

        return matchKeyword && matchRole && matchStatus
      })
    }
  },

  methods: {
    openUser(user) {
      this.selectedUser = user || {}

      this.userForm = user
        ? { ...user }
        : {
            fullName: '',
            email: '',
            username: '',
            role: '',
            warehouse: '',
            status: 'ACTIVE'
          }
    },

    clearFilter() {
      this.filters.keyword = ''
      this.filters.role = ''
      this.filters.status = ''
    },

    saveUser() {
      alert('User saved successfully')
      this.selectedUser = null
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