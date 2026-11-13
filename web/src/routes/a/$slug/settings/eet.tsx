import { zodResolver } from '@hookform/resolvers/zod'
import { useInfiniteQuery, useQuery } from '@tanstack/react-query'
import { createFileRoute, Link } from '@tanstack/react-router'
import {
  CircleAlertIcon,
  CircleCheckIcon,
  ClockIcon,
  FileKeyIcon,
  FlaskConicalIcon,
  LockIcon,
  PlugZapIcon,
  ReceiptTextIcon,
  ShieldCheckIcon,
  Trash2Icon,
  TriangleAlertIcon,
  UploadIcon,
} from 'lucide-react'
import { useId, useRef, useState } from 'react'
import { useForm, useWatch } from 'react-hook-form'
import { toast } from 'sonner'
import { z } from 'zod'
import { applyProblemToForm, errorMessage } from '@/api/errors'
import { accountQueries } from '@/api/queries/accounts'
import {
  eetQueries,
  useDeleteEETCertificate,
  useTestEET,
  useUpdateEETSettings,
  useUploadEETCertificate,
} from '@/api/queries/eet'
import type { EETEnvironment, EETSale, EETSaleStatus, EETSettings, EETTestResult } from '@/api/types'
import { EmptyState } from '@/components/empty-state'
import { EetPok, EetRetryButton, EetSaleFlags, EetStatusBadge } from '@/components/eet/eet-parts'
import { TextField } from '@/components/form/fields'
import { PageError } from '@/components/page-states'
import { ResponsiveDialog } from '@/components/responsive-dialog'
import { ResponsiveList } from '@/components/responsive-list'
import { FormSection, SettingsPage } from '@/components/settings-page'
import { FormSkeleton } from '@/components/skeletons'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Field, FieldContent, FieldDescription, FieldError, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Spinner } from '@/components/ui/spinner'
import { Switch } from '@/components/ui/switch'
import { useCurrentAccount } from '@/hooks/use-current-account'
import { formatDateTime } from '@/lib/date'
import {
  canRetrySale,
  certificateEicMismatch,
  certificateWarning,
  certificateWarningText,
  EET_STATUSES,
  eetStatusLabels,
  eicError,
  failedSummary,
  fileToBase64,
  isPastDeadline,
  pendingSummary,
  registerIdError,
  suggestEic,
  unitIdError,
} from '@/lib/eet'
import { formatMoney } from '@/lib/money'
import { cn } from '@/lib/utils'

const searchSchema = z.object({
  status: z.enum(['pending', 'confirmed', 'failed', 'cancelled']).optional().catch(undefined),
})

export const Route = createFileRoute('/a/$slug/settings/eet')({
  validateSearch: searchSchema,
  head: () => ({ meta: [{ title: 'EET · NanoFaktura' }] }),
  component: EetPage,
})

const environmentItems: { value: EETEnvironment; label: string }[] = [
  { value: 'production', label: 'Ostrý provoz' },
  { value: 'playground', label: 'Testovací prostředí' },
]

/** Je vyplněné vše, co je potřeba k zapnutí a k testu spojení? */
function isConfigured(s: EETSettings): boolean {
  return s.has_certificate && s.eic !== '' && s.unit_id !== '' && s.register_id !== ''
}

function EetPage() {
  const { slug } = Route.useParams()
  const settings = useQuery(eetQueries.settings(slug))

  return (
    <SettingsPage
      title="EET"
      description={
        <>
          Evidence tržeb (EET 2.0, zákon č. 180/2026 Sb.) je povinná od 1. 1. 2027. Evidují se jen{' '}
          <strong className="font-medium text-foreground">kontaktní platby</strong> — přijaté při osobním kontaktu nebo
          v provozovně (hotově, kartou či QR kódem na místě). Úhrada faktury převodem na dálku se neeviduje. Rozhoduje
          skutečný způsob úhrady, ne forma úhrady uvedená na faktuře. Podrobnosti na{' '}
          <a href="https://eet.gov.cz" target="_blank" rel="noreferrer" className="text-primary underline underline-offset-4">
            eet.gov.cz
          </a>
          .
        </>
      }
    >
      {settings.isError ? (
        <PageError error={settings.error} reset={() => void settings.refetch()} />
      ) : settings.data ? (
        <EetSettingsView slug={slug} settings={settings.data} />
      ) : (
        <FormSkeleton fields={5} className="max-w-2xl" />
      )}
    </SettingsPage>
  )
}

