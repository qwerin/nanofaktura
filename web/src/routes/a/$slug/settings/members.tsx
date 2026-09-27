import { useQuery } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import { LockIcon, LogOutIcon, MailPlusIcon, ShieldIcon, UserMinusIcon } from 'lucide-react'
import { useMemo, useState } from 'react'
import { accountQueries } from '@/api/queries/accounts'
import { invitationQueries } from '@/api/queries/invitations'
import { memberQueries } from '@/api/queries/members'
import type { Member } from '@/api/types'
import { PageError } from '@/components/page-states'
import { SettingsPage } from '@/components/settings-page'
import { ListSkeleton } from '@/components/skeletons'
import { InvitationList } from '@/components/team/invitation-list'
import { InviteDialog } from '@/components/team/invite-dialog'
import { ChangeRoleDialog, RemoveMemberDialog } from '@/components/team/member-dialogs'
import {
  assignableRoles,
  canChangeRole,
  canRemove,
  isLastOwner,
  isManager,
  type TeamContext,
} from '@/components/team/permissions'
import { MemberAvatar, RoleBadge } from '@/components/team/role-badge'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { useCurrentAccount } from '@/hooks/use-current-account'
import { formatDate } from '@/lib/date'
import { canManageSettingsRole } from '@/lib/roles'

export const Route = createFileRoute('/a/$slug/settings/members')({
  loader: ({ context, params }) => {
    const role = context.me.accounts?.find((a) => a.slug === params.slug)?.role
    return Promise.all([
      context.queryClient.prefetchQuery(memberQueries.list(params.slug)),
      // Pozvánky smí číst jen správci (ostatním by API vrátilo 403).
      canManageSettingsRole(role) ? context.queryClient.prefetchQuery(invitationQueries.pending(params.slug)) : null,
    ])
  },
  head: () => ({ meta: [{ title: 'Tým · NanoFaktura' }] }),
  component: MembersPage,
})

