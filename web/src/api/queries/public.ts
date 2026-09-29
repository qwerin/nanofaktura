import { queryOptions } from '@tanstack/react-query'
import { api, unwrap } from '../client'
import { keys } from './keys'

function base(): string {
  return import.meta.env.VITE_API_BASE_URL ?? ''
}

export const publicQueries = {
  /** Faktura za veřejným odkazem `/p/$token` (bez přihlášení). */
  invoice: (token: string) =>
    queryOptions({
      queryKey: keys.publicInvoice(token),
      queryFn: ({ signal }) =>
        unwrap(api.GET('/api/public/invoices/{token}', { params: { path: { token } }, signal })),
      staleTime: 5 * 60_000,
      refetchOnWindowFocus: false,
      meta: { silent: true },
    }),
}

export function publicInvoiceFileUrl(token: string, kind: 'pdf' | 'isdoc'): string {
  return `${base()}/api/public/invoices/${encodeURIComponent(token)}/${kind}`
}
