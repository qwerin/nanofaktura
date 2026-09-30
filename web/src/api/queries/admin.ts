import { keepPreviousData, queryOptions, useMutation, useQueryClient } from '@tanstack/react-query'
import { api, unwrap } from '../client'
import type { EmailTestInput } from '../types'
import { keys } from './keys'

// Správa instance (/api/admin/*) — jen pro správce instance (me.instance_admin).

export const adminQueries = {
  status: () =>
    queryOptions({
      queryKey: keys.adminStatus(),
      queryFn: () => unwrap(api.GET('/api/admin/status')),
    }),

  users: (query: string) =>
    queryOptions({
      queryKey: keys.adminUsers(query),
      queryFn: () =>
        unwrap(api.GET('/api/admin/users', { params: { query: { query: query || undefined, per_page: 200 } } })),
      placeholderData: keepPreviousData,
    }),
}

/** Diagnostika SMTP + DNS a odeslání testovacího e-mailu. */
export function useEmailTest() {
  return useMutation({
    mutationFn: (body: EmailTestInput) => unwrap(api.POST('/api/admin/email-test', { body })),
  })
}

export type AdminUserAction = 'reset-2fa' | 'verify-email' | 'send-verification'

export function useAdminUserAction() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, action }: { id: number; action: AdminUserAction }) => {
      const params = { params: { path: { id } } }
      switch (action) {
        case 'reset-2fa':
          return unwrap(api.POST('/api/admin/users/{id}/reset-2fa', params))
        case 'verify-email':
          return unwrap(api.POST('/api/admin/users/{id}/verify-email', params))
        case 'send-verification':
          return unwrap(api.POST('/api/admin/users/{id}/send-verification', params))
      }
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: keys.admin })
      void qc.invalidateQueries({ queryKey: keys.me() })
    },
  })
}
