// Čisté pomocné funkce sekce Banka (formátování, filtry v URL, texty chyb). Testy: format.test.ts.

import { z } from 'zod'
import { isApiError, errorMessage } from '@/api/errors'
import type { BankTransaction, BankTransactionFilters, BankTransactionState } from '@/api/types'
import { formatMoney, parseMoney } from '@/lib/money'

// ---------------------------------------------------------------------------
// Částky

export type Direction = 'in' | 'out'

/** Směr pohybu podle znaménka (+ příchozí, − odchozí). Nulová částka se bere jako příchozí. */
export function directionOf(amount: number): Direction {
  return amount < 0 ? 'out' : 'in'
}

/**
 * Částka se znaménkem pro výpis: `+12 100,00 Kč` / `−46,60 Kč` (skutečné minus U+2212,
 * aby se nelámalo a bylo čitelné). Nula bez znaménka.
 */
export function formatSignedAmount(amount: number, currency: string): string {
  const abs = formatMoney(Math.abs(amount), currency)
  if (amount > 0) return `+${abs}`
  if (amount < 0) return `−${abs}`
  return abs
}

/** Třída barvy částky (tokeny: příchozí zeleně, odchozí výchozí barvou textu). */
export function amountToneClass(amount: number): string {
  if (amount > 0) return 'text-success'
  if (amount < 0) return 'text-foreground'
  return 'text-muted-foreground'
}

// ---------------------------------------------------------------------------
// Důvody návrhů

export type ReasonTone = 'strong' | 'partial' | 'weak'

/**
 * Backend posílá důvody česky (internal/matching). Pro čipy je zkracujeme
 * a přiřazujeme váhu (barvu). Neznámý důvod se zobrazí tak, jak přišel.
 */
const reasonMap: Record<string, { label: string; tone: ReasonTone }> = {
  'VS sedí': { label: 'VS sedí', tone: 'strong' },
  'Částka sedí': { label: 'Částka sedí', tone: 'strong' },
  'Částka odpovídá celkové částce dokladu': { label: 'Celková částka sedí', tone: 'weak' },
  'Částečná úhrada': { label: 'Částečná úhrada', tone: 'partial' },
  'Jméno protistrany sedí': { label: 'Jméno sedí', tone: 'weak' },
  'Účet protistrany sedí': { label: 'Účet sedí', tone: 'weak' },
}

export function suggestionReason(reason: string): { label: string; tone: ReasonTone } {
  return reasonMap[reason] ?? { label: reason, tone: 'weak' }
}

/** Důvody seřazené od nejsilnějších (VS, částka) po slabé. */
export function sortReasons(reasons: readonly string[]): { label: string; tone: ReasonTone }[] {
  const order: Record<ReasonTone, number> = { strong: 0, partial: 1, weak: 2 }
  return reasons.map(suggestionReason).sort((a, b) => order[a.tone] - order[b.tone])
}

/** Síla návrhu podle skóre z backendu (VS 50 + částka 40 = 90 → jistá shoda). */
export function suggestionConfidence(score: number): 'high' | 'medium' | 'low' {
  if (score >= 80) return 'high'
  if (score >= 45) return 'medium'
  return 'low'
}

// ---------------------------------------------------------------------------
// Stavy a záložky

export const stateLabels: Record<BankTransactionState, string> = {
  unmatched: 'Nespárováno',
  suggested: 'Návrh',
  matched: 'Spárováno',
  ignored: 'Ignorováno',
}

export const TABS = ['unmatched', 'suggested', 'matched', 'ignored', 'all'] as const
export type BankTab = (typeof TABS)[number]

export const tabLabels: Record<BankTab, string> = {
  unmatched: 'Nespárované',
  suggested: 'Návrhy',
  matched: 'Spárované',
  ignored: 'Ignorované',
  all: 'Vše',
}

// ---------------------------------------------------------------------------
// Filtry v URL

const isoDate = z.string().regex(/^\d{4}-\d{2}-\d{2}$/)

