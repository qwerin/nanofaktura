import { describe, expect, it } from 'vitest'
import { formatBytes, importWarningText, looksLikeBackup, progressPercent } from './backup'

describe('importWarningText', () => {
  it('translates known codes with Czech plurals', () => {
    expect(importWarningText({ code: 'recurring_deactivated', message: '', count: 1 })).toMatch(/^1 pravidelná faktura byla vypnuta/)
    expect(importWarningText({ code: 'recurring_deactivated', message: '', count: 3 })).toMatch(/^3 pravidelné faktury byly vypnuty/)
    expect(importWarningText({ code: 'webhooks_inactive', message: '', count: 5 })).toMatch(/^5 webhooků je neaktivních/)
    expect(importWarningText({ code: 'bank_tokens_removed', message: '', count: 2 })).toContain('Fio')
    expect(importWarningText({ code: 'reminders_disabled', message: '' })).toContain('upomínky')
  })
  it('falls back to the English message for unknown codes', () => {
    expect(importWarningText({ code: 'something_new', message: 'Something new happened' })).toBe('Something new happened')
  })
})

describe('formatBytes', () => {
  it('formats sizes', () => {
    expect(formatBytes(512)).toBe('512 B')
    expect(formatBytes(1536)).toBe('1,5 kB')
    expect(formatBytes(12.3 * 1024 * 1024)).toBe('12 MB')
    expect(formatBytes(3 * 1024 * 1024)).toBe('3,0 MB')
    expect(formatBytes(-1)).toBe('')
  })
})

describe('progressPercent', () => {
  it('computes a clamped percentage or null', () => {
    expect(progressPercent(50, 200)).toBe(25)
    expect(progressPercent(300, 200)).toBe(100)
    expect(progressPercent(10, 0)).toBeNull()
    expect(progressPercent(10, null)).toBeNull()
  })
})

describe('looksLikeBackup', () => {
  it('accepts zip files', () => {
    expect(looksLikeBackup({ name: 'nanofaktura-firma-2026-09-30.ZIP', type: '' })).toBe(true)
    expect(looksLikeBackup({ name: 'x', type: 'application/zip' })).toBe(true)
    expect(looksLikeBackup({ name: 'faktura.pdf', type: 'application/pdf' })).toBe(false)
  })
})
