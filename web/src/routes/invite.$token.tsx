import { useQuery } from '@tanstack/react-query'
import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { CircleAlertIcon, ClockIcon, LinkIcon, LogInIcon, MailCheckIcon, TriangleAlertIcon } from 'lucide-react'
import { useReducer, useState } from 'react'
import { isApiError } from '@/api/errors'
import { authQueries, useLogout } from '@/api/queries/auth'
import { invitationQueries, useAcceptInvitation } from '@/api/queries/invitations'
import type { InvitationInfo, Me } from '@/api/types'
import { AuthLayout } from '@/components/auth-layout'
import { ButtonLink } from '@/components/button-link'
import { EmptyState } from '@/components/empty-state'
import { FormSkeleton } from '@/components/skeletons'
import { InviteRegisterForm } from '@/components/team/invite-register-form'
import { roleDescriptions, teamErrorMessage } from '@/components/team/permissions'
import { RoleBadge } from '@/components/team/role-badge'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'
import { formatDateTime } from '@/lib/date'

// Veřejná stránka z odkazu v e-mailu s pozvánkou (SPEC §7.13).
export const Route = createFileRoute('/invite/$token')({
  head: () => ({ meta: [{ title: 'Pozvánka · NanoFaktura' }] }),
  component: InvitePage,
})

function InvitePage() {
  const { token } = Route.useParams()
  const info = useQuery(invitationQueries.info(token))
  const me = useQuery(authQueries.me())
  // Po odhlášení useLogout vyčistí cache (qc.clear) — observer se na nový záznam přepojí až při renderu.
  const [, rerender] = useReducer((x: number) => x + 1, 0)

  if (info.isPending || me.isPending) {
    return (
      <AuthLayout title="Pozvánka">
        <FormSkeleton fields={3} />
      </AuthLayout>
    )
  }

  if (info.isError) return <InvitationError error={info.error} loggedIn={Boolean(me.data)} retry={() => void info.refetch()} />

  return <InvitationView token={token} info={info.data} me={me.data ?? null} onLoggedOut={rerender} />
}

function InvitationError({ error, loggedIn, retry }: { error: unknown; loggedIn: boolean; retry: () => void }) {
  const status = isApiError(error) ? error.status : 0
  const home = loggedIn ? <ButtonLink to="/">Do aplikace</ButtonLink> : <ButtonLink to="/login">Přihlásit se</ButtonLink>
  if (status === 404) {
    return (
      <AuthLayout>
        <EmptyState
          icon={LinkIcon}
          title="Pozvánka nenalezena"
          description="Odkaz je neúplný, nebo pozvánku mezitím zrušili či poslali znovu. Požádejte o novou pozvánku."
          action={home}
        />
      </AuthLayout>
    )
  }
  if (status === 410) {
    return (
      <AuthLayout>
        <EmptyState
          icon={ClockIcon}
          title="Pozvánka už neplatí"
          description="Vypršela, nebo už byla přijata. Pokud jste ji přijali, stačí se přihlásit. Jinak požádejte o novou."
          action={home}
        />
      </AuthLayout>
    )
  }
  return (
    <AuthLayout>
      <EmptyState
        icon={CircleAlertIcon}
        title="Pozvánku se nepodařilo načíst"
        description={teamErrorMessage(error)}
        action={<Button onClick={retry}>Zkusit znovu</Button>}
      />
    </AuthLayout>
  )
}

function InvitationSummary({ info }: { info: InvitationInfo }) {
  return (
    <div className="flex flex-col gap-3 rounded-xl border bg-card p-4 text-sm">
      <div className="flex items-center justify-between gap-3">
        <span className="text-muted-foreground">Role</span>
        <RoleBadge role={info.role} />
      </div>
      <p className="text-muted-foreground">{roleDescriptions[info.role]}</p>
      <div className="flex items-center justify-between gap-3 border-t pt-3">
        <span className="text-muted-foreground">Pro e-mail</span>
        <span className="min-w-0 truncate font-medium">{info.email}</span>
      </div>
      <div className="flex items-center justify-between gap-3">
        <span className="text-muted-foreground">Platí do</span>
        <span>{formatDateTime(info.expires_at)}</span>
      </div>
    </div>
  )
}

