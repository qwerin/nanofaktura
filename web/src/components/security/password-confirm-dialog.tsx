import { zodResolver } from '@hookform/resolvers/zod'
import { useId, type ReactNode } from 'react'
import { useForm } from 'react-hook-form'
import { z } from 'zod'
import { TextField } from '@/components/form/fields'
import { ResponsiveDialog } from '@/components/responsive-dialog'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'
import { passwordErrorToForm } from './password-error'

const schema = z.object({ password: z.string().min(1, 'Zadejte své heslo') })
type Values = z.infer<typeof schema>

/** Potvrzení citlivé akce aktuálním heslem (SPEC §3.1). `onConfirm` vyhazuje chyby API. */
export function PasswordConfirmDialog({
  open,
  onOpenChange,
  title,
  description,
  children,
  confirmLabel,
  destructive,
  onConfirm,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  title: string
  description?: ReactNode
  children?: ReactNode
  confirmLabel: string
  destructive?: boolean
  onConfirm: (password: string) => Promise<unknown>
}) {
  const formId = useId()
  const form = useForm<Values>({ resolver: zodResolver(schema), defaultValues: { password: '' } })
  const pending = form.formState.isSubmitting

  const handleOpenChange = (next: boolean) => {
    if (!next && pending) return
    onOpenChange(next)
    if (!next) setTimeout(() => form.reset(), 300)
  }

  const onSubmit = form.handleSubmit(async ({ password }) => {
    try {
      await onConfirm(password)
      handleOpenChange(false)
      form.reset()
    } catch (err) {
      passwordErrorToForm(err, form.setError)
    }
  })

  const rootError = form.formState.errors.root?.message
  return (
    <ResponsiveDialog
      open={open}
      onOpenChange={handleOpenChange}
      title={title}
      description={description}
      footer={
        <>
          <Button type="submit" form={formId} variant={destructive ? 'destructive' : 'default'} disabled={pending}>
            {pending && <Spinner data-icon="inline-start" />}
            {confirmLabel}
          </Button>
          <Button variant="outline" onClick={() => handleOpenChange(false)} disabled={pending}>
            Zrušit
          </Button>
        </>
      }
    >
      <form id={formId} onSubmit={onSubmit} noValidate className="flex flex-col gap-4">
        {children}
        <TextField
          control={form.control}
          name="password"
          label="Vaše heslo"
          description="Pro jistotu potvrďte, že jste to vy."
          type="password"
          autoComplete="current-password"
          enterKeyHint="done"
          autoFocus
        />
        {rootError && (
          <p role="alert" className="text-sm text-destructive">
            {rootError}
          </p>
        )}
      </form>
    </ResponsiveDialog>
  )
}
