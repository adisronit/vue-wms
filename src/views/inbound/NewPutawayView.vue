<template>
  <div class="space-y-6">
    <!-- Page Header -->
    <div class="flex flex-col xl:flex-row xl:items-center xl:justify-between gap-4">
      <div>
        <h1 class="text-2xl font-bold text-slate-800">
          New Putaway Task
        </h1>
        <p class="text-slate-500">
          Move QC passed goods from staging area to storage bin locations
        </p>
      </div>

      <button
        @click="$router.push('/inbound/putaway')"
        class="bg-slate-100 hover:bg-slate-200 text-slate-700 px-5 py-3 rounded-xl font-semibold"
      >
        Back to Putaway
      </button>
    </div>

    <!-- Task Information -->
    <div class="bg-white rounded-2xl border border-slate-200 p-6">
      <h2 class="text-lg font-bold text-slate-800 mb-5">
        Task Information
      </h2>

      <div class="grid grid-cols-1 md:grid-cols-3 gap-4">
        <div>
          <label class="label">Source Document</label>
          <select v-model="form.sourceDocument" class="input" @change="loadSourceItems">
            <option value="">Select source document</option>
            <option value="QC-20260608-001">QC-20260608-001</option>
            <option value="QC-20260608-002">QC-20260608-002</option>
            <option value="RCV-20260608-001">RCV-20260608-001</option>
          </select>
        </div>

        <div>
          <label class="label">Putaway Date</label>
          <input v-model="form.putawayDate" type="date" class="input" />
        </div>

        <div>
          <label class="label">Assigned To</label>
          <input
            v-model="form.assignedTo"
            type="text"
            class="input"
            placeholder="Warehouse staff"
          />
        </div>

        <div>
          <label class="label">Warehouse</label>
          <select v-model="form.warehouse" class="input">
            <option value="">Select warehouse</option>
            <option value="WH-01">WH-01</option>
            <option value="WH-02">WH-02</option>
            <option value="WH-03">WH-03</option>
          </select>
        </div>

        <div>
          <label class="label">From Location</label>
          <select v-model="form.fromLocation" class="input">
            <option value="">Select staging location</option>
            <option value="STAGING-01">STAGING-01</option>
            <option value="STAGING-02">STAGING-02</option>
            <option value="QC-PASS-01">QC-PASS-01</option>
            <option value="QC-HOLD-01">QC-HOLD-01</option>
          </select>
        </div>

        <div>
          <label class="label">Priority</label>
          <select v-model="form.priority" class="input">
            <option value="NORMAL">Normal</option>
            <option value="HIGH">High</option>
            <option value="LOW">Low</option>
          </select>
        </div>
      </div>

      <div class="mt-4">
        <label class="label">Remark</label>
        <textarea
          v-model="form.remark"
          rows="3"
          class="input"
          placeholder="Enter task remark"
        ></textarea>
      </div>
    </div>

    <!-- Scan Area -->
    <div class="bg-orange-50 border border-orange-200 rounded-2xl p-5">
      <div class="flex flex-col xl:flex-row xl:items-center xl:justify-between gap-4 mb-4">
        <div>
          <h2 class="text-lg font-bold text-slate-800">
            Scan Putaway
          </h2>
          <p class="text-sm text-slate-500">
            Scan SKU and actual bin location to confirm movement
          </p>
        </div>

        <span class="badge-orange">
          Scanner Ready
        </span>
      </div>

      <div class="grid grid-cols-1 md:grid-cols-5 gap-4">
        <input
          v-model="scanForm.sku"
          type="text"
          class="input bg-white"
          placeholder="Scan SKU / Barcode"
          @keyup.enter="focusActualBin"
          ref="skuInput"
        />

        <input
          v-model="scanForm.actualBin"
          type="text"
          class="input bg-white"
          placeholder="Scan Actual Bin"
          @keyup.enter="scanItem"
          ref="actualBinInput"
        />

        <input
          v-model.number="scanForm.qty"
          type="number"
          class="input bg-white input-number"
          placeholder="Qty"
        />

        <select v-model="scanForm.status" class="input bg-white">
          <option value="CONFIRMED">Confirmed</option>
          <option value="PENDING">Pending</option>
          <option value="ISSUE">Issue</option>
        </select>

        <button
          @click="scanItem"
          class="bg-orange-600 hover:bg-orange-700 text-white rounded-xl font-semibold"
        >
          Confirm Scan
        </button>
      </div>
    </div>

    <!-- Putaway Items -->
    <div class="bg-white rounded-2xl border border-slate-200 p-6">
      <div class="flex flex-col xl:flex-row xl:items-center xl:justify-between gap-4 mb-5">
        <div>
          <h2 class="text-lg font-bold text-slate-800">
            Putaway Items
          </h2>
          <p class="text-sm text-slate-500">
            Validate source location, suggested bin and actual bin before confirmation
          </p>
        </div>

        <button
          @click="addItem"
          class="bg-orange-600 hover:bg-orange-700 text-white px-5 py-3 rounded-xl font-semibold"
        >
          + Add Item
        </button>
      </div>

      <div class="border border-slate-200 rounded-2xl overflow-hidden">
        <div class="w-full overflow-x-auto">
          <table class="min-w-[1450px] w-full text-sm">
            <thead class="bg-slate-50 text-slate-500">
              <tr>
                <th class="table-th">SKU</th>
                <th class="table-th">Product Name</th>
                <th class="table-th">Lot No.</th>
                <th class="table-th">From Location</th>
                <th class="table-th">Suggested Bin</th>
                <th class="table-th">Actual Bin</th>
                <th class="table-th text-right">Qty</th>
                <th class="table-th">UOM</th>
                <th class="table-th">Status</th>
                <th class="table-th">Remark</th>
                <th class="table-th text-right">Action</th>
              </tr>
            </thead>

            <tbody>
              <tr
                v-for="(item, index) in items"
                :key="item.id"
                class="border-t hover:bg-orange-50"
                :class="item.status === 'ISSUE' ? 'bg-red-50' : ''"
              >
                <td class="table-td">
                  <input v-model="item.sku" class="table-input" />
                </td>

                <td class="table-td">
                  <input v-model="item.productName" class="table-input min-w-[220px]" />
                </td>

                <td class="table-td">
                  <input v-model="item.lotNo" class="table-input" />
                </td>

                <td class="table-td">
                  <input v-model="item.fromLocation" class="table-input" />
                </td>

                <td class="table-td">
                  <input v-model="item.suggestedBin" class="table-input" />
                </td>

                <td class="table-td">
                  <input v-model="item.actualBin" class="table-input" />
                </td>

                <td class="table-td">
                  <input
                    v-model.number="item.qty"
                    type="number"
                    class="table-input input-number"
                  />
                </td>

                <td class="table-td">
                  <select v-model="item.uom" class="table-input">
                    <option value="PCS">PCS</option>
                    <option value="BOX">BOX</option>
                    <option value="ROLL">ROLL</option>
                    <option value="KG">KG</option>
                  </select>
                </td>

                <td class="table-td">
                  <select v-model="item.status" class="table-input min-w-[140px]">
                    <option value="PENDING">Pending</option>
                    <option value="CONFIRMED">Confirmed</option>
                    <option value="ISSUE">Issue</option>
                  </select>
                </td>

                <td class="table-td">
                  <input
                    v-model="item.remark"
                    class="table-input min-w-[220px]"
                    placeholder="Remark"
                  />
                </td>

                <td class="table-td text-right">
                  <button
                    @click="removeItem(index)"
                    class="text-red-600 hover:text-red-700 font-semibold"
                  >
                    Remove
                  </button>
                </td>
              </tr>

              <tr v-if="items.length === 0">
                <td colspan="11" class="text-center py-10 text-slate-400">
                  No putaway items
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </div>

    <!-- Summary -->
    <div class="bg-white rounded-2xl border border-slate-200 p-6">
      <h2 class="text-lg font-bold text-slate-800 mb-5">
        Putaway Summary
      </h2>

      <div class="grid grid-cols-1 md:grid-cols-4 gap-5">
        <div class="summary-card">
          <p class="summary-label">Total Lines</p>
          <h2 class="summary-value text-number">{{ items.length }}</h2>
        </div>

        <div class="summary-card">
          <p class="summary-label">Total Qty</p>
          <h2 class="summary-value text-number">{{ totalQty }}</h2>
        </div>

        <div class="summary-card">
          <p class="summary-label">Confirmed Lines</p>
          <h2 class="summary-value text-green-600 text-number">
            {{ confirmedLines }}
          </h2>
        </div>

        <div class="summary-card">
          <p class="summary-label">Issue Lines</p>
          <h2 class="summary-value text-red-600 text-number">
            {{ issueLines }}
          </h2>
        </div>
      </div>
    </div>

    <!-- Action Bar -->
    <div class="bg-white border border-slate-200 rounded-2xl p-6">
      <div class="flex flex-col md:flex-row justify-end gap-3">
        <button
          @click="saveDraft"
          class="bg-white hover:bg-slate-100 text-slate-700 border border-slate-300 px-6 py-3 rounded-xl font-semibold"
        >
          Save Draft
        </button>

        <button
          @click="confirmPutaway"
          class="bg-orange-600 hover:bg-orange-700 text-white px-6 py-3 rounded-xl font-semibold"
        >
          Confirm Putaway
        </button>
      </div>
    </div>
  </div>
