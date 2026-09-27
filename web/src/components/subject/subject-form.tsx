import { zodResolver } from '@hookform/resolvers/zod'
import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { TriangleAlertIcon } from 'lucide-react'
import { useForm, useWatch } from 'react-hook-form'
import { toast } from 'sonner'
import { z } from 'zod'
import { applyProblemToForm, errorMessage, isApiError } from '@/api/errors'
import { subjectQueries, useCreateSubject, useUpdateSubject } from '@/api/queries/subjects'
import type { AresSubject, CreateSubjectInput, Subject, UpdateSubjectInput } from '@/api/types'
import { SelectField, TextareaField, TextField } from '@/components/form/fields'
import { FormSection } from '@/components/settings-page'
import { StickyActionBar } from '@/components/sticky-action-bar'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'
import { checkCzechAccount, czechAccountErrorMessage, isValidIban, isValidSwift } from '@/lib/bank'
import { isValidIco, optionalEmailSchema, optionalIcoSchema } from '@/lib/validation'
import { IcoAresField } from './ico-ares-field'
import { subjectTypeOptions } from './labels'

const schema = z.object({
  type: z.enum(['customer', 'supplier', 'both']),
  name: z.string().trim().min(1, 'Zadejte název nebo jméno').max(255, 'Nejvýše 255 znaků'),
  full_name: z.string(),
  custom_id: z.string().trim().max(50, 'Nejvýše 50 znaků'),
  registration_no: optionalIcoSchema,
  vat_no: z
    .string()
    .trim()
    .regex(/^([A-Za-z]{2}[0-9A-Za-z]{2,13})?$/, 'DIČ ve tvaru CZ12345678'),
  local_vat_no: z.string().trim(),
  street: z.string(),
  city: z.string(),
  zip: z.string().trim().max(10, 'Nejvýše 10 znaků'),
  country: z.string().trim().regex(/^[A-Za-z]{2}$/, 'Dvoupísmenný kód země, např. CZ'),
  email: optionalEmailSchema,
  email_copy: optionalEmailSchema,
  phone: z.string().trim(),
  web: z.string().trim(),
  bank_account: z
    .string()
    .trim()
    .superRefine((v, ctx) => {
      if (v === '') return
      const r = checkCzechAccount(v)
      if ('error' in r) ctx.addIssue({ code: 'custom', message: czechAccountErrorMessage(r.error) })
    }),
  iban: z
    .string()
    .trim()
    .refine((v) => v === '' || isValidIban(v), 'Neplatný IBAN'),
  swift_bic: z
    .string()
    .trim()
    .refine((v) => v === '' || isValidSwift(v), 'Neplatný SWIFT/BIC (8 nebo 11 znaků)'),
  due_days: z.string().trim().regex(/^\d{0,3}$/, 'Počet dní 0–999'),
  note: z.string(),
})
type Values = z.infer<typeof schema>

const FIELDS = Object.keys(schema.shape)

function toFormValues(s?: Partial<Subject>): Values {
  return {
    type: s?.type ?? 'customer',
    name: s?.name ?? '',
    full_name: s?.full_name ?? '',
    custom_id: s?.custom_id ?? '',
    registration_no: s?.registration_no ?? '',
    vat_no: s?.vat_no ?? '',
    local_vat_no: s?.local_vat_no ?? '',
    street: s?.street ?? '',
    city: s?.city ?? '',
    zip: s?.zip ?? '',
    country: s?.country || 'CZ',
    email: s?.email ?? '',
    email_copy: s?.email_copy ?? '',
    phone: s?.phone ?? '',
    web: s?.web ?? '',
    bank_account: s?.bank_account ?? '',
    iban: s?.iban ?? '',
    swift_bic: s?.swift_bic ?? '',
    due_days: s?.due_days != null ? String(s.due_days) : '',
    note: s?.note ?? '',
  }
}

/** Normalizované hodnoty pro API (bez `due_days`). */
function toBody(v: Values) {
  return {
    type: v.type,
    name: v.name.trim(),
    full_name: v.full_name.trim(),
    custom_id: v.custom_id.trim(),
    registration_no: v.registration_no.trim(),
    vat_no: v.vat_no.trim().toUpperCase(),
    local_vat_no: v.local_vat_no.trim(),
    street: v.street.trim(),
    city: v.city.trim(),
    zip: v.zip.trim(),
    country: v.country.trim().toUpperCase(),
    email: v.email.trim(),
    email_copy: v.email_copy.trim(),
    phone: v.phone.trim(),
    web: v.web.trim(),
    bank_account: v.bank_account.replace(/\s+/g, ''),
    iban: v.iban.replace(/\s+/g, '').toUpperCase(),
    swift_bic: v.swift_bic.trim().toUpperCase(),
    note: v.note,
  }
}

