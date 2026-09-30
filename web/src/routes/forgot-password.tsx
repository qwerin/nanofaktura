import { zodResolver } from '@hookform/resolvers/zod'
import { createFileRoute, Link } from '@tanstack/react-router'
import { MailCheckIcon } from 'lucide-react'
import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { z } from 'zod'
import { applyProblemToForm, errorMessage } from '@/api/errors'
import { useRequestPasswordReset } from '@/api/queries/auth'
import { AuthLayout } from '@/components/auth-layout'
import { ButtonLink } from '@/components/button-link'
import { EmptyState } from '@/components/empty-state'
import { TextField } from '@/components/form/fields'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'

// Zapomenuté heslo (SPEC §3.1): odpověď je vždy stejná, ať e-mail existuje, nebo ne.
export const Route = createFileRoute('/forgot-password')({
  validateSearch: z.object({ email: z.string().optional().catch(undefined) }),
  head: () => ({ meta: [{ title: 'Zapomenuté heslo · NanoFaktura' }] }),
  component: ForgotPasswordPage,
})

const schema = z.object({ email: z.email('Zadejte platný e-mail') })
type Values = z.infer<typeof schema>

function ForgotPasswordPage() {
  const search = Route.useSearch()
  const request = useRequestPasswordReset()
  const [sentTo, setSentTo] = useState<string | null>(null)
  const form = useForm<Values>({ resolver: zodResolver(schema), defaultValues: { email: search.email ?? '' } })

  const onSubmit = form.handleSubmit(async ({ email }) => {
    const normalized = email.trim().toLowerCase()
    try {
      await request.mutateAsync(normalized)
      setSentTo(normalized)
    } catch (err) {
      if (!applyProblemToForm(err, form.setError)) form.setError('root', { message: errorMessage(err) })
    }
  })

  if (sentTo) {
    return (
      <AuthLayout>
        <EmptyState
          icon={MailCheckIcon}
          title="Zkontrolujte e-mail"
          description={
            <>
              Pokud k adrese <strong className="text-foreground">{sentTo}</strong> existuje účet, poslali jsme na ni odkaz
              pro nastavení nového hesla. Odkaz platí 1 hodinu. Nic nepřišlo? Podívejte se i do spamu, případně to
              za pár minut zkuste znovu.
            </>
          }
          action={
            <ButtonLink to="/login" search={{ email: sentTo }}>
              Zpět na přihlášení
            </ButtonLink>
          }
        />
      </AuthLayout>
    )
  }

  const rootError = form.formState.errors.root?.message
  return (
    <AuthLayout
      title="Zapomenuté heslo"
      description="Zadejte e-mail, kterým se přihlašujete. Pošleme vám odkaz pro nastavení nového hesla."
      footer={
        <>
          Heslo si pamatujete?{' '}
          <Link to="/login" className="font-medium text-primary underline-offset-4 hover:underline">
            Přihlaste se
          </Link>
        </>
      }
    >
      <form onSubmit={onSubmit} noValidate className="flex flex-col gap-6">
        <TextField
          control={form.control}
          name="email"
          label="E-mail"
          type="email"
          inputMode="email"
          autoComplete="username"
          autoCapitalize="none"
          spellCheck={false}
          enterKeyHint="send"
          autoFocus
        />
        {rootError && (
          <p role="alert" className="text-sm text-destructive">
            {rootError}
          </p>
        )}
        <Button type="submit" size="lg" className="w-full" disabled={request.isPending}>
          {request.isPending && <Spinner data-icon="inline-start" />}
          Poslat odkaz
        </Button>
      </form>
    </AuthLayout>
  )
}
