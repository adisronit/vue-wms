<template>
  <div class="space-y-6">
    <div class="flex flex-col xl:flex-row xl:items-center xl:justify-between gap-4">
      <div>
        <h1 class="text-2xl font-bold text-slate-800">Stock Adjustment</h1>
        <p class="text-slate-500">
          Create and review inventory adjustment transactions
        </p>
      </div>

      <button
        @click="openNewAdjustment"
        class="bg-orange-600 hover:bg-orange-700 text-white px-5 py-3 rounded-xl font-semibold"
      >
        + New Adjustment
      </button>
    </div>

    <div class="grid grid-cols-1 md:grid-cols-4 gap-5">
      <div class="summary-card">
        <p class="summary-label">Pending Approval</p>
        <h2 class="summary-value">5</h2>
      </div>

      <div class="summary-card">
        <p class="summary-label">Approved Today</p>
        <h2 class="summary-value">12</h2>
      </div>

      <div class="summary-card">
        <p class="summary-label">Rejected</p>
        <h2 class="summary-value text-red-600">2</h2>
      </div>

      <div class="summary-card">
        <p class="summary-label">Total Variance</p>
        <h2 class="summary-value text-orange-600">+38</h2>
      </div>
    </div>

    <div class="bg-white rounded-2xl border border-slate-200 p-5">
      <div class="grid grid-cols-1 md:grid-cols-5 gap-4">
        <input
          v-model="filters.keyword"
          type="text"
          placeholder="Search Adj No, SKU, Reason..."
          class="input"
        />

        <select v-model="filters.type" class="input">
          <option value="">All Types</option>
          <option value="INCREASE">Increase</option>
          <option value="DECREASE">Decrease</option>
        </select>

        <select v-model="filters.status" class="input">
          <option value="">All Status</option>
          <option value="DRAFT">Draft</option>
          <option value="PENDING">Pending Approval</option>
          <option value="APPROVED">Approved</option>
          <option value="REJECTED">Rejected</option>
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
              <th class="table-th">Adjustment No.</th>
              <th class="table-th">Date</th>
              <th class="table-th">SKU</th>
              <th class="table-th">Product</th>
              <th class="table-th">Location</th>
              <th class="table-th">Current Qty</th>
              <th class="table-th">Adjust Qty</th>
              <th class="table-th">New Qty</th>
              <th class="table-th">Reason</th>
              <th class="table-th">Status</th>
              <th class="table-th text-right">Action</th>
            </tr>
          </thead>

          <tbody>
            <tr
              v-for="item in filteredAdjustmentList"
              :key="item.id"
              class="border-t hover:bg-orange-50"
            >
              <td class="table-td font-semibold text-slate-800">
                {{ item.adjustmentNo }}
              </td>
              <td class="table-td">{{ item.date }}</td>
              <td class="table-td">{{ item.sku }}</td>
              <td class="table-td">{{ item.productName }}</td>
              <td class="table-td">{{ item.location }}</td>
              <td class="table-td">{{ item.currentQty }}</td>

              <td
                class="table-td font-semibold"
                :class="item.adjustQty > 0 ? 'text-green-600' : 'text-red-600'"
              >
                {{ item.adjustQty > 0 ? '+' : '' }}{{ item.adjustQty }}
              </td>

              <td class="table-td font-semibold">{{ item.newQty }}</td>
              <td class="table-td">{{ item.reason }}</td>

              <td class="table-td">
                <span :class="getStatusClass(item.status)">
                  {{ item.statusText }}
                </span>
              </td>

              <td class="table-td text-right">
                <button
                  @click="openDetail(item)"
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

    <div
      v-if="showDrawer"
      class="fixed inset-0 bg-black/40 flex justify-end z-50"
    >
      <div class="w-full max-w-2xl bg-white h-full p-6 overflow-y-auto">
        <div class="flex items-center justify-between mb-6">
          <div>
            <h2 class="text-xl font-bold text-slate-800">
              {{ selectedAdjustment ? 'Adjustment Detail' : 'New Stock Adjustment' }}
            </h2>
            <p class="text-slate-500">
              Adjust inventory quantity with approval workflow
            </p>
          </div>

          <button
            @click="closeDrawer"
            class="text-slate-500 hover:text-red-600 text-xl"
          >
            ✕
          </button>
        </div>

        <div class="space-y-4">
          <div>
            <label class="label">SKU / Barcode</label>
            <input
              v-model="form.sku"
              type="text"
              class="input"
              placeholder="Scan or enter SKU"
            />
          </div>

          <div>
            <label class="label">Location</label>
            <input
              v-model="form.location"
              type="text"
              class="input"
              placeholder="Enter location"
            />
          </div>

          <div class="grid grid-cols-1 md:grid-cols-3 gap-4">
            <div>
              <label class="label">Current Qty</label>
              <input
                v-model.number="form.currentQty"
                type="number"
                class="input bg-slate-50"
                readonly
              />
            </div>

            <div>
              <label class="label">Adjustment Type</label>
              <select v-model="form.type" class="input">
                <option value="INCREASE">Increase</option>
                <option value="DECREASE">Decrease</option>
              </select>
            </div>

            <div>
              <label class="label">Adjust Qty</label>
              <input
                v-model.number="form.adjustQty"
                type="number"
                class="input"
                placeholder="Qty"
              />
            </div>
          </div>

          <div class="bg-orange-50 border border-orange-200 rounded-2xl p-4">
            <p class="text-sm text-slate-500">New Quantity</p>
            <h3 class="text-3xl font-bold text-orange-600">
              {{ calculatedNewQty }}
            </h3>
          </div>

          <div>
            <label class="label">Reason</label>
            <select v-model="form.reason" class="input">
              <option value="">Select reason</option>
              <option value="Cycle Count Variance">Cycle Count Variance</option>
              <option value="Damaged Goods">Damaged Goods</option>
              <option value="Lost Goods">Lost Goods</option>
              <option value="Found Goods">Found Goods</option>
              <option value="System Correction">System Correction</option>
            </select>
          </div>

          <div>
            <label class="label">Remark</label>
            <textarea
              v-model="form.remark"
              rows="4"
              class="input"
              placeholder="Enter remark"
            ></textarea>
          </div>

          <div class="grid grid-cols-1 md:grid-cols-2 gap-3 pt-4">
            <button
              @click="submitAdjustment"
              class="bg-orange-600 hover:bg-orange-700 text-white py-3 rounded-xl font-semibold"
            >
              Submit for Approval
            </button>

            <button
              @click="saveDraft"
              class="bg-slate-100 hover:bg-slate-200 text-slate-700 py-3 rounded-xl font-semibold"
            >
              Save Draft
            </button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script>