export const bankSearchSchema = z.object({
  tab: z.enum(TABS).optional().catch(undefined),
  account: z.coerce.number().int().positive().optional().catch(undefined),
  direction: z.enum(['in', 'out']).optional().catch(undefined),
  since: isoDate.optional().catch(undefined),
  until: isoDate.optional().catch(undefined),
  query: z.string().trim().min(1).max(200).optional().catch(undefined),
})
export type BankSearch = z.infer<typeof bankSearchSchema>

/** Záložka z URL; výchozí jsou nespárované. */
export function tabOf(search: Pick<BankSearch, 'tab'>): BankTab {
  return search.tab ?? 'unmatched'
}

/** Filtry mimo stav (sdílené všemi záložkami a počty). */
export function baseFilters(search: BankSearch): BankTransactionFilters {
  const f: BankTransactionFilters = {}
  if (search.account) f.bank_account_id = search.account
  if (search.direction) f.direction = search.direction
  if (search.since) f.since = search.since
  if (search.until) f.until = search.until
  if (search.query) f.query = search.query
  return f
}

/** Search params stránky → query parametry API. „Nespárované“ zahrnují i návrhy (API). */
export function searchToFilters(search: BankSearch): BankTransactionFilters {
  const tab = tabOf(search)
  const f = baseFilters(search)
  if (tab !== 'all') f.state = tab
  return f
}

/** Počet aktivních filtrů v dialogu „Filtry“ (bez záložky a hledání). */
export function countBankFilters(search: BankSearch): number {
  let n = 0
  if (search.account) n++
  if (search.direction) n++
  if (search.since || search.until) n++
  return n
}

// ---------------------------------------------------------------------------
// Protistrana a symboly

/** Hlavní popis pohybu: jméno protistrany → zpráva → účet → „Bez popisu“. */
export function transactionTitle(t: Pick<BankTransaction, 'counterparty_name' | 'message' | 'counterparty_account'>): string {
  return t.counterparty_name || t.message || t.counterparty_account || 'Bez popisu'
}

/** „VS 2026001 · KS 0308“ — jen vyplněné symboly. */
export function symbolsText(t: Pick<BankTransaction, 'variable_symbol' | 'constant_symbol' | 'specific_symbol'>): string {
  return [
    t.variable_symbol && `VS ${t.variable_symbol}`,
    t.constant_symbol && `KS ${t.constant_symbol}`,
    t.specific_symbol && `SS ${t.specific_symbol}`,
  ]
    .filter(Boolean)
    .join(' · ')
}

// ---------------------------------------------------------------------------
// Ruční hledání dokladu

/**
 * Hledaný text, který vypadá jako částka („12 100“, „12100,00 Kč“), → haléře; jinak `null`
 * (pak se hledá číslo dokladu / jméno na serveru). Čistě číselný text bez oddělovačů
 * delší než 6 číslic bereme jako číslo dokladu / VS, ne jako částku.
 */
export function amountQuery(query: string): number | null {
  const q = query.trim()
  if (!/^[\d\s\u00a0.,]+(\s*(kč|czk|eur|€))?$/i.test(q)) return null
  if (/^\d{7,}$/.test(q)) return null
  const amount = parseMoney(q)
  return amount !== null && amount > 0 ? amount : null
}

export interface MatchCandidate {
  id: number
  total: number
  remaining: number
  currency: string
  /** Doklad lze spárovat (ne storno / nedobytný). */
  payable: boolean
}

/**
 * Seřadí kandidáty pro ruční párování: nejdřív zbývající částka = platba, pak celková částka = platba,
 * pak neuhrazené, nakonec uhrazené. Doklady v jiné měně a nesplatitelné vyřadí.
 */
export function rankCandidates<T extends MatchCandidate>(items: readonly T[], amount: number, currency: string): T[] {
  const abs = Math.abs(amount)
  const rank = (c: T) => (c.remaining === abs ? 0 : c.total === abs ? 1 : c.remaining > 0 ? 2 : 3)
  return items
    .filter((c) => c.payable && c.currency === currency)
    .map((c, i) => ({ c, i, r: rank(c) }))
    .sort((a, b) => a.r - b.r || a.i - b.i)
    .map((x) => x.c)
}

