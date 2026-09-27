import { queryOptions, useMutation, useQueryClient } from '@tanstack/react-query'
import { api, unwrap } from '../client'
import type { CreateInvitationInput, Member, MemberRole } from '../types'
import { keys } from './keys'

// Členové účtu (SPEC §7.13). Pozvánky viz ./invitations.ts.

export const memberQueries = {
  /** Všichni členové účtu (řazeno podle e-mailu). Číst smí každý člen. */
  list: (slug: string) =>
    queryOptions({
      queryKey: keys.members(slug),
      queryFn: async () =>
        (
          await unwrap(
            api.GET('/api/accounts/{slug}/members', { params: { path: { slug }, query: { per_page: 200 } } }),
          )
        ).items ?? [],
    }),
}

export function useUpdateMemberRole(slug: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ member, role }: { member: Pick<Member, 'user_id'>; role: MemberRole }) =>
      unwrap(
        api.PATCH('/api/accounts/{slug}/members/{user_id}', {
          params: { path: { slug, user_id: member.user_id } },
          body: { role },
        }),
      ),
    onSuccess: (updated) => {
      qc.setQueryData<Member[]>(keys.members(slug), (old) =>
        old?.map((m) => (m.user_id === updated.user_id ? updated : m)),
      )
      // Změna vlastní role mění oprávnění (useCurrentAccount čte roli z /auth/me).
      void qc.invalidateQueries({ queryKey: keys.me() })
    },
    meta: { silent: true },
  })
}

/** Odebrání člena, nebo opuštění účtu (když `user_id` je přihlášený uživatel). */
export function useRemoveMember(slug: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (member: Pick<Member, 'user_id'>) =>
      unwrap(
        api.DELETE('/api/accounts/{slug}/members/{user_id}', { params: { path: { slug, user_id: member.user_id } } }),
      ),
    onSuccess: (_, member) => {
      qc.setQueryData<Member[]>(keys.members(slug), (old) => old?.filter((m) => m.user_id !== member.user_id))
    },
    meta: { silent: true },
  })
}

export function useInviteMember(slug: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationKey: [...keys.invitations(slug), 'invite'],
    mutationFn: (body: CreateInvitationInput) =>
      unwrap(api.POST('/api/accounts/{slug}/members/invite', { params: { path: { slug } }, body })),
    // Nová pozvánka nahrazuje předchozí na stejný e-mail → celý seznam znovu.
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.invitations(slug) }),
    meta: { silent: true },
  })
}
