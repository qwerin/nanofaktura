// Pravidelné faktury: popisy period, rozvrh výskytů a datové placeholdery šablon.
// Zobrazovací duplikát internal/billing/period.go — zdrojem pravdy je backend
// (`next_occurrence_on` počítá server), tady jde o náhledy ve formulářích.

import { parseISODate, toISODate } from '@/lib/date'
import { plural } from '@/components/invoice/status'

/** Předvolby periody (měsíce). */
export const PERIOD_PRESETS = [1, 3, 6, 12] as const

/** `1` → „měsíčně“, `3` → „čtvrtletně“, `5` → „každých 5 měsíců“. */
export function periodLabel(months: number): string {
  switch (months) {
    case 1:
      return 'měsíčně'
    case 3:
      return 'čtvrtletně'
    case 6:
      return 'pololetně'
    case 12:
      return 'ročně'
  }
  if (months > 12 && months % 12 === 0) {
    const y = months / 12
    return `${plural(y, ['každý', 'každé', 'každých'])} ${y} ${plural(y, ['rok', 'roky', 'let'])}`
  }
  // „každé 2 měsíce“, „každých 5 měsíců“
  return `${plural(months, ['každý', 'každé', 'každých'])} ${months} ${plural(months, ['měsíc', 'měsíce', 'měsíců'])}`
}

/** Krátký popisek předvolby do přepínače. */
export function periodChipLabel(months: number): string {
  return { 1: 'Měsíčně', 3: 'Čtvrtletně', 6: 'Pololetně', 12: 'Ročně' }[months] ?? `${months} měs.`
}

function daysIn(y: number, m0: number): number {
  return new Date(Date.UTC(y, m0 + 1, 0)).getUTCDate()
}

/** Rok/měsíc (0-based, smí přetéct) s dnem oříznutým na délku měsíce. */
function clampedDate(y: number, m0: number, day: number): Date {
  const first = new Date(Date.UTC(y, m0, 1))
  const yy = first.getUTCFullYear()
  const mm = first.getUTCMonth()
  const d = Math.min(Math.max(day, 1), daysIn(yy, mm))
  return new Date(Date.UTC(yy, mm, d))
}

/**
 * Datum + N měsíců s kotvícím dnem oříznutým na délku měsíce (nedriftuje):
 * `addMonths('2026-01-31', 1, 31)` → `'2026-02-28'`. `anchorDay ≤ 0` = den z data.
 */
export function addMonths(date: string, months: number, anchorDay = 0): string | null {
  const t = parseISODate(date)
  if (!t) return null
  const day = anchorDay > 0 ? anchorDay : t.getUTCDate()
  return toISODate(clampedDate(t.getUTCFullYear(), t.getUTCMonth() + months, day))
}

/** První datum ≥ `start`, jehož den je `dayOfMonth` (oříznutý); `≤ 0` = `start`. */
export function firstOccurrence(start: string, dayOfMonth: number): string | null {
  const t = parseISODate(start)
  if (!t) return null
  if (dayOfMonth <= 0) return start
  let d = clampedDate(t.getUTCFullYear(), t.getUTCMonth(), dayOfMonth)
  if (d < t) d = clampedDate(t.getUTCFullYear(), t.getUTCMonth() + 1, dayOfMonth)
  return toISODate(d)
}

/**
 * Náhled rozvrhu: prvních `count` dat vystavení od `start` (bez konce po `endOn`).
 * Kotvou je `dayOfMonth`, jinak den `start`.
 */
export function upcomingOccurrences(
  opts: { start: string; monthsPeriod: number; dayOfMonth?: number; endOn?: string },
  count = 3,
): string[] {
  const first = firstOccurrence(opts.start, opts.dayOfMonth ?? 0)
  if (!first || opts.monthsPeriod < 1) return []
  const anchor = opts.dayOfMonth && opts.dayOfMonth > 0 ? opts.dayOfMonth : (parseISODate(opts.start)?.getUTCDate() ?? 1)
  const out: string[] = []
  for (let i = 0; i < count; i++) {
    const d = i === 0 ? first : addMonths(first, opts.monthsPeriod * i, anchor)
    if (!d || (opts.endOn && d > opts.endOn)) break
    out.push(d)
  }
  return out
}

