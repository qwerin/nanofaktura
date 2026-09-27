import { zodResolver } from '@hookform/resolvers/zod'
import { useId } from 'react'
import { Controller, useForm } from 'react-hook-form'
import { toast } from 'sonner'
import { z } from 'zod'
import { isApiError } from '@/api/errors'
import { useInviteMember } from '@/api/queries/members'
import type { MemberRole } from '@/api/types'
import { TextField } from '@/components/form/fields'
import { ResponsiveDialog } from '@/components/responsive-dialog'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'
import { teamErrorMessage } from './permissions'
import { RolePicker } from './role-picker'

const schema = z.object({
  email: z.email('Zadejte platný e-mail'),
  role: z.enum(['owner', 'admin', 'accountant', 'member']),
})
type Values = z.infer<typeof schema>

export function InviteDialog({
  slug,
  open,
  onOpenChange,
  roles,
  memberEmails,
}: {
  slug: string
  open: boolean
  onOpenChange: (open: boolean) => void
  /** Role, které smí přihlášený uživatel přidělit. */
  roles: MemberRole[]
  /** E-maily stávajících členů (lowercase) — upozorníme dřív, než to odmítne server. */
  memberEmails: string[]
}) {
  const invite = useInviteMember(slug)
  const roleLabelId = useId()
  const form = useForm<Values>({
    resolver: zodResolver(schema),
    defaultValues: { email: '', role: 'member' },
  })

  const close = () => {
    onOpenChange(false)
    setTimeout(() => form.reset(), 300)
  }

  const onSubmit = form.handleSubmit(async (values) => {
    const email = values.email.trim().toLowerCase()
    if (memberEmails.includes(email)) {
      form.setError('email', { message: 'Tento uživatel už je členem účtu.' })
      return
    }
    try {
      await invite.mutateAsync({ email, role: values.role })
      toast.success(`Pozvánka odeslána na ${email}`)
      close()
    } catch (err) {
      const message = teamErrorMessage(err)
      if (isApiError(err) && (err.status === 409 || err.status === 422)) form.setError('email', { message })
      else form.setError('root', { message })
    }
  })

  const rootError = form.formState.errors.root?.message

  return (
    <ResponsiveDialog
      open={open}
      onOpenChange={(o) => (o ? onOpenChange(true) : close())}
      title="Pozvat do týmu"
      description="Pošleme e-mail s odkazem. Pozvánka platí 7 dní; pozvaný si vytvoří přihlášení, nebo se přihlásí stávajícím."
      className="sm:max-w-lg"
      footer={
        <>
          <Button type="submit" form="invite-form" disabled={invite.isPending}>
            {invite.isPending && <Spinner data-icon="inline-start" />}
            Odeslat pozvánku
          </Button>
          <Button variant="outline" onClick={close}>
            Zrušit
          </Button>
        </>
      }
    >
      <form id="invite-form" onSubmit={onSubmit} noValidate className="flex flex-col gap-5">
        <TextField
          control={form.control}
          name="email"
          label="E-mail"
          type="email"
          inputMode="email"
          autoComplete="off"
          autoCapitalize="none"
          spellCheck={false}
          placeholder="kolega@firma.cz"
          enterKeyHint="send"
        />
        <div className="flex flex-col gap-2">
          <span id={roleLabelId} className="text-sm font-medium">
            Role
          </span>
          <Controller
            control={form.control}
            name="role"
            render={({ field }) => (
              <RolePicker
                name="invite-role"
                aria-labelledby={roleLabelId}
                roles={roles}
                value={field.value}
                onChange={field.onChange}
              />
            )}
          />
        </div>
        {rootError && (
          <p role="alert" className="text-sm text-destructive">
            {rootError}
          </p>
        )}
      </form>
    </ResponsiveDialog>
  )
}