// ---------------------------------------------------------------------------
// Registr plátců DPH

/** České DIČ, které registr umí ověřit: volitelně `CZ` + 8–10 číslic. */
export function isCzechVatNo(vatNo: string | null | undefined): boolean {
  return /^(CZ)?\d{8,10}$/i.test((vatNo ?? '').replace(/\s+/g, ''))
}

/** „000019-0002000145399/0800“ → „19-2000145399/0800“ (bez mezer a úvodních nul). */
function normalizeCzechAccount(acc: string): string {
  const [num = '', bank = ''] = acc.replace(/\s+/g, '').split('/')
  const [prefix, number] = num.includes('-') ? num.split('-') : ['', num]
  const p = (prefix ?? '').replace(/^0+/, '')
  const n = (number ?? '').replace(/^0+/, '')
  return `${p ? `${p}-` : ''}${n}/${bank}`
}

/**
 * Je účet kontaktu mezi zveřejněnými účty? `null` = kontakt účet nemá (nelze říct).
 * Porovnává IBAN i české číslo účtu (bez úvodních nul).
 */
export function isAccountPublished(
  subject: { bank_account?: string; iban?: string },
  published: readonly { number: string; iban: string }[],
): boolean | null {
  const iban = (subject.iban ?? '').replace(/\s+/g, '').toUpperCase()
  const number = (subject.bank_account ?? '').trim()
  if (!iban && !number) return null
  return published.some(
    (p) =>
      (iban !== '' && p.iban.replace(/\s+/g, '').toUpperCase() === iban) ||
      (number !== '' && number.includes('/') && normalizeCzechAccount(p.number) === normalizeCzechAccount(number)),
  )
}

// ---------------------------------------------------------------------------
// Formáty výpisů

export const STATEMENT_FORMATS = [
  { value: 'fio_json', label: 'Fio — JSON (API)' },
  { value: 'fio_csv', label: 'Fio — CSV' },
  { value: 'gpc', label: 'ABO / GPC (ČSOB, Fio, KB…)' },
  { value: 'csob_csv', label: 'ČSOB — CSV' },
  { value: 'kb_csv', label: 'Komerční banka — CSV' },
  { value: 'airbank_csv', label: 'Air Bank — CSV' },
] as const

export function statementFormatLabel(format: string): string {
  return STATEMENT_FORMATS.find((f) => f.value === format)?.label ?? format
}

/** Přípony souborů, které dává smysl nabízet v dialogu. */
export const STATEMENT_ACCEPT = '.gpc,.abo,.csv,.json,.txt,text/csv,application/json,text/plain'

export const MAX_STATEMENT_BYTES = 10 * 1024 * 1024

/**
 * Čas poslední synchronizace lidsky: „před chvílí“, „před 5 min“, „před 3 h“, jinak datum a čas.
 * `formatAbsolute` dostává ISO řetězec (kvůli testům bez závislosti na časové zóně).
 */
export function formatSyncedAgo(iso: string, now: number, formatAbsolute: (iso: string) => string): string {
  const at = Date.parse(iso)
  if (Number.isNaN(at)) return ''
  const minutes = Math.floor((now - at) / 60_000)
  if (minutes < 1) return 'před chvílí'
  if (minutes < 60) return `před ${minutes} min`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `před ${hours} h`
  return formatAbsolute(iso)
}

/** Fio API token: 16–128 písmen a číslic (stejná kontrola jako backend). */
export function isValidFioToken(token: string): boolean {
  return /^[A-Za-z0-9]{16,128}$/.test(token.trim())
}

// ---------------------------------------------------------------------------
// Chyby API → česky

