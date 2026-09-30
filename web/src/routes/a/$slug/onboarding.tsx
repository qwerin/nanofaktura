import { zodResolver } from '@hookform/resolvers/zod'
import { useIsMutating, useQuery, useQueryClient } from '@tanstack/react-query'
import { createFileRoute, Link, useNavigate } from '@tanstack/react-router'
import {
  BuildingIcon,
  CheckIcon,
  FilePlusIcon,
  LandmarkIcon,
  PartyPopperIcon,
  PencilLineIcon,
  PercentIcon,
} from 'lucide-react'
import { useEffect, useState, type ReactNode } from 'react'
import { useForm, useWatch } from 'react-hook-form'
import { toast } from 'sonner'
import { z } from 'zod'
import { applyProblemToForm, errorMessage } from '@/api/errors'
import { accountQueries, useUpdateAccount } from '@/api/queries/accounts'
import { bankAccountQueries } from '@/api/queries/bank-accounts'
import { keys } from '@/api/queries/keys'
import type { Account, AresSubject, VatMode } from '@/api/types'
import { BankAccountForm } from '@/components/bank-account/bank-account-form'
import { TextField } from '@/components/form/fields'
import { LogoMark } from '@/components/logo'
import { IcoAresField } from '@/components/subject/ico-ares-field'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { Spinner } from '@/components/ui/spinner'
import { formatIban } from '@/lib/bank'
import { formatAddress } from '@/lib/contact'
import { markOnboarded } from '@/lib/onboarding'
import { cn } from '@/lib/utils'
import { optionalIcoSchema } from '@/lib/validation'

export const Route = createFileRoute('/a/$slug/onboarding')({
  loader: ({ context, params }) =>
    Promise.all([
      context.queryClient.ensureQueryData(accountQueries.detail(params.slug)),
      context.queryClient.prefetchQuery(bankAccountQueries.list(params.slug)),
    ]),
  head: () => ({ meta: [{ title: 'Vítejte · NanoFaktura' }] }),
  component: OnboardingPage,
})

const STEPS = [
  { label: 'Firma', icon: BuildingIcon },
  { label: 'DPH', icon: PercentIcon },
  { label: 'Účet', icon: LandmarkIcon },
  { label: 'Hotovo', icon: CheckIcon },
] as const

function OnboardingPage() {
  const { slug } = Route.useParams()
  const navigate = useNavigate()
  const account = useQuery(accountQueries.detail(slug))
  const [step, setStep] = useState(0)

  // Průvodce se nabízí jen jednou — i když ho uživatel zavře v půlce.
  const queryClient = useQueryClient()
  useEffect(() => {
    void markOnboarded(queryClient, slug)
  }, [slug, queryClient])

  const skip = () => void navigate({ to: '/a/$slug/dashboard', params: { slug }, replace: true })
  const next = () => {
    setStep((s) => Math.min(s + 1, STEPS.length - 1))
    window.scrollTo({ top: 0 })
  }

  return (
    <div className="relative flex min-h-dvh flex-col bg-background pt-safe px-safe">
      <div
        aria-hidden="true"
        className="pointer-events-none absolute inset-x-0 top-0 h-72 bg-[radial-gradient(60%_100%_at_50%_0%,color-mix(in_oklch,var(--primary)_12%,transparent),transparent)]"
      />
      <header className="relative mx-auto flex w-full max-w-xl items-center gap-2.5 px-5 pt-4">
        <LogoMark className="size-8" />
        <span className="font-semibold tracking-tight">NanoFaktura</span>
        {step < STEPS.length - 1 && (
          <Button variant="ghost" className="ml-auto text-muted-foreground" onClick={skip}>
            Přeskočit průvodce
          </Button>
        )}
      </header>

      <main className="relative mx-auto flex w-full max-w-xl flex-1 flex-col px-5 pt-6 pb-8">
        <Progress step={step} />
        {!account.data ? (
          <div className="mt-8 flex flex-col gap-4">
            <Skeleton className="h-8 w-2/3" />
            <Skeleton className="h-12 w-full" />
          </div>
        ) : step === 0 ? (
          <CompanyStep slug={slug} account={account.data} onNext={next} />
        ) : step === 1 ? (
          <VatStep slug={slug} account={account.data} onNext={next} onBack={() => setStep(0)} />
        ) : step === 2 ? (
          <BankStep slug={slug} account={account.data} onNext={next} onBack={() => setStep(1)} />
        ) : (
          <DoneStep slug={slug} />
        )}
      </main>
    </div>
  )
}

