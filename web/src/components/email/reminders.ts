// Kroky automatických upomínek (dny po splatnosti) — stejná pravidla jako backend: 1–365, seřazené, bez duplicit.

import { plural } from '@/components/invoice/status'

export const REMINDER_PRESETS = [3, 7, 14, 30, 60] as const
export const MAX_REMINDER_DAY = 365

export function normalizeReminderDays(days: readonly number[]): number[] {
  return [...new Set(days.filter((d) => Number.isInteger(d) && d >= 1 && d <= MAX_REMINDER_DAY))].sort((a, b) => a - b)
}

/** `[3, 14, 30]` → „3, 14 a 30 dní po splatnosti“. */
export function reminderSummary(days: readonly number[]): string {
  const d = normalizeReminderDays(days)
  if (d.length === 0) return 'žádná upomínka'
  const last = d[d.length - 1] ?? 0
  const list = d.length === 1 ? String(last) : `${d.slice(0, -1).join(', ')} a ${last}`
  return `${list} ${plural(last, ['den', 'dny', 'dní'])} po splatnosti`
}
