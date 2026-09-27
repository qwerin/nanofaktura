// Formulář nákladu: Zod schéma, výchozí hodnoty a převod na API vstup (čisté funkce, testované).

import { z } from 'zod'
import type { Account, CreateExpenseInput, Expense, ExpensePaymentMethod, UpdateExpenseInput } from '@/api/types'
import { addDays, parseISODate, todayISO } from '@/lib/date'
import { parseMoney } from '@/lib/money'
import { parseQuantity } from '@/lib/quantity'
import { draftToLineInput, emptyLine, lineToDraft, type LineDraft } from './calc'

const PAYMENT_METHODS = ['bank', 'cash', 'card', 'cod', 'paypal', 'custom'] as const satisfies readonly ExpensePaymentMethod[]

const optionalDate = z.string().refine((v) => v === '' || parseISODate(v) !== null, 'Neplatné datum')

export const lineSchema = z.object({
  id: z.number().optional(),
  price_item_id: z.number().optional(),
  name: z.string().trim().min(1, 'Zadejte název položky').max(500, 'Nejvýše 500 znaků'),
  quantity: z.string().refine((v) => parseQuantity(v) !== null, 'Neplatné množství'),
  unit_name: z.string().max(50, 'Nejvýše 50 znaků'),
  unit_price: z.string().refine((v) => parseMoney(v) !== null, 'Neplatná cena'),
  vat_rate_bps: z.string(),
}) satisfies z.ZodType<LineDraft>

export const expenseFormSchema = z
  .object({
    number: z.string().trim().max(50, 'Nejvýše 50 znaků'),
    subject_id: z.number().optional(),
    supplier_name: z.string().trim().min(1, 'Zadejte dodavatele'),
    supplier_registration_no: z.string().trim(),
    supplier_vat_no: z.string().trim(),
    original_number: z.string().trim().max(100, 'Nejvýše 100 znaků'),
    variable_symbol: z.string().trim().regex(/^\d{0,10}$/, 'Nejvýše 10 číslic'),
    issued_on: z.string().refine((v) => parseISODate(v) !== null, 'Zadejte datum'),
    taxable_fulfillment_due: optionalDate,
    due_on: optionalDate,
    category: z.string().trim().max(100, 'Nejvýše 100 znaků'),
    description: z.string().trim(),
    private_note: z.string(),
    payment_method: z.enum(PAYMENT_METHODS),
    currency: z.string().min(3),
    exchange_rate: z.string().trim(),
    tax_deductible: z.boolean(),
    prices_include_vat: z.boolean(),
    round_total: z.boolean(),
    lines: z.array(lineSchema).min(1, 'Přidejte aspoň jednu položku'),
  })
  .superRefine((v, ctx) => {
    if (v.exchange_rate !== '' && !/^\d+([.,]\d{1,6})?$/.test(v.exchange_rate)) {
      ctx.addIssue({ code: 'custom', path: ['exchange_rate'], message: 'Kurz ve tvaru 24,355' })
    }
    if (v.due_on && v.issued_on && v.due_on < v.issued_on) {
      ctx.addIssue({ code: 'custom', path: ['due_on'], message: 'Splatnost je před datem vystavení' })
    }
  })

export type ExpenseFormValues = z.infer<typeof expenseFormSchema>

/** Výchozí hodnoty nového nákladu podle nastavení účtu. */
export function newExpenseDefaults(account: Pick<Account, 'default_currency' | 'default_due_days' | 'default_payment_method' | 'default_vat_rate_bps'> | undefined, today = todayISO()): ExpenseFormValues {
  return {
    number: '',
    subject_id: undefined,
    supplier_name: '',
    supplier_registration_no: '',
    supplier_vat_no: '',
    original_number: '',
    variable_symbol: '',
    issued_on: today,
    taxable_fulfillment_due: today,
    due_on: addDays(today, account?.default_due_days ?? 14),
    category: '',
    description: '',
    private_note: '',
    payment_method: account?.default_payment_method ?? 'bank',
    currency: account?.default_currency || 'CZK',
    exchange_rate: '',
    tax_deductible: true,
    prices_include_vat: true,
    round_total: false,
    lines: [emptyLine(account?.default_vat_rate_bps ?? 2100)],
  }
}

/** Uložený náklad → hodnoty formuláře (úprava). */
export function expenseToFormValues(e: Expense): ExpenseFormValues {
  const method = (PAYMENT_METHODS as readonly string[]).includes(e.payment_method)
    ? (e.payment_method as ExpensePaymentMethod)
    : 'custom'
  return {
    number: e.number,
    subject_id: e.subject_id,
    supplier_name: e.supplier_name,
    supplier_registration_no: e.supplier_registration_no,
    supplier_vat_no: e.supplier_vat_no,
    original_number: e.original_number,
    variable_symbol: e.variable_symbol,
    issued_on: e.issued_on,
    taxable_fulfillment_due: e.taxable_fulfillment_due,
    due_on: e.due_on,
    category: e.category,
    description: e.description,
    private_note: e.private_note,
    payment_method: method,
    currency: e.currency,
    exchange_rate: e.exchange_rate && e.exchange_rate !== '1' ? e.exchange_rate.replace('.', ',') : '',
    tax_deductible: e.tax_deductible,
    prices_include_vat: e.prices_include_vat,
    round_total: e.round_total,
    lines: e.lines.length ? e.lines.map(lineToDraft) : [emptyLine(2100)],
  }
}

function supplierFields(v: ExpenseFormValues) {
  // Kontakt z adresáře: backend snapshotuje supplier_* sám. Volný text: posíláme, co uživatel vyplnil.
  if (v.subject_id) return { subject_id: v.subject_id }
  return {
    supplier_name: v.supplier_name.trim(),
    supplier_registration_no: v.supplier_registration_no.trim(),
    supplier_vat_no: v.supplier_vat_no.trim().toUpperCase(),
  }
}

function commonFields(v: ExpenseFormValues, defaultCurrency: string) {
  const foreign = v.currency !== defaultCurrency
  return {
    original_number: v.original_number.trim(),
    variable_symbol: v.variable_symbol.trim(),
    issued_on: v.issued_on,
    taxable_fulfillment_due: v.taxable_fulfillment_due,
    ...(v.due_on ? { due_on: v.due_on } : {}),
    category: v.category.trim(),
    description: v.description.trim(),
    private_note: v.private_note,
    payment_method: v.payment_method,
    currency: v.currency,
    exchange_rate: foreign && v.exchange_rate ? v.exchange_rate.replace(',', '.') : '1',
    tax_deductible: v.tax_deductible,
    prices_include_vat: v.prices_include_vat,
    round_total: v.round_total,
    lines: v.lines.map(draftToLineInput),
  }
}

export function toCreateExpenseInput(v: ExpenseFormValues, defaultCurrency: string): CreateExpenseInput {
  return {
    ...supplierFields(v),
    ...commonFields(v, defaultCurrency),
    ...(v.number.trim() ? { number: v.number.trim() } : {}),
  }
}

export function toUpdateExpenseInput(v: ExpenseFormValues, defaultCurrency: string): UpdateExpenseInput {
  return {
    ...supplierFields(v),
    ...commonFields(v, defaultCurrency),
    ...(v.number.trim() ? { number: v.number.trim() } : {}),
  }
}
