import { useQuery } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import {
  KeyRoundIcon,
  LifeBuoyIcon,
  PlusIcon,
  RefreshCwIcon,
  ShieldAlertIcon,
  ShieldCheckIcon,
  SmartphoneIcon,
  Trash2Icon,
  TriangleAlertIcon,
} from 'lucide-react'
import { useState } from 'react'
import { toast } from 'sonner'
import {
  authQueries,
  useDeleteWebAuthnKey,
  useRegenerateRecoveryCodes,
  useTotpDisable,
} from '@/api/queries/auth'
import type { TwoFactorStatus, WebAuthnKey } from '@/api/types'
import { PageError } from '@/components/page-states'
import { AddKeyDialog } from '@/components/security/add-key-dialog'
import { PasswordConfirmDialog } from '@/components/security/password-confirm-dialog'
import { RecoveryCodesDialog } from '@/components/security/recovery-codes-dialog'
import { TotpSetupDialog } from '@/components/security/totp-setup-dialog'
import { FormSection, SettingsPage } from '@/components/settings-page'
import { FormSkeleton } from '@/components/skeletons'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { useCurrentAccount } from '@/hooks/use-current-account'
import { formatDateTime } from '@/lib/date'
import { webauthnSupportHint } from '@/lib/webauthn'

export const Route = createFileRoute('/a/$slug/settings/security')({
  loader: ({ context }) => context.queryClient.prefetchQuery(authQueries.twoFactor()),
  head: () => ({ meta: [{ title: 'Zabezpečení · NanoFaktura' }] }),
  component: SecurityPage,
})

function SecurityPage() {
  const status = useQuery(authQueries.twoFactor())
  return (
    <SettingsPage
      title="Zabezpečení"
      description="Dvoufázové ověření chrání váš účet, i když někdo zjistí vaše heslo: při přihlášení navíc potvrdíte, že jste to vy — kódem z telefonu nebo bezpečnostním klíčem. Platí pro všechny firmy, ke kterým máte přístup."
    >
      <div className="max-w-4xl">
        {status.isPending ? (
          <FormSkeleton fields={4} />
        ) : status.isError ? (
          <PageError error={status.error} reset={() => void status.refetch()} />
        ) : (
          <SecuritySettings status={status.data} />
        )}
      </div>
    </SettingsPage>
  )
}

function SecuritySettings({ status }: { status: TwoFactorStatus }) {
  const { user } = useCurrentAccount()
  const [codes, setCodes] = useState<string[] | null>(null)
  const showCodes = (c: string[]) => {
    if (c.length > 0) setCodes(c)
  }

  return (
    <>
      <StatusSummary status={status} />
      <TotpSection status={status} onCodes={showCodes} />
      <KeysSection status={status} onCodes={showCodes} />
      {status.enabled && <RecoveryCodesSection status={status} onCodes={showCodes} />}
      <FormSection title="Ztratili jste přístup?" description="Když přijdete o telefon, klíč i záložní kódy.">
        <div className="flex gap-3 text-sm text-muted-foreground">
          <LifeBuoyIcon className="mt-0.5 size-4 shrink-0" />
          <div className="flex min-w-0 flex-col gap-2">
            <p>Obraťte se na správce této instance NanoFaktury. Dvoufázové ověření vám vypne příkazem:</p>
            <code className="block rounded-lg bg-muted px-3 py-2 font-mono text-xs break-all text-foreground">
              nanofaktura user reset-2fa --email {user?.email ?? 'vas@email.cz'}
            </code>
            <p>Pak se přihlásíte jen heslem a ověření si nastavíte znovu.</p>
          </div>
        </div>
      </FormSection>
      <RecoveryCodesDialog codes={codes} email={user?.email} onClose={() => setCodes(null)} />
    </>
  )
}

