// Datumy: v API string `YYYY-MM-DD` (pole *_on), časy RFC3339 (pole *_at).
// Datum bez času zpracováváme vždy v UTC, aby nedocházelo k posunu o den kvůli časové zóně.

import { LOCALE } from './money'

const ISO_DATE = /^(\d{4})-(\d{2})-(\d{2})$/

const dateFmt = new Intl.DateTimeFormat(LOCALE, { day: 'numeric', month: 'numeric', year: 'numeric', timeZone: 'UTC' })
const dateLongFmt = new Intl.DateTimeFormat(LOCALE, { day: 'numeric', month: 'long', year: 'numeric', timeZone: 'UTC' })
const dateTimeFmt = new Intl.DateTimeFormat(LOCALE, {
  day: 'numeric',
  month: 'numeric',
  year: 'numeric',
  hour: 'numeric',
  minute: '2-digit',
})

/** `"2026-09-26"` → `Date` v UTC půlnoci, nebo `null` pro neplatné datum. */
export function parseISODate(iso: string): Date | null {
  const m = ISO_DATE.exec(iso)
  if (!m) return null
  const [y, mo, d] = [Number(m[1]), Number(m[2]), Number(m[3])]
  const date = new Date(Date.UTC(y, mo - 1, d))
  if (date.getUTCFullYear() !== y || date.getUTCMonth() !== mo - 1 || date.getUTCDate() !== d) return null
  return date
}

/** `Date` → `"YYYY-MM-DD"` (UTC složky). */
export function toISODate(date: Date): string {
  return date.toISOString().slice(0, 10)
}

/** Dnešní datum v lokální zóně jako `"YYYY-MM-DD"`. `now` jde podstrčit v testech. */
export function todayISO(now: Date = new Date()): string {
  const y = now.getFullYear()
  const m = String(now.getMonth() + 1).padStart(2, '0')
  const d = String(now.getDate()).padStart(2, '0')
  return `${y}-${m}-${d}`
}

/** `addDays("2026-01-30", 14)` → `"2026-02-13"`. */
export function addDays(iso: string, days: number): string {
  const date = parseISODate(iso)
  if (!date) throw new RangeError(`invalid date: ${iso}`)
  date.setUTCDate(date.getUTCDate() + days)
  return toISODate(date)
}

/** Počet dní z `from` do `to` (kladné = `to` je později). */
export function daysBetween(from: string, to: string): number {
  const a = parseISODate(from)
  const b = parseISODate(to)
  if (!a || !b) throw new RangeError(`invalid date: ${from} / ${to}`)
  return Math.round((b.getTime() - a.getTime()) / 86_400_000)
}

/** `"2026-09-26"` → `"26. 9. 2026"`. Prázdný/neplatný vstup → `""`. */
export function formatDate(iso: string | null | undefined): string {
  if (!iso) return ''
  const date = parseISODate(iso)
  return date ? dateFmt.format(date) : ''
}

/** `"2026-09-26"` → `"26. září 2026"`. */
export function formatDateLong(iso: string | null | undefined): string {
  if (!iso) return ''
  const date = parseISODate(iso)
  return date ? dateLongFmt.format(date) : ''
}

/** RFC3339 → `"26. 9. 2026 9:37"` v lokální zóně. */
export function formatDateTime(rfc3339: string | null | undefined): string {
  if (!rfc3339) return ''
  const date = new Date(rfc3339)
  return Number.isNaN(date.getTime()) ? '' : dateTimeFmt.format(date)
}

/**
 * Lidský popis splatnosti vzhledem k dnešku: „dnes“, „zítra“, „za 5 dní“, „před 3 dny“.
 */
export function formatDueRelative(dueOn: string, today: string = todayISO()): string {
  const diff = daysBetween(today, dueOn)
  if (diff === 0) return 'dnes'
  if (diff === 1) return 'zítra'
  if (diff === -1) return 'včera'
  const n = Math.abs(diff)
  if (diff > 0) return `za ${n} ${n < 5 ? 'dny' : 'dní'}`
  return `před ${n} ${n === 1 ? 'dnem' : 'dny'}`
}
