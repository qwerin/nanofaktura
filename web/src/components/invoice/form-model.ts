// Model formuláře faktury: Zod schéma, převod faktura ↔ hodnoty formuláře ↔ API tělo.
// Čisté funkce (bez Reactu) — pokryté testy v form-model.test.ts.

import { z } from 'zod'
import type {
  Account,
  CreateInvoiceInput,
  DocumentType,
  Invoice,
  InvoiceLineInput,
  PaymentMethod,
  UpdateInvoiceInput,
} from '@/api/types'
import { addDays, parseISODate } from '@/lib/date'
import { divRoundHalfAway, formatMoneyInput, parseMoney } from '@/lib/money'
import { parseQuantity } from '@/lib/quantity'
import type { CalcLine } from './calc'

const moneyString = z.string().refine((v) => parseMoney(v) !== null, 'Zadejte částku, např. 1 250,50')
const quantityString = z.string().refine((v) => parseQuantity(v) !== null, 'Max. 3 desetinná místa')
const dateString = z.string().refine((v) => parseISODate(v) !== null, 'Zadejte datum')
const optionalDate = z.string().refine((v) => v === '' || parseISODate(v) !== null, 'Neplatné datum')

export const lineSchema = z.object({
  /** ID existujícího řádku (PATCH) — `undefined` u nového. */
  id: z.number().optional(),
  /** Položka ceníku, ze které řádek vznikl. */
  price_item_id: z.number().optional(),
  name: z.string().trim().min(1, 'Zadejte název položky'),
  quantity: quantityString,
  unit_name: z.string(),
  unit_price: moneyString,
  vat_rate_bps: z.string(),
})

export const subjectValueSchema = z.object({
  id: z.number(),
  name: z.string(),
  registration_no: z.string().optional(),
  city: z.string().optional(),
})

export const invoiceFormSchema = z
  .object({
    document_type: z.enum(['invoice', 'proforma', 'correction']),
    related_id: z.string(),
    subject: subjectValueSchema.nullable().refine((v) => v !== null, 'Vyberte odběratele'),
    number: z.string(),
    variable_symbol: z.string().trim().regex(/^\d{0,10}$/, 'Nejvýše 10 číslic'),
    order_number: z.string(),
    issued_on: dateString,
    taxable_fulfillment_due: optionalDate,
    due_days: z.string().trim().regex(/^\d{1,3}$/, 'Počet dní 0–999'),
    currency: z.string().min(3),
    exchange_rate: z.string().trim().regex(/^\d+([.,]\d+)?$/, 'Kurz, např. 24,355'),
    language: z.enum(['cs', 'en', 'sk', 'de']),
    payment_method: z.enum(['bank', 'cash', 'card', 'cod', 'paypal', 'custom']),
    custom_payment_method: z.string(),
    bank_account_id: z.string(),
    note: z.string(),
    footer_note: z.string(),
    private_note: z.string(),
    tags: z.array(z.string()),
    prices_include_vat: z.boolean(),
    round_total: z.boolean(),
    reverse_charge: z.boolean(),
    lines: z.array(lineSchema).min(1, 'Přidejte aspoň jednu položku'),
  })
  .superRefine((v, ctx) => {
    if (v.document_type === 'correction' && !v.related_id) {
      ctx.addIssue({ code: 'custom', path: ['related_id'], message: 'Vyberte opravovanou fakturu' })
    }
  })

export type InvoiceFormValues = z.input<typeof invoiceFormSchema>
export type LineValues = z.input<typeof lineSchema>

export function emptyLine(vatRateBps: number): LineValues {
  return { name: '', quantity: '1', unit_name: '', unit_price: '', vat_rate_bps: String(vatRateBps) }
}

/** Výchozí hodnoty nové faktury z nastavení účtu. */
export function newInvoiceValues(
  account: Account,
  opts: { today: string; documentType?: DocumentType; bankAccountId?: number },
): InvoiceFormValues {
  const payer = account.vat_mode !== 'non_vat_payer'
  return {
    document_type: opts.documentType ?? 'invoice',
    related_id: '',
    subject: null,
    number: '',
    variable_symbol: '',
    order_number: '',
    issued_on: opts.today,
    taxable_fulfillment_due: payer ? opts.today : '',
    due_days: String(account.default_due_days),
    currency: account.default_currency || 'CZK',
    exchange_rate: '1',
    language: account.default_language,
    payment_method: account.default_payment_method || 'bank',
    custom_payment_method: '',
    bank_account_id: opts.bankAccountId ? String(opts.bankAccountId) : '',
    note: account.default_note,
    footer_note: account.default_footer_note,
    private_note: '',
    tags: [],
    prices_include_vat: false,
    round_total: account.round_total,
    reverse_charge: false,
    lines: [emptyLine(payer ? account.default_vat_rate_bps : 0)],
  }
}

