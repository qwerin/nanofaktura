import { zodResolver } from '@hookform/resolvers/zod'
import { useQuery } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import { CircleAlertIcon, CircleCheckIcon, ClockIcon, LinkIcon, ShieldCheckIcon } from 'lucide-react'
import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { z } from 'zod'
import { applyProblemToForm, errorMessage, hasErrorCode, isApiError } from '@/api/errors'
import { authQueries, useConfirmPasswordReset } from '@/api/queries/auth'
import type { PasswordResetInfo } from '@/api/types'
import { AuthLayout } from '@/components/auth-layout'
import { ButtonLink } from '@/components/button-link'
import { EmptyState } from '@/components/empty-state'
import { TextField } from '@/components/form/fields'
import { FormSkeleton } from '@/components/skeletons'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { FieldGroup } from '@/components/ui/field'
import { Spinner } from '@/components/ui/spinner'
import { MIN_PASSWORD_LENGTH, passwordSchema } from '@/lib/validation'

// Nastavení nového hesla z odkazu v e-mailu (SPEC §3.1).
export const Route = createFileRoute('/reset-password/$token')({
  head: () => ({ meta: [{ title: 'Nové heslo · NanoFaktura' }] }),
  component: ResetPasswordPage,
})

function ResetPasswordPage() {
  const { token } = Route.useParams()
  const info = useQuery(authQueries.passwordReset(token))
  const [done, setDone] = useState(false)

  if (done && info.data) return <ResetDone info={info.data} />

  if (info.isPending) {
    return (
      <AuthLayout title="Nové heslo">
        <FormSkeleton fields={2} />
      </AuthLayout>
    )
  }

  if (info.isError) {
    return (
      <ResetError
        status={isApiError(info.error) ? info.error.status : 0}
        message={errorMessage(info.error)}
        retry={() => void info.refetch()}
      />
    )
  }

  return <ResetForm token={token} info={info.data} onDone={() => setDone(true)} />
}

function ResetError({ status, message, retry }: { status: number; message?: string; retry?: () => void }) {
  const again = <ButtonLink to="/forgot-password">Poslat nový odkaz</ButtonLink>
  if (status === 404) {
    return (
      <AuthLayout>
        <EmptyState
          icon={LinkIcon}
          title="Odkaz nenalezen"
          description="Odkaz je neúplný nebo neplatný. Zkontrolujte, že jste ho z e-mailu otevřeli celý, případně si nechte poslat nový."
          action={again}
        />
      </AuthLayout>
    )
  }
  if (status === 410) {
    return (
      <AuthLayout>
        <EmptyState
          icon={ClockIcon}
          title="Odkaz už neplatí"
          description="Platí jen 1 hodinu a jde použít jednou. Nechte si poslat nový odkaz."
          action={again}
        />
      </AuthLayout>
    )
  }
  return (
    <AuthLayout>
      <EmptyState
        icon={CircleAlertIcon}
        title="Odkaz se nepodařilo ověřit"
        description={message}
        action={retry ? <Button onClick={retry}>Zkusit znovu</Button> : again}
      />
    </AuthLayout>
  )
}

const schema = z
  .object({
    password: passwordSchema.max(72, 'Heslo může mít nejvýše 72 znaků'),
    confirm: z.string(),
  })
  .refine((v) => v.password === v.confirm, { path: ['confirm'], message: 'Hesla se neshodují' })
type Values = z.infer<typeof schema>

function ResetForm({ token, info, onDone }: { token: string; info: PasswordResetInfo; onDone: () => void }) {
  const confirm = useConfirmPasswordReset(token)
  const [expired, setExpired] = useState(false)
  const form = useForm<Values>({ resolver: zodResolver(schema), defaultValues: { password: '', confirm: '' } })

  const onSubmit = form.handleSubmit(async ({ password }) => {
    try {
      await confirm.mutateAsync(password)
      onDone()
    } catch (err) {
      if (hasErrorCode(err, 'reset_expired') || (isApiError(err) && (err.status === 404 || err.status === 410))) {
        setExpired(true)
      } else if (!applyProblemToForm(err, form.setError)) {
        form.setError('root', { message: errorMessage(err) })
      }
    }
  })

  if (expired) return <ResetError status={410} />

  const rootError = form.formState.errors.root?.message
  return (
    <AuthLayout
      title="Nové heslo"
      description={
        <>
          Nastavte nové heslo pro <strong className="text-foreground">{info.email}</strong>. Musí mít alespoň{' '}
          {MIN_PASSWORD_LENGTH} znaků.
        </>
      }
    >
      <form onSubmit={onSubmit} noValidate className="flex flex-col gap-6">
        {/* Skryté pole s e-mailem pomůže správcům hesel uložit nové heslo ke správnému účtu. */}
        <input type="email" name="username" autoComplete="username" value={info.email} readOnly hidden />
        <FieldGroup className="gap-5">
          <TextField
            control={form.control}
            name="password"
            label="Nové heslo"
            type="password"
            autoComplete="new-password"
            enterKeyHint="next"
            autoFocus
          />
          <TextField
            control={form.control}
            name="confirm"
            label="Nové heslo znovu"
            type="password"
            autoComplete="new-password"
            enterKeyHint="done"
          />
        </FieldGroup>
        <p className="text-sm text-muted-foreground">
          Po změně hesla budete odhlášeni na všech zařízeních.
        </p>
        {rootError && (
          <p role="alert" className="text-sm text-destructive">
            {rootError}
          </p>
        )}
        <Button type="submit" size="lg" className="w-full" disabled={confirm.isPending}>
          {confirm.isPending && <Spinner data-icon="inline-start" />}
          Nastavit heslo
        </Button>
      </form>
    </AuthLayout>
  )
}

function ResetDone({ info }: { info: PasswordResetInfo }) {
  return (
    <AuthLayout>
      <EmptyState
        icon={CircleCheckIcon}
        title="Heslo je změněné"
        description="Teď se přihlaste novým heslem."
        action={
          <div className="flex w-full flex-col gap-4">
            {info.two_factor && (
              <Alert className="text-left">
                <ShieldCheckIcon />
                <AlertDescription>
                  Dvoufázové ověření zůstává zapnuté — po zadání hesla potvrdíte přihlášení kódem z aplikace,
                  bezpečnostním klíčem nebo záložním kódem.
                </AlertDescription>
              </Alert>
            )}
            <ButtonLink to="/login" search={{ email: info.email }} size="lg" className="w-full">
              Přihlásit se
            </ButtonLink>
          </div>
        }
      />
    </AuthLayout>
  )
}
