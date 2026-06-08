<template>
  <div class="space-y-6">
    <div class="flex flex-col xl:flex-row xl:items-center xl:justify-between gap-4">
      <div>
        <h1 class="text-2xl font-bold text-slate-800">Check Stock</h1>
        <p class="text-slate-500">
          Perform physical stock count and compare with system quantity
        </p>
      </div>

      <button
        @click="createCountTask"
        class="bg-orange-600 hover:bg-orange-700 text-white px-5 py-3 rounded-xl font-semibold"
      >
        + New Count Task
      </button>
    </div>

    <div class="bg-white rounded-2xl border border-slate-200 p-6">
      <h2 class="text-lg font-bold text-slate-800 mb-5">Count Information</h2>

      <div class="grid grid-cols-1 md:grid-cols-4 gap-4">
        <div>
          <label class="label">Count Type</label>
          <select v-model="form.countType" class="input">
            <option value="CYCLE_COUNT">Cycle Count</option>
            <option value="FULL_COUNT">Full Count</option>
            <option value="LOCATION_COUNT">Location Count</option>
            <option value="SKU_COUNT">SKU Count</option>
          </select>
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
          <label class="label">Zone</label>
          <select v-model="form.zone" class="input">
            <option value="">All Zone</option>
            <option value="A">Zone A</option>
            <option value="B">Zone B</option>
            <option value="C">Zone C</option>
          </select>
        </div>

        <div>
          <label class="label">Count Date</label>
          <input v-model="form.countDate" type="date" class="input" />
        </div>
      </div>
    </div>

    <div class="bg-orange-50 border border-orange-200 rounded-2xl p-5">
      <div class="flex flex-col xl:flex-row xl:items-center xl:justify-between gap-4 mb-4">
        <div>
          <h2 class="text-lg font-bold text-slate-800">Scan Stock</h2>
          <p class="text-sm text-slate-500">
            Scan SKU, barcode or bin location to update physical count
          </p>
        </div>

        <span class="badge-orange">Scanner Ready</span>
      </div>

      <div class="grid grid-cols-1 md:grid-cols-5 gap-4">
        <input
          v-model="scanForm.sku"
          type="text"
          class="input bg-white"
          placeholder="Scan SKU / Barcode"
          @keyup.enter="focusCountQty"
          ref="skuInput"
        />

        <input
          v-model="scanForm.location"
          type="text"
          class="input bg-white"
          placeholder="Scan Location"
        />

        <input
          v-model.number="scanForm.countQty"
          type="number"
          class="input bg-white input-number"
          placeholder="Count Qty"
          @keyup.enter="confirmScan"
          ref="countQtyInput"
        />

        <select v-model="scanForm.condition" class="input bg-white">
          <option value="GOOD">Good</option>
          <option value="DAMAGED">Damaged</option>
          <option value="HOLD">Hold</option>
        </select>

        <button
          @click="confirmScan"
          class="bg-orange-600 hover:bg-orange-700 text-white rounded-xl font-semibold"
        >
          Confirm Count
        </button>
      </div>
    </div>

    <div class="bg-white rounded-2xl border border-slate-200 p-6">
      <div class="flex flex-col xl:flex-row xl:items-center xl:justify-between gap-4 mb-5">
        <div>
          <h2 class="text-lg font-bold text-slate-800">Stock Count Items</h2>
          <p class="text-sm text-slate-500">
            Review system quantity, counted quantity and variance
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
          <table class="min-w-[1400px] w-full text-sm">
            <thead class="bg-slate-50 text-slate-500">
              <tr>
                <th class="table-th">SKU</th>
                <th class="table-th">Product Name</th>
                <th class="table-th">Location</th>
                <th class="table-th">Lot No.</th>
                <th class="table-th text-right">System Qty</th>
                <th class="table-th text-right">Count Qty</th>
                <th class="table-th text-right">Variance</th>
                <th class="table-th">UOM</th>
                <th class="table-th">Condition</th>
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
                :class="getVarianceRowClass(item)"
              >
                <td class="table-td">
                  <input v-model="item.sku" class="table-input" />
                </td>

                <td class="table-td">
                  <input v-model="item.productName" class="table-input min-w-[220px]" />
                </td>

                <td class="table-td">
                  <input v-model="item.location" class="table-input" />
                </td>

                <td class="table-td">
                  <input v-model="item.lotNo" class="table-input" />
                </td>

                <td class="table-td">
                  <input
                    v-model.number="item.systemQty"
                    type="number"
                    class="table-input input-number bg-slate-50"
                    readonly
                  />
                </td>

                <td class="table-td">
                  <input
                    v-model.number="item.countQty"
                    type="number"
                    class="table-input input-number"
                  />
                </td>

                <td class="table-td text-right font-bold">
                  <span :class="calculateVariance(item) === 0 ? 'text-slate-600' : 'text-red-600'">
                    {{ calculateVariance(item) > 0 ? '+' : '' }}{{ calculateVariance(item) }}
                  </span>
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
                  <select v-model="item.condition" class="table-input min-w-[130px]">
                    <option value="GOOD">Good</option>
                    <option value="DAMAGED">Damaged</option>
                    <option value="HOLD">Hold</option>
                  </select>
                </td>

                <td class="table-td">
                  <select v-model="item.status" class="table-input min-w-[130px]">
                    <option value="COUNTED">Counted</option>
                    <option value="PENDING">Pending</option>
                    <option value="RECOUNT">Recount</option>
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
                <td colspan="12" class="text-center py-10 text-slate-400">
                  No stock count items
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </div>

    <div class="bg-white rounded-2xl border border-slate-200 p-6">
      <h2 class="text-lg font-bold text-slate-800 mb-5">Count Summary</h2>

      <div class="grid grid-cols-1 md:grid-cols-4 gap-5">
        <div class="summary-card">
          <p class="summary-label">Total Lines</p>
          <h2 class="summary-value text-number">{{ items.length }}</h2>
        </div>

        <div class="summary-card">
          <p class="summary-label">Matched Lines</p>
          <h2 class="summary-value text-green-600 text-number">{{ matchedLines }}</h2>
        </div>

        <div class="summary-card">
          <p class="summary-label">Variance Lines</p>
          <h2 class="summary-value text-red-600 text-number">{{ varianceLines }}</h2>
        </div>

        <div class="summary-card">
          <p class="summary-label">Net Variance</p>
          <h2
            class="summary-value text-number"
            :class="netVariance === 0 ? 'text-green-600' : 'text-orange-600'"
          >
            {{ netVariance > 0 ? '+' : '' }}{{ netVariance }}
          </h2>
        </div>
      </div>
    </div>

    <div class="bg-white border border-slate-200 rounded-2xl p-6">
      <div class="flex flex-col md:flex-row justify-end gap-3">
        <button
          @click="saveDraft"
          class="bg-white hover:bg-slate-100 text-slate-700 border border-slate-300 px-6 py-3 rounded-xl font-semibold"
        >
          Save Draft
        </button>

        <button
          @click="createAdjustment"
          class="bg-slate-800 hover:bg-slate-900 text-white px-6 py-3 rounded-xl font-semibold"
        >
          Create Adjustment
        </button>

        <button
          @click="confirmCount"
          class="bg-orange-600 hover:bg-orange-700 text-white px-6 py-3 rounded-xl font-semibold"
        >
          Confirm Count
        </button>
      </div>
    </div>
  </div>
