<template>
  <div class="fixed inset-0 bg-black/40 flex items-center justify-center z-50 p-4">
    <div class="bg-white rounded-2xl shadow-xl w-full max-w-md p-6 space-y-4">
      <div class="flex items-center justify-between">
        <h2 class="text-lg font-bold text-slate-800">
          {{ isSuspending ? $t('platformAdmin.tenantUsers.changeStatus.suspendTitle') : $t('platformAdmin.tenantUsers.changeStatus.reactivateTitle') }}
        </h2>
        <button @click="$emit('close')" class="text-slate-400 hover:text-slate-600">✕</button>
      </div>

      <div>
        <p class="font-semibold text-slate-700">{{ member.full_name }}</p>
        <p class="text-xs text-slate-400">{{ member.username }} &middot; {{ member.email }}</p>
      </div>

      <!-- Suspending is consequential (revokes tenant access immediately)
           and always gets a confirmation step; reactivating applies from
           this same panel without a second step — mirroring
           TenantStatusModal's asymmetric confirm-step precedent. -->
      <template v-if="isSuspending && !confirming">
        <p class="text-sm text-slate-600">{{ $t('platformAdmin.tenantUsers.changeStatus.suspendDescription') }}</p>

        <div>
          <label class="label">{{ $t('platformAdmin.tenantUsers.changeStatus.reasonLabel') }}</label>
          <textarea
            v-model="reason"
            class="input"
            rows="3"
            :placeholder="$t('platformAdmin.tenantUsers.changeStatus.reasonPlaceholder')"
          ></textarea>
        </div>

        <div v-if="statusError" class="bg-red-50 border border-red-200 text-red-600 rounded-xl p-3 text-sm">
          {{ statusError }}
        </div>

        <div class="flex gap-3">
          <button @click="$emit('close')" class="flex-1 bg-slate-100 hover:bg-slate-200 text-slate-700 py-3 rounded-xl font-semibold">
            {{ $t('common.buttons.cancel') }}
          </button>
          <button @click="confirming = true" class="flex-1 bg-red-600 hover:bg-red-700 text-white py-3 rounded-xl font-semibold">
            {{ $t('platformAdmin.tenantUsers.changeStatus.continueButton') }}
          </button>
        </div>
      </template>

      <template v-else-if="isSuspending && confirming">
        <p class="text-sm text-slate-600">{{ $t('platformAdmin.tenantUsers.changeStatus.confirmSuspendMessage') }}</p>

        <div v-if="statusError" class="bg-red-50 border border-red-200 text-red-600 rounded-xl p-3 text-sm">
          {{ statusError }}
        </div>

        <div class="flex gap-3">
          <button @click="confirming = false" class="flex-1 bg-slate-100 hover:bg-slate-200 text-slate-700 py-3 rounded-xl font-semibold">
            {{ $t('common.buttons.cancel') }}
          </button>
          <button @click="submit" :disabled="submitting" class="flex-1 bg-red-600 hover:bg-red-700 text-white py-3 rounded-xl font-semibold disabled:opacity-60">
            {{ submitting ? $t('platformAdmin.tenantUsers.changeStatus.updating') : $t('platformAdmin.tenantUsers.changeStatus.confirmSuspendButton') }}
          </button>
        </div>
      </template>

      <template v-else>
        <p class="text-sm text-slate-600">{{ $t('platformAdmin.tenantUsers.changeStatus.reactivateDescription') }}</p>

        <div v-if="statusError" class="bg-red-50 border border-red-200 text-red-600 rounded-xl p-3 text-sm">
          {{ statusError }}
        </div>

        <div class="flex gap-3">
          <button @click="$emit('close')" class="flex-1 bg-slate-100 hover:bg-slate-200 text-slate-700 py-3 rounded-xl font-semibold">
            {{ $t('common.buttons.cancel') }}
          </button>
          <button @click="submit" :disabled="submitting" class="flex-1 bg-orange-600 hover:bg-orange-700 text-white py-3 rounded-xl font-semibold disabled:opacity-60">
            {{ submitting ? $t('platformAdmin.tenantUsers.changeStatus.updating') : $t('platformAdmin.tenantUsers.changeStatus.reactivateButton') }}
          </button>
        </div>
      </template>
    </div>
  </div>
</template>

<script>
import { changeTenantUserStatus } from '../../services/platformTenantUsers'

export default {
  name: 'MembershipStatusDialog',

  props: {
    tenantId: {
      type: [String, Number],
      required: true
    },
    member: {
      type: Object,
      required: true
    }
  },

  emits: ['close', 'updated'],

  data() {
    return {
      reason: '',
      confirming: false,
      submitting: false,
      statusError: ''
    }
  },

  computed: {
    isSuspending() {
      return this.member.status !== 'DISABLED'
    },
    targetStatus() {
      return this.isSuspending ? 'DISABLED' : 'ACTIVE'
    }
  },

  methods: {
    async submit() {
      this.statusError = ''
      this.submitting = true
      try {
        const res = await changeTenantUserStatus(this.tenantId, this.member.user_id, {
          status: this.targetStatus,
          ...(this.reason.trim() ? { reason: this.reason.trim() } : {})
        })
        this.$emit('updated', res.data)
      } catch (err) {
        if (err.status === 409) {
          this.statusError = this.$t('platformAdmin.tenantUsers.changeStatus.messages.lastAdmin')
        } else {
          this.statusError = err.message || this.$t('platformAdmin.tenantUsers.changeStatus.messages.updateFailed')
        }
        this.confirming = false
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
</style>
