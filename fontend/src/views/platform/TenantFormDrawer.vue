<template>
  <div class="fixed inset-0 bg-black/40 flex justify-end z-50">
    <div class="w-full max-w-lg bg-white h-full p-6 overflow-y-auto">
      <div class="flex items-center justify-between mb-6">
        <div>
          <h2 class="text-xl font-bold text-slate-800">
            {{ tenant ? $t('platformAdmin.form.editTitle') : $t('platformAdmin.form.newTitle') }}
          </h2>
          <p class="text-slate-500">{{ $t('platformAdmin.form.subtitle') }}</p>
        </div>

        <button @click="$emit('close')" class="text-slate-500 hover:text-red-600 text-xl">
          ✕
        </button>
      </div>

      <div class="space-y-4">
        <div>
          <label class="label">{{ $t('platformAdmin.form.fields.name') }}</label>
          <input
            v-model="form.name"
            type="text"
            class="input"
            :placeholder="$t('platformAdmin.form.fields.namePlaceholder')"
          />
        </div>

        <div>
          <label class="label">{{ $t('platformAdmin.form.fields.slug') }}</label>
          <input
            v-model="form.slug"
            type="text"
            class="input"
            :placeholder="$t('platformAdmin.form.fields.slugPlaceholder')"
          />
          <p class="text-xs text-slate-400 mt-1">{{ $t('platformAdmin.form.fields.slugHint') }}</p>
        </div>

        <!-- Only offered on create: status transitions after creation
             always go through the dedicated status-change flow
             (TenantStatusModal), which stamps suspended_at/cancelled_at
             and writes its own audit action — matching the backend's
             own UpdateRequest, which deliberately excludes status. -->
        <div v-if="!tenant">
          <label class="label">{{ $t('platformAdmin.form.fields.initialStatus') }}</label>
          <select v-model="form.status" class="input">
            <option v-for="code in TENANT_STATUSES" :key="code" :value="code">
              {{ $t('platformAdmin.status.' + code.toLowerCase()) }}
            </option>
          </select>
        </div>

        <div v-if="formError" class="bg-red-50 border border-red-200 text-red-600 rounded-xl p-3 text-sm">
          {{ formError }}
        </div>

        <div class="flex gap-3 mt-4">
          <button
            @click="$emit('close')"
            class="flex-1 bg-slate-100 hover:bg-slate-200 text-slate-700 py-3 rounded-xl font-semibold"
          >
            {{ $t('platformAdmin.form.cancelButton') }}
          </button>
          <button
            @click="save"
            :disabled="saving"
            class="flex-1 bg-orange-600 hover:bg-orange-700 text-white py-3 rounded-xl font-semibold disabled:opacity-60"
          >
            {{ saving ? $t('platformAdmin.form.saving') : $t('platformAdmin.form.saveButton') }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script>
import { createTenant, updateTenant, TENANT_STATUSES } from '../../services/platformTenants'

export default {
  name: 'TenantFormDrawer',

  props: {
    // null => create; a Response object ({id, slug, name, status, ...})
    // => edit.
    tenant: {
      type: Object,
      default: null
    }
  },

  emits: ['close', 'saved'],

  data() {
    return {
      TENANT_STATUSES,
      saving: false,
      formError: '',
      form: this.tenant
        ? { name: this.tenant.name, slug: this.tenant.slug }
        : { name: '', slug: '', status: 'TRIAL' }
    }
  },

  methods: {
    async save() {
      this.formError = ''

      const name = this.form.name.trim()
      if (!name) {
        this.formError = this.$t('platformAdmin.form.messages.nameRequired')
        return
      }
      const slug = this.form.slug.trim()
      if (!slug) {
        this.formError = this.$t('platformAdmin.form.messages.slugRequired')
        return
      }

      this.saving = true
      try {
        const res = this.tenant
          ? await updateTenant(this.tenant.id, { name, slug })
          : await createTenant({ name, slug, status: this.form.status })
        this.$emit('saved', res.data)
      } catch (err) {
        if (err.status === 409) {
          this.formError = this.$t('platformAdmin.form.messages.duplicateSlug')
        } else {
          this.formError = err.message || this.$t('platformAdmin.form.messages.saveFailed')
        }
      } finally {
        this.saving = false
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
