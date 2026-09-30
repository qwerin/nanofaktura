// Relativní čas pro timeline a aktivitu: „právě teď“, „před 5 min“, „před 2 h“, „včera 14:05“, „před 3 dny“.
// Starší než týden → datum (bez roku, je-li letošní).

import { LOCALE } from './money'

const timeFmt = new Intl.DateTimeFormat(LOCALE, { hour: 'numeric', minute: '2-digit' })
const dateFmt = new Intl.DateTimeFormat(LOCALE, { day: 'numeric', month: 'numeric', year: 'numeric' })
const dateNoYearFmt = new Intl.DateTimeFormat(LOCALE, { day: 'numeric', month: 'numeric' })

function startOfDay(d: Date): number {
  return new Date(d.getFullYear(), d.getMonth(), d.getDate()).getTime()
}

/** Počet kalendářních dní mezi dvěma okamžiky v lokální zóně (kladné = `to` je později). */
function calendarDays(from: Date, to: Date): number {
  return Math.round((startOfDay(to) - startOfDay(from)) / 86_400_000)
}

/** RFC 3339 → lidský relativní čas vzhledem k `now`. Neplatný vstup → `""`. */
export function formatRelativeTime(rfc3339: string | null | undefined, now: Date = new Date()): string {
  if (!rfc3339) return ''
  const date = new Date(rfc3339)
  if (Number.isNaN(date.getTime())) return ''
  const seconds = Math.round((now.getTime() - date.getTime()) / 1000)
  // Drobný posun hodin serveru a klienta nesmí vypsat „za 3 s“.
  if (seconds < 45) return 'právě teď'
  const minutes = Math.round(seconds / 60)
  if (minutes < 60) return `před ${minutes} min`
  const days = calendarDays(date, now)
  const hours = Math.floor(minutes / 60)
  if (days === 0 || hours < 6) return `před ${hours} h`
  if (days === 1) return `včera ${timeFmt.format(date)}`
  if (days < 7) return `před ${days} dny` // 7. pád: „před 2 dny“ i „před 6 dny“
  return date.getFullYear() === now.getFullYear() ? dateNoYearFmt.format(date) : dateFmt.format(date)
}
