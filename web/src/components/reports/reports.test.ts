import { describe, expect, it } from 'vitest'
import type { IncomeTax, VatReturn, VatWarning } from '@/api/types'
import { axisDomain, customerShare, niceCeil, taxOptions } from './logic'
import { translateVatWarning, vatBalance, vatReturnSections } from './vat-rows'

const zeroPair = { base: 0, vat: 0 }
const emptyReturn: VatReturn = {
  r1: zeroPair,
  r2: zeroPair,
  r3: zeroPair,
  r4: zeroPair,
  r5: zeroPair,
  r6: zeroPair,
  r10: zeroPair,
  r11: zeroPair,
  r12: zeroPair,
  r13: zeroPair,
  r20: 0,
  r21: 0,
  r25: 0,
  r26: 0,
  r40: zeroPair,
  r41: zeroPair,
  r43: zeroPair,
  r44: zeroPair,
  r46: 0,
  r62: 0,
  r63: 0,
  r64: 0,
  r65: 0,
}

describe('chart axis', () => {
  it('rounds up to nice values', () => {
    expect(niceCeil(0)).toBe(0)
    expect(niceCeil(83)).toBe(100)
    expect(niceCeil(1_300)).toBe(2_000)
    expect(niceCeil(2_100)).toBe(2_500)
  })
  it('includes zero and negative values', () => {
    expect(axisDomain([100_00, 50_00])).toEqual({ min: 0, max: 100, ticks: [0, 25, 50, 75, 100] })
    const d = axisDomain([100_00, -30_00])
    expect(d.min).toBeLessThan(0)
    expect(d.ticks).toContain(0)
    expect(d.max).toBeGreaterThanOrEqual(100)
  })
  it('handles an empty year', () => {
    expect(axisDomain([0, 0]).max).toBe(1)
  })
})

describe('top customers', () => {
  it('computes the share', () => {
    expect(customerShare(25_00, 100_00)).toBe(0.25)
    expect(customerShare(5, 0)).toBe(0)
  })
})

describe('taxOptions', () => {
  const tax: IncomeTax = {
    income: 1_000_000_00,
    real_expenses: 200_000_00,
    real_tax_base: 800_000_00,
    invalid_rates: 0,
    rate_basis: 'document',
    flat_rates: [
      { percent: 80, cap: 1_600_000_00, expenses: 800_000_00, tax_base: 200_000_00 },
      { percent: 60, cap: 1_200_000_00, expenses: 600_000_00, tax_base: 400_000_00 },
      { percent: 40, cap: 800_000_00, expenses: 400_000_00, tax_base: 600_000_00 },
      { percent: 30, cap: 600_000_00, expenses: 300_000_00, tax_base: 700_000_00 },
    ],
  }
  it('picks the lowest tax base', () => {
    const { options, bestId } = taxOptions(tax)
    expect(options).toHaveLength(5)
    expect(bestId).toBe('flat-80')
  })
  it('prefers real expenses when they are higher', () => {
    expect(taxOptions({ ...tax, real_expenses: 900_000_00, real_tax_base: 100_000_00 }).bestId).toBe('real')
  })
  it('marks reached caps and has no best option without income', () => {
    const capped = taxOptions({
      ...tax,
      flat_rates: [{ percent: 30, cap: 600_000_00, expenses: 600_000_00, tax_base: 1_400_000_00 }],
    })
    expect(capped.options[1]?.capReached).toBe(true)
    expect(taxOptions({ ...tax, income: 0 }).bestId).toBeNull()
  })
})

describe('VAT return rows', () => {
  it('lists every line in form order', () => {
    const lines = vatReturnSections(emptyReturn).flatMap((s) => s.rows.map((r) => r.line))
    expect(lines).toEqual([
      '1', '2', '3', '4', '5', '6', '10', '11', '12', '13', '20', '21', '25', '26', '40', '41', '43', '44', '46',
      '62', '63', '64', '65',
    ])
  })
  it('hides empty rows but keeps totals', () => {
    const r = { ...emptyReturn, r1: { base: 1000_00, vat: 210_00 }, r62: 210_00, r64: 210_00 }
    const lines = vatReturnSections(r, { hideEmpty: true }).flatMap((s) => s.rows.map((x) => x.line))
    expect(lines).toEqual(['1', '62', '63', '64', '65'])
  })
  it('shows non-empty self-assessment rows with official names', () => {
    const r = { ...emptyReturn, r5: { base: 1000_00, vat: 210_00 }, r43: { base: 1000_00, vat: 210_00 }, r20: 500_00 }
    const rows = vatReturnSections(r, { hideEmpty: true }).flatMap((s) => s.rows)
    expect(rows.map((x) => x.line)).toEqual(['5', '20', '43', '62', '63', '64', '65'])
    expect(rows[0]?.label).toMatch(/^Přijetí služby od osoby registrované v jiném členském státě/)
    expect(rows[1]).toMatchObject({ label: 'Dodání zboží do jiného členského státu', base: 500_00 })
    expect(rows[2]?.label).toMatch(/^Odpočet z plnění ř\. 3–13/)
  })
  it('computes the balance', () => {
    expect(vatBalance({ ...emptyReturn, r64: 500 })).toBe(500)
    expect(vatBalance({ ...emptyReturn, r65: 300 })).toBe(-300)
  })
})

describe('translateVatWarning', () => {
  const w = (code: string, document: string, params?: Record<string, string>) =>
    ({ code, document, message: `${document}: english`, params }) as VatWarning
  it.each([
    [w('unsupported_rate', '2026-0001', { rate: '15.00' }), '2026-0001: sazba 15 % se do přiznání nezahrnuje (jen 21 % a 12 %).'],
    [w('unsupported_rate', '2026-0001', { rate: '10.50' }), '2026-0001: sazba 10,5 % se do přiznání nezahrnuje (jen 21 % a 12 %).'],
    [w('reverse_charge_no_dic', 'F1'), 'F1: přenesená daňová povinnost, ale odběratel nemá české DIČ.'],
    [w('eu_reverse_charge_no_vat', 'F2'), 'F2: služba do EU bez DIČ odběratele.'],
    [w('zero_rate_not_reported', 'F3', { amount: '1500.00' }), 'F3: částka s nulovou sazbou (1500,00 Kč) se v přiznání neuvádí.'],
    [w('missing_taxable_date', 'F4'), 'F4: chybí DUZP — v přiznání je podle data vystavení. Doplňte DUZP.'],
    [w('correction_of_cancelled', 'D1'), 'D1: opravuje stornovanou fakturu, původní doklad v přiznání není.'],
    [w('ec_sales_list', ''), 'Plnění do jiných států EU (ř. 20 a 21) podejte také v souhrnném hlášení.'],
  ])('%o', (input, expected) => {
    expect(translateVatWarning(input)).toBe(expected)
  })
  it('translates every new code', () => {
    for (const code of [
      'possible_reverse_charge',
      'reverse_charge_import',
      'reverse_charge_subject_code',
      'control_statement_monthly',
    ]) {
      expect(translateVatWarning(w(code, 'N1'))).not.toContain('english')
    }
  })
  it('falls back to the message for unknown codes', () => {
    expect(translateVatWarning(w('something', 'A'))).toBe('A: english')
  })
})
