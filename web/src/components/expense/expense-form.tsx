import { zodResolver } from '@hookform/resolvers/zod'
import { useState, type ReactNode } from 'react'
import { Controller, useForm, useWatch, type UseFormReturn } from 'react-hook-form'
import { toast } from 'sonner'
import { applyProblemToForm, errorMessage } from '@/api/errors'
import type { Account } from '@/api/types'
import { SelectField, SwitchField, TextareaField, TextField } from '@/components/form/fields'
import { StickyActionBar } from '@/components/sticky-action-bar'
import { Button } from '@/components/ui/button'
import { Field, FieldError, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Spinner } from '@/components/ui/spinner'
import { addDays } from '@/lib/date'
import { formatMoney } from '@/lib/money'
import { cn } from '@/lib/utils'
import { draftTotals } from './calc'
import { CategoryField } from './category-field'
import { expenseFormSchema, type ExpenseFormValues } from './expense-form-schema'
import { currencyOptions, paymentMethodOptions } from './format'
import { LineEditor, TotalsSummary } from './line-editor'
import { SupplierFields } from './supplier-fields'
import { UnsavedChangesGuard } from './unsaved-changes-guard'

const FORM_FIELDS = Object.keys(expenseFormSchema.shape)
const supplyTypeOptions = [
  { value: 'services', label: 'Služba' },
  { value: 'goods', label: 'Zboží' },
]

interface ExpenseFormProps {
  mode: 'create' | 'edit'
  defaultValues: ExpenseFormValues
  account: Account | undefined
  /** Uloží náklad; chyba (ApiError) se promítne do polí. Po úspěchu stránka naviguje sama. */
  onSubmit: (values: ExpenseFormValues) => Promise<void>
  onCancel: () => void
  /** Obsah nad formulářem (např. focení účtenky u nového nákladu). */
  top?: ReactNode
  submitLabel?: string
}

