<template>
  <div class="space-y-6">
    <!-- Page Header -->
    <div class="flex flex-col xl:flex-row xl:items-center xl:justify-between gap-4">
      <div>
        <h1 class="text-2xl font-bold text-slate-800">
          New Receiving
        </h1>
        <p class="text-slate-500">
          Create a new inbound receiving document
        </p>
      </div>

      <button
        @click="$router.push('/inbound/receiving')"
        class="bg-slate-100 hover:bg-slate-200 text-slate-700 px-5 py-3 rounded-xl font-semibold"
      >
        Back to Receiving
      </button>
    </div>

    <!-- Document Header -->
    <div class="bg-white rounded-2xl border border-slate-200 p-6">
      <h2 class="text-lg font-bold text-slate-800 mb-5">
        Document Information
      </h2>

      <div class="grid grid-cols-1 md:grid-cols-3 gap-4">
        <div>
          <label class="label">Receiving Type</label>
          <select v-model="form.receivingType" class="input">
            <option value="PO">Purchase Order</option>
            <option value="ASN">ASN</option>
            <option value="RETURN">Customer Return</option>
            <option value="TRANSFER">Warehouse Transfer</option>
          </select>
        </div>

        <div>
          <label class="label">Reference No.</label>
          <input
            v-model="form.referenceNo"
            type="text"
            class="input"
            placeholder="PO / ASN / Transfer No."
          />
        </div>

        <div>
          <label class="label">Receiving Date</label>
          <input
            v-model="form.receivingDate"
            type="date"
            class="input"
          />
        </div>

        <div>
          <label class="label">Vendor</label>
          <select v-model="form.vendor" class="input">
            <option value="">Select vendor</option>
            <option value="ABC Supplier">ABC Supplier</option>
            <option value="Global Parts">Global Parts</option>
            <option value="Fast Supply">Fast Supply</option>
            <option value="Orange Logistics">Orange Logistics</option>
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
          <label class="label">Staging Location</label>
          <select v-model="form.stagingLocation" class="input">
            <option value="">Select staging location</option>
            <option value="STAGING-01">STAGING-01</option>
            <option value="STAGING-02">STAGING-02</option>
            <option value="QC-HOLD-01">QC-HOLD-01</option>
          </select>
        </div>
      </div>

      <div class="mt-4">
        <label class="label">Remark</label>
        <textarea
          v-model="form.remark"
          rows="3"
          class="input"
          placeholder="Enter receiving remark"
        ></textarea>
      </div>
    </div>

    <!-- Scan / Add Item -->
    <div class="bg-white rounded-2xl border border-slate-200 p-6">
      <div class="flex flex-col xl:flex-row xl:items-center xl:justify-between gap-4 mb-5">
        <div>
          <h2 class="text-lg font-bold text-slate-800">
            Receiving Items
          </h2>
          <p class="text-slate-500 text-sm">
            Add products manually or scan barcode
          </p>
        </div>

        <button
          @click="addItem"
          class="bg-orange-600 hover:bg-orange-700 text-white px-5 py-3 rounded-xl font-semibold"
        >
          + Add Item
        </button>
      </div>

      <!-- Barcode Scan -->
      <div class="bg-orange-50 border border-orange-200 rounded-2xl p-5 mb-5">
        <label class="label">Scan Barcode / SKU</label>

        <div class="grid grid-cols-1 md:grid-cols-4 gap-4">
          <input
            v-model="scanText"
            type="text"
            class="input bg-white"
            placeholder="Scan barcode or enter SKU"
            @keyup.enter="scanItem"
          />

          <input
            v-model.number="scanQty"
            type="number"
            class="input bg-white"
            placeholder="Qty"
          />

          <select v-model="scanLocation" class="input bg-white">
            <option value="STAGING-01">STAGING-01</option>
            <option value="STAGING-02">STAGING-02</option>
            <option value="QC-HOLD-01">QC-HOLD-01</option>
          </select>

          <button
            @click="scanItem"
            class="bg-orange-600 hover:bg-orange-700 text-white rounded-xl font-semibold"
          >
            Add Scan
          </button>
        </div>
      </div>

      <!-- Item Table -->
      <div class="border border-slate-200 rounded-2xl overflow-hidden">
        <div class="w-full overflow-x-auto">
          <table class="min-w-[1200px] w-full text-sm">
            <thead class="bg-slate-50 text-slate-500">
              <tr>
                <th class="table-th">SKU</th>
                <th class="table-th">Product Name</th>
                <th class="table-th">Barcode</th>
                <th class="table-th">Expected Qty</th>
                <th class="table-th">Receive Qty</th>
                <th class="table-th">UOM</th>
                <th class="table-th">Lot No.</th>
                <th class="table-th">Expiry Date</th>
                <th class="table-th">Location</th>
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
                  <input v-model="item.productName" class="table-input" />
                </td>

                <td class="table-td">
                  <input v-model="item.barcode" class="table-input" />
                </td>

                <td class="table-td">
                  <input v-model.number="item.expectedQty" type="number" class="table-input input-number" />
                </td>

                <td class="table-td">
                  <input v-model.number="item.receiveQty" type="number" class="table-input input-number" />
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
                  <input v-model="item.lotNo" class="table-input" />
                </td>

                <td class="table-td">
                  <input v-model="item.expiryDate" type="date" class="table-input" />
                </td>

                <td class="table-td">
                  <input v-model="item.location" class="table-input" />
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
                  No receiving items
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
        Receiving Summary
      </h2>

      <div class="grid grid-cols-1 md:grid-cols-4 gap-5">
        <div class="summary-card">
          <p class="summary-label">Total Lines</p>
          <h2 class="summary-value">{{ items.length }}</h2>
        </div>

        <div class="summary-card">
          <p class="summary-label">Expected Qty</p>
          <h2 class="summary-value">{{ totalExpectedQty }}</h2>
        </div>

        <div class="summary-card">
          <p class="summary-label">Receive Qty</p>
          <h2 class="summary-value text-orange-600">{{ totalReceiveQty }}</h2>
        </div>

        <div class="summary-card">
          <p class="summary-label">Variance</p>
          <h2
            class="summary-value"
            :class="totalVariance < 0 ? 'text-red-600' : 'text-green-600'"
          >
            {{ totalVariance }}
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
          @click="submitReceiving"
          class="bg-orange-600 hover:bg-orange-700 text-white px-6 py-3 rounded-xl font-semibold"
        >
          Confirm Receiving
        </button>
      </div>
    </div>
  </div>
