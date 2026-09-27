import { describe, expect, it } from 'vitest'
import { datePresets, extraFilterCount, invoiceSearchSchema, toApiFilters } from './filters'

describe('invoice filters', () => {
  it('parses search params and drops invalid values', () => {
    expect(invoiceSearchSchema.parse({ status: 'nope', since: '2026-1-1', sort: 'due_on', query: 'abc' })).toEqual({
      status: undefined,
      document_type: undefined,
      query: 'abc',
      since: undefined,
      until: undefined,
      sort: 'due_on',
    })
  })

  it('maps to API filters without empty or default values', () => {
    expect(toApiFilters({ query: '  ', sort: '-issued_on', status: 'paid' })).toEqual({ status: 'paid' })
    expect(toApiFilters({ query: ' 2026 ', since: '2026-01-01' })).toEqual({ query: '2026', since: '2026-01-01' })
  })

  it('counts extra filters', () => {
    expect(extraFilterCount({})).toBe(0)
    expect(extraFilterCount({ document_type: 'proforma', since: '2026-01-01', until: '2026-02-01', sort: 'due_on' })).toBe(3)
  })

  it('computes date presets', () => {
    const p = Object.fromEntries(datePresets('2026-01-15').map((x) => [x.id, [x.since, x.until]]))
    expect(p['this-month']).toEqual(['2026-01-01', '2026-01-31'])
    expect(p['last-month']).toEqual(['2025-12-01', '2025-12-31'])
    expect(p['this-quarter']).toEqual(['2026-01-01', '2026-03-31'])
    expect(p['last-year']).toEqual(['2025-01-01', '2025-12-31'])
    const feb = Object.fromEntries(datePresets('2028-02-10').map((x) => [x.id, [x.since, x.until]]))
    expect(feb['this-month']).toEqual(['2028-02-01', '2028-02-29'])
  })
})
