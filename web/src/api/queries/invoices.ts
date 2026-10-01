import {
  infiniteQueryOptions,
  queryOptions,
  useMutation,
  useQueryClient,
  type QueryClient,
} from '@tanstack/react-query'
import { api, unwrap } from '../client'
import type {
  CreateInvoiceInput,
  CorrectionInput,
  CreatePaymentInput,
  FinalInvoiceInput,
  Invoice,
  InvoiceAction,
  InvoiceListQuery,
  UpdateInvoiceInput,
} from '../types'
import { keys } from './keys'

/** Filtry seznamu (bez stránkování) — odpovídají search params stránky /invoices. */
export type InvoiceFilters = Omit<InvoiceListQuery, 'page' | 'per_page'>

export const INVOICES_PER_PAGE = 25

export const invoiceQueries = {
  /** Stránkovaný seznam pro „Načíst další“. */
  list: (slug: string, filters: InvoiceFilters, perPage = INVOICES_PER_PAGE) =>
    infiniteQueryOptions({
      queryKey: [...keys.invoiceList(slug, filters), perPage],
      queryFn: ({ pageParam, signal }) =>
        unwrap(
          api.GET('/api/accounts/{slug}/invoices', {
            params: { path: { slug }, query: { ...filters, page: pageParam, per_page: perPage } },
            signal,
          }),
        ),
      initialPageParam: 1,
      getNextPageParam: (last) => (last.page * last.per_page < last.total ? last.page + 1 : undefined),
    }),

  /** Jedna stránka (dashboard: posledních N, po splatnosti). */
  top: (slug: string, filters: InvoiceFilters, perPage: number) =>
    queryOptions({
      queryKey: [...keys.invoiceList(slug, filters), 'top', perPage],
      queryFn: ({ signal }) =>
        unwrap(
          api.GET('/api/accounts/{slug}/invoices', {
            params: { path: { slug }, query: { ...filters, page: 1, per_page: perPage } },
            signal,
          }),
        ),
    }),

  detail: (slug: string, id: number) =>
    queryOptions({
      queryKey: keys.invoiceDetail(slug, id),
      queryFn: ({ signal }) =>
        unwrap(api.GET('/api/accounts/{slug}/invoices/{id}', { params: { path: { slug, id } }, signal })),
    }),
}

/** Po změně faktury: detail do cache, seznamy a přehled zneplatnit. */
function afterChange(qc: QueryClient, slug: string, invoice?: Invoice) {
  if (invoice) qc.setQueryData(keys.invoiceDetail(slug, invoice.id), invoice)
  void qc.invalidateQueries({ queryKey: [...keys.invoices(slug), 'list'] })
  void qc.invalidateQueries({ queryKey: [...keys.account(slug), 'dashboard'] })
}

export function useCreateInvoice(slug: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body: CreateInvoiceInput) =>
      unwrap(api.POST('/api/accounts/{slug}/invoices', { params: { path: { slug } }, body })),
    onSuccess: (inv) => afterChange(qc, slug, inv),
    meta: { silent: true },
  })
}

export function useUpdateInvoice(slug: string, id: number) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body: UpdateInvoiceInput) =>
      unwrap(api.PATCH('/api/accounts/{slug}/invoices/{id}', { params: { path: { slug, id } }, body })),
    onSuccess: (inv) => afterChange(qc, slug, inv),
    meta: { silent: true },
  })
}

export function useDeleteInvoice(slug: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: number) =>
      unwrap(api.DELETE('/api/accounts/{slug}/invoices/{id}', { params: { path: { slug, id } } })),
    onSuccess: (_data, id) => {
      qc.removeQueries({ queryKey: keys.invoiceDetail(slug, id) })
      afterChange(qc, slug)
    },
  })
}

export function useInvoiceAction(slug: string, id: number) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (action: InvoiceAction) =>
      unwrap(
        api.POST('/api/accounts/{slug}/invoices/{id}/actions/{action}', {
          params: { path: { slug, id, action } },
        }),
      ),
    onSuccess: (inv) => afterChange(qc, slug, inv),
  })
}

export function useDuplicateInvoice(slug: string, id: number) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: () =>
      unwrap(api.POST('/api/accounts/{slug}/invoices/{id}/duplicate', { params: { path: { slug, id } } })),
    onSuccess: (inv) => afterChange(qc, slug, inv),
  })
}

export function useCreateCorrection(slug: string, id: number) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body: CorrectionInput = {}) =>
      unwrap(api.POST('/api/accounts/{slug}/invoices/{id}/correction', { params: { path: { slug, id } }, body })),
    onSuccess: (inv) => {
      afterChange(qc, slug, inv)
      void qc.invalidateQueries({ queryKey: keys.invoiceDetail(slug, id) })
    },
    meta: { silent: true },
  })
}

/** Vyúčtování zálohy: nová konečná faktura k proformě (201). */
export function useCreateFinalInvoice(slug: string, id: number) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body: FinalInvoiceInput) =>
      unwrap(api.POST('/api/accounts/{slug}/invoices/{id}/final-invoice', { params: { path: { slug, id } }, body })),
    onSuccess: (inv) => {
      afterChange(qc, slug, inv)
      void qc.invalidateQueries({ queryKey: keys.invoiceDetail(slug, id) })
    },
    meta: { silent: true },
  })
}

export function useCreatePayment(slug: string, id: number) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body: CreatePaymentInput) =>
      unwrap(api.POST('/api/accounts/{slug}/invoices/{id}/payments', { params: { path: { slug, id } }, body })),
    onSuccess: (res) => {
      afterChange(qc, slug, res.invoice)
      // Platba zálohy může vytvořit daňový doklad / vyúčtování — jejich detaily se načtou znovu.
      void qc.invalidateQueries({ queryKey: keys.invoices(slug) })
    },
    meta: { silent: true },
  })
}

export function useDeletePayment(slug: string, id: number) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (paymentId: number) =>
      unwrap(
        api.DELETE('/api/accounts/{slug}/invoices/{id}/payments/{payment_id}', {
          params: { path: { slug, id, payment_id: paymentId } },
        }),
      ),
    onSuccess: () => {
      // I související doklady (daňový doklad k platbě, vyúčtování) se mění.
      void qc.invalidateQueries({ queryKey: keys.invoices(slug) })
      afterChange(qc, slug)
    },
  })
}

/** URL PDF dokladu (endpoint mimo OpenAPI klienta — otevírá se přímo v prohlížeči). */
export function invoicePdfUrl(slug: string, id: number): string {
  const base = import.meta.env.VITE_API_BASE_URL ?? ''
  return `${base}/api/accounts/${encodeURIComponent(slug)}/invoices/${id}/pdf`
}
