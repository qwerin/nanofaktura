// Popisky a drobné čisté helpery pro náklady (a ceník — sdílené volby DPH/měn/plateb).

import type { ExpensePaymentMethod, ExpenseStatus } from '@/api/types'
import type { SelectOption } from '@/components/form/fields'
import { formatVatRate } from '@/lib/money'

export const expenseStatusLabels: Record<ExpenseStatus, string> = {
  open: 'Neuhrazeno',
  overdue: 'Po splatnosti',
  paid: 'Uhrazeno',
}

/** Stavy ve filtru: `unpaid` = ve splatnosti i po ní (API pseudo-stav). */
export const expenseStatusOptions: SelectOption<ExpenseStatus | 'unpaid'>[] = [
  { value: 'unpaid', label: 'Neuhrazené' },
  { value: 'overdue', label: 'Po splatnosti' },
  { value: 'paid', label: 'Uhrazené' },
]

export const paymentMethodLabels: Record<ExpensePaymentMethod, string> = {
  bank: 'Bankovní převod',
  cash: 'Hotově',
  card: 'Kartou',
  cod: 'Dobírka',
  paypal: 'PayPal',
  custom: 'Jiný',
}

export const paymentMethodOptions: SelectOption<ExpensePaymentMethod>[] = (
  Object.entries(paymentMethodLabels) as [ExpensePaymentMethod, string][]
).map(([value, label]) => ({ value, label }))

export function paymentMethodLabel(method: string): string {
  return paymentMethodLabels[method as ExpensePaymentMethod] ?? method
}

export const CURRENCIES = ['CZK', 'EUR', 'USD', 'GBP', 'PLN'] as const

/** Volby měny; neznámou aktuální hodnotu (např. z API) přidá, aby se dala zobrazit. */
export function currencyOptions(current?: string): SelectOption[] {
  const list: string[] = [...CURRENCIES]
  if (current && !list.includes(current)) list.push(current)
  return list.map((c) => ({ value: c, label: c }))
}

/** Aktuální sazby DPH v ČR (+ případná jiná sazba z uloženého řádku). */
export const VAT_RATES_BPS = [2100, 1200, 0] as const

export function vatRateOptions(extra: number[] = []): SelectOption[] {
  const rates = [...new Set<number>([...VAT_RATES_BPS, ...extra.filter((r) => Number.isInteger(r))])].sort((a, b) => b - a)
  return rates.map((r) => ({ value: String(r), label: formatVatRate(r) }))
}

/** Součet částky přes položky, rozdělený podle měny (seznam může míchat CZK a EUR). */
export function sumByCurrency<T extends { currency: string }>(
  items: readonly T[],
  amount: (item: T) => number,
): { currency: string; amount: number }[] {
  const sums = new Map<string, number>()
  for (const it of items) sums.set(it.currency, (sums.get(it.currency) ?? 0) + amount(it))
  return [...sums.entries()]
    .map(([currency, value]) => ({ currency, amount: value }))
    .sort((a, b) => (a.currency === 'CZK' ? -1 : b.currency === 'CZK' ? 1 : a.currency.localeCompare(b.currency)))
}

/** Počet aktivních filtrů (pro odznak na tlačítku „Filtry“). */
export function countActiveFilters(filters: Record<string, unknown>, ignore: readonly string[] = []): number {
  return Object.entries(filters).filter(([k, v]) => !ignore.includes(k) && v !== undefined && v !== '').length
}

/** České skloňování: `plural(3, ['náklad', 'náklady', 'nákladů'])` → `"náklady"`. */
export function plural(n: number, forms: readonly [string, string, string]): string {
  const abs = Math.abs(n)
  if (abs === 1) return forms[0]
  if (abs >= 2 && abs <= 4) return forms[1]
  return forms[2]
}
