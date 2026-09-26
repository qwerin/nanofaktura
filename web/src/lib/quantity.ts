// Množství: v API decimální string s tečkou (`"1.5"`, max 3 desetinná místa, smí být záporné).
// Interně pro výpočty tisíciny (`milli`), stejně jako backend (`quantity_milli`).

import { divRoundHalfAway, LOCALE } from './money'

export const QUANTITY_SCALE = 1000
const MAX_DECIMALS = 3

/**
 * Normalizuje uživatelský vstup („1,5“, „ 2 “, „-0.250“) na API tvar (`"1.5"`, `"2"`, `"-0.25"`).
 * Vrací `null` pro neplatný vstup nebo víc než 3 desetinná místa.
 */
export function parseQuantity(input: string): string | null {
  const s = input.trim().replace(/[\s\u00a0\u202f]/g, '').replace(',', '.').replace(/[\u2212\u2013]/g, '-')
  const m = /^([-+]?)(\d*)(?:\.(\d*))?$/.exec(s)
  if (!m) return null
  const [, sign = '', intRaw = '', fracRaw = ''] = m
  if (intRaw === '' && fracRaw === '') return null
  if (fracRaw.length > MAX_DECIMALS) return null
  const intPart = intRaw.replace(/^0+(?=\d)/, '') || '0'
  const frac = fracRaw.replace(/0+$/, '')
  const body = frac ? `${intPart}.${frac}` : intPart
  if (body === '0') return '0'
  return sign === '-' ? `-${body}` : body
}

/** API string → tisíciny: `"1.5"` → `1500`. Neplatný vstup vyhodí chybu. */
export function quantityToMilli(quantity: string): number {
  const normalized = parseQuantity(quantity)
  if (normalized === null) throw new RangeError(`invalid quantity: ${quantity}`)
  const negative = normalized.startsWith('-')
  const [intPart = '0', frac = ''] = normalized.replace('-', '').split('.')
  const milli = Number(intPart) * QUANTITY_SCALE + Number(frac.padEnd(MAX_DECIMALS, '0'))
  return negative ? -milli : milli
}

/** Tisíciny → API string: `1500` → `"1.5"`. */
export function milliToQuantity(milli: number): string {
  const negative = milli < 0
  const abs = Math.abs(milli)
  const intPart = Math.trunc(abs / QUANTITY_SCALE)
  const frac = String(abs % QUANTITY_SCALE).padStart(MAX_DECIMALS, '0').replace(/0+$/, '')
  const body = frac ? `${intPart}.${frac}` : String(intPart)
  return negative && body !== '0' ? `-${body}` : body
}

/** Zobrazení množství v češtině: `"1.5"` → `"1,5"`, `"1234"` → `"1 234"`. */
export function formatQuantity(quantity: string): string {
  const milli = quantityToMilli(quantity)
  return new Intl.NumberFormat(LOCALE, { maximumFractionDigits: MAX_DECIMALS }).format(
    milli / QUANTITY_SCALE,
  )
}

/**
 * Cena řádku = round_half_away(unit_price × quantity). Jen pro živý náhled ve formuláři —
 * zdrojem pravdy jsou součty z backendu.
 */
export function lineAmount(unitPrice: number, quantity: string): number {
  const milli = quantityToMilli(quantity)
  return Number(divRoundHalfAway(BigInt(unitPrice) * BigInt(milli), BigInt(QUANTITY_SCALE)))
}
