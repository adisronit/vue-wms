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
            <option v-for="r in sourceReceipts" :key="r.id" :value="r.id">{{ r.receipt_no }}</option>
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
          <select v-model="form.warehouseId" class="input">
            <option value="">Select warehouse</option>
            <option v-for="w in warehouses" :key="w.id" :value="w.id">{{ w.code }} — {{ w.name }}</option>
          </select>
        </div>

        <div>
          <label class="label">From Location</label>
          <select v-model="form.fromLocationId" class="input" :disabled="!form.warehouseId">
            <option value="">Select staging location</option>
            <option v-for="l in locations" :key="l.id" :value="l.id">{{ l.code }}</option>
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

      <div v-if="formError" class="mt-4 bg-red-50 border border-red-200 text-red-600 rounded-xl p-3 text-sm">
        {{ formError }}
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
          {{ scanning ? 'Looking up…' : 'Scanner Ready' }}
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
          placeholder="Scan Actual Bin (location code)"
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
          :disabled="scanning"
          class="bg-orange-600 hover:bg-orange-700 text-white rounded-xl font-semibold disabled:opacity-60"
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
                  <input
                    v-model="item.sku"
                    class="table-input"
                    :class="{ 'border-red-400': item.sku && !item.productId }"
                    placeholder="SKU or barcode"
                    @blur="lookupItemProduct(item)"
                    @keyup.enter="lookupItemProduct(item)"
                  />
                </td>

                <td class="table-td">
                  <input v-model="item.productName" class="table-input min-w-[220px]" readonly />
                </td>

                <td class="table-td">
                  <input v-model="item.lotNo" class="table-input" />
                </td>

                <td class="table-td">
                  <select v-model="item.sourceLocationId" class="table-input">
                    <option value="">Select location</option>
                    <option v-for="l in locations" :key="l.id" :value="l.id">{{ l.code }}</option>
                  </select>
                </td>

                <td class="table-td">
                  <!-- No bin-suggestion algorithm exists in this codebase —
                       see PROJECT_STATUS.md. This column is presentational
                       only; the real destination is the "Actual Bin" select. -->
                  <span class="text-slate-400">—</span>
                </td>

                <td class="table-td">
                  <select v-model="item.destinationLocationId" class="table-input">
                    <option value="">Select bin</option>
                    <option v-for="l in locations" :key="l.id" :value="l.id">{{ l.code }}</option>
                  </select>
                </td>

                <td class="table-td">
                  <input
                    v-model.number="item.requestedQty"
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
          :disabled="submitting"
          class="bg-white hover:bg-slate-100 text-slate-700 border border-slate-300 px-6 py-3 rounded-xl font-semibold disabled:opacity-60"
        >
          Save Draft
        </button>

        <button
          v-if="canConfirm"
          @click="confirmPutaway"
          :disabled="submitting"
          class="bg-orange-600 hover:bg-orange-700 text-white px-6 py-3 rounded-xl font-semibold disabled:opacity-60"
        >
          {{ submitting ? 'Submitting…' : 'Confirm Putaway' }}
        </button>
        <p v-else class="text-sm text-slate-500 self-center">
          Your role can save drafts but not confirm putaway.
        </p>
      </div>
    </div>
  </div>
</template>

<script>
import { listWarehouses, listZones, listLocations } from '../../services/warehouses'
import { listProducts } from '../../services/products'
import { listReceipts } from '../../services/receiving'
import { createPutawayTask, confirmPutawayTask, getPutawaySources, newIdempotencyKey } from '../../services/putaway'
import { hasPermission } from '../../services/auth'

function todayDate() {
  return new Date().toISOString().slice(0, 10)
}