function StatusSummary({ status }: { status: TwoFactorStatus }) {
  if (status.enabled) {
    const parts = [
      status.totp && 'ověřovací aplikace',
      status.webauthn.length > 0 &&
        `${status.webauthn.length} ${status.webauthn.length === 1 ? 'bezpečnostní klíč' : status.webauthn.length < 5 ? 'bezpečnostní klíče' : 'bezpečnostních klíčů'}`,
    ].filter(Boolean)
    return (
      <Alert className="mb-2 border-success/40">
        <ShieldCheckIcon className="text-success" />
        <AlertTitle>Dvoufázové ověření je zapnuté</AlertTitle>
        <AlertDescription>Přihlášení potvrzujete: {parts.join(' a ')}.</AlertDescription>
      </Alert>
    )
  }
  return (
    <Alert className="mb-2">
      <ShieldAlertIcon />
      <AlertTitle>Dvoufázové ověření je vypnuté</AlertTitle>
      <AlertDescription>
        K přihlášení stačí heslo. Zapněte ověřovací aplikaci nebo přidejte bezpečnostní klíč — zabere to minutu.
      </AlertDescription>
    </Alert>
  )
}

function TotpSection({ status, onCodes }: { status: TwoFactorStatus; onCodes: (codes: string[]) => void }) {
  const [setupOpen, setSetupOpen] = useState(false)
  const [disableOpen, setDisableOpen] = useState(false)
  const disable = useTotpDisable()
  const lastFactor = status.webauthn.length === 0

  return (
    <FormSection
      title="Ověřovací aplikace"
      description="Aplikace v telefonu (např. Google Authenticator, Microsoft Authenticator, Aegis, 1Password) ukazuje každých 30 sekund nový 6místný kód."
    >
      <div className="flex flex-col gap-4 rounded-xl border p-4 sm:flex-row sm:items-center">
        <div className="flex min-w-0 flex-1 items-center gap-3">
          <SmartphoneIcon className="size-5 shrink-0 text-muted-foreground" />
          <div className="flex min-w-0 flex-col gap-0.5">
            <span className="font-medium">Kód z aplikace</span>
            <span className={status.totp ? 'text-sm font-medium text-success' : 'text-sm text-muted-foreground'}>
              {status.totp ? 'Zapnuto' : 'Nenastaveno'}
            </span>
          </div>
        </div>
        {status.totp ? (
          <Button variant="outline" className="max-sm:w-full" onClick={() => setDisableOpen(true)}>
            Vypnout
          </Button>
        ) : (
          <Button className="max-sm:w-full" onClick={() => setSetupOpen(true)}>
            Nastavit
          </Button>
        )}
      </div>

      <TotpSetupDialog open={setupOpen} onOpenChange={setSetupOpen} onEnabled={onCodes} />
      <PasswordConfirmDialog
        open={disableOpen}
        onOpenChange={setDisableOpen}
        title="Vypnout ověřovací aplikaci?"
        description={
          lastFactor
            ? 'Je to váš jediný druhý způsob ověření — dvoufázové ověření se tím úplně vypne a záložní kódy přestanou platit.'
            : 'Přihlašovat se dál můžete bezpečnostním klíčem nebo záložním kódem.'
        }
        confirmLabel="Vypnout"
        destructive
        onConfirm={async (password) => {
          await disable.mutateAsync(password)
          toast.success('Ověřovací aplikace je vypnutá')
        }}
      />
    </FormSection>
  )
}

