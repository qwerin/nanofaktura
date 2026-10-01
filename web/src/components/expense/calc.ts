// Živý přepočet řádků nákladu ve formuláři — zobrazovací duplikát `internal/billing.Calculate`
// (zdrojem pravdy jsou součty z backendu). Celočíselně přes BigInt, zaokrouhlení half-away.

import type { ExpenseLineInput, PriceItem } from '@/api/types'
import { divRoundHalfAway, formatMoneyInput, parseMoney } from '@/lib/money'
import { parseQuantity, quantityToMilli, QUANTITY_SCALE } from '@/lib/quantity'

export interface CalcLine {
  /** Haléře; s DPH, pokud `pricesIncludeVat`. */
  unitPrice: number
  /** API tvar množství (`"1.5"`). */
  quantity: string
  vatRateBps: number
}

export interface CalcOptions {
  pricesIncludeVat: boolean
  roundTotal: boolean
  /** Přenesená daňová povinnost: dodavatel DPH neúčtuje, sazby jsou jen informativní. */
  reverseCharge?: boolean
}

export interface LineAmounts {
  base: number
  vat: number
  total: number
}

export interface VatRecapRow extends LineAmounts {
  vatRateBps: number
}

export interface Totals {
  lines: LineAmounts[]
  /** Jedna položka na sazbu, od nejvyšší. */
  recap: VatRecapRow[]
  subtotal: number
  vatTotal: number
  rounding: number
  total: number
}

function split(amount: bigint, rate: number, pricesIncludeVat: boolean): LineAmounts {
  let vat = 0n
  if (rate !== 0) {
    const den = 10000n + (pricesIncludeVat ? BigInt(rate) : 0n)
    vat = divRoundHalfAway(amount * BigInt(rate), den)
  }
  if (pricesIncludeVat) return { base: Number(amount - vat), vat: Number(vat), total: Number(amount) }
  return { base: Number(amount), vat: Number(vat), total: Number(amount + vat) }
}

/** Součty dokladu stejně jako backend (SPEC §4.5): DPH se počítá per sazba z rekapitulace. */
export function calculateTotals(lines: CalcLine[], opts: CalcOptions): Totals {
  const sums = new Map<number, bigint>()
  const lineAmounts = lines.map((l) => {
    const amount = divRoundHalfAway(BigInt(l.unitPrice) * BigInt(quantityToMilli(l.quantity)), BigInt(QUANTITY_SCALE))
    sums.set(l.vatRateBps, (sums.get(l.vatRateBps) ?? 0n) + amount)
    return split(amount, opts.reverseCharge ? 0 : l.vatRateBps, opts.pricesIncludeVat)
  })

  const recap: VatRecapRow[] = [...sums.entries()]
    .sort(([a], [b]) => b - a)
    .map(([rate, sum]) => ({ vatRateBps: rate, ...split(sum, opts.reverseCharge ? 0 : rate, opts.pricesIncludeVat) }))

  const subtotal = recap.reduce((s, r) => s + r.base, 0)
  const vatTotal = recap.reduce((s, r) => s + r.vat, 0)
  const before = subtotal + vatTotal
  const total = opts.roundTotal ? Number(divRoundHalfAway(BigInt(before), 100n) * 100n) : before
  return { lines: lineAmounts, recap, subtotal, vatTotal, rounding: total - before, total }
}

// --- Řádky formuláře ---

/** Řádek tak, jak ho drží formulář (vstupy jsou texty, převod až při odeslání). */
export interface LineDraft {
  /** ID existujícího řádku (PATCH). */
  id?: number
  price_item_id?: number
  name: string
  quantity: string
  unit_name: string
  /** Text v korunách, např. `"1 234,50"`. */
  unit_price: string
  /** Basis points jako text (hodnota selectu). */
  vat_rate_bps: string
}

