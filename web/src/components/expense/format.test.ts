import { describe, expect, it } from 'vitest'
import { canEditDocuments } from './permissions'
import { searchToFilters, expenseSearchSchema } from './expense-search'
import { countActiveFilters, currencyOptions, plural, sumByCurrency, vatRateOptions } from './format'

describe('sumByCurrency', () => {
  it('groups by currency, CZK first', () => {
    const items = [
      { currency: 'EUR', total: 100 },
      { currency: 'CZK', total: 250 },
      { currency: 'CZK', total: 50 },
      { currency: 'USD', total: 7 },
    ]
    expect(sumByCurrency(items, (i) => i.total)).toEqual([
      { currency: 'CZK', amount: 300 },
      { currency: 'EUR', amount: 100 },
      { currency: 'USD', amount: 7 },
    ])
    expect(sumByCurrency([], () => 1)).toEqual([])
  })
})

describe('options', () => {
  it('vat rates include an unusual stored rate, sorted desc', () => {
    expect(vatRateOptions([1500]).map((o) => o.value)).toEqual(['2100', '1500', '1200', '0'])
    expect(vatRateOptions([2100, NaN]).map((o) => o.value)).toEqual(['2100', '1200', '0'])
  })

  it('currency options keep an unknown current value', () => {
    expect(currencyOptions('CHF').map((o) => o.value)).toContain('CHF')
    expect(currencyOptions('EUR').filter((o) => o.value === 'EUR')).toHaveLength(1)
  })
})

describe('misc', () => {
  it('counts active filters', () => {
    expect(countActiveFilters({ status: 'paid', query: 'x', category: '', since: undefined }, ['query'])).toBe(1)
  })

  it('Czech plural', () => {
    const f = ['náklad', 'náklady', 'nákladů'] as const
    expect([0, 1, 2, 4, 5, 22].map((n) => plural(n, f))).toEqual(['nákladů', 'náklad', 'náklady', 'náklady', 'nákladů', 'nákladů'])
  })

  it('role helper', () => {
    expect(['owner', 'admin', 'member'].every(canEditDocuments)).toBe(true)
    expect(canEditDocuments('accountant')).toBe(false)
    expect(canEditDocuments(undefined)).toBe(false)
  })
})

describe('expense search', () => {
  it('drops invalid values and maps to API filters', () => {
    const s = expenseSearchSchema.parse({ status: 'nope', subject_id: '12', since: '2026-02-30', until: '2026-03-31', query: '  acme ' })
    expect(s).toEqual({ status: undefined, category: undefined, subject_id: 12, since: undefined, until: '2026-03-31', query: '  acme ' })
    expect(searchToFilters(s)).toEqual({ subject_id: 12, until: '2026-03-31', query: 'acme' })
    expect(searchToFilters({ query: '   ' })).toEqual({})
  })
})