function EetSettingsView({ slug, settings }: { slug: string; settings: EETSettings }) {
  const { canManageSettings } = useCurrentAccount()
  const navigate = Route.useNavigate()
  const readOnly = !canManageSettings
  const showStatus = (status: EETSaleStatus) => void navigate({ search: { status }, replace: true })

  return (
    <div className="max-w-5xl">
      {readOnly && (
        <Alert className="mb-6">
          <LockIcon />
          <AlertDescription>Nastavení EET může měnit jen vlastník nebo administrátor účtu.</AlertDescription>
        </Alert>
      )}

      {(settings.failed_count > 0 || settings.pending_count > 0) && (
        <div className="mb-6 flex flex-col gap-3">
          {settings.failed_count > 0 && (
            <Alert variant="destructive">
              <CircleAlertIcon />
              <AlertTitle>{failedSummary(settings.failed_count)}</AlertTitle>
              <AlertDescription>
                <button type="button" className="underline underline-offset-3" onClick={() => showStatus('failed')}>
                  Zobrazit odmítnuté tržby
                </button>
              </AlertDescription>
            </Alert>
          )}
          {settings.pending_count > 0 && (
            <Alert className="border-warning/50 bg-warning/10">
              <ClockIcon />
              <AlertTitle>{pendingSummary(settings.pending_count)}</AlertTitle>
              <AlertDescription>
                Zákonná lhůta pro zaevidování je 48 hodin od tržby.{' '}
                <button type="button" className="underline underline-offset-3" onClick={() => showStatus('pending')}>
                  Zobrazit čekající tržby
                </button>
              </AlertDescription>
            </Alert>
          )}
        </div>
      )}

      <FormSection
        title="Evidence tržeb"
        description="Po zapnutí se kontaktní platby faktur posílají Finanční správě a ta vrátí potvrzovací kód (POK). Účtenku není povinné tisknout."
      >
        <StatusControls slug={slug} settings={settings} readOnly={readOnly} />
      </FormSection>

      <FormSection
        title="Údaje pro evidenci"
        description="V portálu MOJE daně aktivujte evidenci tržeb a založte evidenční jednotku — dostanete její číslo."
      >
        <IdentityForm key={`${settings.eic}|${settings.unit_id}|${settings.register_id}`} slug={slug} settings={settings} readOnly={readOnly} />
      </FormSection>

      <FormSection
        title="Pokladní certifikát"
        description="Soubor .p12 s heslem stáhnete v portálu MOJE daně. Platí jeden rok, pak ho nahraďte novým."
      >
        <CertificateCard slug={slug} settings={settings} readOnly={readOnly} />
      </FormSection>

      {!readOnly && (
        <FormSection
          title="Test spojení"
          description="Pošle Finanční správě ověřovací zprávu. Nic se tím neeviduje."
        >
          <ConnectionTest slug={slug} settings={settings} />
        </FormSection>
      )}

      <FormSection title="Tržby" description="Zaevidované kontaktní platby a jejich storna, nejnovější nahoře.">
        <SalesList slug={slug} />
      </FormSection>
    </div>
  )
}

// --- Zapnutí a prostředí ---

