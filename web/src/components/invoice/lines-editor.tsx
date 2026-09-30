// Editor položek faktury.
// - desktop: kompaktní tabulka, Enter = další řádek (na posledním přidá nový),
//   přesun tažením za úchyt nebo šipkami nahoru/dolů na úchytu (i Alt+↑/↓ v řádku).
// - mobil: skládací karty s číselnou klávesnicí pro množství a cenu.

import { ChevronDownIcon, ChevronUpIcon, GripVerticalIcon, PackageIcon, PlusIcon, Trash2Icon } from 'lucide-react'
import { useId, useState, type KeyboardEvent, type ReactNode } from 'react'
import {
  Controller,
  useFieldArray,
  useFormState,
  useWatch,
  type Control,
  type UseFormRegister,
  type UseFormSetFocus,
} from 'react-hook-form'
import { Button } from '@/components/ui/button'
import { Field, FieldError, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { useIsMobile } from '@/hooks/use-mobile'
import { formatMoney, formatMoneyInput, formatVatRate } from '@/lib/money'
import { cn } from '@/lib/utils'
import { convertPrice, emptyLine, type LineValues } from './form-model'
import { PriceItemPicker, type PriceItem } from './price-item-picker'

const VAT_RATES = [2100, 1200, 0] as const
const vatItems = VAT_RATES.map((r) => ({ value: String(r), label: formatVatRate(r) }))

/** Formulář s řádky ve tvaru faktury (faktura, šablona faktury). */
export interface LinesFormValues {
  lines: LineValues[]
}

interface LinesEditorProps<T extends LinesFormValues> {
  control: Control<T>
  register: UseFormRegister<T>
  setFocus: UseFormSetFocus<T>
  /** Vypočtená částka řádku (cena × množství) ve stejném pořadí jako řádky. */
  amounts: number[]
  currency: string
  showVat: boolean
  defaultVat: number
  amountLabel: string
  disabled?: boolean
  /** Pro výběr z ceníku. */
  slug: string
  pricesIncludeVat: boolean
}

type InnerProps = LinesEditorProps<LinesFormValues>

export function LinesEditor<T extends LinesFormValues>(outerProps: LinesEditorProps<T>) {
  // Editor pracuje jen s polem `lines`, které má každý T stejné — zúžení typu je bezpečné.
  const props = outerProps as unknown as InnerProps
  const isMobile = useIsMobile()
  const { control, defaultVat } = props
  // `keyName` ≠ `id` — řádky mají vlastní `id` z API.
  const lines = useFieldArray({ control, name: 'lines', keyName: 'key' })
  const { errors } = useFormState({ control, name: 'lines' })
  const rootError = errors.lines?.root?.message ?? errors.lines?.message

  const [pickerOpen, setPickerOpen] = useState(false)
  const values = useWatch({ control, name: 'lines' })
  const pickItem = (item: PriceItem) => {
    const vat = props.showVat ? item.vat_rate_bps : 0
    const line: LineValues = {
      price_item_id: item.id,
      name: item.name,
      quantity: '1',
      unit_name: item.unit_name,
      unit_price: formatMoneyInput(convertPrice(item.unit_price, item.prices_include_vat, props.pricesIncludeVat, vat)),
      vat_rate_bps: String(vat),
    }
    const lastIndex = lines.fields.length - 1
    const last = values?.[lastIndex]
    if (last && !last.name.trim() && !last.unit_price.trim()) lines.update(lastIndex, { ...line, id: last.id })
    else lines.append(line)
  }

  const add = () =>
    lines.append(emptyLine(props.showVat ? defaultVat : 0), {
      shouldFocus: true,
      focusName: `lines.${lines.fields.length}.name`,
    })

  return (
    <div className="flex flex-col gap-3">
      {isMobile ? (
        <MobileLines {...props} lines={lines} />
      ) : (
        <DesktopLines {...props} lines={lines} onAdd={add} />
      )}
      {rootError && <p className="text-sm text-destructive">{rootError}</p>}
      <div className="grid grid-cols-2 gap-2 md:flex">
        <Button type="button" variant="outline" onClick={add} disabled={props.disabled}>
          <PlusIcon data-icon="inline-start" />
          Přidat položku
        </Button>
        <Button type="button" variant="ghost" onClick={() => setPickerOpen(true)} disabled={props.disabled}>
          <PackageIcon data-icon="inline-start" />
          Z ceníku
        </Button>
      </div>
      <PriceItemPicker slug={props.slug} open={pickerOpen} onOpenChange={setPickerOpen} onPick={pickItem} />
    </div>
  )
}

type FieldArray = ReturnType<typeof useFieldArray<LinesFormValues, 'lines', 'key'>>

function VatSelect({
  control,
  index,
  className,
  id,
  disabled,
}: {
  control: Control<LinesFormValues>
  index: number
  className?: string
  id?: string
  disabled?: boolean
}) {
  return (
    <Controller
      control={control}
      name={`lines.${index}.vat_rate_bps`}
      render={({ field }) => (
        <Select items={vatItems} value={field.value} onValueChange={(v) => field.onChange(v ?? '0')} disabled={disabled}>
          <SelectTrigger id={id} ref={field.ref} className={cn('w-full', className)} aria-label="Sazba DPH">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {vatItems.map((o) => (
              <SelectItem key={o.value} value={o.value}>
                {o.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      )}
    />
  )
}

// --- Desktop ---

function DesktopLines({
  control,
  register,
  setFocus,
  amounts,
  currency,
  showVat,
  amountLabel,
  disabled,
  lines,
  onAdd,
}: InnerProps & { lines: FieldArray; onAdd: () => void }) {
  const { errors } = useFormState({ control, name: 'lines' })
  const [dragFrom, setDragFrom] = useState<number | null>(null)
  const [dragOver, setDragOver] = useState<number | null>(null)
  const grid = showVat
    ? 'grid-cols-[1.75rem_minmax(0,1fr)_5.5rem_5rem_8rem_5.5rem_8rem_2rem]'
    : 'grid-cols-[1.75rem_minmax(0,1fr)_5.5rem_5rem_8rem_8rem_2rem]'

  const move = (from: number, to: number) => {
    if (to < 0 || to >= lines.fields.length || from === to) return
    lines.move(from, to)
  }

  const onRowKeyDown = (e: KeyboardEvent<HTMLDivElement>, index: number) => {
    const target = e.target as HTMLElement
    if (e.key === 'Enter' && target.tagName === 'INPUT') {
      e.preventDefault()
      if (index === lines.fields.length - 1) onAdd()
      else setFocus(`lines.${index + 1}.name`)
    } else if (e.altKey && (e.key === 'ArrowUp' || e.key === 'ArrowDown')) {
      e.preventDefault()
      move(index, e.key === 'ArrowUp' ? index - 1 : index + 1)
    }
  }

  return (
    <div className="overflow-hidden rounded-lg border" role="table" aria-label="Položky faktury">
      <div role="row" className={cn('grid items-center gap-2 border-b bg-muted/50 px-2 py-2 text-xs font-medium text-muted-foreground', grid)}>
        <span role="columnheader" aria-label="Pořadí" />
        <span role="columnheader">Název položky</span>
        <span role="columnheader" className="text-right">
          Množství
        </span>
        <span role="columnheader">Jednotka</span>
        <span role="columnheader" className="text-right">
          Cena za jedn.
        </span>
        {showVat && <span role="columnheader">DPH</span>}
        <span role="columnheader" className="text-right">
          {amountLabel}
        </span>
        <span role="columnheader" aria-label="Akce" />
      </div>
      {lines.fields.map((f, i) => {
        const err = errors.lines?.[i]
        return (
          <div
            key={f.key}
            role="row"
            onKeyDown={(e) => onRowKeyDown(e, i)}
            onDragOver={(e) => {
              if (dragFrom === null) return
              e.preventDefault()
              setDragOver(i)
            }}
            onDrop={(e) => {
              e.preventDefault()
              if (dragFrom !== null) move(dragFrom, i)
              setDragFrom(null)
              setDragOver(null)
            }}
            className={cn(
              'group/line grid items-start gap-2 border-b px-2 py-1.5 last:border-b-0',
              grid,
              dragFrom === i && 'opacity-40',
              dragOver === i && dragFrom !== null && dragFrom !== i && 'bg-accent/60',
            )}
          >
            <button
              type="button"
              draggable={!disabled}
              onDragStart={(e) => {
                setDragFrom(i)
                e.dataTransfer.effectAllowed = 'move'
                const row = (e.currentTarget as HTMLElement).parentElement
                if (row) e.dataTransfer.setDragImage(row, 16, 16)
              }}
              onDragEnd={() => {
                setDragFrom(null)
                setDragOver(null)
              }}
              onKeyDown={(e) => {
                if (e.key === 'ArrowUp' || e.key === 'ArrowDown') {
                  e.preventDefault()
                  e.stopPropagation()
                  const to = e.key === 'ArrowUp' ? i - 1 : i + 1
                  move(i, to)
                  requestAnimationFrame(() =>
                    document.querySelector<HTMLElement>(`[data-line-handle="${to}"]`)?.focus(),
                  )
                }
              }}
              data-line-handle={i}
              aria-label={`Položka ${i + 1}: přesunout (šipky nahoru a dolů)`}
              className="mt-1.5 flex h-5 cursor-grab items-center justify-center rounded text-muted-foreground/60 hover:text-foreground focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none active:cursor-grabbing"
            >
              <GripVerticalIcon className="size-4" />
            </button>
            <CellInput error={err?.name?.message}>
              <Input
                {...register(`lines.${i}.name`)}
                placeholder="Např. Vývoj webu"
                autoComplete="off"
                aria-label={`Položka ${i + 1}: název`}
                aria-invalid={Boolean(err?.name) || undefined}
                disabled={disabled}
              />
            </CellInput>
            <CellInput error={err?.quantity?.message}>
              <Input
                {...register(`lines.${i}.quantity`)}
                inputMode="decimal"
                autoComplete="off"
                className="text-right tabular-nums"
                aria-label={`Položka ${i + 1}: množství`}
                aria-invalid={Boolean(err?.quantity) || undefined}
                disabled={disabled}
              />
            </CellInput>
            <Input
              {...register(`lines.${i}.unit_name`)}
              placeholder="ks"
              autoComplete="off"
              aria-label={`Položka ${i + 1}: jednotka`}
              disabled={disabled}
            />
            <CellInput error={err?.unit_price?.message}>
              <Input
                {...register(`lines.${i}.unit_price`)}
                inputMode="decimal"
                autoComplete="off"
                placeholder="0,00"
                className="text-right tabular-nums"
                aria-label={`Položka ${i + 1}: cena za jednotku`}
                aria-invalid={Boolean(err?.unit_price) || undefined}
                disabled={disabled}
              />
            </CellInput>
            {showVat && <VatSelect control={control} index={i} disabled={disabled} />}
            <span className="mt-1.5 truncate text-right text-sm font-medium tabular-nums">
              {formatMoney(amounts[i] ?? 0, currency)}
            </span>
            <Button
              type="button"
              variant="ghost"
              size="icon"
              className="text-muted-foreground hover:text-destructive"
              aria-label={`Odebrat položku ${i + 1}`}
              disabled={disabled || lines.fields.length === 1}
              onClick={() => lines.remove(i)}
            >
              <Trash2Icon />
            </Button>
          </div>
        )
      })}
    </div>
  )
}

function CellInput({ error, children }: { error?: string; children: ReactNode }) {
  return (
    <div className="flex min-w-0 flex-col gap-0.5">
      {children}
      {error && <span className="text-xs text-destructive">{error}</span>}
    </div>
  )
}

// --- Mobil ---

function MobileLines({
  control,
  register,
  amounts,
  currency,
  showVat,
  amountLabel,
  disabled,
  lines,
}: InnerProps & { lines: FieldArray }) {
  const { errors } = useFormState({ control, name: 'lines' })
  // Sbalené karty podle `key`: na začátku řádky s názvem (uložené), nové řádky jsou rozbalené,
  // dokud je uživatel sám nesbalí — i když mezitím dostanou název a přesunou se.
  const [collapsed, setCollapsed] = useState<Set<string>>(() => new Set(lines.fields.filter((f) => f.name).map((f) => f.key)))
  const baseId = useId()

  const toggle = (key: string) =>
    setCollapsed((prev) => {
      const next = new Set(prev)
      if (next.has(key)) next.delete(key)
      else next.add(key)
      return next
    })

  return (
    <ul className="flex flex-col gap-2">
      {lines.fields.map((f, i) => {
        const err = errors.lines?.[i]
        const hasError = Boolean(err)
        const expanded = !collapsed.has(f.key) || hasError || !f.name
        const id = `${baseId}-${i}`
        return (
          <li key={f.key} className={cn('rounded-xl border bg-card', hasError && 'border-destructive/60')}>
            <button
              type="button"
              aria-expanded={expanded}
              aria-controls={`${id}-body`}
              onClick={() => toggle(f.key)}
              className="flex min-h-12 w-full items-center gap-3 px-3 py-2.5 text-left"
            >
              <span className="flex size-6 shrink-0 items-center justify-center rounded-full bg-muted text-xs font-medium text-muted-foreground">
                {i + 1}
              </span>
              <LineSummary control={control} index={i} amount={amounts[i] ?? 0} currency={currency} />
              {expanded ? (
                <ChevronUpIcon className="size-4 shrink-0 text-muted-foreground" />
              ) : (
                <ChevronDownIcon className="size-4 shrink-0 text-muted-foreground" />
              )}
            </button>
            <div id={`${id}-body`} hidden={!expanded} className="flex flex-col gap-3 border-t px-3 pt-3 pb-3">
              <Field data-invalid={Boolean(err?.name) || undefined}>
                <FieldLabel htmlFor={`${id}-name`}>Název položky</FieldLabel>
                <Input
                  id={`${id}-name`}
                  {...register(`lines.${i}.name`)}
                  autoComplete="off"
                  enterKeyHint="next"
                  placeholder="Např. Vývoj webu"
                  aria-invalid={Boolean(err?.name) || undefined}
                  disabled={disabled}
                />
                <FieldError errors={[err?.name]} />
              </Field>
              <div className="grid grid-cols-2 gap-3">
                <Field data-invalid={Boolean(err?.quantity) || undefined}>
                  <FieldLabel htmlFor={`${id}-qty`}>Množství</FieldLabel>
                  <Input
                    id={`${id}-qty`}
                    {...register(`lines.${i}.quantity`)}
                    inputMode="decimal"
                    autoComplete="off"
                    enterKeyHint="next"
                    aria-invalid={Boolean(err?.quantity) || undefined}
                    disabled={disabled}
                  />
                  <FieldError errors={[err?.quantity]} />
                </Field>
                <Field>
                  <FieldLabel htmlFor={`${id}-unit`}>Jednotka</FieldLabel>
                  <Input
                    id={`${id}-unit`}
                    {...register(`lines.${i}.unit_name`)}
                    autoComplete="off"
                    enterKeyHint="next"
                    placeholder="ks, h…"
                    disabled={disabled}
                  />
                </Field>
                <Field data-invalid={Boolean(err?.unit_price) || undefined} className={cn(!showVat && 'col-span-2')}>
                  <FieldLabel htmlFor={`${id}-price`}>Cena za jednotku</FieldLabel>
                  <Input
                    id={`${id}-price`}
                    {...register(`lines.${i}.unit_price`)}
                    inputMode="decimal"
                    autoComplete="off"
                    enterKeyHint="done"
                    placeholder="0,00"
                    aria-invalid={Boolean(err?.unit_price) || undefined}
                    disabled={disabled}
                  />
                  <FieldError errors={[err?.unit_price]} />
                </Field>
                {showVat && (
                  <Field>
                    <FieldLabel htmlFor={`${id}-vat`}>DPH</FieldLabel>
                    <VatSelect control={control} index={i} id={`${id}-vat`} disabled={disabled} />
                  </Field>
                )}
              </div>
              <div className="flex items-center gap-1">
                <span className="mr-auto text-sm text-muted-foreground">
                  {amountLabel}: <span className="font-medium text-foreground tabular-nums">{formatMoney(amounts[i] ?? 0, currency)}</span>
                </span>
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  aria-label="Posunout nahoru"
                  disabled={disabled || i === 0}
                  onClick={() => lines.move(i, i - 1)}
                >
                  <ChevronUpIcon />
                </Button>
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  aria-label="Posunout dolů"
                  disabled={disabled || i === lines.fields.length - 1}
                  onClick={() => lines.move(i, i + 1)}
                >
                  <ChevronDownIcon />
                </Button>
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  className="text-destructive"
                  aria-label="Odebrat položku"
                  disabled={disabled || lines.fields.length === 1}
                  onClick={() => lines.remove(i)}
                >
                  <Trash2Icon />
                </Button>
              </div>
            </div>
          </li>
        )
      })}
    </ul>
  )
}

function LineSummary({
  control,
  index,
  amount,
  currency,
}: {
  control: Control<LinesFormValues>
  index: number
  amount: number
  currency: string
}) {
  const l = useWatch({ control, name: `lines.${index}` })
  return (
    <span className="flex min-w-0 flex-1 flex-col">
      <span className={cn('truncate text-sm font-medium', !l?.name && 'text-muted-foreground')}>
        {l?.name || 'Nová položka'}
      </span>
      <span className="truncate text-xs text-muted-foreground tabular-nums">
        {l?.quantity || '0'} {l?.unit_name} × {l?.unit_price || '0'} = {formatMoney(amount, currency)}
      </span>
    </span>
  )
}
