<template>
  <div class="space-y-6">
    <div class="flex flex-col xl:flex-row xl:items-center xl:justify-between gap-4">
      <div>
        <h1 class="text-2xl font-bold text-slate-800">Picking</h1>
        <p class="text-slate-500">
          Manage outbound picking tasks and confirm picked items
        </p>
      </div>

      <button
        @click="openPicking(null)"
        class="bg-orange-600 hover:bg-orange-700 text-white px-5 py-3 rounded-xl font-semibold"
      >
        + New Picking Task
      </button>
    </div>

    <div class="grid grid-cols-1 md:grid-cols-4 gap-5">
      <div class="summary-card">
        <p class="summary-label">Pending Pick</p>
        <h2 class="summary-value">16</h2>
      </div>

      <div class="summary-card">
        <p class="summary-label">In Progress</p>
        <h2 class="summary-value">7</h2>
      </div>

      <div class="summary-card">
        <p class="summary-label">Completed</p>
        <h2 class="summary-value">28</h2>
      </div>

      <div class="summary-card">
        <p class="summary-label">Short Pick</p>
        <h2 class="summary-value text-red-600">3</h2>
      </div>
    </div>

    <div class="bg-white rounded-2xl border border-slate-200 p-5">
      <div class="grid grid-cols-1 md:grid-cols-5 gap-4">
        <input
          v-model="filters.keyword"
          type="text"
          placeholder="Search Pick No, Order No, SKU..."
          class="input"
        />

        <select v-model="filters.status" class="input">
          <option value="">All Status</option>
          <option value="PENDING">Pending</option>
          <option value="IN_PROGRESS">In Progress</option>
          <option value="COMPLETED">Completed</option>
          <option value="SHORT_PICK">Short Pick</option>
        </select>

        <select v-model="filters.priority" class="input">
          <option value="">All Priority</option>
          <option value="HIGH">High</option>
          <option value="NORMAL">Normal</option>
          <option value="LOW">Low</option>
        </select>

        <input
          v-model="filters.date"
          type="date"
          class="input"
        />

        <button
          @click="clearFilter"
          class="bg-slate-100 hover:bg-slate-200 text-slate-700 rounded-xl font-semibold"
        >
          Clear Filter
        </button>
      </div>
    </div>

    <div class="bg-white rounded-2xl border border-slate-200 shadow-sm overflow-hidden">
      <div class="w-full overflow-x-auto">
        <table class="min-w-[1200px] w-full text-sm">
          <thead class="bg-slate-50 text-slate-500">
            <tr>
              <th class="table-th">Pick No.</th>
              <th class="table-th">Order No.</th>
              <th class="table-th">Customer</th>
              <th class="table-th">Warehouse</th>
              <th class="table-th">Priority</th>
              <th class="table-th">Items</th>
              <th class="table-th">Progress</th>
              <th class="table-th">Due Date</th>
              <th class="table-th">Status</th>
              <th class="table-th text-right">Action</th>
            </tr>
          </thead>

          <tbody>
            <tr
              v-for="item in filteredPickingList"
              :key="item.id"
              class="border-t hover:bg-orange-50"
            >
              <td class="table-td font-semibold text-slate-800">
                {{ item.pickNo }}
              </td>
              <td class="table-td">{{ item.orderNo }}</td>
              <td class="table-td">{{ item.customer }}</td>
              <td class="table-td">{{ item.warehouse }}</td>

              <td class="table-td">
                <span :class="getPriorityClass(item.priority)">
                  {{ item.priority }}
                </span>
              </td>

              <td class="table-td">{{ item.items }}</td>

              <td class="table-td">
                <div class="w-32">
                  <div class="h-2 bg-slate-200 rounded-full overflow-hidden">
                    <div
                      class="h-2 bg-orange-600 rounded-full"
                      :style="{ width: item.progress + '%' }"
                    ></div>
                  </div>
                  <p class="text-xs text-slate-500 mt-1">
                    {{ item.progress }}%
                  </p>
                </div>
              </td>

              <td class="table-td">{{ item.dueDate }}</td>

              <td class="table-td">
                <span :class="getStatusClass(item.status)">
                  {{ item.statusText }}
                </span>
              </td>

              <td class="table-td text-right">
                <button
                  @click="openPicking(item)"
                  class="text-orange-600 hover:text-orange-700 font-semibold"
                >
                  Start Pick
                </button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <div
      v-if="selectedPicking"
      class="fixed inset-0 bg-black/40 flex justify-end z-50"
    >
      <div class="w-full max-w-3xl bg-white h-full p-6 overflow-y-auto">
        <div class="flex items-center justify-between mb-6">
          <div>
            <h2 class="text-xl font-bold text-slate-800">
              Picking Task
            </h2>
            <p class="text-slate-500">
              {{ selectedPicking.pickNo }} / {{ selectedPicking.orderNo }}
            </p>
          </div>

          <button
            @click="selectedPicking = null"
            class="text-slate-500 hover:text-red-600 text-xl"
          >
            ✕
          </button>
        </div>

        <div class="bg-slate-50 rounded-2xl p-5 mb-6">
          <div class="grid grid-cols-2 md:grid-cols-4 gap-4">
            <div>
              <p class="detail-label">Customer</p>
              <p class="detail-value">{{ selectedPicking.customer }}</p>
            </div>

            <div>
              <p class="detail-label">Warehouse</p>
              <p class="detail-value">{{ selectedPicking.warehouse }}</p>
            </div>

            <div>
              <p class="detail-label">Priority</p>
              <p class="detail-value text-orange-600">
                {{ selectedPicking.priority }}
              </p>
            </div>

            <div>
              <p class="detail-label">Due Date</p>
              <p class="detail-value">{{ selectedPicking.dueDate }}</p>
            </div>
          </div>
        </div>

        <div class="bg-white border border-slate-200 rounded-2xl p-5 mb-6">
          <label class="label">Scan SKU / Barcode / Location</label>
          <input
            v-model="scanText"
            type="text"
            class="input"
            placeholder="Scan barcode, SKU, or bin location"
          />
        </div>

        <div class="space-y-4">
          <h3 class="font-bold text-slate-800">
            Picking Items
          </h3>

          <div
            v-for="product in pickingProducts"
            :key="product.id"
            class="border border-slate-200 rounded-2xl p-4"
          >
            <div class="flex flex-col md:flex-row md:items-start md:justify-between gap-4">
              <div>
                <p class="font-bold text-slate-800">
                  {{ product.sku }}
                </p>
                <p class="text-sm text-slate-500">
                  {{ product.productName }}
                </p>
                <p class="text-sm text-orange-600 font-semibold mt-2">
                  Location: {{ product.location }}
                </p>
              </div>

              <div class="grid grid-cols-3 gap-3 min-w-[280px]">
                <div>
                  <p class="detail-label">Order</p>
                  <p class="detail-value">{{ product.orderQty }}</p>
                </div>

                <div>
                  <p class="detail-label">Picked</p>
                  <p class="detail-value text-green-600">
                    {{ product.pickedQty }}
                  </p>
                </div>

                <div>
                  <p class="detail-label">Remain</p>
                  <p class="detail-value text-red-600">
                    {{ product.orderQty - product.pickedQty }}
                  </p>
                </div>
              </div>
            </div>

            <div class="grid grid-cols-1 md:grid-cols-3 gap-3 mt-4">
              <input
                v-model.number="product.inputQty"
                type="number"
                class="input"
                placeholder="Pick Qty"
              />

              <button
                @click="confirmPick(product)"
                class="bg-orange-600 hover:bg-orange-700 text-white rounded-xl font-semibold"
              >
                Confirm Pick
              </button>

              <button
                @click="shortPick(product)"
                class="bg-red-600 hover:bg-red-700 text-white rounded-xl font-semibold"
              >
                Short Pick
              </button>
            </div>
          </div>
        </div>

        <button
          @click="completePicking"
          class="w-full bg-green-600 hover:bg-green-700 text-white py-3 rounded-xl font-semibold mt-6"
        >
          Complete Picking
        </button>
      </div>
    </div>
  </div>
