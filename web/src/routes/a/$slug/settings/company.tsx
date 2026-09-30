import { zodResolver } from '@hookform/resolvers/zod'
import { useQuery } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import { LockIcon } from 'lucide-react'
import { useEffect } from 'react'
import { useForm, useWatch } from 'react-hook-form'
import { toast } from 'sonner'
import { z } from 'zod'
import { applyProblemToForm, errorMessage } from '@/api/errors'
import { accountQueries, useUpdateAccount } from '@/api/queries/accounts'
import type { Account, UpdateAccountInput } from '@/api/types'
import {
  SelectField,
  SwitchField,
  TextareaField,
  TextField,
  type SelectOption,
} from '@/components/form/fields'
import { PageError } from '@/components/page-states'
import { FormSection, SettingsPage } from '@/components/settings-page'
import { FormSkeleton } from '@/components/skeletons'
import { StickyActionBar } from '@/components/sticky-action-bar'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'
import { useCurrentAccount } from '@/hooks/use-current-account'
import { officeOfBranch, TAX_OFFICES, taxOfficeBranches } from '@/lib/tax-offices'
import { optionalEmailSchema, optionalIcoSchema } from '@/lib/validation'

export const Route = createFileRoute('/a/$slug/settings/company')({
  loader: ({ context, params }) => context.queryClient.prefetchQuery(accountQueries.detail(params.slug)),
  head: () => ({ meta: [{ title: 'Firma · NanoFaktura' }] }),
  component: CompanyPage,
})

// --- Volby ---

const vatModeOptions: SelectOption<Account['vat_mode']>[] = [
  { value: 'non_vat_payer', label: 'Neplátce DPH' },
  { value: 'vat_payer', label: 'Plátce DPH' },
  { value: 'identified_person', label: 'Identifikovaná osoba' },
]
const currencyOptions: SelectOption[] = ['CZK', 'EUR', 'USD', 'GBP', 'PLN'].map((c) => ({ value: c, label: c }))
const paymentMethodOptions: SelectOption[] = [
  { value: 'bank', label: 'Bankovní převod' },
  { value: 'cash', label: 'Hotově' },
  { value: 'card', label: 'Kartou' },
  { value: 'cod', label: 'Dobírka' },
  { value: 'paypal', label: 'PayPal' },
]
const languageOptions: SelectOption<Account['default_language']>[] = [
  { value: 'cs', label: 'Čeština' },
  { value: 'en', label: 'Angličtina' },
  { value: 'sk', label: 'Slovenština' },
  { value: 'de', label: 'Němčina' },
]
const vatRateOptions: SelectOption[] = [
  { value: '2100', label: '21 %' },
  { value: '1200', label: '12 %' },
  { value: '0', label: '0 %' },
]

const vatPeriodOptions: SelectOption<Account['vat_period']>[] = [
  { value: 'month', label: 'Měsíční' },
  { value: 'quarter', label: 'Čtvrtletní' },
]
const taxOfficeOptions: SelectOption[] = TAX_OFFICES.map((o) => ({ value: o.code, label: `${o.name} (${o.code})` }))

// --- Formulář ---

const schema = z.object({
  name: z.string().trim().min(1, 'Zadejte název'),
  registration_no: optionalIcoSchema,
  vat_no: z
    .string()
    .trim()
    .regex(/^([A-Za-z]{2}[0-9A-Za-z]{2,13})?$/, 'DIČ ve tvaru CZ12345678'),
  vat_mode: z.enum(['non_vat_payer', 'vat_payer', 'identified_person']),
  registered_by: z.string(),
  street: z.string(),
  city: z.string(),
  zip: z.string().trim().max(10, 'Nejvýše 10 znaků'),
  country: z.string().trim().regex(/^[A-Za-z]{2}$/, 'Dvoupísmenný kód země, např. CZ'),
  email: optionalEmailSchema,
  phone: z.string(),
  web: z.string(),
  default_currency: z.string().min(1),
  default_due_days: z.string().trim().regex(/^\d{1,3}$/, 'Počet dní 0\u2013999'),
  default_payment_method: z.enum(['bank', 'cash', 'card', 'cod', 'paypal', 'custom']),
  default_language: z.enum(['cs', 'en', 'sk', 'de']),
  default_vat_rate_bps: z.string(),
  round_total: z.boolean(),
  default_note: z.string(),
  default_footer_note: z.string(),
  vat_period: z.enum(['month', 'quarter']),
  c_ufo: z.string().regex(/^\d{0,3}$/),
  c_pracufo: z.string().regex(/^\d{0,4}$/),
})
type Values = z.infer<typeof schema>

