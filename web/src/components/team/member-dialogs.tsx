import { useQueryClient } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { TriangleAlertIcon } from 'lucide-react'
import { useId, useState } from 'react'
import { toast } from 'sonner'
import { authQueries } from '@/api/queries/auth'
import { keys } from '@/api/queries/keys'
import { useRemoveMember, useUpdateMemberRole } from '@/api/queries/members'
import type { Member, MemberRole } from '@/api/types'
import { ResponsiveDialog } from '@/components/responsive-dialog'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'
import { roleLabels } from '@/lib/roles'
import { isManager, teamErrorMessage } from './permissions'
import { RolePicker } from './role-picker'

export function ChangeRoleDialog({
  slug,
  member,
  isSelf,
  roles,
  onClose,
}: {
  slug: string
  member: Member | null
  isSelf: boolean
  roles: MemberRole[]
  onClose: () => void
}) {
  return (
    <ResponsiveDialog
      open={member !== null}
      onOpenChange={(o) => !o && onClose()}
      title={isSelf ? 'Změnit svou roli' : 'Změnit roli'}
      description={member ? `${member.name || member.email} · ${member.email}` : undefined}
      className="sm:max-w-lg"
    >
      {member && <ChangeRoleForm key={member.user_id} slug={slug} member={member} isSelf={isSelf} roles={roles} onClose={onClose} />}
    </ResponsiveDialog>
  )
}

function ChangeRoleForm({
  slug,
  member,
  isSelf,
  roles,
  onClose,
}: {
  slug: string
  member: Member
  isSelf: boolean
  roles: MemberRole[]
  onClose: () => void
}) {
  const update = useUpdateMemberRole(slug)
  const [role, setRole] = useState<MemberRole>(member.role)
  const [error, setError] = useState<string | null>(null)
  const labelId = useId()
  // Vlastní role, kterou nesmím přidělit (admin → owner), zůstává ve výběru jako aktuální stav.
  const options = roles.includes(member.role) ? roles : [member.role, ...roles]
  const losesManagement = isSelf && isManager(member.role) && !isManager(role)

  const save = () => {
    if (role === member.role) return onClose()
    setError(null)
    update.mutate(
      { member, role },
      {
        onSuccess: () => {
          toast.success(isSelf ? `Vaše role je nyní ${roleLabels[role]}` : `${member.name || member.email}: ${roleLabels[role]}`)
          onClose()
        },
        onError: (err) => setError(teamErrorMessage(err)),
      },
    )
  }

  return (
    <div className="flex flex-col gap-4">
      <span id={labelId} className="sr-only">
        Role
      </span>
      <RolePicker name="member-role" aria-labelledby={labelId} roles={options} value={role} onChange={setRole} />
      {losesManagement && (
        <Alert>
          <TriangleAlertIcon />
          <AlertDescription>Přijdete o správu týmu a nastavení. Vrátit roli vám pak musí jiný správce.</AlertDescription>
        </Alert>
      )}
      {error && (
        <p role="alert" className="text-sm text-destructive">
          {error}
        </p>
      )}
      <div className="flex flex-col gap-2 sm:flex-row-reverse">
        <Button onClick={save} disabled={update.isPending}>
          {update.isPending && <Spinner data-icon="inline-start" />}
          Uložit roli
        </Button>
        <Button variant="outline" onClick={onClose}>
          Zrušit
        </Button>
      </div>
    </div>
  )
}

export function RemoveMemberDialog({
  slug,
  member,
  isSelf,
  accountName,
  onClose,
}: {
  slug: string
  member: Member | null
  isSelf: boolean
  accountName: string
  onClose: () => void
}) {
  const remove = useRemoveMember(slug)
  const qc = useQueryClient()
  const navigate = useNavigate()
  const [error, setError] = useState<string | null>(null)

  const confirm = () => {
    if (!member) return
    setError(null)
    remove.mutate(member, {
      onSuccess: async () => {
        if (!isSelf) {
          toast.success(`${member.name || member.email} už nemá přístup k účtu`)
          onClose()
          return
        }
        toast.success(`Opustili jste účet ${accountName}`)
        // Nejdřív pryč ze stránky účtu (jinak by se jeho data zkoušela načíst a skončila 404),
        // pak teprve obnovit /auth/me a zahodit data opuštěného účtu.
        const next = qc.getQueryData(authQueries.me().queryKey)?.accounts?.find((a) => a.slug !== slug)
        await qc.cancelQueries({ queryKey: keys.account(slug) })
        if (next) await navigate({ to: '/a/$slug/dashboard', params: { slug: next.slug }, replace: true })
        else {
          // Bez dalšího účtu: `/` podle čerstvého /auth/me nabídne založení účtu.
          qc.setQueryData(authQueries.me().queryKey, (me) => (me ? { ...me, accounts: [] } : me))
          await navigate({ to: '/', replace: true })
        }
        qc.removeQueries({ queryKey: keys.account(slug) })
        void qc.invalidateQueries({ queryKey: keys.me() })
        void qc.invalidateQueries({ queryKey: keys.accounts() })
      },
      onError: (err) => setError(teamErrorMessage(err)),
    })
  }

  return (
    <ResponsiveDialog
      open={member !== null}
      onOpenChange={(o) => {
        if (!o) {
          setError(null)
          onClose()
        }
      }}
      title={isSelf ? 'Opustit účet?' : 'Odebrat z účtu?'}
      description={
        member &&
        (isSelf ? (
          <>
            Ztratíte přístup k účtu <strong className="text-foreground">{accountName}</strong> a jeho dokladům. Zpět se
            dostanete jen novou pozvánkou.
          </>
        ) : (
          <>
            <strong className="text-foreground">{member.name || member.email}</strong> ztratí přístup k účtu{' '}
            <strong className="text-foreground">{accountName}</strong>. Jeho uživatelský účet i doklady, které vystavil,
            zůstanou zachované.
          </>
        ))
      }
      footer={
        <>
          <Button variant="destructive" disabled={remove.isPending} onClick={confirm}>
            {remove.isPending && <Spinner data-icon="inline-start" />}
            {isSelf ? 'Opustit účet' : 'Odebrat'}
          </Button>
          <Button variant="outline" onClick={onClose}>
            Zrušit
          </Button>
        </>
      }
    >
      {error && (
        <p role="alert" className="text-sm text-destructive">
          {error}
        </p>
      )}
    </ResponsiveDialog>
  )
}
