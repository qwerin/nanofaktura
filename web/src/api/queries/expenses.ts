import {
  infiniteQueryOptions,
  keepPreviousData,
  queryOptions,
  useMutation,
  useQueryClient,
} from '@tanstack/react-query'
import { api, unwrap } from '../client'
import type {
  CreateExpenseInput,
  CreateExpensePaymentInput,
  Expense,
  ExpenseListFilters,
  ResponseBody,
  UpdateExpenseInput,
} from '../types'
import { keys } from './keys'

export const EXPENSES_PER_PAGE = 50

export const expenseQueries = {
  /** Stránkovaný seznam („Načíst další“). Filtry odpovídají query parametrům API. */
  list: (slug: string, filters: ExpenseListFilters) =>
    infiniteQueryOptions({
      queryKey: keys.expenseList(slug, filters),
      queryFn: ({ pageParam, signal }) =>
        unwrap(
          api.GET('/api/accounts/{slug}/expenses', {
            params: { path: { slug }, query: { ...filters, page: pageParam, per_page: EXPENSES_PER_PAGE } },
            signal,
          }),
        ),
      initialPageParam: 1,
      getNextPageParam: (last) => (last.page * last.per_page < last.total ? last.page + 1 : undefined),
      placeholderData: keepPreviousData,
    }),

  detail: (slug: string, id: number) =>
    queryOptions({
      queryKey: keys.expenseDetail(slug, id),
      queryFn: () => unwrap(api.GET('/api/accounts/{slug}/expenses/{id}', { params: { path: { slug, id } } })),
    }),

  /** Existující kategorie (našeptávač). */
  categories: (slug: string, query: string) =>
    queryOptions({
      queryKey: keys.expenseCategories(slug, query),
      queryFn: async ({ signal }) =>
        (
          await unwrap(
            api.GET('/api/accounts/{slug}/expenses/categories', {
              params: { path: { slug }, query: query ? { query } : {} },
              signal,
            }),
          )
        ).items ?? [],
      staleTime: 60_000,
      placeholderData: keepPreviousData,
    }),

  /** Dodavatelé z adresáře kontaktů (typ supplier i both). */
  supplierSearch: (slug: string, query: string) =>
    queryOptions({
      queryKey: keys.expenseSupplierSearch(slug, query),
      queryFn: async ({ signal }) =>
        (
          await unwrap(
            api.GET('/api/accounts/{slug}/subjects', {
              params: { path: { slug }, query: { type: 'supplier', per_page: 20, ...(query ? { query } : {}) } },
              signal,
            }),
          )
        ).items ?? [],
      staleTime: 30_000,
      placeholderData: keepPreviousData,
    }),

  /** Jeden kontakt (název dodavatele pro filtr v URL). */
  supplier: (slug: string, id: number) =>
    queryOptions({
      queryKey: keys.expenseSupplier(slug, id),
      queryFn: () => unwrap(api.GET('/api/accounts/{slug}/subjects/{id}', { params: { path: { slug, id } } })),
      staleTime: 5 * 60_000,
    }),
}

/** Kontakt nabízený jako dodavatel (odvozeno z odpovědi GET /subjects). */
export type SupplierOption = ResponseBody<'/api/accounts/{slug}/subjects/{id}', 'get'>

/** Uloží čerstvý náklad do cache detailu a zneplatní seznamy. */
function useStoreExpense(slug: string) {
  const qc = useQueryClient()
  return (expense: Expense) => {
    qc.setQueryData(keys.expenseDetail(slug, expense.id), expense)
    void qc.invalidateQueries({ queryKey: [...keys.expenses(slug), 'list'] })
    void qc.invalidateQueries({ queryKey: [...keys.expenses(slug), 'categories'] })
    // Řádky s ceníkovou položkou mění sklad.
    void qc.invalidateQueries({ queryKey: keys.priceItems(slug) })
  }
}

export function useCreateExpense(slug: string) {
  const store = useStoreExpense(slug)
  return useMutation({
    mutationFn: (body: CreateExpenseInput) =>
      unwrap(api.POST('/api/accounts/{slug}/expenses', { params: { path: { slug } }, body })),
    onSuccess: store,
    meta: { silent: true },
  })
}

export function useUpdateExpense(slug: string, id: number) {
  const store = useStoreExpense(slug)
  return useMutation({
    mutationFn: (body: UpdateExpenseInput) =>
      unwrap(api.PATCH('/api/accounts/{slug}/expenses/{id}', { params: { path: { slug, id } }, body })),
    onSuccess: store,
    meta: { silent: true },
  })
}

export function useDeleteExpense(slug: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: number) =>
      unwrap(api.DELETE('/api/accounts/{slug}/expenses/{id}', { params: { path: { slug, id } } })),
    onSuccess: (_data, id) => {
      qc.removeQueries({ queryKey: keys.expenseDetail(slug, id) })
      void qc.invalidateQueries({ queryKey: [...keys.expenses(slug), 'list'] })
      void qc.invalidateQueries({ queryKey: keys.priceItems(slug) })
    },
  })
}

export function useExpenseAction(slug: string, id: number) {
  const store = useStoreExpense(slug)
  return useMutation({
    mutationFn: (action: 'lock' | 'unlock') =>
      unwrap(
        api.POST('/api/accounts/{slug}/expenses/{id}/actions/{action}', { params: { path: { slug, id, action } } }),
      ),
    onSuccess: store,
  })
}

export function useCreateExpensePayment(slug: string, id: number) {
  const store = useStoreExpense(slug)
  return useMutation({
    mutationFn: (body: CreateExpensePaymentInput) =>
      unwrap(api.POST('/api/accounts/{slug}/expenses/{id}/payments', { params: { path: { slug, id } }, body })),
    onSuccess: (result) => store(result.expense),
    meta: { silent: true },
  })
}

export function useDeleteExpensePayment(slug: string, id: number) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (paymentId: number) =>
      unwrap(
        api.DELETE('/api/accounts/{slug}/expenses/{id}/payments/{payment_id}', {
          params: { path: { slug, id, payment_id: paymentId } },
        }),
      ),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: keys.expenseDetail(slug, id) })
      void qc.invalidateQueries({ queryKey: [...keys.expenses(slug), 'list'] })
    },
  })
}