function toFormValues(a: Account): Values {
  return {
    name: a.name,
    registration_no: a.registration_no,
    vat_no: a.vat_no,
    vat_mode: a.vat_mode,
    registered_by: a.registered_by,
    street: a.street,
    city: a.city,
    zip: a.zip,
    country: a.country || 'CZ',
    email: a.email,
    phone: a.phone,
    web: a.web,
    default_currency: a.default_currency || 'CZK',
    default_due_days: String(a.default_due_days),
    default_payment_method: a.default_payment_method || 'bank',
    default_language: a.default_language,
    default_vat_rate_bps: String(a.default_vat_rate_bps),
    round_total: a.round_total,
    default_note: a.default_note,
    default_footer_note: a.default_footer_note,
    vat_period: a.vat_period,
    c_ufo: a.c_ufo || (a.c_pracufo ? (officeOfBranch(a.c_pracufo) ?? '') : ''),
    c_pracufo: a.c_pracufo,
  }
}

/** Převod hodnot formuláře na PATCH tělo — posílají se jen změněná pole. */
function toPatch(values: Values, dirty: Partial<Record<keyof Values, unknown>>): UpdateAccountInput {
  // formulář pokrývá jen profil firmy; ostatní nastavení účtu (logo, e-maily…) mají vlastní stránky
  const all: Pick<Required<UpdateAccountInput>, keyof Values> = {
    ...values,
    name: values.name.trim(),
    country: values.country.trim().toUpperCase(),
    vat_no: values.vat_no.trim().toUpperCase(),
    default_due_days: Number(values.default_due_days),
    default_vat_rate_bps: Number(values.default_vat_rate_bps),
  }
  const patch: Record<string, unknown> = {}
  for (const key of Object.keys(dirty) as (keyof Values)[]) {
    if (dirty[key]) patch[key] = all[key]
  }
  return patch as UpdateAccountInput
}

function CompanyPage() {
  const { slug } = Route.useParams()
  const account = useQuery(accountQueries.detail(slug))

  return (
    <SettingsPage
      title="Firma"
      description="Fakturační údaje, které se tisknou na doklady, a výchozí hodnoty nových faktur."
    >
      {account.isError ? (
        <PageError error={account.error} reset={() => void account.refetch()} />
      ) : account.data ? (
        <CompanyForm key={slug} slug={slug} account={account.data} />
      ) : (
        <FormSkeleton fields={8} className="max-w-2xl" />
      )}
    </SettingsPage>
  )
}