function Progress({ step }: { step: number }) {
  return (
    <nav aria-label="Postup průvodce">
      <p className="mb-2 text-sm text-muted-foreground">
        Krok {step + 1} ze {STEPS.length}
      </p>
      <ol className="grid grid-cols-4 gap-2">
        {STEPS.map((s, i) => (
          <li key={s.label} aria-current={i === step ? 'step' : undefined} className="flex flex-col gap-1.5">
            <span
              className={cn(
                'h-1.5 rounded-full transition-colors',
                i < step ? 'bg-primary' : i === step ? 'bg-primary/60' : 'bg-muted',
              )}
            />
            <span className={cn('text-xs', i === step ? 'font-medium text-foreground' : 'text-muted-foreground')}>
              {s.label}
            </span>
          </li>
        ))}
      </ol>
    </nav>
  )
}

function StepHeading({ title, children }: { title: string; children?: ReactNode }) {
  return (
    <div className="mt-8 mb-6 flex flex-col gap-2">
      <h1 className="text-2xl font-semibold tracking-tight">{title}</h1>
      {children && <p className="text-muted-foreground">{children}</p>}
    </div>
  )
}

/** Spodní lišta s akcemi kroku — na mobilu přilepená dole, velké cíle. */
function StepActions({ children }: { children: ReactNode }) {
  return (
    <div className="sticky bottom-0 -mx-5 mt-auto flex flex-col-reverse gap-2 border-t bg-background/90 px-5 pt-3 pb-[max(0.75rem,env(safe-area-inset-bottom))] backdrop-blur-md sm:static sm:mx-0 sm:mt-8 sm:flex-row sm:justify-end sm:border-t-0 sm:bg-transparent sm:px-0 sm:backdrop-blur-none [&>*]:h-12 sm:[&>*]:h-9">
      {children}
    </div>
  )
}

// --- Krok 1: firma podle IČO ---

const companySchema = z.object({
  registration_no: optionalIcoSchema,
  name: z.string().trim().min(1, 'Zadejte název firmy nebo své jméno'),
  vat_no: z
    .string()
    .trim()
    .regex(/^([A-Za-z]{2}[0-9A-Za-z]{2,13})?$/, 'DIČ ve tvaru CZ12345678'),
  street: z.string(),
  city: z.string(),
  zip: z.string().trim().max(10, 'Nejvýše 10 znaků'),
  country: z.string(),
})
type CompanyValues = z.infer<typeof companySchema>