</template>

<script>
export default {
  name: 'NewPutawayView',

  data() {
    return {
      form: {
        sourceDocument: '',
        putawayDate: '2026-06-08',
        assignedTo: 'Admin User',
        warehouse: '',
        fromLocation: 'STAGING-01',
        priority: 'NORMAL',
        remark: ''
      },

      scanForm: {
        sku: '',
        actualBin: '',
        qty: 1,
        status: 'CONFIRMED'
      },

      mockSourceItems: {
        'QC-20260608-001': [
          {
            id: 1,
            sku: 'SKU-0001',
            productName: 'Barcode Label Roll',
            lotNo: 'LOT-0001',
            fromLocation: 'QC-PASS-01',
            suggestedBin: 'A-01-01-01',
            actualBin: '',
            qty: 100,
            uom: 'ROLL',
            status: 'PENDING',
            remark: ''
          },
          {
            id: 2,
            sku: 'SKU-0002',
            productName: 'Packing Box Size M',
            lotNo: 'LOT-0002',
            fromLocation: 'QC-PASS-01',
            suggestedBin: 'B-02-03-01',
            actualBin: '',
            qty: 250,
            uom: 'PCS',
            status: 'PENDING',
            remark: ''
          }
        ],
        'QC-20260608-002': [
          {
            id: 3,
            sku: 'SKU-0003',
            productName: 'RFID Tag',
            lotNo: 'LOT-0003',
            fromLocation: 'QC-PASS-01',
            suggestedBin: 'A-03-02-01',
            actualBin: '',
            qty: 500,
            uom: 'PCS',
            status: 'PENDING',
            remark: ''
          }
        ],
        'RCV-20260608-001': [
          {
            id: 4,
            sku: 'SKU-0004',
            productName: 'Carton Box Large',
            lotNo: 'LOT-0004',
            fromLocation: 'STAGING-01',
            suggestedBin: 'C-01-05-01',
            actualBin: '',
            qty: 300,
            uom: 'PCS',
            status: 'PENDING',
            remark: ''
          }
        ]
      },

      items: []
    }
  },

  computed: {
    totalQty() {
      return this.items.reduce((sum, item) => {
        return sum + Number(item.qty || 0)
      }, 0)
    },

    confirmedLines() {
      return this.items.filter((item) => item.status === 'CONFIRMED').length
    },

    issueLines() {
      return this.items.filter((item) => item.status === 'ISSUE').length
    }
  },

  mounted() {
    this.$nextTick(() => {
      if (this.$refs.skuInput) {
        this.$refs.skuInput.focus()
      }
    })
  },

  methods: {
    loadSourceItems() {
      if (!this.form.sourceDocument) {
        this.items = []
        return
      }

      const sourceItems = this.mockSourceItems[this.form.sourceDocument] || []

      this.items = sourceItems.map((item) => {
        return { ...item }
      })

      if (this.items.length > 0) {
        this.form.fromLocation = this.items[0].fromLocation
      }
    },

    focusActualBin() {
      if (this.$refs.actualBinInput) {
        this.$refs.actualBinInput.focus()
      }
    },

    addItem() {
      this.items.push({
        id: Date.now(),
        sku: '',
        productName: '',
        lotNo: '',
        fromLocation: this.form.fromLocation || 'STAGING-01',
        suggestedBin: '',
        actualBin: '',
        qty: 0,
        uom: 'PCS',
        status: 'PENDING',
        remark: ''
      })
    },

    scanItem() {
      if (!this.scanForm.sku) {
        alert('SKU or Barcode is required')
        return
      }

      if (!this.scanForm.actualBin) {
        alert('Actual Bin is required')
        return
      }

      const matchedItem = this.items.find((item) => {
        return item.sku === this.scanForm.sku
      })

      if (!matchedItem) {
        this.items.push({
          id: Date.now(),
          sku: this.scanForm.sku,
          productName: 'Scanned Product',
          lotNo: '',
          fromLocation: this.form.fromLocation || 'STAGING-01',
          suggestedBin: '',
          actualBin: this.scanForm.actualBin,
          qty: Number(this.scanForm.qty || 1),
          uom: 'PCS',
          status: this.scanForm.status,
          remark: 'Added by scan'
        })
      } else {
        matchedItem.actualBin = this.scanForm.actualBin
        matchedItem.qty = Number(this.scanForm.qty || matchedItem.qty)
        matchedItem.status = this.scanForm.status

        if (
          matchedItem.suggestedBin &&
          matchedItem.actualBin !== matchedItem.suggestedBin
        ) {
          matchedItem.status = 'ISSUE'
          matchedItem.remark = 'Actual bin does not match suggested bin'
        }
      }

      this.scanForm = {
        sku: '',
        actualBin: '',
        qty: 1,
        status: 'CONFIRMED'
      }

      this.$nextTick(() => {
        if (this.$refs.skuInput) {
          this.$refs.skuInput.focus()
        }
      })
    },

    removeItem(index) {
      this.items.splice(index, 1)
    },

    saveDraft() {
      alert('Putaway draft saved successfully')
    },

    confirmPutaway() {
      if (!this.form.sourceDocument) {
        alert('Source Document is required')
        return
      }

      if (!this.form.warehouse) {
        alert('Warehouse is required')
        return
      }

      if (this.items.length === 0) {
        alert('At least one putaway item is required')
        return
      }

      const invalidItem = this.items.find((item) => {
        return !item.actualBin || !item.qty || item.qty <= 0
      })

      if (invalidItem) {
        alert('Actual Bin and Qty are required for all items')
        return
      }

      const issueItem = this.items.find((item) => {
        return item.status === 'ISSUE'
      })

      if (issueItem) {
        alert('Some items have location issues. Please review before confirmation.')
        return
      }

      alert('Putaway confirmed successfully')
      this.$router.push('/inbound/putaway')
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
  padding: 10px 12px;
  color: #475569;
  white-space: nowrap;
}

.table-input {
  width: 100%;
  min-width: 120px;
  border: 1px solid #cbd5e1;
  border-radius: 10px;
  padding: 9px 10px;
  outline: none;
}

.table-input:focus {
  border-color: #ea580c;
  box-shadow: 0 0 0 2px #fed7aa;
}

.input-number,
.text-number {
  text-align: right;
  font-variant-numeric: tabular-nums;
}

.summary-card {
  background: #f8fafc;
  border: 1px solid #e2e8f0;
  border-radius: 18px;
  padding: 20px;
}

.summary-label {
  color: #64748b;
  font-size: 14px;
}

.summary-value {
  font-size: 28px;
  font-weight: 800;
  color: #0f172a;
  margin-top: 8px;
}

.badge-orange {
  background: #ffedd5;
  color: #ea580c;
  padding: 6px 12px;
  border-radius: 999px;
  font-weight: 700;
  font-size: 12px;
}
</style>