function InvitationView({
  token,
  info,
  me,
  onLoggedOut,
}: {
  token: string
  info: InvitationInfo
  me: Me | null
  onLoggedOut: () => void
}) {
  const sameUser = me && me.user.email.toLowerCase() === info.email.toLowerCase()
  return (
    <AuthLayout
      title={
        <>
          Pozvánka do účtu <span className="text-primary">{info.account_name}</span>
        </>
      }
      description={
        me
          ? sameUser
            ? 'Po přijetí uvidíte účet v přepínači účtů vedle svých ostatních.'
            : undefined
          : info.user_exists
            ? 'S tímto e-mailem už v NanoFaktuře účet máte. Přihlaste se a pozvánku přijměte.'
            : 'Vytvořte si přihlášení — stačí jméno a heslo. Pak rovnou vstoupíte do účtu.'
      }
    >
      <InvitationSummary info={info} />
      {!me ? (
        info.user_exists ? (
          <ButtonLink
            to="/login"
            search={{ redirect: `/invite/${token}` }}
            size="lg"
            className="w-full"
          >
            <LogInIcon data-icon="inline-start" />
            Přihlásit se a přijmout
          </ButtonLink>
        ) : (
          <InviteRegisterForm token={token} email={info.email} />
        )
      ) : sameUser ? (
        <AcceptInvitation token={token} me={me} />
      ) : (
        <WrongUser me={me} info={info} onLoggedOut={onLoggedOut} />
      )}
    </AuthLayout>
  )
}

function AcceptInvitation({ token, me }: { token: string; me: Me }) {
  const accept = useAcceptInvitation(token)
  const navigate = useNavigate()
  const [error, setError] = useState<{ message: string; alreadyMember: boolean } | null>(null)

  const run = () => {
    setError(null)
    accept.mutate(undefined, {
      onSuccess: (account) =>
        void navigate({ to: '/a/$slug/dashboard', params: { slug: account.slug }, replace: true }),
      onError: (err) =>
        setError({ message: teamErrorMessage(err), alreadyMember: isApiError(err) && err.status === 409 }),
    })
  }

  return (
    <div className="flex flex-col gap-4">
      <p className="text-sm text-muted-foreground">
        Přihlášeni jako <strong className="text-foreground">{me.user.name || me.user.email}</strong>.
      </p>
      {error && (
        <Alert variant={error.alreadyMember ? 'default' : 'destructive'}>
          <TriangleAlertIcon />
          <AlertDescription>{error.message}</AlertDescription>
        </Alert>
      )}
      {error?.alreadyMember ? (
        <ButtonLink to="/" size="lg" className="w-full">
          Do aplikace
        </ButtonLink>
      ) : (
        <Button size="lg" className="w-full" onClick={run} disabled={accept.isPending}>
          {accept.isPending ? <Spinner data-icon="inline-start" /> : <MailCheckIcon data-icon="inline-start" />}
          Přijmout pozvánku
        </Button>
      )}
    </div>
  )
}

function WrongUser({ me, info, onLoggedOut }: { me: Me; info: InvitationInfo; onLoggedOut: () => void }) {
  const logout = useLogout()
  return (
    <div className="flex flex-col gap-4">
      <Alert>
        <TriangleAlertIcon />
        <AlertTitle>Jste přihlášeni pod jiným e-mailem</AlertTitle>
        <AlertDescription>
          Pozvánka patří adrese <strong className="text-foreground">{info.email}</strong>, vy jste přihlášeni jako{' '}
          <strong className="text-foreground">{me.user.email}</strong>. Odhlaste se a pokračujte jako pozvaný uživatel.
        </AlertDescription>
      </Alert>
      <Button size="lg" className="w-full" onClick={() => logout.mutate(undefined, { onSettled: onLoggedOut })} disabled={logout.isPending}>
        {logout.isPending && <Spinner data-icon="inline-start" />}
        Odhlásit se a pokračovat
      </Button>
      <ButtonLink to="/" variant="outline" size="lg" className="w-full">
        Zpět do aplikace
      </ButtonLink>
    </div>
  )
}