export function emptyLine(defaultVatRateBps: number): LineDraft {
  return { name: '', quantity: '1', unit_name: '', unit_price: '', vat_rate_bps: String(defaultVatRateBps) }
}

/** Řádek pro výpočet, nebo `null`, když vstupy ještě nejsou platné. */
export function draftToCalcLine(d: LineDraft): CalcLine | null {
  const unitPrice = parseMoney(d.unit_price)
  const quantity = parseQuantity(d.quantity)
  const rate = Number(d.vat_rate_bps)
  if (unitPrice === null || quantity === null || !Number.isInteger(rate)) return null
  return { unitPrice, quantity, vatRateBps: rate }
}

/** Živé součty z rozpracovaných řádků; neplatné řádky se počítají jako nulové. */
export function draftTotals(drafts: LineDraft[], opts: CalcOptions): Totals {
  return calculateTotals(
    drafts.map((d) => draftToCalcLine(d) ?? { unitPrice: 0, quantity: '0', vatRateBps: Number(d.vat_rate_bps) || 0 }),
    opts,
  )
}

/** Převod platného řádku formuláře na vstup API. Volat až po validaci. */
export function draftToLineInput(d: LineDraft): ExpenseLineInput {
  const calc = draftToCalcLine(d)
  if (!calc) throw new RangeError('invalid line')
  return {
    ...(d.id ? { id: d.id } : {}),
    ...(d.price_item_id ? { price_item_id: d.price_item_id } : {}),
    name: d.name.trim(),
    quantity: calc.quantity,
    unit_name: d.unit_name.trim(),
    unit_price: calc.unitPrice,
    vat_rate_bps: calc.vatRateBps,
  }
}

/** Řádek uloženého nákladu → řádek formuláře. */
export function lineToDraft(l: {
  id: number
  price_item_id?: number
  name: string
  quantity: string
  unit_name: string
  unit_price: number
  vat_rate_bps: number
}): LineDraft {
  return {
    id: l.id,
    ...(l.price_item_id ? { price_item_id: l.price_item_id } : {}),
    name: l.name,
    quantity: l.quantity.replace('.', ','),
    unit_name: l.unit_name,
    unit_price: formatMoneyInput(l.unit_price),
    vat_rate_bps: String(l.vat_rate_bps),
  }
}

/**
 * Přepočte jednotkovou cenu mezi „s DPH“ a „bez DPH“ (zaokrouhlení half-away na haléře).
 * Používá se, když se ceník a doklad liší v `prices_include_vat`.
 */
export function convertUnitPrice(unitPrice: number, rateBps: number, fromIncludesVat: boolean, toIncludesVat: boolean): number {
  if (fromIncludesVat === toIncludesVat || rateBps === 0) return unitPrice
  const price = BigInt(unitPrice)
  const rate = BigInt(rateBps)
  return fromIncludesVat
    ? Number(divRoundHalfAway(price * 10000n, 10000n + rate))
    : Number(divRoundHalfAway(price * (10000n + rate), 10000n))
}

/** Řádek formuláře předvyplněný z položky ceníku (název, jednotka, cena, sazba). */
export function lineFromPriceItem(item: PriceItem, pricesIncludeVat: boolean, base?: LineDraft): LineDraft {
  const price = convertUnitPrice(item.unit_price, item.vat_rate_bps, item.prices_include_vat, pricesIncludeVat)
  return {
    ...(base?.id ? { id: base.id } : {}),
    price_item_id: item.id,
    name: item.name,
    quantity: base && base.quantity.trim() !== '' ? base.quantity : '1',
    unit_name: item.unit_name,
    unit_price: formatMoneyInput(price),
    vat_rate_bps: String(item.vat_rate_bps),
  }
}

/** Je řádek úplně prázdný (uživatel na něj ještě nesáhl)? */
export function isBlankLine(d: LineDraft): boolean {
  return d.name.trim() === '' && d.unit_price.trim() === '' && !d.price_item_id
}
