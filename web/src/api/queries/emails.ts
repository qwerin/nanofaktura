import { queryOptions, useMutation, useQueryClient } from '@tanstack/react-query'
import { api, unwrap } from '../client'
import type { EmailKind, EmailLang, SendInvoiceInput } from '../types'
import { keys } from './keys'

export interface EmailPreviewParams {
  kind: EmailKind
  lang?: EmailLang
  /** Bez faktury = ukázková data. */
  invoice_id?: number
}

export const emailQueries = {
  /** Historie e-mailů faktury (nejnovější první). */
  invoiceEmails: (slug: string, invoiceId: number) =>
    queryOptions({
      queryKey: keys.invoiceEmails(slug, invoiceId),
      queryFn: ({ signal }) =>
        unwrap(
          api.GET('/api/accounts/{slug}/invoices/{id}/emails', {
            params: { path: { slug, id: invoiceId }, query: { per_page: 50 } },
            signal,
          }),
        ),
    }),

  /** Vyrenderovaný předmět a text podle uložené šablony účtu (+ výchozí příjemci faktury). */
  preview: (slug: string, params: EmailPreviewParams) =>
    queryOptions({
      queryKey: keys.emailPreview(slug, params),
      queryFn: ({ signal }) =>
        unwrap(
          api.GET('/api/accounts/{slug}/email-templates/preview', {
            params: { path: { slug }, query: params },
            signal,
          }),
        ),
      staleTime: 0,
    }),
}

export function useSendInvoice(slug: string, invoiceId: number) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body: SendInvoiceInput) =>
      unwrap(api.POST('/api/accounts/{slug}/invoices/{id}/send', { params: { path: { slug, id: invoiceId } }, body })),
    onSettled: () => {
      // I neúspěšný pokus se zapisuje do historie; faktura může přejít do „odeslaná“.
      void qc.invalidateQueries({ queryKey: keys.invoiceEmails(slug, invoiceId) })
      void qc.invalidateQueries({ queryKey: keys.invoiceDetail(slug, invoiceId) })
      void qc.invalidateQueries({ queryKey: [...keys.invoices(slug), 'list'] })
      void qc.invalidateQueries({ queryKey: [...keys.account(slug), 'dashboard'] })
    },
    meta: { silent: true },
  })
}
