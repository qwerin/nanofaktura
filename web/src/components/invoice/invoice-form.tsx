// Formulář faktury — společný pro novou fakturu i úpravu.

import { zodResolver } from '@hookform/resolvers/zod'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useBlocker, useNavigate } from '@tanstack/react-router'
import { ChevronDownIcon, EllipsisIcon, SendIcon, TriangleAlertIcon } from 'lucide-react'
import { useId, useMemo, useRef, useState, type ReactNode } from 'react'
import { Controller, useForm, useWatch, type Path } from 'react-hook-form'
import { toast } from 'sonner'
import { api, unwrap } from '@/api/client'
import { errorMessage, isApiError } from '@/api/errors'
import { bankAccountQueries } from '@/api/queries/bank-accounts'
import { useCreateInvoice, useIssueInvoice, useUpdateInvoice } from '@/api/queries/invoices'
import { keys } from '@/api/queries/keys'
import type { Account, Invoice } from '@/api/types'
import { SelectField, SwitchField, TextareaField, TextField, type SelectOption } from '@/components/form/fields'
import { ResponsiveDialog } from '@/components/responsive-dialog'
import { StickyActionBar } from '@/components/sticky-action-bar'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { Field, FieldDescription, FieldError, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Spinner } from '@/components/ui/spinner'
import { formatDate, formatDateLong, todayISO } from '@/lib/date'
import { formatMoney } from '@/lib/money'
import { cn } from '@/lib/utils'
import { calculateTotals, chargesNoVat } from './calc'
import {
  apiFieldToFormField,
  computeDueOn,
  CORRECTION_REASONS,
  invoiceFormSchemaFor,
  invoiceToValues,
  newInvoiceValues,
  toCalcLine,
  toCreateBody,
  toPatchBody,
  requiresTaxableDate,
  type FormDocumentType,
  type InvoiceFormValues,
} from './form-model'
import { LinesEditor } from './lines-editor'
import { documentTypeShortLabels, issuedMessage, paymentMethodLabels } from './status'
import { SubjectPicker } from './subject-picker'
import { TagInput } from './tag-input'
import { TotalsPanel } from './totals-panel'

const currencyOptions: SelectOption[] = ['CZK', 'EUR', 'USD', 'GBP', 'PLN'].map((c) => ({ value: c, label: c }))
const paymentOptions: SelectOption[] = (Object.keys(paymentMethodLabels) as (keyof typeof paymentMethodLabels)[]).map(
  (v) => ({ value: v, label: paymentMethodLabels[v] }),
)
const languageOptions: SelectOption[] = [
  { value: 'cs', label: 'Čeština' },
  { value: 'en', label: 'Angličtina' },
  { value: 'sk', label: 'Slovenština' },
  { value: 'de', label: 'Němčina' },
]
const DUE_CHIPS = [7, 14, 30] as const
const supplyTypeOptions: SelectOption[] = [
  { value: 'services', label: 'Služba – daň odvede zákazník' },
  { value: 'goods', label: 'Zboží do EU – osvobozeno dle § 64' },
]

/**
 * Jak formulář uložit: `save` = vystavit novou / uložit změny, `sent` = vystavit a označit jako odeslanou,
 * `draft` = nová jako koncept, `issue` = uložit změny konceptu a vystavit ho.
 */
type SubmitMode = 'save' | 'sent' | 'draft' | 'issue'


interface InvoiceFormProps {
  slug: string
  account: Account
  /** Existující faktura = režim úprav. */
  invoice?: Invoice
  /** Předvolby nové faktury (z URL). */
  preset?: { documentType?: FormDocumentType; subject?: InvoiceFormValues['subject']; relatedId?: number }
}

