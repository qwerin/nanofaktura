// Formulář šablony faktury — skládá se z dílů formuláře faktury (odběratel, řádky, součty, štítky)
// a přidává panel s datovými placeholdery pro pravidelné faktury.

import { zodResolver } from '@hookform/resolvers/zod'
import { useQuery } from '@tanstack/react-query'
import { useBlocker } from '@tanstack/react-router'
import { BracesIcon } from 'lucide-react'
import { useId, useMemo, useRef, type FormEvent } from 'react'
import { Controller, useForm, useWatch, type Path } from 'react-hook-form'
import { toast } from 'sonner'
import { errorMessage, isApiError } from '@/api/errors'
import { bankAccountQueries } from '@/api/queries/bank-accounts'
import { useCreateTemplate, useUpdateTemplate } from '@/api/queries/templates'
import type { Account, InvoiceTemplate } from '@/api/types'
import { SelectField, SwitchField, TextareaField, TextField, type SelectOption } from '@/components/form/fields'
import { calculateTotals, chargesNoVat } from '@/components/invoice/calc'
import { apiFieldToFormField, toCalcLine } from '@/components/invoice/form-model'
import { LinesEditor } from '@/components/invoice/lines-editor'
import { documentTypeShortLabels, paymentMethodLabels } from '@/components/invoice/status'
import { SubjectPicker } from '@/components/invoice/subject-picker'
import { TagInput } from '@/components/invoice/tag-input'
import { TotalsPanel } from '@/components/invoice/totals-panel'
import { ResponsiveDialog } from '@/components/responsive-dialog'
import { StickyActionBar } from '@/components/sticky-action-bar'
import { Button } from '@/components/ui/button'
import { Field, FieldDescription, FieldError, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Spinner } from '@/components/ui/spinner'
import { formatDate, todayISO } from '@/lib/date'
import { formatMoney } from '@/lib/money'
import { cn } from '@/lib/utils'
import { FormCard } from './form-card'
import { Segmented } from './segmented'
import { DATE_PLACEHOLDERS, dateVars, hasDatePlaceholder, renderDatePlaceholders } from './period'
import { PlaceholderChips } from './placeholder-chips'
import {
  newTemplateValues,
  templateFormSchema,
  templateToValues,
  toTemplateBody,
  type TemplateFormValues,
  type TemplateSubjectValue,
} from './template-model'
import { usePlaceholderInsert } from './use-placeholder-insert'

const currencyOptions: SelectOption[] = ['CZK', 'EUR', 'USD', 'GBP', 'PLN'].map((c) => ({ value: c, label: c }))
const paymentOptions: SelectOption[] = (Object.keys(paymentMethodLabels) as (keyof typeof paymentMethodLabels)[]).map(
  (v) => ({ value: v, label: paymentMethodLabels[v] }),
)
const languageOptions: SelectOption[] = [
  { value: 'cs', label: 'Čeština' },
  { value: 'en', label: 'Angličtina' },
]
const DUE_CHIPS = [7, 14, 30] as const

/** Pole, do kterých lze vkládat placeholdery (backend je renderuje v řádcích a textech). */
const PLACEHOLDER_FIELD = /^(lines\.\d+\.name|note|footer_note|order_number|private_note)$/

interface TemplateFormProps {
  slug: string
  account: Account
  template?: InvoiceTemplate
  /** Odběratel existující šablony nebo předvolba nové. */
  subject?: TemplateSubjectValue
  onSaved: (template: InvoiceTemplate, created: boolean) => void
  readOnly?: boolean
}

