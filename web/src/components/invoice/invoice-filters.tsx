import { useId, useState, type ReactNode } from 'react'
import { SearchIcon, SlidersHorizontalIcon, XIcon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Drawer, DrawerContent, DrawerFooter, DrawerHeader, DrawerTitle } from '@/components/ui/drawer'
import { Field, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { InputGroup, InputGroupAddon, InputGroupButton, InputGroupInput } from '@/components/ui/input-group'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { cn } from '@/lib/utils'
import {
  DEFAULT_SORT,
  datePresets,
  extraFilterCount,
  sortLabels,
  statusTabs,
  type InvoiceSearch,
  type InvoiceSort,
} from './filters'
import { documentTypeShortLabels } from './status'

type Patch = Partial<InvoiceSearch>

interface Props {
  search: InvoiceSearch
  /** Rozpracovaný text hledání (debounce řeší rodič). */
  queryInput: string
  onQueryInput: (v: string) => void
  onChange: (patch: Patch) => void
}

const docTypeItems = [
  { value: 'all', label: 'Všechny doklady' },
  ...(['invoice', 'proforma', 'correction', 'tax_document'] as const).map((v) => ({ value: v, label: documentTypeShortLabels[v] })),
]
const sortItems = (Object.keys(sortLabels) as InvoiceSort[]).map((v) => ({ value: v, label: sortLabels[v] }))

/** Záložky stavu — vodorovně posuvné na mobilu (posouvá se jen řada, ne stránka). */
export function StatusChips({ value, onChange }: { value: InvoiceSearch['status']; onChange: (v: InvoiceSearch['status']) => void }) {
  return (
    <div
      role="tablist"
      aria-label="Stav faktury"
      className="-mx-4 flex gap-1.5 overflow-x-auto px-4 pb-1 [scrollbar-width:none] md:mx-0 md:flex-wrap md:px-0 [&::-webkit-scrollbar]:hidden"
    >
      {statusTabs.map((t) => {
        const active = t.value === value
        return (
          <button
            key={t.label}
            type="button"
            role="tab"
            aria-selected={active}
            onClick={() => onChange(t.value)}
            className={cn(
              'h-9 shrink-0 rounded-full border px-3.5 text-sm font-medium transition-colors md:h-8 md:px-3',
              'focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none',
              active
                ? 'border-primary bg-primary text-primary-foreground'
                : 'bg-background text-muted-foreground hover:bg-muted hover:text-foreground dark:bg-input/30',
            )}
          >
            {t.label}
          </button>
        )
      })}
    </div>
  )
}

function SearchInput({ value, onChange, className }: { value: string; onChange: (v: string) => void; className?: string }) {
  return (
    <InputGroup className={cn('h-11 md:h-8', className)}>
      <InputGroupAddon>
        <SearchIcon />
      </InputGroupAddon>
      <InputGroupInput
        type="search"
        inputMode="search"
        enterKeyHint="search"
        placeholder="Číslo, odběratel, VS…"
        aria-label="Hledat faktury"
        value={value}
        onChange={(e) => onChange(e.target.value)}
      />
      {value && (
        <InputGroupAddon align="inline-end">
          <InputGroupButton size="icon-xs" aria-label="Vymazat hledání" onClick={() => onChange('')}>
            <XIcon />
          </InputGroupButton>
        </InputGroupAddon>
      )}
    </InputGroup>
  )
}

function DocTypeSelect({ value, onChange, className }: { value: InvoiceSearch['document_type']; onChange: (p: Patch) => void; className?: string }) {
  return (
    <Select
      items={docTypeItems}
      value={value ?? 'all'}
      onValueChange={(v) => onChange({ document_type: v === 'all' || v === null ? undefined : (v as InvoiceSearch['document_type']) })}
    >
      <SelectTrigger className={className} aria-label="Typ dokladu">
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {docTypeItems.map((o) => (
          <SelectItem key={o.value} value={o.value}>
            {o.label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}

function SortSelect({ value, onChange, className }: { value: InvoiceSearch['sort']; onChange: (p: Patch) => void; className?: string }) {
  return (
    <Select
      items={sortItems}
      value={value ?? DEFAULT_SORT}
      onValueChange={(v) => onChange({ sort: v === DEFAULT_SORT || v === null ? undefined : (v as InvoiceSort) })}
    >
      <SelectTrigger className={className} aria-label="Řazení">
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {sortItems.map((o) => (
          <SelectItem key={o.value} value={o.value}>
            {o.label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}

function PeriodFields({ search, onChange }: { search: InvoiceSearch; onChange: (p: Patch) => void }) {
  const sinceId = useId()
  const untilId = useId()
  const presets = datePresets()
  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-wrap gap-1.5">
        {presets.map((p) => {
          const active = search.since === p.since && search.until === p.until
          return (
            <Button
              key={p.id}
              type="button"
              size="sm"
              variant={active ? 'default' : 'outline'}
              className="h-9 rounded-full md:h-7"
              onClick={() => onChange(active ? { since: undefined, until: undefined } : { since: p.since, until: p.until })}
            >
              {p.label}
            </Button>
          )
        })}
      </div>
      <div className="grid grid-cols-2 gap-3">
        <Field>
          <FieldLabel htmlFor={sinceId}>Vystaveno od</FieldLabel>
          <Input id={sinceId} type="date" value={search.since ?? ''} onChange={(e) => onChange({ since: e.target.value || undefined })} />
        </Field>
        <Field>
          <FieldLabel htmlFor={untilId}>Vystaveno do</FieldLabel>
          <Input id={untilId} type="date" value={search.until ?? ''} onChange={(e) => onChange({ until: e.target.value || undefined })} />
        </Field>
      </div>
    </div>
  )
}

/** Hledání + filtry: na mobilu hledání a tlačítko „Filtry“ (Drawer), na desktopu jedna řada. */
export function InvoiceFilterBar({ search, queryInput, onQueryInput, onChange }: Props) {
  const [open, setOpen] = useState(false)
  const extra = extraFilterCount(search)
  const hasPeriod = Boolean(search.since || search.until)

  return (
    <div className="flex flex-col gap-3">
      {/* Mobil */}
      <div className="flex gap-2 md:hidden">
        <SearchInput value={queryInput} onChange={onQueryInput} className="flex-1" />
        <Button variant="outline" size="icon" aria-label={`Filtry${extra ? ` (${extra})` : ''}`} onClick={() => setOpen(true)} className="relative">
          <SlidersHorizontalIcon />
          {extra > 0 && (
            <span className="absolute -top-1 -right-1 flex size-5 items-center justify-center rounded-full bg-primary text-[0.7rem] font-semibold text-primary-foreground">
              {extra}
            </span>
          )}
        </Button>
        <Drawer open={open} onOpenChange={setOpen} showSwipeHandle>
          <DrawerContent>
            <DrawerHeader className="group-data-[swipe-axis=y]/drawer-popup:text-left">
              <DrawerTitle className="text-lg">Filtry</DrawerTitle>
            </DrawerHeader>
            <div className="flex min-h-0 flex-1 flex-col gap-5 overflow-y-auto p-4">
              <FilterGroup label="Typ dokladu">
                <DocTypeSelect value={search.document_type} onChange={onChange} className="w-full" />
              </FilterGroup>
              <FilterGroup label="Období">
                <PeriodFields search={search} onChange={onChange} />
              </FilterGroup>
              <FilterGroup label="Řazení">
                <SortSelect value={search.sort} onChange={onChange} className="w-full" />
              </FilterGroup>
            </div>
            <DrawerFooter className="flex-row pb-[max(1rem,env(safe-area-inset-bottom))] *:flex-1">
              <Button
                variant="outline"
                disabled={extra === 0}
                onClick={() => onChange({ document_type: undefined, since: undefined, until: undefined, sort: undefined })}
              >
                Zrušit filtry
              </Button>
              <Button onClick={() => setOpen(false)}>Hotovo</Button>
            </DrawerFooter>
          </DrawerContent>
        </Drawer>
      </div>

      {/* Desktop */}
      <div className="hidden flex-wrap items-center gap-2 md:flex">
        <SearchInput value={queryInput} onChange={onQueryInput} className="w-72" />
        <DocTypeSelect value={search.document_type} onChange={onChange} className="w-44" />
        <PeriodPopoverless search={search} onChange={onChange} />
        {(extra > 0 || hasPeriod) && (
          <Button
            variant="ghost"
            size="sm"
            onClick={() => onChange({ document_type: undefined, since: undefined, until: undefined, sort: undefined })}
          >
            <XIcon data-icon="inline-start" />
            Zrušit filtry
          </Button>
        )}
      </div>
    </div>
  )
}

/** Desktop: období jako select předvoleb + dvě data. */
function PeriodPopoverless({ search, onChange }: { search: InvoiceSearch; onChange: (p: Patch) => void }) {
  const presets = datePresets()
  const current = presets.find((p) => p.since === search.since && p.until === search.until)
  const items = [
    { value: 'all', label: 'Celé období' },
    ...presets.map((p) => ({ value: p.id, label: p.label })),
    ...(search.since || search.until ? [{ value: 'custom', label: 'Vlastní období' }] : []),
  ]
  const value = current ? current.id : search.since || search.until ? 'custom' : 'all'
  return (
    <div className="flex items-center gap-2">
      <Select
        items={items}
        value={value}
        onValueChange={(v) => {
          if (v === 'all') onChange({ since: undefined, until: undefined })
          const p = presets.find((x) => x.id === v)
          if (p) onChange({ since: p.since, until: p.until })
        }}
      >
        <SelectTrigger className="w-40" aria-label="Období">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {items.map((o) => (
            <SelectItem key={o.value} value={o.value}>
              {o.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <Input
        type="date"
        aria-label="Vystaveno od"
        className="w-36"
        value={search.since ?? ''}
        onChange={(e) => onChange({ since: e.target.value || undefined })}
      />
      <span className="text-muted-foreground">–</span>
      <Input
        type="date"
        aria-label="Vystaveno do"
        className="w-36"
        value={search.until ?? ''}
        onChange={(e) => onChange({ until: e.target.value || undefined })}
      />
    </div>
  )
}

function FilterGroup({ label, children }: { label: string; children: ReactNode }) {
  return (
    <section className="flex flex-col gap-2">
      <h3 className="text-sm font-medium">{label}</h3>
      {children}
    </section>
  )
}