export function InvoiceForm({ slug, account, invoice, preset }: InvoiceFormProps) {
  const navigate = useNavigate()
  const qc = useQueryClient()
  const isEdit = Boolean(invoice)
  const bankAccounts = useQuery(bankAccountQueries.list(slug))
  const create = useCreateInvoice(slug)
  const update = useUpdateInvoice(slug, invoice?.id ?? 0)
  const issue = useIssueInvoice(slug)
  const pending = create.isPending || update.isPending || issue.isPending
  const [mode, setMode] = useState<SubmitMode>('save')
  const isDraft = invoice?.status === 'draft'
  const allowLeave = useRef(false)

  const vatMode = invoice?.your_vat_mode ?? account.vat_mode
  const payer = vatMode !== 'non_vat_payer'

  const defaults = useMemo<InvoiceFormValues>(() => {
    if (invoice) return invoiceToValues(invoice)
    const v = newInvoiceValues(account, { today: todayISO(), documentType: preset?.documentType })
    if (preset?.subject) v.subject = preset.subject
    if (preset?.relatedId) v.related_id = String(preset.relatedId)
    return v
  }, [invoice, account, preset])

  const schema = useMemo(() => invoiceFormSchemaFor(vatMode), [vatMode])
  const form = useForm<InvoiceFormValues>({ resolver: zodResolver(schema), defaultValues: defaults })
  const { control, formState, setValue } = form

  const [documentType, currency, issuedOn, dueDays, paymentMethod, pricesIncludeVat, roundTotal, reverseCharge, lines, subject] =
    useWatch({
      control,
      name: [
        'document_type',
        'currency',
        'issued_on',
        'due_days',
        'payment_method',
        'prices_include_vat',
        'round_total',
        'reverse_charge',
        'lines',
        'subject',
      ],
    })

  const totals = calculateTotals((lines ?? []).map(toCalcLine), {
    pricesIncludeVat,
    reverseCharge: payer && reverseCharge,
    roundTotal,
    nonVatPayer: chargesNoVat(vatMode, payer && reverseCharge),
  })
  const amounts = (lines ?? []).map((l) => {
    const c = toCalcLine(l)
    return calculateTotals([c], { pricesIncludeVat, reverseCharge: true, roundTotal: false, nonVatPayer: true }).total
  })
  const dueOn = computeDueOn(issuedOn, dueDays)

  // Výchozí bankovní účet měny u nové faktury (když uživatel nic nevybral).
  const accountsForCurrency = (bankAccounts.data ?? []).filter((b) => b.currency === currency)
  const bankItems: SelectOption[] = [
    { value: '', label: accountsForCurrency.length ? 'Výchozí účet měny' : 'Bez bankovního účtu' },
    ...(bankAccounts.data ?? []).map((b) => ({
      value: String(b.id),
      label: `${b.name} · ${b.number || b.iban}${b.currency !== currency ? ` (${b.currency})` : ''}`,
    })),
  ]

  // Neuložené změny: blokace navigace v aplikaci i zavření záložky.
  const blocker = useBlocker({
    shouldBlockFn: () => formState.isDirty && !allowLeave.current,
    enableBeforeUnload: () => formState.isDirty && !allowLeave.current,
    withResolver: true,
  })

  const onIssuedChange = (next: string) => {
    // DUZP drží krok s datem vystavení, dokud ho uživatel nezmění ručně.
    const prev = form.getValues('issued_on')
    if (payer && form.getValues('taxable_fulfillment_due') === prev) {
      setValue('taxable_fulfillment_due', next, { shouldDirty: true })
    }
  }

  const applyServerErrors = (err: unknown): boolean => {
    if (!isApiError(err) || !err.problem.errors?.length) return false
    let mapped = false
    for (const e of err.problem.errors) {
      if (!e.location?.startsWith('body.')) continue
      const path = apiFieldToFormField(e.location.slice(5).replace(/\[(\d+)\]/g, '.$1'))
      form.setError(path as Path<InvoiceFormValues>, { type: 'server', message: e.message ?? 'Neplatná hodnota' })
      mapped = true
    }
    return mapped
  }

  const submit = (submitMode: SubmitMode) =>
    form.handleSubmit(
      async (values) => {
        setMode(submitMode)
        try {
          let saved: Invoice
          let message = 'Změny uloženy'
          if (invoice) {
            const patch = toPatchBody(values, formState.dirtyFields as Record<string, unknown>, payer)
            saved = Object.keys(patch).length ? await update.mutateAsync(patch) : invoice
            if (submitMode === 'issue') {
              try {
                saved = await issue.mutateAsync(saved.id)
                message = issuedMessage(saved)
              } catch (err) {
                toast.error(`Změny jsou uložené, ale doklad se nevystavil: ${errorMessage(err)}`)
                allowLeave.current = true
                void navigate({ to: '/a/$slug/invoices/$invoiceId', params: { slug, invoiceId: saved.id } })
                return
              }
            }
          } else if (submitMode === 'draft') {
            saved = await create.mutateAsync({ ...toCreateBody(values, payer), draft: true })
            message = 'Koncept uložen'
          } else {
            saved = await create.mutateAsync(toCreateBody(values, payer))
            message = issuedMessage(saved)
            if (submitMode === 'sent') {
              try {
                saved = await unwrap(
                  api.POST('/api/accounts/{slug}/invoices/{id}/actions/{action}', {
                    params: { path: { slug, id: saved.id, action: 'mark_as_sent' } },
                  }),
                )
                qc.setQueryData(keys.invoiceDetail(slug, saved.id), saved)
              } catch (err) {
                toast.error(`Faktura je uložená, ale neoznačila se jako odeslaná: ${errorMessage(err)}`)
              }
            }
          }
          toast.success(message)
          if (saved.warnings.some((w) => w.code === 'no_bank_account')) {
            toast.warning(`Pro měnu ${saved.currency} nemáte bankovní účet — doklad je bez platebních údajů a QR.`)
          }
          allowLeave.current = true
          void navigate({ to: '/a/$slug/invoices/$invoiceId', params: { slug, invoiceId: saved.id }, replace: !isEdit })
        } catch (err) {
          if (applyServerErrors(err)) toast.error('Zkontrolujte zvýrazněná pole.')
          else toast.error(errorMessage(err))
        }
      },
      () => toast.error('Zkontrolujte zvýrazněná pole.'),
    )()

  const ids = { subject: useId(), tags: useId(), related: useId(), bank: useId() }
  const docTypes: FormDocumentType[] = ['invoice', 'proforma', 'correction']

  return (
    <form
      noValidate
      onSubmit={(e) => {
        e.preventDefault()
        void submit('save')
      }}
    >
      <div className="flex flex-col gap-4 lg:gap-6">
        <div className="grid gap-4 lg:grid-cols-2 lg:items-start lg:gap-6">
          {/* Typ dokladu + odběratel */}
          <FormCard title="Odběratel">
            {!isEdit ? (
              <Controller
                control={control}
                name="document_type"
                render={({ field }) => (
                  <div role="radiogroup" aria-label="Typ dokladu" className="grid grid-cols-3 gap-1 rounded-lg bg-muted p-1">
                    {docTypes.map((t) => (
                      <button
                        key={t}
                        type="button"
                        role="radio"
                        aria-checked={field.value === t}
                        onClick={() => field.onChange(t)}
                        className={cn(
                          'h-10 rounded-md text-sm font-medium text-muted-foreground transition-colors md:h-8',
                          'focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none',
                          field.value === t ? 'bg-background text-foreground shadow-sm dark:bg-input/40' : 'hover:text-foreground',
                        )}
                      >
                        {documentTypeShortLabels[t]}
                      </button>
                    ))}
                  </div>
                )}
              />
            ) : null}
            <Controller
              control={control}
              name="subject"
              render={({ field, fieldState }) => (
                <Field data-invalid={fieldState.invalid || undefined}>
                  <FieldLabel htmlFor={ids.subject} className="sr-only">
                    Odběratel
                  </FieldLabel>
                  <SubjectPicker
                    id={ids.subject}
                    slug={slug}
                    value={field.value}
                    invalid={fieldState.invalid}
                    onChange={(s) => {
                      field.onChange({ id: s.id, name: s.name, registration_no: s.registration_no, city: s.city })
                      // Splatnost odběratele přebíjí výchozí splatnost účtu (jen u nové faktury).
                      if (!isEdit && s.due_days != null) setValue('due_days', String(s.due_days), { shouldDirty: true })
                      if (form.getValues('related_id')) setValue('related_id', '', { shouldDirty: true })
                    }}
                  />
                  <FieldError errors={[fieldState.error]} />
                </Field>
              )}
            />
            {documentType === 'correction' && (
              <>
                <RelatedInvoiceField slug={slug} subjectId={subject?.id} control={control} id={ids.related} currentId={invoice?.id} />
                <CorrectionReasonField control={control} required={vatMode === 'vat_payer'} />
              </>
            )}
          </FormCard>

          {/* Data */}
          <FormCard title="Údaje dokladu">
            <div className="grid gap-4 sm:grid-cols-2">
              <Controller
                control={control}
                name="issued_on"
                render={({ field, fieldState }) => (
                  <Field data-invalid={fieldState.invalid || undefined}>
                    <FieldLabel htmlFor={`${ids.subject}-issued`}>Datum vystavení</FieldLabel>
                    <Input
                      id={`${ids.subject}-issued`}
                      type="date"
                      {...field}
                      onChange={(e) => {
                        onIssuedChange(e.target.value)
                        field.onChange(e.target.value)
                      }}
                      aria-invalid={fieldState.invalid || undefined}
                    />
                    <FieldError errors={[fieldState.error]} />
                  </Field>
                )}
              />
              {payer && (
                <TextField
                  control={control}
                  name="taxable_fulfillment_due"
                  label="Datum zdanitelného plnění"
                  type="date"
                  description={requiresTaxableDate(vatMode, reverseCharge) ? undefined : 'Nepovinné.'}
                />
              )}
              <Controller
                control={control}
                name="due_days"
                render={({ field, fieldState }) => (
                  <Field data-invalid={fieldState.invalid || undefined} className={cn(!payer && 'sm:col-span-1')}>
                    <FieldLabel htmlFor={`${ids.subject}-due`}>Splatnost (dní)</FieldLabel>
                    <div className="flex gap-2">
                      <Input
                        id={`${ids.subject}-due`}
                        {...field}
                        inputMode="numeric"
                        autoComplete="off"
                        maxLength={3}
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
                            onClick={() => field.onChange(String(d))}
                          >
                            {d}
                          </Button>
                        ))}
                      </div>
                    </div>
                    {dueOn && (
                      <FieldDescription>
                        Splatná <span className="font-medium text-foreground">{formatDateLong(dueOn)}</span>
                      </FieldDescription>
                    )}
                    <FieldError errors={[fieldState.error]} />
                  </Field>
                )}
              />
              <SelectField control={control} name="currency" label="Měna" options={currencyOptions} />
              {currency !== 'CZK' && (
                <TextField control={control} name="exchange_rate" label="Kurz" inputMode="decimal" autoComplete="off" description="Kč za 1 jednotku měny." />
              )}
            </div>
          </FormCard>
        </div>

          {/* Položky */}
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
              disabled={pending}
              slug={slug}
              pricesIncludeVat={pricesIncludeVat}
            />
            <div className="grid gap-4 md:grid-cols-[minmax(0,1fr)_20rem] md:items-start">
              <div className="flex flex-col gap-2">
                {payer && (
                  <SwitchField control={control} name="prices_include_vat" label="Ceny včetně DPH" description="Zadané ceny obsahují DPH." />
                )}
                <SwitchField control={control} name="round_total" label="Zaokrouhlit celkem" description="Na celé jednotky měny." />
                {payer && (
                  <SwitchField
                    control={control}
                    name="reverse_charge"
                    label="Přenesená daňová povinnost"
                    description="DPH odvede odběratel."
                  />
                )}
                {payer && reverseCharge && (
                  <SelectField control={control} name="supply_type" label="Druh plnění" options={supplyTypeOptions} />
                )}
              </div>
              <div className="rounded-lg bg-muted/50 p-4">
                <TotalsPanel totals={totals} currency={currency} showVat={payer} reverseCharge={reverseCharge} />
              </div>
            </div>
          </FormCard>

        <div className="grid gap-4 lg:grid-cols-2 lg:items-start lg:gap-6">
          {/* Platba */}
          <FormCard title="Platba">
            <div className="grid gap-4 sm:grid-cols-2">
              <SelectField control={control} name="payment_method" label="Způsob úhrady" options={paymentOptions} />
              {paymentMethod === 'custom' ? (
                <TextField control={control} name="custom_payment_method" label="Vlastní způsob úhrady" />
              ) : (
                <div className="max-sm:hidden" />
              )}
              {paymentMethod === 'bank' && (
                <Controller
                  control={control}
                  name="bank_account_id"
                  render={({ field, fieldState }) => (
                    <Field data-invalid={fieldState.invalid || undefined} className="sm:col-span-2">
                      <FieldLabel htmlFor={ids.bank}>Bankovní účet</FieldLabel>
                      <Select items={bankItems} value={field.value} onValueChange={(v) => field.onChange(v ?? '')}>
                        <SelectTrigger id={ids.bank} className="w-full" aria-invalid={fieldState.invalid || undefined}>
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
                      {!bankAccounts.isPending && (bankAccounts.data ?? []).length === 0 && (
                        <FieldDescription>Bankovní účty přidáte v Nastavení → Bankovní účty.</FieldDescription>
                      )}
                      <FieldError errors={[fieldState.error]} />
                    </Field>
                  )}
                />
              )}
            </div>
          </FormCard>

          {/* Texty */}
          <FormCard title="Texty na dokladu">
            <TextareaField control={control} name="note" label="Text nad položkami" rows={2} />
            <TextareaField control={control} name="footer_note" label="Patička" rows={2} />
            <MoreOptions>
              <TextareaField control={control} name="private_note" label="Soukromá poznámka" description="Na doklad se netiskne." rows={2} />
              <Controller
                control={control}
                name="tags"
                render={({ field }) => (
                  <Field>
                    <FieldLabel htmlFor={ids.tags}>Štítky</FieldLabel>
                    <TagInput id={ids.tags} value={field.value} onChange={field.onChange} />
                  </Field>
                )}
              />
              <div className="grid gap-4 sm:grid-cols-2">
                <TextField
                  control={control}
                  name="number"
                  label="Číslo dokladu"
                  autoComplete="off"
                  placeholder={isEdit && !isDraft ? undefined : isDraft ? 'Přidělí se při vystavení' : 'Přidělí se automaticky'}
                />
                <TextField
                  control={control}
                  name="variable_symbol"
                  label="Variabilní symbol"
                  inputMode="numeric"
                  maxLength={10}
                  autoComplete="off"
                  placeholder={isEdit && !isDraft ? undefined : 'Podle čísla dokladu'}
                />
                <TextField control={control} name="order_number" label="Číslo objednávky" autoComplete="off" />
                <SelectField control={control} name="language" label="Jazyk dokladu" options={languageOptions} />
              </div>
            </MoreOptions>
          </FormCard>
        </div>

      </div>

      <StickyActionBar
        start={
          <span>
            Celkem <span className="font-semibold text-foreground tabular-nums">{formatMoney(totals.total, currency)}</span>
            {formState.isDirty && <span className="ml-3">· neuložené změny</span>}
          </span>
        }
      >
        {!isEdit && (
          <>
            {/* Mobil: „jako odeslanou“ v menu, ať se vejdou Koncept + Vystavit. */}
            <DropdownMenu>
              <DropdownMenuTrigger
                render={<Button type="button" variant="ghost" size="icon" aria-label="Další možnosti uložení" className="flex-none! md:hidden" />}
                disabled={pending}
              >
                <EllipsisIcon />
              </DropdownMenuTrigger>
              <DropdownMenuContent align="start" side="top" className="w-auto min-w-52">
                <DropdownMenuItem className="min-h-11" onClick={() => void submit('sent')}>
                  <SendIcon />
                  Vystavit a označit jako odeslanou
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
            <Button type="button" variant="outline" disabled={pending} onClick={() => void submit('draft')}>
              {pending && mode === 'draft' && <Spinner data-icon="inline-start" />}
              <span className="md:hidden">Koncept</span>
              <span className="max-md:hidden">Uložit jako koncept</span>
            </Button>
            <Button type="button" variant="outline" disabled={pending} onClick={() => void submit('sent')} className="max-md:hidden">
              {pending && mode === 'sent' && <Spinner data-icon="inline-start" />}
              Vystavit a označit jako odeslanou
            </Button>
          </>
        )}
        {isDraft ? (
          <>
            <Button type="submit" variant="outline" disabled={pending || !formState.isDirty}>
              {pending && mode === 'save' && <Spinner data-icon="inline-start" />}
              <span className="md:hidden">Uložit</span>
              <span className="max-md:hidden">Uložit koncept</span>
            </Button>
            <Button type="button" disabled={pending} onClick={() => void submit('issue')}>
              {pending && mode === 'issue' && <Spinner data-icon="inline-start" />}
              <span className="md:hidden">Vystavit</span>
              <span className="max-md:hidden">Uložit a vystavit</span>
            </Button>
          </>
        ) : (
          <Button type="submit" disabled={pending || (isEdit && !formState.isDirty)}>
            {pending && mode === 'save' && <Spinner data-icon="inline-start" />}
            {isEdit ? 'Uložit změny' : 'Vystavit'}
          </Button>
        )}
      </StickyActionBar>

      <ResponsiveDialog
        open={blocker.status === 'blocked'}
        onOpenChange={(o) => !o && blocker.reset?.()}
        title="Zahodit neuložené změny?"
        description="Pokud odejdete, změny ve faktuře se ztratí."
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

