import { useQuery } from '@tanstack/react-query'
import { ChevronDownIcon } from 'lucide-react'
import { useState } from 'react'
import { eventQueries } from '@/api/queries/events'
import { Checkbox } from '@/components/ui/checkbox'
import { Skeleton } from '@/components/ui/skeleton'
import { cn } from '@/lib/utils'
import { ALL_EVENTS, groupCatalog, isCovered, toggleAll, toggleEvent, toggleGroup, type CatalogGroup } from './webhook-events'

/** Výběr událostí z GET /events/catalog: „Všechny“, celé skupiny (`invoice.*`) nebo jednotlivé názvy. */
export function EventPicker({ slug, value, onChange }: { slug: string; value: string[]; onChange: (v: string[]) => void }) {
  const catalog = useQuery(eventQueries.catalog(slug))
  const all = value.includes(ALL_EVENTS)

  return (
    <div className="flex flex-col gap-2">
      <Row
        checked={all}
        onChange={() => onChange(toggleAll(value))}
        label="Všechny události"
        hint="* — včetně událostí přidaných v budoucnu"
        strong
      />
      {catalog.isPending ? (
        <Skeleton className="h-40 w-full" />
      ) : catalog.isError ? (
        <p className="text-sm text-destructive">Katalog událostí se nepodařilo načíst.</p>
      ) : (
        <div className={cn('flex flex-col divide-y rounded-lg border', all && 'opacity-50')}>
          {groupCatalog(catalog.data).map((g) => (
            <Group key={g.prefix} group={g} value={value} disabled={all} onChange={onChange} />
          ))}
        </div>
      )}
    </div>
  )
}

function Group({
  group: g,
  value,
  disabled,
  onChange,
}: {
  group: CatalogGroup
  value: string[]
  disabled: boolean
  onChange: (v: string[]) => void
}) {
  const wildcard = value.includes(g.wildcard)
  const picked = g.entries.filter((e) => value.includes(e.name)).length
  const [open, setOpen] = useState(picked > 0 && !wildcard)
  return (
    <div className="flex flex-col">
      <div className="flex items-center gap-1 pr-1">
        <Row
          checked={disabled || wildcard}
          indeterminate={!wildcard && picked > 0}
          disabled={disabled}
          onChange={() => onChange(toggleGroup(value, g.prefix))}
          label={g.label}
          hint={wildcard ? `${g.wildcard} — všechny` : picked > 0 ? `vybráno ${picked} z ${g.entries.length}` : g.wildcard}
          className="flex-1"
        />
        <button
          type="button"
          aria-expanded={open}
          aria-label={`${open ? 'Skrýt' : 'Zobrazit'} jednotlivé události: ${g.label}`}
          onClick={() => setOpen((o) => !o)}
          className="flex size-11 shrink-0 items-center justify-center rounded-md text-muted-foreground hover:bg-muted md:size-8"
        >
          <ChevronDownIcon className={cn('size-4 transition-transform', open && 'rotate-180')} />
        </button>
      </div>
      {open && (
        <div className="flex flex-col pb-1 pl-7">
          {g.entries.map((e) => (
            <Row
              key={e.name}
              checked={isCovered(value, e.name)}
              disabled={disabled || wildcard}
              onChange={() => onChange(toggleEvent(value, e.name))}
              label={e.description}
              hint={e.name}
              mono
            />
          ))}
        </div>
      )}
    </div>
  )
}

function Row({
  checked,
  indeterminate,
  disabled,
  onChange,
  label,
  hint,
  strong,
  mono,
  className,
}: {
  checked: boolean
  indeterminate?: boolean
  disabled?: boolean
  onChange: () => void
  label: string
  hint?: string
  strong?: boolean
  mono?: boolean
  className?: string
}) {
  return (
    <label className={cn('flex min-h-11 cursor-pointer items-center gap-3 rounded-md px-3 py-1.5 md:min-h-9', disabled && 'cursor-default', className)}>
      <Checkbox checked={checked} indeterminate={indeterminate} disabled={disabled} onCheckedChange={onChange} />
      <span className="flex min-w-0 flex-col">
        <span className={cn('text-sm', strong && 'font-medium')}>{label}</span>
        {hint && <span className={cn('truncate text-xs text-muted-foreground', mono && 'font-mono')}>{hint}</span>}
      </span>
    </label>
  )
}
