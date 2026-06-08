<template>
  <div class="space-y-6">
    <!-- Page Header -->
    <div class="flex flex-col xl:flex-row xl:items-center xl:justify-between gap-4">
      <div>
        <h1 class="text-2xl font-bold text-slate-800">Warehouse Setup</h1>
        <p class="text-slate-500">
          Manage warehouse master data, zones and storage locations
        </p>
      </div>

      <button
        @click="openWarehouse(null)"
        class="bg-orange-600 hover:bg-orange-700 text-white px-5 py-3 rounded-xl font-semibold"
      >
        + New Warehouse
      </button>
    </div>

    <!-- Summary Cards -->
    <div class="grid grid-cols-1 md:grid-cols-4 gap-5">
      <div class="summary-card">
        <p class="summary-label">Total Warehouses</p>
        <h2 class="summary-value">3</h2>
      </div>

      <div class="summary-card">
        <p class="summary-label">Total Zones</p>
        <h2 class="summary-value">18</h2>
      </div>

      <div class="summary-card">
        <p class="summary-label">Total Locations</p>
        <h2 class="summary-value">10,500</h2>
      </div>

      <div class="summary-card">
        <p class="summary-label">Avg Utilization</p>
        <h2 class="summary-value text-orange-600">78%</h2>
      </div>
    </div>

    <!-- Filter -->
    <div class="bg-white rounded-2xl border border-slate-200 p-5">
      <div class="grid grid-cols-1 md:grid-cols-4 gap-4">
        <input
          v-model="filters.keyword"
          type="text"
          placeholder="Search warehouse..."
          class="input"
        />

        <select v-model="filters.type" class="input">
          <option value="">All Types</option>
          <option value="Distribution Center">Distribution Center</option>
          <option value="Factory Warehouse">Factory Warehouse</option>
          <option value="Transit Warehouse">Transit Warehouse</option>
        </select>

        <select v-model="filters.status" class="input">
          <option value="">All Status</option>
          <option value="ACTIVE">Active</option>
          <option value="INACTIVE">Inactive</option>
        </select>

        <button
          @click="clearFilter"
          class="bg-slate-100 hover:bg-slate-200 text-slate-700 rounded-xl font-semibold"
        >
          Clear Filter
        </button>
      </div>
    </div>

    <!-- Warehouse Table -->
    <div class="bg-white rounded-2xl border border-slate-200 shadow-sm overflow-hidden">
      <div class="w-full overflow-x-auto">
        <table class="min-w-[1150px] w-full text-sm">
          <thead class="bg-slate-50 text-slate-500">
            <tr>
              <th class="table-th">Warehouse Code</th>
              <th class="table-th">Warehouse Name</th>
              <th class="table-th">Type</th>
              <th class="table-th">Address</th>
              <th class="table-th">Zones</th>
              <th class="table-th">Locations</th>
              <th class="table-th">Utilization</th>
              <th class="table-th">Status</th>
              <th class="table-th text-right">Action</th>
            </tr>
          </thead>

          <tbody>
            <tr
              v-for="warehouse in filteredWarehouses"
              :key="warehouse.id"
              class="border-t hover:bg-orange-50"
            >
              <td class="table-td font-semibold text-slate-800">
                {{ warehouse.code }}
              </td>

              <td class="table-td">
                {{ warehouse.name }}
              </td>

              <td class="table-td">
                {{ warehouse.type }}
              </td>

              <td class="table-td">
                {{ warehouse.address }}
              </td>

              <td class="table-td">
                {{ warehouse.zones }}
              </td>

              <td class="table-td">
                {{ warehouse.locations }}
              </td>

              <td class="table-td">
                <div class="w-32">
                  <div class="h-2 bg-slate-200 rounded-full overflow-hidden">
                    <div
                      class="h-2 bg-orange-600 rounded-full"
                      :style="{ width: warehouse.utilization + '%' }"
                    ></div>
                  </div>
                  <p class="text-xs text-slate-500 mt-1">
                    {{ warehouse.utilization }}%
                  </p>
                </div>
              </td>

              <td class="table-td">
                <span :class="getStatusClass(warehouse.status)">
                  {{ warehouse.status }}
                </span>
              </td>

              <td class="table-td text-right">
                <button
                  @click="openWarehouse(warehouse)"
                  class="text-orange-600 hover:text-orange-700 font-semibold"
                >
                  Manage
                </button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- Warehouse Drawer -->
    <div
      v-if="selectedWarehouse"
      class="fixed inset-0 bg-black/40 flex justify-end z-50"
    >
      <div class="w-full max-w-4xl bg-white h-full p-6 overflow-y-auto">
        <div class="flex items-center justify-between mb-6">
          <div>
            <h2 class="text-xl font-bold text-slate-800">
              {{ selectedWarehouse.id ? 'Warehouse Detail' : 'New Warehouse' }}
            </h2>
            <p class="text-slate-500">
              Warehouse master data and location structure
            </p>
          </div>

          <button
            @click="selectedWarehouse = null"
            class="text-slate-500 hover:text-red-600 text-xl"
          >
            ✕
          </button>
        </div>

        <!-- Tabs -->
        <div class="flex gap-2 mb-6 overflow-x-auto">
          <button
            v-for="tab in tabs"
            :key="tab.key"
            @click="activeTab = tab.key"
            class="tab-button"
            :class="activeTab === tab.key ? 'tab-active' : ''"
          >
            {{ tab.label }}
          </button>
        </div>

        <!-- Warehouse Info -->
        <div v-if="activeTab === 'INFO'" class="space-y-4">
          <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div>
              <label class="label">Warehouse Code</label>
              <input v-model="warehouseForm.code" type="text" class="input" />
            </div>

            <div>
              <label class="label">Warehouse Name</label>
              <input v-model="warehouseForm.name" type="text" class="input" />
            </div>

            <div>
              <label class="label">Warehouse Type</label>
              <select v-model="warehouseForm.type" class="input">
                <option value="Distribution Center">Distribution Center</option>
                <option value="Factory Warehouse">Factory Warehouse</option>
                <option value="Transit Warehouse">Transit Warehouse</option>
              </select>
            </div>

            <div>
              <label class="label">Status</label>
              <select v-model="warehouseForm.status" class="input">
                <option value="ACTIVE">Active</option>
                <option value="INACTIVE">Inactive</option>
              </select>
            </div>

            <div>
              <label class="label">Contact Person</label>
              <input v-model="warehouseForm.contactPerson" type="text" class="input" />
            </div>

            <div>
              <label class="label">Phone</label>
              <input v-model="warehouseForm.phone" type="text" class="input" />
            </div>
          </div>

          <div>
            <label class="label">Address</label>
            <textarea v-model="warehouseForm.address" rows="4" class="input"></textarea>
          </div>

          <button
            @click="saveWarehouse"
            class="w-full bg-orange-600 hover:bg-orange-700 text-white py-3 rounded-xl font-semibold"
          >
            Save Warehouse
          </button>
        </div>

        <!-- Zone Setup -->
        <div v-if="activeTab === 'ZONE'" class="space-y-4">
          <div class="flex justify-between items-center">
            <h3 class="font-bold text-slate-800">Zone Setup</h3>

            <button
              @click="addZone"
              class="bg-orange-600 hover:bg-orange-700 text-white px-4 py-2 rounded-xl font-semibold"
            >
              + Add Zone
            </button>
          </div>

          <div
            v-for="zone in zones"
            :key="zone.id"
            class="border border-slate-200 rounded-2xl p-4"
          >
            <div class="grid grid-cols-1 md:grid-cols-4 gap-4">
              <input v-model="zone.code" class="input" placeholder="Zone Code" />
              <input v-model="zone.name" class="input" placeholder="Zone Name" />

              <select v-model="zone.type" class="input">
                <option value="RECEIVING">Receiving</option>
                <option value="STORAGE">Storage</option>
                <option value="PICKING">Picking</option>
                <option value="PACKING">Packing</option>
                <option value="SHIPPING">Shipping</option>
                <option value="QC">QC</option>
                <option value="HOLD">Hold</option>
              </select>

              <select v-model="zone.status" class="input">
                <option value="ACTIVE">Active</option>
                <option value="INACTIVE">Inactive</option>
              </select>
            </div>
          </div>
        </div>

        <!-- Location Structure -->
        <div v-if="activeTab === 'LOCATION'" class="space-y-4">
          <div class="bg-slate-50 border border-slate-200 rounded-2xl p-5">
            <h3 class="font-bold text-slate-800 mb-4">Location Structure</h3>

            <div class="space-y-3">
              <div
                v-for="zone in locationTree"
                :key="zone.zoneCode"
                class="tree-zone"
              >
                <p class="font-bold text-orange-600">
                  {{ zone.zoneCode }} - {{ zone.zoneName }}
                </p>

                <div
                  v-for="aisle in zone.aisles"
                  :key="aisle.aisleCode"
                  class="ml-5 mt-3"
                >
                  <p class="font-semibold text-slate-700">
                    └─ Aisle {{ aisle.aisleCode }}
                  </p>

                  <div
                    v-for="rack in aisle.racks"
                    :key="rack.rackCode"
                    class="ml-5 mt-2"
                  >
                    <p class="text-slate-600">
                      └─ Rack {{ rack.rackCode }}
                    </p>

                    <div class="ml-5 mt-2 grid grid-cols-2 md:grid-cols-4 gap-2">
                      <button
                        v-for="bin in rack.bins"
                        :key="bin"
                        class="bin-button"
                      >
                        {{ bin }}
                      </button>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </div>

          <button
            @click="generateLocations"
            class="bg-orange-600 hover:bg-orange-700 text-white px-5 py-3 rounded-xl font-semibold"
          >
            Generate Bin Locations
          </button>
        </div>

        <!-- Capacity Dashboard -->
        <div v-if="activeTab === 'CAPACITY'" class="space-y-5">
          <div class="bg-orange-50 border border-orange-200 rounded-2xl p-6">
            <p class="text-slate-500">Warehouse Utilization</p>
            <h2 class="text-4xl font-bold text-orange-600 mt-2">
              {{ warehouseForm.utilization }}%
            </h2>

            <div class="h-3 bg-orange-200 rounded-full mt-5 overflow-hidden">
              <div
                class="h-3 bg-orange-600 rounded-full"
                :style="{ width: warehouseForm.utilization + '%' }"
              ></div>
            </div>
          </div>

          <div class="grid grid-cols-1 md:grid-cols-3 gap-4">
            <div class="summary-card">
              <p class="summary-label">Total Capacity</p>
              <h2 class="summary-value">6,000</h2>
            </div>

            <div class="summary-card">
              <p class="summary-label">Occupied</p>
              <h2 class="summary-value">4,680</h2>
            </div>

            <div class="summary-card">
              <p class="summary-label">Available</p>
              <h2 class="summary-value">1,320</h2>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script>
