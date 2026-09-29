import {
  infiniteQueryOptions,
  keepPreviousData,
  queryOptions,
  useMutation,
  useQueryClient,
  type InfiniteData,
  type QueryClient,
} from '@tanstack/react-query'
import { api, unwrap } from '../client'
import { ApiError, type Problem } from '../errors'
import type {
  BankImportResult,
  BankTransaction,
  BankTransactionFilters,
  BankTransactionList,
  MatchTransactionInput,
} from '../types'
import { keys } from './keys'

export const BANK_TRANSACTIONS_PER_PAGE = 30

export const bankQueries = {
  /** Pohyby na účtech („Načíst další“). Filtry = query parametry API. */
  transactions: (slug: string, filters: BankTransactionFilters) =>
    infiniteQueryOptions({
      queryKey: keys.bankTransactionList(slug, filters),
      queryFn: ({ pageParam, signal }) =>
        unwrap(
          api.GET('/api/accounts/{slug}/bank-transactions', {
            params: { path: { slug }, query: { ...filters, page: pageParam, per_page: BANK_TRANSACTIONS_PER_PAGE } },
            signal,
          }),
        ),
      initialPageParam: 1,
      getNextPageParam: (last) => (last.page * last.per_page < last.total ? last.page + 1 : undefined),
      placeholderData: keepPreviousData,
    }),

  /** Jen počet pohybů odpovídajících filtrům (záložky, karty účtů). */
  count: (slug: string, filters: BankTransactionFilters) =>
    queryOptions({
      queryKey: keys.bankTransactionCount(slug, filters),
      queryFn: async ({ signal }) =>
        (
          await unwrap(
            api.GET('/api/accounts/{slug}/bank-transactions', {
              params: { path: { slug }, query: { ...filters, page: 1, per_page: 1 } },
              signal,
            }),
          )
        ).total,
      placeholderData: keepPreviousData,
    }),

  /** Kandidáti pro ruční spárování — faktury (pod prefixem `invoices`, zneplatní se s nimi). */
  invoiceCandidates: (slug: string, query: string) =>
    queryOptions({
      queryKey: [...keys.invoiceList(slug, { query }), 'bank-match'],
      queryFn: async ({ signal }) =>
        (
          await unwrap(
            api.GET('/api/accounts/{slug}/invoices', {
              params: { path: { slug }, query: { ...(query ? { query } : {}), sort: '-issued_on', per_page: 50 } },
              signal,
            }),
          )
        ).items,
      placeholderData: keepPreviousData,
    }),

  /** Kandidáti pro ruční spárování — náklady. */
  expenseCandidates: (slug: string, query: string) =>
    queryOptions({
      queryKey: [...keys.expenseList(slug, { query }), 'bank-match'],
      queryFn: async ({ signal }) =>
        (
          await unwrap(
            api.GET('/api/accounts/{slug}/expenses', {
              params: { path: { slug }, query: { ...(query ? { query } : {}), sort: '-issued_on', per_page: 50 } },
              signal,
            }),
          )
        ).items,
      placeholderData: keepPreviousData,
    }),

  /** Stav kontaktu v registru plátců DPH (MF ČR). Načítá se líně, chyby řeší komponenta. */
  vatStatus: (slug: string, subjectId: number) =>
    queryOptions({
      queryKey: keys.subjectVatStatus(slug, subjectId),
      queryFn: ({ signal }) =>
        unwrap(api.GET('/api/accounts/{slug}/subjects/{id}/vat-status', { params: { path: { slug, id: subjectId } }, signal })),
      staleTime: 60 * 60_000,
      retry: false,
      meta: { silent: true },
    }),
}

/** 429 ze synchronizace nese `Retry-After` — `unwrap` hlavičky nevrací, proto vlastní chyba. */
export class RateLimitedError extends ApiError {
  readonly retryAfter: string | null
  constructor(problem: Problem, retryAfter: string | null) {
    super(429, problem)
    this.retryAfter = retryAfter
  }
}

type TransactionPages = InfiniteData<BankTransactionList, number>

/** Přepíše transakci ve všech načtených seznamech (bez refetch). */
function patchTransaction(qc: QueryClient, slug: string, id: number, patch: (t: BankTransaction) => BankTransaction) {
  qc.setQueriesData<TransactionPages>({ queryKey: [...keys.bankTransactions(slug), 'list'] }, (data) =>
    data
      ? {
          ...data,
          pages: data.pages.map((p) => ({ ...p, items: p.items.map((t) => (t.id === id ? patch(t) : t)) })),
        }
      : data,
  )
}

/**
 * Po změně transakce: seznamy jen označit za zastaralé (aktuální pohled si změněný řádek
 * ponechá — uživatel vidí výsledek a může akci vrátit), počty a doklady načíst znovu.
 */
