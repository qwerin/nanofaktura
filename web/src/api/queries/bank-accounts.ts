import { queryOptions, useMutation, useQueryClient } from '@tanstack/react-query'
import { api, unwrap } from '../client'
import type { BankAccount, CreateBankAccountInput, UpdateBankAccountInput } from '../types'
import { keys } from './keys'

export const bankAccountQueries = {
  /** Všechny bankovní účty firmy (řazené: měna, výchozí první, název). */
  list: (slug: string) =>
    queryOptions({
      queryKey: keys.bankAccounts(slug),
      queryFn: async () =>
        (
          await unwrap(
            api.GET('/api/accounts/{slug}/bank-accounts', { params: { path: { slug }, query: { per_page: 200 } } }),
          )
        ).items ?? [],
    }),
}

export function useCreateBankAccount(slug: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationKey: [...keys.bankAccounts(slug), 'save'],
    mutationFn: (body: CreateBankAccountInput) =>
      unwrap(api.POST('/api/accounts/{slug}/bank-accounts', { params: { path: { slug } }, body })),
    // Změna výchozího účtu mění i ostatní záznamy → celý seznam znovu.
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.bankAccounts(slug) }),
    meta: { silent: true },
  })
}

export function useUpdateBankAccount(slug: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationKey: [...keys.bankAccounts(slug), 'save'],
    mutationFn: ({ id, body }: { id: number; body: UpdateBankAccountInput }) =>
      unwrap(api.PATCH('/api/accounts/{slug}/bank-accounts/{id}', { params: { path: { slug, id } }, body })),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.bankAccounts(slug) }),
    meta: { silent: true },
  })
}

export function useDeleteBankAccount(slug: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (account: Pick<BankAccount, 'id'>) =>
      unwrap(api.DELETE('/api/accounts/{slug}/bank-accounts/{id}', { params: { path: { slug, id: account.id } } })),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.bankAccounts(slug) }),
  })
}