export function ExpenseForm({ mode, defaultValues, account, onSubmit, onCancel, top, submitLabel = 'Uložit' }: ExpenseFormProps) {
  const form = useForm<ExpenseFormValues>({
    resolver: zodResolver(expenseFormSchema),
    defaultValues,
    mode: 'onTouched',
  })
  const [saving, setSaving] = useState(false)
  const currency = useWatch({ control: form.control, name: 'currency' })
  const lines = useWatch({ control: form.control, name: 'lines' })
  const pricesIncludeVat = useWatch({ control: form.control, name: 'prices_include_vat' })
  const roundTotal = useWatch({ control: form.control, name: 'round_total' })
  const reverseCharge = useWatch({ control: form.control, name: 'reverse_charge' })
  const totals = draftTotals(lines ?? [], { pricesIncludeVat, roundTotal, reverseCharge })
  const defaultCurrency = account?.default_currency || 'CZK'
  const vatPayer = account?.vat_mode === 'vat_payer'
  // Samovyměření DPH se týká plátců i identifikovaných osob.
  const canReverseCharge = account !== undefined && account.vat_mode !== 'non_vat_payer'

  const submit = form.handleSubmit(
    async (values) => {
      setSaving(true)
      try {
        await onSubmit(values)
      } catch (err) {
        if (applyProblemToForm(err, form.setError, FORM_FIELDS)) toast.error('Zkontrolujte zvýrazněná pole.')
        else toast.error(errorMessage(err))
      } finally {
        setSaving(false)
      }
    },
    () => toast.error('Zkontrolujte zvýrazněná pole.'),
  )

  return (
    <form onSubmit={submit} noValidate className="flex flex-col">
      <UnsavedChangesGuard when={form.formState.isDirty && !saving} />

      {top}

      <Section title="Dodavatel" description="Od koho je doklad a jak ho dohledáte.">
        <SupplierFields form={form} disabled={saving} />
        <div className="grid gap-5 sm:grid-cols-2">
          <TextField control={form.control} name="original_number" label="Číslo dokladu dodavatele" autoComplete="off" enterKeyHint="next" placeholder="např. FV2026-0142" disabled={saving} />
          <TextField control={form.control} name="variable_symbol" label="Variabilní symbol" inputMode="numeric" autoComplete="off" enterKeyHint="next" maxLength={10} description="Když ho nevyplníte, vezme se z čísla dokladu." disabled={saving} />
        </div>
      </Section>

      <Section title="Údaje dokladu" description="Data, kategorie a platba.">
        <DateFields form={form} defaultDueDays={account?.default_due_days ?? 14} disabled={saving} />
        <div className="grid gap-5 sm:grid-cols-2">
          <CategoryField control={form.control} name="category" disabled={saving} />
          <SelectField control={form.control} name="payment_method" label="Způsob úhrady" options={paymentMethodOptions} disabled={saving} />
        </div>
        <TextField control={form.control} name="description" label="Popis" autoComplete="off" placeholder="Stručně, za co je náklad" disabled={saving} />
        <div className="grid gap-5 sm:grid-cols-2">
          <SelectField control={form.control} name="currency" label="Měna" options={currencyOptions(currency)} disabled={saving} />
          {currency !== defaultCurrency && (
            <TextField
              control={form.control}
              name="exchange_rate"
              label={`Kurz (${defaultCurrency} za 1 ${currency})`}
              inputMode="decimal"
              autoComplete="off"
              placeholder="např. 24,355"
              disabled={saving}
            />
          )}
        </div>
      </Section>

      <section className="flex flex-col gap-4 border-b py-6 md:py-8">
        <div className="flex flex-col gap-1">
          <h2 className="text-base font-semibold tracking-tight">Položky</h2>
          <p className="text-sm text-muted-foreground">Ceny opište z dokladu. Položka z ceníku se skladem naskladní zboží.</p>
        </div>
        <div className="grid gap-3 sm:grid-cols-2">
          <SwitchField control={form.control} name="prices_include_vat" label="Ceny včetně DPH" description="Jak jsou ceny uvedené na dokladu." disabled={saving} />
          <SwitchField control={form.control} name="round_total" label="Zaokrouhlit celkovou částku" description="Na celé koruny, typicky u plateb v hotovosti." disabled={saving} />
        </div>
        {(canReverseCharge || reverseCharge) && (
          <SwitchField
            control={form.control}
            name="reverse_charge"
            label="Daň přiznávám já jako odběratel (přenesená daňová povinnost)"
            description="Služby nebo zboží z EU, stavební práce (§ 92a), služby ze zahraničí. Dodavatel DPH neúčtuje — u položek zvolte sazbu, kterou daň přiznáte."
            disabled={saving}
          />
        )}
        {reverseCharge && (
          <SelectField control={form.control} name="supply_type" label="Druh plnění" options={supplyTypeOptions} disabled={saving} />
        )}
        <LineEditor form={form} defaultVatRateBps={account?.default_vat_rate_bps ?? 2100} disabled={saving} />
        <div className="flex flex-col items-end gap-2">
          <TotalsSummary totals={totals} currency={currency} className="w-full rounded-xl border bg-muted/30 p-4 sm:max-w-xs" />
          {reverseCharge && (
            <p className="w-full text-xs text-muted-foreground sm:max-w-xs">
              DPH na dokladu je 0 — daň podle sazeb položek přiznáte v přiznání k DPH (a případně si ji odečtete).
            </p>
          )}
        </div>
      </section>

      <Section title="Další" description="Daňová uznatelnost a interní poznámka.">
        <SwitchField
          control={form.control}
          name="tax_deductible"
          label="Daňově uznatelný náklad"
          description="Daň z příjmů: započítá se do daňových výdajů."
          disabled={saving}
        />
        {vatPayer && (
          <SwitchField
            control={form.control}
            name="vat_deductible"
            label="Uplatnit odpočet DPH"
            description="DPH: daň z dokladu si odečtete v přiznání k DPH."
            disabled={saving}
          />
        )}
        {mode === 'edit' && (
          <TextField control={form.control} name="number" label="Interní číslo" autoComplete="off" description="Přiděluje se automaticky z číselné řady nákladů." disabled={saving} />
        )}
        <TextareaField control={form.control} name="private_note" label="Interní poznámka" rows={3} placeholder="Vidíte jen vy" disabled={saving} />
      </Section>

      <StickyActionBar
        start={
          <span>
            Celkem <strong className="text-foreground tabular-nums">{formatMoney(totals.total, currency)}</strong>
          </span>
        }
      >
        <Button type="button" variant="outline" onClick={onCancel} disabled={saving} className="max-md:hidden">
          Zrušit
        </Button>
        <Button type="submit" disabled={saving}>
          {saving && <Spinner data-icon="inline-start" />}
          {submitLabel}
          <span className="font-normal opacity-80 tabular-nums md:hidden">· {formatMoney(totals.total, currency)}</span>
        </Button>
      </StickyActionBar>
    </form>
  )
}

