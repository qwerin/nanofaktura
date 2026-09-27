import { PackageIcon, PlusIcon, Trash2Icon, XIcon } from 'lucide-react'
import { useId, useMemo, useState } from 'react'
import {
  Controller,
  useFieldArray,
  useWatch,
  type Control,
  type FieldPath,
  type UseFormReturn,
} from 'react-hook-form'
import { toast } from 'sonner'
import type { PriceItem } from '@/api/types'
import { PriceItemPicker } from '@/components/price-item/price-item-picker'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { FieldError } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { useIsMobile } from '@/hooks/use-mobile'
import { formatMoney, formatMoneyInput, formatVatRate, parseMoney } from '@/lib/money'
import { parseQuantity } from '@/lib/quantity'
import { cn } from '@/lib/utils'
import { draftToCalcLine, draftTotals, emptyLine, isBlankLine, lineFromPriceItem, type Totals } from './calc'
import type { ExpenseFormValues } from './expense-form-schema'
import { vatRateOptions } from './format'

interface LineEditorProps {
  form: UseFormReturn<ExpenseFormValues>
  defaultVatRateBps: number
  disabled?: boolean
}

/**
 * Editor řádků nákladu: na mobilu skládací karty, na desktopu tabulka. Živý přepočet řádků,
 * výběr z ceníku (vyplní název, jednotku, cenu a DPH).
 */
export function LineEditor({ form, defaultVatRateBps, disabled }: LineEditorProps) {
  const isMobile = useIsMobile()
  const { control } = form
  const { fields, append, remove, update } = useFieldArray({ control, name: 'lines' })
  const lines = useWatch({ control, name: 'lines' })
  const pricesIncludeVat = useWatch({ control, name: 'prices_include_vat' })
  const currency = useWatch({ control, name: 'currency' })
  const [pickerOpen, setPickerOpen] = useState(false)
  const [pickerTarget, setPickerTarget] = useState<number | null>(null)
  const linesError = form.formState.errors.lines

  const extraRates = useMemo(() => (lines ?? []).map((l) => Number(l.vat_rate_bps)), [lines])
  const rateOptions = vatRateOptions(extraRates)

  const openPicker = (index: number | null) => {
    setPickerTarget(index)
    setPickerOpen(true)
  }

  const onPick = (item: PriceItem) => {
    if (item.currency !== currency) {
      toast.warning(`Položka „${item.name}“ má cenu v ${item.currency}, náklad je v ${currency}. Zkontrolujte cenu.`)
    }
    const current = form.getValues('lines')
    // Cíl: vybraný řádek, jinak poslední prázdný řádek, jinak nový řádek.
    let target = pickerTarget
    if (target === null) {
      const last = current.length - 1
      target = last >= 0 && current[last] && isBlankLine(current[last]) ? last : null
    }
    if (target === null) {
      append(lineFromPriceItem(item, pricesIncludeVat))
    } else {
      update(target, lineFromPriceItem(item, pricesIncludeVat, current[target]))
      form.clearErrors(`lines.${target}`)
    }
  }

  const Row = isMobile ? LineCard : LineRow

  return (
    <div className="flex flex-col gap-3">
      {!isMobile && (
        <div className="grid grid-cols-[minmax(0,1fr)_5.5rem_5rem_8rem_6.5rem_7.5rem_2rem] gap-2 px-1 text-xs font-medium text-muted-foreground">
          <span>Položka</span>
          <span className="text-right">Množství</span>
          <span>Jednotka</span>
          <span className="text-right">Cena/jedn. {pricesIncludeVat ? 's DPH' : 'bez DPH'}</span>
          <span>DPH</span>
          <span className="text-right">Celkem</span>
          <span />
        </div>
      )}
      <ul className="flex flex-col gap-3 md:gap-2">
        {fields.map((f, index) => (
          <Row
            key={f.id}
            index={index}
            control={control}
            total={lineTotal(lines?.[index], pricesIncludeVat)}
            currency={currency}
            rateOptions={rateOptions}
            canRemove={fields.length > 1}
            disabled={disabled}
            onRemove={() => remove(index)}
            onPick={() => openPicker(index)}
            onUnlink={() => form.setValue(`lines.${index}.price_item_id`, undefined, { shouldDirty: true })}
            linked={Boolean(lines?.[index]?.price_item_id)}
            pricesIncludeVat={pricesIncludeVat}
          />
        ))}
      </ul>
      {linesError && !Array.isArray(linesError) && (linesError.message || linesError.root?.message) && (
        <p className="text-sm text-destructive" role="alert">
          {linesError.message || linesError.root?.message}
        </p>
      )}
      <div className="grid grid-cols-2 gap-2 md:flex">
        <Button type="button" variant="outline" disabled={disabled} onClick={() => append(emptyLine(defaultVatRateBps))}>
          <PlusIcon data-icon="inline-start" />
          Přidat položku
        </Button>
        <Button type="button" variant="outline" disabled={disabled} onClick={() => openPicker(null)}>
          <PackageIcon data-icon="inline-start" />
          Z ceníku
        </Button>
      </div>
      <PriceItemPicker open={pickerOpen} onOpenChange={setPickerOpen} onPick={onPick} />
    </div>
  )
}

