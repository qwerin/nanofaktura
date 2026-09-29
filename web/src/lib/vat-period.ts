// Zdaňovací období DPH: `YYYY-MM` (měsíc) nebo `YYYY-Qn` (čtvrtletí) — stejný formát jako `?period=` API.

export type VatPeriodKind = 'month' | 'quarter'

export type VatPeriod =
  | { kind: 'month'; year: number; month: number } // month 1–12
  | { kind: 'quarter'; year: number; quarter: number } // quarter 1–4

const MONTH_RE = /^(\d{4})-(0[1-9]|1[0-2])$/
const QUARTER_RE = /^(\d{4})-Q([1-4])$/

const MONTH_NAMES = ['leden', 'únor', 'březen', 'duben', 'květen', 'červen', 'červenec', 'srpen', 'září', 'říjen', 'listopad', 'prosinec']

/** `"2026-09"` / `"2026-Q3"` → období, jinak `null`. */
export function parseVatPeriod(s: string | undefined | null): VatPeriod | null {
  if (!s) return null
  let m = MONTH_RE.exec(s)
  if (m) return { kind: 'month', year: Number(m[1]), month: Number(m[2]) }
  m = QUARTER_RE.exec(s)
  if (m) return { kind: 'quarter', year: Number(m[1]), quarter: Number(m[2]) }
  return null
}

export function formatVatPeriod(p: VatPeriod): string {
  return p.kind === 'month' ? `${p.year}-${String(p.month).padStart(2, '0')}` : `${p.year}-Q${p.quarter}`
}

/** Posun o `delta` období stejného druhu (přes přelom roku). */
export function shiftVatPeriod(p: VatPeriod, delta: number): VatPeriod {
  if (p.kind === 'month') {
    const idx = p.year * 12 + (p.month - 1) + delta
    return { kind: 'month', year: Math.floor(idx / 12), month: (((idx % 12) + 12) % 12) + 1 }
  }
  const idx = p.year * 4 + (p.quarter - 1) + delta
  return { kind: 'quarter', year: Math.floor(idx / 4), quarter: (((idx % 4) + 4) % 4) + 1 }
}

/** Období obsahující datum `today` (`YYYY-MM-DD`, bere se jen rok a měsíc). */
export function vatPeriodOf(kind: VatPeriodKind, today: string): VatPeriod {
  const year = Number(today.slice(0, 4))
  const month = Number(today.slice(5, 7))
  return kind === 'month' ? { kind, year, month } : { kind, year, quarter: Math.ceil(month / 3) }
}

/** Výchozí období reportu = předchozí (za které se podává přiznání). Stejně jako backend bez `period`. */
export function defaultVatPeriod(kind: VatPeriodKind, today: string): VatPeriod {
  return shiftVatPeriod(vatPeriodOf(kind, today), -1)
}

/** „září 2026“ / „3. čtvrtletí 2026“. */
export function vatPeriodLabel(p: VatPeriod): string {
  return p.kind === 'month' ? `${MONTH_NAMES[p.month - 1]} ${p.year}` : `${p.quarter}. čtvrtletí ${p.year}`
}

/** První a poslední den období (`YYYY-MM-DD`). */
export function vatPeriodRange(p: VatPeriod): { from: string; to: string } {
  const firstMonth = p.kind === 'month' ? p.month : (p.quarter - 1) * 3 + 1
  const lastMonth = p.kind === 'month' ? p.month : firstMonth + 2
  const lastDay = new Date(Date.UTC(p.year, lastMonth, 0)).getUTCDate()
  const mm = (m: number) => String(m).padStart(2, '0')
  return { from: `${p.year}-${mm(firstMonth)}-01`, to: `${p.year}-${mm(lastMonth)}-${lastDay}` }
}

/**
 * Lhůta pro podání přiznání a kontrolního hlášení: 25. den po skončení období
 * (bez posunu za víkend/svátek — ten řeší daňový řád, tady jen orientačně).
 */
export function vatFilingDeadline(p: VatPeriod): string {
  const { to } = vatPeriodRange(p)
  const y = Number(to.slice(0, 4))
  const m = Number(to.slice(5, 7)) // 1–12, následující měsíc = m+1
  const ny = m === 12 ? y + 1 : y
  const nm = m === 12 ? 1 : m + 1
  return `${ny}-${String(nm).padStart(2, '0')}-25`
}

/** Zda je období celé v minulosti (lze podávat). */
export function isVatPeriodClosed(p: VatPeriod, today: string): boolean {
  return vatPeriodRange(p).to < today
}
