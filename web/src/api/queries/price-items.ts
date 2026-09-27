import {
  infiniteQueryOptions,
  keepPreviousData,
  queryOptions,
  useMutation,
  useQueryClient,
} from '@tanstack/react-query'
import { api, unwrap } from '../client'
import type {
  CreatePriceItemInput,
  CreateStockMoveInput,
  PriceItem,
  PriceItemListFilters,
  UpdatePriceItemInput,
} from '../types'
import { keys } from './keys'

export const PRICE_ITEMS_PER_PAGE = 50
export const STOCK_MOVES_PER_PAGE = 30

export const priceItemQueries = {
  list: (slug: string, filters: PriceItemListFilters) =>
    infiniteQueryOptions({
      queryKey: keys.priceItemList(slug, filters),
      queryFn: ({ pageParam, signal }) =>
        unwrap(
          api.GET('/api/accounts/{slug}/price-items', {
            params: { path: { slug }, query: { ...filters, page: pageParam, per_page: PRICE_ITEMS_PER_PAGE } },
            signal,
          }),
        ),
      initialPageParam: 1,
      getNextPageParam: (last) => (last.page * last.per_page < last.total ? last.page + 1 : undefined),
      placeholderData: keepPreviousData,
    }),

  /** Našeptávač položek (výběr do řádku dokladu) — jen aktivní, prvních 20. */
  search: (slug: string, query: string) =>
    queryOptions({
      queryKey: keys.priceItemList(slug, { search: query }),
      queryFn: async ({ signal }) =>
        (
          await unwrap(
            api.GET('/api/accounts/{slug}/price-items', {
              params: { path: { slug }, query: { per_page: 20, ...(query ? { query } : {}) } },
              signal,
            }),
          )
        ).items ?? [],
      placeholderData: keepPreviousData,
    }),

  detail: (slug: string, id: number) =>
    queryOptions({
      queryKey: keys.priceItemDetail(slug, id),
      queryFn: () => unwrap(api.GET('/api/accounts/{slug}/price-items/{id}', { params: { path: { slug, id } } })),
    }),

  stockMoves: (slug: string, id: number) =>
    infiniteQueryOptions({
      queryKey: keys.stockMoves(slug, id),
      queryFn: ({ pageParam, signal }) =>
        unwrap(
          api.GET('/api/accounts/{slug}/price-items/{id}/stock-moves', {
            params: { path: { slug, id }, query: { page: pageParam, per_page: STOCK_MOVES_PER_PAGE } },
            signal,
          }),
        ),
      initialPageParam: 1,
      getNextPageParam: (last) => (last.page * last.per_page < last.total ? last.page + 1 : undefined),
    }),
}

function useStorePriceItem(slug: string) {
  const qc = useQueryClient()
  return (item: PriceItem) => {
    qc.setQueryData(keys.priceItemDetail(slug, item.id), item)
    void qc.invalidateQueries({ queryKey: [...keys.priceItems(slug), 'list'] })
  }
}

export function useCreatePriceItem(slug: string) {
  const store = useStorePriceItem(slug)
  return useMutation({
    mutationFn: (body: CreatePriceItemInput) =>
      unwrap(api.POST('/api/accounts/{slug}/price-items', { params: { path: { slug } }, body })),
    onSuccess: store,
    meta: { silent: true },
  })
}

export function useUpdatePriceItem(slug: string, id: number) {
  const store = useStorePriceItem(slug)
  return useMutation({
    mutationFn: (body: UpdatePriceItemInput) =>
      unwrap(api.PATCH('/api/accounts/{slug}/price-items/{id}', { params: { path: { slug, id } }, body })),
    onSuccess: store,
    meta: { silent: true },
  })
}

/** Archivace / obnovení (PATCH `archived`) — bez vlastního zpracování chyb, toastuje se globálně. */
export function useArchivePriceItem(slug: string, id: number) {
  const store = useStorePriceItem(slug)
  return useMutation({
    mutationFn: (archived: boolean) =>
      unwrap(api.PATCH('/api/accounts/{slug}/price-items/{id}', { params: { path: { slug, id } }, body: { archived } })),
    onSuccess: store,
  })
}

export function useDeletePriceItem(slug: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: number) =>
      unwrap(api.DELETE('/api/accounts/{slug}/price-items/{id}', { params: { path: { slug, id } } })),
    onSuccess: (_data, id) => {
      qc.removeQueries({ queryKey: keys.priceItemDetail(slug, id) })
      qc.removeQueries({ queryKey: keys.stockMoves(slug, id) })
      void qc.invalidateQueries({ queryKey: [...keys.priceItems(slug), 'list'] })
    },
  })
}

export function useCreateStockMove(slug: string, id: number) {
  const qc = useQueryClient()
  const store = useStorePriceItem(slug)
  return useMutation({
    mutationFn: (body: CreateStockMoveInput) =>
      unwrap(
        api.POST('/api/accounts/{slug}/price-items/{id}/stock-moves', { params: { path: { slug, id } }, body }),
      ),
    onSuccess: (result) => {
      store(result.price_item)
      void qc.invalidateQueries({ queryKey: keys.stockMoves(slug, id) })
    },
    meta: { silent: true },
  })
}

export function useDeleteStockMove(slug: string, id: number) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (moveId: number) =>
      unwrap(
        api.DELETE('/api/accounts/{slug}/price-items/{id}/stock-moves/{move_id}', {
          params: { path: { slug, id, move_id: moveId } },
        }),
      ),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: keys.priceItemDetail(slug, id) })
      void qc.invalidateQueries({ queryKey: keys.stockMoves(slug, id) })
      void qc.invalidateQueries({ queryKey: [...keys.priceItems(slug), 'list'] })
    },
  })
}