function lineTotal(line: ExpenseFormValues['lines'][number] | undefined, pricesIncludeVat: boolean): number | null {
  if (!line) return null
  const calc = draftToCalcLine(line)
  if (!calc) return null
  // Informativně: celkem s DPH za řádek.
  return draftTotals([line], { pricesIncludeVat, roundTotal: false }).lines[0]?.total ?? null
}

interface RowProps {
  index: number
  control: Control<ExpenseFormValues>
  total: number | null
  currency: string
  rateOptions: { value: string; label: string }[]
  canRemove: boolean
  disabled?: boolean
  onRemove: () => void
  onPick: () => void
  onUnlink: () => void
  linked: boolean
  pricesIncludeVat: boolean
}

/** Mobil: karta s poli pod sebou. */
function LineCard({ index, control, total, currency, rateOptions, canRemove, disabled, onRemove, onPick, onUnlink, linked, pricesIncludeVat }: RowProps) {
  const id = useId()
  return (
    <li className="flex flex-col gap-3 rounded-xl border bg-card p-3">
      <div className="flex items-center gap-2">
        <span className="text-sm font-medium text-muted-foreground">Položka {index + 1}</span>
        {linked && <LinkedBadge onUnlink={onUnlink} disabled={disabled} />}
        <span className="flex-1" />
        {!linked && (
          <Button type="button" variant="ghost" size="sm" className="h-9 text-primary" onClick={onPick} disabled={disabled}>
            <PackageIcon data-icon="inline-start" />
            Z ceníku
          </Button>
        )}
        {canRemove && (
          <Button type="button" variant="ghost" size="icon" className="-mr-2 size-9 text-muted-foreground" aria-label={`Odebrat položku ${index + 1}`} onClick={onRemove} disabled={disabled}>
            <Trash2Icon />
          </Button>
        )}
      </div>
      <LineInput control={control} name={`lines.${index}.name`} label="Název" id={`${id}-name`} placeholder="Co jste nakoupili" autoCapitalize="sentences" disabled={disabled} />
      <div className="grid grid-cols-2 gap-3">
        <QuantityInput control={control} name={`lines.${index}.quantity`} label="Množství" id={`${id}-qty`} disabled={disabled} />
        <LineInput control={control} name={`lines.${index}.unit_name`} label="Jednotka" id={`${id}-unit`} placeholder="ks, h, …" disabled={disabled} />
        <MoneyInput control={control} name={`lines.${index}.unit_price`} label={pricesIncludeVat ? 'Cena s DPH' : 'Cena bez DPH'} id={`${id}-price`} disabled={disabled} />
        <VatSelect control={control} name={`lines.${index}.vat_rate_bps`} label="DPH" id={`${id}-vat`} options={rateOptions} disabled={disabled} />
      </div>
      <div className="flex items-baseline justify-between border-t pt-2 text-sm">
        <span className="text-muted-foreground">Celkem s DPH</span>
        <span className="font-semibold tabular-nums">{total === null ? '—' : formatMoney(total, currency)}</span>
      </div>
    </li>
  )
}