/** Hodnoty formuláře z uložené faktury (úprava). */
export function invoiceToValues(inv: Invoice): InvoiceFormValues {
  return {
    document_type: inv.document_type,
    related_id: inv.related_id ? String(inv.related_id) : '',
    subject: { id: inv.subject_id, name: inv.client_name, registration_no: inv.client_registration_no, city: inv.client_city },
    number: inv.number,
    variable_symbol: inv.variable_symbol,
    order_number: inv.order_number,
    issued_on: inv.issued_on,
    taxable_fulfillment_due: inv.taxable_fulfillment_due,
    due_days: String(inv.due_days),
    currency: inv.currency,
    exchange_rate: inv.exchange_rate || '1',
    language: inv.language,
    payment_method: inv.payment_method,
    custom_payment_method: inv.custom_payment_method,
    bank_account_id: inv.bank_account_id ? String(inv.bank_account_id) : '',
    note: inv.note,
    footer_note: inv.footer_note,
    private_note: inv.private_note,
    tags: inv.tags ?? [],
    prices_include_vat: inv.prices_include_vat,
    round_total: inv.round_total,
    reverse_charge: inv.reverse_charge,
    lines: inv.lines.map((l) => ({
      id: l.id,
      price_item_id: l.price_item_id,
      name: l.name,
      quantity: l.quantity.replace('.', ','),
      unit_name: l.unit_name,
      unit_price: formatMoneyInput(l.unit_price),
      vat_rate_bps: String(l.vat_rate_bps),
    })),
  }
}

/** Řádek formuláře → vstup výpočtu (neplatné hodnoty = 0). */
export function toCalcLine(l: Partial<LineValues>): CalcLine {
  return {
    quantity: parseQuantity(l.quantity ?? '') ?? '0',
    unitPrice: parseMoney(l.unit_price ?? '') ?? 0,
    vatRateBps: Number(l.vat_rate_bps) || 0,
  }
}

export function computeDueOn(issuedOn: string, dueDays: string): string | null {
  if (!parseISODate(issuedOn) || !/^\d{1,3}$/.test(dueDays.trim())) return null
  return addDays(issuedOn, Number(dueDays))
}

function lineToInput(l: LineValues, withId: boolean): InvoiceLineInput {
  return {
    ...(withId && l.id ? { id: l.id } : {}),
    ...(l.price_item_id ? { price_item_id: l.price_item_id } : {}),
    name: l.name.trim(),
    quantity: parseQuantity(l.quantity) ?? '1',
    unit_name: l.unit_name.trim(),
    unit_price: parseMoney(l.unit_price) ?? 0,
    vat_rate_bps: Number(l.vat_rate_bps) || 0,
  }
}

type Body = Omit<CreateInvoiceInput, '$schema'>

/** Úplné tělo z hodnot formuláře (create posílá vše, patch jen změněné klíče). */
function fullBody(v: InvoiceFormValues, opts: { payer: boolean; withLineIds: boolean }): Body {
  const body: Body = {
    document_type: v.document_type,
    subject_id: v.subject?.id ?? 0,
    issued_on: v.issued_on,
    due_days: Number(v.due_days),
    currency: v.currency,
    exchange_rate: v.exchange_rate.trim().replace(',', '.'),
    language: v.language,
    payment_method: v.payment_method as PaymentMethod,
    custom_payment_method: v.payment_method === 'custom' ? v.custom_payment_method.trim() : '',
    order_number: v.order_number.trim(),
    note: v.note,
    footer_note: v.footer_note,
    private_note: v.private_note,
    tags: v.tags,
    prices_include_vat: v.prices_include_vat,
    round_total: v.round_total,
    reverse_charge: opts.payer ? v.reverse_charge : false,
    lines: v.lines.map((l) => lineToInput(l, opts.withLineIds)),
  }
  if (opts.payer) body.taxable_fulfillment_due = v.taxable_fulfillment_due
  if (v.bank_account_id) body.bank_account_id = Number(v.bank_account_id)
  if (v.related_id) body.related_id = Number(v.related_id)
  if (v.number.trim()) body.number = v.number.trim()
  if (v.variable_symbol.trim()) body.variable_symbol = v.variable_symbol.trim()
  return body
}

export function toCreateBody(v: InvoiceFormValues, payer: boolean): CreateInvoiceInput {
  return fullBody(v, { payer, withLineIds: false })
}

/** Mapování pole formuláře → klíče API těla (pro PATCH jen změněných polí). */
const fieldToKeys: Partial<Record<keyof InvoiceFormValues, (keyof Body)[]>> = {
  subject: ['subject_id'],
  payment_method: ['payment_method', 'custom_payment_method'],
  custom_payment_method: ['custom_payment_method'],
}

export function toPatchBody(
  v: InvoiceFormValues,
  dirty: Partial<Record<keyof InvoiceFormValues, unknown>>,
  payer: boolean,
): UpdateInvoiceInput {
  const all = fullBody(v, { payer, withLineIds: true }) as Record<string, unknown>
  const patch: Record<string, unknown> = {}
  for (const field of Object.keys(dirty) as (keyof InvoiceFormValues)[]) {
    if (!dirty[field] || field === 'document_type') continue
    for (const key of fieldToKeys[field] ?? [field as keyof Body]) {
      if (key in all) patch[key] = all[key]
      // Vymazané volitelné hodnoty posíláme jako prázdný řetězec.
      else if (key === 'variable_symbol' || key === 'taxable_fulfillment_due') patch[key] = ''
    }
  }
  return patch as UpdateInvoiceInput
}

/** API chyba `body.subject_id` apod. → název pole formuláře. */
export function apiFieldToFormField(path: string): string {
  if (path === 'subject_id') return 'subject'
  return path
}

/**
 * Cena z ceníku převedená na režim faktury (s DPH / bez DPH), zaokrouhleno half-away.
 * `convertPrice(12100, true, false, 2100)` → 10000.
 */
export function convertPrice(price: number, fromGross: boolean, toGross: boolean, rateBps: number): number {
  if (fromGross === toGross || rateBps === 0) return price
  const [num, den] = fromGross ? [10000, 10000 + rateBps] : [10000 + rateBps, 10000]
  return Number(divRoundHalfAway(BigInt(price) * BigInt(num), BigInt(den)))
}
