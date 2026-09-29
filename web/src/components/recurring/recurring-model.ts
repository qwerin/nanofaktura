// Model formuláře pravidelné faktury (čisté funkce, testy v recurring-model.test.ts).

import { z } from 'zod'
import type { CreateRecurringInput, Recurring, UpdateRecurringInput } from '@/api/types'
import { parseISODate } from '@/lib/date'
import { upcomingOccurrences } from './period'

export const recurringFormSchema = z
  .object({
    name: z.string().trim().min(1, 'Zadejte název').max(200, 'Nejvýše 200 znaků'),
    template_id: z.string().min(1, 'Vyberte šablonu'),
    start_on: z.string().refine((v) => parseISODate(v) !== null, 'Zadejte datum'),
    months_period: z
      .string()
      .trim()
      .regex(/^\d{1,3}$/, 'Počet měsíců 1–120')
      .refine((v) => Number(v) >= 1 && Number(v) <= 120, 'Počet měsíců 1–120'),
    day_of_month: z
      .string()
      .trim()
      .refine((v) => v === '' || (/^\d{1,2}$/.test(v) && Number(v) >= 1 && Number(v) <= 31), 'Den 1–31'),
    end_on: z.string().refine((v) => v === '' || parseISODate(v) !== null, 'Neplatné datum'),
    issue_as: z.enum(['invoice', 'proforma']),
    send_email: z.boolean(),
    active: z.boolean(),
  })
  .superRefine((v, ctx) => {
    if (v.end_on && v.start_on && v.end_on < v.start_on) {
      ctx.addIssue({ code: 'custom', path: ['end_on'], message: 'Konec musí být po začátku' })
    }
  })

export type RecurringFormValues = z.input<typeof recurringFormSchema>

export function newRecurringValues(today: string, templateId?: number): RecurringFormValues {
  return {
    name: '',
    template_id: templateId ? String(templateId) : '',
    start_on: today,
    months_period: '1',
    day_of_month: '',
    end_on: '',
    issue_as: 'invoice',
    send_email: false,
    active: true,
  }
}

export function recurringToValues(r: Recurring): RecurringFormValues {
  return {
    name: r.name,
    template_id: String(r.template_id),
    start_on: r.start_on,
    months_period: String(r.months_period),
    day_of_month: r.day_of_month ? String(r.day_of_month) : '',
    end_on: r.end_on,
    issue_as: r.issue_as,
    send_email: r.send_email,
    active: r.active,
  }
}

export function toCreateRecurringBody(v: RecurringFormValues): CreateRecurringInput {
  const body: CreateRecurringInput = {
    name: v.name.trim(),
    template_id: Number(v.template_id),
    start_on: v.start_on,
    months_period: Number(v.months_period),
    issue_as: v.issue_as,
    send_email: v.send_email,
    active: v.active,
  }
  if (v.day_of_month.trim()) body.day_of_month = Number(v.day_of_month)
  if (v.end_on) body.end_on = v.end_on
  return body
}

/** PATCH jen změněných polí (`day_of_month` 0 = den zahájení, `end_on` "" = bez konce). */
export function toPatchRecurringBody(
  v: RecurringFormValues,
  dirty: Partial<Record<keyof RecurringFormValues, unknown>>,
): UpdateRecurringInput {
  const patch: UpdateRecurringInput = {}
  if (dirty.name) patch.name = v.name.trim()
  if (dirty.template_id) patch.template_id = Number(v.template_id)
  if (dirty.start_on) patch.start_on = v.start_on
  if (dirty.months_period) patch.months_period = Number(v.months_period)
  if (dirty.day_of_month) patch.day_of_month = v.day_of_month.trim() ? Number(v.day_of_month) : 0
  if (dirty.end_on) patch.end_on = v.end_on
  if (dirty.issue_as) patch.issue_as = v.issue_as
  if (dirty.send_email) patch.send_email = v.send_email
  return patch
}

/**
 * Kolik faktur plánovač vystaví zpětně (výskyty od začátku do dneška včetně) —
 * u začátku v minulosti se zmeškaná období dohánějí, každá k datu svého výskytu.
 */
export function missedOccurrences(
  opts: { start: string; monthsPeriod: number; dayOfMonth?: number; endOn?: string },
  today: string,
  limit = 1000,
): number {
  const dates = upcomingOccurrences(opts, limit)
  let n = 0
  for (const d of dates) {
    if (d > today) break
    n++
  }
  return n
}