export function TemplateForm({ slug, account, template, subject, onSaved, readOnly }: TemplateFormProps) {
  const isEdit = Boolean(template)
  const bankAccounts = useQuery(bankAccountQueries.list(slug))
  const create = useCreateTemplate(slug)
  const update = useUpdateTemplate(slug, template?.id ?? 0)
  const pending = create.isPending || update.isPending
  const allowLeave = useRef(false)
  const vatMode = account.vat_mode
  const payer = vatMode !== 'non_vat_payer'

  const defaults = useMemo<TemplateFormValues>(() => {
    if (template) return templateToValues(template, account, subject ?? null)
    const v = newTemplateValues(account)
    if (subject) v.subject = subject
    return v
  }, [template, account, subject])

  const form = useForm<TemplateFormValues>({ resolver: zodResolver(templateFormSchema), defaultValues: defaults })
  const { control, formState, setValue } = form

  const [documentType, currency, dueDays, paymentMethod, pricesIncludeVat, roundTotal, reverseCharge, lines, language, note, footerNote] =
    useWatch({
      control,
      name: [
        'document_type',
        'currency',
        'due_days',
        'payment_method',
        'prices_include_vat',
        'round_total',
        'reverse_charge',
        'lines',
        'language',
        'note',
        'footer_note',
      ],
    })

  const totals = calculateTotals((lines ?? []).map(toCalcLine), {
    pricesIncludeVat,
    reverseCharge: payer && reverseCharge,
    roundTotal,
    nonVatPayer: chargesNoVat(vatMode, payer && reverseCharge),
  })
  const amounts = (lines ?? []).map(
    (l) => calculateTotals([toCalcLine(l)], { pricesIncludeVat, reverseCharge: true, roundTotal: false, nonVatPayer: true }).total,
  )

  const bankItems: SelectOption[] = [
    { value: '', label: 'Výchozí účet měny' },
    ...(bankAccounts.data ?? []).map((b) => ({
      value: String(b.id),
      label: `${b.name} · ${b.number || b.iban}${b.currency !== currency ? ` (${b.currency})` : ''}`,
    })),
  ]

  const placeholders = usePlaceholderInsert({
    accept: (name) => PLACEHOLDER_FIELD.test(name),
    setValue: (name, value) =>
      setValue(name as Path<TemplateFormValues>, value, { shouldDirty: true, shouldValidate: formState.isSubmitted }),
    fallback: 'lines.0.name',
  })

  const blocker = useBlocker({
    shouldBlockFn: () => formState.isDirty && !allowLeave.current,
    enableBeforeUnload: () => formState.isDirty && !allowLeave.current,
    withResolver: true,
  })

  const applyServerErrors = (err: unknown): boolean => {
    if (!isApiError(err) || !err.problem.errors?.length) return false
    let mapped = false
    for (const e of err.problem.errors) {
      if (!e.location?.startsWith('body.')) continue
      const path = apiFieldToFormField(e.location.slice(5).replace(/\[(\d+)\]/g, '.$1'))
      form.setError(path as Path<TemplateFormValues>, { type: 'server', message: e.message ?? 'Neplatná hodnota' })
      mapped = true
    }
    return mapped
  }

  const onSubmit = (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault()
    void form.handleSubmit(
    async (values) => {
      const body = toTemplateBody(values, payer)
      try {
        const saved = template
          ? await update.mutateAsync({ ...body, bank_account_id: body.bank_account_id ?? 0 })
          : await create.mutateAsync(body)
        toast.success(isEdit ? 'Šablona uložena' : `Šablona „${saved.name}“ vytvořena`)
        allowLeave.current = true
        form.reset(values)
        onSaved(saved, !isEdit)
      } catch (err) {
        if (applyServerErrors(err)) toast.error('Zkontrolujte zvýrazněná pole.')
        else toast.error(errorMessage(err))
      }
    },
    () => toast.error('Zkontrolujte zvýrazněná pole.'),
    )()
  }

  const ids = { subject: useId(), tags: useId(), bank: useId(), due: useId() }
  const panel = (
    <PlaceholderPanel
      lines={(lines ?? []).map((l) => l.name)}
      texts={[
        ['Text nad položkami', note],
        ['Patička', footerNote],
      ]}
      lang={language}
      onInsert={placeholders.insert}
      activeName={placeholders.activeName}
      disabled={readOnly || pending}
    />
  )

  return (
    <form noValidate onSubmit={onSubmit} onFocusCapture={placeholders.onFocusCapture}>
      <fieldset disabled={readOnly} className="contents">
        <div className="flex min-w-0 flex-col gap-4 lg:gap-6">
            <FormCard title="Šablona">
              <TextField
                control={control}
                name="name"
                label="Název šablony"
                autoComplete="off"
                placeholder="Např. Měsíční paušál – ACME"
                enterKeyHint="next"
              />
              <Controller
                control={control}
                name="document_type"
                render={({ field }) => (
                  <Field>
                    <FieldLabel>Vystavit jako</FieldLabel>
                    <Segmented
                      value={field.value}
                      onChange={field.onChange}
                      options={(['invoice', 'proforma'] as const).map((t) => ({ value: t, label: documentTypeShortLabels[t] }))}
                      label="Typ dokladu"
                    />
                  </Field>
                )}
              />
              <Controller
                control={control}
                name="subject"
                render={({ field, fieldState }) => (
                  <Field data-invalid={fieldState.invalid || undefined}>
                    <FieldLabel htmlFor={ids.subject}>Odběratel</FieldLabel>
                    <SubjectPicker
                      id={ids.subject}
                      slug={slug}
                      value={field.value}
                      invalid={fieldState.invalid}
                      disabled={readOnly}
                      onChange={(s) => {
                        field.onChange({ id: s.id, name: s.name, registration_no: s.registration_no, city: s.city })
                        if (s.due_days != null && !form.getValues('due_days')) {
                          setValue('due_days', String(s.due_days), { shouldDirty: true })
                        }
                      }}
                    />
                    <FieldError errors={[fieldState.error]} />
                  </Field>
                )}
              />
            </FormCard>

            <FormCard title="Položky">
              <LinesEditor
                control={control}
                register={form.register}
                setFocus={form.setFocus}
                amounts={amounts}
                currency={currency}
                showVat={payer}
                defaultVat={account.default_vat_rate_bps}
                amountLabel={pricesIncludeVat ? 'Celkem s DPH' : payer ? 'Bez DPH' : 'Celkem'}
                disabled={pending || readOnly}
                slug={slug}
                pricesIncludeVat={pricesIncludeVat}
              />
              {panel}
              <div className="grid gap-4 md:grid-cols-[minmax(0,1fr)_20rem] md:items-start">
                <div className="flex flex-col gap-2">
                  {payer && (
                    <SwitchField control={control} name="prices_include_vat" label="Ceny včetně DPH" description="Zadané ceny obsahují DPH." />
                  )}
                  <SwitchField control={control} name="round_total" label="Zaokrouhlit celkem" description="Na celé jednotky měny." />
                  {payer && (
                    <SwitchField control={control} name="reverse_charge" label="Přenesená daňová povinnost" description="DPH odvede odběratel." />
                  )}
                </div>
                <div className="rounded-lg bg-muted/50 p-4">
                  <TotalsPanel totals={totals} currency={currency} showVat={payer} reverseCharge={reverseCharge} />
                </div>
              </div>
            </FormCard>

            <div className="grid gap-4 lg:grid-cols-2 lg:items-start lg:gap-6">
              <FormCard title="Platba a splatnost">
                <Controller
                  control={control}
                  name="due_days"
                  render={({ field, fieldState }) => (
                    <Field data-invalid={fieldState.invalid || undefined}>
                      <FieldLabel htmlFor={ids.due}>Splatnost (dní)</FieldLabel>
                      <div className="flex gap-2">
                        <Input
                          id={ids.due}
                          {...field}
                          inputMode="numeric"
                          autoComplete="off"
                          maxLength={3}
                          placeholder="—"
                          className="w-20"
                          aria-invalid={fieldState.invalid || undefined}
                        />
                        <div className="flex flex-1 gap-1">
                          {DUE_CHIPS.map((d) => (
                            <Button
                              key={d}
                              type="button"
                              variant={field.value === String(d) ? 'secondary' : 'ghost'}
                              className={cn('flex-1 px-2 tabular-nums', field.value === String(d) && 'ring-1 ring-primary/30')}
                              onClick={() => field.onChange(field.value === String(d) ? '' : String(d))}
                            >
                              {d}
                            </Button>
                          ))}
                        </div>
                      </div>
                      <FieldDescription>
                        {dueDays?.trim() ? 'Od data vystavení faktury.' : `Prázdné = podle odběratele, jinak ${account.default_due_days} dní.`}
                      </FieldDescription>
                      <FieldError errors={[fieldState.error]} />
                    </Field>
                  )}
                />
                <div className="grid gap-4 sm:grid-cols-2">
                  <SelectField control={control} name="currency" label="Měna" options={currencyOptions} />
                  {currency !== 'CZK' ? (
                    <TextField control={control} name="exchange_rate" label="Kurz" inputMode="decimal" autoComplete="off" />
                  ) : (
                    <div className="max-sm:hidden" />
                  )}
                  <SelectField control={control} name="payment_method" label="Způsob úhrady" options={paymentOptions} />
                  {paymentMethod === 'custom' && (
                    <TextField control={control} name="custom_payment_method" label="Vlastní způsob úhrady" />
                  )}
                </div>
                {paymentMethod === 'bank' && (
                  <Controller
                    control={control}
                    name="bank_account_id"
                    render={({ field }) => (
                      <Field>
                        <FieldLabel htmlFor={ids.bank}>Bankovní účet</FieldLabel>
                        <Select items={bankItems} value={field.value} onValueChange={(v) => field.onChange(v ?? '')} disabled={readOnly}>
                          <SelectTrigger id={ids.bank} className="w-full">
                            <SelectValue />
                          </SelectTrigger>
                          <SelectContent>
                            {bankItems.map((o) => (
                              <SelectItem key={o.value} value={o.value}>
                                {o.label}
                              </SelectItem>
                            ))}
                          </SelectContent>
                        </Select>
                      </Field>
                    )}
                  />
                )}
              </FormCard>

              <FormCard title="Texty na dokladu">
                <TextareaField control={control} name="note" label="Text nad položkami" rows={2} />
                <TextareaField control={control} name="footer_note" label="Patička" rows={2} />
                <div className="grid gap-4 sm:grid-cols-2">
                  <TextField control={control} name="order_number" label="Číslo objednávky" autoComplete="off" />
                  <SelectField control={control} name="language" label="Jazyk dokladu" options={languageOptions} />
                </div>
                <TextareaField control={control} name="private_note" label="Soukromá poznámka" description="Na doklad se netiskne." rows={2} />
                <Controller
                  control={control}
                  name="tags"
                  render={({ field }) => (
                    <Field>
                      <FieldLabel htmlFor={ids.tags}>Štítky</FieldLabel>
                      <TagInput id={ids.tags} value={field.value} onChange={field.onChange} disabled={readOnly} />
                    </Field>
                  )}
                />
              </FormCard>
            </div>
        </div>
      </fieldset>

      {!readOnly && (
        <StickyActionBar
          start={
            <span>
              {documentType === 'proforma' ? 'Zálohová faktura' : 'Faktura'} ·{' '}
              <span className="font-semibold text-foreground tabular-nums">{formatMoney(totals.total, currency)}</span>
              {formState.isDirty && <span className="ml-3">· neuložené změny</span>}
            </span>
          }
        >
          <Button type="submit" disabled={pending || (isEdit && !formState.isDirty)}>
            {pending && <Spinner data-icon="inline-start" />}
            {isEdit ? 'Uložit změny' : 'Uložit šablonu'}
          </Button>
        </StickyActionBar>
      )}

      <ResponsiveDialog
        open={blocker.status === 'blocked'}
        onOpenChange={(o) => !o && blocker.reset?.()}
        title="Zahodit neuložené změny?"
        description="Pokud odejdete, změny v šabloně se ztratí."
        footer={
          <>
            <Button variant="destructive" onClick={() => blocker.proceed?.()}>
              Zahodit a odejít
            </Button>
            <Button variant="outline" onClick={() => blocker.reset?.()}>
              Zůstat
            </Button>
          </>
        }
      />
    </form>
  )
}


