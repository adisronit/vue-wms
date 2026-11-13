<template>
  <div class="image-upload">
    <div class="preview" :class="{ 'preview--empty': !previewUrl }">
      <img v-if="previewUrl" :src="previewUrl" :alt="altText" />
      <span v-else class="placeholder">{{ $t('productMaster.image.placeholder') }}</span>
    </div>

    <div class="controls">
      <input
        ref="fileInput"
        type="file"
        accept="image/jpeg,image/png,image/webp"
        class="hidden-input"
        :disabled="disabled || uploading || removing"
        @change="onFileChange"
      />
      <button
        type="button"
        class="btn btn--secondary"
        :disabled="disabled || uploading || removing"
        @click="$refs.fileInput.click()"
      >
        {{ uploading ? $t('productMaster.image.uploading') : previewUrl ? $t('productMaster.image.replaceButton') : $t('productMaster.image.uploadButton') }}
      </button>
      <button
        v-if="previewUrl && !pendingFile"
        type="button"
        class="btn btn--danger"
        :disabled="disabled || uploading || removing"
        @click="removeImage"
      >
        {{ removing ? $t('productMaster.image.removing') : $t('productMaster.image.removeButton') }}
      </button>

      <p class="help">{{ $t('productMaster.image.help') }}</p>
      <p v-if="errorMessage" class="error">{{ errorMessage }}</p>
    </div>
  </div>
</template>

<script>
import { uploadProductImage, removeProductImage, fetchProductImageBlob } from '../../services/products'

const ALLOWED_TYPES = ['image/jpeg', 'image/png', 'image/webp']
const MAX_BYTES = 5 * 1024 * 1024

// Self-contained product image widget covering both the Add flow
// (productId is null — a selected file is held as `pendingFile` and
// previewed locally via a blob: URL, with no server round trip until
// the caller invokes uploadPendingFile(newId) once the product itself
// has been created) and the Edit flow (productId is already known — a
// selected file uploads immediately, matching "real persistent upload"
// rather than a draft state that could be lost). The server is always
// the real validator (content-sniffed MIME, 5MB cap); the client-side
// checks here exist only to fail fast without a round trip.
export default {
  name: 'ProductImageUpload',

  props: {
    productId: { type: [String, Number], default: null },
    hasImage: { type: Boolean, default: false },
    altText: { type: String, default: '' },
    disabled: { type: Boolean, default: false }
  },

  emits: ['uploaded', 'removed', 'error'],

  data() {
    return {
      previewUrl: '',
      pendingFile: null,
      uploading: false,
      removing: false,
      errorMessage: '',
      objectUrlToRevoke: null
    }
  },

  watch: {
    productId() {
      this.loadExistingImage()
    },
    hasImage() {
      this.loadExistingImage()
    }
  },

  mounted() {
    this.loadExistingImage()
  },

  beforeUnmount() {
    this.releaseObjectUrl()
  },

  methods: {
    releaseObjectUrl() {
      if (this.objectUrlToRevoke) {
        URL.revokeObjectURL(this.objectUrlToRevoke)
        this.objectUrlToRevoke = null
      }
    },

    async loadExistingImage() {
      this.releaseObjectUrl()
      this.pendingFile = null
      if (!this.productId || !this.hasImage) {
        this.previewUrl = ''
        return
      }
      try {
        const blob = await fetchProductImageBlob(this.productId)
        const url = URL.createObjectURL(blob)
        this.objectUrlToRevoke = url
        this.previewUrl = url
      } catch {
        // A product with has_image=true whose file can't be fetched
        // (e.g. a stale cross-tenant edge case) falls back to the
        // placeholder rather than a broken-image icon.
        this.previewUrl = ''
      }
    },

    validate(file) {
      if (!ALLOWED_TYPES.includes(file.type)) {
        return this.$t('productMaster.image.errors.invalidType')
      }
      if (file.size > MAX_BYTES) {
        return this.$t('productMaster.image.errors.tooLarge')
      }
      return ''
    },

    onFileChange(event) {
      const file = event.target.files && event.target.files[0]
      event.target.value = ''
      if (!file) return

      const validationError = this.validate(file)
      if (validationError) {
        this.errorMessage = validationError
        return
      }
      this.errorMessage = ''

      if (!this.productId) {
        // Add flow: no product exists yet to attach this to. Preview
        // locally; the parent view must call uploadPendingFile(newId)
        // once it has created the product and knows its real ID.
        this.releaseObjectUrl()
        const url = URL.createObjectURL(file)
        this.objectUrlToRevoke = url
        this.previewUrl = url
        this.pendingFile = file
        return
      }

      this.doUpload(this.productId, file)
    },

    async doUpload(targetProductId, file) {
      this.uploading = true
      this.errorMessage = ''
      try {
        const res = await uploadProductImage(targetProductId, file)
        this.releaseObjectUrl()
        const url = URL.createObjectURL(file)
        this.objectUrlToRevoke = url
        this.previewUrl = url
        this.pendingFile = null
        this.$emit('uploaded', res.data)
      } catch (err) {
        this.errorMessage = err.message || this.$t('productMaster.image.errors.uploadFailed')
        this.$emit('error', this.errorMessage)
      } finally {
        this.uploading = false
      }
    },

    // Called by the parent view after a successful Create, when this
    // component was used in Add mode (productId was null while the
    // user picked a file).
    async uploadPendingFile(newProductId) {
      if (!this.pendingFile) return
      await this.doUpload(newProductId, this.pendingFile)
    },

    async removeImage() {
      if (!this.productId) return
      this.removing = true
      this.errorMessage = ''
      try {
        const res = await removeProductImage(this.productId)
        this.releaseObjectUrl()
        this.previewUrl = ''
        this.$emit('removed', res.data)
      } catch (err) {
        this.errorMessage = err.message || this.$t('productMaster.image.errors.removeFailed')
        this.$emit('error', this.errorMessage)
      } finally {
        this.removing = false
      }
    }
  }
}
</script>

<style scoped>
.image-upload {
  display: flex;
  gap: 16px;
  align-items: flex-start;
}

.preview {
  width: 120px;
  height: 120px;
  border-radius: 14px;
  border: 1px solid #e2e8f0;
  background: #f8fafc;
  overflow: hidden;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}

.preview img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.placeholder {
  color: #94a3b8;
  font-size: 0.75rem;
  text-align: center;
  padding: 0 10px;
}

.controls {
  flex: 1;
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 10px;
}

.hidden-input {
  display: none;
}

.btn {
  padding: 9px 16px;
  border-radius: 10px;
  font-weight: 600;
  font-size: 0.85rem;
  cursor: pointer;
  border: 1px solid transparent;
}

.btn:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}

.btn--secondary {
  background: #f1f5f9;
  color: #334155;
}

.btn--secondary:hover:not(:disabled) {
  background: #e2e8f0;
}

.btn--danger {
  background: #fff;
  color: #dc2626;
  border-color: #fecaca;
}

.btn--danger:hover:not(:disabled) {
  background: #fef2f2;
}

.help {
  flex-basis: 100%;
  color: #94a3b8;
  font-size: 0.75rem;
  margin: 0;
}

.error {
  flex-basis: 100%;
  color: #dc2626;
  font-size: 0.8rem;
  margin: 0;
}
</style>
