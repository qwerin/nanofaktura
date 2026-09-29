import { queryOptions, useMutation, useQueryClient, type QueryClient } from '@tanstack/react-query'
import { api, unwrap } from '../client'
import type {
  CreateTemplateInput,
  Invoice,
  InvoiceTemplate,
  SaveAsTemplateInput,
  TemplateIssueInput,
  UpdateTemplateInput,
} from '../types'
import { keys } from './keys'

/** Šablon bývá málo — seznam se načítá najednou (výběr v pravidelné faktuře, přehled). */
export const TEMPLATES_PER_PAGE = 100

export interface TemplateFilters {
  query?: string
  subject_id?: number
}

export const templateQueries = {
  list: (slug: string, filters: TemplateFilters = {}) =>
    queryOptions({
      queryKey: keys.templateList(slug, filters),
      queryFn: ({ signal }) =>
        unwrap(
          api.GET('/api/accounts/{slug}/templates', {
            params: { path: { slug }, query: { ...filters, per_page: TEMPLATES_PER_PAGE } },
            signal,
          }),
        ),
    }),

  detail: (slug: string, id: number) =>
    queryOptions({
      queryKey: keys.templateDetail(slug, id),
      queryFn: ({ signal }) =>
        unwrap(api.GET('/api/accounts/{slug}/templates/{id}', { params: { path: { slug, id } }, signal })),
    }),
}

function afterChange(qc: QueryClient, slug: string, template?: InvoiceTemplate) {
  if (template) qc.setQueryData(keys.templateDetail(slug, template.id), template)
  void qc.invalidateQueries({ queryKey: [...keys.templates(slug), 'list'] })
}

/** Nová faktura: seznamy faktur a přehled zneplatnit, detail rovnou do cache. */
function afterInvoiceCreated(qc: QueryClient, slug: string, inv: Invoice) {
  qc.setQueryData(keys.invoiceDetail(slug, inv.id), inv)
  void qc.invalidateQueries({ queryKey: [...keys.invoices(slug), 'list'] })
  void qc.invalidateQueries({ queryKey: [...keys.account(slug), 'dashboard'] })
}

export function useCreateTemplate(slug: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body: CreateTemplateInput) =>
      unwrap(api.POST('/api/accounts/{slug}/templates', { params: { path: { slug } }, body })),
    onSuccess: (t) => afterChange(qc, slug, t),
    meta: { silent: true },
  })
}

export function useUpdateTemplate(slug: string, id: number) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body: UpdateTemplateInput) =>
      unwrap(api.PATCH('/api/accounts/{slug}/templates/{id}', { params: { path: { slug, id } }, body })),
    onSuccess: (t) => {
      afterChange(qc, slug, t)
      // Pravidelné faktury zobrazují částku a odběratele ze šablony.
      void qc.invalidateQueries({ queryKey: keys.recurring(slug) })
    },
    meta: { silent: true },
  })
}

export function useDeleteTemplate(slug: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: number) =>
      unwrap(api.DELETE('/api/accounts/{slug}/templates/{id}', { params: { path: { slug, id } } })),
    onSuccess: (_d, id) => {
      qc.removeQueries({ queryKey: keys.templateDetail(slug, id) })
      afterChange(qc, slug)
    },
  })
}

export function useCreateInvoiceFromTemplate(slug: string, id: number) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body: TemplateIssueInput = {}) =>
      unwrap(
        api.POST('/api/accounts/{slug}/templates/{id}/create-invoice', { params: { path: { slug, id } }, body }),
      ),
    onSuccess: (inv) => afterInvoiceCreated(qc, slug, inv),
  })
}

export function useSaveAsTemplate(slug: string, invoiceId: number) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body: SaveAsTemplateInput) =>
      unwrap(
        api.POST('/api/accounts/{slug}/invoices/{id}/save-as-template', {
          params: { path: { slug, id: invoiceId } },
          body,
        }),
      ),
    onSuccess: (t) => afterChange(qc, slug, t),
    meta: { silent: true },
  })
}
