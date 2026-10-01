import { describe, expect, it } from 'vitest'
import type { Account } from '@/api/types'
import {
  computeDueOn,
  convertPrice,
  invoiceFormSchema,
  invoiceFormSchemaFor,
  newInvoiceValues,
  toCreateBody,
  toPatchBody,
} from './form-model'

const account = {
  vat_mode: 'vat_payer',
  default_due_days: 14,
  default_currency: 'CZK',
  default_language: 'cs',
  default_payment_method: 'bank',
  default_note: 'Fakturujeme',
  default_footer_note: '',
  default_vat_rate_bps: 2100,
  round_total: false,
} as Account

describe('invoice form model', () => {
  it('builds defaults from the account', () => {
    const v = newInvoiceValues(account, { today: '2026-09-26', bankAccountId: 3 })
    expect(v.taxable_fulfillment_due).toBe('2026-09-26')
    expect(v.bank_account_id).toBe('3')
    expect(v.lines[0]?.vat_rate_bps).toBe('2100')
    const nonPayer = newInvoiceValues({ ...account, vat_mode: 'non_vat_payer' }, { today: '2026-09-26' })
    expect(nonPayer.taxable_fulfillment_due).toBe('')
    expect(nonPayer.lines[0]?.vat_rate_bps).toBe('0')
  })

  it('validates required fields', () => {
    const v = newInvoiceValues(account, { today: '2026-09-26' })
    const r = invoiceFormSchema.safeParse(v)
    expect(r.success).toBe(false)
    const paths = r.error?.issues.map((i) => i.path.join('.'))
    expect(paths).toContain('subject')
    expect(paths).toContain('lines.0.name')
    expect(paths).toContain('lines.0.unit_price')
  })

  it('maps values to a create body', () => {
    const v = {
      ...newInvoiceValues(account, { today: '2026-09-26' }),
      subject: { id: 7, name: 'ACME' },
      exchange_rate: '1',
      lines: [{ name: ' Práce ', quantity: '1,5', unit_name: 'h', unit_price: '1 000,50', vat_rate_bps: '1200' }],
    }
    expect(invoiceFormSchema.safeParse(v).success).toBe(true)
    const body = toCreateBody(v, true)
    expect(body.subject_id).toBe(7)
    expect(body.due_days).toBe(14)
    expect(body.taxable_fulfillment_due).toBe('2026-09-26')
    expect(body.lines).toEqual([{ name: 'Práce', quantity: '1.5', unit_name: 'h', unit_price: 100050, vat_rate_bps: 1200 }])
    expect(body.number).toBeUndefined()
    expect(toCreateBody(v, false).taxable_fulfillment_due).toBeUndefined()
  })

  it('patch contains only dirty fields', () => {
    const v = {
      ...newInvoiceValues(account, { today: '2026-09-26' }),
      subject: { id: 7, name: 'ACME' },
      lines: [{ id: 5, name: 'A', quantity: '1', unit_name: '', unit_price: '10', vat_rate_bps: '2100' }],
    }
    expect(toPatchBody(v, { subject: true, lines: [{ name: true }] }, true)).toEqual({
      subject_id: 7,
      lines: [{ id: 5, name: 'A', quantity: '1', unit_name: '', unit_price: 1000, vat_rate_bps: 2100 }],
    })
    expect(toPatchBody({ ...v, variable_symbol: '' }, { variable_symbol: true, number: true }, true)).toEqual({
      variable_symbol: '',
    })
  })

  it('VAT payer rules: DUZP, correction reason, exchange rate', () => {
    const base = {
      ...newInvoiceValues(account, { today: '2026-09-26' }),
      subject: { id: 7, name: 'ACME' },
      lines: [{ name: 'Práce', quantity: '1', unit_name: 'h', unit_price: '100', vat_rate_bps: '2100' }],
    }
    const issues = (mode: Parameters<typeof invoiceFormSchemaFor>[0], v: typeof base) =>
      invoiceFormSchemaFor(mode).safeParse(v).error?.issues.map((i) => i.path.join('.')) ?? []

    expect(issues('vat_payer', { ...base, taxable_fulfillment_due: '' })).toEqual(['taxable_fulfillment_due'])
    expect(issues('non_vat_payer', { ...base, taxable_fulfillment_due: '' })).toEqual([])
    // identifikovaná osoba: DUZP jen u přenesení daňové povinnosti
    expect(issues('identified_person', { ...base, taxable_fulfillment_due: '' })).toEqual([])
    expect(issues('identified_person', { ...base, taxable_fulfillment_due: '', reverse_charge: true })).toEqual(['taxable_fulfillment_due'])

    const correction = { ...base, document_type: 'correction' as const, related_id: '3' }
    expect(issues('vat_payer', correction)).toEqual(['correction_reason'])
    expect(issues('non_vat_payer', correction)).toEqual([])
    expect(issues('vat_payer', { ...correction, correction_reason: 'Vrácení zboží' })).toEqual([])
    expect(toCreateBody({ ...correction, correction_reason: ' Sleva z ceny ' }, true).correction_reason).toBe('Sleva z ceny')

    expect(issues('vat_payer', { ...base, exchange_rate: '0' })).toEqual(['exchange_rate'])
    expect(issues('vat_payer', { ...base, exchange_rate: '0,000' })).toEqual(['exchange_rate'])
    expect(issues('vat_payer', { ...base, exchange_rate: '24,5' })).toEqual([])
  })

  it('sends supply_type only with reverse charge', () => {
    const v = { ...newInvoiceValues(account, { today: '2026-09-26' }), subject: { id: 7, name: 'ACME' }, supply_type: 'goods' as const }
    expect(toCreateBody(v, true).supply_type).toBeUndefined()
    expect(toCreateBody({ ...v, reverse_charge: true }, true).supply_type).toBe('goods')
    expect(toCreateBody({ ...v, reverse_charge: true }, false).supply_type).toBeUndefined()
  })

  it('computes the due date', () => {
    expect(computeDueOn('2026-09-26', '14')).toBe('2026-10-10')
    expect(computeDueOn('2026-09-26', 'x')).toBeNull()
  })
})

describe('convertPrice', () => {
  it('converts between gross and net prices', () => {
    expect(convertPrice(12100, true, false, 2100)).toBe(10000)
    expect(convertPrice(10000, false, true, 2100)).toBe(12100)
    expect(convertPrice(999, false, false, 2100)).toBe(999)
    expect(convertPrice(100, true, false, 0)).toBe(100)
  })
})