/** Desktop: jeden řádek tabulky. */
function LineRow({ index, control, total, currency, rateOptions, canRemove, disabled, onRemove, onPick, onUnlink, linked }: RowProps) {
  const id = useId()
  return (
    <li className="grid grid-cols-[minmax(0,1fr)_5.5rem_5rem_8rem_6.5rem_7.5rem_2rem] items-start gap-2 rounded-lg px-1">
      <div className="flex min-w-0 flex-col gap-1">
        <div className="relative">
          <LineInput control={control} name={`lines.${index}.name`} label={`Název položky ${index + 1}`} srLabel id={`${id}-name`} placeholder="Název položky" disabled={disabled} className={linked ? 'pr-8' : 'pr-8'} />
          <Button
            type="button"
            variant="ghost"
            size="icon-sm"
            className="absolute top-0.5 right-0.5 text-muted-foreground hover:text-primary"
            aria-label="Vybrat z ceníku"
            title="Vybrat z ceníku"
            onClick={onPick}
            disabled={disabled}
          >
            <PackageIcon />
          </Button>
        </div>
        {linked && <LinkedBadge onUnlink={onUnlink} disabled={disabled} />}
      </div>
      <QuantityInput control={control} name={`lines.${index}.quantity`} label="Množství" srLabel id={`${id}-qty`} disabled={disabled} />
      <LineInput control={control} name={`lines.${index}.unit_name`} label="Jednotka" srLabel id={`${id}-unit`} placeholder="ks" disabled={disabled} />
      <MoneyInput control={control} name={`lines.${index}.unit_price`} label="Cena za jednotku" srLabel id={`${id}-price`} disabled={disabled} />
      <VatSelect control={control} name={`lines.${index}.vat_rate_bps`} label="Sazba DPH" srLabel id={`${id}-vat`} options={rateOptions} disabled={disabled} />
      <span className="flex h-8 items-center justify-end text-sm font-medium tabular-nums">
        {total === null ? '—' : formatMoney(total, currency)}
      </span>
      <Button
        type="button"
        variant="ghost"
        size="icon-sm"
        className="mt-0.5 text-muted-foreground hover:text-destructive"
        aria-label={`Odebrat položku ${index + 1}`}
        onClick={onRemove}
        disabled={disabled || !canRemove}
      >
        <Trash2Icon />
      </Button>
    </li>
  )
}

function LinkedBadge({ onUnlink, disabled }: { onUnlink: () => void; disabled?: boolean }) {
  return (
    <Badge variant="secondary" className="h-6 gap-1 pr-0.5">
      <PackageIcon data-icon="inline-start" />
      Z ceníku
      <button
        type="button"
        className="flex size-5 items-center justify-center rounded-full hover:bg-foreground/10"
        aria-label="Odpojit od ceníku"
        title="Odpojit od ceníku (sklad se nebude měnit)"
        onClick={onUnlink}
        disabled={disabled}
      >
        <XIcon className="size-3" />
      </button>
    </Badge>
  )
}

// --- Pole řádku ---

interface CellProps {
  control: Control<ExpenseFormValues>
  name: FieldPath<ExpenseFormValues>
  label: string
  id: string
  /** Popisek jen pro čtečky (desktopová tabulka má hlavičku). */
  srLabel?: boolean
  placeholder?: string
  disabled?: boolean
  className?: string
  autoCapitalize?: string
}

function CellShell({ id, label, srLabel, error, children }: { id: string; label: string; srLabel?: boolean; error?: { message?: string }; children: React.ReactNode }) {
  return (
    <div className="flex min-w-0 flex-col gap-1.5">
      <label htmlFor={id} className={cn('text-sm font-medium', srLabel && 'sr-only')}>
        {label}
      </label>
      {children}
      <FieldError errors={[error]} className="text-xs" />
    </div>
  )
}