function toCreateBody(v: Values): CreateSubjectInput {
  const body: Record<string, unknown> = {}
  for (const [k, val] of Object.entries(toBody(v))) {
    if (val !== '') body[k] = val
  }
  if (v.due_days.trim() !== '') body.due_days = Number(v.due_days)
  return body as CreateSubjectInput
}

/** PATCH posílá jen změněná pole. Prázdná hodnota pole vymaže. */
function toPatchBody(v: Values, dirty: Partial<Record<keyof Values, unknown>>): UpdateSubjectInput {
  const all = toBody(v)
  const patch: Record<string, unknown> = {}
  for (const key of Object.keys(dirty) as (keyof Values)[]) {
    if (!dirty[key]) continue
    if (key === 'due_days') {
      // Backend zatím neumí due_days vymazat (PATCH bez null) — prázdné pole se neposílá.
      if (v.due_days.trim() !== '') patch.due_days = Number(v.due_days)
    } else {
      patch[key] = all[key]
    }
  }
  return patch as UpdateSubjectInput
}

interface SubjectFormProps {
  slug: string
  /** Úprava existujícího kontaktu; bez něj se zakládá nový. */
  subject?: Subject
  /** Předvyplnění nového kontaktu (např. název z hledání). */
  defaults?: Partial<Subject>
  onSaved: (subject: Subject) => void
  onCancel?: () => void
  /** `sticky` = tlačítka ve spodní liště (stránka); `none` = vlastní tlačítka přes `formId` (dialog). */
  actions?: 'sticky' | 'none'
  formId?: string
}

/**
 * Formulář kontaktu (odběratel / dodavatel). IČO je první — „Načíst z ARES“ doplní název, DIČ a adresu.
 * Použitelný na stránce i v dialogu (`actions="none"` + `formId`).
 */