/** Sekundy z hlavičky Retry-After (čísla i HTTP datum); výchozí 30 s (limit Fio). */
export function parseRetryAfter(header: string | null | undefined, now: number = Date.now()): number {
  if (!header) return 30
  const n = Number(header.trim())
  if (Number.isFinite(n) && n >= 0) return Math.ceil(n)
  const at = Date.parse(header)
  if (!Number.isNaN(at)) return Math.max(0, Math.ceil((at - now) / 1000))
  return 30
}

export type SyncError =
  | { kind: 'rate_limited'; retryAfter: number }
  | { kind: 'not_configured' }
  | { kind: 'bad_token' }
  | { kind: 'too_many' }
  | { kind: 'other'; message: string }

/** Rozliší chybu `POST /bank-accounts/{id}/sync` (429 / 409 / 422 / 502). */
export function classifySyncError(err: unknown, retryAfter?: number): SyncError {
  if (isApiError(err)) {
    const detail = (err.problem.detail ?? '').toLowerCase()
    if (err.status === 429) return { kind: 'rate_limited', retryAfter: retryAfter ?? 30 }
    if (err.status === 409) return { kind: 'not_configured' }
    if (err.status === 422) {
      if (detail.includes('too many')) return { kind: 'too_many' }
      return { kind: 'bad_token' }
    }
    if (err.status === 502) return { kind: 'other', message: 'Fio API je teď nedostupné, zkuste to později.' }
  }
  return { kind: 'other', message: errorMessage(err) }
}

export function syncErrorText(e: SyncError): string {
  switch (e.kind) {
    case 'rate_limited':
      return `Fio povoluje jeden dotaz za 30 s — zkuste za ${e.retryAfter} s.`
    case 'not_configured':
      return 'Automatická synchronizace není u účtu nastavená.'
    case 'bad_token':
      return 'Fio odmítlo API token (neplatný, expirovaný nebo bez oprávnění). Zadejte nový token.'
    case 'too_many':
      return 'V období je příliš mnoho pohybů. V nastavení účtu zvolte pozdější datum „Synchronizovat od“.'
    case 'other':
      return e.message
  }
}

function problemTexts(err: unknown): string {
  if (!isApiError(err)) return ''
  const p = err.problem
  return [p.detail, ...(p.errors ?? []).map((e) => e.message)].filter(Boolean).join(' ').toLowerCase()
}

/** Chyba importu výpisu česky. */
export function importErrorText(err: unknown): string {
  if (isApiError(err)) {
    const t = problemTexts(err)
    if (err.status === 413 || t.includes('larger than')) return 'Soubor je větší než 10 MB.'
    if (t.includes('another bank account')) {
      const acc = /\(([^)]+)\)/.exec(err.problem.errors?.[0]?.message ?? '')?.[1]
      return `Výpis patří k jinému bankovnímu účtu${acc ? ` (${acc})` : ''}. Vyberte správný účet.`
    }
    if (t.includes('unknown format')) return 'Neznámý formát výpisu.'
    if (t.includes('file is empty')) return 'Soubor je prázdný.'
    if (t.includes('cannot read') || err.status === 422)
      return 'Soubor se nepodařilo přečíst jako bankovní výpis. Zkuste vybrat formát ručně.'
  }
  return errorMessage(err)
}

/** Chyba párování / odpárování / ignorování česky. */
export function matchErrorText(err: unknown): string {
  if (isApiError(err)) {
    const t = problemTexts(err)
    if (t.includes('already matched')) return 'Platba už je spárovaná — obnovte stránku.'
    if (t.includes('is not matched')) return 'Platba není spárovaná.'
    if (t.includes('is matched')) return 'Spárovanou platbu nelze ignorovat — nejdřív zrušte spárování.'
    if (t.includes('zero-amount')) return 'Nulovou platbu nelze spárovat.'
    if (t.includes('cannot add a payment')) return 'Ke stornované nebo nedobytné faktuře nelze přidat platbu.'
    if (t.includes('the invoice is in') || t.includes('the expense is in')) return 'Doklad je v jiné měně než platba.'
    if (t.includes('not found')) return 'Doklad nebyl nalezen.'
  }
  return errorMessage(err)
}
