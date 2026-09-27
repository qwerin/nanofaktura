// Čisté pomocníky pro filtry seznamu faktur.

import { z } from 'zod'
import type { InvoiceFilters } from '@/api/queries/invoices'
import type { InvoiceStatus } from '@/api/types'
import { parseISODate, todayISO, toISODate } from '@/lib/date'

const isoDate = z.string().regex(/^\d{4}-\d{2}-\d{2}$/)

/** Search params stránky /invoices (neplatné hodnoty se tiše zahodí). */
export const invoiceSearchSchema = z.object({
  status: z.enum(['open', 'sent', 'paid', 'overdue', 'cancelled', 'uncollectible']).optional().catch(undefined),
  document_type: z.enum(['invoice', 'proforma', 'correction']).optional().catch(undefined),
  query: z.string().optional().catch(undefined),
  since: isoDate.optional().catch(undefined),
  until: isoDate.optional().catch(undefined),
  sort: z.enum(['-issued_on', 'issued_on', '-number', 'due_on', '-total']).optional().catch(undefined),
})
export type InvoiceSearch = z.infer<typeof invoiceSearchSchema>
export type InvoiceSort = NonNullable<InvoiceSearch['sort']>

export const DEFAULT_SORT: InvoiceSort = '-issued_on'

export const sortLabels: Record<InvoiceSort, string> = {
  '-issued_on': 'Nejnovější',
  issued_on: 'Nejstarší',
  '-number': 'Podle čísla',
  due_on: 'Podle splatnosti',
  '-total': 'Nejvyšší částka',
}

/** Záložky stavu v pořadí zobrazení. */
export const statusTabs: { value: InvoiceStatus | undefined; label: string }[] = [
  { value: undefined, label: 'Vše' },
  { value: 'overdue', label: 'Po splatnosti' },
  { value: 'open', label: 'Otevřené' },
  { value: 'sent', label: 'Odeslané' },
  { value: 'paid', label: 'Uhrazené' },
  { value: 'cancelled', label: 'Stornované' },
  { value: 'uncollectible', label: 'Nedobytné' },
]

/** Search params → filtry API (prázdné hodnoty pryč, aby se nelišil query klíč). */
export function toApiFilters(s: InvoiceSearch): InvoiceFilters {
  const f: InvoiceFilters = {}
  if (s.status) f.status = s.status
  if (s.document_type) f.document_type = s.document_type
  if (s.query?.trim()) f.query = s.query.trim()
  if (s.since) f.since = s.since
  if (s.until) f.until = s.until
  if (s.sort && s.sort !== DEFAULT_SORT) f.sort = s.sort
  return f
}

/** Počet aktivních filtrů mimo stav a hledání (odznak na tlačítku „Filtry“). */
export function extraFilterCount(s: InvoiceSearch): number {
  return [s.document_type, s.since || s.until, s.sort && s.sort !== DEFAULT_SORT].filter(Boolean).length
}

export interface DatePreset {
  id: string
  label: string
  since: string
  until: string
}

function monthRange(y: number, m: number): [string, string] {
  const start = new Date(Date.UTC(y, m, 1))
  const end = new Date(Date.UTC(y, m + 1, 0))
  return [toISODate(start), toISODate(end)]
}

/** Rychlé volby období podle data vystavení. */
export function datePresets(today: string = todayISO()): DatePreset[] {
  const d = parseISODate(today) ?? new Date()
  const y = d.getUTCFullYear()
  const m = d.getUTCMonth()
  const [tmS, tmU] = monthRange(y, m)
  const [lmS, lmU] = monthRange(m === 0 ? y - 1 : y, m === 0 ? 11 : m - 1)
  const q = Math.floor(m / 3)
  const [qS] = monthRange(y, q * 3)
  const [, qU] = monthRange(y, q * 3 + 2)
  return [
    { id: 'this-month', label: 'Tento měsíc', since: tmS, until: tmU },
    { id: 'last-month', label: 'Minulý měsíc', since: lmS, until: lmU },
    { id: 'this-quarter', label: 'Toto čtvrtletí', since: qS, until: qU },
    { id: 'this-year', label: 'Letos', since: `${y}-01-01`, until: `${y}-12-31` },
    { id: 'last-year', label: 'Loni', since: `${y - 1}-01-01`, until: `${y - 1}-12-31` },
  ]
}
