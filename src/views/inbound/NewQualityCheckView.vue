<template>
  <div class="space-y-6">
    <!-- Page Header -->
    <div class="flex flex-col xl:flex-row xl:items-center xl:justify-between gap-4">
      <div>
        <h1 class="text-2xl font-bold text-slate-800">New Quality Check</h1>
        <p class="text-slate-500">
          Create inspection record for received items before putaway
        </p>
      </div>

      <button
        @click="$router.push('/inbound/quality-check')"
        class="bg-slate-100 hover:bg-slate-200 text-slate-700 px-5 py-3 rounded-xl font-semibold"
      >
        Back to QC List
      </button>
    </div>

    <!-- QC Header -->
    <div class="bg-white rounded-2xl border border-slate-200 p-6">
      <h2 class="text-lg font-bold text-slate-800 mb-5">
        QC Document Information
      </h2>

      <div class="grid grid-cols-1 md:grid-cols-3 gap-4">
        <div>
          <label class="label">Receiving No.</label>
          <select v-model="form.receivingNo" class="input">
            <option value="">Select receiving document</option>
            <option value="RCV-20260608-001">RCV-20260608-001</option>
            <option value="RCV-20260608-002">RCV-20260608-002</option>
            <option value="RCV-20260608-003">RCV-20260608-003</option>
          </select>
        </div>

        <div>
          <label class="label">QC Date</label>
          <input v-model="form.qcDate" type="date" class="input" />
        </div>

        <div>
          <label class="label">Inspector</label>
          <input
            v-model="form.inspector"
            type="text"
            class="input"
            placeholder="Inspector name"
          />
        </div>

        <div>
          <label class="label">Warehouse</label>
          <select v-model="form.warehouse" class="input">
            <option value="">Select warehouse</option>
            <option value="WH-01">WH-01</option>
            <option value="WH-02">WH-02</option>
          </select>
        </div>

        <div>
          <label class="label">QC Type</label>
          <select v-model="form.qcType" class="input">
            <option value="NORMAL">Normal Inspection</option>
            <option value="FULL">Full Inspection</option>
            <option value="RANDOM">Random Sampling</option>
            <option value="DAMAGE">Damage Inspection</option>
          </select>
        </div>

        <div>
          <label class="label">Overall Result</label>
          <select v-model="form.overallResult" class="input">
            <option value="">Pending</option>
            <option value="PASS">Pass</option>
            <option value="HOLD">Hold</option>
            <option value="FAIL">Fail</option>
          </select>
        </div>
      </div>

      <div class="mt-4">
        <label class="label">Remark</label>
        <textarea
          v-model="form.remark"
          rows="3"
          class="input"
          placeholder="Enter QC remark"
        ></textarea>
      </div>
    </div>

    <!-- Scan Area -->
    <div class="bg-orange-50 border border-orange-200 rounded-2xl p-5">
      <label class="label">Scan SKU / Barcode</label>

      <div class="grid grid-cols-1 md:grid-cols-5 gap-4">
        <input
          v-model="scanText"
          type="text"
          class="input bg-white"
          placeholder="Scan barcode or SKU"
          @keyup.enter="scanItem"
        />

        <input
          v-model.number="scanSampleQty"
          type="number"
          class="input bg-white input-number"
          placeholder="Sample Qty"
        />

        <input
          v-model.number="scanDefectQty"
          type="number"
          class="input bg-white input-number"
          placeholder="Defect Qty"
        />

        <select v-model="scanResult" class="input bg-white">
          <option value="PASS">Pass</option>
          <option value="HOLD">Hold</option>
          <option value="FAIL">Fail</option>
        </select>

        <button
          @click="scanItem"
          class="bg-orange-600 hover:bg-orange-700 text-white rounded-xl font-semibold"
        >
          Add QC
        </button>
      </div>
    </div>

    <!-- QC Items -->
    <div class="bg-white rounded-2xl border border-slate-200 p-6">
      <div class="flex items-center justify-between mb-5">
        <div>
          <h2 class="text-lg font-bold text-slate-800">QC Items</h2>
          <p class="text-sm text-slate-500">
            Inspect received items by SKU, lot and sample quantity
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
          <table class="min-w-[1350px] w-full text-sm">
            <thead class="bg-slate-50 text-slate-500">
              <tr>
                <th class="table-th">SKU</th>
                <th class="table-th">Product Name</th>
                <th class="table-th">Lot No.</th>
                <th class="table-th">Received Qty</th>
                <th class="table-th">Sample Qty</th>
                <th class="table-th">Defect Qty</th>
                <th class="table-th">Defect Reason</th>
                <th class="table-th">Result</th>
                <th class="table-th">Remark</th>
                <th class="table-th text-right">Action</th>
              </tr>
            </thead>

            <tbody>
              <tr
                v-for="(item, index) in items"
                :key="item.id"
                class="border-t hover:bg-orange-50"
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
                  <input
                    v-model.number="item.receivedQty"
                    type="number"
                    class="table-input input-number"
                  />
                </td>

                <td class="table-td">
                  <input
                    v-model.number="item.sampleQty"
                    type="number"
                    class="table-input input-number"
                  />
                </td>

                <td class="table-td">
                  <input
                    v-model.number="item.defectQty"
                    type="number"
                    class="table-input input-number"
                  />
                </td>

                <td class="table-td">
                  <select v-model="item.defectReason" class="table-input min-w-[180px]">
                    <option value="">None</option>
                    <option value="DAMAGED">Damaged</option>
                    <option value="WRONG_ITEM">Wrong Item</option>
                    <option value="EXPIRED">Expired</option>
                    <option value="SHORTAGE">Shortage</option>
                    <option value="PACKAGING_ISSUE">Packaging Issue</option>
                  </select>
                </td>

                <td class="table-td">
                  <select v-model="item.result" class="table-input">
                    <option value="PASS">Pass</option>
                    <option value="HOLD">Hold</option>
                    <option value="FAIL">Fail</option>
                  </select>
                </td>

                <td class="table-td">
                  <input v-model="item.remark" class="table-input min-w-[220px]" />
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
                <td colspan="10" class="text-center py-10 text-slate-400">
                  No QC items
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
        QC Summary
      </h2>

      <div class="grid grid-cols-1 md:grid-cols-4 gap-5">
        <div class="summary-card">
          <p class="summary-label">Total Lines</p>
          <h2 class="summary-value text-number">{{ items.length }}</h2>
        </div>

        <div class="summary-card">
          <p class="summary-label">Sample Qty</p>
          <h2 class="summary-value text-number">{{ totalSampleQty }}</h2>
        </div>

        <div class="summary-card">
          <p class="summary-label">Defect Qty</p>
          <h2 class="summary-value text-red-600 text-number">
            {{ totalDefectQty }}
          </h2>
        </div>

        <div class="summary-card">
          <p class="summary-label">Pass Rate</p>
          <h2 class="summary-value text-green-600 text-number">
            {{ passRate }}%
          </h2>
        </div>
      </div>
    </div>

    <!-- Action Bar -->
    <div class="sticky bottom-0 bg-slate-100 border-t border-slate-200 py-4">
      <div class="flex flex-col md:flex-row justify-end gap-3">
        <button
          @click="saveDraft"
          class="bg-white hover:bg-slate-100 text-slate-700 border border-slate-300 px-6 py-3 rounded-xl font-semibold"
        >
          Save Draft
        </button>

        <button
          @click="submitQc"
          class="bg-orange-600 hover:bg-orange-700 text-white px-6 py-3 rounded-xl font-semibold"
        >
          Confirm QC
        </button>
      </div>
    </div>
  </div>
