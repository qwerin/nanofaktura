import { describe, expect, it } from 'vitest'
import { calculateTotals, chargesNoVat, type CalcOptions } from './calc'
import { allowedActions, dueInfo, plural, storedStatus } from './status'

const net: CalcOptions = { pricesIncludeVat: false, reverseCharge: false, roundTotal: false, nonVatPayer: false }

describe('calculateTotals', () => {
  it('net prices: VAT per rate from the summed base', () => {
    const t = calculateTotals(
      [
        { quantity: '1', unitPrice: 10000, vatRateBps: 2100 },
        { quantity: '1.5', unitPrice: 333, vatRateBps: 2100 },
        { quantity: '2', unitPrice: 5000, vatRateBps: 1200 },
      ],
      net,
    )
    // 333 × 1.5 = 499.5 → 500
    expect(t.lines[1]).toEqual({ base: 500, vat: 105, total: 605 })
    expect(t.vatRecap).toEqual([
      { vatRateBps: 2100, base: 10500, vat: 2205, total: 12705 },
      { vatRateBps: 1200, base: 10000, vat: 1200, total: 11200 },
    ])
    expect(t.subtotal).toBe(20500)
    expect(t.vatTotal).toBe(3405)
    expect(t.total).toBe(23905)
    expect(t.rounding).toBe(0)
  })

  it('gross prices: VAT extracted from the total', () => {
    const t = calculateTotals([{ quantity: '1', unitPrice: 12100, vatRateBps: 2100 }], { ...net, pricesIncludeVat: true })
    expect(t.vatRecap).toEqual([{ vatRateBps: 2100, base: 10000, vat: 2100, total: 12100 }])
    expect(t.total).toBe(12100)
  })

  it('reverse charge and non-payer: no VAT', () => {
    const lines = [{ quantity: '1', unitPrice: 10000, vatRateBps: 2100 }]
    expect(calculateTotals(lines, { ...net, reverseCharge: true }).vatRecap[0]).toEqual({
      vatRateBps: 2100,
      base: 10000,
      vat: 0,
      total: 10000,
    })
    expect(calculateTotals(lines, { ...net, nonVatPayer: true }).vatRecap[0]?.vatRateBps).toBe(0)
  })

  it('identified person: no domestic VAT, rates kept on reverse charge', () => {
    expect(chargesNoVat('identified_person', false)).toBe(true)
    expect(chargesNoVat('identified_person', true)).toBe(false)
    expect(chargesNoVat('non_vat_payer', true)).toBe(true)
    expect(chargesNoVat('vat_payer', false)).toBe(false)
  })

  it('round_total rounds half away from zero, also for negative totals', () => {
    const t = calculateTotals([{ quantity: '1', unitPrice: 12350, vatRateBps: 0 }], { ...net, roundTotal: true })
    expect(t.total).toBe(12400)
    expect(t.rounding).toBe(50)
    const neg = calculateTotals([{ quantity: '-1', unitPrice: 12350, vatRateBps: 0 }], { ...net, roundTotal: true })
    expect(neg.total).toBe(-12400)
    expect(neg.rounding).toBe(-50)
  })

  it('invalid quantity counts as zero', () => {
    expect(calculateTotals([{ quantity: 'abc', unitPrice: 100, vatRateBps: 0 }], net).total).toBe(0)
  })
})

describe('dueInfo', () => {
  const today = '2026-09-26'
  const base = { status: 'open' as const, paid_on: '' }
  it('describes the due date', () => {
    expect(dueInfo({ ...base, due_on: '2026-09-29' }, today)).toEqual({ text: 'splatná za 3 dny', tone: 'warning' })
    expect(dueInfo({ ...base, due_on: '2026-10-06' }, today)).toEqual({ text: 'splatná za 10 dní', tone: 'muted' })
    expect(dueInfo({ ...base, due_on: '2026-09-26' }, today)?.text).toBe('splatná dnes')
    expect(dueInfo({ ...base, due_on: '2026-09-27' }, today)?.text).toBe('splatná zítra')
    expect(dueInfo({ ...base, status: 'overdue', due_on: '2026-09-21' }, today)).toEqual({
      text: '5 dní po splatnosti',
      tone: 'destructive',
    })
    expect(dueInfo({ ...base, status: 'overdue', due_on: '2026-09-25' }, today)?.text).toBe('1 den po splatnosti')
  })
  it('has nothing for closed documents', () => {
    expect(dueInfo({ ...base, status: 'cancelled', due_on: '2026-09-01' }, today)).toBeNull()
    expect(dueInfo({ ...base, status: 'paid', due_on: '2026-09-01' }, today)?.tone).toBe('success')
  })
})

describe('plural', () => {
  it('uses Czech forms', () => {
    expect([1, 2, 4, 5, 0].map((n) => plural(n, ['den', 'dny', 'dní']))).toEqual(['den', 'dny', 'dny', 'dní', 'dní'])
  })
})

describe('allowedActions', () => {
  const inv = {
    status: 'open' as const,
    document_type: 'invoice' as const,
    paid_amount: 0,
    remaining_amount: 1000,
    payment_count: 0,
  }

  it('open invoice', () => {
    const a = allowedActions(inv)
    for (const x of ['edit', 'delete', 'mark_as_sent', 'cancel', 'mark_as_uncollectible', 'lock', 'correction', 'add_payment', 'duplicate'] as const) {
      expect(a.has(x), x).toBe(true)
    }
    expect(a.has('undo_cancel')).toBe(false)
    expect(a.has('unlock')).toBe(false)
  })

  it('overdue maps to the stored status by sent_at', () => {
    expect(storedStatus({ status: 'overdue' })).toBe('open')
    expect(storedStatus({ status: 'overdue', sent_at: '2026-01-01T00:00:00Z' })).toBe('sent')
    expect(allowedActions({ ...inv, status: 'overdue', sent_at: 'x' }).has('mark_as_sent')).toBe(false)
  })

  it('payments block cancel and delete', () => {
    const a = allowedActions({ ...inv, status: 'sent', payment_count: 1, paid_amount: 500, remaining_amount: 500 })
    expect(a.has('cancel')).toBe(false)
    expect(a.has('delete')).toBe(false)
    expect(a.has('add_payment')).toBe(true)
  })

  it('locked, cancelled, uncollectible', () => {
    const locked = allowedActions({ ...inv, locked_at: 'x' })
    expect(locked.has('edit')).toBe(false)
    expect(locked.has('delete')).toBe(false)
    expect(locked.has('unlock')).toBe(true)
    const cancelled = allowedActions({ ...inv, status: 'cancelled' })
    expect(cancelled.has('undo_cancel')).toBe(true)
    expect(cancelled.has('edit')).toBe(false)
    expect(cancelled.has('add_payment')).toBe(false)
    expect(allowedActions({ ...inv, status: 'uncollectible' }).has('undo_uncollectible')).toBe(true)
  })

  it('paid invoice and proforma', () => {
    const paid = allowedActions({ ...inv, status: 'paid', payment_count: 1, paid_amount: 1000, remaining_amount: 0 })
    expect(paid.has('add_payment')).toBe(false)
    expect(paid.has('mark_as_uncollectible')).toBe(false)
    expect(paid.has('edit')).toBe(true)
    expect(allowedActions({ ...inv, document_type: 'proforma' }).has('correction')).toBe(false)
  })
})