function Section({ title, description, children, className }: { title: string; description?: string; children: ReactNode; className?: string }) {
  return (
    <section className={cn('grid gap-4 border-b py-6 first:pt-0 md:grid-cols-[minmax(0,13rem)_minmax(0,1fr)] md:gap-10 md:py-8', className)}>
      <div>
        <h2 className="text-base font-semibold tracking-tight">{title}</h2>
        {description && <p className="mt-1 text-sm text-muted-foreground">{description}</p>}
      </div>
      <div className="flex min-w-0 flex-col gap-5">{children}</div>
    </section>
  )
}

/**
 * Datumy: změna data vystavení posune DUZP a splatnost, pokud je uživatel ručně nezměnil
 * (DUZP = vystavení, splatnost = vystavení + výchozí počet dní).
 */
function DateFields({ form, defaultDueDays, disabled }: { form: UseFormReturn<ExpenseFormValues>; defaultDueDays: number; disabled?: boolean }) {
  return (
    <div className="grid gap-5 sm:grid-cols-3">
      <Controller
        control={form.control}
        name="issued_on"
        render={({ field, fieldState }) => (
          <Field data-invalid={fieldState.invalid || undefined}>
            <FieldLabel htmlFor="expense-issued-on">Datum vystavení</FieldLabel>
            <Input
              id="expense-issued-on"
              type="date"
              {...field}
              disabled={disabled}
              aria-invalid={fieldState.invalid || undefined}
              onChange={(e) => {
                const prev = field.value
                const next = e.target.value
                field.onChange(next)
                if (!next || !prev) return
                const { taxable_fulfillment_due: duzp, due_on: due } = form.getValues()
                if (duzp === prev) form.setValue('taxable_fulfillment_due', next, { shouldDirty: true })
                try {
                  if (due === addDays(prev, defaultDueDays)) form.setValue('due_on', addDays(next, defaultDueDays), { shouldDirty: true })
                } catch {
                  // neplatné datum během psaní — nic neposouváme
                }
              }}
            />
            <FieldError errors={[fieldState.error]} />
          </Field>
        )}
      />
      <DateInput form={form} name="taxable_fulfillment_due" label="DUZP" disabled={disabled} />
      <DateInput form={form} name="due_on" label="Splatnost" disabled={disabled} />
    </div>
  )
}

function DateInput({ form, name, label, disabled }: { form: UseFormReturn<ExpenseFormValues>; name: 'taxable_fulfillment_due' | 'due_on'; label: string; disabled?: boolean }) {
  const id = `expense-${name}`
  return (
    <Controller
      control={form.control}
      name={name}
      render={({ field, fieldState }) => (
        <Field data-invalid={fieldState.invalid || undefined}>
          <FieldLabel htmlFor={id}>{label}</FieldLabel>
          <Input id={id} type="date" {...field} disabled={disabled} aria-invalid={fieldState.invalid || undefined} />
          <FieldError errors={[fieldState.error]} />
        </Field>
      )}
    />
  )
}
