import { CopyIcon, MailIcon, RotateCwIcon, XIcon } from 'lucide-react'
import { useState } from 'react'
import { toast } from 'sonner'
import { useResendInvitation, useRevokeInvitation } from '@/api/queries/invitations'
import type { Invitation, MemberRole } from '@/api/types'
import { ResponsiveDialog } from '@/components/responsive-dialog'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'
import { formatDateTime } from '@/lib/date'
import { canManageInvitation, teamErrorMessage } from './permissions'
import { RoleBadge } from './role-badge'

/** Čekající pozvánky: kopírování odkazu, „Poslat znovu“ (stejný odkaz, delší platnost), zrušení. */
export function InvitationList({
  slug,
  invitations,
  actor,
}: {
  slug: string
  invitations: Invitation[]
  actor: MemberRole | undefined
}) {
  const resend = useResendInvitation(slug)
  const [revoking, setRevoking] = useState<Invitation | null>(null)

  const doResend = (inv: Invitation) =>
    resend.mutate(inv, {
      onSuccess: () => toast.success(`Pozvánka znovu odeslána na ${inv.email}`),
      onError: (err) => toast.error(teamErrorMessage(err)),
    })
  const copyLink = (inv: Invitation) =>
    void navigator.clipboard.writeText(inv.invite_url).then(
      () => toast.success('Odkaz pozvánky zkopírován'),
      () => toast.error('Odkaz se nepodařilo zkopírovat'),
    )

  return (
    <>
      <ul className="divide-y overflow-hidden rounded-xl border bg-card">
        {invitations.map((inv) => {
          const manageable = canManageInvitation(inv.role, actor)
          const resending = resend.isPending && resend.variables?.id === inv.id
          const inviter = inv.invited_by_name
          return (
            <li key={inv.id} className="flex flex-col gap-3 p-4 md:flex-row md:items-center">
              <div className="flex min-w-0 flex-1 items-start gap-3">
                <span className="flex size-10 shrink-0 items-center justify-center rounded-full border border-dashed text-muted-foreground">
                  <MailIcon className="size-4" />
                </span>
                <div className="flex min-w-0 flex-col gap-1">
                  <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
                    <span className="truncate font-medium">{inv.email}</span>
                    <RoleBadge role={inv.role} />
                  </div>
                  <span className="text-xs text-muted-foreground">
                    {inviter ? `Pozval(a) ${inviter} · ` : ''}platí do {formatDateTime(inv.expires_at)}
                  </span>
                </div>
              </div>
              {manageable && (
                <div className="flex flex-wrap gap-2 max-md:pl-13 md:shrink-0">
                  {inv.invite_url && (
                    <Button variant="outline" className="max-md:flex-1" onClick={() => copyLink(inv)}>
                      <CopyIcon data-icon="inline-start" />
                      Kopírovat odkaz
                    </Button>
                  )}
                  <Button variant="outline" className="max-md:flex-1" disabled={resending} onClick={() => doResend(inv)}>
                    {resending ? <Spinner data-icon="inline-start" /> : <RotateCwIcon data-icon="inline-start" />}
                    Poslat znovu
                  </Button>
                  <Button
                    variant="ghost"
                    className="text-destructive hover:text-destructive max-md:flex-1"
                    onClick={() => setRevoking(inv)}
                  >
                    <XIcon data-icon="inline-start" />
                    Zrušit
                  </Button>
                </div>
              )}
            </li>
          )
        })}
      </ul>
      <RevokeInvitationDialog slug={slug} invitation={revoking} onClose={() => setRevoking(null)} />
    </>
  )
}

function RevokeInvitationDialog({
  slug,
  invitation,
  onClose,
}: {
  slug: string
  invitation: Invitation | null
  onClose: () => void
}) {
  const revoke = useRevokeInvitation(slug)
  return (
    <ResponsiveDialog
      open={invitation !== null}
      onOpenChange={(o) => !o && onClose()}
      title="Zrušit pozvánku?"
      description={
        invitation && (
          <>
            Odkaz v e-mailu pro <strong className="text-foreground">{invitation.email}</strong> přestane platit.
          </>
        )
      }
      footer={
        <>
          <Button
            variant="destructive"
            disabled={revoke.isPending}
            onClick={() =>
              invitation &&
              revoke.mutate(invitation, {
                onSuccess: () => {
                  toast.success('Pozvánka zrušena')
                  onClose()
                },
                onError: (err) => toast.error(teamErrorMessage(err)),
              })
            }
          >
            {revoke.isPending && <Spinner data-icon="inline-start" />}
            Zrušit pozvánku
          </Button>
          <Button variant="outline" onClick={onClose}>
            Ponechat
          </Button>
        </>
      }
    />
  )
}
