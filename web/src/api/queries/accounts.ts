import { queryOptions, useMutation, useQueryClient } from '@tanstack/react-query'
import { api, unwrap } from '../client'
import type { CreateAccountInput, UpdateAccountInput } from '../types'
import { keys } from './keys'

export const accountQueries = {
  list: () =>
    queryOptions({
      queryKey: keys.accounts(),
      queryFn: async () => (await unwrap(api.GET('/api/accounts'))).items ?? [],
    }),

  detail: (slug: string) =>
    queryOptions({
      queryKey: keys.accountDetail(slug),
      queryFn: () => unwrap(api.GET('/api/accounts/{slug}', { params: { path: { slug } } })),
    }),
}

export function useCreateAccount() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body: CreateAccountInput) => unwrap(api.POST('/api/accounts', { body })),
    onSuccess: async (account) => {
      qc.setQueryData(keys.accountDetail(account.slug), account)
      // Přepínač účtů čte seznam z /api/auth/me.
      await Promise.all([
        qc.invalidateQueries({ queryKey: keys.me() }),
        qc.invalidateQueries({ queryKey: keys.accounts() }),
      ])
    },
    meta: { silent: true },
  })
}

export function useUpdateAccount(slug: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body: UpdateAccountInput) =>
      unwrap(api.PATCH('/api/accounts/{slug}', { params: { path: { slug } }, body })),
    onSuccess: (account) => {
      qc.setQueryData(keys.accountDetail(slug), account)
      void qc.invalidateQueries({ queryKey: keys.me() })
      void qc.invalidateQueries({ queryKey: keys.accounts() })
    },
    meta: { silent: true },
  })
}
