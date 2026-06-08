<template>
  <div class="space-y-6">
    <!-- Page Header -->
    <div class="flex flex-col xl:flex-row xl:items-center xl:justify-between gap-4">
      <div>
        <h1 class="text-2xl font-bold text-slate-800">
          Stock Inquiry
        </h1>
        <p class="text-slate-500">
          Search and monitor warehouse inventory by SKU, lot, zone and location
        </p>
      </div>

      <button
        class="bg-orange-600 hover:bg-orange-700 text-white px-5 py-3 rounded-xl font-semibold"
      >
        Export Stock
      </button>
    </div>

    <!-- Summary Cards -->
    <div class="grid grid-cols-1 md:grid-cols-4 gap-5">
      <div class="summary-card">
        <p class="summary-label">Total SKUs</p>
        <h2 class="summary-value">1,248</h2>
      </div>

      <div class="summary-card">
        <p class="summary-label">Available Qty</p>
        <h2 class="summary-value">85,420</h2>
      </div>

      <div class="summary-card">
        <p class="summary-label">Reserved Qty</p>
        <h2 class="summary-value">6,310</h2>
      </div>

      <div class="summary-card">
        <p class="summary-label">Low Stock</p>
        <h2 class="summary-value text-red-600">18</h2>
      </div>
    </div>

    <!-- Filter -->
    <div class="bg-white rounded-2xl border border-slate-200 p-5">
      <div class="grid grid-cols-1 md:grid-cols-5 gap-4">
        <input
          v-model="filters.keyword"
          type="text"
          placeholder="Search SKU, Product, Barcode..."
          class="input"
        />

        <select v-model="filters.warehouse" class="input">
          <option value="">All Warehouses</option>
          <option value="WH-01">WH-01</option>
          <option value="WH-02">WH-02</option>
          <option value="WH-03">WH-03</option>
        </select>

        <select v-model="filters.zone" class="input">
          <option value="">All Zones</option>
          <option value="A">Zone A</option>
          <option value="B">Zone B</option>
          <option value="C">Zone C</option>
        </select>

        <select v-model="filters.stockStatus" class="input">
          <option value="">All Stock Status</option>
          <option value="AVAILABLE">Available</option>
          <option value="RESERVED">Reserved</option>
          <option value="HOLD">Hold</option>
          <option value="LOW_STOCK">Low Stock</option>
        </select>

        <button
          @click="clearFilter"
          class="bg-slate-100 hover:bg-slate-200 text-slate-700 rounded-xl font-semibold"
        >
          Clear Filter
        </button>
      </div>
    </div>

    <!-- Stock Table -->
    <div class="bg-white rounded-2xl border border-slate-200 shadow-sm overflow-hidden">
      <div class="w-full overflow-x-auto">
        <table class="min-w-[1250px] w-full text-sm">
          <thead class="bg-slate-50 text-slate-500">
            <tr>
              <th class="table-th">SKU</th>
              <th class="table-th">Product Name</th>
              <th class="table-th">Barcode</th>
              <th class="table-th">Warehouse</th>
              <th class="table-th">Location</th>
              <th class="table-th">Lot No.</th>
              <th class="table-th">Available</th>
              <th class="table-th">Reserved</th>
              <th class="table-th">On Hand</th>
              <th class="table-th">UOM</th>
              <th class="table-th">Status</th>
              <th class="table-th text-right">Action</th>
            </tr>
          </thead>

          <tbody>
            <tr
              v-for="item in filteredStockList"
              :key="item.id"
              class="border-t hover:bg-orange-50"
            >
              <td class="table-td font-semibold text-slate-800">
                {{ item.sku }}
              </td>

              <td class="table-td">
                {{ item.productName }}
              </td>

              <td class="table-td">
                {{ item.barcode }}
              </td>

              <td class="table-td">
                {{ item.warehouse }}
              </td>

              <td class="table-td">
                {{ item.location }}
              </td>

              <td class="table-td">
                {{ item.lotNo }}
              </td>

              <td class="table-td font-semibold text-green-600">
                {{ item.availableQty }}
              </td>

              <td class="table-td text-blue-600">
                {{ item.reservedQty }}
              </td>

              <td class="table-td font-semibold">
                {{ item.onHandQty }}
              </td>

              <td class="table-td">
                {{ item.uom }}
              </td>

              <td class="table-td">
                <span :class="getStatusClass(item.stockStatus)">
                  {{ item.stockStatusText }}
                </span>
              </td>

              <td class="table-td text-right">
                <button
                  @click="openStockDetail(item)"
                  class="text-orange-600 hover:text-orange-700 font-semibold"
                >
                  View Detail
                </button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- Stock Detail Drawer -->
    <div
      v-if="selectedStock"
      class="fixed inset-0 bg-black/40 flex justify-end z-50"
    >
      <div class="w-full max-w-2xl bg-white h-full p-6 overflow-y-auto">
        <div class="flex items-center justify-between mb-6">
          <div>
            <h2 class="text-xl font-bold text-slate-800">
              Stock Detail
            </h2>
            <p class="text-slate-500">
              {{ selectedStock.sku }} / {{ selectedStock.location }}
            </p>
          </div>

          <button
            @click="selectedStock = null"
            class="text-slate-500 hover:text-red-600 text-xl"
          >
            ✕
          </button>
        </div>

        <!-- Product Info -->
        <div class="bg-slate-50 rounded-2xl p-5 mb-6">
          <div class="grid grid-cols-2 gap-4">
            <div>
              <p class="detail-label">Product Name</p>
              <p class="detail-value">{{ selectedStock.productName }}</p>
            </div>

            <div>
              <p class="detail-label">Barcode</p>
              <p class="detail-value">{{ selectedStock.barcode }}</p>
            </div>

            <div>
              <p class="detail-label">Warehouse</p>
              <p class="detail-value">{{ selectedStock.warehouse }}</p>
            </div>

            <div>
              <p class="detail-label">Location</p>
              <p class="detail-value text-orange-600">
                {{ selectedStock.location }}
              </p>
            </div>

            <div>
              <p class="detail-label">Lot No.</p>
              <p class="detail-value">{{ selectedStock.lotNo }}</p>
            </div>

            <div>
              <p class="detail-label">Expiry Date</p>
              <p class="detail-value">{{ selectedStock.expiryDate }}</p>
            </div>
          </div>
        </div>

        <!-- Quantity Summary -->
        <div class="grid grid-cols-3 gap-4 mb-6">
          <div class="qty-card">
            <p class="summary-label">Available</p>
            <h3 class="qty-value text-green-600">
              {{ selectedStock.availableQty }}
            </h3>
          </div>

          <div class="qty-card">
            <p class="summary-label">Reserved</p>
            <h3 class="qty-value text-blue-600">
              {{ selectedStock.reservedQty }}
            </h3>
          </div>

          <div class="qty-card">
            <p class="summary-label">On Hand</p>
            <h3 class="qty-value">
              {{ selectedStock.onHandQty }}
            </h3>
          </div>
        </div>

        <!-- Mock Movement History -->
        <div>
          <h3 class="font-bold text-slate-800 mb-4">
            Recent Movement
          </h3>

          <div class="space-y-3">
            <div
              v-for="movement in movementHistory"
              :key="movement.id"
              class="border border-slate-200 rounded-xl p-4"
            >
              <div class="flex justify-between">
                <div>
                  <p class="font-semibold text-slate-800">
                    {{ movement.type }}
                  </p>
                  <p class="text-sm text-slate-500">
                    {{ movement.documentNo }}
                  </p>
                </div>

                <div class="text-right">
                  <p
                    class="font-bold"
                    :class="movement.qty > 0 ? 'text-green-600' : 'text-red-600'"
                  >
                    {{ movement.qty > 0 ? '+' : '' }}{{ movement.qty }}
                  </p>
                  <p class="text-xs text-slate-500">
                    {{ movement.date }}
                  </p>
                </div>
              </div>
            </div>
          </div>
        </div>

        <button
          class="w-full bg-orange-600 hover:bg-orange-700 text-white py-3 rounded-xl font-semibold mt-6"
        >
          Create Adjustment
        </button>
      </div>
    </div>
  </div>
