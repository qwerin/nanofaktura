// Popisky a řazení úkolů (čistá logika, testovatelná).

import type { Todo, TodoRelatedType } from '@/api/types'
import { daysBetween, todayISO } from '@/lib/date'

/** Filtry seznamu úkolů podle typu vazby (hodnota `related_type` v API). */
export const todoTypeFilters: { value: TodoRelatedType; label: string }[] = [
  { value: 'invoice', label: 'Faktury' },
  { value: 'expense', label: 'Náklady' },
  { value: 'bank_transaction', label: 'Banka' },
  { value: 'price_item', label: 'Sklad' },
  { value: 'recurring', label: 'Pravidelné' },
  { value: 'subject', label: 'Kontakty' },
]

/** Text odkazu na záznam úkolu: „Otevřít fakturu“ … */
export const relatedLinkLabels: Record<string, string> = {
  invoice: 'Otevřít fakturu',
  expense: 'Otevřít náklad',
  subject: 'Otevřít kontakt',
  price_item: 'Otevřít položku',
  recurring: 'Otevřít pravidelnou fakturu',
  bank_transaction: 'Otevřít banku',
}

export type DueState = 'none' | 'overdue' | 'today' | 'soon' | 'later'

/** Stav termínu otevřeného úkolu vzhledem k dnešku (hotové úkoly → `none`). */
export function todoDueState(todo: Pick<Todo, 'due_on' | 'completed'>, today: string = todayISO()): DueState {
  if (!todo.due_on || todo.completed) return 'none'
  const diff = daysBetween(today, todo.due_on)
  if (diff < 0) return 'overdue'
  if (diff === 0) return 'today'
  if (diff <= 3) return 'soon'
  return 'later'
}
