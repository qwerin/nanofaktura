import { describe, expect, it } from 'vitest'
import type { PriceItem } from '@/api/types'
import {
  calculateTotals,
  convertUnitPrice,
  draftToLineInput,
  draftTotals,
  isBlankLine,
  lineFromPriceItem,
  lineToDraft,
  type LineDraft,
} from './calc'

describe('calculateTotals', () => {
  it('net prices: VAT per rate from the summed base', () => {
    const t = calculateTotals(
      [
        { unitPrice: 10000, quantity: '1.5', vatRateBps: 2100 },
        { unitPrice: 333, quantity: '3', vatRateBps: 2100 },
        { unitPrice: 5000, quantity: '1', vatRateBps: 1200 },
      ],
      { pricesIncludeVat: false, roundTotal: false },
    )
    expect(t.lines.map((l) => l.base)).toEqual([15000, 999, 5000])
    expect(t.recap).toEqual([
      { vatRateBps: 2100, base: 15999, vat: 3360, total: 19359 },
      { vatRateBps: 1200, base: 5000, vat: 600, total: 5600 },
    ])
    expect(t.subtotal).toBe(20999)
    expect(t.vatTotal).toBe(3960)
    expect(t.total).toBe(24959)
    expect(t.rounding).toBe(0)
  })

  it('gross prices: VAT extracted from the total', () => {
    const t = calculateTotals([{ unitPrice: 12100, quantity: '1', vatRateBps: 2100 }], { pricesIncludeVat: true, roundTotal: false })
    expect(t.recap[0]).toEqual({ vatRateBps: 2100, base: 10000, vat: 2100, total: 12100 })
    expect(t.total).toBe(12100)
  })

  it('reverse charge: no VAT on the document, rates kept in the recap', () => {
    const t = calculateTotals([{ unitPrice: 10000, quantity: '1', vatRateBps: 2100 }], {
      pricesIncludeVat: false,
      roundTotal: false,
      reverseCharge: true,
    })
    expect(t.recap).toEqual([{ vatRateBps: 2100, base: 10000, vat: 0, total: 10000 }])
    expect(t.total).toBe(10000)
  })

  it('round_total rounds half away from zero to whole units', () => {
    const t = calculateTotals([{ unitPrice: 10050, quantity: '1', vatRateBps: 0 }], { pricesIncludeVat: true, roundTotal: true })
    expect(t.total).toBe(10100)
    expect(t.rounding).toBe(50)
    const neg = calculateTotals([{ unitPrice: -10050, quantity: '1', vatRateBps: 0 }], { pricesIncludeVat: true, roundTotal: true })
    expect(neg.total).toBe(-10100)
    expect(neg.rounding).toBe(-50)
  })

  it('handles negative quantities and empty input', () => {
    const t = calculateTotals([{ unitPrice: 1000, quantity: '-2', vatRateBps: 2100 }], { pricesIncludeVat: false, roundTotal: false })
    expect(t.total).toBe(-2420)
    expect(calculateTotals([], { pricesIncludeVat: false, roundTotal: false }).total).toBe(0)
  })
})

const draft = (patch: Partial<LineDraft> = {}): LineDraft => ({
  name: 'Papír',
  quantity: '2',
  unit_name: 'ks',
  unit_price: '121,00',
  vat_rate_bps: '2100',
  ...patch,
})

describe('drafts', () => {
  it('invalid drafts count as zero in live totals', () => {
    const t = draftTotals([draft(), draft({ unit_price: 'abc' })], { pricesIncludeVat: true, roundTotal: false })
    expect(t.total).toBe(24200)
    expect(t.lines[1]?.total).toBe(0)
  })

  it('converts a draft to API input', () => {
    expect(draftToLineInput(draft({ id: 7, price_item_id: 3, quantity: '1,5', name: '  Papír ' }))).toEqual({
      id: 7,
      price_item_id: 3,
      name: 'Papír',
      quantity: '1.5',
      unit_name: 'ks',
      unit_price: 12100,
      vat_rate_bps: 2100,
    })
    expect(() => draftToLineInput(draft({ quantity: 'x' }))).toThrow()
  })

  it('round-trips a saved line', () => {
    expect(lineToDraft({ id: 1, name: 'A', quantity: '1.25', unit_name: 'h', unit_price: 123450, vat_rate_bps: 1200 })).toEqual({
      id: 1,
      name: 'A',
      quantity: '1,25',
      unit_name: 'h',
      unit_price: '1234,50',
      vat_rate_bps: '1200',
    })
  })

  it('detects blank lines', () => {
    expect(isBlankLine(draft({ name: ' ', unit_price: '' }))).toBe(true)
    expect(isBlankLine(draft({ name: '', unit_price: '', price_item_id: 1 }))).toBe(false)
    expect(isBlankLine(draft())).toBe(false)
  })
})

describe('price items', () => {
  it('converts unit prices between gross and net', () => {
    expect(convertUnitPrice(10000, 2100, false, true)).toBe(12100)
    expect(convertUnitPrice(12100, 2100, true, false)).toBe(10000)
    expect(convertUnitPrice(9999, 2100, true, false)).toBe(8264) // 8263.6 → 8264
    expect(convertUnitPrice(5000, 0, false, true)).toBe(5000)
    expect(convertUnitPrice(5000, 2100, true, true)).toBe(5000)
  })

  it('fills a line from a price item, keeping id and quantity of the target line', () => {
    const item = { id: 9, name: 'Kabel', unit_name: 'ks', unit_price: 10000, vat_rate_bps: 2100, prices_include_vat: false } as PriceItem
    expect(lineFromPriceItem(item, true, draft({ id: 4, quantity: '3' }))).toEqual({
      id: 4,
      price_item_id: 9,
      name: 'Kabel',
      quantity: '3',
      unit_name: 'ks',
      unit_price: '121,00',
      vat_rate_bps: '2100',
    })
    expect(lineFromPriceItem(item, false).quantity).toBe('1')
  })
})