</template>

<script>
export default {
  name: 'PickingView',

  data() {
    return {
      selectedPicking: null,
      scanText: '',

      filters: {
        keyword: '',
        status: '',
        priority: '',
        date: ''
      },

      pickingList: [
        {
          id: 1,
          pickNo: 'PICK-20260608-001',
          orderNo: 'SO-20260608-001',
          customer: 'ABC Customer',
          warehouse: 'WH-01',
          priority: 'HIGH',
          items: 5,
          progress: 0,
          dueDate: '2026-06-08',
          status: 'PENDING',
          statusText: 'Pending'
        },
        {
          id: 2,
          pickNo: 'PICK-20260608-002',
          orderNo: 'SO-20260608-002',
          customer: 'Global Retail',
          warehouse: 'WH-01',
          priority: 'NORMAL',
          items: 8,
          progress: 45,
          dueDate: '2026-06-08',
          status: 'IN_PROGRESS',
          statusText: 'In Progress'
        },
        {
          id: 3,
          pickNo: 'PICK-20260608-003',
          orderNo: 'SO-20260608-003',
          customer: 'Fast Shop',
          warehouse: 'WH-02',
          priority: 'LOW',
          items: 3,
          progress: 100,
          dueDate: '2026-06-09',
          status: 'COMPLETED',
          statusText: 'Completed'
        }
      ],

      pickingProducts: [
        {
          id: 1,
          sku: 'SKU-0001',
          productName: 'Barcode Label Roll',
          location: 'A-01-01',
          orderQty: 20,
          pickedQty: 0,
          inputQty: 0
        },
        {
          id: 2,
          sku: 'SKU-0002',
          productName: 'Packing Box Size M',
          location: 'B-02-03',
          orderQty: 50,
          pickedQty: 0,
          inputQty: 0
        },
        {
          id: 3,
          sku: 'SKU-0003',
          productName: 'RFID Tag',
          location: 'A-03-02',
          orderQty: 10,
          pickedQty: 0,
          inputQty: 0
        }
      ]
    }
  },

  computed: {
    filteredPickingList() {
      return this.pickingList.filter((item) => {
        const keyword = this.filters.keyword.toLowerCase()

        const matchKeyword =
          item.pickNo.toLowerCase().includes(keyword) ||
          item.orderNo.toLowerCase().includes(keyword) ||
          item.customer.toLowerCase().includes(keyword)

        const matchStatus =
          !this.filters.status || item.status === this.filters.status

        const matchPriority =
          !this.filters.priority || item.priority === this.filters.priority

        const matchDate =
          !this.filters.date || item.dueDate === this.filters.date

        return matchKeyword && matchStatus && matchPriority && matchDate
      })
    }
  },

  methods: {
    openPicking(item) {
      this.selectedPicking = item || {
        pickNo: 'PICK-NEW',
        orderNo: 'SO-NEW',
        customer: 'New Customer',
        warehouse: 'WH-01',
        priority: 'NORMAL',
        dueDate: '2026-06-08'
      }

      this.scanText = ''
    },

    clearFilter() {
      this.filters.keyword = ''
      this.filters.status = ''
      this.filters.priority = ''
      this.filters.date = ''
    },

    confirmPick(product) {
      if (!product.inputQty || product.inputQty <= 0) {
        alert('Pick Qty must be greater than zero')
        return
      }

      const remainQty = product.orderQty - product.pickedQty

      if (product.inputQty > remainQty) {
        alert('Pick Qty cannot exceed remaining quantity')
        return
      }

      product.pickedQty += product.inputQty
      product.inputQty = 0
    },

    shortPick(product) {
      alert(`Short pick reported for ${product.sku}`)
    },

    completePicking() {
      alert('Picking task completed successfully')
      this.selectedPicking = null
    },

    getStatusClass(status) {
      if (status === 'PENDING') return 'badge-gray'
      if (status === 'IN_PROGRESS') return 'badge-orange'
      if (status === 'COMPLETED') return 'badge-green'
      if (status === 'SHORT_PICK') return 'badge-red'
      return 'badge-gray'
    },

    getPriorityClass(priority) {
      if (priority === 'HIGH') return 'badge-red'
      if (priority === 'NORMAL') return 'badge-blue'
      if (priority === 'LOW') return 'badge-gray'
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

.detail-label {
  color: #64748b;
  font-size: 13px;
}

.detail-value {
  color: #0f172a;
  font-weight: 700;
  margin-top: 4px;
}

.badge-gray {
  background: #f1f5f9;
  color: #475569;
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

.badge-blue {
  background: #dbeafe;
  color: #2563eb;
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