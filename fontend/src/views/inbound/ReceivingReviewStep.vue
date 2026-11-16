<template>
  <div class="space-y-6 pb-24">
    <!-- Header Review -->
    <FormSection :title="$t('newReceiving.review.documentTitle')">
      <dl class="grid grid-cols-1 md:grid-cols-3 gap-4 text-sm">
        <div><dt class="text-slate-500">{{ $t('newReceiving.fields.receivingType') }}</dt><dd class="font-semibold">{{ form.receivingType || '—' }}</dd></div>
        <div><dt class="text-slate-500">{{ $t('newReceiving.fields.referenceNo') }}</dt><dd class="font-semibold">{{ form.referenceNo || '—' }}</dd></div>
        <div><dt class="text-slate-500">{{ $t('newReceiving.fields.receivingDate') }}</dt><dd class="font-semibold">{{ form.receivingDate || '—' }}</dd></div>

        <div><dt class="text-slate-500">{{ $t('newReceiving.fields.supplier') }}</dt><dd class="font-semibold">{{ supplierDisplayName || form.vendor || '—' }}</dd></div>
        <div><dt class="text-slate-500">{{ $t('newReceiving.fields.warehouse') }}</dt><dd class="font-semibold">{{ warehouseLabel }}</dd></div>
        <div><dt class="text-slate-500">{{ $t('newReceiving.fields.stagingLocation') }}</dt><dd class="font-semibold">{{ stagingLocationLabel }}</dd></div>

        <div><dt class="text-slate-500">{{ $t('newReceiving.logistics.invoiceNo') }}</dt><dd class="font-semibold">{{ form.invoiceNo || '—' }}</dd></div>
        <div><dt class="text-slate-500">{{ $t('newReceiving.logistics.deliveryNoteNo') }}</dt><dd class="font-semibold">{{ form.deliveryNoteNo || '—' }}</dd></div>
        <div><dt class="text-slate-500">{{ $t('newReceiving.logistics.dockDoor') }}</dt><dd class="font-semibold">{{ form.dockDoor || '—' }}</dd></div>

        <div><dt class="text-slate-500">{{ $t('newReceiving.logistics.vehicleRegNo') }}</dt><dd class="font-semibold">{{ form.vehicleRegNo || '—' }}</dd></div>
        <div><dt class="text-slate-500">{{ $t('newReceiving.logistics.containerNo') }}</dt><dd class="font-semibold">{{ form.containerNo || '—' }}</dd></div>
        <div><dt class="text-slate-500">{{ $t('newReceiving.logistics.sealNo') }}</dt><dd class="font-semibold">{{ form.sealNo || '—' }}</dd></div>

        <div><dt class="text-slate-500">{{ $t('newReceiving.logistics.arrivalTime') }}</dt><dd class="font-semibold">{{ form.arrivalTime || '—' }}</dd></div>
        <div><dt class="text-slate-500">{{ $t('newReceiving.logistics.unloadingStartTime') }}</dt><dd class="font-semibold">{{ form.unloadingStartTime || '—' }}</dd></div>
        <div><dt class="text-slate-500">{{ $t('newReceiving.logistics.unloadingEndTime') }}</dt><dd class="font-semibold">{{ form.unloadingEndTime || '—' }}</dd></div>

        <div class="md:col-span-3"><dt class="text-slate-500">{{ $t('newReceiving.fields.remark') }}</dt><dd class="font-semibold">{{ form.remark || '—' }}</dd></div>
      </dl>
    </FormSection>

    <!-- Totals -->
    <FormSection :title="$t('newReceiving.summary.title')">
      <div class="grid grid-cols-2 md:grid-cols-5 gap-5 mb-5">
        <div class="summary-card">
          <p class="summary-label">{{ $t('newReceiving.summary.skuCount') }}</p>
          <h2 class="summary-value">{{ totals.skuCount }}</h2>
        </div>
        <div class="summary-card">
          <p class="summary-label">{{ $t('newReceiving.summary.totalLines') }}</p>
          <h2 class="summary-value">{{ totals.lineCount }}</h2>
        </div>
        <div class="summary-card">
          <p class="summary-label">{{ $t('newReceiving.summary.splitCount') }}</p>
          <h2 class="summary-value">{{ totals.splitCount }}</h2>
        </div>
        <div class="summary-card">
          <p class="summary-label">{{ $t('newReceiving.summary.shortageOverage') }}</p>
          <h2 class="summary-value">
            <span class="text-red-600">{{ totals.shortageCount }}</span>
            /
            <span class="text-green-600">{{ totals.overageCount }}</span>
          </h2>
        </div>
        <div class="summary-card">
          <p class="summary-label">{{ $t('newReceiving.summary.qcRequiredCount') }}</p>
          <h2 class="summary-value text-blue-600">{{ totals.qcRequiredCount }}</h2>
        </div>
      </div>

      <!-- Quantities by UOM — never combined across incompatible units. -->
      <div class="overflow-x-auto">
        <table class="w-full text-sm">
          <thead class="text-slate-500">
            <tr>
              <th class="text-left py-2 pr-4">{{ $t('newReceiving.summary.uom') }}</th>
              <th class="text-right py-2 pr-4">{{ $t('newReceiving.summary.expectedQty') }}</th>
              <th class="text-right py-2 pr-4">{{ $t('newReceiving.summary.receiveQty') }}</th>
              <th class="text-right py-2">{{ $t('newReceiving.summary.variance') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="row in totals.byUom" :key="row.uom" class="border-t">
              <td class="py-2 pr-4 font-semibold">{{ row.uom }}</td>
              <td class="py-2 pr-4 text-right tabular-nums">{{ row.expectedQty }}</td>
              <td class="py-2 pr-4 text-right tabular-nums text-orange-600">{{ row.receivedQty }}</td>
              <td
                class="py-2 text-right tabular-nums font-semibold"
                :class="row.receivedQty - row.expectedQty < 0 ? 'text-red-600' : 'text-green-600'"
              >
                {{ row.receivedQty - row.expectedQty }}
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </FormSection>

    <!-- Validation Summary -->
    <FormSection v-if="issues.length > 0" :title="$t('newReceiving.review.validationTitle')">
      <div class="space-y-2">
        <div
          v-for="(group, idx) in groupedIssues"
          :key="idx"
          class="flex items-start gap-3 rounded-xl p-3 text-sm"
          :class="group.level === 'error' ? 'bg-red-50 text-red-700 border border-red-200' : 'bg-amber-50 text-amber-800 border border-amber-200'"
        >
          <span class="font-bold">{{ group.level === 'error' ? '⛔' : '⚠' }}</span>
          <div>
            <p class="font-semibold">{{ group.label }}</p>
            <p>{{ $t(group.message) }}</p>
          </div>
        </div>
      </div>

      <div v-if="warnings.length > 0 && errors.length === 0" class="mt-4 bg-slate-50 border border-slate-200 rounded-xl p-4">
        <label class="flex items-start gap-3 text-sm">
          <input
            type="checkbox"
            v-model="acknowledgeWarnings"
            :disabled="!canApproveExceptions"
            class="mt-1"
          />
          <span>
            {{ $t('newReceiving.review.acknowledgeWarnings') }}
            <span v-if="!canApproveExceptions" class="block text-slate-500">
              {{ $t('newReceiving.review.acknowledgeWarningsNeedsApproval') }}
            </span>
          </span>
        </label>
      </div>
    </FormSection>

    <!-- Product-level Review -->
    <FormSection :title="$t('newReceiving.review.itemsTitle')">
      <div class="mb-4">
        <input
          v-model="searchText"
          type="text"
          class="input max-w-sm"
          :placeholder="$t('newReceiving.review.searchPlaceholder')"
        />
      </div>

      <DataTable
        :is-empty="filteredItems.length === 0"
        :empty-text="$t('newReceiving.table.noItems')"
        :colspan="11"
        min-width="1300px"
      >
        <template #head>
          <tr>
            <th class="table-th"></th>
            <th class="table-th">{{ $t('newReceiving.table.sku') }}</th>
            <th class="table-th">{{ $t('newReceiving.table.productName') }}</th>
            <th class="table-th">{{ $t('newReceiving.review.brand') }}</th>
            <th class="table-th">{{ $t('newReceiving.review.packSize') }}</th>
            <th class="table-th">{{ $t('newReceiving.review.storageType') }}</th>
            <th class="table-th text-right">{{ $t('newReceiving.table.expectedQty') }}</th>
            <th class="table-th text-right">{{ $t('newReceiving.table.receiveQty') }}</th>
            <th class="table-th text-right">{{ $t('newReceiving.review.difference') }}</th>
            <th class="table-th">{{ $t('newReceiving.review.status') }}</th>
            <th class="table-th"></th>
          </tr>
        </template>

        <template v-for="row in filteredItems" :key="row.item.id">
          <tr class="border-t hover:bg-orange-50">
            <td class="table-td">
              <button
                v-if="(row.item.splits || []).length > 0"
                @click="toggleExpanded(row.item.id)"
                class="text-slate-500 hover:text-orange-600 font-bold w-5"
              >
                {{ expanded[row.item.id] ? '▾' : '▸' }}
              </button>
            </td>
            <td class="table-td font-semibold text-slate-800">{{ row.item.sku || '—' }}</td>
            <td class="table-td">{{ row.item.productName || '—' }}</td>
            <td class="table-td">{{ row.item.brandName || '—' }}</td>
            <td class="table-td">
              <span v-if="row.item.product && row.item.product.pack_size">
                {{ row.item.product.pack_size }} {{ row.item.product.pack_size_unit || '' }}
                <span class="text-slate-400">({{ packsReceived(row.item) }} {{ $t('newReceiving.review.packs') }})</span>
              </span>
              <span v-else>—</span>
            </td>
            <td class="table-td">{{ storageTypeLabel(row.item.product) }}</td>
            <td class="table-td text-right tabular-nums">{{ row.item.expectedQty }} {{ row.item.uom }}</td>
            <td class="table-td text-right tabular-nums text-orange-600 font-semibold">{{ row.item.receiveQty }} {{ row.item.uom }}</td>
            <td
              class="table-td text-right tabular-nums font-semibold"
              :class="diff(row.item) < 0 ? 'text-red-600' : diff(row.item) > 0 ? 'text-green-600' : 'text-slate-500'"
            >
              {{ diff(row.item) }}
            </td>
            <td class="table-td">
              <StatusBadge :tone="row.badge.tone">{{ $t(row.badge.label) }}</StatusBadge>
            </td>
            <td class="table-td text-right whitespace-nowrap">
              <span v-if="(row.item.splits || []).length > 0" class="text-slate-500 text-xs">
                {{ (row.item.splits || []).length }} {{ $t('newReceiving.review.splitsLabel') }}
              </span>
            </td>
          </tr>

          <tr v-if="expanded[row.item.id] && (row.item.splits || []).length > 0" :key="'splits-' + row.item.id" class="border-t bg-slate-50">
            <td colspan="11" class="p-4">
              <div class="overflow-x-auto">
                <table class="w-full text-sm">
                  <thead class="text-slate-500">
                    <tr>
                      <th class="text-left py-2 pr-4">{{ $t('newReceiving.table.lotNo') }}</th>
                      <th class="text-left py-2 pr-4">{{ $t('newReceiving.review.mfgDate') }}</th>
                      <th class="text-left py-2 pr-4">{{ $t('newReceiving.table.expiryDate') }}</th>
                      <th class="text-left py-2 pr-4">{{ $t('newReceiving.splits.palletId') }}</th>
                      <th class="text-right py-2 pr-4">{{ $t('newReceiving.splits.qty') }}</th>
                      <th class="text-right py-2 pr-4">{{ $t('newReceiving.splits.cartonCount') }}</th>
                      <th class="text-left py-2 pr-4">{{ $t('newReceiving.table.location') }}</th>
                      <th class="text-right py-2 pr-4">{{ $t('newReceiving.review.temperature') }}</th>
                      <th class="text-left py-2 pr-4">{{ $t('newReceiving.review.countryOfOrigin') }}</th>
                      <th class="text-left py-2">{{ $t('newReceiving.review.condition') }}</th>
                    </tr>
                  </thead>
                  <tbody>
                    <tr
                      v-for="(split, sIdx) in row.item.splits"
                      :key="sIdx"
                      class="border-t"
                      :class="splitHasError(row.item.id, sIdx) ? 'bg-red-50' : splitHasWarning(row.item.id, sIdx) ? 'bg-amber-50' : ''"
                    >
                      <td class="py-2 pr-4">{{ split.lotNo || '—' }}</td>
                      <td class="py-2 pr-4">{{ split.mfgDate || '—' }}</td>
                      <td class="py-2 pr-4">{{ split.expiryDate || '—' }}</td>
                      <td class="py-2 pr-4">{{ split.palletId || '—' }}</td>
                      <td class="py-2 pr-4 text-right tabular-nums">{{ split.quantity }} {{ row.item.uom }}</td>
                      <td class="py-2 pr-4 text-right tabular-nums">{{ split.cartonCount || '—' }}</td>
                      <td class="py-2 pr-4">{{ locationCode(split.locationId) }}</td>
                      <td class="py-2 pr-4 text-right tabular-nums">{{ split.temperatureC !== null && split.temperatureC !== '' ? split.temperatureC + '°C' : '—' }}</td>
                      <td class="py-2 pr-4">{{ split.countryOfOrigin || '—' }}</td>
                      <td class="py-2">{{ split.conditionNote || '—' }}</td>
                    </tr>
                  </tbody>
                </table>
              </div>
            </td>
          </tr>
        </template>
      </DataTable>
    </FormSection>

    <!-- Sticky Footer -->
    <div class="fixed bottom-0 left-0 right-0 bg-white border-t border-slate-200 shadow-[0_-4px_12px_rgba(0,0,0,0.05)] z-40">
      <div class="max-w-7xl mx-auto px-4 md:px-8 py-4 flex flex-col md:flex-row items-center justify-between gap-3">
        <div class="flex items-center gap-4 text-sm">
          <span v-if="errors.length > 0" class="text-red-600 font-semibold">
            ⛔ {{ $t('newReceiving.review.errorCount', { count: errors.length }) }}
          </span>
          <span v-else-if="warnings.length > 0" class="text-amber-700 font-semibold">
            ⚠ {{ $t('newReceiving.review.warningCount', { count: warnings.length }) }}
          </span>
          <span v-else class="text-green-600 font-semibold">
            ✓ {{ $t('newReceiving.review.readyToConfirm') }}
          </span>
        </div>

        <div class="flex flex-wrap items-center gap-3">
          <button
            @click="$emit('edit-document')"
            class="bg-white hover:bg-slate-100 text-slate-700 border border-slate-300 px-5 py-3 rounded-xl font-semibold"
          >
            {{ $t('newReceiving.review.editDocument') }}
          </button>
          <button
            @click="$emit('edit-items')"
            class="bg-white hover:bg-slate-100 text-slate-700 border border-slate-300 px-5 py-3 rounded-xl font-semibold"
          >
            {{ $t('newReceiving.review.editItems') }}
          </button>
          <button
            @click="$emit('save-draft')"
            :disabled="submitting"
            class="bg-white hover:bg-slate-100 text-slate-700 border border-slate-300 px-5 py-3 rounded-xl font-semibold disabled:opacity-60"
          >
            {{ $t('newReceiving.actions.saveDraft') }}
          </button>

          <button
            v-if="canConfirm"
            @click="$emit('confirm')"
            :disabled="submitting || !canSubmitConfirm"
            class="bg-orange-600 hover:bg-orange-700 text-white px-6 py-3 rounded-xl font-semibold disabled:opacity-40"
          >
            {{ $t('newReceiving.actions.confirmReceiving') }}
          </button>
          <p v-else class="text-sm text-slate-500">
            {{ $t('newReceiving.actions.viewOnlyNote') }}
          </p>
        </div>
      </div>
    </div>
  </div>
</template>

<script>
import FormSection from '../../components/ui/FormSection.vue'
import DataTable from '../../components/ui/DataTable.vue'
import StatusBadge from '../../components/ui/StatusBadge.vue'
import { computeReviewTotals, validateReceivingReview } from './receivingReview'

export default {
  name: 'ReceivingReviewStep',

  components: { FormSection, DataTable, StatusBadge },

  props: {
    form: { type: Object, required: true },
    items: { type: Array, required: true },
    locations: { type: Array, default: () => [] },
    warehouseLabel: { type: String, default: '—' },
    stagingLocationLabel: { type: String, default: '—' },
    supplierDisplayName: { type: String, default: '' },
    submitting: { type: Boolean, default: false },
    canConfirm: { type: Boolean, default: false },
    canApproveExceptions: { type: Boolean, default: false }
  },

  emits: ['edit-document', 'edit-items', 'save-draft', 'confirm', 'validation-change'],

  data() {
    return {
      searchText: '',
      expanded: {},
      acknowledgeWarnings: false
    }
  },

  computed: {
    totals() {
      const totals = computeReviewTotals(this.items)
      return { ...totals, byUom: Object.values(totals.byUom) }
    },

    validation() {
      return validateReceivingReview(this.form, this.items)
    },

    issues() {
      return this.validation.issues
    },

    errors() {
      return this.validation.errors
    },

    warnings() {
      return this.validation.warnings
    },

    // Confirm is blocked on any error, always. With warnings and no
    // errors, Confirm unblocks only once a user who holds
    // receiving.approve has checked "acknowledge" — ordinary staff see
    // the checkbox disabled, matching receiving.approve's existing
    // reserved-for-supervisor-exceptions role in this codebase.
    canSubmitConfirm() {
      if (this.errors.length > 0) return false
      if (this.warnings.length > 0) return this.canApproveExceptions && this.acknowledgeWarnings
      return true
    },

    groupedIssues() {
      return this.issues.map((issue) => ({
        level: issue.level,
        message: issue.message,
        label: this.issueLabel(issue)
      }))
    },

    filteredItems() {
      const text = this.searchText.trim().toLowerCase()
      return this.items
        .map((item, index) => ({ item, index, badge: this.badgeFor(item, index) }))
        .filter(({ item }) => {
          if (!text) return true
          return (
            (item.sku || '').toLowerCase().includes(text) ||
            (item.productName || '').toLowerCase().includes(text) ||
            (item.barcode || '').toLowerCase().includes(text)
          )
        })
    }
  },

  watch: {
    warnings() {
      // A changed warning set invalidates any prior acknowledgement —
      // re-editing items after acknowledging must not silently carry
      // the old approval forward onto new/different warnings.
      this.acknowledgeWarnings = false
    }
  },

  methods: {
    toggleExpanded(id) {
      this.expanded[id] = !this.expanded[id]
    },

    diff(item) {
      return Number(item.receiveQty || 0) - Number(item.expectedQty || 0)
    },

    packsReceived(item) {
      const packSize = item.product && item.product.pack_size
      if (!packSize) return '—'
      return (Number(item.receiveQty || 0) / packSize).toFixed(2)
    },

    storageTypeLabel(product) {
      if (!product || !product.storage_type) return '—'
      return this.$t(`newReceiving.review.storageTypeOptions.${product.storage_type.toLowerCase()}`)
    },

    locationCode(id) {
      const loc = this.locations.find((l) => l.id === id)
      return loc ? loc.code : '—'
    },

    itemIssues(itemIndex) {
      return this.issues.filter((i) => i.itemIndex === itemIndex)
    },

    splitHasError(itemId, splitIndex) {
      const itemIndex = this.items.findIndex((i) => i.id === itemId)
      return this.errors.some((e) => e.itemIndex === itemIndex && e.splitIndex === splitIndex)
    },

    splitHasWarning(itemId, splitIndex) {
      const itemIndex = this.items.findIndex((i) => i.id === itemId)
      return this.warnings.some((w) => w.itemIndex === itemIndex && w.splitIndex === splitIndex)
    },

    badgeFor(item, itemIndex) {
      const itemHasError = this.errors.some((e) => e.itemIndex === itemIndex)
      const itemHasWarning = this.warnings.some((w) => w.itemIndex === itemIndex)
      if (itemHasError) return { tone: 'red', label: 'newReceiving.review.statusOptions.error' }
      if (itemHasWarning) return { tone: 'orange', label: 'newReceiving.review.statusOptions.warning' }
      if (item.product && item.product.qc_required) return { tone: 'blue', label: 'newReceiving.review.statusOptions.qcRequired' }
      return { tone: 'green', label: 'newReceiving.review.statusOptions.normal' }
    },

    issueLabel(issue) {
      const sku = (this.items[issue.itemIndex] && this.items[issue.itemIndex].sku) || ''
      if (issue.scope === 'header') return this.$t('newReceiving.review.issueScopes.header')
      if (issue.splitIndex !== null && issue.splitIndex !== undefined) {
        return this.$t('newReceiving.review.issueScopes.split', { sku, split: issue.splitIndex + 1 })
      }
      return this.$t('newReceiving.review.issueScopes.line', { sku })
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

.table-th {
  padding: 14px 16px;
  font-weight: var(--fw-table-header);
  font-size: var(--fs-table-header);
  white-space: nowrap;
}

.table-td {
  padding: 10px 12px;
  color: #475569;
  white-space: nowrap;
}

.summary-card {
  background: #f8fafc;
  border: 1px solid #e2e8f0;
  border-radius: 18px;
  padding: 16px;
}

.summary-label {
  color: #64748b;
  font-size: 13px;
}

.summary-value {
  font-size: var(--fs-kpi);
  font-weight: var(--fw-kpi);
  color: #0f172a;
  margin-top: 6px;
  font-variant-numeric: tabular-nums;
}
</style>
