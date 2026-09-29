// Texty a formátování veřejné stránky faktury v jazyce dokladu (cs, en; sk → cs, de a ostatní → en).

import type { PublicInvoice } from '@/api/types'

export type PublicLang = 'cs' | 'en'

export function publicLang(language: string | undefined): PublicLang {
  const l = (language ?? '').toLowerCase()
  return l === 'cs' || l === 'sk' || l === '' ? 'cs' : 'en'
}

const cs = {
  locale: 'cs-CZ',
  invoice: 'Faktura',
  taxInvoice: 'Faktura – daňový doklad',
  proforma: 'Zálohová faktura',
  correction: 'Opravný daňový doklad',
  correctionNonPayer: 'Opravná faktura',
  number: 'č.',
  toPay: 'K úhradě',
  total: 'Celkem',
  due: 'Splatnost',
  dueOn: 'splatná',
  paid: 'Uhrazeno',
  paidOn: 'Uhrazeno dne',
  paidThanks: 'Děkujeme, faktura je uhrazena.',
  overdue: 'Po splatnosti',
  overdueDays: (n: number) => `${n} ${n === 1 ? 'den' : n < 5 ? 'dny' : 'dní'} po splatnosti`,
  dueIn: (n: number) => (n === 0 ? 'Splatná dnes' : `Splatná za ${n} ${n === 1 ? 'den' : n < 5 ? 'dny' : 'dní'}`),
  cancelled: 'Doklad byl stornován',
  cancelledHint: 'Tento doklad je neplatný, nic neplaťte.',
  uncollectible: 'Pohledávka byla odepsána',
  partiallyPaid: 'Částečně uhrazeno',
  refund: 'K vrácení',
  payment: 'Platební údaje',
  qrHint: 'Naskenujte v mobilní aplikaci své banky',
  account: 'Číslo účtu',
  iban: 'IBAN',
  swift: 'SWIFT/BIC',
  vs: 'Variabilní symbol',
  amount: 'Částka',
  copy: 'Kopírovat',
  copied: 'Zkopírováno',
  method: 'Způsob úhrady',
  methods: { bank: 'Bankovní převod', cash: 'Hotově', card: 'Kartou', cod: 'Dobírka', paypal: 'PayPal', custom: '' },
  downloadPdf: 'Stáhnout PDF',
  downloadIsdoc: 'Stáhnout ISDOC',
  isdocHint: 'Pro import do účetnictví',
  supplier: 'Dodavatel',
  customer: 'Odběratel',
  regNo: 'IČO',
  vatNo: 'DIČ',
  nonPayer: 'Neplátce DPH',
  identified: 'Identifikovaná osoba k DPH',
  issued: 'Vystaveno',
  taxPoint: 'Datum zdanitelného plnění',
  order: 'Objednávka',
  related: (n: string) => `K dokladu ${n}`,
  items: 'Položky',
  itemsCount: (n: number) => `${n} ${n === 1 ? 'položka' : n < 5 ? 'položky' : 'položek'}`,
  showItems: 'Zobrazit položky',
  vatRecap: 'Rekapitulace DPH',
  rate: 'Sazba',
  base: 'Základ',
  vat: 'DPH',
  subtotal: 'Bez DPH',
  rounding: 'Zaokrouhlení',
  alreadyPaid: 'Již uhrazeno',
  remaining: 'Zbývá uhradit',
  reverseCharge: 'Daň odvede zákazník (přenesení daňové povinnosti).',
  poweredBy: 'Vystaveno v aplikaci',
  notFoundTitle: 'Odkaz na fakturu neplatí',
  notFoundText: 'Doklad neexistuje, nebo dodavatel odkaz zneplatnil. Požádejte ho o nový.',
  errorTitle: 'Fakturu se nepodařilo načíst',
  retry: 'Zkusit znovu',
  web: 'Web',
  email: 'E-mail',
  phone: 'Telefon',
  exchangeRate: 'Kurz',
}

type Labels = typeof cs

