import { describe, expect, it } from 'vitest'
import type { Account, InvoiceTemplate, Recurring } from '@/api/types'
import {
  missedOccurrences,
  newRecurringValues,
  recurringToValues,
  toCreateRecurringBody,
  toPatchRecurringBody,
} from './recurring-model'
import { templateToValues, templateTotal, toTemplateBody } from './template-model'

const account = {
  vat_mode: 'vat_payer',
  default_vat_rate_bps: 2100,
  round_total: false,
  default_currency: 'CZK',
  default_language: 'cs',
  default_payment_method: 'bank',
  default_note: 'Fakturujeme:',
  default_footer_note: '',
  default_due_days: 14,
} as Account

const template = {
  id: 1,
  name: 'Paušál',
  document_type: 'invoice',
  subject_id: 7,
  currency: '',
  exchange_rate: '',
  language: '',
  payment_method: '',
  custom_payment_method: '',
  order_number: '',
  private_note: '',
  tags: ['it'],
  prices_include_vat: false,
  reverse_charge: false,
  lines: [
    { name: 'Správa za {MONTH_NAME}', quantity: '1.5', unit_name: 'h', unit_price: 100000 },
    { name: 'Hosting', quantity: '1', unit_name: '', unit_price: 50000, vat_rate_bps: 1200 },
  ],
} as unknown as InvoiceTemplate

describe('template model', () => {
  it('fills empty template fields from account defaults', () => {
    const v = templateToValues(template, account, { id: 7, name: 'ACME' })
    expect(v.currency).toBe('CZK')
    expect(v.payment_method).toBe('bank')
    expect(v.note).toBe('Fakturujeme:')
    expect(v.due_days).toBe('')
    expect(v.lines[0]).toMatchObject({ quantity: '1,5', unit_price: '1000,00', vat_rate_bps: '2100' })
  })
  it('round-trips to an API body', () => {
    const body = toTemplateBody(templateToValues(template, account, { id: 7, name: 'ACME' }), true)
    expect(body.subject_id).toBe(7)
    expect(body.due_days).toBeUndefined()
    expect(body.lines?.[0]).toEqual({ name: 'Správa za {MONTH_NAME}', quantity: '1.5', unit_name: 'h', unit_price: 100000, vat_rate_bps: 2100 })
    expect(body.lines?.[1]?.vat_rate_bps).toBe(1200)
  })
  it('computes the total with VAT', () => {
    // 1500 + 21 % = 1815; 500 + 12 % = 560
    expect(templateTotal(template, account)).toBe(237500)
    expect(templateTotal(template, { ...account, vat_mode: 'non_vat_payer' })).toBe(200000)
  })
})

describe('recurring model', () => {
  it('builds create body without empty optionals', () => {
    const v = { ...newRecurringValues('2026-09-27', 3), name: ' Paušál ' }
    expect(toCreateRecurringBody(v)).toEqual({
      name: 'Paušál',
      template_id: 3,
      start_on: '2026-09-27',
      months_period: 1,
      issue_as: 'invoice',
      send_email: false,
      active: true,
    })
  })
  it('patches only dirty fields and clears day/end', () => {
    const r = { name: 'X', template_id: 3, start_on: '2026-01-01', months_period: 3, day_of_month: 15, end_on: '2027-01-01', issue_as: 'invoice', send_email: true, active: true } as Recurring
    const v = { ...recurringToValues(r), day_of_month: '', end_on: '' }
    expect(toPatchRecurringBody(v, { day_of_month: true, end_on: true })).toEqual({ day_of_month: 0, end_on: '' })
  })
  it('counts catch-up occurrences for a start in the past', () => {
    expect(missedOccurrences({ start: '2026-07-01', monthsPeriod: 1 }, '2026-09-27')).toBe(3)
    expect(missedOccurrences({ start: '2026-09-27', monthsPeriod: 1 }, '2026-09-27')).toBe(1)
    expect(missedOccurrences({ start: '2026-10-01', monthsPeriod: 1 }, '2026-09-27')).toBe(0)
  })
})