/** Panel placeholderů: klik vloží token na kurzor, pod tím živá ukázka pro dnešní datum. */
function PlaceholderPanel({
  lines,
  texts,
  lang,
  onInsert,
  activeName,
  disabled,
}: {
  lines: string[]
  texts: [label: string, value: string][]
  lang: 'cs' | 'en'
  onInsert: (token: string) => void
  activeName: string | null
  disabled?: boolean
}) {
  const today = todayISO()
  const vars = dateVars(today, lang) ?? {}
  const examples = [
    ...lines.map((l, i) => [`Položka ${i + 1}`, l] as const),
    ...texts,
  ].filter(([, v]) => v && hasDatePlaceholder(v))
  const target = activeName ? fieldLabel(activeName) : null

  return (
    <section className="flex flex-col gap-3 rounded-xl border border-dashed bg-muted/30 p-4 text-sm" aria-label="Proměnné v textech">
      <div className="flex items-start gap-2">
        <BracesIcon className="mt-0.5 size-4 shrink-0 text-primary" />
        <div className="min-w-0">
          <h3 className="font-semibold">Proměnné podle data vystavení</h3>
          <p className="text-xs text-muted-foreground">
            Klepnutím vložíte na místo kurzoru{target ? ` (${target})` : ' — nejdřív klepněte do názvu položky nebo textu'}.
          </p>
        </div>
      </div>
      <PlaceholderChips
        items={DATE_PLACEHOLDERS.map((p) => ({ ...p, example: vars[p.token.slice(1, -1)] }))}
        onInsert={onInsert}
        disabled={disabled}
      />
      <p className="text-xs text-muted-foreground">Názvy měsíců jsou v 1. pádě: „za měsíc {'{MONTH_NAME}'}“ → „za měsíc {vars.MONTH_NAME}“.</p>
      {examples.length > 0 && (
        <div className="flex flex-col gap-1.5 border-t pt-3">
          <p className="text-xs font-medium text-muted-foreground">Ukázka pro vystavení {formatDate(today)}</p>
          <ul className="flex flex-col gap-1">
            {examples.map(([label, v]) => (
              <li key={label} className="flex flex-col rounded-md bg-background px-2 py-1.5 dark:bg-input/30">
                <span className="text-[0.7rem] text-muted-foreground">{label}</span>
                <span className="break-words whitespace-pre-line">{renderDatePlaceholders(v, today, lang)}</span>
              </li>
            ))}
          </ul>
        </div>
      )}
    </section>
  )
}

function fieldLabel(name: string): string {
  const m = /^lines\.(\d+)\.name$/.exec(name)
  if (m) return `položka ${Number(m[1]) + 1}`
  return (
    { note: 'text nad položkami', footer_note: 'patička', order_number: 'číslo objednávky', private_note: 'soukromá poznámka' }[name] ??
    name
  )
}

