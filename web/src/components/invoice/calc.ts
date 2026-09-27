// Živý přepočet faktury ve formuláři — zobrazovací duplikát internal/billing/calc.go.
// Zdrojem pravdy je backend: po uložení se zobrazují hodnoty ze serveru.

import { divRoundHalfAway } from '@/lib/money'
import { lineAmount } from '@/lib/quantity'

export interface CalcLine {
  /** API tvar množství (`"1.5"`), neplatné → řádek se počítá jako 0. */
  quantity: string
  unitPrice: number
  vatRateBps: number
}

export interface CalcOptions {
  pricesIncludeVat: boolean
  reverseCharge: boolean
  roundTotal: boolean
  nonVatPayer: boolean
}

export interface CalcAmounts {
  base: number
  vat: number
  total: number
}

export interface CalcRecap extends CalcAmounts {
  vatRateBps: number
}

export interface CalcResult {
  lines: CalcAmounts[]
  vatRecap: CalcRecap[]
  subtotal: number
  vatTotal: number
  rounding: number
  total: number
}

function mulDiv(a: number, b: number, den: number): number {
  return Number(divRoundHalfAway(BigInt(a) * BigInt(b), BigInt(den)))
}

function split(amount: number, rate: number, o: CalcOptions): CalcAmounts {
  let vat = 0
  if (!o.reverseCharge && !o.nonVatPayer && rate !== 0) {
    vat = mulDiv(amount, rate, o.pricesIncludeVat ? 10000 + rate : 10000)
  }
  return o.pricesIncludeVat
    ? { base: amount - vat, vat, total: amount }
    : { base: amount, vat, total: amount + vat }
}

function safeLineAmount(unitPrice: number, quantity: string): number {
  try {
    return lineAmount(unitPrice, quantity)
  } catch {
    return 0
  }
}

/** Součty dokladu: řádky, rekapitulace DPH (nejvyšší sazba první), zaokrouhlení. */
export function calculateTotals(lines: CalcLine[], o: CalcOptions): CalcResult {
  const sums = new Map<number, number>()
  const out: CalcAmounts[] = lines.map((l) => {
    const rate = o.nonVatPayer ? 0 : l.vatRateBps
    const amount = safeLineAmount(l.unitPrice, l.quantity)
    sums.set(rate, (sums.get(rate) ?? 0) + amount)
    return split(amount, rate, o)
  })

  const vatRecap: CalcRecap[] = [...sums.keys()]
    .sort((a, b) => b - a)
    .map((rate) => ({ vatRateBps: rate, ...split(sums.get(rate) ?? 0, rate, o) }))
  const subtotal = vatRecap.reduce((s, r) => s + r.base, 0)
  const vatTotal = vatRecap.reduce((s, r) => s + r.vat, 0)
  const before = subtotal + vatTotal
  const total = o.roundTotal ? Number(divRoundHalfAway(BigInt(before), 100n)) * 100 : before
  return { lines: out, vatRecap, subtotal, vatTotal, rounding: total - before, total }
}