function CompanyForm({ slug, account }: { slug: string; account: Account }) {
  const { canManageSettings } = useCurrentAccount()
  const update = useUpdateAccount(slug)
  const form = useForm<Values>({
    resolver: zodResolver(schema),
    // `values` drží formulář v synchronizaci s cache; rozeditovaná pole se při refetchi nepřepíšou.
    values: toFormValues(account),
    resetOptions: { keepDirtyValues: true },
  })
  const { control, formState } = form
  const vatMode = useWatch({ control: form.control, name: 'vat_mode' })
  const taxOffice = useWatch({ control: form.control, name: 'c_ufo' })
  // Pracoviště musí patřit ke zvolenému úřadu — po změně úřadu se vymaže.
  useEffect(() => {
    const branch = form.getValues('c_pracufo')
    if (branch && !taxOfficeBranches(taxOffice).some((b) => b.code === branch)) {
      form.setValue('c_pracufo', '', { shouldDirty: true })
    }
  }, [taxOffice, form])
  const branchOptions: SelectOption[] = [
    { value: '', label: 'Neuvádět' },
    ...taxOfficeBranches(taxOffice).map((b) => ({ value: b.code, label: `${b.name} (${b.code})` })),
  ]

  const onSubmit = form.handleSubmit(async (values) => {
    const patch = toPatch(values, formState.dirtyFields)
    if (Object.keys(patch).length === 0) return
    try {
      const saved = await update.mutateAsync(patch)
      form.reset(toFormValues(saved))
      toast.success('Údaje firmy uloženy')
    } catch (err) {
      if (applyProblemToForm(err, form.setError)) {
        toast.error('Zkontrolujte zvýrazněná pole.')
      } else {
        toast.error(errorMessage(err))
      }
    }
  })

  return (
    <form onSubmit={onSubmit} noValidate className="max-w-4xl">
      {!canManageSettings && (
        <Alert className="mb-6">
          <LockIcon />
          <AlertDescription>Údaje firmy může měnit jen vlastník nebo administrátor účtu.</AlertDescription>
        </Alert>
      )}

      <fieldset disabled={!canManageSettings || update.isPending} className="contents">
        <FormSection title="Základní údaje" description="Jak se firma zobrazuje na fakturách.">
          <TextField control={control} name="name" label="Název firmy / jméno" autoComplete="organization" />
          <div className="grid gap-5 sm:grid-cols-2">
            <TextField
              control={control}
              name="registration_no"
              label="IČO"
              inputMode="numeric"
              maxLength={8}
              autoComplete="off"
              placeholder="12345678"
            />
            <TextField
              control={control}
              name="vat_no"
              label="DIČ"
              autoCapitalize="characters"
              autoComplete="off"
              spellCheck={false}
              placeholder="CZ12345678"
            />
          </div>
          <SelectField
            control={control}
            name="vat_mode"
            label="Režim DPH"
            options={vatModeOptions}
            description={
              vatMode === 'non_vat_payer'
                ? 'Na fakturách se nebude počítat DPH a zobrazí se „Neplátce DPH“.'
                : vatMode === 'identified_person'
                  ? 'DPH se na fakturách za tuzemská plnění nepočítá.'
                  : 'Na fakturách se počítá DPH a zobrazuje rekapitulace.'
            }
          />
          <TextareaField
            control={control}
            name="registered_by"
            label="Zápis v rejstříku"
            rows={2}
            placeholder="Zapsán v živnostenském rejstříku…"
          />
        </FormSection>

        {vatMode === 'vat_payer' && (
          <FormSection
            title="Daně a DPH"
            description="Pro přehled DPH a export přiznání a kontrolního hlášení do EPO."
          >
            <SelectField
              control={control}
              name="vat_period"
              label="Zdaňovací období"
              options={vatPeriodOptions}
              description="Čtvrtletně smí podávat jen plátci s obratem do 10 mil. Kč (ne v prvním roce registrace)."
              className="sm:max-w-60"
            />
            <SelectField
              control={control}
              name="c_ufo"
              label="Finanční úřad"
              options={taxOfficeOptions}
              placeholder="Vyberte finanční úřad…"
              description="Krajský úřad podle sídla (u fyzické osoby bydliště). Kód c_ufo z číselníku EPO."
            />
            <SelectField
              control={control}
              name="c_pracufo"
              label="Územní pracoviště"
              options={branchOptions}
              placeholder={taxOffice ? 'Neuvádět' : 'Nejdřív vyberte finanční úřad'}
              disabled={!taxOffice}
              description="Nepovinné — rozhoduje finanční úřad. Kód c_pracufo."
            />
          </FormSection>
        )}

        <FormSection title="Adresa" description="Sídlo nebo místo podnikání.">
          <TextField control={control} name="street" label="Ulice a číslo" autoComplete="street-address" />
          <div className="grid gap-5 sm:grid-cols-[1fr_8rem]">
            <TextField control={control} name="city" label="Město" autoComplete="address-level2" />
            <TextField
              control={control}
              name="zip"
              label="PSČ"
              inputMode="numeric"
              autoComplete="postal-code"
              maxLength={10}
            />
          </div>
          <TextField
            control={control}
            name="country"
            label="Země"
            description="Kód země ISO (CZ, SK, DE…)."
            autoCapitalize="characters"
            autoComplete="country"
            maxLength={2}
            className="sm:max-w-40"
          />
        </FormSection>

        <FormSection title="Kontakt" description="Zobrazí se v hlavičce faktury.">
          <TextField
            control={control}
            name="email"
            label="E-mail"
            type="email"
            inputMode="email"
            autoComplete="email"
            autoCapitalize="none"
            spellCheck={false}
          />
          <div className="grid gap-5 sm:grid-cols-2">
            <TextField control={control} name="phone" label="Telefon" type="tel" inputMode="tel" autoComplete="tel" />
            <TextField
              control={control}
              name="web"
              label="Web"
              type="url"
              inputMode="url"
              autoComplete="url"
              autoCapitalize="none"
              spellCheck={false}
              placeholder="https://"
            />
          </div>
        </FormSection>

        <FormSection title="Výchozí hodnoty faktur" description="Předvyplní se u každé nové faktury, lze je tam změnit.">
          <div className="grid gap-5 sm:grid-cols-2">
            <SelectField control={control} name="default_currency" label="Měna" options={currencyOptions} />
            <TextField
              control={control}
              name="default_due_days"
              label="Splatnost (dní)"
              inputMode="numeric"
              maxLength={3}
              autoComplete="off"
            />
            <SelectField
              control={control}
              name="default_payment_method"
              label="Způsob úhrady"
              options={paymentMethodOptions}
            />
            <SelectField control={control} name="default_language" label="Jazyk dokladu" options={languageOptions} />
            {vatMode === 'vat_payer' && (
              <SelectField
                control={control}
                name="default_vat_rate_bps"
                label="Výchozí sazba DPH"
                options={vatRateOptions}
              />
            )}
          </div>
          <SwitchField
            control={control}
            name="round_total"
            label="Zaokrouhlovat celkovou částku"
            description="Částka k úhradě se zaokrouhlí na celé koruny."
          />
        </FormSection>

        <FormSection title="Texty na dokladech">
          <TextareaField
            control={control}
            name="default_note"
            label="Text nad položkami"
            rows={3}
            placeholder="Fakturujeme vám za dodané služby:"
          />
          <TextareaField
            control={control}
            name="default_footer_note"
            label="Patička"
            rows={3}
            placeholder="Děkujeme za spolupráci."
          />
        </FormSection>
      </fieldset>

      {canManageSettings && (
        <StickyActionBar start={formState.isDirty ? 'Máte neuložené změny.' : 'Vše uloženo.'}>
          {formState.isDirty && (
            <Button type="button" variant="outline" onClick={() => form.reset()} disabled={update.isPending}>
              Zahodit
            </Button>
          )}
          <Button type="submit" disabled={!formState.isDirty || update.isPending}>
            {update.isPending && <Spinner data-icon="inline-start" />}
            Uložit změny
          </Button>
        </StickyActionBar>
      )}
    </form>
  )
}
