<template>
  <div class="space-y-6">
    <PageHeader :title="$t('pickingBatches.title')" :subtitle="$t('pickingBatches.subtitle')">
      <template #actions>
        <button
          v-if="canCreate"
          @click="$router.push('/outbound/picking-batches/new')"
          class="bg-orange-600 hover:bg-orange-700 text-white px-5 py-3 rounded-xl font-semibold"
        >
          {{ $t('pickingBatches.newButton') }}
        </button>
      </template>
    </PageHeader>

    <div v-if="loadError" class="bg-red-50 border border-red-200 text-red-600 rounded-xl p-3 text-sm">
      {{ loadError }}
    </div>

    <FilterBar>
      <select v-model="filters.status" class="input" @change="fetchBatches">
        <option value="">{{ $t('pickingBatches.filters.allStatus') }}</option>
        <option value="DRAFT">{{ $t('pickingBatches.statusLabels.draft') }}</option>
        <option value="CONFIRMED">{{ $t('pickingBatches.statusLabels.confirmed') }}</option>
        <option value="COMPLETED">{{ $t('pickingBatches.statusLabels.completed') }}</option>
        <option value="CANCELLED">{{ $t('pickingBatches.statusLabels.cancelled') }}</option>
      </select>

      <select v-model="filters.warehouseId" class="input" @change="fetchBatches">
        <option value="">{{ $t('pickingBatches.filters.allWarehouses') }}</option>
        <option v-for="w in warehouses" :key="w.id" :value="w.id">{{ w.code }}</option>
      </select>

      <button @click="clearFilter" class="bg-slate-100 hover:bg-slate-200 text-slate-700 rounded-xl font-semibold">
        {{ $t('common.buttons.clearFilter') }}
      </button>
    </FilterBar>

    <DataTable
      :loading="loading"
      :is-empty="batchList.length === 0"
      :loading-text="$t('pickingBatches.loading')"
      :empty-text="$t('pickingBatches.noBatches')"
      :colspan="8"
      min-width="950px"
    >
      <template #head>
        <tr>
          <th class="table-th">{{ $t('pickingBatches.table.batchNo') }}</th>
          <th class="table-th">{{ $t('pickingBatches.table.warehouse') }}</th>
          <th class="table-th text-right">{{ $t('pickingBatches.table.orders') }}</th>
          <th class="table-th text-right">{{ $t('pickingBatches.table.lines') }}</th>
          <th class="table-th">{{ $t('pickingBatches.table.pickNo') }}</th>
          <th class="table-th">{{ $t('pickingBatches.table.status') }}</th>
          <th class="table-th">{{ $t('pickingBatches.table.createdAt') }}</th>
          <th class="table-th text-right">{{ $t('pickingBatches.table.action') }}</th>
        </tr>
      </template>

      <tr v-for="b in batchList" :key="b.id" class="border-t hover:bg-orange-50">
        <td class="table-td font-semibold text-slate-800">{{ b.batch_no }}</td>
        <td class="table-td">{{ b.warehouse_code }}</td>
        <td class="table-td text-right">{{ b.order_count }}</td>
        <td class="table-td text-right">{{ b.line_count }}</td>
        <td class="table-td">{{ b.pick_no || '—' }}</td>
        <td class="table-td"><StatusBadge :tone="statusTone(b.status)">{{ statusLabel(b.status) }}</StatusBadge></td>
        <td class="table-td">{{ formatDate(b.created_at) }}</td>
        <td class="table-td text-right">
          <button @click="$router.push(`/outbound/picking-batches/${b.id}`)" class="text-orange-600 hover:underline font-semibold">
            {{ $t('common.buttons.view') }}
          </button>
        </td>
      </tr>
    </DataTable>
  </div>
</template>

<script>
import { listPickingBatches } from '../../services/pickingBatches'
import { listWarehouses } from '../../services/warehouses'
import { hasPermission } from '../../services/auth'
import PageHeader from '../../components/ui/PageHeader.vue'
import FilterBar from '../../components/ui/FilterBar.vue'
import DataTable from '../../components/ui/DataTable.vue'
import StatusBadge from '../../components/ui/StatusBadge.vue'

export default {
  name: 'PickingBatchPage',
  components: { PageHeader, FilterBar, DataTable, StatusBadge },
  data() {
    return {
      loading: false,
      loadError: '',
      batchList: [],
      warehouses: [],
      filters: { status: '', warehouseId: '' }
    }
  },
  computed: {
    canCreate() { return hasPermission('picking_batch.create') }
  },
  async mounted() {
    await this.fetchWarehouses()
    await this.fetchBatches()
  },
  methods: {
    async fetchWarehouses() {
      try {
        const res = await listWarehouses({ limit: 100 })
        this.warehouses = res.data
      } catch (err) {
        this.loadError = err.message || this.$t('pickingBatches.messages.loadWarehousesFailed')
      }
    },

    async fetchBatches() {
      this.loading = true
      this.loadError = ''
      try {
        const res = await listPickingBatches(this.filters)
        this.batchList = res.data
      } catch (err) {
        this.loadError = err.message || this.$t('pickingBatches.messages.loadFailed')
      } finally {
        this.loading = false
      }
    },

    clearFilter() {
      this.filters = { status: '', warehouseId: '' }
      this.fetchBatches()
    },

    formatDate(value) {
      if (!value) return '—'
      return this.$d(new Date(value), 'long')
    },

    statusTone(status) {
      if (status === 'COMPLETED') return 'green'
      if (status === 'CONFIRMED') return 'orange'
      if (status === 'CANCELLED') return 'red'
      return 'gray'
    },

    statusLabel(status) {
      switch (status) {
        case 'DRAFT': return this.$t('pickingBatches.statusLabels.draft')
        case 'CONFIRMED': return this.$t('pickingBatches.statusLabels.confirmed')
        case 'COMPLETED': return this.$t('pickingBatches.statusLabels.completed')
        case 'CANCELLED': return this.$t('pickingBatches.statusLabels.cancelled')
        default: return status
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

.table-th {
  padding: 14px 16px;
  font-weight: var(--fw-table-header);
  font-size: var(--fs-table-header);
  white-space: nowrap;
}

.table-td {
  padding: 14px 16px;
  color: #475569;
  white-space: nowrap;
}
</style>
