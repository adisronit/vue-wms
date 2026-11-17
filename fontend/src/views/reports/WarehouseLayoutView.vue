<template>
  <div>
    <PageHeader :title="$t('warehouseLayout.title')" :subtitle="$t('reports.sectionTitle')" />

    <div v-if="loadError" class="bg-red-50 border border-red-200 text-red-700 rounded-2xl p-4 mb-6">
      {{ loadError }}
    </div>

    <div class="bg-white rounded-2xl border border-slate-200 p-4 mb-4 flex flex-wrap items-end gap-4">
      <label class="text-sm">
        <span class="block text-xs font-semibold text-slate-500 mb-1">{{ $t('common.labels.warehouse') }}</span>
        <select v-model="selectedWarehouseId" class="field-select" @change="onWarehouseChange">
          <option value="">{{ $t('common.labels.selectOption') }}</option>
          <option v-for="wh in warehouses" :key="wh.id" :value="wh.id">{{ wh.name }}</option>
        </select>
      </label>

      <button v-if="canEdit && !isEditing" @click="startEditing" class="btn-primary">
        {{ $t('warehouseLayout.actions.edit') }}
      </button>
      <template v-if="isEditing">
        <button @click="onSave" :disabled="saving" class="btn-primary">
          {{ saving ? $t('warehouseLayout.actions.saving') : $t('warehouseLayout.actions.save') }}
        </button>
        <button @click="cancelEditing" class="btn-secondary">{{ $t('common.buttons.cancel') }}</button>
      </template>

      <button @click="showVersions = !showVersions" class="btn-secondary">
        {{ $t('warehouseLayout.actions.history') }}
      </button>
      <button @click="onPreviewPdf" class="btn-secondary">{{ $t('reports.export.previewPdf') }}</button>
      <button @click="onDownloadPdf" class="btn-secondary">{{ $t('reports.export.downloadPdf') }}</button>

      <span v-if="viewingVersionId" class="text-xs text-orange-700 bg-orange-50 border border-orange-200 rounded-full px-3 py-1">
        {{ $t('warehouseLayout.viewingHistoricalVersion', { n: viewingVersionNo }) }}
        <button class="underline ml-1" @click="returnToLatest">{{ $t('warehouseLayout.actions.backToLatest') }}</button>
      </span>
    </div>

    <div v-if="!selectedWarehouseId" class="bg-slate-50 rounded-2xl p-8 text-center text-slate-500">
      {{ $t('warehouseLayout.selectWarehousePrompt') }}
    </div>

    <div v-else class="grid grid-cols-1 lg:grid-cols-4 gap-4">
      <!-- Toolbar / property panel -->
      <div v-if="isEditing" class="lg:col-span-1 space-y-4">
        <div class="bg-white rounded-2xl border border-slate-200 p-4">
          <h3 class="text-sm font-semibold text-slate-600 mb-2">{{ $t('warehouseLayout.toolbar.addShape') }}</h3>
          <div class="grid grid-cols-2 gap-2">
            <button v-for="t in objectTypes" :key="t" @click="addShape(t)" class="text-xs px-2 py-2 rounded-lg bg-slate-100 hover:bg-slate-200">
              {{ $t('warehouseLayout.objectTypes.' + t) }}
            </button>
          </div>
        </div>

        <div class="bg-white rounded-2xl border border-slate-200 p-4">
          <h3 class="text-sm font-semibold text-slate-600 mb-2">{{ $t('warehouseLayout.toolbar.backgroundImage') }}</h3>
          <input type="file" accept="image/png,image/jpeg" @change="onUploadImage" class="text-xs" />
          <p v-if="uploadingImage" class="text-xs text-slate-400 mt-1">{{ $t('warehouseLayout.toolbar.uploading') }}</p>
          <label class="block mt-3 text-sm">
            <span class="block text-xs font-semibold text-slate-500 mb-1">{{ $t('warehouseLayout.toolbar.scale') }}</span>
            <input v-model.number="scalePxPerMeter" type="number" min="0" step="0.1" class="field-input w-full" :placeholder="$t('warehouseLayout.toolbar.notCalibrated')" />
          </label>
        </div>

        <div v-if="selectedObject" class="bg-white rounded-2xl border border-slate-200 p-4 space-y-2">
          <h3 class="text-sm font-semibold text-slate-600">{{ $t('warehouseLayout.toolbar.properties') }}</h3>
          <label class="block text-sm">
            <span class="block text-xs font-semibold text-slate-500 mb-1">{{ $t('warehouseLayout.properties.label') }}</span>
            <input v-model="selectedObject.label" type="text" class="field-input w-full" />
          </label>
          <div class="grid grid-cols-2 gap-2">
            <label class="block text-sm">
              <span class="block text-xs font-semibold text-slate-500 mb-1">X</span>
              <input v-model.number="selectedObject.x" type="number" class="field-input w-full" />
            </label>
            <label class="block text-sm">
              <span class="block text-xs font-semibold text-slate-500 mb-1">Y</span>
              <input v-model.number="selectedObject.y" type="number" class="field-input w-full" />
            </label>
            <label class="block text-sm">
              <span class="block text-xs font-semibold text-slate-500 mb-1">{{ $t('warehouseLayout.properties.width') }}</span>
              <input v-model.number="selectedObject.width" type="number" min="1" class="field-input w-full" />
            </label>
            <label class="block text-sm">
              <span class="block text-xs font-semibold text-slate-500 mb-1">{{ $t('warehouseLayout.properties.height') }}</span>
              <input v-model.number="selectedObject.height" type="number" min="1" class="field-input w-full" />
            </label>
          </div>
          <label class="block text-sm">
            <span class="block text-xs font-semibold text-slate-500 mb-1">{{ $t('warehouseLayout.properties.rotation') }}</span>
            <input v-model.number="selectedObject.rotation_degrees" type="number" class="field-input w-full" />
          </label>
          <div class="grid grid-cols-2 gap-2">
            <label class="block text-sm">
              <span class="block text-xs font-semibold text-slate-500 mb-1">{{ $t('warehouseLayout.properties.fillColor') }}</span>
              <input v-model="selectedObject.fill_color" type="color" class="w-full h-9" />
            </label>
            <label class="block text-sm">
              <span class="block text-xs font-semibold text-slate-500 mb-1">{{ $t('warehouseLayout.properties.strokeColor') }}</span>
              <input v-model="selectedObject.stroke_color" type="color" class="w-full h-9" />
            </label>
          </div>
          <label class="block text-sm" v-if="selectedObject.object_type === 'ZONE'">
            <span class="block text-xs font-semibold text-slate-500 mb-1">{{ $t('warehouseLayout.properties.linkedZone') }}</span>
            <select v-model="selectedObject.reference_id" class="field-select w-full" @change="selectedObject.reference_type = 'ZONE'">
              <option :value="null">{{ $t('common.labels.selectOption') }}</option>
              <option v-for="z in zones" :key="z.id" :value="z.id">{{ z.code }} - {{ z.name }}</option>
            </select>
          </label>
          <button @click="deleteSelected" class="w-full text-sm text-red-600 bg-red-50 rounded-lg py-2 mt-2">
            {{ $t('warehouseLayout.actions.delete') }}
          </button>
        </div>
      </div>

      <!-- Canvas -->
      <div :class="isEditing ? 'lg:col-span-3' : 'lg:col-span-4'" class="bg-white rounded-2xl border border-slate-200 p-2 overflow-auto" style="max-height: 75vh;">
        <svg
          ref="svgRef"
          :viewBox="viewBox"
          class="w-full"
          style="min-height: 500px; background: #f8fafc;"
          @mousemove="onMouseMove"
          @mouseup="onMouseUp"
          @mouseleave="onMouseUp"
        >
          <image v-if="backgroundImageUrl" :href="backgroundImageUrl" x="0" y="0" :width="backgroundImageWidthPx || 1000" :height="backgroundImageHeightPx || 700" opacity="0.6" />

          <g v-for="obj in displayObjects" :key="obj.id">
            <rect
              :x="obj.x" :y="obj.y" :width="obj.width" :height="obj.height"
              :fill="obj.fill_color || '#e2e8f0'" :stroke="obj.stroke_color || '#64748b'" stroke-width="1.5"
              :transform="rotationTransform(obj)"
              :class="isEditing ? 'cursor-move' : ''"
              @mousedown="isEditing ? onMouseDownShape($event, obj) : null"
            />
            <text
              :x="obj.x + obj.width / 2" :y="obj.y + obj.height / 2"
              text-anchor="middle" dominant-baseline="middle"
              font-size="11" fill="#1e293b" style="pointer-events: none;"
              :transform="rotationTransform(obj)"
            >{{ obj.reference_label || obj.label || obj.object_type }}</text>

            <rect
              v-if="isEditing && selectedObjectId === obj.id"
              :x="obj.x + obj.width - 6" :y="obj.y + obj.height - 6" width="10" height="10"
              fill="#ea580c" class="cursor-nwse-resize"
              @mousedown.stop="onMouseDownResize($event, obj)"
            />
            <rect
              v-if="isEditing"
              :x="obj.x" :y="obj.y" :width="obj.width" :height="obj.height"
              fill="transparent" :stroke="selectedObjectId === obj.id ? '#ea580c' : 'transparent'" stroke-width="2" stroke-dasharray="4"
              style="pointer-events: none;"
            />
          </g>
        </svg>
      </div>

      <!-- Version history -->
      <div v-if="showVersions" class="lg:col-span-4 bg-white rounded-2xl border border-slate-200 p-4">
        <h3 class="text-sm font-semibold text-slate-600 mb-2">{{ $t('warehouseLayout.history.title') }}</h3>
        <table class="w-full text-sm">
          <thead class="text-xs text-slate-500 uppercase">
            <tr>
              <th class="table-th">{{ $t('warehouseLayout.history.version') }}</th>
              <th class="table-th">{{ $t('warehouseLayout.history.objectCount') }}</th>
              <th class="table-th">{{ $t('warehouseLayout.history.remark') }}</th>
              <th class="table-th">{{ $t('warehouseLayout.history.createdBy') }}</th>
              <th class="table-th">{{ $t('warehouseLayout.history.createdAt') }}</th>
              <th class="table-th"></th>
            </tr>
          </thead>
          <tbody class="divide-y divide-slate-100">
            <tr v-for="v in versions" :key="v.id" class="hover:bg-slate-50">
              <td class="table-td">{{ v.version_no }}</td>
              <td class="table-td">{{ v.object_count }}</td>
              <td class="table-td">{{ v.remark || '-' }}</td>
              <td class="table-td">{{ v.created_by_name || '-' }}</td>
              <td class="table-td">{{ formatDate(v.created_at) }}</td>
              <td class="table-td text-right">
                <button class="text-orange-600" @click="viewVersion(v.id)">{{ $t('warehouseLayout.history.view') }}</button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <ReportPrintPreview v-model="showPreview" :pdf-url="previewUrl" />
  </div>
