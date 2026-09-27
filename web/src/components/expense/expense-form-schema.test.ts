import { describe, expect, it } from 'vitest'
import type { Expense } from '@/api/types'
import {
  expenseFormSchema,
  expenseToFormValues,
  newExpenseDefaults,
  toCreateExpenseInput,
  toUpdateExpenseInput,
  type ExpenseFormValues,
} from './expense-form-schema'

const account = { default_currency: 'CZK', default_due_days: 14, default_payment_method: 'bank', default_vat_rate_bps: 2100 } as const

function valid(patch: Partial<ExpenseFormValues> = {}): ExpenseFormValues {
  return {
    ...newExpenseDefaults(account, '2026-09-26'),
    supplier_name: 'Papírnictví U Nováků',
    lines: [{ name: 'Papír A4', quantity: '2', unit_name: 'bal', unit_price: '129,90', vat_rate_bps: '2100' }],
    ...patch,
  }
}

describe('newExpenseDefaults', () => {
  it('uses account defaults and computes due date', () => {
    const d = newExpenseDefaults(account, '2026-09-26')
    expect(d.issued_on).toBe('2026-09-26')
    expect(d.taxable_fulfillment_due).toBe('2026-09-26')
    expect(d.due_on).toBe('2026-10-10')
    expect(d.lines).toHaveLength(1)
    expect(d.lines[0]?.vat_rate_bps).toBe('2100')
    expect(d.tax_deductible).toBe(true)
  })
})

describe('expenseFormSchema', () => {
  it('accepts a valid expense', () => {
    expect(expenseFormSchema.safeParse(valid()).success).toBe(true)
  })

  it('reports field paths incl. lines', () => {
    const r = expenseFormSchema.safeParse(
      valid({
        supplier_name: ' ',
        variable_symbol: '12345678901',
        exchange_rate: 'abc',
        due_on: '2026-09-01',
        lines: [{ name: '', quantity: '1,2345', unit_name: '', unit_price: 'x', vat_rate_bps: '2100' }],
      }),
    )
    expect(r.success).toBe(false)
    const paths = r.error?.issues.map((i) => i.path.join('.')).sort()
    expect(paths).toEqual(['due_on', 'exchange_rate', 'lines.0.name', 'lines.0.quantity', 'lines.0.unit_price', 'supplier_name', 'variable_symbol'])
  })

  it('requires at least one line', () => {
    const r = expenseFormSchema.safeParse(valid({ lines: [] }))
    expect(r.error?.issues[0]?.path).toEqual(['lines'])
  })
})

describe('to API input', () => {
  it('free-text supplier sends supplier_* and default exchange rate', () => {
    const input = toCreateExpenseInput(valid({ supplier_vat_no: 'cz123', exchange_rate: '25' }), 'CZK')
    expect(input).toMatchObject({
      supplier_name: 'Papírnictví U Nováků',
      supplier_vat_no: 'CZ123',
      exchange_rate: '1',
      lines: [{ name: 'Papír A4', quantity: '2', unit_name: 'bal', unit_price: 12990, vat_rate_bps: 2100 }],
    })
    expect(input).not.toHaveProperty('subject_id')
    expect(input).not.toHaveProperty('number')
  })

  it('linked supplier sends only subject_id; foreign currency sends the rate', () => {
    const input = toUpdateExpenseInput(valid({ subject_id: 5, currency: 'EUR', exchange_rate: '24,355', number: 'N2026-0001' }), 'CZK')
    expect(input.subject_id).toBe(5)
    expect(input).not.toHaveProperty('supplier_name')
    expect(input.exchange_rate).toBe('24.355')
    expect(input.number).toBe('N2026-0001')
  })

  it('expenseToFormValues maps a saved expense', () => {
    const e = {
      ...valid(),
      id: 1,
      number: 'N2026-0001',
      subject_id: undefined,
      exchange_rate: '1',
      payment_method: 'weird',
      lines: [{ id: 3, name: 'A', quantity: '1.5', unit_name: 'ks', unit_price: 1000, vat_rate_bps: 2100, base: 0, vat: 0, total: 0, position: 0 }],
    } as unknown as Expense
    const v = expenseToFormValues(e)
    expect(v.payment_method).toBe('custom')
    expect(v.exchange_rate).toBe('')
    expect(v.lines[0]).toMatchObject({ id: 3, quantity: '1,5', unit_price: '10,00' })
  })
})
