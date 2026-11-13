import { describe, it, expect, beforeEach } from 'vitest'
import i18n, { setLocale, getStoredLocale, SUPPORTED_LOCALES } from '../index.js'

describe('language switching', () => {
  beforeEach(() => {
    localStorage.clear()
    setLocale('en-US')
  })

  it('switches the active locale instantly — i18n.global.locale updates synchronously, no reload involved', () => {
    expect(i18n.global.locale).toBe('en-US')
    setLocale('th-TH')
    expect(i18n.global.locale).toBe('th-TH')
    setLocale('zh-CN')
    expect(i18n.global.locale).toBe('zh-CN')
  })

  it('persists the chosen locale to localStorage so it survives reopening the app', () => {
    setLocale('zh-CN')
    expect(getStoredLocale()).toBe('zh-CN')
  })

  it('updates document.documentElement.lang for accessibility', () => {
    setLocale('th-TH')
    expect(document.documentElement.getAttribute('lang')).toBe('th-TH')
  })

  it('rejects an unsupported locale code — falls back to leaving the current locale untouched', () => {
    setLocale('en-US')
    setLocale('fr-FR')
    expect(i18n.global.locale).toBe('en-US')
  })

  it('exposes exactly the three required locales: th-TH, en-US, zh-CN', () => {
    const codes = SUPPORTED_LOCALES.map((l) => l.code).sort()
    expect(codes).toEqual(['en-US', 'th-TH', 'zh-CN'])
  })

  it('every platformAdmin.* key resolves to real text in all three locales, never a raw key path', () => {
    const keys = [
      'platformAdmin.tenants.title',
      'platformAdmin.form.newTitle',
      'platformAdmin.status.suspended',
      'platformAdmin.detail.backButton',
    ]
    for (const locale of ['en-US', 'th-TH', 'zh-CN']) {
      setLocale(locale)
      for (const key of keys) {
        const resolved = i18n.global.t(key)
        expect(resolved).not.toBe(key)
        expect(resolved.length).toBeGreaterThan(0)
      }
    }
  })
})