export default {
  name: 'StockAdjustmentView',

  data() {
    return {
      showDrawer: false,
      selectedAdjustment: null,

      filters: {
        keyword: '',
        type: '',
        status: '',
        date: ''
      },

      form: {
        sku: '',
        location: '',
        currentQty: 100,
        type: 'INCREASE',
        adjustQty: 0,
        reason: '',
        remark: ''
      },

      adjustmentList: [
        {
          id: 1,
          adjustmentNo: 'ADJ-20260608-001',
          date: '2026-06-08',
          sku: 'SKU-0001',
          productName: 'Barcode Label Roll',
          location: 'A-01-01',
          currentQty: 120,
          adjustQty: 10,
          newQty: 130,
          type: 'INCREASE',
          reason: 'Found Goods',
          status: 'APPROVED',
          statusText: 'Approved'
        },
        {
          id: 2,
          adjustmentNo: 'ADJ-20260608-002',
          date: '2026-06-08',
          sku: 'SKU-0002',
          productName: 'Packing Box Size M',
          location: 'B-02-03',
          currentQty: 80,
          adjustQty: -5,
          newQty: 75,
          type: 'DECREASE',
          reason: 'Damaged Goods',
          status: 'PENDING',
          statusText: 'Pending Approval'
        },
        {
          id: 3,
          adjustmentNo: 'ADJ-20260608-003',
          date: '2026-06-08',
          sku: 'SKU-0003',
          productName: 'RFID Tag',
          location: 'A-03-02',
          currentQty: 15,
          adjustQty: -2,
          newQty: 13,
          type: 'DECREASE',
          reason: 'Cycle Count Variance',
          status: 'REJECTED',
          statusText: 'Rejected'
        }
      ]
    }
  },

  computed: {
    calculatedNewQty() {
      const qty = Number(this.form.adjustQty || 0)

      if (this.form.type === 'INCREASE') {
        return Number(this.form.currentQty || 0) + qty
      }

      return Number(this.form.currentQty || 0) - qty
    },

    filteredAdjustmentList() {
      return this.adjustmentList.filter((item) => {
        const keyword = this.filters.keyword.toLowerCase()

        const matchKeyword =
          item.adjustmentNo.toLowerCase().includes(keyword) ||
          item.sku.toLowerCase().includes(keyword) ||
          item.productName.toLowerCase().includes(keyword) ||
          item.reason.toLowerCase().includes(keyword)

        const matchType =
          !this.filters.type || item.type === this.filters.type

        const matchStatus =
          !this.filters.status || item.status === this.filters.status

        const matchDate =
          !this.filters.date || item.date === this.filters.date

        return matchKeyword && matchType && matchStatus && matchDate
      })
    }
  },

  methods: {
    openNewAdjustment() {
      this.selectedAdjustment = null
      this.form = {
        sku: '',
        location: '',
        currentQty: 100,
        type: 'INCREASE',
        adjustQty: 0,
        reason: '',
        remark: ''
      }
      this.showDrawer = true
    },

    openDetail(item) {
      this.selectedAdjustment = item
      this.form = {
        sku: item.sku,
        location: item.location,
        currentQty: item.currentQty,
        type: item.type,
        adjustQty: Math.abs(item.adjustQty),
        reason: item.reason,
        remark: ''
      }
      this.showDrawer = true
    },

    closeDrawer() {
      this.showDrawer = false
    },

    clearFilter() {
      this.filters.keyword = ''
      this.filters.type = ''
      this.filters.status = ''
      this.filters.date = ''
    },

    submitAdjustment() {
      if (!this.form.sku) {
        alert('SKU is required')
        return
      }

      if (!this.form.location) {
        alert('Location is required')
        return
      }

      if (!this.form.adjustQty || this.form.adjustQty <= 0) {
        alert('Adjust Qty must be greater than zero')
        return
      }

      if (!this.form.reason) {
        alert('Reason is required')
        return
      }

      alert('Stock adjustment submitted for approval')
      this.showDrawer = false
    },

    saveDraft() {
      alert('Draft saved successfully')
      this.showDrawer = false
    },

    getStatusClass(status) {
      if (status === 'DRAFT') return 'badge-gray'
      if (status === 'PENDING') return 'badge-orange'
      if (status === 'APPROVED') return 'badge-green'
      if (status === 'REJECTED') return 'badge-red'
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