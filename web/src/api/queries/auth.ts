import { queryOptions, useMutation, useQueryClient } from '@tanstack/react-query'
import { api, unwrap } from '../client'
import { isApiError } from '../errors'
import type { CreateTokenInput, LoginInput, Me, RegisterInput, UpdateMeInput } from '../types'
import { keys } from './keys'

// --- Queries (queryOptions → použitelné v komponentách i v route loaderech) ---

export const authQueries = {
  status: () =>
    queryOptions({
      queryKey: keys.authStatus(),
      queryFn: () => unwrap(api.GET('/api/auth/status')),
      staleTime: 5 * 60_000,
    }),

  /** Přihlášený uživatel, nebo `null` když session neexistuje (401). */
  me: () =>
    queryOptions({
      queryKey: keys.me(),
      queryFn: async (): Promise<Me | null> => {
        try {
          return await unwrap(api.GET('/api/auth/me'))
        } catch (err) {
          if (isApiError(err) && err.status === 401) return null
          throw err
        }
      },
      staleTime: 5 * 60_000,
      meta: { silent: true },
    }),

  tokens: () =>
    queryOptions({
      queryKey: keys.tokens(),
      queryFn: async () => (await unwrap(api.GET('/api/auth/tokens'))).items ?? [],
    }),
}

// --- Mutations ---

export function useLogin() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body: LoginInput) => unwrap(api.POST('/api/auth/login', { body })),
    onSuccess: (me) => qc.setQueryData(keys.me(), me),
    meta: { silent: true },
  })
}

export function useRegister() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body: RegisterInput) => unwrap(api.POST('/api/auth/register', { body })),
    onSuccess: (me) => {
      qc.setQueryData(keys.me(), me)
      void qc.invalidateQueries({ queryKey: keys.authStatus() })
    },
    meta: { silent: true },
  })
}

export function useLogout() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: () => unwrap(api.POST('/api/auth/logout')),
    onSettled: () => {
      // I při chybě zahodíme lokální stav — uživatel chtěl pryč.
      qc.clear()
      qc.setQueryData(keys.me(), null)
    },
  })
}

export function useUpdateMe() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body: UpdateMeInput) => unwrap(api.PATCH('/api/auth/me', { body })),
    onSuccess: (me) => qc.setQueryData(keys.me(), me),
    meta: { silent: true },
  })
}

export function useCreateToken() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body: CreateTokenInput) => unwrap(api.POST('/api/auth/tokens', { body })),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.tokens() }),
    meta: { silent: true },
  })
}

export function useRevokeToken() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: number) =>
      unwrap(api.DELETE('/api/auth/tokens/{id}', { params: { path: { id } } })),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.tokens() }),
  })
}
