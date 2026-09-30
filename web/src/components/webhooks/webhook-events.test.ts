import { describe, expect, it } from 'vitest'
import { describeEvents, groupCatalog, httpStatusTone, isCovered, toggleAll, toggleEvent, toggleGroup } from './webhook-events'

describe('webhook events selection', () => {
  const catalog = [
    { name: 'invoice.created', description: 'a' },
    { name: 'invoice.paid', description: 'b' },
    { name: 'expense_payment.created', description: 'c' },
  ]
  it('groups the catalog by prefix', () => {
    const g = groupCatalog(catalog)
    expect(g.map((x) => x.wildcard)).toEqual(['invoice.*', 'expense_payment.*'])
    expect(g[0]?.label).toBe('Faktury')
    expect(g[0]?.entries).toHaveLength(2)
  })
  it('covers by exact name, prefix and star', () => {
    expect(isCovered(['invoice.*'], 'invoice.paid')).toBe(true)
    expect(isCovered(['*'], 'bank.matched')).toBe(true)
    expect(isCovered(['invoice.paid'], 'invoice.created')).toBe(false)
    expect(isCovered(['expense.*'], 'expense_payment.created')).toBe(false)
  })
  it('toggles names, groups and all', () => {
    expect(toggleEvent(['invoice.paid'], 'invoice.paid')).toEqual([])
    expect(toggleGroup(['invoice.paid', 'bank.matched'], 'invoice')).toEqual(['bank.matched', 'invoice.*'])
    expect(toggleGroup(['invoice.*'], 'invoice')).toEqual([])
    expect(toggleAll(['invoice.paid'])).toEqual(['*'])
    expect(toggleAll(['*'])).toEqual([])
  })
  it('describes the selection', () => {
    expect(describeEvents([])).toEqual(['Všechny události'])
    expect(describeEvents(['invoice.*', 'bank.matched'])).toEqual(['Faktury (vše)', 'bank.matched'])
  })
  it('classifies HTTP status', () => {
    expect(httpStatusTone(204)).toBe('success')
    expect(httpStatusTone(500)).toBe('destructive')
    expect(httpStatusTone(0)).toBe('muted')
  })
})
