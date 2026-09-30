import { ArrowLeftIcon, KeyRoundIcon } from 'lucide-react'
import { useId, useState, type FormEvent } from 'react'
import { codeMessages, errorMessage, hasErrorCode, isApiError } from '@/api/errors'
import { useLoginCode, useLoginWebAuthn } from '@/api/queries/auth'
import type { Me, TwoFactorChallenge } from '@/api/types'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Field, FieldDescription, FieldError, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Spinner } from '@/components/ui/spinner'
import { isWebAuthnCancelled, webauthnErrorMessage, webauthnSupportHint } from '@/lib/webauthn'
import { TOTP_LENGTH, TotpCodeInput } from './totp-code-input'

type Mode = 'totp' | 'webauthn' | 'recovery'

/**
 * Druhý krok přihlášení (SPEC §3.1) — na stejné stránce jako heslo.
 * Výchozí je kód z aplikace; bezpečnostní klíč se spouští až kliknutím (prohlížeč vyžaduje gesto uživatele).
 */
export function TwoFactorLogin({
  challenge,
  onSuccess,
  onBack,
  onExpired,
}: {
  challenge: TwoFactorChallenge
  onSuccess: (me: Me) => void
  onBack: () => void
  /** Výzva vypršela / příliš mnoho pokusů → zpět na heslo s touto hláškou. */
  onExpired: (message: string) => void
}) {
  const hasTotp = challenge.methods.includes('totp')
  const hasKey = challenge.methods.includes('webauthn')
  const hasRecovery = challenge.methods.includes('recovery')
  const [mode, setMode] = useState<Mode>(hasTotp ? 'totp' : hasKey ? 'webauthn' : 'recovery')
  const [code, setCode] = useState('')
  const [recovery, setRecovery] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [keyNote, setKeyNote] = useState<string | null>(null)
  const [keyError, setKeyError] = useState<string | null>(null)
  const loginCode = useLoginCode()
  const loginKey = useLoginWebAuthn()
  const recoveryId = useId()
  const busy = loginCode.isPending || loginKey.isPending

  const handleError = (err: unknown) => {
    if (hasErrorCode(err, 'two_factor_expired')) {
      onExpired(codeMessages.two_factor_expired!)
      return
    }
    if (hasErrorCode(err, 'invalid_code')) {
      setError(
        mode === 'recovery'
          ? 'Záložní kód není správný nebo už byl použit.'
          : 'Kód není správný. Zadejte aktuální kód z aplikace.',
      )
      return
    }
    setError(errorMessage(err))
  }

  const submitCode = (value: string) => {
    if (busy) return
    setError(null)
    loginCode.mutate(
      { token: challenge.token, code: value.trim() },
      {
        onSuccess,
        onError: (err) => {
          handleError(err)
          if (mode === 'totp') setCode('')
        },
      },
    )
  }

  const runKey = () => {
    if (busy) return
    setError(null)
    setKeyError(null)
    setKeyNote(null)
    loginKey.mutate(challenge.token, {
      onSuccess,
      onError: (err) => {
        if (isApiError(err)) {
          if (hasErrorCode(err, 'two_factor_expired')) onExpired(codeMessages.two_factor_expired!)
          else if (hasErrorCode(err, 'webauthn_failed')) setKeyError('Klíč se nepodařilo ověřit. Použijte klíč přidaný k tomuto účtu.')
          else setKeyError(errorMessage(err))
        } else if (isWebAuthnCancelled(err)) {
          setKeyNote('Ověření bylo přerušeno. Až budete mít klíč připravený, zkuste to znovu.')
        } else {
          setKeyError(webauthnErrorMessage(err))
        }
      },
    })
  }

  const switchMode = (next: Mode) => {
    setMode(next)
    setError(null)
    setKeyError(null)
    setKeyNote(null)
  }

  const onSubmit = (e: FormEvent) => {
    e.preventDefault()
    if (mode === 'totp') {
      if (code.length !== TOTP_LENGTH) setError(`Zadejte ${TOTP_LENGTH}místný kód.`)
      else submitCode(code)
    } else if (mode === 'recovery') {
      if (!recovery.trim()) setError('Zadejte záložní kód.')
      else submitCode(recovery)
    } else {
      runKey()
    }
  }

  const supportHint = hasKey ? webauthnSupportHint() : null
  const keyButton = (primary: boolean) => (
    <Button
      type={primary ? 'submit' : 'button'}
      size="lg"
      variant={primary ? 'default' : 'outline'}
      className="w-full"
      onClick={primary ? undefined : runKey}
      disabled={busy || Boolean(supportHint)}
    >
      {loginKey.isPending ? <Spinner data-icon="inline-start" /> : <KeyRoundIcon data-icon="inline-start" />}
      Použít bezpečnostní klíč
    </Button>
  )

  return (
    <form onSubmit={onSubmit} noValidate className="flex flex-col gap-5">
      {mode === 'totp' && (
        <TotpCodeInput
          value={code}
          onChange={(v) => {
            setCode(v)
            if (error) setError(null)
          }}
          onComplete={submitCode}
          error={error ?? undefined}
          description="Otevřete ověřovací aplikaci v telefonu a opište aktuální kód pro NanoFakturu."
          disabled={loginCode.isPending}
          autoFocus
        />
      )}

      {mode === 'webauthn' && (
        <p className="text-sm text-muted-foreground">
          Připojte bezpečnostní klíč (např. YubiKey) nebo použijte passkey v telefonu či počítači a potvrďte přihlášení.
        </p>
      )}

      {mode === 'recovery' && (
        <Field data-invalid={Boolean(error) || undefined}>
          <FieldLabel htmlFor={recoveryId}>Záložní kód</FieldLabel>
          <Input
            id={recoveryId}
            value={recovery}
            onChange={(e) => {
              setRecovery(e.target.value)
              if (error) setError(null)
            }}
            placeholder="xxxxx-xxxxx"
            autoComplete="off"
            autoCapitalize="none"
            autoCorrect="off"
            spellCheck={false}
            enterKeyHint="go"
            autoFocus
            aria-invalid={Boolean(error) || undefined}
            className="font-mono"
          />
          <FieldDescription>Každý záložní kód jde použít jen jednou.</FieldDescription>
          <FieldError>{error}</FieldError>
        </Field>
      )}

      {keyError && mode !== 'recovery' && (
        <p role="alert" className="text-sm text-destructive">
          {keyError}
        </p>
      )}
      {keyNote && mode !== 'recovery' && <p className="text-sm text-muted-foreground">{keyNote}</p>}
      {supportHint && mode === 'webauthn' && (
        <Alert>
          <AlertDescription>{supportHint}</AlertDescription>
        </Alert>
      )}

      <div className="flex flex-col gap-3">
        {mode === 'totp' && (
          <>
            <Button type="submit" size="lg" className="w-full" disabled={busy}>
              {loginCode.isPending && <Spinner data-icon="inline-start" />}
              Ověřit
            </Button>
            {hasKey && keyButton(false)}
          </>
        )}
        {mode === 'webauthn' && keyButton(true)}
        {mode === 'recovery' && (
          <Button type="submit" size="lg" className="w-full" disabled={busy}>
            {loginCode.isPending && <Spinner data-icon="inline-start" />}
            Ověřit záložní kód
          </Button>
        )}
      </div>

      <div className="flex flex-col items-center gap-1 text-sm">
        {mode !== 'recovery' && hasRecovery && (
          <Button type="button" variant="link" onClick={() => switchMode('recovery')}>
            Použít záložní kód
          </Button>
        )}
        {mode === 'recovery' && hasTotp && (
          <Button type="button" variant="link" onClick={() => switchMode('totp')}>
            Zadat kód z aplikace
          </Button>
        )}
        {mode === 'recovery' && !hasTotp && hasKey && (
          <Button type="button" variant="link" onClick={() => switchMode('webauthn')}>
            Použít bezpečnostní klíč
          </Button>
        )}
        <Button type="button" variant="ghost" onClick={onBack} disabled={busy}>
          <ArrowLeftIcon data-icon="inline-start" />
          Zpět
        </Button>
      </div>
    </form>
  )
}
