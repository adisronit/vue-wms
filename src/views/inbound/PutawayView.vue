<template>
  <div class="space-y-6">
    <div class="flex flex-col xl:flex-row xl:items-center xl:justify-between gap-4">
      <div>
        <h1 class="text-2xl font-bold text-slate-800">Putaway</h1>
        <p class="text-slate-500">
          Move inspected goods from staging area to storage locations
        </p>
      </div>

      <button
        @click="$router.push('/inbound/putaway/new')"
        class="bg-orange-600 hover:bg-orange-700 text-white px-5 py-3 rounded-xl font-semibold"
      >
        + New Putaway Task
      </button>
    </div>

    <div class="grid grid-cols-1 md:grid-cols-4 gap-5">
      <div class="summary-card">
        <p class="summary-label">Pending Putaway</p>
        <h2 class="summary-value">14</h2>
      </div>

      <div class="summary-card">
        <p class="summary-label">In Progress</p>
        <h2 class="summary-value">6</h2>
      </div>

      <div class="summary-card">
        <p class="summary-label">Completed</p>
        <h2 class="summary-value">32</h2>
      </div>

      <div class="summary-card">
        <p class="summary-label">Location Issues</p>
        <h2 class="summary-value text-red-600">3</h2>
      </div>
    </div>

    <div class="bg-white rounded-2xl border border-slate-200 p-5">
      <div class="grid grid-cols-1 md:grid-cols-4 gap-4">
        <input
          v-model="filters.keyword"
          type="text"
          placeholder="Search Task, SKU, Location..."
          class="input"
        />

        <select v-model="filters.status" class="input">
          <option value="">All Status</option>
          <option value="PENDING">Pending</option>
          <option value="IN_PROGRESS">In Progress</option>
          <option value="COMPLETED">Completed</option>
          <option value="ISSUE">Issue</option>
        </select>

        <select v-model="filters.zone" class="input">
          <option value="">All Zones</option>
          <option value="A">Zone A</option>
          <option value="B">Zone B</option>
          <option value="C">Zone C</option>
        </select>

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
        <table class="min-w-[1150px] w-full text-sm">
          <thead class="bg-slate-50 text-slate-500">
            <tr>
              <th class="table-th">Task No.</th>
              <th class="table-th">Receiving No.</th>
              <th class="table-th">SKU</th>
              <th class="table-th">Product</th>
              <th class="table-th">Qty</th>
              <th class="table-th">From Location</th>
              <th class="table-th">Suggested Bin</th>
              <th class="table-th">Actual Bin</th>
              <th class="table-th">Status</th>
              <th class="table-th text-right">Action</th>
            </tr>
          </thead>

          <tbody>
            <tr
              v-for="item in filteredPutawayList"
              :key="item.id"
              class="border-t hover:bg-orange-50"
            >
              <td class="table-td font-semibold text-slate-800">
                {{ item.taskNo }}
              </td>
              <td class="table-td">{{ item.receivingNo }}</td>
              <td class="table-td">{{ item.sku }}</td>
              <td class="table-td">{{ item.productName }}</td>
              <td class="table-td">{{ item.qty }}</td>
              <td class="table-td">{{ item.fromLocation }}</td>
              <td class="table-td">{{ item.suggestedBin }}</td>
              <td class="table-td">{{ item.actualBin || '-' }}</td>
              <td class="table-td">
                <span :class="getStatusClass(item.status)">
                  {{ item.statusText }}
                </span>
              </td>
              <td class="table-td text-right">
                <button
                  @click="openPutaway(item)"
                  class="text-orange-600 hover:text-orange-700 font-semibold"
                >
                  Putaway
                </button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <div
      v-if="selectedPutaway"
      class="fixed inset-0 bg-black/40 flex justify-end z-50"
    >
      <div class="w-full max-w-2xl bg-white h-full p-6 overflow-y-auto">
        <div class="flex items-center justify-between mb-6">
          <div>
            <h2 class="text-xl font-bold text-slate-800">
              Putaway Task
            </h2>
            <p class="text-slate-500">
              {{ selectedPutaway.taskNo }} / {{ selectedPutaway.sku }}
            </p>
          </div>

          <button
            @click="selectedPutaway = null"
            class="text-slate-500 hover:text-red-600 text-xl"
          >
            ✕
          </button>
        </div>

        <div class="bg-slate-50 rounded-2xl p-5 mb-6">
          <div class="grid grid-cols-2 gap-4">
            <div>
              <p class="detail-label">Product</p>
              <p class="detail-value">{{ selectedPutaway.productName }}</p>
            </div>

            <div>
              <p class="detail-label">Quantity</p>
              <p class="detail-value">{{ selectedPutaway.qty }}</p>
            </div>

            <div>
              <p class="detail-label">From Location</p>
              <p class="detail-value">{{ selectedPutaway.fromLocation }}</p>
            </div>

            <div>
              <p class="detail-label">Suggested Bin</p>
              <p class="detail-value text-orange-600">
                {{ selectedPutaway.suggestedBin }}
              </p>
            </div>
          </div>
        </div>

        <div class="space-y-4">
          <div>
            <label class="label">Scan SKU / Barcode</label>
            <input
              v-model="putawayForm.barcode"
              type="text"
              class="input"
              placeholder="Scan or enter barcode"
            />
          </div>

          <div>
            <label class="label">Actual Bin Location</label>
            <input
              v-model="putawayForm.actualBin"
              type="text"
              class="input"
              placeholder="Scan or enter bin location"
            />
          </div>

          <div>
            <label class="label">Putaway Qty</label>
            <input
              v-model="putawayForm.qty"
              type="number"
              class="input"
              placeholder="Enter putaway quantity"
            />
          </div>

          <div>
            <label class="label">Remark</label>
            <textarea
              v-model="putawayForm.remark"
              rows="4"
              class="input"
              placeholder="Enter remark"
            ></textarea>
          </div>

          <div class="grid grid-cols-1 md:grid-cols-2 gap-3 pt-4">
            <button
              @click="confirmPutaway"
              class="bg-orange-600 hover:bg-orange-700 text-white py-3 rounded-xl font-semibold"
            >
              Confirm Putaway
            </button>

            <button
              @click="markIssue"
              class="bg-red-600 hover:bg-red-700 text-white py-3 rounded-xl font-semibold"
            >
              Report Issue
            </button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script>
