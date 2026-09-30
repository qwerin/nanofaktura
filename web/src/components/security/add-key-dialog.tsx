import { zodResolver } from '@hookform/resolvers/zod'
import { useId, useState } from 'react'
import { useForm } from 'react-hook-form'
import { toast } from 'sonner'
import { z } from 'zod'
import { hasErrorCode, isApiError } from '@/api/errors'
import { useAddWebAuthnKey } from '@/api/queries/auth'
import { TextField } from '@/components/form/fields'
import { ResponsiveDialog } from '@/components/responsive-dialog'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'
import { isWebAuthnCancelled, webauthnErrorMessage } from '@/lib/webauthn'
import { passwordErrorToForm } from './password-error'

const schema = z.object({
  name: z.string().trim().min(1, 'Pojmenujte klíč').max(100, 'Nejvýše 100 znaků'),
  password: z.string().min(1, 'Zadejte své heslo'),
})
type Values = z.infer<typeof schema>

/** Přidání bezpečnostního klíče (YubiKey, passkey): název + heslo → dialog prohlížeče → uloženo. */
export function AddKeyDialog({
  open,
  onOpenChange,
  onAdded,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  /** Klíč přidán; `codes` jsou nové záložní kódy (jen u prvního faktoru). */
  onAdded: (codes: string[]) => void
}) {
  const add = useAddWebAuthnKey()
  const formId = useId()
  const [note, setNote] = useState<string | null>(null)
  const form = useForm<Values>({ resolver: zodResolver(schema), defaultValues: { name: 'Bezpečnostní klíč', password: '' } })

  const handleOpenChange = (next: boolean) => {
    if (!next && add.isPending) return
    onOpenChange(next)
    if (!next)
      setTimeout(() => {
        form.reset()
        setNote(null)
      }, 300)
  }

  const onSubmit = form.handleSubmit(async ({ name, password }) => {
    setNote(null)
    try {
      const created = await add.mutateAsync({ name: name.trim(), password })
      toast.success(`Klíč „${created.key.name}“ je přidaný`)
      handleOpenChange(false)
      onAdded(created.recovery_codes ?? [])
    } catch (err) {
      if (isApiError(err)) {
        if (hasErrorCode(err, 'already_exists')) form.setError('root', { message: 'Tento klíč už je přidaný.' })
        else if (hasErrorCode(err, 'webauthn_failed'))
          form.setError('root', { message: 'Klíč se nepodařilo ověřit. Zkuste to znovu, případně jiným klíčem.' })
        else passwordErrorToForm(err, form.setError)
      } else if (isWebAuthnCancelled(err)) {
        setNote('Přidání klíče bylo přerušeno. Až budete připraveni, zkuste to znovu.')
      } else {
        form.setError('root', { message: webauthnErrorMessage(err) ?? '' })
      }
    }
  })

  const rootError = form.formState.errors.root?.message
  return (
    <ResponsiveDialog
      open={open}
      onOpenChange={handleOpenChange}
      title="Přidat bezpečnostní klíč"
      description="Po potvrzení vás prohlížeč vyzve, abyste připojili klíč (např. YubiKey) a dotkli se ho, nebo použili passkey v telefonu či počítači."
      footer={
        <>
          <Button type="submit" form={formId} disabled={add.isPending}>
            {add.isPending && <Spinner data-icon="inline-start" />}
            Přidat klíč
          </Button>
          <Button variant="outline" onClick={() => handleOpenChange(false)} disabled={add.isPending}>
            Zrušit
          </Button>
        </>
      }
    >
      <form id={formId} onSubmit={onSubmit} noValidate className="flex flex-col gap-4">
        <TextField
          control={form.control}
          name="name"
          label="Název klíče"
          description="Podle názvu klíč poznáte v seznamu, např. „YubiKey na klíčích“."
          autoComplete="off"
          enterKeyHint="next"
        />
        <TextField
          control={form.control}
          name="password"
          label="Vaše heslo"
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
        {note && <p className="text-sm text-muted-foreground">{note}</p>}
        <p className="text-xs text-muted-foreground">
          Po přidání klíče vás odhlásíme na ostatních zařízeních; na tomto zůstanete přihlášeni.
        </p>
      </form>
    </ResponsiveDialog>
  )
}
