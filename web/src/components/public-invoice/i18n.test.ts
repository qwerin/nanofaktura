import { describe, expect, it } from 'vitest'
import {
  amountForCopy,
  documentTitle,
  formatAmount,
  formatIban,
  formatPublicDate,
  paymentState,
  publicLabels,
  publicLang,
} from './i18n'

const cs = publicLabels('cs')
const en = publicLabels('en')

describe('publicLang', () => {
  it.each([
    ['cs', 'cs'],
    ['sk', 'cs'],
    ['', 'cs'],
    ['en', 'en'],
    ['de', 'en'],
  ])('%s → %s', (input, expected) => {
    expect(publicLang(input)).toBe(expected)
  })
})

describe('paymentState', () => {
  const base = { status: 'open' as const, cancelled: false, remaining_amount: 100_00, total: 100_00, due_on: '2026-10-01' }
  it('open with days until due', () => {
    expect(paymentState(base, '2026-09-27')).toEqual({ state: 'open', days: 4 })
  })
  it('overdue by date even when status lags', () => {
    expect(paymentState(base, '2026-10-03')).toEqual({ state: 'overdue', days: -2 })
  })
  it('paid, cancelled, refund, uncollectible', () => {
    expect(paymentState({ ...base, status: 'paid', remaining_amount: 0 }, '2026-09-27').state).toBe('paid')
    expect(paymentState({ ...base, cancelled: true, status: 'cancelled' }, '2026-09-27').state).toBe('cancelled')
    expect(paymentState({ ...base, remaining_amount: -50_00, total: -50_00 }, '2026-09-27').state).toBe('refund')
    expect(paymentState({ ...base, status: 'uncollectible' }, '2026-09-27').state).toBe('uncollectible')
  })
})

describe('formatting', () => {
  it('formats amounts per language', () => {
    expect(formatAmount(123450, 'CZK', cs).replace(/\s/g, ' ')).toBe('1 234,50 Kč')
    expect(formatAmount(123450, 'EUR', en)).toBe('€1,234.50')
  })
  it('amount for copying has no grouping', () => {
    expect(amountForCopy(123450, cs)).toBe('1234,50')
    expect(amountForCopy(123405, en)).toBe('1234.05')
    expect(amountForCopy(-5, cs)).toBe('-0,05')
  })
  it('formats dates', () => {
    expect(formatPublicDate('2026-09-27', cs).replace(/\s/g, ' ')).toBe('27. 9. 2026')
    expect(formatPublicDate('2026-09-27', en)).toBe('27 Sept 2026')
    expect(formatPublicDate('', cs)).toBe('')
  })
  it('groups IBAN', () => {
    expect(formatIban('CZ6508000000192000145399')).toBe('CZ65 0800 0000 1920 0014 5399')
  })
})

describe('documentTitle', () => {
  const payer = { name: '', street: '', city: '', zip: '', country: 'CZ', registration_no: '', vat_no: '', vat_mode: 'vat_payer' as const }
  it('depends on type and VAT mode', () => {
    expect(documentTitle({ document_type: 'invoice', supplier: payer }, cs)).toBe('Faktura – daňový doklad')
    expect(documentTitle({ document_type: 'invoice', supplier: { ...payer, vat_mode: 'non_vat_payer' } }, cs)).toBe('Faktura')
    expect(documentTitle({ document_type: 'proforma', supplier: payer }, en)).toBe('Proforma invoice')
    expect(documentTitle({ document_type: 'correction', supplier: payer }, cs)).toBe('Opravný daňový doklad')
  })
})
