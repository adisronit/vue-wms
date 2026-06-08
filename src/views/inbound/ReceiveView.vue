<template>
  <div>
    <!-- Page Header -->
    <div class="flex flex-col md:flex-row md:items-center md:justify-between gap-4 mb-6">
      <div>
        <h1 class="text-2xl font-bold text-slate-800">
          Goods Receiving
        </h1>
        <p class="text-slate-500">
          Manage inbound purchase orders and receiving tasks
        </p>
      </div>

      <button
        @click="$router.push('/inbound/receiving/new')"
        class="bg-orange-600 hover:bg-orange-700 text-white px-5 py-3 rounded-xl font-semibold"
      >
        + New Receiving
      </button>
    </div>

    <!-- Summary Cards -->
    <div class="grid grid-cols-1 md:grid-cols-4 gap-5 mb-6">
      <div class="summary-card">
        <p class="summary-label">Waiting Receive</p>
        <h2 class="summary-value">12</h2>
      </div>

      <div class="summary-card">
        <p class="summary-label">Receiving</p>
        <h2 class="summary-value">5</h2>
      </div>

      <div class="summary-card">
        <p class="summary-label">Partial Received</p>
        <h2 class="summary-value">3</h2>
      </div>

      <div class="summary-card">
        <p class="summary-label">Completed</p>
        <h2 class="summary-value">18</h2>
      </div>
    </div>

    <!-- Filter -->
    <div class="bg-white rounded-2xl border border-slate-200 p-5 mb-6">
      <div class="grid grid-cols-1 md:grid-cols-4 gap-4">
        <input
          v-model="filters.keyword"
          type="text"
          placeholder="Search PO, ASN, Vendor..."
          class="input"
        />

        <select v-model="filters.status" class="input">
          <option value="">All Status</option>
          <option value="WAITING">Waiting</option>
          <option value="RECEIVING">Receiving</option>
          <option value="PARTIAL">Partial Received</option>
          <option value="COMPLETED">Completed</option>
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

    <!-- Receiving Table -->
    <div class="bg-white rounded-2xl border border-slate-200 shadow-sm overflow-hidden">
      <div class="w-full overflow-x-auto">
        <table class="min-w-[1100px] w-full text-sm">
          <thead class="bg-slate-50 text-slate-500">
            <tr>
              <th class="table-th">Document No.</th>
              <th class="table-th">ASN No.</th>
              <th class="table-th">Vendor</th>
              <th class="table-th">Warehouse</th>
              <th class="table-th">Expected Date</th>
              <th class="table-th">Items</th>
              <th class="table-th">Progress</th>
              <th class="table-th">Status</th>
              <th class="table-th text-right">Action</th>
            </tr>
          </thead>

          <tbody>
            <tr
              v-for="item in filteredReceivingList"
              :key="item.id"
              class="border-t hover:bg-orange-50"
            >
              <td class="table-td font-semibold text-slate-800">
                {{ item.documentNo }}
              </td>

              <td class="table-td">
                {{ item.asnNo }}
              </td>

              <td class="table-td">
                {{ item.vendor }}
              </td>

              <td class="table-td">
                {{ item.warehouse }}
              </td>

              <td class="table-td">
                {{ item.expectedDate }}
              </td>

              <td class="table-td">
                {{ item.items }}
              </td>

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

              <td class="table-td">
                <span :class="getStatusClass(item.status)">
                  {{ item.statusText }}
                </span>
              </td>

              <td class="table-td text-right">
                <button
                  @click="openReceiving(item)"
                  class="text-orange-600 hover:text-orange-700 font-semibold"
                >
                  Start Receive
                </button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- Mock Detail Panel -->
    <div
      v-if="selectedReceiving"
      class="fixed inset-0 bg-black/40 flex justify-end z-50"
    >
      <div class="w-full max-w-xl bg-white h-full p-6 overflow-y-auto">
        <div class="flex items-center justify-between mb-6">
          <div>
            <h2 class="text-xl font-bold text-slate-800">
              Receiving Detail
            </h2>
            <p class="text-slate-500">
              {{ selectedReceiving.documentNo }}
            </p>
          </div>

          <button
            @click="selectedReceiving = null"
            class="text-slate-500 hover:text-red-600"
          >
            ✕
          </button>
        </div>

        <div class="space-y-4">
          <div class="detail-row">
            <span>Vendor</span>
            <strong>{{ selectedReceiving.vendor }}</strong>
          </div>

          <div class="detail-row">
            <span>Warehouse</span>
            <strong>{{ selectedReceiving.warehouse }}</strong>
          </div>

          <div class="detail-row">
            <span>Expected Date</span>
            <strong>{{ selectedReceiving.expectedDate }}</strong>
          </div>
        </div>

        <div class="mt-8">
          <h3 class="font-bold text-slate-800 mb-4">
            Receiving Items
          </h3>

          <div
            v-for="product in mockProducts"
            :key="product.sku"
            class="border border-slate-200 rounded-xl p-4 mb-3"
          >
            <div class="flex justify-between">
              <div>
                <p class="font-semibold text-slate-800">
                  {{ product.sku }}
                </p>
                <p class="text-sm text-slate-500">
                  {{ product.name }}
                </p>
              </div>

              <div class="text-right">
                <p class="text-sm text-slate-500">PO Qty</p>
                <p class="font-bold">{{ product.poQty }}</p>
              </div>
            </div>

            <div class="grid grid-cols-2 gap-3 mt-4">
              <input
                v-model="product.receiveQty"
                type="number"
                placeholder="Receive Qty"
                class="input"
              />

              <input
                v-model="product.location"
                type="text"
                placeholder="Staging Location"
                class="input"
              />
            </div>
          </div>

          <button class="w-full bg-orange-600 hover:bg-orange-700 text-white py-3 rounded-xl font-semibold mt-4">
            Confirm Receiving
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script>
export default {
  name: 'ReceivingPage',

  data() {
    return {
      selectedReceiving: null,

      filters: {
        keyword: '',
        status: '',
        date: ''
      },

      receivingList: [
        {
          id: 1,
          documentNo: 'PO-20260608-001',
          asnNo: 'ASN-0001',
          vendor: 'ABC Supplier',
          warehouse: 'WH-01',
          expectedDate: '2026-06-08',
          items: 12,
          progress: 0,
          status: 'WAITING',
          statusText: 'Waiting'
        },
        {
          id: 2,
          documentNo: 'PO-20260608-002',
          asnNo: 'ASN-0002',
          vendor: 'Global Parts',
          warehouse: 'WH-01',
          expectedDate: '2026-06-08',
          items: 8,
          progress: 45,
          status: 'RECEIVING',
          statusText: 'Receiving'
        },
        {
          id: 3,
          documentNo: 'PO-20260608-003',
          asnNo: 'ASN-0003',
          vendor: 'Fast Supply',
          warehouse: 'WH-02',
          expectedDate: '2026-06-09',
          items: 5,
          progress: 70,
          status: 'PARTIAL',
          statusText: 'Partial'
        },
        {
          id: 4,
          documentNo: 'PO-20260608-004',
          asnNo: 'ASN-0004',
          vendor: 'Orange Logistics',
          warehouse: 'WH-02',
          expectedDate: '2026-06-09',
          items: 20,
          progress: 100,
          status: 'COMPLETED',
          statusText: 'Completed'
        }
      ],

      mockProducts: [
        {
          sku: 'SKU-0001',
          name: 'Barcode Label Roll',
          poQty: 100,
          receiveQty: '',
          location: 'STAGING-01'
        },
        {
          sku: 'SKU-0002',
          name: 'Packing Box Size M',
          poQty: 250,
          receiveQty: '',
          location: 'STAGING-01'
        },
        {
          sku: 'SKU-0003',
          name: 'RFID Tag',
          poQty: 500,
          receiveQty: '',
          location: 'STAGING-01'
        }
      ]
    }
  },

  computed: {
    filteredReceivingList() {
      return this.receivingList.filter((item) => {
        const keyword = this.filters.keyword.toLowerCase()

        const matchKeyword =
          item.documentNo.toLowerCase().includes(keyword) ||
          item.asnNo.toLowerCase().includes(keyword) ||
          item.vendor.toLowerCase().includes(keyword)

        const matchStatus =
          !this.filters.status || item.status === this.filters.status

        const matchDate =
          !this.filters.date || item.expectedDate === this.filters.date

        return matchKeyword && matchStatus && matchDate
      })
    }
  },

  methods: {
    openReceiving(item) {
      this.selectedReceiving = item
    },

    clearFilter() {
      this.filters.keyword = ''
      this.filters.status = ''
      this.filters.date = ''
    },

    getStatusClass(status) {
      if (status === 'WAITING') return 'badge-gray'
      if (status === 'RECEIVING') return 'badge-orange'
      if (status === 'PARTIAL') return 'badge-blue'
      if (status === 'COMPLETED') return 'badge-green'

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

.detail-row {
  display: flex;
  justify-content: space-between;
  border-bottom: 1px solid #e2e8f0;
  padding-bottom: 12px;
  color: #475569;
}
</style>