function CompanyStep({ slug, account, onNext }: { slug: string; account: Account; onNext: () => void }) {
  const update = useUpdateAccount(slug)
  const [manual, setManual] = useState(Boolean(account.registration_no || account.street))
  const [loaded, setLoaded] = useState<AresSubject | null>(null)
  const form = useForm<CompanyValues>({
    resolver: zodResolver(companySchema),
    defaultValues: {
      registration_no: account.registration_no,
      name: account.name,
      vat_no: account.vat_no,
      street: account.street,
      city: account.city,
      zip: account.zip,
      country: account.country || 'CZ',
    },
  })
  const ico = useWatch({ control: form.control, name: 'registration_no' })

  const onLoaded = (d: AresSubject) => {
    const opts = { shouldDirty: true, shouldValidate: true } as const
    form.setValue('registration_no', d.registration_no, opts)
    form.setValue('name', d.name, opts)
    form.setValue('vat_no', d.vat_no, opts)
    form.setValue('street', d.street, opts)
    form.setValue('city', d.city, opts)
    form.setValue('zip', d.zip, opts)
    form.setValue('country', d.country || 'CZ', opts)
    setLoaded(d)
  }

  const onSubmit = form.handleSubmit(async (v) => {
    const patch = {
      registration_no: v.registration_no.trim(),
      name: v.name.trim(),
      vat_no: v.vat_no.trim().toUpperCase(),
      street: v.street.trim(),
      city: v.city.trim(),
      zip: v.zip.trim(),
      country: (v.country || 'CZ').toUpperCase(),
    }
    const changed = (Object.keys(patch) as (keyof typeof patch)[]).some((k) => patch[k] !== (account[k] ?? ''))
    if (!changed) return onNext()
    try {
      await update.mutateAsync(patch)
      onNext()
    } catch (err) {
      if (!applyProblemToForm(err, form.setError)) toast.error(errorMessage(err))
    }
  })

  const showFields = manual || loaded !== null

  return (
    <form onSubmit={onSubmit} noValidate className="flex flex-1 flex-col">
      <StepHeading title="Vítejte! Začneme vaší firmou">
        Zadejte IČO a údaje pro faktury načteme z ARES. Vše půjde později změnit v Nastavení.
      </StepHeading>

      <fieldset disabled={update.isPending} className="flex flex-col gap-5">
        <IcoAresField control={form.control} name="registration_no" onLoaded={onLoaded} size="lg" autoFocus />

        {loaded && !manual && (
          <div className="rounded-xl border bg-card p-4">
            <p className="font-semibold">{loaded.name}</p>
            <p className="mt-1 text-sm text-muted-foreground">{formatAddress(loaded) || 'Adresa neuvedena'}</p>
            {loaded.vat_no && <p className="mt-1 text-sm text-muted-foreground">DIČ {loaded.vat_no}</p>}
            <Button type="button" variant="link" className="mt-1 h-auto px-0" onClick={() => setManual(true)}>
              <PencilLineIcon data-icon="inline-start" />
              Upravit údaje
            </Button>
          </div>
        )}

        {!showFields && (
          <Button type="button" variant="link" className="self-start px-0" onClick={() => setManual(true)}>
            Nemám IČO / vyplním ručně
          </Button>
        )}

        {manual && (
          <div className="flex flex-col gap-5">
            <TextField control={form.control} name="name" label="Název firmy / jméno" autoComplete="organization" />
            <TextField
              control={form.control}
              name="vat_no"
              label="DIČ"
              description="Jen pokud ho máte."
              autoCapitalize="characters"
              autoComplete="off"
              spellCheck={false}
              placeholder="CZ12345678"
            />
            <TextField control={form.control} name="street" label="Ulice a číslo" autoComplete="street-address" />
            <div className="grid grid-cols-[1fr_7rem] gap-3">
              <TextField control={form.control} name="city" label="Město" autoComplete="address-level2" />
              <TextField
                control={form.control}
                name="zip"
                label="PSČ"
                inputMode="numeric"
                autoComplete="postal-code"
                maxLength={10}
              />
            </div>
          </div>
        )}
      </fieldset>

      <StepActions>
        <Button type="submit" size="lg" disabled={update.isPending || (!showFields && !ico)}>
          {update.isPending && <Spinner data-icon="inline-start" />}
          Pokračovat
        </Button>
        {!showFields && (
          <Button type="button" variant="ghost" size="lg" onClick={onNext}>
            Teď ne
          </Button>
        )}
      </StepActions>
    </form>
  )
}

// --- Krok 2: DPH ---

const vatOptions: { value: VatMode; title: string; text: string }[] = [
  {
    value: 'non_vat_payer',
    title: 'Neplátce DPH',
    text: 'Většina začínajících OSVČ. Na fakturách nebude DPH, jen „Neplátce DPH“.',
  },
  {
    value: 'vat_payer',
    title: 'Plátce DPH',
    text: 'Jste registrovaní k DPH (povinně nad 2 mil. Kč obratu nebo dobrovolně). Faktury budou s DPH a rekapitulací.',
  },
  {
    value: 'identified_person',
    title: 'Identifikovaná osoba',
    text: 'Nakupujete nebo prodáváte služby v EU, ale plátci nejste. Tuzemské faktury jsou bez DPH.',
  },
]

