<template>
  <div class="space-y-6">
    <div class="flex flex-col xl:flex-row xl:items-center xl:justify-between gap-4">
      <div>
        <h1 class="text-2xl font-bold text-slate-800">Shipping</h1>
        <p class="text-slate-500">
          Manage packed orders, shipment confirmation and delivery dispatch
        </p>
      </div>

      <button
        @click="openShipping(null)"
        class="bg-orange-600 hover:bg-orange-700 text-white px-5 py-3 rounded-xl font-semibold"
      >
        + New Shipment
      </button>
    </div>

    <div class="grid grid-cols-1 md:grid-cols-4 gap-5">
      <div class="summary-card">
        <p class="summary-label">Ready to Ship</p>
        <h2 class="summary-value">18</h2>
      </div>

      <div class="summary-card">
        <p class="summary-label">In Transit</p>
        <h2 class="summary-value">24</h2>
      </div>

      <div class="summary-card">
        <p class="summary-label">Delivered</p>
        <h2 class="summary-value">46</h2>
      </div>

      <div class="summary-card">
        <p class="summary-label">Delivery Issues</p>
        <h2 class="summary-value text-red-600">4</h2>
      </div>
    </div>

    <div class="bg-white rounded-2xl border border-slate-200 p-5">
      <div class="grid grid-cols-1 md:grid-cols-5 gap-4">
        <input
          v-model="filters.keyword"
          type="text"
          placeholder="Search Shipment No, Order No, Customer..."
          class="input"
        />

        <select v-model="filters.status" class="input">
          <option value="">All Status</option>
          <option value="READY">Ready to Ship</option>
          <option value="DISPATCHED">Dispatched</option>
          <option value="IN_TRANSIT">In Transit</option>
          <option value="DELIVERED">Delivered</option>
          <option value="ISSUE">Issue</option>
        </select>

        <select v-model="filters.carrier" class="input">
          <option value="">All Carriers</option>
          <option value="DHL">DHL</option>
          <option value="KERRY">Kerry Express</option>
          <option value="FLASH">Flash Express</option>
          <option value="INTERNAL">Internal Truck</option>
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
        <table class="min-w-[1250px] w-full text-sm">
          <thead class="bg-slate-50 text-slate-500">
            <tr>
              <th class="table-th">Shipment No.</th>
              <th class="table-th">Order No.</th>
              <th class="table-th">Customer</th>
              <th class="table-th">Carrier</th>
              <th class="table-th">Tracking No.</th>
              <th class="table-th">Packages</th>
              <th class="table-th">Weight</th>
              <th class="table-th">Ship Date</th>
              <th class="table-th">Status</th>
              <th class="table-th text-right">Action</th>
            </tr>
          </thead>

          <tbody>
            <tr
              v-for="item in filteredShippingList"
              :key="item.id"
              class="border-t hover:bg-orange-50"
            >
              <td class="table-td font-semibold text-slate-800">
                {{ item.shipmentNo }}
              </td>

              <td class="table-td">{{ item.orderNo }}</td>
              <td class="table-td">{{ item.customer }}</td>
              <td class="table-td">{{ item.carrier }}</td>
              <td class="table-td">{{ item.trackingNo || '-' }}</td>
              <td class="table-td">{{ item.packages }}</td>
              <td class="table-td">{{ item.weight }} kg</td>
              <td class="table-td">{{ item.shipDate }}</td>

              <td class="table-td">
                <span :class="getStatusClass(item.status)">
                  {{ item.statusText }}
                </span>
              </td>

              <td class="table-td text-right">
                <button
                  @click="openShipping(item)"
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

    <div
      v-if="selectedShipping"
      class="fixed inset-0 bg-black/40 flex justify-end z-50"
    >
      <div class="w-full max-w-3xl bg-white h-full p-6 overflow-y-auto">
        <div class="flex items-center justify-between mb-6">
          <div>
            <h2 class="text-xl font-bold text-slate-800">
              Shipping Detail
            </h2>
            <p class="text-slate-500">
              {{ selectedShipping.shipmentNo }} / {{ selectedShipping.orderNo }}
            </p>
          </div>

          <button
            @click="selectedShipping = null"
            class="text-slate-500 hover:text-red-600 text-xl"
          >
            ✕
          </button>
        </div>

        <div class="bg-slate-50 rounded-2xl p-5 mb-6">
          <div class="grid grid-cols-2 md:grid-cols-4 gap-4">
            <div>
              <p class="detail-label">Customer</p>
              <p class="detail-value">{{ selectedShipping.customer }}</p>
            </div>

            <div>
              <p class="detail-label">Carrier</p>
              <p class="detail-value text-orange-600">{{ selectedShipping.carrier }}</p>
            </div>

            <div>
              <p class="detail-label">Packages</p>
              <p class="detail-value">{{ selectedShipping.packages }}</p>
            </div>

            <div>
              <p class="detail-label">Total Weight</p>
              <p class="detail-value">{{ selectedShipping.weight }} kg</p>
            </div>
          </div>
        </div>

        <div class="space-y-4">
          <div>
            <label class="label">Scan Order / Package / Tracking</label>
            <input
              v-model="shippingForm.scanText"
              type="text"
              class="input"
              placeholder="Scan barcode or enter tracking number"
            />
          </div>

          <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div>
              <label class="label">Carrier</label>
              <select v-model="shippingForm.carrier" class="input">
                <option value="">Select carrier</option>
                <option value="DHL">DHL</option>
                <option value="KERRY">Kerry Express</option>
                <option value="FLASH">Flash Express</option>
                <option value="INTERNAL">Internal Truck</option>
              </select>
            </div>

            <div>
              <label class="label">Tracking No.</label>
              <input
                v-model="shippingForm.trackingNo"
                type="text"
                class="input"
                placeholder="Enter tracking number"
              />
            </div>
          </div>

          <div class="grid grid-cols-1 md:grid-cols-3 gap-4">
            <div>
              <label class="label">Package Count</label>
              <input
                v-model.number="shippingForm.packages"
                type="number"
                class="input"
                placeholder="Packages"
              />
            </div>

            <div>
              <label class="label">Total Weight</label>
              <input
                v-model.number="shippingForm.weight"
                type="number"
                class="input"
                placeholder="Weight"
              />
            </div>

            <div>
              <label class="label">Ship Date</label>
              <input
                v-model="shippingForm.shipDate"
                type="date"
                class="input"
              />
            </div>
          </div>

          <div>
            <label class="label">Delivery Address</label>
            <textarea
              v-model="shippingForm.address"
              rows="4"
              class="input"
              placeholder="Enter delivery address"
            ></textarea>
          </div>

          <div>
            <label class="label">Remark</label>
            <textarea
              v-model="shippingForm.remark"
              rows="3"
              class="input"
              placeholder="Enter remark"
            ></textarea>
          </div>
        </div>

        <div class="mt-6">
          <h3 class="font-bold text-slate-800 mb-4">
            Package Items
          </h3>

          <div
            v-for="product in packageItems"
            :key="product.id"
            class="border border-slate-200 rounded-2xl p-4 mb-3"
          >
            <div class="flex justify-between gap-4">
              <div>
                <p class="font-bold text-slate-800">{{ product.sku }}</p>
                <p class="text-sm text-slate-500">{{ product.productName }}</p>
              </div>

              <div class="text-right">
                <p class="detail-label">Qty</p>
                <p class="detail-value">{{ product.qty }}</p>
              </div>
            </div>
          </div>
        </div>

        <div class="grid grid-cols-1 md:grid-cols-3 gap-3 mt-6">
          <button
            @click="confirmDispatch"
            class="bg-orange-600 hover:bg-orange-700 text-white py-3 rounded-xl font-semibold"
          >
            Confirm Dispatch
          </button>

          <button
            @click="markDelivered"
            class="bg-green-600 hover:bg-green-700 text-white py-3 rounded-xl font-semibold"
          >
            Mark Delivered
          </button>

          <button
            @click="reportIssue"
            class="bg-red-600 hover:bg-red-700 text-white py-3 rounded-xl font-semibold"
          >
            Report Issue
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script>
export default {
  name: 'ShippingView',

  data() {
    return {
      selectedShipping: null,

      filters: {
        keyword: '',
        status: '',
        carrier: '',
        date: ''
      },

      shippingForm: {
        scanText: '',
        carrier: '',
        trackingNo: '',
        packages: 1,
        weight: 0,
        shipDate: '',
        address: '',
        remark: ''
      },

      shippingList: [
        {
          id: 1,
          shipmentNo: 'SHP-20260608-001',
          orderNo: 'SO-20260608-001',
          customer: 'ABC Customer',
          carrier: 'DHL',
          trackingNo: '',
          packages: 2,
          weight: 18.5,
          shipDate: '2026-06-08',
          status: 'READY',
          statusText: 'Ready to Ship'
        },
        {
          id: 2,
          shipmentNo: 'SHP-20260608-002',
          orderNo: 'SO-20260608-002',
          customer: 'Global Retail',
          carrier: 'KERRY',
          trackingNo: 'KR20260608001',
          packages: 5,
          weight: 42.1,
          shipDate: '2026-06-08',
          status: 'IN_TRANSIT',
          statusText: 'In Transit'
        },
        {
          id: 3,
          shipmentNo: 'SHP-20260608-003',
          orderNo: 'SO-20260608-003',
          customer: 'Fast Shop',
          carrier: 'FLASH',
          trackingNo: 'FL20260608002',
          packages: 1,
          weight: 7.8,
          shipDate: '2026-06-09',
          status: 'DELIVERED',
          statusText: 'Delivered'
        }
      ],

      packageItems: [
        {
          id: 1,
          sku: 'SKU-0001',
          productName: 'Barcode Label Roll',
          qty: 20
        },
        {
          id: 2,
          sku: 'SKU-0002',
          productName: 'Packing Box Size M',
          qty: 50
        },
        {
          id: 3,
          sku: 'SKU-0003',
          productName: 'RFID Tag',
          qty: 10
        }
      ]
    }
  },

  computed: {
    filteredShippingList() {
      return this.shippingList.filter((item) => {
        const keyword = this.filters.keyword.toLowerCase()

        const matchKeyword =
          item.shipmentNo.toLowerCase().includes(keyword) ||
          item.orderNo.toLowerCase().includes(keyword) ||
          item.customer.toLowerCase().includes(keyword) ||
          item.carrier.toLowerCase().includes(keyword) ||
          item.trackingNo.toLowerCase().includes(keyword)

        const matchStatus =
          !this.filters.status || item.status === this.filters.status

        const matchCarrier =
          !this.filters.carrier || item.carrier === this.filters.carrier

        const matchDate =
          !this.filters.date || item.shipDate === this.filters.date

        return matchKeyword && matchStatus && matchCarrier && matchDate
      })
    }
  },

  methods: {
    openShipping(item) {
      this.selectedShipping = item || {
        shipmentNo: 'SHP-NEW',
        orderNo: 'SO-NEW',
        customer: 'New Customer',
        carrier: '',
        trackingNo: '',
        packages: 1,
        weight: 0,
        shipDate: '2026-06-08'
      }

      this.shippingForm = {
        scanText: '',
        carrier: this.selectedShipping.carrier,
        trackingNo: this.selectedShipping.trackingNo,
        packages: this.selectedShipping.packages,
        weight: this.selectedShipping.weight,
        shipDate: this.selectedShipping.shipDate,
        address: '',
        remark: ''
      }
    },

    clearFilter() {
      this.filters.keyword = ''
      this.filters.status = ''
      this.filters.carrier = ''
      this.filters.date = ''
    },

    confirmDispatch() {
      if (!this.shippingForm.carrier) {
        alert('Carrier is required')
        return
      }

      if (!this.shippingForm.trackingNo) {
        alert('Tracking No. is required')
        return
      }

      alert('Shipment dispatched successfully')
      this.selectedShipping = null
    },

    markDelivered() {
      alert('Shipment marked as delivered')
      this.selectedShipping = null
    },

    reportIssue() {
      alert('Shipping issue has been reported')
      this.selectedShipping = null
    },

    getStatusClass(status) {
      if (status === 'READY') return 'badge-gray'
      if (status === 'DISPATCHED') return 'badge-orange'
      if (status === 'IN_TRANSIT') return 'badge-blue'
      if (status === 'DELIVERED') return 'badge-green'
      if (status === 'ISSUE') return 'badge-red'
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