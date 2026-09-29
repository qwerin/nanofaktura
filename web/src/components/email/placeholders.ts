// E-maily k fakturám: popisky druhů, placeholdery a orientační náhled textu.
// Zdrojem pravdy je backend (internal/api/emails.go) — ten texty renderuje při odeslání
// a v GET /email-templates/preview. Tady jen živý náhled rozepsané (neuložené) šablony.

import type { EmailKind, EmailLang, Invoice } from '@/api/types'
import { renderPlaceholders } from '@/components/recurring/period'
import { daysBetween, parseISODate } from '@/lib/date'
import { MINOR_PER_MAJOR } from '@/lib/money'

export const emailKindLabels: Record<EmailKind, string> = {
  invoice: 'Faktura',
  reminder: 'Upomínka',
  paid_thanks: 'Poděkování za platbu',
}

export const emailKindDescriptions: Record<EmailKind, string> = {
  invoice: 'Odeslání dokladu klientovi (PDF v příloze).',
  reminder: 'Připomenutí neuhrazeného dokladu po splatnosti.',
  paid_thanks: 'Poděkování po úplném uhrazení.',
}

export const emailLangLabels: Record<EmailLang, string> = { cs: 'Čeština', en: 'Angličtina' }

export const EMAIL_KINDS: readonly EmailKind[] = ['invoice', 'reminder', 'paid_thanks']
export const EMAIL_LANGS: readonly EmailLang[] = ['cs', 'en']

export interface EmailPlaceholder {
  token: string
  label: string
}

/** Placeholdery e-mailových šablon (nejčastější první). */
export const EMAIL_PLACEHOLDERS: readonly EmailPlaceholder[] = [
  { token: '{number}', label: 'číslo dokladu' },
  { token: '{total}', label: 'celkem' },
  { token: '{remaining}', label: 'zbývá uhradit' },
  { token: '{due_on}', label: 'splatnost' },
  { token: '{public_url}', label: 'odkaz pro klienta' },
  { token: '{account_name}', label: 'vaše firma' },
  { token: '{client_name}', label: 'klient' },
  { token: '{vs}', label: 'variabilní symbol' },
  { token: '{iban}', label: 'IBAN' },
  { token: '{bank_account}', label: 'číslo účtu' },
  { token: '{payment_info}', label: 'platební údaje' },
  { token: '{issued_on}', label: 'datum vystavení' },
  { token: '{days_overdue}', label: 'dní po splatnosti' },
  { token: '{document}', label: 'typ dokladu' },
  { token: '{document_title}', label: 'Typ dokladu (velké písm.)' },
]

type DocType = Invoice['document_type']

const documentNames: Record<DocType, Record<EmailLang, [string, string]>> = {
  invoice: { cs: ['faktura', 'Faktura'], en: ['invoice', 'Invoice'] },
  proforma: { cs: ['zálohová faktura', 'Zálohová faktura'], en: ['proforma invoice', 'Proforma invoice'] },
  correction: { cs: ['opravný daňový doklad', 'Opravný daňový doklad'], en: ['credit note', 'Credit note'] },
}

const LOCALES: Record<EmailLang, string> = { cs: 'cs-CZ', en: 'en-GB' }

function money(minor: number, currency: string, lang: EmailLang): string {
  return new Intl.NumberFormat(LOCALES[lang], { style: 'currency', currency, minimumFractionDigits: 2 }).format(
    minor / MINOR_PER_MAJOR,
  )
}

function date(iso: string, lang: EmailLang): string {
  const d = parseISODate(iso)
  if (!d) return iso
  return new Intl.DateTimeFormat(LOCALES[lang], {
    day: 'numeric',
    month: lang === 'cs' ? 'numeric' : 'short',
    year: 'numeric',
    timeZone: 'UTC',
  }).format(d)
}

/** Podmnožina faktury potřebná pro náhled. */
export type EmailInvoice = Pick<
  Invoice,
  | 'document_type'
  | 'number'
  | 'total'
  | 'paid_amount'
  | 'currency'
  | 'issued_on'
  | 'due_on'
  | 'public_token'
  | 'client_name'
  | 'variable_symbol'
  | 'iban'
  | 'bank_account'
  | 'payment_method'
