<template>
  <div v-if="!confirming" class="fixed inset-0 bg-black/40 flex items-center justify-center z-50 p-4">
    <div class="bg-white rounded-2xl shadow-xl w-full max-w-md p-6 space-y-4">
      <div class="flex items-center justify-between">
        <h2 class="text-lg font-bold text-slate-800">{{ $t('platformAdmin.status.changeTitle') }}</h2>
        <button @click="$emit('close')" class="text-slate-400 hover:text-slate-600">✕</button>
      </div>

      <div>
        <label class="label">{{ $t('platformAdmin.status.currentStatus') }}</label>
        <p class="font-semibold text-slate-700">{{ $t('platformAdmin.status.' + tenant.status.toLowerCase()) }}</p>
      </div>

      <div>
        <label class="label">{{ $t('platformAdmin.status.newStatus') }}</label>
        <select v-model="newStatus" class="input">
          <option v-for="code in TENANT_STATUSES" :key="code" :value="code">
            {{ $t('platformAdmin.status.' + code.toLowerCase()) }}
          </option>
        </select>
      </div>

      <div>
        <label class="label">{{ $t('platformAdmin.status.reasonLabel') }}</label>
        <textarea
          v-model="reason"
          class="input"
          rows="3"
          :placeholder="$t('platformAdmin.status.reasonPlaceholder')"
        ></textarea>
      </div>

      <div v-if="statusError" class="bg-red-50 border border-red-200 text-red-600 rounded-xl p-3 text-sm">
        {{ statusError }}
      </div>

      <div class="flex gap-3">
        <button
          @click="$emit('close')"
          class="flex-1 bg-slate-100 hover:bg-slate-200 text-slate-700 py-3 rounded-xl font-semibold"
        >
          {{ $t('platformAdmin.status.cancelButton') }}
        </button>
        <button
          @click="onContinue"
          :disabled="newStatus === tenant.status"
          class="flex-1 bg-orange-600 hover:bg-orange-700 text-white py-3 rounded-xl font-semibold disabled:opacity-60"
        >
          {{ $t('platformAdmin.status.updateButton') }}
        </button>
      </div>
    </div>
  </div>

  <!-- Confirmation step: only reached for SUSPENDED/CANCELLED, per
       Phase 7's explicit requirement — every other transition
       (TRIAL/ACTIVE) applies immediately from the step above. Reuses
       the shared ConfirmDialog since this step is a pure confirm/cancel
       action with no extra fields, same shape ConfirmDialog was
       generalized from. -->
  <ConfirmDialog
    v-else
    :open="true"
    :title="$t('platformAdmin.status.changeTitle')"
    :message="newStatus === 'CANCELLED'
      ? $t('platformAdmin.status.confirmCancelMessage')
      : $t('platformAdmin.status.confirmSuspendMessage')"
    :confirm-label="$t('platformAdmin.status.confirmButton')"
    :cancel-label="$t('platformAdmin.status.cancelButton')"
    :loading-label="$t('platformAdmin.status.updating')"
    danger
    :loading="submitting"
    :error-message="statusError"
    @confirm="submit"
    @cancel="confirming = false"
  />
</template>

<script>
import { updateTenantStatus, TENANT_STATUSES } from '../../services/platformTenants'
import ConfirmDialog from '../../components/ui/ConfirmDialog.vue'

export default {
  name: 'TenantStatusModal',

  components: { ConfirmDialog },

  props: {
    tenant: {
      type: Object,
      required: true
    }
  },

  emits: ['close', 'updated'],

  data() {
    return {
      TENANT_STATUSES,
      newStatus: this.tenant.status,
      reason: '',
      confirming: false,
      submitting: false,
      statusError: ''
    }
  },

  methods: {
    onContinue() {
      this.statusError = ''
      // SUSPENDED/CANCELLED are consequential (block user access /
      // significant lifecycle change) and always get a confirmation
      // step; TRIAL/ACTIVE apply immediately — matching Phase 7's
      // "confirmation dialog for suspension/cancellation" requirement
      // precisely rather than confirming every transition uniformly.
      if (this.newStatus === 'SUSPENDED' || this.newStatus === 'CANCELLED') {
        this.confirming = true
        return
      }
      this.submit()
    },

    async submit() {
      this.statusError = ''
      this.submitting = true
      try {
        const res = await updateTenantStatus(this.tenant.id, {
          status: this.newStatus,
          ...(this.reason.trim() ? { reason: this.reason.trim() } : {})
        })
        this.$emit('updated', res.data)
      } catch (err) {
        this.statusError = err.message || this.$t('platformAdmin.status.messages.updateFailed')
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
  font-weight: var(--fw-label);
  font-size: var(--fs-label);
}
</style>