</template>

<script>
export default {
  name: 'NewReceivingView',

  data() {
    return {
      scanText: '',
      scanQty: 1,
      scanLocation: 'STAGING-01',

      form: {
        receivingType: 'PO',
        referenceNo: '',
        receivingDate: '2026-06-08',
        vendor: '',
        warehouse: '',
        stagingLocation: 'STAGING-01',
        remark: ''
      },

      items: [
        {
          id: 1,
          sku: 'SKU-0001',
          productName: 'Barcode Label Roll',
          barcode: '885000001001',
          expectedQty: 100,
          receiveQty: 0,
          uom: 'ROLL',
          lotNo: '',
          expiryDate: '',
          location: 'STAGING-01'
        }
      ]
    }
  },

  computed: {
    totalExpectedQty() {
      return this.items.reduce((sum, item) => {
        return sum + Number(item.expectedQty || 0)
      }, 0)
    },

    totalReceiveQty() {
      return this.items.reduce((sum, item) => {
        return sum + Number(item.receiveQty || 0)
      }, 0)
    },

    totalVariance() {
      return this.totalReceiveQty - this.totalExpectedQty
    }
  },

  methods: {
    addItem() {
      this.items.push({
        id: Date.now(),
        sku: '',
        productName: '',
        barcode: '',
        expectedQty: 0,
        receiveQty: 0,
        uom: 'PCS',
        lotNo: '',
        expiryDate: '',
        location: this.form.stagingLocation || 'STAGING-01'
      })
    },

    scanItem() {
      if (!this.scanText) {
        alert('Barcode or SKU is required')
        return
      }

      const existingItem = this.items.find((item) => {
        return item.sku === this.scanText || item.barcode === this.scanText
      })

      if (existingItem) {
        existingItem.receiveQty += Number(this.scanQty || 1)
      } else {
        this.items.push({
          id: Date.now(),
          sku: this.scanText,
          productName: 'Scanned Product',
          barcode: this.scanText,
          expectedQty: 0,
          receiveQty: Number(this.scanQty || 1),
          uom: 'PCS',
          lotNo: '',
          expiryDate: '',
          location: this.scanLocation
        })
      }

      this.scanText = ''
      this.scanQty = 1
    },

    removeItem(index) {
      this.items.splice(index, 1)
    },

    saveDraft() {
      alert('Receiving draft saved successfully')
    },

    submitReceiving() {
      if (!this.form.referenceNo) {
        alert('Reference No. is required')
        return
      }

      if (!this.form.vendor) {
        alert('Vendor is required')
        return
      }

      if (!this.form.warehouse) {
        alert('Warehouse is required')
        return
      }

      if (this.items.length === 0) {
        alert('At least one item is required')
        return
      }

      alert('Receiving confirmed successfully')
      this.$router.push('/inbound/receiving')
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

.text-number {
  text-align: right;
  font-variant-numeric: tabular-nums;
}

.input-number {
  text-align: right;
  font-variant-numeric: tabular-nums;
}
</style>