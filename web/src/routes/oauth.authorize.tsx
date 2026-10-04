import { useQuery } from '@tanstack/react-query'
import { createFileRoute, redirect, useLocation, useNavigate } from '@tanstack/react-router'
import { ArrowUpRightIcon, BanIcon, CheckIcon, CircleAlertIcon, PlugIcon, TriangleAlertIcon } from 'lucide-react'
import { useState } from 'react'
import { z } from 'zod'
import { errorMessage, isApiError } from '@/api/errors'
import { authQueries, useLogout, useOAuthDecision, type OAuthAuthorizeParams } from '@/api/queries/auth'
import type { Me, OAuthConsent } from '@/api/types'
import { AuthLayout } from '@/components/auth-layout'
import { ButtonLink } from '@/components/button-link'
import { EmptyState } from '@/components/empty-state'
import { FormSkeleton } from '@/components/skeletons'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'

// Souhlas s připojením aplikace (OAuth pro MCP klienty, např. konektor v claude.ai). Sem posílá prohlížeč
// discovery serveru; parametry žádosti se jen předají API (ověří je backend) a po rozhodnutí se odchází
// na adresu z odpovědi (`redirect_to`). Při chybě žádosti nikdy nepřesměrováváme.
const param = z.string().optional().catch(undefined)
const searchSchema = z.object({
  client_id: param,
  redirect_uri: param,
  response_type: param,
  state: param,
  code_challenge: param,
  code_challenge_method: param,
  scope: param,
  resource: param,
})

export const Route = createFileRoute('/oauth/authorize')({
  validateSearch: searchSchema,
  beforeLoad: async ({ context, location }) => {
    const me = await context.queryClient.ensureQueryData(authQueries.me())
    // Po přihlášení (i s 2FA) se login vrátí sem i s celou žádostí v query.
    if (!me) throw redirect({ to: '/login', search: { redirect: location.href }, replace: true })
  },
  head: () => ({ meta: [{ title: 'Připojit aplikaci · NanoFaktura' }] }),
  component: AuthorizePage,
})

const TITLE = 'Připojit aplikaci k NanoFaktuře'

function AuthorizePage() {
  const params: OAuthAuthorizeParams = Route.useSearch()
  const consent = useQuery(authQueries.oauthConsent(params))
  const me = useQuery(authQueries.me())

  if (consent.isPending || me.isPending) {
    return (
      <AuthLayout title={TITLE}>
        <FormSkeleton fields={3} />
      </AuthLayout>
    )
  }
  if (consent.isError) return <ConsentError error={consent.error} />
  return <ConsentView params={params} consent={consent.data} me={me.data ?? null} />
}

/** Chybná žádost (neznámá aplikace, cizí návratová adresa, chybějící PKCE …): jen vysvětlení, žádná tlačítka. */
function ConsentError({ error }: { error: unknown }) {
  const message =
    isApiError(error) && error.status === 422 && !error.code
      ? 'Žádost aplikace o přístup je neúplná nebo neplatná. Zkuste aplikaci připojit znovu.'
      : errorMessage(error)
  return (
    <AuthLayout>
      <EmptyState
        icon={CircleAlertIcon}
        title="Aplikaci nelze připojit"
        description={message}
        action={
          <ButtonLink to="/" variant="outline">
            Do aplikace
          </ButtonLink>
        }
      />
    </AuthLayout>
  )
}

const can = [
  'Číst a hledat faktury, náklady, kontakty a přehledy ve všech vašich účtech.',
  'Připravovat koncepty faktur, zapisovat náklady, kontakty a úhrady.',
  'Vše se stejnými oprávněními, jaká máte vy.',
]