function MembersPage() {
  const { slug } = Route.useParams()
  const { user, role, membership } = useCurrentAccount()
  const manager = isManager(role)
  const members = useQuery(memberQueries.list(slug))
  const invitations = useQuery({ ...invitationQueries.pending(slug), enabled: manager })
  const account = useQuery(accountQueries.detail(slug))
  const accountName = account.data?.name ?? membership?.name ?? ''

  const [inviteOpen, setInviteOpen] = useState(false)
  const [changing, setChanging] = useState<Member | null>(null)
  const [removing, setRemoving] = useState<Member | null>(null)

  const ctx: TeamContext = useMemo(
    () => ({ actor: role, ownerCount: members.data?.filter((m) => m.role === 'owner').length ?? 0 }),
    [role, members.data],
  )
  // Já nahoře, pak podle role a jména.
  const sorted = useMemo(() => {
    const order = { owner: 0, admin: 1, accountant: 2, member: 3 } as const
    return [...(members.data ?? [])].sort(
      (a, b) =>
        Number(b.user_id === user?.id) - Number(a.user_id === user?.id) ||
        order[a.role] - order[b.role] ||
        (a.name || a.email).localeCompare(b.name || b.email, 'cs'),
    )
  }, [members.data, user?.id])

  const roles = assignableRoles(role)
  const isSelf = (m: Member) => m.user_id === user?.id

  return (
    <SettingsPage
      title="Tým"
      description="Kdo má přístup k tomuto účtu a co v něm smí dělat. Každý člen se přihlašuje vlastním e-mailem a heslem."
      actions={manager ? [{ label: 'Pozvat', icon: MailPlusIcon, primary: true, onClick: () => setInviteOpen(true) }] : undefined}
    >
      {!manager && (
        <Alert className="mb-6 max-w-2xl">
          <LockIcon />
          <AlertDescription>Členy a jejich role spravuje vlastník nebo administrátor účtu.</AlertDescription>
        </Alert>
      )}

      <section className="flex max-w-3xl flex-col gap-3">
        <div className="flex items-end justify-between gap-3">
          <h2 className="text-base font-semibold tracking-tight">
            Členové{members.data ? <span className="ml-1.5 text-muted-foreground">{members.data.length}</span> : null}
          </h2>
          {manager && (
            <Button className="max-md:hidden" onClick={() => setInviteOpen(true)}>
              <MailPlusIcon data-icon="inline-start" />
              Pozvat člena
            </Button>
          )}
        </div>

        {members.isError ? (
          <PageError error={members.error} reset={() => void members.refetch()} />
        ) : members.isPending ? (
          <ListSkeleton rows={3} />
        ) : (
          <ul className="divide-y overflow-hidden rounded-xl border bg-card">
            {sorted.map((m) => {
              const self = isSelf(m)
              const target = { role: m.role, isSelf: self }
              const changeable = canChangeRole(target, ctx)
              const removable = canRemove(target, ctx)
              return (
                <li key={m.user_id} className="flex flex-col gap-3 p-4 md:flex-row md:items-center">
                  <div className="flex min-w-0 flex-1 items-start gap-3">
                    <MemberAvatar name={m.name || m.email} />
                    <div className="flex min-w-0 flex-col gap-1">
                      <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
                        <span className="truncate font-medium">{m.name || m.email}</span>
                        {self && (
                          <Badge variant="outline" className="text-muted-foreground">
                            vy
                          </Badge>
                        )}
                        <RoleBadge role={m.role} />
                      </div>
                      <span className="truncate text-sm text-muted-foreground">{m.email}</span>
                      <span className="text-xs text-muted-foreground">
                        Členem od {formatDate(m.joined_at.slice(0, 10))}
                        {isLastOwner(target, ctx) ? ' · jediný vlastník' : ''}
                      </span>
                    </div>
                  </div>
                  {(changeable || removable) && (
                    <div className="flex gap-2 max-md:pl-13 md:shrink-0">
                      {changeable && (
                        <Button variant="outline" className="max-md:flex-1" onClick={() => setChanging(m)}>
                          <ShieldIcon data-icon="inline-start" />
                          Změnit roli
                        </Button>
                      )}
                      {removable && (
                        <Button
                          variant="ghost"
                          className="text-destructive hover:text-destructive max-md:flex-1"
                          onClick={() => setRemoving(m)}
                        >
                          {self ? <LogOutIcon data-icon="inline-start" /> : <UserMinusIcon data-icon="inline-start" />}
                          {self ? 'Opustit účet' : 'Odebrat'}
                        </Button>
                      )}
                    </div>
                  )}
                </li>
              )
            })}
          </ul>
        )}
      </section>

      {manager && (
        <section className="mt-10 flex max-w-3xl flex-col gap-3">
          <div>
            <h2 className="text-base font-semibold tracking-tight">Čekající pozvánky</h2>
            <p className="mt-1 text-sm text-muted-foreground">
              Odkaz na přijetí je jen v e-mailu. Pokud se ztratil, pošlete pozvánku znovu — předchozí odkaz tím přestane platit.
            </p>
          </div>
          {invitations.isError ? (
            <PageError error={invitations.error} reset={() => void invitations.refetch()} />
          ) : invitations.isPending ? (
            <ListSkeleton rows={1} />
          ) : invitations.data.length === 0 ? (
            <div className="flex flex-col items-start gap-3 rounded-xl border border-dashed p-4 text-sm text-muted-foreground sm:flex-row sm:items-center sm:justify-between">
              <span>Žádné pozvánky nečekají na přijetí.</span>
              <Button variant="outline" className="max-sm:w-full" onClick={() => setInviteOpen(true)}>
                <MailPlusIcon data-icon="inline-start" />
                Pozvat člena
              </Button>
            </div>
          ) : (
            <InvitationList slug={slug} invitations={invitations.data} members={members.data ?? []} actor={role} />
          )}
        </section>
      )}

      {manager && (
        <InviteDialog
          slug={slug}
          open={inviteOpen}
          onOpenChange={setInviteOpen}
          roles={roles}
          memberEmails={(members.data ?? []).map((m) => m.email.toLowerCase())}
        />
      )}
      <ChangeRoleDialog
        slug={slug}
        member={changing}
        isSelf={changing ? isSelf(changing) : false}
        roles={roles}
        onClose={() => setChanging(null)}
      />
      <RemoveMemberDialog
        slug={slug}
        member={removing}
        isSelf={removing ? isSelf(removing) : false}
        accountName={accountName}
        onClose={() => setRemoving(null)}
      />
    </SettingsPage>
  )
}