export function SubjectForm({
  slug,
  subject,
  defaults,
  onSaved,
  onCancel,
  actions = 'sticky',
  formId,
}: SubjectFormProps) {
  const isEdit = Boolean(subject)
  const create = useCreateSubject(slug)
  const update = useUpdateSubject(slug, subject?.id ?? 0)
  const pending = create.isPending || update.isPending

  const form = useForm<Values>({
    resolver: zodResolver(schema),
    defaultValues: toFormValues(subject ?? defaults),
  })
  const { control, formState, setValue } = form
  // Čtení v renderu přihlásí odběr dirtyFields (proxy formState v RHF).
  const { dirtyFields, isDirty } = formState
  const [country, ico] = useWatch({ control, name: ['country', 'registration_no'] })

  // Upozornění na existující kontakt se stejným IČO.
  const icoValid = ico.length === 8 && isValidIco(ico)
  const duplicates = useQuery({
    ...subjectQueries.search(slug, ico, 5),
    enabled: icoValid && ico !== subject?.registration_no,
    select: (items) => items.filter((s) => s.registration_no === ico && s.id !== subject?.id),
  })
  const duplicate = icoValid ? duplicates.data?.[0] : undefined

  const fillFromAres = (d: AresSubject) => {
    const opts = { shouldDirty: true, shouldValidate: true } as const
    setValue('registration_no', d.registration_no, opts)
    if (d.name) setValue('name', d.name, opts)
    setValue('vat_no', d.vat_no, opts)
    if (d.street || d.city) {
      setValue('street', d.street, opts)
      setValue('city', d.city, opts)
      setValue('zip', d.zip, opts)
    }
    if (d.country) setValue('country', d.country, opts)
    toast.success('Údaje doplněny z ARES')
  }

  const onSubmit = form.handleSubmit(async (values) => {
    try {
      let saved: Subject
      if (subject) {
        const patch = toPatchBody(values, dirtyFields)
        saved = Object.keys(patch).length ? await update.mutateAsync(patch) : subject
      } else {
        saved = await create.mutateAsync(toCreateBody(values))
      }
      form.reset(toFormValues(saved))
      toast.success(isEdit ? 'Kontakt uložen' : 'Kontakt vytvořen')
      onSaved(saved)
    } catch (err) {
      if (applyProblemToForm(err, form.setError, FIELDS)) {
        toast.error('Zkontrolujte zvýrazněná pole.')
      } else if (isApiError(err) && err.status === 409) {
        form.setError('custom_id', { type: 'server', message: 'Toto vlastní ID už má jiný kontakt' })
        toast.error('Vlastní ID už používá jiný kontakt.')
      } else {
        toast.error(errorMessage(err))
      }
    }
  })

  return (
    <form id={formId} onSubmit={onSubmit} noValidate className="max-w-4xl">
      <fieldset disabled={pending} className="contents">
        <FormSection title="Základní údaje" description="IČO stačí — zbytek doplníme z ARES.">
          <IcoAresField
            control={control}
            name="registration_no"
            onLoaded={fillFromAres}
            autoFocus={!isEdit && actions === 'none'}
          />
          {duplicate && (
            <Alert>
              <TriangleAlertIcon />
              <AlertDescription>
                Kontakt s tímto IČO už máte:{' '}
                <Link
                  to="/a/$slug/subjects/$subjectId"
                  params={{ slug, subjectId: duplicate.id }}
                  className="font-medium text-foreground underline underline-offset-4"
                >
                  {duplicate.name}
                </Link>
              </AlertDescription>
            </Alert>
          )}
          <TextField control={control} name="name" label="Název firmy / jméno" autoComplete="organization" enterKeyHint="next" />
          <div className="grid gap-5 sm:grid-cols-2">
            <TextField
              control={control}
              name="vat_no"
              label="DIČ"
              autoCapitalize="characters"
              autoComplete="off"
              spellCheck={false}
              placeholder="CZ12345678"
            />
            <SelectField control={control} name="type" label="Typ kontaktu" options={subjectTypeOptions} />
          </div>
          {country.toUpperCase() === 'SK' && (
            <TextField
              control={control}
              name="local_vat_no"
              label="IČ DPH"
              description="Slovenské identifikační číslo pro DPH (SK1234567890)."
              autoCapitalize="characters"
              autoComplete="off"
              spellCheck={false}
            />
          )}
          <div className="grid gap-5 sm:grid-cols-2">
            <TextField control={control} name="full_name" label="Kontaktní osoba" autoComplete="name" />
            <TextField
              control={control}
              name="custom_id"
              label="Vlastní ID"
              description="Vaše interní označení, např. z účetnictví."
              autoComplete="off"
              spellCheck={false}
            />
          </div>
        </FormSection>

        <FormSection title="Adresa" description="Fakturační adresa, která se tiskne na doklad.">
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

        <FormSection title="Kontakt" description="Na e-mail se posílají faktury.">
          <div className="grid gap-5 sm:grid-cols-2">
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
            <TextField
              control={control}
              name="email_copy"
              label="Kopie e-mailu"
              description="Např. účetní odběratele."
              type="email"
              inputMode="email"
              autoComplete="off"
              autoCapitalize="none"
              spellCheck={false}
            />
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

        <FormSection title="Platební údaje" description="Hodí se hlavně u dodavatelů a pro vratky.">
          <TextField
            control={control}
            name="bank_account"
            label="Číslo účtu"
            placeholder="19-2000145399/0800"
            inputMode="text"
            autoComplete="off"
            spellCheck={false}
          />
          <div className="grid gap-5 sm:grid-cols-[1fr_12rem]">
            <TextField
              control={control}
              name="iban"
              label="IBAN"
              autoCapitalize="characters"
              autoComplete="off"
              spellCheck={false}
            />
            <TextField
              control={control}
              name="swift_bic"
              label="SWIFT/BIC"
              autoCapitalize="characters"
              autoComplete="off"
              spellCheck={false}
              maxLength={11}
            />
          </div>
          <TextField
            control={control}
            name="due_days"
            label="Splatnost faktur (dní)"
            description="Prázdné = výchozí splatnost z nastavení firmy."
            inputMode="numeric"
            autoComplete="off"
            maxLength={3}
            className="sm:max-w-56"
          />
        </FormSection>

        <FormSection title="Poznámka" description="Soukromá — na doklady se netiskne.">
          <TextareaField control={control} name="note" label="Poznámka" rows={3} />
        </FormSection>
      </fieldset>

      {actions === 'sticky' && (
        <StickyActionBar start={isDirty ? 'Máte neuložené změny.' : undefined}>
          {onCancel && (
            <Button type="button" variant="outline" onClick={onCancel} disabled={pending}>
              Zrušit
            </Button>
          )}
          <Button type="submit" disabled={pending || (isEdit && !isDirty)}>
            {pending && <Spinner data-icon="inline-start" />}
            {isEdit ? 'Uložit změny' : 'Vytvořit kontakt'}
          </Button>
        </StickyActionBar>
      )}
    </form>
  )
}