export default {
  name: 'WarehouseView',

  data() {
    return {
      selectedWarehouse: null,
      activeTab: 'INFO',

      filters: {
        keyword: '',
        type: '',
        status: ''
      },

      tabs: [
        { key: 'INFO', label: 'Warehouse Info' },
        { key: 'ZONE', label: 'Zone Setup' },
        { key: 'LOCATION', label: 'Location Structure' },
        { key: 'CAPACITY', label: 'Capacity Dashboard' }
      ],

      warehouseForm: {
        code: '',
        name: '',
        type: 'Distribution Center',
        address: '',
        contactPerson: '',
        phone: '',
        status: 'ACTIVE',
        utilization: 0
      },

      warehouses: [
        {
          id: 1,
          code: 'WH-01',
          name: 'Bangkok Warehouse',
          type: 'Distribution Center',
          address: 'Bangkok',
          zones: 8,
          locations: 4500,
          utilization: 78,
          status: 'ACTIVE'
        },
        {
          id: 2,
          code: 'WH-02',
          name: 'Rayong Warehouse',
          type: 'Factory Warehouse',
          address: 'Rayong',
          zones: 6,
          locations: 3800,
          utilization: 65,
          status: 'ACTIVE'
        },
        {
          id: 3,
          code: 'WH-03',
          name: 'Chonburi Warehouse',
          type: 'Transit Warehouse',
          address: 'Chonburi',
          zones: 4,
          locations: 2200,
          utilization: 72,
          status: 'ACTIVE'
        }
      ],

      zones: [
        { id: 1, code: 'A', name: 'Storage Zone A', type: 'STORAGE', status: 'ACTIVE' },
        { id: 2, code: 'B', name: 'Picking Zone B', type: 'PICKING', status: 'ACTIVE' },
        { id: 3, code: 'C', name: 'Shipping Zone C', type: 'SHIPPING', status: 'ACTIVE' }
      ],

      locationTree: [
        {
          zoneCode: 'A',
          zoneName: 'Storage Zone A',
          aisles: [
            {
              aisleCode: '01',
              racks: [
                {
                  rackCode: '01',
                  bins: ['A-01-01-01', 'A-01-01-02', 'A-01-01-03']
                },
                {
                  rackCode: '02',
                  bins: ['A-01-02-01', 'A-01-02-02', 'A-01-02-03']
                }
              ]
            },
            {
              aisleCode: '02',
              racks: [
                {
                  rackCode: '01',
                  bins: ['A-02-01-01', 'A-02-01-02', 'A-02-01-03']
                }
              ]
            }
          ]
        }
      ]
    }
  },

  computed: {
    filteredWarehouses() {
      return this.warehouses.filter((warehouse) => {
        const keyword = this.filters.keyword.toLowerCase()

        const matchKeyword =
          warehouse.code.toLowerCase().includes(keyword) ||
          warehouse.name.toLowerCase().includes(keyword) ||
          warehouse.address.toLowerCase().includes(keyword)

        const matchType =
          !this.filters.type || warehouse.type === this.filters.type

        const matchStatus =
          !this.filters.status || warehouse.status === this.filters.status

        return matchKeyword && matchType && matchStatus
      })
    }
  },

  methods: {
    openWarehouse(warehouse) {
      this.selectedWarehouse = warehouse || {
        id: null
      }

      this.activeTab = 'INFO'

      this.warehouseForm = warehouse
        ? {
            code: warehouse.code,
            name: warehouse.name,
            type: warehouse.type,
            address: warehouse.address,
            contactPerson: 'Warehouse Manager',
            phone: '02-000-0000',
            status: warehouse.status,
            utilization: warehouse.utilization
          }
        : {
            code: '',
            name: '',
            type: 'Distribution Center',
            address: '',
            contactPerson: '',
            phone: '',
            status: 'ACTIVE',
            utilization: 0
          }
    },

    clearFilter() {
      this.filters.keyword = ''
      this.filters.type = ''
      this.filters.status = ''
    },

    addZone() {
      this.zones.push({
        id: Date.now(),
        code: '',
        name: '',
        type: 'STORAGE',
        status: 'ACTIVE'
      })
    },

    saveWarehouse() {
      alert('Warehouse saved successfully')
      this.selectedWarehouse = null
    },

    generateLocations() {
      alert('Bin locations generated successfully')
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

.summary-card {
  background: white;
  border: 1px solid #e2e8f0;
  border-radius: 18px;
  padding: 20px;
}

.summary-label {
  color: #64748b;
  font-size: 14px;
}

.summary-value {
  font-size: 30px;
  font-weight: 800;
  color: #0f172a;
  margin-top: 8px;
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

.tree-zone {
  background: white;
  border: 1px solid #e2e8f0;
  border-radius: 16px;
  padding: 16px;
}

.bin-button {
  background: #fff7ed;
  color: #ea580c;
  border: 1px solid #fed7aa;
  padding: 8px 10px;
  border-radius: 10px;
  font-weight: 600;
  font-size: 12px;
}

.bin-button:hover {
  background: #ea580c;
  color: white;
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