function LineInput({ control, name, label, id, srLabel, placeholder, disabled, className, autoCapitalize }: CellProps) {
  return (
    <Controller
      control={control}
      name={name}
      render={({ field, fieldState }) => (
        <CellShell id={id} label={label} srLabel={srLabel} error={fieldState.error}>
          <Input
            id={id}
            {...field}
            value={String(field.value ?? '')}
            placeholder={placeholder}
            disabled={disabled}
            autoComplete="off"
            autoCapitalize={autoCapitalize}
            enterKeyHint="next"
            aria-invalid={fieldState.invalid || undefined}
            className={className}
          />
        </CellShell>
      )}
    />
  )
}

function QuantityInput({ control, name, label, id, srLabel, disabled }: CellProps) {
  return (
    <Controller
      control={control}
      name={name}
      render={({ field, fieldState }) => (
        <CellShell id={id} label={label} srLabel={srLabel} error={fieldState.error}>
          <Input
            id={id}
            {...field}
            value={String(field.value ?? '')}
            inputMode="decimal"
            autoComplete="off"
            enterKeyHint="next"
            disabled={disabled}
            className="text-right tabular-nums"
            aria-invalid={fieldState.invalid || undefined}
            onBlur={() => {
              const q = parseQuantity(String(field.value ?? ''))
              if (q !== null) field.onChange(q.replace('.', ','))
              field.onBlur()
            }}
          />
        </CellShell>
      )}
    />
  )
}

function MoneyInput({ control, name, label, id, srLabel, disabled }: CellProps) {
  return (
    <Controller
      control={control}
      name={name}
      render={({ field, fieldState }) => (
        <CellShell id={id} label={label} srLabel={srLabel} error={fieldState.error}>
          <Input
            id={id}
            {...field}
            value={String(field.value ?? '')}
            inputMode="decimal"
            autoComplete="off"
            enterKeyHint="next"
            placeholder="0,00"
            disabled={disabled}
            className="text-right tabular-nums"
            aria-invalid={fieldState.invalid || undefined}
            onBlur={() => {
              const m = parseMoney(String(field.value ?? ''))
              if (m !== null) field.onChange(formatMoneyInput(m))
              field.onBlur()
            }}
          />
        </CellShell>
      )}
    />
  )
}

function VatSelect({ control, name, label, id, srLabel, disabled, options }: CellProps & { options: { value: string; label: string }[] }) {
  return (
    <Controller
      control={control}
      name={name}
      render={({ field, fieldState }) => (
        <CellShell id={id} label={label} srLabel={srLabel} error={fieldState.error}>
          <Select items={options} value={String(field.value ?? '')} onValueChange={(v) => field.onChange(v)} disabled={disabled}>
            <SelectTrigger id={id} ref={field.ref} onBlur={field.onBlur} className="w-full" aria-invalid={fieldState.invalid || undefined}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {options.map((o) => (
                <SelectItem key={o.value} value={o.value}>
                  {o.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </CellShell>
      )}
    />
  )
}

/** Rekapitulace a součty (živý náhled). */
export function TotalsSummary({ totals, currency, className }: { totals: Totals; currency: string; className?: string }) {
  return (
    <dl className={cn('flex flex-col gap-1.5 text-sm', className)}>
      <div className="flex justify-between gap-4">
        <dt className="text-muted-foreground">Základ</dt>
        <dd className="tabular-nums">{formatMoney(totals.subtotal, currency)}</dd>
      </div>
      {totals.recap
        .filter((r) => r.vatRateBps !== 0 || totals.recap.length > 1)
        .map((r) => (
          <div key={r.vatRateBps} className="flex justify-between gap-4">
            <dt className="text-muted-foreground">DPH {formatVatRate(r.vatRateBps)}</dt>
            <dd className="tabular-nums">{formatMoney(r.vat, currency)}</dd>
          </div>
        ))}
      {totals.rounding !== 0 && (
        <div className="flex justify-between gap-4">
          <dt className="text-muted-foreground">Zaokrouhlení</dt>
          <dd className="tabular-nums">{formatMoney(totals.rounding, currency)}</dd>
        </div>
      )}
      <div className="mt-1 flex items-baseline justify-between gap-4 border-t pt-2">
        <dt className="font-medium">Celkem</dt>
        <dd className="text-lg font-semibold tabular-nums">{formatMoney(totals.total, currency)}</dd>
      </div>
    </dl>
  )
}
