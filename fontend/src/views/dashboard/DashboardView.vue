<template>
  <div>
    <div class="mb-6">
      <h1 class="text-2xl font-bold text-slate-800">
        {{ $t('dashboard.title') }}
      </h1>
      <p class="text-slate-500">
        {{ $t('dashboard.subtitle') }}
      </p>
    </div>

    <p v-if="loadError" class="text-red-600 mb-4">{{ loadError }}</p>

    <div class="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-4 gap-6">
      <div class="summary-card">
        <p class="summary-label">{{ $t('dashboard.cards.pendingReceiving') }}</p>
        <h2 class="summary-value">{{ loading ? '…' : summary.pending_receiving_count }}</h2>
        <p class="summary-note text-orange-600">{{ $t('dashboard.cards.pendingReceivingNote') }}</p>
      </div>

      <div class="summary-card">
        <p class="summary-label">{{ $t('dashboard.cards.availableStock') }}</p>
        <h2 class="summary-value">{{ loading ? '…' : formatQty(summary.available_stock_qty) }}</h2>
        <p class="summary-note text-green-600">{{ $t('dashboard.cards.availableStockNote') }}</p>
      </div>

      <div class="summary-card">
        <p class="summary-label">{{ $t('dashboard.cards.pendingPicking') }}</p>
        <h2 class="summary-value">{{ loading ? '…' : summary.pending_picking_count }}</h2>
        <p class="summary-note text-blue-600">{{ $t('dashboard.cards.pendingPickingNote') }}</p>
      </div>

      <div class="summary-card">
        <p class="summary-label">{{ $t('dashboard.cards.lowStock') }}</p>
        <h2 class="summary-value">{{ loading ? '…' : summary.low_stock_count }}</h2>
        <p class="summary-note text-red-600">{{ $t('dashboard.cards.lowStockNote') }}</p>
      </div>
    </div>

    <div class="grid grid-cols-1 xl:grid-cols-3 gap-6 mt-6">
      <div class="bg-white rounded-2xl shadow-sm border border-slate-200 p-6 xl:col-span-2">
        <h2 class="text-lg font-bold text-slate-800 mb-4">
          {{ $t('dashboard.recentInbound') }}
        </h2>

        <div class="overflow-x-auto">
          <table class="w-full text-sm">
            <thead>
              <tr class="text-left text-slate-500 border-b">
                <th class="py-3">{{ $t('dashboard.table.documentNo') }}</th>
                <th class="py-3">{{ $t('dashboard.table.vendor') }}</th>
                <th class="py-3">{{ $t('dashboard.table.status') }}</th>
                <th class="py-3">{{ $t('dashboard.table.date') }}</th>
              </tr>
            </thead>

            <tbody>
              <tr v-if="!loading && summary.recent_inbound.length === 0">
                <td colspan="4" class="py-4 text-center text-slate-400">{{ $t('dashboard.noRecentInbound') }}</td>
              </tr>
              <tr v-for="receipt in summary.recent_inbound" :key="receipt.id" class="border-b last:border-b-0">
                <td class="py-3 font-medium">{{ receipt.receipt_no }}</td>
                <td class="py-3">{{ receipt.vendor || '-' }}</td>
                <td class="py-3">
                  <span :class="statusBadgeClass(receipt.status)">{{ statusLabel(receipt.status) }}</span>
                </td>
                <td class="py-3">{{ formatDate(receipt.receiving_date) }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <div class="bg-white rounded-2xl shadow-sm border border-slate-200 p-6">
        <h2 class="text-lg font-bold text-slate-800 mb-4">
          {{ $t('dashboard.quickActions') }}
        </h2>

        <div class="space-y-3">
          <button class="quick-button" @click="$router.push('/inbound/receiving/new')">
            📥 {{ $t('dashboard.quickAction.startReceiving') }}
          </button>

          <button class="quick-button" @click="$router.push('/inventory/stock-inquiry')">
            📦 {{ $t('dashboard.quickAction.stockInquiry') }}
          </button>

          <button class="quick-button" @click="$router.push('/outbound/picking')">
            📤 {{ $t('dashboard.quickAction.createPickingTask') }}
          </button>

          <button class="quick-button" @click="$router.push('/inventory/stock-adjustment')">
            🔄 {{ $t('dashboard.quickAction.stockAdjustment') }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script>
import { getDashboardSummary } from '../../services/dashboard'
import i18n from '../../i18n'

function statusText(status) {
  const t = i18n.global.t
  switch (status) {
    case 'DRAFT': return t('dashboard.status.draft')
    case 'WAITING': return t('dashboard.status.waiting')
    case 'PARTIALLY_RECEIVED': return t('dashboard.status.partiallyReceived')
    case 'COMPLETED': return t('dashboard.status.completed')
    case 'CANCELLED': return t('dashboard.status.cancelled')
    default: return status
  }
}

export default {
  name: 'DashboardPage',
  data() {
    return {
      loading: true,
      loadError: '',
      summary: {
        pending_receiving_count: 0,
        available_stock_qty: 0,
        pending_picking_count: 0,
        low_stock_count: 0,
        recent_inbound: []
      }
    }
  },
  async mounted() {
    await this.fetchSummary()
  },
  methods: {
    async fetchSummary() {
      this.loading = true
      this.loadError = ''
      try {
        const res = await getDashboardSummary()
        this.summary = res.data
      } catch (err) {
        this.loadError = err.message || this.$t('dashboard.loadFailed')
      } finally {
        this.loading = false
      }
    },
    formatQty(value) {
      return this.$n(Number(value || 0), 'decimal')
    },
    formatDate(isoString) {
      if (!isoString) return '—'
      return this.$d(new Date(isoString), 'short')
    },
    statusLabel(status) {
      return statusText(status)
    },
    statusBadgeClass(status) {
      if (status === 'PARTIALLY_RECEIVED') return 'badge-orange'
      if (status === 'COMPLETED') return 'badge-green'
      return 'badge-gray'
    }
  }
}
</script>

<style scoped>
.summary-card {
  background: white;
  border: 1px solid #e2e8f0;
  border-radius: 18px;
  padding: 22px;
  box-shadow: 0 1px 3px rgba(15, 23, 42, 0.08);
}

.summary-label {
  color: #64748b;
  font-size: 14px;
}

.summary-value {
  font-size: var(--fs-kpi);
  font-weight: var(--fw-kpi);
  color: #0f172a;
  margin-top: 8px;
  font-variant-numeric: tabular-nums;
}

.summary-note {
  font-size: 13px;
  margin-top: 6px;
  font-weight: 600;
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

.badge-gray {
  background: #f1f5f9;
  color: #475569;
  padding: 5px 10px;
  border-radius: 999px;
  font-weight: 600;
  font-size: 12px;
}

.quick-button {
  width: 100%;
  text-align: left;
  padding: 14px 16px;
  border-radius: 14px;
  background: #f8fafc;
  color: #334155;
  font-weight: 600;
  cursor: pointer;
  transition: 0.2s;
}

.quick-button:hover {
  background: #ea580c;
  color: white;
}
</style>