</template>

<script>
export default {
  name: 'StockInquiryView',

  data() {
    return {
      selectedStock: null,

      filters: {
        keyword: '',
        warehouse: '',
        zone: '',
        stockStatus: ''
      },

      stockList: [
        {
          id: 1,
          sku: 'SKU-0001',
          productName: 'Barcode Label Roll',
          barcode: '885000001001',
          warehouse: 'WH-01',
          zone: 'A',
          location: 'A-01-01',
          lotNo: 'LOT-0001',
          expiryDate: '2027-06-08',
          availableQty: 120,
          reservedQty: 20,
          onHandQty: 140,
          uom: 'ROLL',
          stockStatus: 'AVAILABLE',
          stockStatusText: 'Available'
        },
        {
          id: 2,
          sku: 'SKU-0002',
          productName: 'Packing Box Size M',
          barcode: '885000001002',
          warehouse: 'WH-01',
          zone: 'B',
          location: 'B-02-03',
          lotNo: 'LOT-0002',
          expiryDate: '-',
          availableQty: 80,
          reservedQty: 30,
          onHandQty: 110,
          uom: 'PCS',
          stockStatus: 'AVAILABLE',
          stockStatusText: 'Available'
        },
        {
          id: 3,
          sku: 'SKU-0003',
          productName: 'RFID Tag',
          barcode: '885000001003',
          warehouse: 'WH-02',
          zone: 'A',
          location: 'A-03-02',
          lotNo: 'LOT-0003',
          expiryDate: '2028-01-15',
          availableQty: 10,
          reservedQty: 5,
          onHandQty: 15,
          uom: 'PCS',
          stockStatus: 'LOW_STOCK',
          stockStatusText: 'Low Stock'
        },
        {
          id: 4,
          sku: 'SKU-0004',
          productName: 'Carton Box Large',
          barcode: '885000001004',
          warehouse: 'WH-03',
          zone: 'C',
          location: 'C-01-05',
          lotNo: 'LOT-0004',
          expiryDate: '-',
          availableQty: 0,
          reservedQty: 50,
          onHandQty: 50,
          uom: 'PCS',
          stockStatus: 'RESERVED',
          stockStatusText: 'Reserved'
        },
        {
          id: 5,
          sku: 'SKU-0005',
          productName: 'Damaged Pallet',
          barcode: '885000001005',
          warehouse: 'WH-01',
          zone: 'C',
          location: 'HOLD-01',
          lotNo: 'LOT-0005',
          expiryDate: '-',
          availableQty: 0,
          reservedQty: 0,
          onHandQty: 8,
          uom: 'PCS',
          stockStatus: 'HOLD',
          stockStatusText: 'Hold'
        }
      ],

      movementHistory: [
        {
          id: 1,
          type: 'Receiving',
          documentNo: 'RCV-20260608-001',
          qty: 100,
          date: '2026-06-08'
        },
        {
          id: 2,
          type: 'Putaway',
          documentNo: 'PA-20260608-001',
          qty: 100,
          date: '2026-06-08'
        },
        {
          id: 3,
          type: 'Picking',
          documentNo: 'PICK-20260609-001',
          qty: -20,
          date: '2026-06-09'
        }
      ]
    }
  },

  computed: {
    filteredStockList() {
      return this.stockList.filter((item) => {
        const keyword = this.filters.keyword.toLowerCase()

        const matchKeyword =
          item.sku.toLowerCase().includes(keyword) ||
          item.productName.toLowerCase().includes(keyword) ||
          item.barcode.toLowerCase().includes(keyword) ||
          item.location.toLowerCase().includes(keyword) ||
          item.lotNo.toLowerCase().includes(keyword)

        const matchWarehouse =
          !this.filters.warehouse || item.warehouse === this.filters.warehouse

        const matchZone =
          !this.filters.zone || item.zone === this.filters.zone

        const matchStatus =
          !this.filters.stockStatus ||
          item.stockStatus === this.filters.stockStatus

        return matchKeyword && matchWarehouse && matchZone && matchStatus
      })
    }
  },

  methods: {
    openStockDetail(item) {
      this.selectedStock = item
    },

    clearFilter() {
      this.filters.keyword = ''
      this.filters.warehouse = ''
      this.filters.zone = ''
      this.filters.stockStatus = ''
    },

    getStatusClass(status) {
      if (status === 'AVAILABLE') return 'badge-green'
      if (status === 'RESERVED') return 'badge-blue'
      if (status === 'HOLD') return 'badge-orange'
      if (status === 'LOW_STOCK') return 'badge-red'
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

.summary-card,
.qty-card {
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

.qty-value {
  font-size: 24px;
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