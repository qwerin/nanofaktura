import { zodResolver } from '@hookform/resolvers/zod'
import { useState } from 'react'
import { useForm, useWatch } from 'react-hook-form'
import { toast } from 'sonner'
import { applyProblemToForm, errorMessage } from '@/api/errors'
import { ConfirmDialog } from '@/components/expense/confirm-dialog'
import { currencyOptions, vatRateOptions } from '@/components/expense/format'
import { UnsavedChangesGuard } from '@/components/expense/unsaved-changes-guard'
import { SelectField, SwitchField, TextareaField, TextField } from '@/components/form/fields'
import { FormSection } from '@/components/settings-page'
import { StickyActionBar } from '@/components/sticky-action-bar'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'
import { formatMoney, parseMoney } from '@/lib/money'
import { priceItemFormSchema, type PriceItemFormValues } from './price-item-form-schema'

const FORM_FIELDS = Object.keys(priceItemFormSchema.shape)

interface PriceItemFormProps {
  mode: 'create' | 'edit'
  defaultValues: PriceItemFormValues
  onSubmit: (values: PriceItemFormValues) => Promise<void>
  onCancel: () => void
  /** U úpravy: položka má pohyby — vypnutí skladu je potřeba potvrdit. */
  hasStock?: boolean
}

export function PriceItemForm({ mode, defaultValues, onSubmit, onCancel, hasStock }: PriceItemFormProps) {
  const form = useForm<PriceItemFormValues>({ resolver: zodResolver(priceItemFormSchema), defaultValues, mode: 'onTouched' })
  const [saving, setSaving] = useState(false)
  const [confirmStockOff, setConfirmStockOff] = useState<PriceItemFormValues | null>(null)
  const trackStock = useWatch({ control: form.control, name: 'track_stock' })
  const currency = useWatch({ control: form.control, name: 'currency' })
  const unitPrice = useWatch({ control: form.control, name: 'unit_price' })
  const unitName = useWatch({ control: form.control, name: 'unit_name' })
  const includeVat = useWatch({ control: form.control, name: 'prices_include_vat' })
  const vatRate = useWatch({ control: form.control, name: 'vat_rate_bps' })
  const price = parseMoney(unitPrice)

  const save = async (values: PriceItemFormValues) => {
    setSaving(true)
    try {
      await onSubmit(values)
    } catch (err) {
      if (applyProblemToForm(err, form.setError, FORM_FIELDS)) toast.error('Zkontrolujte zvýrazněná pole.')
      else toast.error(errorMessage(err))
    } finally {
      setSaving(false)
    }
  }

  const submit = form.handleSubmit(
    (values) => {
      if (mode === 'edit' && hasStock && defaultValues.track_stock && !values.track_stock) {
        setConfirmStockOff(values)
        return
      }
      return save(values)
    },
    () => toast.error('Zkontrolujte zvýrazněná pole.'),
  )

  return (
    <form onSubmit={submit} noValidate>
      <UnsavedChangesGuard when={form.formState.isDirty && !saving} />
      <FormSection title="Položka" description="Jak se položka jmenuje na dokladech.">
        <TextField control={form.control} name="name" label="Název" autoComplete="off" autoCapitalize="sentences" enterKeyHint="next" placeholder="např. Konzultace, Kabel USB-C" disabled={saving} />
        <div className="grid gap-5 sm:grid-cols-2">
          <TextField control={form.control} name="sku" label="Kód (SKU)" autoComplete="off" autoCapitalize="characters" spellCheck={false} enterKeyHint="next" description="Nepovinné, pro rychlé hledání." disabled={saving} />
          <TextField control={form.control} name="unit_name" label="Jednotka" autoComplete="off" enterKeyHint="next" placeholder="ks, h, kg…" disabled={saving} />
        </div>
      </FormSection>

      <FormSection title="Cena" description="Předvyplní se do řádku dokladu.">
        <div className="grid gap-5 sm:grid-cols-2">
          <TextField control={form.control} name="unit_price" label={includeVat ? 'Cena za jednotku s DPH' : 'Cena za jednotku bez DPH'} inputMode="decimal" autoComplete="off" placeholder="0,00" enterKeyHint="next" disabled={saving} />
          <SelectField control={form.control} name="vat_rate_bps" label="Sazba DPH" options={vatRateOptions([Number(vatRate)])} disabled={saving} />
          <SelectField control={form.control} name="currency" label="Měna" options={currencyOptions(currency)} disabled={saving} />
        </div>
        <SwitchField control={form.control} name="prices_include_vat" label="Cena včetně DPH" description="Zadáváte koncovou cenu, DPH se z ní dopočítá." disabled={saving} />
      </FormSection>

      <FormSection title="Sklad" description="Faktury zboží odepisují, náklady ho naskladňují.">
        <SwitchField control={form.control} name="track_stock" label="Sledovat skladové zásoby" description="Každý doklad s touto položkou zapíše pohyb na skladě." disabled={saving} />
        {trackStock && (
          <div className="grid gap-5 sm:grid-cols-2">
            {mode === 'create' && (
              <TextField control={form.control} name="stock_quantity" label={`Počáteční stav${unitName ? ` (${unitName})` : ''}`} inputMode="decimal" autoComplete="off" placeholder="0" description="Zapíše se jako příjem na sklad." disabled={saving} />
            )}
            <TextField control={form.control} name="min_stock" label={`Minimální stav${unitName ? ` (${unitName})` : ''}`} inputMode="decimal" autoComplete="off" placeholder="bez limitu" description="Pod touto hranicí upozorníme na nízký stav." disabled={saving} />
          </div>
        )}
      </FormSection>

      <FormSection title="Poznámka">
        <TextareaField control={form.control} name="note" label="Interní poznámka" rows={3} disabled={saving} />
      </FormSection>

      <StickyActionBar
        start={price !== null && price !== 0 ? <span>{formatMoney(price, currency)} {includeVat ? 's DPH' : 'bez DPH'}{unitName && ` / ${unitName}`}</span> : undefined}
      >
        <Button type="button" variant="outline" onClick={onCancel} disabled={saving} className="max-md:hidden">
          Zrušit
        </Button>
        <Button type="submit" disabled={saving}>
          {saving && <Spinner data-icon="inline-start" />}
          {mode === 'create' ? 'Přidat do ceníku' : 'Uložit'}
        </Button>
      </StickyActionBar>

      <ConfirmDialog
        open={confirmStockOff !== null}
        onOpenChange={(o) => !o && setConfirmStockOff(null)}
        title="Vypnout sledování skladu?"
        description="Historie pohybů zůstane, ale nové doklady už stav skladu měnit nebudou."
        confirmLabel="Vypnout a uložit"
        onConfirm={() => {
          const v = confirmStockOff
          setConfirmStockOff(null)
          if (v) void save(v)
        }}
      />
    </form>
  )
}
