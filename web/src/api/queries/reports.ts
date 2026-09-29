import { queryOptions } from '@tanstack/react-query'
import { api, unwrap } from '../client'
import { keys } from './keys'

function base(): string {
  return import.meta.env.VITE_API_BASE_URL ?? ''
}

export const reportQueries = {
  /** Přehledy za rok (bez `year` = aktuální rok podle serveru). */
  overview: (slug: string, year?: number) =>
    queryOptions({
      queryKey: keys.reportOverview(slug, year),
      queryFn: ({ signal }) =>
        unwrap(
          api.GET('/api/accounts/{slug}/reports/overview', {
            params: { path: { slug }, query: year ? { year } : {} },
            signal,
          }),
        ),
    }),

  /** Podklady DPH za období `YYYY-MM` / `YYYY-Qn` (bez `period` = předchozí období dle nastavení účtu). */
  vat: (slug: string, period?: string) =>
    queryOptions({
      queryKey: keys.reportVat(slug, period),
      queryFn: ({ signal }) =>
        unwrap(
          api.GET('/api/accounts/{slug}/reports/vat', {
            params: { path: { slug }, query: period ? { period } : {} },
            signal,
          }),
        ),
    }),
}

/** URL XML pro EPO: přiznání (DPHDP3) nebo kontrolní hlášení (DPHKH1). */
export function vatXmlUrl(slug: string, form: 'dphdp3' | 'dphkh1', period: string): string {
  return `${base()}/api/accounts/${encodeURIComponent(slug)}/reports/vat/${form}.xml?period=${encodeURIComponent(period)}`
}
