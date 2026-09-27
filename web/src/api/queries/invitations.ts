import { queryOptions, useMutation, useQueryClient } from '@tanstack/react-query'
import { api, unwrap } from '../client'
import type { Invitation } from '../types'
import { keys } from './keys'

// Pozvánky do účtu (SPEC §7.13). Vytvoření pozvánky: useInviteMember v ./members.ts.

export const invitationQueries = {
  /** Čekající (nepřijaté, neexpirované) pozvánky účtu — jen pro vlastníka/administrátora. */
  pending: (slug: string) =>
    queryOptions({
      queryKey: keys.invitations(slug),
      queryFn: async () =>
        (
          await unwrap(
            api.GET('/api/accounts/{slug}/invitations', { params: { path: { slug }, query: { per_page: 200 } } }),
          )
        ).items ?? [],
    }),

  /** Veřejný detail pozvánky (bez přihlášení). 404 = neznámý token, 410 = vypršela / použitá. */
  info: (token: string) =>
    queryOptions({
      queryKey: keys.invitationInfo(token),
      queryFn: () => unwrap(api.GET('/api/invitations/{token}', { params: { path: { token } } })),
      retry: false,
      staleTime: 60_000,
      meta: { silent: true },
    }),
}

export function useRevokeInvitation(slug: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (invitation: Pick<Invitation, 'id'>) =>
      unwrap(
        api.DELETE('/api/accounts/{slug}/invitations/{id}', { params: { path: { slug, id: invitation.id } } }),
      ),
    onSuccess: (_, invitation) => {
      qc.setQueryData<Invitation[]>(keys.invitations(slug), (old) => old?.filter((i) => i.id !== invitation.id))
    },
    meta: { silent: true },
  })
}

/** Přijetí pozvánky přihlášeným uživatelem → členství v účtu (`MeAccount`). */
export function useAcceptInvitation(token: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: () => unwrap(api.POST('/api/invitations/{token}/accept', { params: { path: { token } } })),
    onSuccess: async () => {
      // Guard /a/$slug čte členství z /auth/me — musí být aktuální před navigací.
      await qc.refetchQueries({ queryKey: keys.me() })
      void qc.invalidateQueries({ queryKey: keys.accounts() })
      qc.removeQueries({ queryKey: keys.invitationInfo(token) })
    },
    meta: { silent: true },
  })
}
