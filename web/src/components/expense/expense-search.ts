import { z } from 'zod'
import type { ExpenseListFilters } from '@/api/types'
import { parseISODate } from '@/lib/date'

const isoDate = z.string().refine((v) => parseISODate(v) !== null)

/** Filtry seznamu nákladů v URL. Neplatné hodnoty se tiše zahodí. */
export const expenseSearchSchema = z.object({
  status: z.enum(['unpaid', 'open', 'overdue', 'paid']).optional().catch(undefined),
  category: z.string().optional().catch(undefined),
  subject_id: z.coerce.number().int().positive().optional().catch(undefined),
  since: isoDate.optional().catch(undefined),
  until: isoDate.optional().catch(undefined),
  query: z.string().optional().catch(undefined),
})

export type ExpenseSearch = z.infer<typeof expenseSearchSchema>

/** URL filtry → query parametry API (prázdné hodnoty vynechá). */
export function searchToFilters(s: ExpenseSearch): ExpenseListFilters {
  const f: ExpenseListFilters = {}
  if (s.status) f.status = s.status
  if (s.category) f.category = s.category
  if (s.subject_id) f.subject_id = s.subject_id
  if (s.since) f.since = s.since
  if (s.until) f.until = s.until
  const q = s.query?.trim()
  if (q) f.query = q
  return f
}
