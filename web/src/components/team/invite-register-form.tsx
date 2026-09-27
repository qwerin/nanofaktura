import { zodResolver } from '@hookform/resolvers/zod'
import { useNavigate } from '@tanstack/react-router'
import { useForm } from 'react-hook-form'
import { z } from 'zod'
import { applyProblemToForm, errorMessage, isApiError } from '@/api/errors'
import { useRegister } from '@/api/queries/auth'
import { TextField } from '@/components/form/fields'
import { Button } from '@/components/ui/button'
import { Field, FieldDescription, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Spinner } from '@/components/ui/spinner'
import { MIN_PASSWORD_LENGTH, passwordSchema } from '@/lib/validation'

const schema = z.object({
  name: z.string().trim().min(1, 'Zadejte své jméno'),
  password: passwordSchema,
})
type Values = z.infer<typeof schema>

/**
 * Registrace z pozvánky: e-mail je daný pozvánkou, nový účet (firma) se nezakládá —
 * uživatel rovnou vstoupí do účtu, kam byl pozván. Funguje i při vypnuté registraci.
 */
export function InviteRegisterForm({ token, email }: { token: string; email: string }) {
  const navigate = useNavigate()
  const register = useRegister()
  const form = useForm<Values>({ resolver: zodResolver(schema), defaultValues: { name: '', password: '' } })

  const onSubmit = form.handleSubmit(async (values) => {
    try {
      const me = await register.mutateAsync({ ...values, email, invitation_token: token })
      const slug = me.accounts?.[0]?.slug
      if (slug) await navigate({ to: '/a/$slug/dashboard', params: { slug }, replace: true })
      else await navigate({ to: '/', replace: true })
    } catch (err) {
      if (applyProblemToForm(err, form.setError, ['name', 'password'])) return
      form.setError('root', {
        message: isApiError(err)
          ? err.status === 404 || err.status === 410
            ? 'Pozvánka mezitím přestala platit. Požádejte o novou.'
            : err.status === 409
              ? 'Uživatel s tímto e-mailem už existuje. Přihlaste se.'
              : err.status === 422
                ? 'Registrace se nezdařila — e-mail nesedí s pozvánkou.'
                : errorMessage(err)
          : errorMessage(err),
      })
    }
  })

  const rootError = form.formState.errors.root?.message

  return (
    <form onSubmit={onSubmit} noValidate className="flex flex-col gap-6">
      <FieldGroup className="gap-5">
        <Field>
          <FieldLabel htmlFor="invite-email">E-mail</FieldLabel>
          <Input id="invite-email" type="email" value={email} readOnly disabled autoComplete="username" />
          <FieldDescription>Pozvánka platí jen pro tuto adresu.</FieldDescription>
        </Field>
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
          name="password"
          label="Heslo"
          type="password"
          autoComplete="new-password"
          description={`Alespoň ${MIN_PASSWORD_LENGTH} znaků.`}
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
        Vytvořit přihlášení a vstoupit
      </Button>
    </form>
  )
}