function afterTransactionChange(qc: QueryClient, slug: string, touchesDocuments: boolean) {
  void qc.invalidateQueries({ queryKey: [...keys.bankTransactions(slug), 'list'], refetchType: 'none' })
  void qc.invalidateQueries({ queryKey: [...keys.bankTransactions(slug), 'count'] })
  if (touchesDocuments) {
    void qc.invalidateQueries({ queryKey: keys.invoices(slug) })
    void qc.invalidateQueries({ queryKey: keys.expenses(slug) })
    void qc.invalidateQueries({ queryKey: [...keys.account(slug), 'dashboard'] })
  }
}

/** Po importu/synchronizaci/rematch se mohl změnit kdejaký pohyb i doklad → vše znovu. */
function afterBulkChange(qc: QueryClient, slug: string) {
  void qc.invalidateQueries({ queryKey: keys.bankTransactions(slug) })
  void qc.invalidateQueries({ queryKey: keys.bankAccounts(slug) })
  void qc.invalidateQueries({ queryKey: keys.invoices(slug) })
  void qc.invalidateQueries({ queryKey: keys.expenses(slug) })
  void qc.invalidateQueries({ queryKey: [...keys.account(slug), 'dashboard'] })
}

export function useImportStatement(slug: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ bankAccountId, file, format }: { bankAccountId: number; file: File; format?: string }) =>
      unwrap(
        api.POST('/api/accounts/{slug}/bank-accounts/{id}/import', {
          params: { path: { slug, id: bankAccountId } },
          // Schéma popisuje soubor jako string (binary); skutečně posíláme File v FormData.
          body: { file: file as unknown as string, format },
          bodySerializer: () => {
            const fd = new FormData()
            fd.append('file', file, file.name)
            if (format) fd.append('format', format)
            return fd
          },
        }),
      ) as Promise<BankImportResult>,
    onSuccess: () => afterBulkChange(qc, slug),
    meta: { silent: true },
  })
}

export function useSyncBankAccount(slug: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationKey: [...keys.bankTransactions(slug), 'sync'],
    mutationFn: async (bankAccountId: number) => {
      const res = await api.POST('/api/accounts/{slug}/bank-accounts/{id}/sync', {
        params: { path: { slug, id: bankAccountId } },
      })
      if (res.response.status === 429) {
        const problem = (res.error && typeof res.error === 'object' ? res.error : { title: 'Too Many Requests' }) as Problem
        throw new RateLimitedError(problem, res.response.headers.get('Retry-After'))
      }
      return unwrap(Promise.resolve(res))
    },
    onSuccess: () => afterBulkChange(qc, slug),
    meta: { silent: true },
  })
}

export function useRematch(slug: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: () => unwrap(api.POST('/api/accounts/{slug}/bank-transactions/rematch', { params: { path: { slug } } })),
    onSuccess: () => afterBulkChange(qc, slug),
  })
}

export function useMatchTransaction(slug: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, body }: { id: number; body: MatchTransactionInput }) =>
      unwrap(api.POST('/api/accounts/{slug}/bank-transactions/{id}/match', { params: { path: { slug, id } }, body })),
    onSuccess: (t) => {
      patchTransaction(qc, slug, t.id, () => t)
      afterTransactionChange(qc, slug, true)
    },
    meta: { silent: true },
  })
}

export function useUnmatchTransaction(slug: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: number) =>
      unwrap(api.POST('/api/accounts/{slug}/bank-transactions/{id}/unmatch', { params: { path: { slug, id } } })),
    onSuccess: (t) => {
      patchTransaction(qc, slug, t.id, () => t)
      afterTransactionChange(qc, slug, true)
    },
    meta: { silent: true },
  })
}

/** Ignorovat / obnovit — optimisticky (bez vlivu na doklady), při chybě se vrátí. */
export function useSetIgnored(slug: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, ignored }: { id: number; ignored: boolean }) =>
      unwrap(
        ignored
          ? api.POST('/api/accounts/{slug}/bank-transactions/{id}/ignore', { params: { path: { slug, id } } })
          : api.POST('/api/accounts/{slug}/bank-transactions/{id}/unignore', { params: { path: { slug, id } } }),
      ),
    onMutate: async ({ id, ignored }) => {
      await qc.cancelQueries({ queryKey: [...keys.bankTransactions(slug), 'list'] })
      const snapshot = qc.getQueriesData<TransactionPages>({ queryKey: [...keys.bankTransactions(slug), 'list'] })
      patchTransaction(qc, slug, id, (t) => ({
        ...t,
        ignored,
        state: ignored ? 'ignored' : t.suggestions.length > 0 ? 'suggested' : 'unmatched',
      }))
      return { snapshot }
    },
    onError: (_err, _vars, ctx) => {
      for (const [key, data] of ctx?.snapshot ?? []) qc.setQueryData(key, data)
    },
    onSuccess: (t) => {
      patchTransaction(qc, slug, t.id, () => t)
      afterTransactionChange(qc, slug, false)
    },
    meta: { silent: true },
  })
}
