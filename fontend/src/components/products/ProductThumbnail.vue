<template>
  <div class="thumb" :class="{ 'thumb--empty': !previewUrl }">
    <img v-if="previewUrl" :src="previewUrl" :alt="altText" />
    <span v-else class="thumb-placeholder" aria-hidden="true">🖼</span>
  </div>
</template>

<script>
import { fetchProductImageBlob } from '../../services/products'

// Read-only counterpart to ProductImageUpload.vue — the Product List
// table's thumbnail column needs the same authenticated-blob-fetch
// (GET /products/:id/image requires a Bearer token a plain <img src>
// can't send) but none of the upload/replace/remove controls.
export default {
  name: 'ProductThumbnail',

  props: {
    productId: { type: [String, Number], required: true },
    hasImage: { type: Boolean, default: false },
    altText: { type: String, default: '' }
  },

  data() {
    return { previewUrl: '', objectUrlToRevoke: null }
  },

  watch: {
    hasImage() {
      this.load()
    },
    productId() {
      this.load()
    }
  },

  mounted() {
    this.load()
  },

  beforeUnmount() {
    if (this.objectUrlToRevoke) URL.revokeObjectURL(this.objectUrlToRevoke)
  },

  methods: {
    async load() {
      if (this.objectUrlToRevoke) {
        URL.revokeObjectURL(this.objectUrlToRevoke)
        this.objectUrlToRevoke = null
      }
      if (!this.hasImage) {
        this.previewUrl = ''
        return
      }
      try {
        const blob = await fetchProductImageBlob(this.productId)
        const url = URL.createObjectURL(blob)
        this.objectUrlToRevoke = url
        this.previewUrl = url
      } catch {
        this.previewUrl = ''
      }
    }
  }
}
</script>

<style scoped>
.thumb {
  width: 44px;
  height: 44px;
  border-radius: 10px;
  border: 1px solid #e2e8f0;
  background: #f8fafc;
  overflow: hidden;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}

.thumb img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.thumb-placeholder {
  font-size: 1.1rem;
  opacity: 0.4;
}
</style>
