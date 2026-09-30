import { zodResolver } from '@hookform/resolvers/zod'
import { useQuery } from '@tanstack/react-query'
import { createFileRoute, Link, redirect, useRouter } from '@tanstack/react-router'
import { useEffect, useState } from 'react'
import { useForm, useWatch } from 'react-hook-form'
import { z } from 'zod'
import { errorMessage, isApiError } from '@/api/errors'
import { authQueries, useLogin } from '@/api/queries/auth'
import { invitationQueries } from '@/api/queries/invitations'
import type { TwoFactorChallenge } from '@/api/types'
import { AuthLayout } from '@/components/auth-layout'
import { TextField } from '@/components/form/fields'
import { TwoFactorLogin } from '@/components/security/two-factor-login'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { FieldGroup } from '@/components/ui/field'
import { Spinner } from '@/components/ui/spinner'
import { safeRedirect } from '@/lib/safe-redirect'

const searchSchema = z.object({
  redirect: z.string().optional().catch(undefined),
  /** Předvyplněný e-mail (např. po obnově hesla). */
  email: z.string().optional().catch(undefined),
})

/** Token pozvánky z `redirect=/invite/<token>` (přihlášení z odkazu v pozvánce). */
function inviteTokenFrom(redirect: string | undefined): string | undefined {
  if (!redirect?.startsWith('/invite/')) return undefined
  return redirect.slice('/invite/'.length).split(/[/?#]/)[0] || undefined
}

export const Route = createFileRoute('/login')({
  validateSearch: searchSchema,
  beforeLoad: async ({ context, search }) => {
    // Nedostupný backend nesmí zablokovat zobrazení formuláře.
    const me = await context.queryClient.ensureQueryData(authQueries.me()).catch(() => null)
    if (me) throw redirect({ href: safeRedirect(search.redirect), replace: true })
  },
  head: () => ({ meta: [{ title: 'Přihlášení · NanoFaktura' }] }),
  component: LoginPage,
})

const schema = z.object({
  email: z.email('Zadejte platný e-mail'),
  password: z.string().min(1, 'Zadejte heslo'),
})
type Values = z.infer<typeof schema>

function LoginPage() {
  const search = Route.useSearch()
  const router = useRouter()
  const login = useLogin()
  const status = useQuery({ ...authQueries.status(), meta: { silent: true } })
  const form = useForm<Values>({
    resolver: zodResolver(schema),
    defaultValues: { email: search.email ?? '', password: '' },
  })
  // Po hesle u uživatele s 2FA: druhý krok na téže stránce.
  const [challenge, setChallenge] = useState<TwoFactorChallenge | null>(null)
  const typedEmail = useWatch({ control: form.control, name: 'email' })
  const finish = () => router.history.push(safeRedirect(search.redirect))
  // Přihlášení z pozvánky: e-mail předvyplníme z pozvánky (ne z URL) a po přihlášení se vrátíme na /invite/$token.
  const inviteToken = inviteTokenFrom(search.redirect)
  const invitation = useQuery({ ...invitationQueries.info(inviteToken ?? ''), enabled: Boolean(inviteToken) })
  const invitedEmail = invitation.data?.email
  useEffect(() => {
    if (!invitedEmail || form.getValues('email')) return
    form.setValue('email', invitedEmail)
    form.setFocus('password')
  }, [invitedEmail, form])

  const onSubmit = form.handleSubmit(async (values) => {
    try {
      const result = await login.mutateAsync({ ...values, email: values.email.trim().toLowerCase() })
      if (result.two_factor) {
        setChallenge(result.two_factor)
        return
      }
      finish()
    } catch (err) {
      form.setError('root', {
        message:
          isApiError(err) && (err.status === 401 || err.status === 422)
            ? 'Nesprávný e-mail nebo heslo.'
            : errorMessage(err),
      })
    }
  })

  const rootError = form.formState.errors.root?.message
  // Na čisté instanci bez uživatelů rovnou nabídneme registraci.
  const firstRun = status.data && !status.data.has_users

  if (challenge) {
    return (
      <AuthLayout title="Ověření přihlášení" description="Váš účet chrání dvoufázové ověření. Potvrďte, že jste to vy.">
        <TwoFactorLogin
          challenge={challenge}
          onSuccess={finish}
          onBack={() => {
            setChallenge(null)
            form.resetField('password')
          }}
          onExpired={(message) => {
            setChallenge(null)
            form.resetField('password')
            form.setError('root', { message })
          }}
        />
      </AuthLayout>
    )
  }

  return (
    <AuthLayout
      title="Přihlášení"
      description={
        inviteToken
          ? 'Přihlaste se a pak pozvánku přijmete.'
          : 'Vítejte zpět. Přihlaste se ke svému účtu.'
      }
      footer={
        status.data?.signup_allowed ? (
          <>
            Nemáte účet?{' '}
            <Link to="/register" className="font-medium text-primary underline-offset-4 hover:underline">
              Zaregistrujte se
            </Link>
          </>
        ) : null
      }
    >
      {firstRun && (
        <Alert>
          <AlertDescription>
            Tato instance zatím nemá žádného uživatele.{' '}
            <Link to="/register" className="font-medium text-primary underline-offset-4 hover:underline">
              Vytvořte první účet
            </Link>
            .
          </AlertDescription>
        </Alert>
      )}
      <form onSubmit={onSubmit} noValidate className="flex flex-col gap-6">
        <FieldGroup className="gap-5">
          <TextField
            control={form.control}
            name="email"
            label="E-mail"
            type="email"
            inputMode="email"
            autoComplete="username"
            autoCapitalize="none"
            spellCheck={false}
            enterKeyHint="next"
            autoFocus
          />
          <div className="flex flex-col gap-1">
            <TextField
              control={form.control}
              name="password"
              label="Heslo"
              type="password"
              autoComplete="current-password"
              enterKeyHint="go"
            />
            <Link
              to="/forgot-password"
              search={{ email: typedEmail.trim() || undefined }}
              className="-mb-2 inline-flex min-h-11 items-center self-end text-sm font-medium text-primary underline-offset-4 hover:underline md:min-h-8"
            >
              Zapomenuté heslo?
            </Link>
          </div>
        </FieldGroup>
        {rootError && (
          <p role="alert" className="text-sm text-destructive">
            {rootError}
          </p>
        )}
        <Button type="submit" size="lg" className="w-full" disabled={login.isPending}>
          {login.isPending && <Spinner data-icon="inline-start" />}
          Přihlásit se
        </Button>
      </form>
    </AuthLayout>
  )
}