function StatusControls({ slug, settings, readOnly }: { slug: string; settings: EETSettings; readOnly: boolean }) {
  const update = useUpdateEETSettings(slug)
  const ids = { enabled: useId(), env: useId() }
  const configured = isConfigured(settings)
  const disabled = readOnly || update.isPending

  const setEnabled = (enabled: boolean) =>
    update.mutate(
      { enabled },
      {
        onSuccess: () => toast.success(enabled ? 'Evidence tržeb zapnuta' : 'Evidence tržeb vypnuta'),
        onError: (err) => toast.error(errorMessage(err)),
      },
    )
  const setEnvironment = (environment: EETEnvironment) =>
    update.mutate(
      { environment },
      {
        onSuccess: () => toast.success(environment === 'production' ? 'Přepnuto na ostrý provoz' : 'Přepnuto na testovací prostředí'),
        onError: (err) => toast.error(errorMessage(err)),
      },
    )

  return (
    <>
      <Field orientation="horizontal" className="rounded-lg border p-3 md:p-4">
        <FieldContent>
          <FieldLabel htmlFor={ids.enabled}>Evidovat kontaktní platby</FieldLabel>
          <FieldDescription>
            {settings.enabled
              ? 'Zapnuto — u platby faktury zvolíte, jestli jde o kontaktní platbu.'
              : configured
                ? 'Vypnuto — platby se do EET neposílají.'
                : 'Nejdřív vyplňte údaje pro evidenci a nahrajte pokladní certifikát.'}
          </FieldDescription>
        </FieldContent>
        <Switch
          id={ids.enabled}
          checked={settings.enabled}
          onCheckedChange={(v) => setEnabled(v)}
          disabled={disabled || (!settings.enabled && !configured)}
        />
      </Field>

      <Field>
        <FieldLabel htmlFor={ids.env}>Prostředí</FieldLabel>
        <Select
          items={environmentItems}
          value={settings.environment}
          onValueChange={(v) => {
            if (v && v !== settings.environment) setEnvironment(v as EETEnvironment)
          }}
          disabled={disabled}
        >
          <SelectTrigger id={ids.env} className="w-full sm:max-w-xs">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {environmentItems.map((o) => (
              <SelectItem key={o.value} value={o.value}>
                {o.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <FieldDescription>Testovací prostředí Finanční správy slouží jen k vyzkoušení.</FieldDescription>
      </Field>

      {settings.environment === 'playground' && (
        <Alert className="border-warning/50 bg-warning/10">
          <FlaskConicalIcon />
          <AlertTitle>Testovací prostředí — tržby se ve skutečnosti neevidují</AlertTitle>
          <AlertDescription>
            Potvrzení z testovacího prostředí nejsou platná (potvrzovací kód končí „-ff“). Pro splnění zákonné povinnosti
            přepněte na ostrý provoz.
          </AlertDescription>
        </Alert>
      )}
    </>
  )
}

// --- EIČ, evidenční jednotka, pokladna ---

const identitySchema = z.object({
  eic: z.string().superRefine((v, ctx) => {
    const msg = eicError(v.trim().toUpperCase())
    if (msg) ctx.addIssue({ code: 'custom', message: msg })
  }),
  unit_id: z.string().superRefine((v, ctx) => {
    const msg = unitIdError(v.trim())
    if (msg) ctx.addIssue({ code: 'custom', message: msg })
  }),
  register_id: z.string().superRefine((v, ctx) => {
    const msg = registerIdError(v.trim())
    if (msg) ctx.addIssue({ code: 'custom', message: msg })
  }),
})
type IdentityValues = z.infer<typeof identitySchema>

function IdentityForm({ slug, settings, readOnly }: { slug: string; settings: EETSettings; readOnly: boolean }) {
  const account = useQuery(accountQueries.detail(slug))
  const update = useUpdateEETSettings(slug)
  const form = useForm<IdentityValues>({
    resolver: zodResolver(identitySchema),
    // Označení pokladny si volí uživatel sám; výchozí je „1“.
    defaultValues: { eic: settings.eic, unit_id: settings.unit_id, register_id: settings.register_id || '1' },
  })
  const eic = useWatch({ control: form.control, name: 'eic' })
  const suggestion = readOnly ? null : suggestEic(account.data?.vat_no, eic ?? '')

  const onSubmit = form.handleSubmit(async (v) => {
    try {
      await update.mutateAsync({
        eic: v.eic.trim().toUpperCase(),
        unit_id: v.unit_id.trim(),
        register_id: v.register_id.trim(),
      })
      toast.success('Údaje pro evidenci uloženy')
    } catch (err) {
      if (applyProblemToForm(err, form.setError)) toast.error('Zkontrolujte zvýrazněná pole.')
      else toast.error(errorMessage(err))
    }
  })

  return (
    <form onSubmit={onSubmit} noValidate className="flex flex-col gap-5">
      <fieldset disabled={readOnly || update.isPending} className="contents">
        <div className="flex flex-col gap-2">
          <TextField
            control={form.control}
            name="eic"
            label="EIČ"
            placeholder="CZ12345678"
            autoComplete="off"
            autoCapitalize="characters"
            spellCheck={false}
            maxLength={12}
            description="Evidenční identifikační číslo — obvykle vaše DIČ."
          />
          {suggestion && (
            <Button
              type="button"
              variant="outline"
              size="sm"
              className="self-start max-md:h-10"
              onClick={() => form.setValue('eic', suggestion, { shouldDirty: true, shouldValidate: true })}
            >
              Použít DIČ firmy {suggestion}
            </Button>
          )}
        </div>
        <div className="grid gap-5 sm:grid-cols-2">
          <TextField
            control={form.control}
            name="unit_id"
            label="Číslo evidenční jednotky"
            inputMode="numeric"
            autoComplete="off"
            maxLength={9}
            description="Přidělí ho portál MOJE daně při založení evidenční jednotky."
          />
          <TextField
            control={form.control}
            name="register_id"
            label="Označení pokladny"
            autoComplete="off"
            autoCapitalize="none"
            spellCheck={false}
            maxLength={20}
            description="Volíte si sami, např. „1“."
          />
        </div>
        {!readOnly && (
          <Button type="submit" className="w-full sm:w-auto sm:self-start">
            {update.isPending && <Spinner data-icon="inline-start" />}
            Uložit údaje
          </Button>
        )}
      </fieldset>
    </form>
  )
}

// --- Certifikát ---

function CertificateCard({ slug, settings, readOnly }: { slug: string; settings: EETSettings; readOnly: boolean }) {
  const [uploadOpen, setUploadOpen] = useState(false)
  const [removeOpen, setRemoveOpen] = useState(false)
  const remove = useDeleteEETCertificate(slug)
  const warning = settings.has_certificate ? certificateWarning(settings.certificate_valid_to) : null
  const mismatch = settings.has_certificate && certificateEicMismatch(settings.certificate_eic, settings.eic)

  return (
    <>
      <div className="flex flex-col gap-4 rounded-xl border bg-card p-4">
        <div className="flex items-start gap-3">
          <span
            className={cn(
              'flex size-9 shrink-0 items-center justify-center rounded-lg',
              settings.has_certificate ? 'bg-success/12 text-success dark:bg-success/20' : 'bg-muted text-muted-foreground',
            )}
          >
            {settings.has_certificate ? <ShieldCheckIcon className="size-5" /> : <FileKeyIcon className="size-5" />}
          </span>
          {settings.has_certificate ? (
            <dl className="grid min-w-0 flex-1 grid-cols-[auto_minmax(0,1fr)] gap-x-4 gap-y-1 text-sm">
              <dt className="text-muted-foreground">Stav</dt>
              <dd className="font-medium">Certifikát je nahraný</dd>
              {settings.certificate_eic && (
                <>
                  <dt className="text-muted-foreground">Vystaven pro</dt>
                  <dd className="font-mono break-all">{settings.certificate_eic}</dd>
                </>
              )}
              {settings.certificate_valid_to && (
                <>
                  <dt className="text-muted-foreground">Platí do</dt>
                  <dd>{formatDateTime(settings.certificate_valid_to)}</dd>
                </>
              )}
            </dl>
          ) : (
            <div className="min-w-0 flex-1 text-sm">
              <p className="font-medium">Certifikát zatím není nahraný</p>
              <p className="text-muted-foreground">Bez něj nejde evidenci zapnout. Certifikát ukládáme zašifrovaně.</p>
            </div>
          )}
        </div>

        {warning && (
          <Alert variant={warning.level === 'expired' ? 'destructive' : 'default'} className={cn(warning.level === 'expiring' && 'border-warning/50 bg-warning/10')}>
            <TriangleAlertIcon />
            <AlertDescription>{certificateWarningText(warning)}</AlertDescription>
          </Alert>
        )}
        {mismatch && (
          <Alert className="border-warning/50 bg-warning/10">
            <TriangleAlertIcon />
            <AlertDescription>
              Certifikát je vystaven pro EIČ {settings.certificate_eic}, v údajích pro evidenci ale máte {settings.eic}. Finanční
              správa by takové tržby odmítla — opravte EIČ, nebo nahrajte správný certifikát.
            </AlertDescription>
          </Alert>
        )}

        {!readOnly && (
          <div className="grid gap-2 sm:flex">
            <Button variant={settings.has_certificate ? 'outline' : 'default'} onClick={() => setUploadOpen(true)}>
              <UploadIcon data-icon="inline-start" />
              {settings.has_certificate ? 'Nahradit certifikát' : 'Nahrát certifikát'}
            </Button>
            {settings.has_certificate && (
              <Button variant="outline" className="text-destructive hover:text-destructive" onClick={() => setRemoveOpen(true)}>
                <Trash2Icon data-icon="inline-start" />
                Odebrat
              </Button>
            )}
          </div>
        )}
      </div>

      {!readOnly && <CertificateDialog slug={slug} open={uploadOpen} onOpenChange={setUploadOpen} />}

      <ResponsiveDialog
        open={removeOpen}
        onOpenChange={(o) => !remove.isPending && setRemoveOpen(o)}
        title="Odebrat certifikát?"
        description={
          settings.enabled
            ? 'Certifikát se smaže a evidence tržeb se vypne — kontaktní platby se přestanou posílat do EET.'
            : 'Certifikát se smaže. Pro zapnutí evidence ho budete muset nahrát znovu.'
        }
        footer={
          <>
            <Button
              variant="destructive"
              disabled={remove.isPending}
              onClick={() =>
                remove.mutate(undefined, {
                  onSuccess: () => {
                    setRemoveOpen(false)
                    toast.success('Certifikát odebrán')
                  },
                })
              }
            >
              {remove.isPending && <Spinner data-icon="inline-start" />}
              Odebrat certifikát
            </Button>
            <Button variant="outline" disabled={remove.isPending} onClick={() => setRemoveOpen(false)}>
              Zpět
            </Button>
          </>
        }
      />
    </>
  )
}

/** Soubor .p12 je malý (jednotky kB); větší soubor certifikát není. */
const MAX_CERTIFICATE_BYTES = 1024 * 1024

function CertificateDialog({ slug, open, onOpenChange }: { slug: string; open: boolean; onOpenChange: (o: boolean) => void }) {
  const upload = useUploadEETCertificate(slug)
  const ids = { file: useId(), password: useId() }
  const fileInput = useRef<HTMLInputElement>(null)
  const [file, setFile] = useState<File | null>(null)
  const [password, setPassword] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  const close = (o: boolean) => {
    if (busy) return
    if (!o) {
      setFile(null)
      setPassword('')
      setError(null)
    }
    onOpenChange(o)
  }

  const submit = async () => {
    if (!file) {
      setError('Vyberte soubor s certifikátem (.p12).')
      return
    }
    if (file.size === 0 || file.size > MAX_CERTIFICATE_BYTES) {
      setError('Soubor nevypadá jako pokladní certifikát (.p12).')
      return
    }
    setBusy(true)
    setError(null)
    try {
      const pkcs12 = await fileToBase64(file)
      const saved = await upload.mutateAsync({ pkcs12, password })
      toast.success('Certifikát nahrán', {
        description: saved.certificate_valid_to ? `Platí do ${formatDateTime(saved.certificate_valid_to)}` : undefined,
      })
      setFile(null)
      setPassword('')
      onOpenChange(false)
    } catch (err) {
      setError(errorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <ResponsiveDialog
      open={open}
      onOpenChange={close}
      title="Nahrát pokladní certifikát"
      description="Soubor .p12 a heslo k němu z portálu MOJE daně. Soubor se přečte ve vašem prohlížeči a uloží zašifrovaně."
      footer={
        <>
          <Button type="submit" form={`${ids.file}-form`} disabled={busy}>
            {busy && <Spinner data-icon="inline-start" />}
            Nahrát certifikát
          </Button>
          <Button variant="outline" disabled={busy} onClick={() => close(false)}>
            Zrušit
          </Button>
        </>
      }
    >
      <form
        id={`${ids.file}-form`}
        noValidate
        className="flex flex-col gap-4"
        onSubmit={(e) => {
          e.preventDefault()
          e.stopPropagation()
          void submit()
        }}
      >
        <Field>
          <FieldLabel htmlFor={ids.file}>Soubor certifikátu (.p12)</FieldLabel>
          <input
            ref={fileInput}
            id={ids.file}
            type="file"
            accept=".p12,.pfx,application/x-pkcs12"
            className="sr-only"
            onChange={(e) => {
              setFile(e.target.files?.[0] ?? null)
              setError(null)
            }}
          />
          <Button type="button" variant="outline" className="justify-start" onClick={() => fileInput.current?.click()}>
            <FileKeyIcon data-icon="inline-start" />
            <span className="truncate">{file ? file.name : 'Vybrat soubor'}</span>
          </Button>
        </Field>
        <Field>
          <FieldLabel htmlFor={ids.password}>Heslo k certifikátu</FieldLabel>
          <Input
            id={ids.password}
            type="password"
            autoComplete="off"
            value={password}
            onChange={(e) => {
              setPassword(e.target.value)
              setError(null)
            }}
          />
        </Field>
        {error && <FieldError errors={[{ message: error }]} />}
      </form>
    </ResponsiveDialog>
  )
}

// --- Test spojení ---

function ConnectionTest({ slug, settings }: { slug: string; settings: EETSettings }) {
  const test = useTestEET(slug)
  const [result, setResult] = useState<EETTestResult | null>(null)
  const configured = isConfigured(settings)

  return (
    <>
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center">
        <Button
          variant="outline"
          className="w-full sm:w-auto"
          disabled={!configured || test.isPending}
          onClick={() => {
            setResult(null)
            test.mutate(undefined, { onSuccess: setResult, onError: (err) => toast.error(errorMessage(err)) })
          }}
        >
          {test.isPending ? <Spinner data-icon="inline-start" /> : <PlugZapIcon data-icon="inline-start" />}
          Otestovat spojení
        </Button>
        {!configured && (
          <p className="text-sm text-muted-foreground">Nejdřív vyplňte údaje pro evidenci a nahrajte certifikát.</p>
        )}
      </div>
      {result && <TestResult result={result} />}
    </>
  )
}

function TestResult({ result }: { result: EETTestResult }) {
  return (
    <div
      role="status"
      className={cn(
        'flex flex-col gap-2 rounded-xl border p-4 text-sm',
        result.ok ? 'border-success/40 bg-success/5' : 'border-destructive/40 bg-destructive/5',
      )}
    >
      <p className={cn('flex items-center gap-2 font-medium', result.ok ? 'text-success' : 'text-destructive')}>
        {result.ok ? <CircleCheckIcon className="size-4 shrink-0" /> : <CircleAlertIcon className="size-4 shrink-0" />}
        {result.ok ? 'Spojení funguje' : 'Spojení se nepovedlo'}
      </p>
      {result.message && (
        <p className="break-words">
          {result.message}
          {result.code ? <span className="text-muted-foreground"> (kód {result.code})</span> : null}
        </p>
      )}
      <p className="text-xs text-muted-foreground">
        Prostředí: {result.environment === 'playground' ? 'testovací' : 'ostrý provoz'}
      </p>
      {result.warnings.length > 0 && (
        <ul className="flex flex-col gap-1">
          {result.warnings.map((w) => (
            <li key={`${w.code}-${w.text}`} className="flex gap-2 text-warning-foreground dark:text-warning">
              <TriangleAlertIcon className="mt-0.5 size-4 shrink-0" />
              <span className="break-words">
                {w.text} <span className="text-xs opacity-70">(kód {w.code})</span>
              </span>
            </li>
          ))}
        </ul>
      )}
      {result.transaction_id && (
        <p className="text-xs text-muted-foreground">
          ID transakce (pro dotazy na Finanční správu): <span className="font-mono break-all">{result.transaction_id}</span>
        </p>
      )}
    </div>
  )
}

// --- Tržby ---

function SalesList({ slug }: { slug: string }) {
  const { canEdit } = useCurrentAccount()
  const { status } = Route.useSearch()
  const navigate = Route.useNavigate()
  const list = useInfiniteQuery(eetQueries.sales(slug, status ? { status } : {}))
  const items = list.data?.pages.flatMap((p) => p.items ?? [])
  const setStatus = (s: EETSaleStatus | undefined) => void navigate({ search: s ? { status: s } : {}, replace: true })

  return (
    <div className="flex min-w-0 flex-col gap-3">
      <div role="group" aria-label="Stav tržby" className="-mx-4 flex gap-2 overflow-x-auto px-4 pb-1 [scrollbar-width:none] md:mx-0 md:flex-wrap md:px-0">
        <Chip active={!status} onClick={() => setStatus(undefined)}>
          Vše
        </Chip>
        {EET_STATUSES.map((s) => (
          <Chip key={s} active={status === s} onClick={() => setStatus(s)}>
            {eetStatusLabels[s]}
          </Chip>
        ))}
      </div>

      {list.isError ? (
        <PageError error={list.error} reset={() => void list.refetch()} />
      ) : (
        <ResponsiveList
          items={items}
          isLoading={list.isPending}
          getKey={(s) => s.id}
          empty={
            <EmptyState
              icon={ReceiptTextIcon}
              title={status ? 'Žádné tržby v tomto stavu' : 'Zatím žádné tržby'}
              description={
                status
                  ? undefined
                  : 'Tržba vznikne, když u platby faktury označíte, že jde o kontaktní platbu.'
              }
            />
          }
          columns={[
            {
              id: 'status',
              header: 'Stav',
              cell: (s) => (
                <div className="flex flex-wrap items-center gap-1.5">
                  <EetStatusBadge status={s.status} />
                  <EetSaleFlags sale={s} />
                </div>
              ),
            },
            { id: 'invoice', header: 'Doklad', cell: (s) => <InvoiceLink slug={slug} sale={s} /> },
            { id: 'sold_at', header: 'Čas tržby', cell: (s) => <span className="whitespace-nowrap">{formatDateTime(s.sold_at)}</span> },
            {
              id: 'amount',
              header: 'Částka',
              align: 'right',
              cell: (s) => <span className="font-medium whitespace-nowrap">{formatMoney(s.amount, 'CZK')}</span>,
            },
            { id: 'pok', header: 'Potvrzení', className: 'whitespace-normal', cell: (s) => <SaleOutcome sale={s} /> },
            {
              id: 'actions',
              header: <span className="sr-only">Akce</span>,
              align: 'right',
              cell: (s) => (canEdit && canRetrySale(s) ? <EetRetryButton slug={slug} sale={s} /> : null),
            },
          ]}
          renderCard={(s) => (
            <div className="flex flex-col gap-2">
              <div className="flex items-baseline gap-3">
                <span className="min-w-0 flex-1 truncate">
                  <InvoiceLink slug={slug} sale={s} />
                </span>
                <span className="shrink-0 font-semibold tabular-nums">{formatMoney(s.amount, 'CZK')}</span>
              </div>
              <div className="flex flex-wrap items-center gap-1.5">
                <EetStatusBadge status={s.status} />
                <EetSaleFlags sale={s} />
                <span className="flex-1" />
                <span className="text-xs text-muted-foreground">{formatDateTime(s.sold_at)}</span>
              </div>
              <SaleOutcome sale={s} />
              {canEdit && canRetrySale(s) && <EetRetryButton slug={slug} sale={s} className="w-full" />}
            </div>
          )}
          footer={
            list.hasNextPage && (
              <Button variant="outline" onClick={() => void list.fetchNextPage()} disabled={list.isFetchingNextPage} className="w-full md:w-auto">
                {list.isFetchingNextPage && <Spinner data-icon="inline-start" />}
                Načíst další
              </Button>
            )
          }
        />
      )}
    </div>
  )
}

function InvoiceLink({ slug, sale }: { slug: string; sale: EETSale }) {
  return (
    <Link
      to="/a/$slug/invoices/$invoiceId"
      params={{ slug, invoiceId: sale.invoice_id }}
      className="font-medium text-primary underline-offset-4 hover:underline"
    >
      {sale.invoice_number || `Doklad #${sale.invoice_id}`}
    </Link>
  )
}

/** Výsledek odeslání: POK, nebo chyba a lhůta. */
function SaleOutcome({ sale }: { sale: EETSale }) {
  const late = isPastDeadline(sale)
  return (
    <div className="flex min-w-0 flex-col gap-1">
      {sale.pok && <EetPok pok={sale.pok} />}
      {sale.error && sale.status !== 'confirmed' && (
        <p className="text-xs break-words text-destructive">
          {sale.error}
          {sale.error_code ? ` (kód ${sale.error_code})` : ''}
        </p>
      )}
      {canRetrySale(sale) && (
        <p className={cn('text-xs', late ? 'font-medium text-destructive' : 'text-muted-foreground')}>
          {late ? 'Lhůta 48 hodin uplynula ' : 'Zaevidovat do '}
          {formatDateTime(sale.deadline_at)}
          {sale.attempts > 0 && ` · pokusů: ${sale.attempts}`}
        </p>
      )}
      {sale.warnings.length > 0 && (
        <ul className="text-xs text-warning-foreground dark:text-warning">
          {sale.warnings.map((w) => (
            <li key={w} className="break-words">
              {w}
            </li>
          ))}
        </ul>
      )}
      {!sale.pok && !sale.error && !canRetrySale(sale) && sale.warnings.length === 0 && <span className="text-xs text-muted-foreground">—</span>}
    </div>
  )
}

function Chip({ active, onClick, children }: { active: boolean; onClick: () => void; children: string }) {
  return (
    <button
      type="button"
      aria-pressed={active}
      onClick={onClick}
      className={cn(
        'h-11 shrink-0 rounded-full border px-3.5 text-sm font-medium transition-colors focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none md:h-7 md:px-3 md:text-xs',
        active ? 'border-primary bg-primary text-primary-foreground' : 'bg-background text-muted-foreground hover:text-foreground',
      )}
    >
      {children}
    </button>
  )
}
