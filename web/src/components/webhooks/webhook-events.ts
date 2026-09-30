// Výběr událostí webhooku: přesné názvy, „prefix.*“ a „*“ (SPEC §7.10). Čistá logika, testovaná.

import type { EventCatalogEntry } from '@/api/types'

export const ALL_EVENTS = '*'

const prefixLabels: Record<string, string> = {
  invoice: 'Faktury',
  payment: 'Platby faktur',
  email: 'E-maily',
  public: 'Veřejný odkaz',
  expense: 'Náklady',
  expense_payment: 'Platby nákladů',
  subject: 'Kontakty',
  price_item: 'Ceník',
  stock: 'Sklad',
  recurring: 'Pravidelné faktury',
  bank: 'Banka',
  webhook: 'Webhooky',
}

export function prefixOf(name: string): string {
  const i = name.indexOf('.')
  return i < 0 ? name : name.slice(0, i)
}

export function prefixLabel(prefix: string): string {
  return prefixLabels[prefix] ?? prefix
}

export interface CatalogGroup {
  prefix: string
  label: string
  /** Hodnota zástupného filtru, např. `invoice.*`. */
  wildcard: string
  entries: EventCatalogEntry[]
}

/** Katalog seskupený podle prefixu (pořadí skupin i položek jako v katalogu). */
export function groupCatalog(entries: readonly EventCatalogEntry[]): CatalogGroup[] {
  const groups = new Map<string, CatalogGroup>()
  for (const e of entries) {
    const prefix = prefixOf(e.name)
    let g = groups.get(prefix)
    if (!g) {
      g = { prefix, label: prefixLabel(prefix), wildcard: `${prefix}.*`, entries: [] }
      groups.set(prefix, g)
    }
    g.entries.push(e)
  }
  return [...groups.values()]
}

/** Pokrývá výběr danou událost? (`*`, `prefix.*` nebo přesný název) */
export function isCovered(selected: readonly string[], name: string): boolean {
  return selected.includes(ALL_EVENTS) || selected.includes(`${prefixOf(name)}.*`) || selected.includes(name)
}

/** Zapne/vypne jednu událost. */
export function toggleEvent(selected: readonly string[], name: string): string[] {
  return selected.includes(name) ? selected.filter((s) => s !== name) : [...selected, name]
}

/** Zapne/vypne celou skupinu (`prefix.*`); jednotlivé názvy skupiny se při zapnutí odstraní jako nadbytečné. */
export function toggleGroup(selected: readonly string[], prefix: string): string[] {
  const wildcard = `${prefix}.*`
  if (selected.includes(wildcard)) return selected.filter((s) => s !== wildcard)
  return [...selected.filter((s) => s === ALL_EVENTS || prefixOf(s) !== prefix || s.endsWith('.*')), wildcard]
}

/** „Všechny události“: zapnutí nahradí celý výběr hvězdičkou. */
export function toggleAll(selected: readonly string[]): string[] {
  return selected.includes(ALL_EVENTS) ? [] : [ALL_EVENTS]
}

/** Krátký popis výběru pro seznam: „Všechny události“, „Faktury (vše), payment.created“ … */
export function describeEvents(selected: readonly string[]): string[] {
  if (selected.length === 0 || selected.includes(ALL_EVENTS)) return ['Všechny události']
  return selected.map((s) => (s.endsWith('.*') ? `${prefixLabel(s.slice(0, -2))} (vše)` : s))
}

/** Tón stavového kódu odpovědi: 2xx úspěch, 0 = bez odpovědi / síťová chyba. */
export function httpStatusTone(status: number): 'success' | 'destructive' | 'muted' {
  if (status === 0) return 'muted'
  return status >= 200 && status < 300 ? 'success' : 'destructive'
}