export default {
  name: 'PutawayView',

  data() {
    return {
      selectedPutaway: null,

      filters: {
        keyword: '',
        status: '',
        zone: ''
      },

      putawayForm: {
        barcode: '',
        actualBin: '',
        qty: '',
        remark: ''
      },

      putawayList: [
        {
          id: 1,
          taskNo: 'PA-20260608-001',
          receivingNo: 'RCV-20260608-001',
          sku: 'SKU-0001',
          productName: 'Barcode Label Roll',
          qty: 100,
          fromLocation: 'STAGING-01',
          suggestedBin: 'A-01-01',
          actualBin: '',
          zone: 'A',
          status: 'PENDING',
          statusText: 'Pending'
        },
        {
          id: 2,
          taskNo: 'PA-20260608-002',
          receivingNo: 'RCV-20260608-002',
          sku: 'SKU-0002',
          productName: 'Packing Box Size M',
          qty: 250,
          fromLocation: 'STAGING-01',
          suggestedBin: 'B-02-03',
          actualBin: 'B-02-03',
          zone: 'B',
          status: 'IN_PROGRESS',
          statusText: 'In Progress'
        },
        {
          id: 3,
          taskNo: 'PA-20260608-003',
          receivingNo: 'RCV-20260608-003',
          sku: 'SKU-0003',
          productName: 'RFID Tag',
          qty: 500,
          fromLocation: 'STAGING-02',
          suggestedBin: 'A-03-02',
          actualBin: 'A-03-02',
          zone: 'A',
          status: 'COMPLETED',
          statusText: 'Completed'
        },
        {
          id: 4,
          taskNo: 'PA-20260608-004',
          receivingNo: 'RCV-20260608-004',
          sku: 'SKU-0004',
          productName: 'Carton Box Large',
          qty: 300,
          fromLocation: 'STAGING-03',
          suggestedBin: 'C-01-05',
          actualBin: '',
          zone: 'C',
          status: 'ISSUE',
          statusText: 'Issue'
        }
      ]
    }
  },

  computed: {
    filteredPutawayList() {
      return this.putawayList.filter((item) => {
        const keyword = this.filters.keyword.toLowerCase()

        const matchKeyword =
          item.taskNo.toLowerCase().includes(keyword) ||
          item.receivingNo.toLowerCase().includes(keyword) ||
          item.sku.toLowerCase().includes(keyword) ||
          item.productName.toLowerCase().includes(keyword) ||
          item.suggestedBin.toLowerCase().includes(keyword)

        const matchStatus =
          !this.filters.status || item.status === this.filters.status

        const matchZone =
          !this.filters.zone || item.zone === this.filters.zone

        return matchKeyword && matchStatus && matchZone
      })
    }
  },

  methods: {
    openPutaway(item) {
      this.selectedPutaway = item
      this.putawayForm.barcode = item.sku
      this.putawayForm.actualBin = item.actualBin || item.suggestedBin
      this.putawayForm.qty = item.qty
      this.putawayForm.remark = ''
    },

    clearFilter() {
      this.filters.keyword = ''
      this.filters.status = ''
      this.filters.zone = ''
    },

    confirmPutaway() {
      if (!this.putawayForm.barcode) {
        alert('Barcode is required')
        return
      }

      if (!this.putawayForm.actualBin) {
        alert('Actual Bin Location is required')
        return
      }

      if (!this.putawayForm.qty) {
        alert('Putaway Qty is required')
        return
      }

      alert('Putaway confirmed successfully')
      this.selectedPutaway = null
    },

    markIssue() {
      alert('Putaway issue has been reported')
      this.selectedPutaway = null
    },

    getStatusClass(status) {
      if (status === 'PENDING') return 'badge-gray'
      if (status === 'IN_PROGRESS') return 'badge-orange'
      if (status === 'COMPLETED') return 'badge-green'
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