</template>

<script>
export default {
  name: 'CheckStockView',

  data() {
    return {
      form: {
        countType: 'CYCLE_COUNT',
        warehouse: '',
        zone: '',
        countDate: '2026-06-08'
      },

      scanForm: {
        sku: '',
        location: '',
        countQty: 1,
        condition: 'GOOD'
      },

      stockMaster: [
        {
          sku: 'SKU-0001',
          productName: 'Barcode Label Roll',
          location: 'A-01-01',
          lotNo: 'LOT-0001',
          systemQty: 120,
          uom: 'ROLL'
        },
        {
          sku: 'SKU-0002',
          productName: 'Packing Box Size M',
          location: 'B-02-03',
          lotNo: 'LOT-0002',
          systemQty: 80,
          uom: 'PCS'
        },
        {
          sku: 'SKU-0003',
          productName: 'RFID Tag',
          location: 'A-03-02',
          lotNo: 'LOT-0003',
          systemQty: 15,
          uom: 'PCS'
        }
      ],

      items: []
    }
  },

  computed: {
    matchedLines() {
      return this.items.filter((item) => this.calculateVariance(item) === 0).length
    },

    varianceLines() {
      return this.items.filter((item) => this.calculateVariance(item) !== 0).length
    },

    netVariance() {
      return this.items.reduce((sum, item) => {
        return sum + this.calculateVariance(item)
      }, 0)
    }
  },

  mounted() {
    this.items = this.stockMaster.map((item, index) => {
      return {
        id: index + 1,
        ...item,
        countQty: 0,
        condition: 'GOOD',
        status: 'PENDING',
        remark: ''
      }
    })
  },

  methods: {
    focusCountQty() {
      if (this.$refs.countQtyInput) {
        this.$refs.countQtyInput.focus()
      }
    },

    confirmScan() {
      if (!this.scanForm.sku) {
        alert('SKU or Barcode is required')
        return
      }

      const stock = this.stockMaster.find((item) => {
        return item.sku === this.scanForm.sku
      })

      if (!stock) {
        alert('Stock item not found')
        return
      }

      const existingItem = this.items.find((item) => {
        return item.sku === stock.sku && item.location === stock.location
      })

      if (existingItem) {
        existingItem.countQty = Number(this.scanForm.countQty || 0)
        existingItem.condition = this.scanForm.condition
        existingItem.status = 'COUNTED'
      }

      this.scanForm = {
        sku: '',
        location: '',
        countQty: 1,
        condition: 'GOOD'
      }

      this.$nextTick(() => {
        if (this.$refs.skuInput) {
          this.$refs.skuInput.focus()
        }
      })
    },

    addItem() {
      this.items.push({
        id: Date.now(),
        sku: '',
        productName: '',
        location: '',
        lotNo: '',
        systemQty: 0,
        countQty: 0,
        uom: 'PCS',
        condition: 'GOOD',
        status: 'PENDING',
        remark: ''
      })
    },

    removeItem(index) {
      this.items.splice(index, 1)
    },

    calculateVariance(item) {
      return Number(item.countQty || 0) - Number(item.systemQty || 0)
    },

    getVarianceRowClass(item) {
      if (this.calculateVariance(item) === 0) return ''
      return 'bg-red-50'
    },

    saveDraft() {
      alert('Stock count draft saved successfully')
    },

    createAdjustment() {
      if (this.varianceLines === 0) {
        alert('No variance found')
        return
      }

      alert('Stock adjustment created from variance items')
      this.$router.push('/inventory/stock-adjustment/new')
    },

    confirmCount() {
      if (!this.form.warehouse) {
        alert('Warehouse is required')
        return
      }

      const pendingItem = this.items.find((item) => item.status === 'PENDING')

      if (pendingItem) {
        alert('Some items are still pending count')
        return
      }

      alert('Stock count confirmed successfully')
    },

    createCountTask() {
      alert('New count task created')
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