// --- Datové placeholdery šablon ---

export type PlaceholderLang = 'cs' | 'en'

const MONTH_NAMES: Record<PlaceholderLang, readonly string[]> = {
  cs: ['leden', 'únor', 'březen', 'duben', 'květen', 'červen', 'červenec', 'srpen', 'září', 'říjen', 'listopad', 'prosinec'],
  en: ['January', 'February', 'March', 'April', 'May', 'June', 'July', 'August', 'September', 'October', 'November', 'December'],
}

export interface PlaceholderInfo {
  token: string
  label: string
}

/** Placeholdery v názvech položek a textech šablony (počítají se z data vystavení). */
export const DATE_PLACEHOLDERS: readonly PlaceholderInfo[] = [
  { token: '{MONTH_NAME}', label: 'název měsíce' },
  { token: '{MONTH}', label: 'měsíc (MM)' },
  { token: '{YEAR}', label: 'rok' },
  { token: '{PREV_MONTH_NAME}', label: 'minulý měsíc' },
  { token: '{PREV_MONTH}', label: 'minulý měsíc (MM)' },
  { token: '{NEXT_MONTH_NAME}', label: 'příští měsíc' },
  { token: '{NEXT_MONTH}', label: 'příští měsíc (MM)' },
  { token: '{PREV_YEAR}', label: 'minulý rok' },
  { token: '{NEXT_YEAR}', label: 'příští rok' },
  { token: '{QUARTER}', label: 'čtvrtletí (1–4)' },
]

const two = (n: number) => String(n).padStart(2, '0')

/** Hodnoty placeholderů pro datum vystavení; `null` pro neplatné datum. */
export function dateVars(date: string, lang: PlaceholderLang = 'cs'): Record<string, string> | null {
  const t = parseISODate(date)
  if (!t) return null
  const y = t.getUTCFullYear()
  const m0 = t.getUTCMonth()
  const prev = clampedDate(y, m0 - 1, 1).getUTCMonth()
  const next = clampedDate(y, m0 + 1, 1).getUTCMonth()
  const names = MONTH_NAMES[lang]
  return {
    MONTH: two(m0 + 1),
    MONTH_NAME: names[m0] ?? '',
    PREV_MONTH: two(prev + 1),
    PREV_MONTH_NAME: names[prev] ?? '',
    NEXT_MONTH: two(next + 1),
    NEXT_MONTH_NAME: names[next] ?? '',
    YEAR: String(y),
    PREV_YEAR: String(y - 1),
    NEXT_YEAR: String(y + 1),
    QUARTER: String(Math.floor(m0 / 3) + 1),
  }
}

/** Nahradí `{…}` z `vars`; neznámé placeholdery zůstanou beze změny. */
export function renderPlaceholders(text: string, vars: Record<string, string>): string {
  return text.replace(/\{([^{}]*)\}/g, (whole, name: string) => (Object.hasOwn(vars, name) ? (vars[name] ?? '') : whole))
}

/** „Služby za měsíc {MONTH_NAME} {YEAR}“ → „Služby za měsíc září 2026“. */
export function renderDatePlaceholders(text: string, date: string, lang: PlaceholderLang = 'cs'): string {
  if (!text.includes('{')) return text
  const vars = dateVars(date, lang)
  return vars ? renderPlaceholders(text, vars) : text
}

/** Obsahuje text nějaký známý datový placeholder? */
export function hasDatePlaceholder(text: string): boolean {
  return DATE_PLACEHOLDERS.some((p) => text.includes(p.token))
}

/** Vloží `token` do `value` místo výběru `[start, end)`; vrací nový text a pozici kurzoru. */
export function insertAt(value: string, token: string, start?: number | null, end?: number | null): { value: string; caret: number } {
  const s = start ?? value.length
  const e = end ?? s
  return { value: value.slice(0, s) + token + value.slice(e), caret: s + token.length }
}