</template>

<script>
export default {
  name: 'NewQualityCheckView',

  data() {
    return {
      scanText: '',
      scanSampleQty: 1,
      scanDefectQty: 0,
      scanResult: 'PASS',

      form: {
        receivingNo: '',
        qcDate: '2026-06-08',
        inspector: 'Admin User',
        warehouse: '',
        qcType: 'NORMAL',
        overallResult: '',
        remark: ''
      },

      items: [
        {
          id: 1,
          sku: 'SKU-0001',
          productName: 'Barcode Label Roll',
          lotNo: 'LOT-0001',
          receivedQty: 100,
          sampleQty: 10,
          defectQty: 0,
          defectReason: '',
          result: 'PASS',
          remark: ''
        }
      ]
    }
  },

  computed: {
    totalSampleQty() {
      return this.items.reduce((sum, item) => {
        return sum + Number(item.sampleQty || 0)
      }, 0)
    },

    totalDefectQty() {
      return this.items.reduce((sum, item) => {
        return sum + Number(item.defectQty || 0)
      }, 0)
    },

    passRate() {
      if (this.totalSampleQty === 0) return 0

      const passQty = this.totalSampleQty - this.totalDefectQty
      return ((passQty / this.totalSampleQty) * 100).toFixed(2)
    }
  },

  methods: {
    addItem() {
      this.items.push({
        id: Date.now(),
        sku: '',
        productName: '',
        lotNo: '',
        receivedQty: 0,
        sampleQty: 0,
        defectQty: 0,
        defectReason: '',
        result: 'PASS',
        remark: ''
      })
    },

    scanItem() {
      if (!this.scanText) {
        alert('Barcode or SKU is required')
        return
      }

      const existingItem = this.items.find((item) => {
        return item.sku === this.scanText
      })

      if (existingItem) {
        existingItem.sampleQty += Number(this.scanSampleQty || 1)
        existingItem.defectQty += Number(this.scanDefectQty || 0)
        existingItem.result = this.scanResult
      } else {
        this.items.push({
          id: Date.now(),
          sku: this.scanText,
          productName: 'Scanned Product',
          lotNo: '',
          receivedQty: 0,
          sampleQty: Number(this.scanSampleQty || 1),
          defectQty: Number(this.scanDefectQty || 0),
          defectReason: '',
          result: this.scanResult,
          remark: ''
        })
      }

      this.scanText = ''
      this.scanSampleQty = 1
      this.scanDefectQty = 0
      this.scanResult = 'PASS'
    },

    removeItem(index) {
      this.items.splice(index, 1)
    },

    saveDraft() {
      alert('QC draft saved successfully')
    },

    submitQc() {
      if (!this.form.receivingNo) {
        alert('Receiving No. is required')
        return
      }

      if (!this.form.warehouse) {
        alert('Warehouse is required')
        return
      }

      if (this.items.length === 0) {
        alert('At least one QC item is required')
        return
      }

      const invalidFailItem = this.items.find((item) => {
        return item.result === 'FAIL' && !item.defectReason
      })

      if (invalidFailItem) {
        alert('Defect Reason is required for failed item')
        return
      }

      alert('QC confirmed successfully')
      this.$router.push('/inbound/quality-check')
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
</style>