function ConsentView({ params, consent, me }: { params: OAuthAuthorizeParams; consent: OAuthConsent; me: Me | null }) {
  const decide = useOAuthDecision()
  // Po úspěchu prohlížeč odchází na adresu aplikace — tlačítka necháme zamčená až do opuštění stránky.
  const [leaving, setLeaving] = useState<'approve' | 'deny' | null>(null)
  const [error, setError] = useState<string | null>(null)
  const busy = decide.isPending || leaving !== null

  const submit = (approve: boolean) => {
    setError(null)
    decide.mutate(
      {
        client_id: params.client_id ?? '',
        redirect_uri: params.redirect_uri ?? '',
        response_type: params.response_type ?? '',
        code_challenge: params.code_challenge ?? '',
        code_challenge_method: params.code_challenge_method ?? '',
        state: params.state,
        scope: params.scope,
        resource: params.resource,
        approve,
      },
      {
        onSuccess: ({ redirect_to }) => {
          setLeaving(approve ? 'approve' : 'deny')
          window.location.assign(redirect_to)
        },
        onError: (err) => setError(errorMessage(err)),
      },
    )
  }
  const pending = (approve: boolean) =>
    (decide.isPending && decide.variables?.approve === approve) || leaving === (approve ? 'approve' : 'deny')

  return (
    <AuthLayout title={TITLE} className="sm:max-w-md">
      <div className="flex flex-col gap-4 rounded-xl border bg-card p-4 text-sm">
        <div className="flex items-start gap-3">
          <span className="flex size-10 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary">
            <PlugIcon className="size-5" />
          </span>
          <div className="flex min-w-0 flex-col gap-1">
            <p className="text-base">
              <strong className="font-semibold break-words">{consent.client_name}</strong> chce přístup k vašemu účtu
            </p>
            <p className="text-xs text-muted-foreground">
              Název uvádí sama aplikace. Povolte přístup, jen pokud jste ji právě připojovali vy.
            </p>
          </div>
        </div>
        <div className="flex flex-col gap-0.5 rounded-lg bg-muted px-3 py-2.5">
          <span className="text-xs text-muted-foreground">Po povolení se vrátíte na</span>
          <span className="flex min-w-0 items-center gap-1.5 font-semibold">
            <span className="min-w-0 break-all">{consent.redirect_host}</span>
            <ArrowUpRightIcon className="size-4 shrink-0 text-muted-foreground" />
          </span>
        </div>
        {me && <SignedInAs me={me} />}
      </div>

      <div className="flex flex-col gap-3 text-sm">
        <h2 className="font-medium">Aplikace bude moci</h2>
        <ul className="flex flex-col gap-2">
          {can.map((text) => (
            <li key={text} className="flex gap-2.5">
              <CheckIcon className="mt-0.5 size-4 shrink-0 text-primary" />
              <span>{text}</span>
            </li>
          ))}
          <li className="flex gap-2.5">
            <BanIcon className="mt-0.5 size-4 shrink-0 text-muted-foreground" />
            <span>Nic neodešle e-mailem, nic nesmaže a nezmění nastavení.</span>
          </li>
        </ul>
        <p className="text-muted-foreground">
          Přístup můžete kdykoli zrušit v <em>Nastavení → API tokeny → Připojené aplikace</em>.
        </p>
      </div>

      {error && (
        <Alert variant="destructive">
          <TriangleAlertIcon />
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}
      <div className="flex flex-col gap-3">
        <Button size="lg" className="w-full" disabled={busy} onClick={() => submit(true)}>
          {pending(true) && <Spinner data-icon="inline-start" />}
          Povolit přístup
        </Button>
        <Button size="lg" variant="outline" className="w-full" disabled={busy} onClick={() => submit(false)}>
          {pending(false) && <Spinner data-icon="inline-start" />}
          Zamítnout
        </Button>
      </div>
    </AuthLayout>
  )
}

/** Kdo je přihlášený + přepnutí na jiný účet (odhlásit a zpět přes přihlášení sem). */
function SignedInAs({ me }: { me: Me }) {
  const logout = useLogout()
  const navigate = useNavigate()
  const href = useLocation({ select: (l) => l.href })
  const switchUser = () =>
    logout.mutate(undefined, {
      onSettled: () => void navigate({ to: '/login', search: { redirect: href }, replace: true }),
    })
  return (
    <div className="flex flex-wrap items-center justify-between gap-x-3 gap-y-1 border-t pt-3">
      <p className="min-w-0 text-muted-foreground">
        Přihlášeni jako{' '}
        <strong className="font-medium break-all text-foreground">
          {me.user.name ? `${me.user.name} (${me.user.email})` : me.user.email}
        </strong>
      </p>
      <Button
        variant="link"
        size="sm"
        className="h-auto min-h-11 px-0 md:min-h-8"
        disabled={logout.isPending}
        onClick={switchUser}
      >
        {logout.isPending && <Spinner data-icon="inline-start" />}
        Přihlásit jiným účtem
      </Button>
    </div>
  )
}
