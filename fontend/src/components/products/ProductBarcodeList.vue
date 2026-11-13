<template>
  <div class="barcode-list">
    <p v-if="!productId" class="hint">{{ $t('productMaster.barcodes.saveFirstHint') }}</p>

    <template v-else>
      <div v-if="listError" class="error">{{ listError }}</div>

      <table v-if="barcodes.length > 0" class="table">
        <thead>
          <tr>
            <th>{{ $t('productMaster.barcodes.table.value') }}</th>
            <th>{{ $t('productMaster.barcodes.table.type') }}</th>
            <th>{{ $t('productMaster.barcodes.table.uom') }}</th>
            <th>{{ $t('productMaster.barcodes.table.qty') }}</th>
            <th>{{ $t('productMaster.barcodes.table.primary') }}</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="b in barcodes" :key="b.id">
            <td>{{ b.barcode_value }}</td>
            <td>{{ $t('productMaster.barcodes.types.' + b.barcode_type) }}</td>
            <td>{{ b.uom || '—' }}</td>
            <td>{{ b.qty_per_barcode }}</td>
            <td>{{ b.is_primary ? $t('common.labels.yes') : '—' }}</td>
            <td class="text-right">
              <button type="button" class="link-btn link-btn--danger" @click="removeBarcode(b.id)">
                {{ $t('common.buttons.delete') }}
              </button>
            </td>
          </tr>
        </tbody>
      </table>
      <p v-else class="hint">{{ $t('productMaster.barcodes.empty') }}</p>

      <div class="add-row">
        <input v-model="form.barcodeValue" type="text" class="input" :placeholder="$t('productMaster.barcodes.form.valuePlaceholder')" />
        <select v-model="form.barcodeType" class="input">
          <option value="SUPPLIER_BARCODE">{{ $t('productMaster.barcodes.types.SUPPLIER_BARCODE') }}</option>
          <option value="INTERNAL_BARCODE">{{ $t('productMaster.barcodes.types.INTERNAL_BARCODE') }}</option>
          <option value="BOX_CODE">{{ $t('productMaster.barcodes.types.BOX_CODE') }}</option>
          <option value="QR_CODE">{{ $t('productMaster.barcodes.types.QR_CODE') }}</option>
        </select>
        <input v-model="form.uom" type="text" class="input input--narrow" :placeholder="$t('productMaster.barcodes.form.uomPlaceholder')" />
        <input v-model.number="form.qtyPerBarcode" type="number" min="1" class="input input--narrow" />
        <label class="checkbox-label">
          <input v-model="form.isPrimary" type="checkbox" />
          {{ $t('productMaster.barcodes.form.primary') }}
        </label>
        <button type="button" class="btn-add" :disabled="adding || !form.barcodeValue" @click="addBarcode">
          {{ adding ? $t('common.buttons.submitting') : $t('productMaster.barcodes.form.addButton') }}
        </button>
      </div>
      <p v-if="addError" class="error">{{ addError }}</p>
    </template>
  </div>
</template>

<script>
import { listProductBarcodes, createProductBarcode, deleteProductBarcode } from '../../services/products'

// Makes the Product Add/Edit form's Barcode Mapping section real
// (wired to the migrations/0055 product_barcodes table) — before the
// EWTA enhancement this tab's multi-row data was decorative, rebuilt
// from scratch on every open and never persisted. See
// internal/product's own pre-change package doc.
export default {
  name: 'ProductBarcodeList',

  props: {
    productId: { type: [String, Number], default: null }
  },

  data() {
    return {
      barcodes: [],
      listError: '',
      addError: '',
      adding: false,
      form: this.emptyForm()
    }
  },

  watch: {
    productId(next) {
      if (next) this.fetchBarcodes()
    }
  },

  mounted() {
    if (this.productId) this.fetchBarcodes()
  },

  methods: {
    emptyForm() {
      return { barcodeValue: '', barcodeType: 'SUPPLIER_BARCODE', uom: '', qtyPerBarcode: 1, isPrimary: false }
    },

    async fetchBarcodes() {
      this.listError = ''
      try {
        const res = await listProductBarcodes(this.productId)
        this.barcodes = res.data
      } catch (err) {
        this.listError = err.message || this.$t('productMaster.barcodes.loadFailed')
      }
    },

    async addBarcode() {
      this.addError = ''
      this.adding = true
      try {
        await createProductBarcode(this.productId, {
          barcode_value: this.form.barcodeValue,
          barcode_type: this.form.barcodeType,
          uom: this.form.uom,
          qty_per_barcode: this.form.qtyPerBarcode || 1,
          is_primary: this.form.isPrimary
        })
        this.form = this.emptyForm()
        await this.fetchBarcodes()
      } catch (err) {
        this.addError = err.message || this.$t('productMaster.barcodes.addFailed')
      } finally {
        this.adding = false
      }
    },

    async removeBarcode(barcodeId) {
      try {
        await deleteProductBarcode(this.productId, barcodeId)
        await this.fetchBarcodes()
      } catch (err) {
        this.listError = err.message || this.$t('productMaster.barcodes.removeFailed')
      }
    }
  }
}
</script>

<style scoped>
.hint {
  color: #94a3b8;
  font-size: 0.85rem;
}

.error {
  color: #dc2626;
  font-size: 0.85rem;
  margin: 6px 0;
}

.table {
  width: 100%;
  font-size: 0.85rem;
  border-collapse: collapse;
  margin-bottom: 14px;
}

.table th {
  text-align: left;
  padding: 8px 10px;
  color: #64748b;
  font-weight: 600;
  border-bottom: 1px solid #e2e8f0;
}

.table td {
  padding: 8px 10px;
  border-bottom: 1px solid #f1f5f9;
  color: #334155;
}

.text-right {
  text-align: right;
}

.link-btn {
  background: none;
  border: none;
  cursor: pointer;
  font-weight: 600;
  font-size: 0.82rem;
  padding: 0;
}

.link-btn--danger {
  color: #dc2626;
}

.add-row {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  align-items: center;
}

.input {
  border: 1px solid #cbd5e1;
  border-radius: 10px;
  padding: 8px 10px;
  outline: none;
  flex: 1;
  min-width: 140px;
}

.input--narrow {
  flex: 0 0 80px;
  min-width: 70px;
}

.input:focus {
  border-color: #ea580c;
  box-shadow: 0 0 0 2px #fed7aa;
}

.checkbox-label {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 0.82rem;
  color: #475569;
  white-space: nowrap;
}

.btn-add {
  background: #ea580c;
  color: #fff;
  border: none;
  border-radius: 10px;
  padding: 9px 16px;
  font-weight: 600;
  font-size: 0.85rem;
  cursor: pointer;
  white-space: nowrap;
}

.btn-add:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}
</style>