>

/** Hodnoty placeholderů pro fakturu — zrcadlo `emailVars` na backendu. */
export function emailVars(
  inv: EmailInvoice,
  opts: { accountName: string; lang: EmailLang; origin: string; today: string },
): Record<string, string> {
  const { lang } = opts
  const [doc, docTitle] = documentNames[inv.document_type][lang]
  const overdue = inv.due_on && inv.due_on < opts.today ? daysBetween(inv.due_on, opts.today) : 0
  const pay: string[] = []
  if (inv.payment_method === 'bank') {
    const labels =
      lang === 'cs' ? ['Číslo účtu', 'IBAN', 'Variabilní symbol'] : ['Bank account', 'IBAN', 'Payment reference']
    ;[inv.bank_account, inv.iban, inv.variable_symbol].forEach((v, i) => {
      if (v) pay.push(`${labels[i]}: ${v}`)
    })
  }
  return {
    number: inv.number,
    document: doc,
    document_title: docTitle,
    total: money(inv.total, inv.currency, lang),
    remaining: money(inv.total - inv.paid_amount, inv.currency, lang),
    issued_on: date(inv.issued_on, lang),
    due_on: date(inv.due_on, lang),
    days_overdue: String(overdue),
    public_url: `${opts.origin}/p/${inv.public_token}`,
    account_name: opts.accountName,
    client_name: inv.client_name,
    vs: inv.variable_symbol,
    iban: inv.iban,
    bank_account: inv.bank_account,
    payment_info: pay.join('\n'),
  }
}

/** Sloučí opakované prázdné řádky (po prázdných placeholderech) a ořízne okraje. */
export function cleanBlankLines(s: string): string {
  const out: string[] = []
  let blank = false
  for (const line of s.replace(/\r\n/g, '\n').split('\n')) {
    const b = line.trim() === ''
    if (b && blank) continue
    blank = b
    out.push(line.replace(/[ \t]+$/, ''))
  }
  return out.join('\n').trim()
}

/** Náhled e-mailu: šablona + podpis, placeholdery z `vars` (neznámé zůstanou). */
export function renderEmail(
  tpl: { subject: string; body: string },
  vars: Record<string, string>,
  signature = '',
): { subject: string; body: string } {
  let body = renderPlaceholders(tpl.body, vars)
  const sig = signature.trim()
  if (sig) body = `${body.replace(/[\n ]+$/, '')}\n\n${sig}`
  return { subject: renderPlaceholders(tpl.subject, vars).trim(), body: cleanBlankLines(body) }
}

/** Ukázková faktura pro náhled, když účet ještě žádnou nemá. */
export function sampleEmailInvoice(today: string, currency = 'CZK'): EmailInvoice {
  return {
    document_type: 'invoice',
    number: '2026-0001',
    total: 1_210_000,
    paid_amount: 0,
    currency,
    issued_on: today,
    due_on: today,
    public_token: 'ukazka',
    client_name: 'Vzorový klient s.r.o.',
    variable_symbol: '20260001',
    iban: 'CZ6508000000000123456789',
    bank_account: '123456789/0800',
    payment_method: 'bank',
  }
}

/** Druhy e-mailu, které má u faktury smysl ručně poslat. */
export function sendableKinds(inv: { status: Invoice['status']; document_type: DocType }): EmailKind[] {
  const out: EmailKind[] = ['invoice']
  if (inv.document_type === 'correction') return out
  // Upomínka až u odeslaného nebo neuhrazeného po splatnosti.
  if (inv.status === 'overdue' || inv.status === 'sent') out.push('reminder')
  if (inv.status === 'paid') out.push('paid_thanks')
  return out
}

/** Rozdělí zadané adresy (čárka, středník, mezera, nový řádek). */
export function splitAddresses(input: string): string[] {
  return input
    .split(/[\s,;]+/)
    .map((s) => s.trim())
    .filter(Boolean)
}

/** Jednoduchá kontrola tvaru adresy (přesnou validaci dělá backend). */
export function isEmailLike(s: string): boolean {
  return /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(s)
}
