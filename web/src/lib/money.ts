// Peníze: v API i v aplikaci vždy celé číslo v nejmenší jednotce (haléře/centy).
// Na float se převádí jen při zobrazení (Intl), parsování je čistě řetězcové.

export const LOCALE = 'cs-CZ'
export const DEFAULT_CURRENCY = 'CZK'

/** Počet nejmenších jednotek v jedné hlavní (API používá vždy 2 desetinná místa). */
export const MINOR_PER_MAJOR = 100

const formatterCache = new Map<string, Intl.NumberFormat>()

function currencyFormatter(currency: string, fractionDigits: number): Intl.NumberFormat {
  const key = `${currency}:${fractionDigits}`
  let f = formatterCache.get(key)
  if (!f) {
    f = new Intl.NumberFormat(LOCALE, {
      style: 'currency',
      currency,
      minimumFractionDigits: fractionDigits,
      maximumFractionDigits: fractionDigits,
    })
    formatterCache.set(key, f)
  }
  return f
}

/**
 * Naformátuje částku v haléřích, např. `formatMoney(123450)` → `"1 234,50 Kč"`.
 * `opts.hideZeroDecimals` vynechá `,00` u celých částek (vhodné pro dashboard).
 */
export function formatMoney(
  amount: number,
  currency: string = DEFAULT_CURRENCY,
  opts: { hideZeroDecimals?: boolean } = {},
): string {
  const digits = opts.hideZeroDecimals && amount % MINOR_PER_MAJOR === 0 ? 0 : 2
  return currencyFormatter(currency, digits).format(amount / MINOR_PER_MAJOR)
}

/** Částka bez měny pro předvyplnění inputu: `123450` → `"1234,50"`. */
export function formatMoneyInput(amount: number): string {
  const negative = amount < 0
  const abs = Math.abs(amount)
  const major = Math.trunc(abs / MINOR_PER_MAJOR)
  const minor = abs % MINOR_PER_MAJOR
  return `${negative ? '-' : ''}${major},${String(minor).padStart(2, '0')}`
}

/**
 * Parsuje uživatelský vstup na haléře. Přijímá mezery (i nezlomitelné) jako oddělovač tisíců,
 * čárku nebo tečku jako desetinný oddělovač, volitelnou měnu („Kč“, „CZK“, „€“) a znaménko minus.
 * Vrací `null`, pokud vstup není platná částka nebo má víc než 2 desetinná místa.
 *
 * `parseMoney("1 234,50")` → `123450`
 */
export function parseMoney(input: string): number | null {
  let s = input
    .trim()
    .replace(/[\s\u00a0\u202f']/g, '')
    .replace(/(kč|czk|eur|usd|€|\$)/gi, '')
    .replace(/[\u2212\u2013]/g, '-') // typografické mínus
  if (s === '') return null

  let negative = false
  if (s.startsWith('-')) {
    negative = true
    s = s.slice(1)
  } else if (s.startsWith('+')) {
    s = s.slice(1)
  }

  const lastComma = s.lastIndexOf(',')
  const lastDot = s.lastIndexOf('.')
  let intPart: string
  let fracPart = ''

  if (lastComma >= 0 && lastDot >= 0) {
    // Oba oddělovače: poslední je desetinný, ten druhý jsou tisíce.
    const decIdx = Math.max(lastComma, lastDot)
    const thousandsSep = decIdx === lastComma ? '.' : ','
    intPart = s.slice(0, decIdx).split(thousandsSep).join('')
    fracPart = s.slice(decIdx + 1)
  } else if (lastComma >= 0 || lastDot >= 0) {
    const sep = lastComma >= 0 ? ',' : '.'
    const parts = s.split(sep)
    if (parts.length > 2) {
      // „1.234.567“ — více oddělovačů stejného typu = tisíce
      if (!parts.slice(1).every((p) => p.length === 3)) return null
      intPart = parts.join('')
    } else {
      intPart = parts[0] ?? ''
      fracPart = parts[1] ?? ''
    }
  } else {
    intPart = s
  }

  if (intPart === '') intPart = '0'
  if (!/^\d+$/.test(intPart) || !/^\d{0,2}$/.test(fracPart)) return null

  const value = Number(intPart) * MINOR_PER_MAJOR + Number(fracPart.padEnd(2, '0'))
  if (!Number.isSafeInteger(value)) return null
  return negative && value !== 0 ? -value : value
}

/**
 * Zaokrouhlení „half away from zero“ podílu dvou celých čísel (BigInt, bez ztráty přesnosti).
 * Stejné pravidlo používá backend (internal/billing).
 */
export function divRoundHalfAway(numerator: bigint, denominator: bigint): bigint {
  if (denominator === 0n) throw new RangeError('division by zero')
  const negative = numerator < 0n !== denominator < 0n
  const n = numerator < 0n ? -numerator : numerator
  const d = denominator < 0n ? -denominator : denominator
  const q = (n * 2n + d) / (d * 2n)
  return negative ? -q : q
}

/** Sazba DPH v basis points → text: `2100` → `"21 %"`, `1250` → `"12,5 %"`. */
export function formatVatRate(bps: number): string {
  return `${new Intl.NumberFormat(LOCALE, { maximumFractionDigits: 2 }).format(bps / 100)}\u00a0%`
}
