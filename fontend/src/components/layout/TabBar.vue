<template>
  <div
    v-if="tabs.length > 0"
    role="tablist"
    aria-label="Open pages"
    class="h-11 shrink-0 bg-white border-b border-slate-200 flex items-stretch overflow-x-auto tabbar-scroll"
    @keydown.left.prevent="focusAdjacent(-1)"
    @keydown.right.prevent="focusAdjacent(1)"
  >
    <div
      v-for="tab in tabs"
      :key="tab.key"
      role="tab"
      :aria-selected="tab.key === activeKey"
      :tabindex="tab.key === activeKey ? 0 : -1"
      :data-tab-key="tab.key"
      class="tab-item group flex items-center gap-2 px-4 border-r border-slate-100 cursor-pointer select-none whitespace-nowrap text-sm"
      :class="tab.key === activeKey ? 'tab-active' : 'text-slate-500 hover:bg-slate-50 hover:text-slate-700'"
      @click="activate(tab)"
      @keydown.enter="activate(tab)"
      @keydown.space.prevent="activate(tab)"
    >
      <span class="truncate max-w-[12rem]">{{ tabTitle(tab) }}</span>
      <button
        type="button"
        class="tab-close shrink-0 w-5 h-5 rounded-full flex items-center justify-center text-slate-400 hover:bg-slate-200 hover:text-slate-700"
        :aria-label="$t('common.buttons.close')"
        @click.stop="requestClose(tab)"
      >
        ✕
      </button>
    </div>

    <ConfirmDialog
      :open="confirmCloseOpen"
      :title="$t('tabs.closeTitle')"
      :message="$t('tabs.closeMessage')"
      :confirm-label="$t('tabs.closeConfirm')"
      :cancel-label="$t('tabs.closeCancel')"
      danger
      @confirm="confirmClose"
      @cancel="cancelClose"
    />
  </div>
</template>

<script>
import { state, closeTab } from '../../services/tabs'
import { isTabDirty } from '../../services/dirtyRegistry'
import ConfirmDialog from '../ui/ConfirmDialog.vue'

export default {
  name: 'TabBar',

  components: { ConfirmDialog },

  emits: ['navigate'],

  data() {
    return {
      pendingCloseKey: null
    }
  },

  computed: {
    tabs() {
      return state.tabs
    },
    activeKey() {
      return state.activeKey
    },
    confirmCloseOpen() {
      return this.pendingCloseKey !== null
    }
  },

  methods: {
    tabTitle(tab) {
      return tab.titleKey ? this.$t(tab.titleKey) : tab.name
    },

    activate(tab) {
      if (tab.key === this.activeKey) return
      this.$emit('navigate', tab.fullPath)
    },

    requestClose(tab) {
      if (isTabDirty(tab.key)) {
        this.pendingCloseKey = tab.key
        return
      }
      this.doClose(tab.key)
    },

    confirmClose() {
      const key = this.pendingCloseKey
      this.pendingCloseKey = null
      if (key) this.doClose(key)
    },

    cancelClose() {
      this.pendingCloseKey = null
    },

    doClose(key) {
      const wasActive = key === this.activeKey
      const nextKey = closeTab(key)
      if (!wasActive) return
      const nextTab = this.tabs.find((t) => t.key === nextKey)
      this.$emit('navigate', nextTab ? nextTab.fullPath : '/dashboard')
    },

    focusAdjacent(direction) {
      const current = state.activeKey
      const idx = this.tabs.findIndex((t) => t.key === current)
      if (idx === -1) return
      const next = this.tabs[idx + direction]
      if (next) this.activate(next)
    }
  }
}
</script>

<style scoped>
.tab-item {
  position: relative;
}

.tab-active {
  color: #0f172a;
  font-weight: 600;
  background: #fff7ed;
  border-bottom: 2px solid #ea580c;
}

.tab-close {
  font-size: 10px;
  line-height: 1;
}

.tabbar-scroll::-webkit-scrollbar {
  height: 4px;
}

.tabbar-scroll::-webkit-scrollbar-thumb {
  background: #cbd5e1;
  border-radius: 10px;
}
</style>
