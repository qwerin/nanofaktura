import { zodResolver } from '@hookform/resolvers/zod'
import { createFileRoute } from '@tanstack/react-router'
import { useForm } from 'react-hook-form'
import { toast } from 'sonner'
import { z } from 'zod'
import { applyProblemToForm, errorMessage, isApiError } from '@/api/errors'
import { useUpdateMe } from '@/api/queries/auth'
import { TextField } from '@/components/form/fields'
import { FormSection, SettingsPage } from '@/components/settings-page'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'
import { useCurrentAccount } from '@/hooks/use-current-account'
import { MIN_PASSWORD_LENGTH, passwordSchema } from '@/lib/validation'

export const Route = createFileRoute('/a/$slug/settings/profile')({
  head: () => ({ meta: [{ title: 'Můj profil · NanoFaktura' }] }),
  component: ProfilePage,
})

function ProfilePage() {
  const { user } = useCurrentAccount()
  if (!user) return null
  return (
    <SettingsPage title="Můj profil" description="Údaje vašeho uživatelského účtu. Platí pro všechny firmy, ke kterým máte přístup.">
      <div className="max-w-4xl">
        <NameForm key={user.name} name={user.name} email={user.email} />
        <PasswordForm />
      </div>
    </SettingsPage>
  )
}

const nameSchema = z.object({ name: z.string().trim().min(1, 'Zadejte jméno') })
type NameValues = z.infer<typeof nameSchema>

function NameForm({ name, email }: { name: string; email: string }) {
  const update = useUpdateMe()
  const form = useForm<NameValues>({ resolver: zodResolver(nameSchema), defaultValues: { name } })

  const onSubmit = form.handleSubmit(async (values) => {
    try {
      await update.mutateAsync({ name: values.name })
      toast.success('Jméno uloženo')
    } catch (err) {
      if (!applyProblemToForm(err, form.setError)) toast.error(errorMessage(err))
    }
  })

  return (
    <form onSubmit={onSubmit} noValidate>
      <FormSection title="Osobní údaje" description={<>Přihlašovací e-mail: <span className="font-medium text-foreground">{email}</span></>}>
        <TextField control={form.control} name="name" label="Jméno" autoComplete="name" enterKeyHint="done" />
        <div>
          <Button type="submit" disabled={update.isPending || !form.formState.isDirty} className="max-md:w-full">
            {update.isPending && <Spinner data-icon="inline-start" />}
            Uložit jméno
          </Button>
        </div>
      </FormSection>
    </form>
  )
}

const passwordFormSchema = z
  .object({
    current_password: z.string().min(1, 'Zadejte současné heslo'),
    new_password: passwordSchema,
    confirm: z.string(),
  })
  .refine((v) => v.new_password === v.confirm, { path: ['confirm'], message: 'Hesla se neshodují' })
type PasswordValues = z.infer<typeof passwordFormSchema>

function PasswordForm() {
  const update = useUpdateMe()
  const form = useForm<PasswordValues>({
    resolver: zodResolver(passwordFormSchema),
    defaultValues: { current_password: '', new_password: '', confirm: '' },
  })

  const onSubmit = form.handleSubmit(async ({ current_password, new_password }) => {
    try {
      await update.mutateAsync({ current_password, password: new_password })
      form.reset()
      toast.success('Heslo změněno')
    } catch (err) {
      if (applyProblemToForm(err, form.setError)) return
      if (isApiError(err) && (err.status === 403 || err.status === 401 || err.status === 422)) {
        form.setError('current_password', { message: 'Současné heslo není správné' })
      } else {
        toast.error(errorMessage(err))
      }
    }
  })

  return (
    <form onSubmit={onSubmit} noValidate>
      <FormSection title="Změna hesla" description={`Nové heslo musí mít alespoň ${MIN_PASSWORD_LENGTH} znaků.`}>
        <TextField
          control={form.control}
          name="current_password"
          label="Současné heslo"
          type="password"
          autoComplete="current-password"
          enterKeyHint="next"
        />
        <TextField
          control={form.control}
          name="new_password"
          label="Nové heslo"
          type="password"
          autoComplete="new-password"
          enterKeyHint="next"
        />
        <TextField
          control={form.control}
          name="confirm"
          label="Nové heslo znovu"
          type="password"
          autoComplete="new-password"
          enterKeyHint="done"
        />
        <div>
          <Button type="submit" disabled={update.isPending} className="max-md:w-full">
            {update.isPending && <Spinner data-icon="inline-start" />}
            Změnit heslo
          </Button>
        </div>
      </FormSection>
    </form>
  )
}
