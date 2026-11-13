<template>
  <div class="space-y-6">
    <div class="flex items-center justify-between">
      <router-link to="/platform/tenants" class="text-slate-500 hover:text-orange-600 text-sm font-semibold">
        &larr; {{ $t('platformAdmin.detail.backButton') }}
      </router-link>
    </div>

    <div v-if="loadError" class="bg-red-50 border border-red-200 text-red-600 rounded-xl p-4 text-sm">
      {{ loadError }}
    </div>

    <div v-else-if="loading" class="text-center py-16 text-slate-400">
      {{ $t('platformAdmin.tenants.table.loading') }}
    </div>

    <template v-else-if="tenant">
      <div class="bg-white rounded-2xl border border-slate-200 p-6">
        <PageHeader :title="tenant.name" :subtitle="tenant.slug">
          <template #actions>
            <StatusBadge :tone="statusTone(tenant.status)">
              {{ $t('platformAdmin.status.' + tenant.status.toLowerCase()) }}
            </StatusBadge>
            <button
              @click="showForm = true"
              class="bg-slate-100 hover:bg-slate-200 text-slate-700 px-4 py-2 rounded-xl font-semibold"
            >
              {{ $t('platformAdmin.detail.editButton') }}
            </button>
            <button
              @click="showStatus = true"
              class="bg-orange-600 hover:bg-orange-700 text-white px-4 py-2 rounded-xl font-semibold"
            >
              {{ $t('platformAdmin.detail.changeStatusButton') }}
            </button>
          </template>
        </PageHeader>
      </div>

      <div class="bg-white rounded-2xl border border-slate-200 p-2 flex gap-2 w-fit">
        <button @click="activeTab = 'overview'" :class="tabClass('overview')">
          {{ $t('platformAdmin.detail.tabs.overview') }}
        </button>
        <button @click="activeTab = 'users'" :class="tabClass('users')">
          {{ $t('platformAdmin.detail.tabs.users') }}
        </button>
      </div>

      <div v-if="activeTab === 'overview'" class="bg-white rounded-2xl border border-slate-200 p-6">
        <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
          <div>
            <p class="detail-label">{{ $t('platformAdmin.detail.fields.slug') }}</p>
            <p class="detail-value">{{ tenant.slug }}</p>
          </div>

          <div>
            <p class="detail-label">{{ $t('platformAdmin.detail.fields.status') }}</p>
            <p class="detail-value">
              <StatusBadge :tone="statusTone(tenant.status)">
                {{ $t('platformAdmin.status.' + tenant.status.toLowerCase()) }}
              </StatusBadge>
            </p>
          </div>

          <div>
            <p class="detail-label">{{ $t('platformAdmin.detail.fields.createdAt') }}</p>
            <p class="detail-value">{{ $d(new Date(tenant.created_at), 'long') }}</p>
          </div>

          <div>
            <p class="detail-label">{{ $t('platformAdmin.detail.fields.updatedAt') }}</p>
            <p class="detail-value">{{ $d(new Date(tenant.updated_at), 'long') }}</p>
          </div>

          <div v-if="tenant.suspended_at">
            <p class="detail-label">{{ $t('platformAdmin.detail.fields.suspendedAt') }}</p>
            <p class="detail-value">{{ $d(new Date(tenant.suspended_at), 'long') }}</p>
          </div>

          <div v-if="tenant.cancelled_at">
            <p class="detail-label">{{ $t('platformAdmin.detail.fields.cancelledAt') }}</p>
            <p class="detail-value">{{ $d(new Date(tenant.cancelled_at), 'long') }}</p>
          </div>
        </div>
      </div>

      <TenantUserList v-else-if="activeTab === 'users'" :tenant-id="tenant.id" />
    </template>

    <TenantFormDrawer
      v-if="showForm && tenant"
      :tenant="tenant"
      @close="showForm = false"
      @saved="onSaved"
    />

    <TenantStatusModal
      v-if="showStatus && tenant"
      :tenant="tenant"
      @close="showStatus = false"
      @updated="onStatusUpdated"
    />
  </div>
</template>

<script>
import { getTenant } from '../../services/platformTenants'
import TenantFormDrawer from './TenantFormDrawer.vue'
import TenantStatusModal from './TenantStatusModal.vue'
import TenantUserList from './TenantUserList.vue'
import PageHeader from '../../components/ui/PageHeader.vue'
import StatusBadge from '../../components/ui/StatusBadge.vue'

export default {
  name: 'TenantDetailView',

  components: { TenantFormDrawer, TenantStatusModal, TenantUserList, PageHeader, StatusBadge },

  data() {
    return {
      tenant: null,
      loading: false,
      loadError: '',
      showForm: false,
      showStatus: false,
      activeTab: 'overview'
    }
  },

  mounted() {
    this.fetchTenant()
  },

  methods: {
    async fetchTenant() {
      this.loading = true
      this.loadError = ''
      try {
        const res = await getTenant(this.$route.params.id)
        this.tenant = res.data
      } catch (err) {
        if (err.status === 404) {
          this.loadError = this.$t('platformAdmin.detail.notFound')
        } else {
          this.loadError = err.message || this.$t('platformAdmin.detail.loadFailed')
        }
      } finally {
        this.loading = false
      }
    },

    onSaved(savedTenant) {
      this.showForm = false
      this.tenant = savedTenant
    },

    onStatusUpdated(updatedTenant) {
      this.showStatus = false
      this.tenant = updatedTenant
    },

    statusTone(status) {
      if (status === 'ACTIVE') return 'green'
      if (status === 'TRIAL') return 'orange'
      if (status === 'SUSPENDED') return 'gray'
      if (status === 'CANCELLED') return 'red'
      return 'gray'
    },

    tabClass(tab) {
      return this.activeTab === tab
        ? 'px-4 py-2 rounded-xl font-semibold text-sm bg-orange-600 text-white'
        : 'px-4 py-2 rounded-xl font-semibold text-sm text-slate-600 hover:bg-slate-100'
    }
  }
}
</script>

<style scoped>
.detail-label {
  font-size: 12px;
  color: #94a3b8;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.5px;
  margin-bottom: 4px;
}

.detail-value {
  font-size: 15px;
  color: #334155;
  font-weight: 600;
}
</style>
