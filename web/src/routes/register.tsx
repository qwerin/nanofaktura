import { zodResolver } from '@hookform/resolvers/zod'
import { useQuery } from '@tanstack/react-query'
import { createFileRoute, Link, redirect, useNavigate } from '@tanstack/react-router'
import { LockIcon } from 'lucide-react'
import { useForm } from 'react-hook-form'
import { z } from 'zod'
import { applyProblemToForm, errorMessage, isApiError } from '@/api/errors'
import { authQueries, useRegister } from '@/api/queries/auth'
import { AuthLayout } from '@/components/auth-layout'
import { ButtonLink } from '@/components/button-link'
import { EmptyState } from '@/components/empty-state'
import { TextField } from '@/components/form/fields'
import { FormSkeleton } from '@/components/skeletons'
import { Button } from '@/components/ui/button'
import { FieldGroup } from '@/components/ui/field'
import { Spinner } from '@/components/ui/spinner'
import { MIN_PASSWORD_LENGTH, passwordSchema } from '@/lib/validation'

export const Route = createFileRoute('/register')({
  beforeLoad: async ({ context }) => {
    const me = await context.queryClient.ensureQueryData(authQueries.me()).catch(() => null)
    if (me) throw redirect({ to: '/', replace: true })
  },
  head: () => ({ meta: [{ title: 'Registrace · NanoFaktura' }] }),
  component: RegisterPage,
})

const schema = z.object({
  name: z.string().trim().min(1, 'Zadejte své jméno'),
  email: z.email('Zadejte platný e-mail'),
  password: passwordSchema,
  account_name: z.string().trim().min(1, 'Zadejte název firmy nebo své jméno'),
})
type Values = z.infer<typeof schema>

function RegisterPage() {
  const status = useQuery({ ...authQueries.status(), meta: { silent: true } })

  if (status.isPending) {
    return (
      <AuthLayout title="Registrace">
        <FormSkeleton fields={4} />
      </AuthLayout>
    )
  }

  if (!status.data?.signup_allowed) {
    return (
      <AuthLayout>
        <EmptyState
          icon={LockIcon}
          title="Registrace je vypnutá"
          description={
            status.isError
              ? errorMessage(status.error)
              : 'Nové účty na této instanci zakládá administrátor. Máte-li pozvánku, otevřete odkaz z e-mailu. Pokud už účet máte, přihlaste se.'
          }
          action={<ButtonLink to="/login">Přihlásit se</ButtonLink>}
        />
      </AuthLayout>
    )
  }

  return <RegisterForm firstUser={!status.data.has_users} />
}

function RegisterForm({ firstUser }: { firstUser: boolean }) {
  const navigate = useNavigate()
  const register = useRegister()
  const form = useForm<Values>({
    resolver: zodResolver(schema),
    defaultValues: { name: '', email: '', password: '', account_name: '' },
  })

  const onSubmit = form.handleSubmit(async (values) => {
    try {
      const me = await register.mutateAsync({ ...values, email: values.email.trim().toLowerCase() })
      const slug = me.accounts?.[0]?.slug
      if (slug) await navigate({ to: '/a/$slug/settings/company', params: { slug }, replace: true })
      else await navigate({ to: '/', replace: true })
    } catch (err) {
      if (applyProblemToForm(err, form.setError)) return
      form.setError('root', {
        message:
          isApiError(err) && err.status === 403
            ? 'Registrace není na této instanci povolena.'
            : errorMessage(err),
      })
    }
  })

  const rootError = form.formState.errors.root?.message

  return (
    <AuthLayout
      title={firstUser ? 'Vítejte v NanoFaktuře' : 'Registrace'}
      description={
        firstUser
          ? 'Vytvořte první uživatelský účet. Bude vlastníkem prvního fakturačního účtu.'
          : 'Založte si účet a začněte fakturovat.'
      }
      footer={
        <>
          Už máte účet?{' '}
          <Link to="/login" className="font-medium text-primary underline-offset-4 hover:underline">
            Přihlaste se
          </Link>
        </>
      }
    >
      <form onSubmit={onSubmit} noValidate className="flex flex-col gap-6">
        <FieldGroup className="gap-5">
          <TextField
            control={form.control}
            name="name"
            label="Vaše jméno"
            autoComplete="name"
            enterKeyHint="next"
            autoFocus
          />
          <TextField
            control={form.control}
            name="email"
            label="E-mail"
            type="email"
            inputMode="email"
            autoComplete="email"
            autoCapitalize="none"
            spellCheck={false}
            enterKeyHint="next"
          />
          <TextField
            control={form.control}
            name="password"
            label="Heslo"
            type="password"
            autoComplete="new-password"
            description={`Alespoň ${MIN_PASSWORD_LENGTH} znaků.`}
            enterKeyHint="next"
          />
          <TextField
            control={form.control}
            name="account_name"
            label="Název firmy / OSVČ"
            autoComplete="organization"
            description="Za koho budete vystavovat faktury. Další údaje doplníte v nastavení."
            enterKeyHint="go"
          />
        </FieldGroup>
        {rootError && (
          <p role="alert" className="text-sm text-destructive">
            {rootError}
          </p>
        )}
        <Button type="submit" size="lg" className="w-full" disabled={register.isPending}>
          {register.isPending && <Spinner data-icon="inline-start" />}
          Vytvořit účet
        </Button>
      </form>
    </AuthLayout>
  )
}
