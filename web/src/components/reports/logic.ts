// Čisté pomocníky přehledů (osa grafu, podíly, varianty výdajů) — testované v reports.test.ts.

import type { IncomeTax } from '@/api/types'

export const MONTHS_SHORT = ['led', 'úno', 'bře', 'dub', 'kvě', 'čvn', 'čvc', 'srp', 'zář', 'říj', 'lis', 'pro']
export const MONTHS_LONG = ['leden', 'únor', 'březen', 'duben', 'květen', 'červen', 'červenec', 'srpen', 'září', 'říjen', 'listopad', 'prosinec']

/** „Hezký“ strop osy: 1, 2, 2.5, 5 × 10^n nad hodnotou. */
export function niceCeil(value: number): number {
  if (value <= 0) return 0
  const exp = 10 ** Math.floor(Math.log10(value))
  for (const m of [1, 2, 2.5, 5, 10]) {
    if (m * exp >= value) return m * exp
  }
  return 10 * exp
}

/** Rozsah osy Y v hlavních jednotkách (Kč): [min ≤ 0, max ≥ 0], obojí „hezké“, a rysky. */
export function axisDomain(valuesMinor: number[]): { min: number; max: number; ticks: number[] } {
  const major = valuesMinor.map((v) => v / 100)
  const hi = Math.max(0, ...major)
  const lo = Math.min(0, ...major)
  let max = niceCeil(hi)
  let min = lo < 0 ? -niceCeil(-lo) : 0
  if (max === 0 && min === 0) max = 1
  // Krok rysek: čtvrtina většího rozpětí, aby nula ležela na rysce.
  const step = Math.max(max, -min) / (min < 0 && max > 0 ? 2 : 4)
  if (max > 0) max = Math.ceil(max / step) * step
  if (min < 0) min = -Math.ceil(-min / step) * step
  const ticks: number[] = []
  for (let t = min; t <= max + step / 2; t += step) ticks.push(Math.round(t * 100) / 100)
  return { min, max, ticks }
}

/** Podíl zákazníka na tržbách (0–1); bez tržeb 0. */
export function customerShare(total: number, revenueTotal: number): number {
  if (revenueTotal <= 0) return 0
  return Math.max(0, total / revenueTotal)
}

/** Druhy činnosti, pro které se paušál používá (§ 7 odst. 7 ZDP), zkráceně. */
export const flatRateActivities: Record<number, string> = {
  80: 'Řemeslné živnosti, zemědělství, lesnictví',
  60: 'Ostatní živnosti (volné, vázané, koncesované)',
  40: 'Svobodná povolání, autorská práva, jiné podnikání',
  30: 'Pronájem obchodního majetku',
}

export interface TaxOption {
  id: string
  title: string
  subtitle: string
  expenses: number
  taxBase: number
  capReached: boolean
  cap?: number
}

/** Všechny varianty (skutečné výdaje + paušály) a id té s nejnižším základem daně. */
export function taxOptions(t: IncomeTax): { options: TaxOption[]; bestId: string | null } {
  const options: TaxOption[] = [
    {
      id: 'real',
      title: 'Skutečné výdaje',
      subtitle: 'Zaplacené daňově uznatelné náklady',
      expenses: t.real_expenses,
      taxBase: t.real_tax_base,
      capReached: false,
    },
    ...t.flat_rates.map((f) => ({
      id: `flat-${f.percent}`,
      title: `Paušál ${f.percent} %`,
      subtitle: flatRateActivities[f.percent] ?? '',
      expenses: f.expenses,
      taxBase: f.tax_base,
      capReached: f.expenses >= f.cap && f.cap > 0,
      cap: f.cap,
    })),
  ]
  if (t.income <= 0) return { options, bestId: null }
  let best = options[0]!
  for (const o of options) if (o.taxBase < best.taxBase) best = o
  return { options, bestId: best.id }
}