function KeysSection({ status, onCodes }: { status: TwoFactorStatus; onCodes: (codes: string[]) => void }) {
  const [addOpen, setAddOpen] = useState(false)
  const [deleting, setDeleting] = useState<WebAuthnKey | null>(null)
  const remove = useDeleteWebAuthnKey()
  const hint = webauthnSupportHint()
  const lastFactor = !status.totp && status.webauthn.length === 1

  return (
    <FormSection
      title="Bezpečnostní klíče"
      description="Fyzický klíč do USB nebo přes NFC (např. YubiKey), případně passkey v telefonu nebo počítači. Stačí se ho dotknout."
    >
      {hint && (
        <Alert>
          <TriangleAlertIcon />
          <AlertDescription>{hint}</AlertDescription>
        </Alert>
      )}
      {status.webauthn.length > 0 && (
        <ul className="flex flex-col divide-y rounded-xl border">
          {status.webauthn.map((k) => (
            <li key={k.id} className="flex items-center gap-3 py-2 pr-2 pl-4">
              <KeyRoundIcon className="size-5 shrink-0 text-muted-foreground" />
              <div className="flex min-w-0 flex-1 flex-col gap-0.5 py-1">
                <span className="truncate font-medium">{k.name}</span>
                <span className="text-xs text-muted-foreground">
                  Přidán {formatDateTime(k.created_at)}
                  {' · '}
                  {k.last_used_at ? `použit ${formatDateTime(k.last_used_at)}` : 'zatím nepoužit'}
                </span>
              </div>
              <Button
                variant="ghost"
                size="icon"
                aria-label={`Odebrat klíč ${k.name}`}
                className="text-destructive hover:text-destructive"
                onClick={() => setDeleting(k)}
              >
                <Trash2Icon />
              </Button>
            </li>
          ))}
        </ul>
      )}
      <div>
        <Button
          variant={status.webauthn.length > 0 ? 'outline' : 'default'}
          className="max-sm:w-full"
          onClick={() => setAddOpen(true)}
          disabled={Boolean(hint)}
        >
          <PlusIcon data-icon="inline-start" />
          Přidat klíč
        </Button>
      </div>

      <AddKeyDialog open={addOpen} onOpenChange={setAddOpen} onAdded={onCodes} />
      <PasswordConfirmDialog
        open={deleting !== null}
        onOpenChange={(open) => !open && setDeleting(null)}
        title="Odebrat bezpečnostní klíč?"
        description={
          deleting && (
            <>
              Klíčem <strong className="text-foreground">{deleting.name}</strong> se už nepřihlásíte.
              {lastFactor && ' Je to váš jediný druhý způsob ověření — dvoufázové ověření se tím úplně vypne a záložní kódy přestanou platit.'}
            </>
          )
        }
        confirmLabel="Odebrat klíč"
        destructive
        onConfirm={async (password) => {
          if (!deleting) return
          await remove.mutateAsync({ id: deleting.id, password })
          toast.success('Klíč odebrán')
        }}
      />
    </FormSection>
  )
}

function RecoveryCodesSection({ status, onCodes }: { status: TwoFactorStatus; onCodes: (codes: string[]) => void }) {
  const [open, setOpen] = useState(false)
  const regenerate = useRegenerateRecoveryCodes()
  const left = status.recovery_codes_left
  const low = left <= 3

  return (
    <FormSection
      title="Záložní kódy"
      description="Jednorázové kódy pro případ, že nemáte po ruce telefon ani klíč. Uschovejte je mimo telefon."
    >
      <div className="flex flex-col gap-4 rounded-xl border p-4 sm:flex-row sm:items-center">
        <div className="flex min-w-0 flex-1 flex-col gap-0.5">
          <span className="font-medium">
            {left === 0 ? 'Žádný nepoužitý kód' : `Zbývá ${left} ${left === 1 ? 'kód' : left < 5 ? 'kódy' : 'kódů'}`}
          </span>
          <span className="text-sm text-muted-foreground">Vygenerováním nových přestanou staré platit.</span>
        </div>
        <Button variant={low ? 'default' : 'outline'} className="max-sm:w-full" onClick={() => setOpen(true)}>
          <RefreshCwIcon data-icon="inline-start" />
          Vygenerovat nové
        </Button>
      </div>
      {low && (
        <Alert variant="destructive">
          <TriangleAlertIcon />
          <AlertDescription>
            {left === 0
              ? 'Všechny záložní kódy jsou použité. Vygenerujte si nové, ať máte čím se přihlásit, když ztratíte telefon nebo klíč.'
              : 'Záložních kódů už moc nezbývá. Vygenerujte si nové.'}
          </AlertDescription>
        </Alert>
      )}
      <PasswordConfirmDialog
        open={open}
        onOpenChange={setOpen}
        title="Vygenerovat nové záložní kódy?"
        description="Dosavadní kódy (i nepoužité) přestanou platit."
        confirmLabel="Vygenerovat"
        onConfirm={async (password) => {
          const { recovery_codes } = await regenerate.mutateAsync(password)
          onCodes(recovery_codes ?? [])
        }}
      />
    </FormSection>
  )
}
