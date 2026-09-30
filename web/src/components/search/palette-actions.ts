// Akce a navigace v command paletě + hledání bez ohledu na diakritiku.

import type { SearchResults } from '@/api/types'

/** „Nová Faktura“ → „nova faktura“. */
export function normalizeText(s: string): string {
  return s.normalize('NFD').replace(/\p{Diacritic}/gu, '').toLowerCase().trim()
}

/** Obsahuje text (nebo některé klíčové slovo) všechna slova dotazu? Prázdný dotaz = ano. */
export function matchesQuery(query: string, label: string, keywords: string[] = []): boolean {
  const words = normalizeText(query).split(/\s+/).filter(Boolean)
  if (words.length === 0) return true
  const haystack = normalizeText([label, ...keywords].join(' '))
  return words.every((w) => haystack.includes(w))
}

export type ResultGroupKey = keyof SearchResults

/** Pořadí a popisky skupin výsledků. */
export const resultGroups: { key: ResultGroupKey; heading: string }[] = [
  { key: 'invoices', heading: 'Faktury' },
  { key: 'expenses', heading: 'Náklady' },
  { key: 'subjects', heading: 'Kontakty' },
  { key: 'price_items', heading: 'Ceník' },
]

export function countResults(r: SearchResults | undefined): number {
  if (!r) return 0
  return resultGroups.reduce((n, g) => n + (r[g.key]?.length ?? 0), 0)
}
