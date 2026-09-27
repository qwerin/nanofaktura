import { infiniteQueryOptions, queryOptions, useMutation, useQueryClient } from '@tanstack/react-query'
import { api, unwrap } from '../client'
import type { CreateSubjectInput, Subject, SubjectType, UpdateSubjectInput } from '../types'
import { keys } from './keys'

export const SUBJECTS_PER_PAGE = 50

export interface SubjectFilters {
  query?: string
  type?: SubjectType
}

export const subjectQueries = {
  /** Stránkovaný seznam („Načíst další“). `customer`/`supplier` zahrnují i typ `both`. */
  list: (slug: string, filters: SubjectFilters = {}) =>
    infiniteQueryOptions({
      queryKey: keys.subjectList(slug, filters),
      queryFn: ({ pageParam, signal }) =>
        unwrap(
          api.GET('/api/accounts/{slug}/subjects', {
            params: {
              path: { slug },
              query: {
                page: pageParam,
                per_page: SUBJECTS_PER_PAGE,
                query: filters.query || undefined,
                type: filters.type,
              },
            },
            signal,
          }),
        ),
      initialPageParam: 1,
      getNextPageParam: (last) => (last.page * last.per_page < last.total ? last.page + 1 : undefined),
    }),

  /** Prvních `perPage` kontaktů odpovídajících dotazu (název, IČO, e-mail). */
  search: (slug: string, query: string, perPage = 10) =>
    queryOptions({
      queryKey: keys.subjectSearch(slug, query, perPage),
      queryFn: async ({ signal }) =>
        (
          await unwrap(
            api.GET('/api/accounts/{slug}/subjects', {
              params: { path: { slug }, query: { query, per_page: perPage } },
              signal,
            }),
          )
        ).items ?? [],
    }),

  detail: (slug: string, id: number) =>
    queryOptions({
      queryKey: keys.subjectDetail(slug, id),
      queryFn: () => unwrap(api.GET('/api/accounts/{slug}/subjects/{id}', { params: { path: { slug, id } } })),
    }),

  /** Posledních 20 dokladů kontaktu (detail kontaktu). */
  invoices: (slug: string, subjectId: number) =>
    queryOptions({
      queryKey: keys.subjectInvoices(slug, subjectId),
      queryFn: () =>
        unwrap(
          api.GET('/api/accounts/{slug}/invoices', {
            params: { path: { slug }, query: { subject_id: subjectId, per_page: 20, sort: '-issued_on' } },
          }),
        ),
    }),
}

export function useCreateSubject(slug: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body: CreateSubjectInput) =>
      unwrap(api.POST('/api/accounts/{slug}/subjects', { params: { path: { slug } }, body })),
    onSuccess: (subject) => {
      qc.setQueryData(keys.subjectDetail(slug, subject.id), subject)
      void qc.invalidateQueries({ queryKey: [...keys.subjects(slug), 'list'] })
    },
    meta: { silent: true },
  })
}

export function useUpdateSubject(slug: string, id: number) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body: UpdateSubjectInput) =>
      unwrap(api.PATCH('/api/accounts/{slug}/subjects/{id}', { params: { path: { slug, id } }, body })),
    onSuccess: (subject) => {
      qc.setQueryData(keys.subjectDetail(slug, id), subject)
      void qc.invalidateQueries({ queryKey: [...keys.subjects(slug), 'list'] })
    },
    meta: { silent: true },
  })
}

/** Smazání kontaktu. 409 = kontakt má faktury (řeší volající, proto `silent`). */
export function useDeleteSubject(slug: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (subject: Pick<Subject, 'id'>) =>
      unwrap(api.DELETE('/api/accounts/{slug}/subjects/{id}', { params: { path: { slug, id: subject.id } } })),
    onSuccess: (_data, subject) => {
      qc.removeQueries({ queryKey: keys.subjectDetail(slug, subject.id) })
      void qc.invalidateQueries({ queryKey: [...keys.subjects(slug), 'list'] })
    },
    meta: { silent: true },
  })
}
