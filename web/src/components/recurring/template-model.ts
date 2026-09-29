// Model formuláře šablony faktury: Zod schéma, šablona ↔ hodnoty formuláře ↔ API tělo.
// Řádky mají stejný tvar jako ve formuláři faktury (sdílený LinesEditor).

import { z } from 'zod'
import type { Account, CreateTemplateInput, InvoiceTemplate, PaymentMethod } from '@/api/types'
import { calculateTotals, chargesNoVat } from '@/components/invoice/calc'
import { emptyLine, lineSchema, subjectValueSchema } from '@/components/invoice/form-model'
import { formatMoneyInput, parseMoney } from '@/lib/money'
import { parseQuantity } from '@/lib/quantity'

export const templateFormSchema = z.object({
  name: z.string().trim().min(1, 'Zadejte název šablony').max(200, 'Nejvýše 200 znaků'),
  document_type: z.enum(['invoice', 'proforma']),
  subject: subjectValueSchema.nullable().refine((v) => v !== null, 'Vyberte odběratele'),
  due_days: z
    .string()
    .trim()
    .regex(/^(\d{1,3})?$/, 'Počet dní 0–999'),
  currency: z.string().min(3),
  exchange_rate: z
    .string()
    .trim()
    .regex(/^\d+([.,]\d+)?$/, 'Kurz, např. 24,355'),
  language: z.enum(['cs', 'en']),
  payment_method: z.enum(['bank', 'cash', 'card', 'cod', 'paypal', 'custom']),
  custom_payment_method: z.string(),
  bank_account_id: z.string(),
  order_number: z.string(),
  note: z.string(),
  footer_note: z.string(),
  private_note: z.string(),
  tags: z.array(z.string()),
  prices_include_vat: z.boolean(),
  round_total: z.boolean(),
  reverse_charge: z.boolean(),
  lines: z.array(lineSchema).min(1, 'Přidejte aspoň jednu položku'),
})

export type TemplateFormValues = z.input<typeof templateFormSchema>
export type TemplateSubjectValue = TemplateFormValues['subject']

/** Nová šablona s výchozími hodnotami účtu. */
export function newTemplateValues(account: Account): TemplateFormValues {
  const payer = account.vat_mode !== 'non_vat_payer'
  return {
    name: '',
    document_type: 'invoice',
    subject: null,
    due_days: '',
    currency: account.default_currency || 'CZK',
    exchange_rate: '1',
    language: account.default_language,
    payment_method: account.default_payment_method || 'bank',
    custom_payment_method: '',
    bank_account_id: '',
    order_number: '',
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

/** Hodnoty formuláře z uložené šablony; prázdná pole šablony = aktuální výchozí hodnoty účtu. */
export function templateToValues(t: InvoiceTemplate, account: Account, subject: TemplateSubjectValue): TemplateFormValues {
  const payer = account.vat_mode !== 'non_vat_payer'
  return {
    name: t.name,
    document_type: t.document_type,
    subject,
    due_days: t.due_days != null ? String(t.due_days) : '',
    currency: t.currency || account.default_currency || 'CZK',
    exchange_rate: t.exchange_rate || '1',
    language: t.language === 'en' || t.language === 'cs' ? t.language : account.default_language,
    payment_method: (t.payment_method || account.default_payment_method || 'bank') as PaymentMethod,
    custom_payment_method: t.custom_payment_method,
    bank_account_id: t.bank_account_id ? String(t.bank_account_id) : '',
    order_number: t.order_number,
    note: t.note ?? account.default_note,
    footer_note: t.footer_note ?? account.default_footer_note,
    private_note: t.private_note,
    tags: t.tags ?? [],
    prices_include_vat: t.prices_include_vat,
    round_total: t.round_total ?? account.round_total,
    reverse_charge: t.reverse_charge,
    lines: t.lines.map((l) => ({
      price_item_id: l.price_item_id,
      name: l.name,
      quantity: l.quantity.replace('.', ','),
      unit_name: l.unit_name,
      unit_price: formatMoneyInput(l.unit_price),
      vat_rate_bps: String(l.vat_rate_bps ?? (payer ? account.default_vat_rate_bps : 0)),
    })),
  }
}

/** Úplné tělo šablony (create i PATCH — řádky se vždy nahrazují celé). */
export function toTemplateBody(v: TemplateFormValues, payer: boolean): CreateTemplateInput {
  const body: CreateTemplateInput = {
    name: v.name.trim(),
    document_type: v.document_type,
    subject_id: v.subject?.id ?? 0,
    currency: v.currency,
    exchange_rate: v.exchange_rate.trim().replace(',', '.'),
    language: v.language,
    payment_method: v.payment_method,
    custom_payment_method: v.payment_method === 'custom' ? v.custom_payment_method.trim() : '',
    order_number: v.order_number.trim(),
    note: v.note,
    footer_note: v.footer_note,
    private_note: v.private_note,
    tags: v.tags,
    prices_include_vat: v.prices_include_vat,
    round_total: v.round_total,
    reverse_charge: payer ? v.reverse_charge : false,
    lines: v.lines.map((l) => ({
      ...(l.price_item_id ? { price_item_id: l.price_item_id } : {}),
      name: l.name.trim(),
      quantity: parseQuantity(l.quantity) ?? '1',
      unit_name: l.unit_name.trim(),
      unit_price: parseMoney(l.unit_price) ?? 0,
      vat_rate_bps: payer ? Number(l.vat_rate_bps) || 0 : 0,
    })),
  }
  if (v.due_days.trim() !== '') body.due_days = Number(v.due_days)
  body.bank_account_id = v.bank_account_id ? Number(v.bank_account_id) : undefined
  return body
}

/** Částka šablony (součet řádků, orientačně — přesně spočítá backend při vystavení). */
export function templateTotal(
  t: Pick<InvoiceTemplate, 'lines' | 'prices_include_vat' | 'reverse_charge' | 'round_total'>,
  account: Pick<Account, 'vat_mode' | 'default_vat_rate_bps' | 'round_total'>,
): number {
  const payer = account.vat_mode !== 'non_vat_payer'
  return calculateTotals(
    t.lines.map((l) => ({
      quantity: l.quantity,
      unitPrice: l.unit_price,
      vatRateBps: payer ? (l.vat_rate_bps ?? account.default_vat_rate_bps) : 0,
    })),
    {
      pricesIncludeVat: t.prices_include_vat,
      reverseCharge: payer && t.reverse_charge,
      roundTotal: t.round_total ?? account.round_total,
      nonVatPayer: chargesNoVat(account.vat_mode, payer && t.reverse_charge),
    },
  ).total
}