function FormCard({ title, children, className, action }: { title: string; children: ReactNode; className?: string; action?: ReactNode }) {
  return (
    <section className={cn('flex flex-col gap-4 rounded-xl border bg-card p-4 text-card-foreground md:p-5', className)}>
      <div className="flex items-center justify-between gap-2">
        <h2 className="text-base font-semibold tracking-tight">{title}</h2>
        {action}
      </div>
      {children}
    </section>
  )
}

function MoreOptions({ children }: { children: ReactNode }) {
  const [open, setOpen] = useState(false)
  const id = useId()
  return (
    <div className="flex flex-col gap-4">
      <button
        type="button"
        aria-expanded={open}
        aria-controls={id}
        onClick={() => setOpen((o) => !o)}
        className="-mx-1 flex h-11 items-center gap-2 rounded-md px-1 text-sm font-medium text-primary hover:underline md:h-8"
      >
        <ChevronDownIcon className={cn('size-4 transition-transform', open && 'rotate-180')} />
        Další možnosti
        <span className="font-normal text-muted-foreground">— soukromá poznámka, štítky, číslo, VS…</span>
      </button>
      <div id={id} hidden={!open} className="flex flex-col gap-4">
        {children}
      </div>
    </div>
  )
}

/** Výběr opravované faktury pro opravný doklad (faktury vybraného odběratele). */
function RelatedInvoiceField({
  slug,
  subjectId,
  control,
  id,
  currentId,
}: {
  slug: string
  subjectId?: number
  control: ReturnType<typeof useForm<InvoiceFormValues>>['control']
  id: string
  currentId?: number
}) {
  const list = useQuery({
    queryKey: [...keys.invoiceList(slug, { document_type: 'invoice', subject_id: subjectId }), 'related'],
    queryFn: ({ signal }) =>
      unwrap(
        api.GET('/api/accounts/{slug}/invoices', {
          params: { path: { slug }, query: { document_type: 'invoice', subject_id: subjectId, per_page: 100 } },
          signal,
        }),
      ),
    enabled: Boolean(subjectId),
  })
  const items: SelectOption[] = (list.data?.items ?? [])
    .filter((i) => i.id !== currentId)
    // Koncept se opravit nedá — nemá číslo a nikde se nepočítá.
    .filter((i) => i.status !== 'draft')
    .map((i) => ({ value: String(i.id), label: `${i.number} · ${formatDate(i.issued_on)} · ${formatMoney(i.total, i.currency)}` }))

  return (
    <Controller
      control={control}
      name="related_id"
      render={({ field, fieldState }) => (
        <Field data-invalid={fieldState.invalid || undefined}>
          <FieldLabel htmlFor={id}>Opravovaná faktura</FieldLabel>
          <Select items={items} value={field.value || null} onValueChange={(v) => field.onChange(v ?? '')} disabled={!subjectId}>
            <SelectTrigger id={id} className="w-full" aria-invalid={fieldState.invalid || undefined}>
              <SelectValue placeholder={subjectId ? (list.isPending ? 'Načítám…' : 'Vyberte fakturu…') : 'Nejdřív vyberte odběratele'} />
            </SelectTrigger>
            <SelectContent>
              {items.map((o) => (
                <SelectItem key={o.value} value={o.value}>
                  {o.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          {subjectId && !list.isPending && items.length === 0 && (
            <FieldDescription>Tento odběratel nemá žádnou fakturu k opravě.</FieldDescription>
          )}
          <FieldError errors={[fieldState.error]} />
        </Field>
      )}
    />
  )
}

/** Důvod opravy s návrhy — u plátce povinný. */
function CorrectionReasonField({
  control,
  required,
}: {
  control: ReturnType<typeof useForm<InvoiceFormValues>>['control']
  required: boolean
}) {
  return (
    <Controller
      control={control}
      name="correction_reason"
      render={({ field, fieldState }) => (
        <Field data-invalid={fieldState.invalid || undefined}>
          <FieldLabel htmlFor="invoice-correction-reason">Důvod opravy{required ? '' : ' (nepovinné)'}</FieldLabel>
          <Input id="invoice-correction-reason" {...field} autoComplete="off" maxLength={500} aria-invalid={fieldState.invalid || undefined} />
          <div className="flex flex-wrap gap-1">
            {CORRECTION_REASONS.map((r) => (
              <Button key={r} type="button" variant="outline" size="sm" onClick={() => field.onChange(r)}>
                {r}
              </Button>
            ))}
          </div>
          <FieldError errors={[fieldState.error]} />
        </Field>
      )}
    />
  )
}

/** Upozornění, když fakturu nejde upravit. */
export function NotEditableAlert({ reason }: { reason: string }) {
  return (
    <Alert>
      <TriangleAlertIcon />
      <AlertDescription>{reason}</AlertDescription>
    </Alert>
  )
}