</template>

<script>
import PageHeader from '../../components/ui/PageHeader.vue'
import ReportPrintPreview from '../../components/reports/ReportPrintPreview.vue'
import { hasPermission } from '../../services/auth'
import { listWarehouses, listZones } from '../../services/warehouses'
import {
  getLayout, saveLayout, listLayoutVersions, getLayoutVersion,
  uploadLayoutBackgroundImage, previewLayoutPdf, downloadLayoutPdf,
} from '../../services/warehouseLayout'

const OBJECT_TYPES = ['ZONE', 'LOCATION', 'RACK', 'AISLE', 'DOCK', 'WALL', 'LABEL', 'SHAPE']
let tempIdCounter = -1

export default {
  name: 'WarehouseLayoutView',
  components: { PageHeader, ReportPrintPreview },

  data() {
    return {
      warehouses: [],
      zones: [],
      selectedWarehouseId: '',

      objects: [],
      backgroundImageId: null,
      backgroundImageUrl: null,
      backgroundImageWidthPx: null,
      backgroundImageHeightPx: null,
      scalePxPerMeter: null,

      selectedObjectId: null,
      isEditing: false,
      saving: false,
      uploadingImage: false,

      versions: [],
      showVersions: false,
      viewingVersionId: null,
      viewingVersionNo: null,

      loadError: '',
      objectTypes: OBJECT_TYPES,

      drag: null,
      resize: null,

      showPreview: false,
      previewUrl: null,
    }
  },

  computed: {
    canEdit() {
      return hasPermission('layout.update')
    },
    selectedObject() {
      return this.objects.find((o) => o.id === this.selectedObjectId) || null
    },
    displayObjects() {
      return this.objects
    },
    viewBox() {
      const w = this.backgroundImageWidthPx || 1000
      const h = this.backgroundImageHeightPx || 700
      return `0 0 ${w} ${h}`
    },
  },

  async mounted() {
    await this.fetchWarehouses()
  },

  methods: {
    formatDate(v) {
      return v ? new Date(v).toLocaleString() : '-'
    },
    rotationTransform(obj) {
      if (!obj.rotation_degrees) return ''
      const cx = obj.x + obj.width / 2
      const cy = obj.y + obj.height / 2
      return `rotate(${obj.rotation_degrees} ${cx} ${cy})`
    },

    async fetchWarehouses() {
      try {
        const res = await listWarehouses({ status: 'ACTIVE', limit: 100 })
        this.warehouses = res.data
      } catch {
        this.warehouses = []
      }
    },
    async onWarehouseChange() {
      this.isEditing = false
      this.viewingVersionId = null
      this.showVersions = false
      if (!this.selectedWarehouseId) return
      await Promise.all([this.fetchLayout(), this.fetchZones()])
    },
    async fetchZones() {
      try {
        const res = await listZones(this.selectedWarehouseId)
        this.zones = res.data
      } catch {
        this.zones = []
      }
    },
    async fetchLayout() {
      this.loadError = ''
      try {
        const res = await getLayout(this.selectedWarehouseId)
        this.applyVersion(res.data)
      } catch (err) {
        this.loadError = err.message || 'Failed to load layout'
      }
    },
    applyVersion(version) {
      if (!version) {
        this.objects = []
        this.backgroundImageId = null
        this.backgroundImageUrl = null
        this.backgroundImageWidthPx = null
        this.backgroundImageHeightPx = null
        this.scalePxPerMeter = null
        return
      }
      this.objects = (version.objects || []).map((o) => ({ ...o }))
      this.backgroundImageId = version.background_image_id
      this.backgroundImageUrl = version.background_image_url
      this.backgroundImageWidthPx = version.background_image_width_px
      this.backgroundImageHeightPx = version.background_image_height_px
      this.scalePxPerMeter = version.scale_px_per_meter
    },

    startEditing() {
      this.isEditing = true
      this.viewingVersionId = null
    },
    cancelEditing() {
      this.isEditing = false
      this.selectedObjectId = null
      this.fetchLayout()
    },

    addShape(type) {
      const id = tempIdCounter--
      this.objects.push({
        id, object_type: type, x: 50, y: 50, width: 120, height: 80, rotation_degrees: 0,
        fill_color: '#e2e8f0', stroke_color: '#64748b', label: type, z_index: this.objects.length,
        reference_type: null, reference_id: null, reference_label: null,
      })
      this.selectedObjectId = id
    },
    deleteSelected() {
      if (this.selectedObjectId == null) return
      this.objects = this.objects.filter((o) => o.id !== this.selectedObjectId)
      this.selectedObjectId = null
    },

    svgPoint(evt) {
      const svg = this.$refs.svgRef
      const pt = svg.createSVGPoint()
      pt.x = evt.clientX
      pt.y = evt.clientY
      const ctm = svg.getScreenCTM()
      if (!ctm) return { x: 0, y: 0 }
      const inv = ctm.inverse()
      const transformed = pt.matrixTransform(inv)
      return { x: transformed.x, y: transformed.y }
    },
    onMouseDownShape(evt, obj) {
      this.selectedObjectId = obj.id
      const p = this.svgPoint(evt)
      this.drag = { id: obj.id, offsetX: p.x - obj.x, offsetY: p.y - obj.y }
    },
    onMouseDownResize(evt, obj) {
      this.selectedObjectId = obj.id
      const p = this.svgPoint(evt)
      this.resize = { id: obj.id, startX: p.x, startY: p.y, startW: obj.width, startH: obj.height }
    },
    onMouseMove(evt) {
      if (this.drag) {
        const obj = this.objects.find((o) => o.id === this.drag.id)
        if (!obj) return
        const p = this.svgPoint(evt)
        obj.x = Math.round(p.x - this.drag.offsetX)
        obj.y = Math.round(p.y - this.drag.offsetY)
      } else if (this.resize) {
        const obj = this.objects.find((o) => o.id === this.resize.id)
        if (!obj) return
        const p = this.svgPoint(evt)
        obj.width = Math.max(10, Math.round(this.resize.startW + (p.x - this.resize.startX)))
        obj.height = Math.max(10, Math.round(this.resize.startH + (p.y - this.resize.startY)))
      }
    },
    onMouseUp() {
      this.drag = null
      this.resize = null
    },

    async onUploadImage(evt) {
      const file = evt.target.files[0]
      if (!file) return
      this.uploadingImage = true
      try {
        const res = await uploadLayoutBackgroundImage(this.selectedWarehouseId, file)
        this.backgroundImageId = res.data.id
        this.backgroundImageUrl = res.data.url
        this.backgroundImageWidthPx = res.data.width_px
        this.backgroundImageHeightPx = res.data.height_px
      } catch (err) {
        this.loadError = err.message || 'Failed to upload image'
      } finally {
        this.uploadingImage = false
      }
    },

    async onSave() {
      this.saving = true
      this.loadError = ''
      try {
        const payload = {
          backgroundImageId: this.backgroundImageId || undefined,
          scalePxPerMeter: this.scalePxPerMeter || undefined,
          objects: this.objects.map((o) => ({
            object_type: o.object_type,
            reference_type: o.reference_type || undefined,
            reference_id: o.reference_id || undefined,
            x: o.x, y: o.y, width: o.width, height: o.height,
            rotation_degrees: o.rotation_degrees || 0,
            fill_color: o.fill_color || undefined,
            stroke_color: o.stroke_color || undefined,
            label: o.label || undefined,
            z_index: o.z_index || 0,
          })),
        }
        await saveLayout(this.selectedWarehouseId, payload)
        this.isEditing = false
        this.selectedObjectId = null
        await this.fetchLayout()
      } catch (err) {
        this.loadError = err.message || 'Failed to save layout'
      } finally {
        this.saving = false
      }
    },

    async fetchVersionsIfNeeded() {
      if (this.versions.length === 0) {
        await this.fetchVersions()
      }
    },
    async fetchVersions() {
      try {
        const res = await listLayoutVersions(this.selectedWarehouseId)
        this.versions = res.data
      } catch {
        this.versions = []
      }
    },
    async viewVersion(versionId) {
      try {
        const res = await getLayoutVersion(this.selectedWarehouseId, versionId)
        this.applyVersion(res.data)
        this.viewingVersionId = versionId
        this.viewingVersionNo = res.data.version_no
        this.isEditing = false
        this.showVersions = false
      } catch (err) {
        this.loadError = err.message || 'Failed to load version'
      }
    },
    returnToLatest() {
      this.viewingVersionId = null
      this.fetchLayout()
    },

    async onPreviewPdf() {
      const blob = await previewLayoutPdf(this.selectedWarehouseId)
      this.previewUrl = URL.createObjectURL(blob)
      this.showPreview = true
    },
    async onDownloadPdf() {
      await downloadLayoutPdf(this.selectedWarehouseId)
    },
  },

  watch: {
    showVersions(val) {
      if (val) this.fetchVersionsIfNeeded()
    },
  },
}
</script>

<style scoped>
.field-select, .field-input {
  border: 1px solid #cbd5e1;
  border-radius: 8px;
  padding: 6px 10px;
  font-size: 14px;
  min-width: 140px;
}
.btn-primary {
  background: #ea580c;
  color: white;
  padding: 8px 16px;
  border-radius: 8px;
  font-size: 14px;
  font-weight: 600;
}
.btn-primary:disabled {
  opacity: 0.6;
}
.btn-secondary {
  background: #f1f5f9;
  color: #334155;
  padding: 8px 16px;
  border-radius: 8px;
  font-size: 14px;
  font-weight: 600;
}
.table-th {
  padding: 8px 12px;
  text-align: left;
}
.table-td {
  padding: 8px 12px;
  font-size: 13px;
  color: #334155;
}
</style>