const en: Labels = {
  locale: 'en-GB',
  invoice: 'Invoice',
  taxInvoice: 'Tax invoice',
  proforma: 'Proforma invoice',
  correction: 'Credit note',
  correctionNonPayer: 'Credit note',
  number: 'No.',
  toPay: 'Amount due',
  total: 'Total',
  due: 'Due date',
  dueOn: 'due',
  paid: 'Paid',
  paidOn: 'Paid on',
  paidThanks: 'Thank you, this invoice has been paid.',
  overdue: 'Overdue',
  overdueDays: (n: number) => `${n} ${n === 1 ? 'day' : 'days'} overdue`,
  dueIn: (n: number) => (n === 0 ? 'Due today' : `Due in ${n} ${n === 1 ? 'day' : 'days'}`),
  cancelled: 'This document was cancelled',
  cancelledHint: 'The document is void, no payment is required.',
  uncollectible: 'Written off',
  partiallyPaid: 'Partially paid',
  refund: 'To be refunded',
  payment: 'Payment details',
  qrHint: 'Scan with your banking app (Czech QR Platba)',
  account: 'Account number',
  iban: 'IBAN',
  swift: 'SWIFT/BIC',
  vs: 'Variable symbol',
  amount: 'Amount',
  copy: 'Copy',
  copied: 'Copied',
  method: 'Payment method',
  methods: { bank: 'Bank transfer', cash: 'Cash', card: 'Card', cod: 'Cash on delivery', paypal: 'PayPal', custom: '' },
  downloadPdf: 'Download PDF',
  downloadIsdoc: 'Download ISDOC',
  isdocHint: 'For accounting software',
  supplier: 'Supplier',
  customer: 'Customer',
  regNo: 'Reg. No.',
  vatNo: 'VAT No.',
  nonPayer: 'Not a VAT payer',
  identified: 'Identified person for VAT',
  issued: 'Issued',
  taxPoint: 'Tax point date',
  order: 'Order',
  related: (n: string) => `Relates to ${n}`,
  items: 'Items',
  itemsCount: (n: number) => `${n} ${n === 1 ? 'item' : 'items'}`,
  showItems: 'Show items',
  vatRecap: 'VAT summary',
  rate: 'Rate',
  base: 'Base',
  vat: 'VAT',
  subtotal: 'Without VAT',
  rounding: 'Rounding',
  alreadyPaid: 'Already paid',
  remaining: 'Remaining',
  reverseCharge: 'Reverse charge — VAT to be accounted for by the customer.',
  poweredBy: 'Issued with',
  notFoundTitle: 'This invoice link is not valid',
  notFoundText: 'The document does not exist or the supplier has revoked the link. Please ask them for a new one.',
  errorTitle: 'The invoice could not be loaded',
  retry: 'Try again',
  web: 'Web',
  email: 'E-mail',
  phone: 'Phone',
  exchangeRate: 'Exchange rate',
}

export function publicLabels(lang: PublicLang): Labels {
  return lang === 'cs' ? cs : en
}

export type PublicLabels = Labels

/** Titulek dokladu podle typu a režimu DPH dodavatele. */
export function documentTitle(inv: Pick<PublicInvoice, 'document_type' | 'supplier'>, t: Labels): string {
  const payer = inv.supplier.vat_mode === 'vat_payer'
  if (inv.document_type === 'proforma') return t.proforma
  if (inv.document_type === 'correction') return payer ? t.correction : t.correctionNonPayer
  return payer ? t.taxInvoice : t.invoice
}

export type PaymentState = 'cancelled' | 'paid' | 'uncollectible' | 'overdue' | 'open' | 'refund'

/** Stav z pohledu zákazníka. `days`: dní do splatnosti (záporné = po splatnosti). */
export function paymentState(
  inv: Pick<PublicInvoice, 'status' | 'cancelled' | 'remaining_amount' | 'due_on' | 'total'>,
  today: string,
): { state: PaymentState; days: number } {
  const days = daysBetweenISO(today, inv.due_on)
  if (inv.cancelled || inv.status === 'cancelled') return { state: 'cancelled', days }
  if (inv.status === 'uncollectible') return { state: 'uncollectible', days }
  if (inv.status === 'paid' || inv.remaining_amount === 0) return { state: 'paid', days }
  if (inv.remaining_amount < 0) return { state: 'refund', days }
  if (inv.status === 'overdue' || days < 0) return { state: 'overdue', days }
  return { state: 'open', days }
}

function daysBetweenISO(from: string, to: string): number {
  const a = Date.parse(`${from}T00:00:00Z`)
  const b = Date.parse(`${to}T00:00:00Z`)
  if (Number.isNaN(a) || Number.isNaN(b)) return 0
  return Math.round((b - a) / 86_400_000)
}

/** Peníze v jazyce dokladu (haléře → text). */
export function formatAmount(amount: number, currency: string, t: Labels): string {
  return new Intl.NumberFormat(t.locale, { style: 'currency', currency, minimumFractionDigits: 2, maximumFractionDigits: 2 }).format(
    amount / 100,
  )
}

/** Částka pro zkopírování do bankovní aplikace: „1234,50“ (cs) / „1234.50“ (en), bez měny a mezer. */
export function amountForCopy(amount: number, t: Labels): string {
  const abs = Math.abs(amount)
  const s = `${Math.trunc(abs / 100)}${t.locale === 'cs-CZ' ? ',' : '.'}${String(abs % 100).padStart(2, '0')}`
  return amount < 0 ? `-${s}` : s
}

export function formatPublicDate(iso: string, t: Labels): string {
  const ms = Date.parse(`${iso}T00:00:00Z`)
  if (!iso || Number.isNaN(ms)) return ''
  return new Intl.DateTimeFormat(t.locale, {
    day: 'numeric',
    month: t.locale === 'cs-CZ' ? 'numeric' : 'short',
    year: 'numeric',
    timeZone: 'UTC',
  }).format(new Date(ms))
}

/** Číslo účtu / IBAN čitelně po čtveřicích (IBAN), české číslo beze změny. */
export function formatIban(iban: string): string {
  return iban.replace(/\s+/g, '').replace(/(.{4})(?=.)/g, '$1 ')
}