function VatStep({
  slug,
  account,
  onNext,
  onBack,
}: {
  slug: string
  account: Account
  onNext: () => void
  onBack: () => void
}) {
  const update = useUpdateAccount(slug)
  const [mode, setMode] = useState<VatMode>(
    account.vat_mode !== 'non_vat_payer' ? account.vat_mode : account.vat_no ? 'vat_payer' : 'non_vat_payer',
  )

  const save = () => {
    if (mode === account.vat_mode) return onNext()
    update.mutate({ vat_mode: mode }, { onSuccess: onNext, onError: (err) => toast.error(errorMessage(err)) })
  }

  return (
    <div className="flex flex-1 flex-col">
      <StepHeading title="Jste plátce DPH?">
        Podle toho budeme na fakturách počítat daň. Nejste-li si jistí, zvolte neplátce — půjde to změnit.
      </StepHeading>
      <div role="radiogroup" aria-label="Režim DPH" className="flex flex-col gap-3">
        {vatOptions.map((o) => {
          const checked = mode === o.value
          return (
            <button
              key={o.value}
              type="button"
              role="radio"
              aria-checked={checked}
              onClick={() => setMode(o.value)}
              className={cn(
                'flex items-start gap-3 rounded-xl border bg-card p-4 text-left transition-colors hover:bg-muted/50',
                checked && 'border-primary bg-primary/5 ring-1 ring-primary',
              )}
            >
              <span
                className={cn(
                  'mt-0.5 flex size-5 shrink-0 items-center justify-center rounded-full border',
                  checked && 'border-primary bg-primary text-primary-foreground',
                )}
              >
                {checked && <CheckIcon className="size-3.5" />}
              </span>
              <span className="flex flex-col gap-1">
                <span className="font-medium">{o.title}</span>
                <span className="text-sm text-muted-foreground">{o.text}</span>
              </span>
            </button>
          )
        })}
      </div>
      <StepActions>
        <Button size="lg" onClick={save} disabled={update.isPending}>
          {update.isPending && <Spinner data-icon="inline-start" />}
          Pokračovat
        </Button>
        <Button variant="ghost" size="lg" onClick={onBack}>
          Zpět
        </Button>
      </StepActions>
    </div>
  )
}

// --- Krok 3: bankovní účet ---

function BankStep({
  slug,
  account,
  onNext,
  onBack,
}: {
  slug: string
  account: Account
  onNext: () => void
  onBack: () => void
}) {
  const banks = useQuery(bankAccountQueries.list(slug))
  const saving = useIsMutating({ mutationKey: [...keys.bankAccounts(slug), 'save'] }) > 0
  const existing = banks.data ?? []

  return (
    <div className="flex flex-1 flex-col">
      <StepHeading title="Kam vám mají platit?">
        Číslo účtu se vytiskne na fakturu a vygenerujeme z něj QR platbu.
      </StepHeading>
      {banks.isPending ? (
        <Skeleton className="h-40 w-full" />
      ) : existing.length > 0 ? (
        <ul className="flex flex-col gap-2">
          {existing.map((b) => (
            <li key={b.id} className="flex items-center gap-3 rounded-xl border bg-card p-4">
              <LandmarkIcon className="size-5 text-primary" />
              <div className="min-w-0">
                <p className="font-medium">{b.name}</p>
                <p className="font-mono text-sm break-all text-muted-foreground">{b.number || formatIban(b.iban)}</p>
              </div>
            </li>
          ))}
        </ul>
      ) : (
        <BankAccountForm
          slug={slug}
          defaultCurrency={account.default_currency || 'CZK'}
          isFirst
          simple
          formId="onboarding-bank"
          onSaved={onNext}
        />
      )}
      <StepActions>
        {existing.length > 0 ? (
          <Button size="lg" onClick={onNext}>
            Pokračovat
          </Button>
        ) : (
          <Button type="submit" form="onboarding-bank" size="lg" disabled={saving || banks.isPending}>
            {saving && <Spinner data-icon="inline-start" />}
            Uložit a pokračovat
          </Button>
        )}
        <Button variant="ghost" size="lg" onClick={existing.length > 0 ? onBack : onNext}>
          {existing.length > 0 ? 'Zpět' : 'Přidám později'}
        </Button>
      </StepActions>
    </div>
  )
}

// --- Krok 4: hotovo ---

function DoneStep({ slug }: { slug: string }) {
  return (
    <div className="flex flex-1 flex-col">
      <div className="mt-10 flex flex-col items-center gap-4 text-center">
        <span className="flex size-16 items-center justify-center rounded-full bg-success/10 text-success">
          <PartyPopperIcon className="size-8" />
        </span>
        <h1 className="text-2xl font-semibold tracking-tight">Hotovo, můžete fakturovat</h1>
        <p className="max-w-sm text-muted-foreground">
          Údaje firmy, číselné řady a výchozí splatnost doladíte kdykoli v Nastavení.
        </p>
      </div>
      <StepActions>
        <Button size="lg" nativeButton={false} render={<Link to="/a/$slug/invoices/new" params={{ slug }} />}>
          <FilePlusIcon data-icon="inline-start" />
          Vystavit první fakturu
        </Button>
        <Button
          variant="ghost"
          size="lg"
          nativeButton={false}
          render={<Link to="/a/$slug/dashboard" params={{ slug }} />}
        >
          Přejít na přehled
        </Button>
      </StepActions>
    </div>
  )
}
