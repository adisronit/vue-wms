<!--
  Reusable language switcher — Thai / English / Simplified Chinese.
  Switches instantly (no logout, no page reload): it only calls
  setLocale(), which reassigns vue-i18n's reactive global locale and
  persists the choice to localStorage (see src/i18n/index.js). Used in
  MainLayout.vue's header (authenticated pages) and LoginView.vue
  (public page) — both render this exact same component so the choice
  behaves identically everywhere.

  Keyboard accessible: a native <button> toggle (Enter/Space activate
  it by default) plus a role="menu"/role="menuitem" listbox that
  supports Up/Down/Home/End/Escape and closes on blur or an outside
  click.
-->
<template>
  <div class="lang-switcher" @keydown.escape="close">
    <button
      type="button"
      class="lang-trigger"
      :aria-expanded="open ? 'true' : 'false'"
      aria-haspopup="menu"
      :aria-label="$t('auth.login.language')"
      @click="toggle"
    >
      <span class="lang-flag" aria-hidden="true">🌐</span>
      <span class="lang-current">{{ currentOption.shortLabel }}</span>
      <span class="lang-caret" aria-hidden="true">▾</span>
    </button>

    <ul
      v-if="open"
      ref="menu"
      class="lang-menu"
      role="menu"
      :aria-label="$t('auth.login.language')"
      @keydown.down.prevent="moveFocus(1)"
      @keydown.up.prevent="moveFocus(-1)"
      @keydown.home.prevent="focusAt(0)"
      @keydown.end.prevent="focusAt(options.length - 1)"
    >
      <li
        v-for="(option, index) in options"
        :key="option.code"
        role="menuitem"
        tabindex="0"
        class="lang-option"
        :class="{ 'lang-option-active': option.code === current }"
        :ref="(el) => setOptionRef(el, index)"
        @click="choose(option.code)"
        @keydown.enter.prevent="choose(option.code)"
        @keydown.space.prevent="choose(option.code)"
      >
        <span class="lang-option-label">{{ option.label }}</span>
        <span v-if="option.code === current" class="lang-option-check" aria-hidden="true">✓</span>
      </li>
    </ul>
  </div>
</template>

<script>
import { SUPPORTED_LOCALES, setLocale } from '../i18n'

export default {
  name: 'LanguageSwitcher',

  data() {
    return {
      open: false,
      options: SUPPORTED_LOCALES,
      optionRefs: []
    }
  },

  computed: {
    current() {
      return this.$i18n.locale
    },
    currentOption() {
      return this.options.find((o) => o.code === this.current) || this.options[0]
    }
  },

  mounted() {
    document.addEventListener('click', this.handleOutsideClick)
  },

  beforeUnmount() {
    document.removeEventListener('click', this.handleOutsideClick)
  },

  methods: {
    toggle() {
      this.open = !this.open
      if (this.open) {
        this.$nextTick(() => this.focusAt(this.options.findIndex((o) => o.code === this.current)))
      }
    },

    close() {
      this.open = false
    },

    choose(code) {
      setLocale(code)
      this.close()
    },

    handleOutsideClick(event) {
      if (!this.open) return
      if (this.$el && !this.$el.contains(event.target)) {
        this.close()
      }
    },

    setOptionRef(el, index) {
      this.optionRefs[index] = el
    },

    focusAt(index) {
      const safeIndex = Math.max(0, Math.min(index, this.options.length - 1))
      this.optionRefs[safeIndex]?.focus()
    },

    moveFocus(delta) {
      const activeIndex = this.optionRefs.findIndex((el) => el === document.activeElement)
      const nextIndex = (activeIndex + delta + this.options.length) % this.options.length
      this.focusAt(nextIndex)
    }
  }
}
</script>

<style scoped>
.lang-switcher {
  position: relative;
}

.lang-trigger {
  display: flex;
  align-items: center;
  gap: 6px;
  height: 40px;
  padding: 0 14px;
  border-radius: 9999px;
  background: #f1f5f9;
  color: #334155;
  font-weight: 600;
  font-size: 13px;
  border: 1px solid transparent;
  cursor: pointer;
  transition: background 0.15s;
}

.lang-trigger:hover {
  background: #e2e8f0;
}

.lang-trigger:focus-visible {
  outline: 2px solid #ea580c;
  outline-offset: 2px;
}

.lang-caret {
  font-size: 10px;
  opacity: 0.7;
}

.lang-menu {
  position: absolute;
  right: 0;
  top: calc(100% + 8px);
  min-width: 160px;
  background: white;
  border: 1px solid #e2e8f0;
  border-radius: 14px;
  box-shadow: 0 10px 25px rgba(15, 23, 42, 0.12);
  padding: 6px;
  z-index: 60;
  list-style: none;
  margin: 0;
}

.lang-option {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding: 10px 12px;
  border-radius: 10px;
  color: #334155;
  font-size: 14px;
  cursor: pointer;
}

.lang-option:hover,
.lang-option:focus-visible {
  background: #fff7ed;
  outline: none;
}

.lang-option-active {
  font-weight: 600;
  color: #ea580c;
}

.lang-option-check {
  color: #ea580c;
  font-weight: 600;
}
</style>