export default {
  name: 'NewPutawayView',

  data() {
    return {
      warehouses: [],
      locations: [],
      sourceReceipts: [],

      formError: '',
      submitting: false,
      scanning: false,

      form: {
        sourceDocument: '',
        putawayDate: todayDate(),
        assignedTo: 'Admin User',
        warehouseId: '',
        fromLocationId: '',
        priority: 'NORMAL',
        remark: ''
      },

      scanForm: {
        sku: '',
        actualBin: '',
        qty: 1,
        status: 'CONFIRMED'
      },

      items: []
    }
  },

  computed: {
    totalQty() {
      return this.items.reduce((sum, item) => {
        return sum + Number(item.requestedQty || 0)
      }, 0)
    },

    confirmedLines() {
      return this.items.filter((item) => item.status === 'CONFIRMED').length
    },

    issueLines() {
      return this.items.filter((item) => item.status === 'ISSUE').length
    },

    canConfirm() {
      return hasPermission('putaway.approve')
    }
  },

  watch: {
    'form.warehouseId'(warehouseId) {
      this.form.fromLocationId = ''
      this.fetchLocationsForWarehouse(warehouseId)
    }
  },

  mounted() {
    this.fetchWarehouses()
    this.fetchSourceReceipts()
    this.$nextTick(() => {
      if (this.$refs.skuInput) {
        this.$refs.skuInput.focus()
      }
    })
  },

  methods: {
    async fetchWarehouses() {
      try {
        const res = await listWarehouses({ limit: 100 })
        this.warehouses = res.data
      } catch (err) {
        this.formError = err.message
      }
    },

    // Only receipts with something to put away are useful here — a
    // WAITING receipt has no received_qty yet, and DRAFT/CANCELLED have
    // nothing real to move. There is no dedicated putaway-eligible
    // receipts endpoint; this reuses GET /receipts (Phase 6) directly,
    // filtered to the two statuses that can have received_qty > 0 lines.
    async fetchSourceReceipts() {
      try {
        const [partial, completed] = await Promise.all([
          listReceipts({ status: 'PARTIALLY_RECEIVED', limit: 100 }),
          listReceipts({ status: 'COMPLETED', limit: 100 })
        ])
        this.sourceReceipts = [...partial.data, ...completed.data]
      } catch (err) {
        this.formError = err.message
      }
    },

    async fetchLocationsForWarehouse(warehouseId) {
      this.locations = []
      if (!warehouseId) return
      try {
        const zonesRes = await listZones(warehouseId)
        const allLocations = []
        for (const zone of zonesRes.data) {
          const locRes = await listLocations(zone.id)
          allLocations.push(...locRes.data)
        }
        this.locations = allLocations
      } catch (err) {
        this.formError = err.message
      }
    },

    async findProduct(text) {
      const trimmed = (text || '').trim()
      if (!trimmed) return null
      const res = await listProducts({ search: trimmed, limit: 20 })
      return res.data.find((p) => p.sku === trimmed || p.barcode === trimmed) || null
    },

    findLocationByCode(code) {
      const trimmed = (code || '').trim()
      if (!trimmed) return null
      return this.locations.find((l) => l.code === trimmed) || null
    },

    async lookupItemProduct(item) {
      if (!item.sku) return
      try {
        const product = await this.findProduct(item.sku)
        if (!product) {
          item.productId = null
          item.productName = ''
          this.formError = `No product found for SKU/barcode "${item.sku}"`
          return
        }
        item.productId = product.id
        item.sku = product.sku
        item.productName = product.name
        item.uom = product.base_uom || item.uom
      } catch (err) {
        this.formError = err.message
      }
    },

    async loadSourceItems() {
      this.items = []
      if (!this.form.sourceDocument) return

      this.formError = ''
      try {
        const res = await getPutawaySources(this.form.sourceDocument)
        this.items = res.data.map((line) => ({
          id: `receipt-${line.receipt_item_id}`,
          receiptItemId: line.receipt_item_id,
          productId: line.product_id,
          sku: line.sku,
          productName: line.product_name,
          lotNo: line.lot_no,
          sourceLocationId: line.source_location_id,
          destinationLocationId: '',
          requestedQty: line.remaining_qty,
          uom: line.uom,
          status: 'PENDING',
          remark: ''
        }))

        if (this.items.length > 0 && !this.form.fromLocationId) {
          this.form.fromLocationId = this.items[0].sourceLocationId
        }
      } catch (err) {
        this.formError = err.message
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
        receiptItemId: null,
        productId: null,
        sku: '',
        productName: '',
        lotNo: '',
        sourceLocationId: this.form.fromLocationId || '',
        destinationLocationId: '',
        requestedQty: 0,
        uom: 'PCS',
        status: 'PENDING',
        remark: ''
      })
    },

    async scanItem() {
      if (!this.scanForm.sku) {
        this.formError = 'SKU or Barcode is required'
        return
      }
      if (!this.scanForm.actualBin) {
        this.formError = 'Actual Bin is required'
        return
      }

      this.scanning = true
      this.formError = ''
      try {
        const product = await this.findProduct(this.scanForm.sku)
        if (!product) {
          this.formError = `No product found for SKU/barcode "${this.scanForm.sku}"`
          return
        }

        const destination = this.findLocationByCode(this.scanForm.actualBin)
        if (!destination) {
          this.formError = `No location found for bin code "${this.scanForm.actualBin}"`
          return
        }

        const matchedItem = this.items.find((item) => item.productId === product.id)

        if (!matchedItem) {
          this.items.push({
            id: Date.now(),
            receiptItemId: null,
            productId: product.id,
            sku: product.sku,
            productName: product.name,
            lotNo: '',
            sourceLocationId: this.form.fromLocationId || '',
            destinationLocationId: destination.id,
            requestedQty: Number(this.scanForm.qty || 1),
            uom: product.base_uom || 'PCS',
            status: this.scanForm.status,
            remark: this.scanForm.status === 'ISSUE' ? 'Reported by scan' : ''
          })
        } else {
          matchedItem.destinationLocationId = destination.id
          matchedItem.requestedQty = Number(this.scanForm.qty || matchedItem.requestedQty)
          matchedItem.status = this.scanForm.status
        }

        this.scanForm = { sku: '', actualBin: '', qty: 1, status: 'CONFIRMED' }

        this.$nextTick(() => {
          if (this.$refs.skuInput) {
            this.$refs.skuInput.focus()
          }
        })
      } catch (err) {
        this.formError = err.message
      } finally {
        this.scanning = false
      }
    },

    removeItem(index) {
      this.items.splice(index, 1)
    },

    // Every line needs a real destination before it can even be created
    // (the backend requires destination_location_id and rejects it
    // matching source_location_id — see internal/putaway) — matching
    // the original mock's own confirmPutaway() validation, which also
    // required Actual Bin + Qty for every line, not just actioned ones.
    validateForm() {
      if (!this.form.warehouseId) return 'Warehouse is required'
      if (this.items.length === 0) return 'At least one putaway item is required'
      if (this.items.some((item) => !item.productId)) {
        return 'Every item must resolve to a known product — check the SKU/barcode column'
      }
      if (this.items.some((item) => !item.sourceLocationId)) return 'Every item must have a from location'
      if (this.items.some((item) => !item.destinationLocationId)) return 'Every item needs an Actual Bin'
      if (this.items.some((item) => item.sourceLocationId === item.destinationLocationId)) {
        return 'From Location and Actual Bin must differ'
      }
      if (this.items.some((item) => !item.requestedQty || item.requestedQty <= 0)) {
        return 'Every item needs a quantity greater than zero'
      }
      if (this.items.some((item) => item.status === 'ISSUE' && !item.remark)) {
        return 'Every issue item needs a remark'
      }
      return ''
    },

    buildCreatePayload() {
      return {
        receipt_id: this.form.sourceDocument || null,
        warehouse_id: this.form.warehouseId,
        priority: this.form.priority,
        assigned_to: this.form.assignedTo || null,
        remark: this.form.remark || null,
        items: this.items.map((item) => ({
          receipt_item_id: item.receiptItemId,
          product_id: item.productId,
          lot_no: item.lotNo || '',
          source_location_id: item.sourceLocationId,
          destination_location_id: item.destinationLocationId,
          requested_qty: Number(item.requestedQty || 0),
          uom: item.uom,
          remark: item.remark || null
        }))
      }
    },

    async saveDraft() {
      const error = this.validateForm()
      if (error) {
        this.formError = error
        return
      }

      this.submitting = true
      this.formError = ''
      try {
        await createPutawayTask(this.buildCreatePayload())
        alert('Putaway draft saved successfully')
        this.$router.push('/inbound/put-away')
      } catch (err) {
        this.formError = err.message
      } finally {
        this.submitting = false
      }
    },

    async confirmPutaway() {
      const error = this.validateForm()
      if (error) {
        this.formError = error
        return
      }

      const actioned = this.items.filter((item) => item.status === 'CONFIRMED' || item.status === 'ISSUE')
      if (actioned.length === 0) {
        this.formError = 'Set at least one item to Confirmed or Issue before confirming putaway'
        return
      }

      this.submitting = true
      this.formError = ''
      try {
        const created = await createPutawayTask(this.buildCreatePayload())

        // Items come back in the same order they were submitted in, the
        // same guarantee Receiving's Confirm relies on.
        const confirmItems = created.data.items
          .map((respItem, idx) => ({ respItem, srcItem: this.items[idx] }))
          .filter(({ srcItem }) => srcItem.status === 'CONFIRMED' || srcItem.status === 'ISSUE')
          .map(({ respItem, srcItem }) => ({
            item_id: respItem.id,
            action: srcItem.status,
            quantity: Number(srcItem.requestedQty || 0),
            destination_location_id: srcItem.destinationLocationId || undefined,
            remark: srcItem.remark || ''
          }))

        await confirmPutawayTask(created.data.id, {
          idempotency_key: newIdempotencyKey(),
          items: confirmItems
        })

        alert('Putaway confirmed successfully')
        this.$router.push('/inbound/put-away')
      } catch (err) {
        this.formError = err.message
      } finally {
        this.submitting = false
      }
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