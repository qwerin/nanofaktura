import { describe, expect, it } from 'vitest'
import { countResults, matchesQuery, normalizeText } from './palette-actions'

describe('palette search helpers', () => {
  it('normalizes diacritics and case', () => {
    expect(normalizeText('  Nová Faktura ')).toBe('nova faktura')
  })
  it('matches all words against label and keywords', () => {
    expect(matchesQuery('', 'Nová faktura')).toBe(true)
    expect(matchesQuery('nova fak', 'Nová faktura')).toBe(true)
    expect(matchesQuery('vydaj', 'Nový náklad', ['výdaj'])).toBe(true)
    expect(matchesQuery('kontakt', 'Nová faktura')).toBe(false)
  })
  it('counts results', () => {
    expect(countResults(undefined)).toBe(0)
    const hit = { type: 'invoice' as const, id: 1, title: 't', subtitle: '', url_hint: '/' }
    expect(countResults({ invoices: [hit], expenses: [], subjects: [hit], price_items: [] })).toBe(2)
  })
})
