// Central vue-i18n setup for Smart WMS.
//
// Supported locales: Thai (th-TH), English (en-US, fallback), Simplified
// Chinese (zh-CN). Translation messages are structured per module (see
// src/i18n/locales/*.json) so each view owns one top-level namespace —
// e.g. `receiving.*`, `inventory.stockAdjustment.*` — rather than one
// flat key list.
//
// Persistence (Phase 6 — SaaS compatibility): for this demo, the active
// locale is read from/written to localStorage only. The resolution
// order mirrors what a real tenant/user-preference system would later
// slot into without changing any component code:
//   1. A locale the user explicitly chose before (localStorage)
//   2. A tenant-level default language (not implemented yet — no
//      backend/DB field exists for it; `resolveTenantDefaultLocale`
//      below is the single seam a future Phase would wire up)
//   3. The system fallback (English)
// No new database migration or backend field is introduced here.
import { createI18n } from 'vue-i18n'

import enUS from './locales/en-US.json'
import thTH from './locales/th-TH.json'
import zhCN from './locales/zh-CN.json'

export const SUPPORTED_LOCALES = [
  { code: 'th-TH', label: 'ไทย', shortLabel: 'TH' },
  { code: 'en-US', label: 'English', shortLabel: 'EN' },
  { code: 'zh-CN', label: '简体中文', shortLabel: '中文' },
]

export const FALLBACK_LOCALE = 'en-US'
const STORAGE_KEY = 'wms_locale'

// resolveTenantDefaultLocale is a deliberate no-op seam for Phase 6:
// once a tenant record carries a default_language column, this is the
// one place that would read it (e.g. from the authenticated user's
// profile) instead of returning null. Keeping it as an explicit,
// named function — rather than inlining "null" at the call site below
// — means the real implementation is a one-function change, not a
// hunt through every call site that resolves the active locale.
function resolveTenantDefaultLocale() {
  return null
}

function isSupportedLocale(code) {
  return SUPPORTED_LOCALES.some((l) => l.code === code)
}

export function getStoredLocale() {
  try {
    const stored = localStorage.getItem(STORAGE_KEY)
    return isSupportedLocale(stored) ? stored : null
  } catch {
    // localStorage can throw in some privacy modes — fall through to
    // the next resolution step rather than breaking the app.
    return null
  }
}

function detectBrowserLocale() {
  const candidates = navigator.languages && navigator.languages.length
    ? navigator.languages
    : [navigator.language]

  for (const candidate of candidates) {
    if (!candidate) continue
    const lower = candidate.toLowerCase()
    if (lower.startsWith('th')) return 'th-TH'
    if (lower.startsWith('zh')) return 'zh-CN'
    if (lower.startsWith('en')) return 'en-US'
  }
  return null
}

// resolveInitialLocale implements the §6 resolution order documented
// above: explicit user choice -> tenant default (not wired up yet) ->
// browser hint -> system fallback.
export function resolveInitialLocale() {
  return (
    getStoredLocale() ||
    resolveTenantDefaultLocale() ||
    detectBrowserLocale() ||
    FALLBACK_LOCALE
  )
}

export function persistLocale(code) {
  if (!isSupportedLocale(code)) return
  try {
    localStorage.setItem(STORAGE_KEY, code)
  } catch {
    // Non-critical: the language still switches for this session even
    // if it can't be remembered for next time.
  }
}

// Locale-aware date/time formatting (Phase 5). Values passed to $d must
// stay real Date objects / epoch values from the backend — this only
// changes how they are *displayed*, never what is stored or sent back.
const datetimeFormats = {
  'en-US': {
    short: { year: 'numeric', month: 'short', day: 'numeric' },
    long: { year: 'numeric', month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' },
    time: { hour: '2-digit', minute: '2-digit' },
  },
  'th-TH': {
    short: { year: 'numeric', month: 'short', day: 'numeric' },
    long: { year: 'numeric', month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' },
    time: { hour: '2-digit', minute: '2-digit' },
  },
  'zh-CN': {
    short: { year: 'numeric', month: 'short', day: 'numeric' },
    long: { year: 'numeric', month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' },
    time: { hour: '2-digit', minute: '2-digit' },
  },
}

// Locale-aware number/currency formatting (Phase 5). Quantities use
// each locale's own grouping/decimal separator convention automatically
// via Intl.NumberFormat under the hood; the underlying numeric value
// passed in is never mutated.
const numberFormats = {
  'en-US': {
    decimal: { style: 'decimal', minimumFractionDigits: 0, maximumFractionDigits: 4 },
    integer: { style: 'decimal', maximumFractionDigits: 0 },
    currency: { style: 'currency', currency: 'USD' },
  },
  'th-TH': {
    decimal: { style: 'decimal', minimumFractionDigits: 0, maximumFractionDigits: 4 },
    integer: { style: 'decimal', maximumFractionDigits: 0 },
    currency: { style: 'currency', currency: 'THB' },
  },
  'zh-CN': {
    decimal: { style: 'decimal', minimumFractionDigits: 0, maximumFractionDigits: 4 },
    integer: { style: 'decimal', maximumFractionDigits: 0 },
    currency: { style: 'currency', currency: 'CNY' },
  },
}

const i18n = createI18n({
  // Legacy mode (the default) is what makes `this.$t()`/`this.$d()`/
  // `this.$n()` available directly inside every Options API component's
  // template and methods, matching how this codebase is already
  // written (see MainLayout.vue, LoginView.vue) — no per-component
  // `useI18n()` wiring needed.
  legacy: true,
  locale: resolveInitialLocale(),
  fallbackLocale: FALLBACK_LOCALE,
  messages: {
    'en-US': enUS,
    'th-TH': thTH,
    'zh-CN': zhCN,
  },
  datetimeFormats,
  numberFormats,
  // Surfaces a missing key during development instead of silently
  // rendering the raw key path, without throwing in production.
  missingWarn: true,
  fallbackWarn: false,
})

// setLocale is the single place that changes the active language,
// persists the choice, and keeps <html lang> correct for accessibility/
// screen readers — called by LanguageSwitcher.vue. Switching never
// triggers a route change, a logout, or a page reload: vue-i18n's
// `i18n.global.locale` is a reactive ref that every mounted
// component's `$t` call re-reads on the next render.
export function setLocale(code) {
  if (!isSupportedLocale(code)) return
  i18n.global.locale = code
  persistLocale(code)
  document.documentElement.setAttribute('lang', code)
}

document.documentElement.setAttribute('lang', i18n.global.locale)

export default i18n
