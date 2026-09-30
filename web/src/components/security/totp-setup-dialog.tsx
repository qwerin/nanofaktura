import { zodResolver } from '@hookform/resolvers/zod'
import { ExternalLinkIcon } from 'lucide-react'
import { useId, useState, type FormEvent } from 'react'
import { useForm } from 'react-hook-form'
import { toast } from 'sonner'
import { z } from 'zod'
import { codeMessages, errorMessage, hasErrorCode } from '@/api/errors'
import { useTotpEnable, useTotpSetup } from '@/api/queries/auth'
import type { TOTPSetup } from '@/api/types'
import { CopyButton } from '@/components/copy-button'
import { TextField } from '@/components/form/fields'
import { QrCode } from '@/components/public-invoice/qr-code'
import { ResponsiveDialog } from '@/components/responsive-dialog'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'
import { groupSecret } from '@/lib/two-factor'
import { passwordErrorToForm } from './password-error'
import { TOTP_LENGTH, TotpCodeInput } from './totp-code-input'

const passwordSchema = z.object({ password: z.string().min(1, 'Zadejte své heslo') })
type PasswordValues = z.infer<typeof passwordSchema>

/** Zapnutí ověřovací aplikace: heslo → QR kód / klíč → první kód → (záložní kódy předá rodiči). */
export function TotpSetupDialog({
  open,
  onOpenChange,
  onEnabled,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  /** Zapnuto; `codes` jsou nové záložní kódy (prázdné, když už uživatel nějaké má). */
  onEnabled: (codes: string[]) => void
}) {
  const [setup, setSetup] = useState<TOTPSetup | null>(null)
  const [code, setCode] = useState('')
  const [codeError, setCodeError] = useState<string | null>(null)
  const start = useTotpSetup()
  const enable = useTotpEnable()
  const passwordFormId = useId()
  const codeFormId = useId()
  const form = useForm<PasswordValues>({ resolver: zodResolver(passwordSchema), defaultValues: { password: '' } })

  const reset = () => {
    setSetup(null)
    setCode('')
    setCodeError(null)
    form.reset()
  }

  const handleOpenChange = (next: boolean) => {
    if (!next && enable.isPending) return
    onOpenChange(next)
    if (!next) setTimeout(reset, 300)
  }

  const onPassword = form.handleSubmit(async ({ password }) => {
    try {
      setSetup(await start.mutateAsync(password))
    } catch (err) {
      passwordErrorToForm(err, form.setError)
    }
  })

  const submitCode = (value: string) => {
    if (enable.isPending) return
    setCodeError(null)
    enable.mutate(value, {
      onSuccess: ({ recovery_codes }) => {
        toast.success('Ověřovací aplikace je zapnutá')
        handleOpenChange(false)
        onEnabled(recovery_codes ?? [])
      },
      onError: (err) => {
        setCode('')
        if (hasErrorCode(err, 'invalid_code')) {
          setCodeError('Kód nesedí. Zkontrolujte, že máte v telefonu správný čas, a zadejte aktuální kód.')
        } else if (hasErrorCode(err, 'totp_not_pending')) {
          setSetup(null)
          form.reset()
          form.setError('root', { message: codeMessages.totp_not_pending })
        } else {
          setCodeError(errorMessage(err))
        }
      },
    })
  }

  const onCode = (e: FormEvent) => {
    e.preventDefault()
    if (code.length !== TOTP_LENGTH) setCodeError(`Zadejte ${TOTP_LENGTH}místný kód z aplikace.`)
    else submitCode(code)
  }

  if (!setup) {
    const rootError = form.formState.errors.root?.message
    return (
      <ResponsiveDialog
        open={open}
        onOpenChange={handleOpenChange}
        title="Zapnout ověřovací aplikaci"
        description="Při přihlášení pak kromě hesla zadáte kód z aplikace v telefonu (např. Google Authenticator, Microsoft Authenticator, Aegis nebo 1Password)."
        footer={
          <>
            <Button type="submit" form={passwordFormId} disabled={start.isPending}>
              {start.isPending && <Spinner data-icon="inline-start" />}
              Pokračovat
            </Button>
            <Button variant="outline" onClick={() => handleOpenChange(false)}>
              Zrušit
            </Button>
          </>
        }
      >
        <form id={passwordFormId} onSubmit={onPassword} noValidate className="flex flex-col gap-4">
          <TextField
            control={form.control}
            name="password"
            label="Vaše heslo"
            description="Pro jistotu potvrďte, že jste to vy."
            type="password"
            autoComplete="current-password"
            enterKeyHint="next"
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

  return (
    <ResponsiveDialog
      open={open}
      onOpenChange={handleOpenChange}
      title="Naskenujte QR kód"
      description="V ověřovací aplikaci přidejte nový účet a naskenujte kód. Pak opište 6místný kód, který aplikace ukáže."
      footer={
        <>
          <Button type="submit" form={codeFormId} disabled={enable.isPending}>
            {enable.isPending && <Spinner data-icon="inline-start" />}
            Zapnout
          </Button>
          <Button variant="outline" onClick={() => handleOpenChange(false)} disabled={enable.isPending}>
            Zrušit
          </Button>
        </>
      }
    >
      <form id={codeFormId} onSubmit={onCode} noValidate className="flex flex-col gap-5">
        <div className="flex flex-col items-center gap-3">
          <QrCode value={setup.otpauth_url} label="QR kód pro ověřovací aplikaci" className="w-40 max-w-full sm:w-48" />
          {/* Na telefonu QR kód nenaskenujete — odkaz otevře ověřovací aplikaci přímo. */}
          <a
            href={setup.otpauth_url}
            className="inline-flex min-h-11 items-center gap-1.5 text-sm font-medium text-primary underline-offset-4 hover:underline md:hidden"
          >
            <ExternalLinkIcon className="size-4" />
            Otevřít v aplikaci v tomto telefonu
          </a>
        </div>
        <div className="flex flex-col gap-2">
          <p className="text-sm text-muted-foreground">Nejde to naskenovat? Zadejte v aplikaci tento klíč ručně:</p>
          <code
            aria-label="Tajný klíč"
            className="block rounded-lg border bg-muted px-3 py-2.5 text-center font-mono text-sm text-balance break-words select-all"
          >
            {groupSecret(setup.secret)}
          </code>
          <CopyButton value={setup.secret} label="Zkopírovat klíč" className="w-full" />
        </div>
        <TotpCodeInput
          value={code}
          onChange={(v) => {
            setCode(v)
            if (codeError) setCodeError(null)
          }}
          onComplete={submitCode}
          error={codeError ?? undefined}
          label="Kód z aplikace"
          disabled={enable.isPending}
        />
        <p className="text-xs text-muted-foreground">
          Po zapnutí vás odhlásíme na ostatních zařízeních; na tomto zůstanete přihlášeni.
        </p>
      </form>
    </ResponsiveDialog>
  )
}
