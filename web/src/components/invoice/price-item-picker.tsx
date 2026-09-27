// Výběr položky z ceníku (GET /price-items?query=) — vloží řádek s názvem, jednotkou, cenou a DPH.

import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { PackageIcon, SearchIcon } from 'lucide-react'
import { useState, type KeyboardEvent } from 'react'
import { api, unwrap } from '@/api/client'
import { keys } from '@/api/queries/keys'
import type { ResponseBody } from '@/api/types'
import { ResponsiveDialog } from '@/components/responsive-dialog'
import { InputGroup, InputGroupAddon, InputGroupInput } from '@/components/ui/input-group'
import { Skeleton } from '@/components/ui/skeleton'
import { formatMoney, formatVatRate } from '@/lib/money'
import { cn } from '@/lib/utils'
import { useDebouncedValue } from './use-debounced'

export type PriceItem = ResponseBody<'/api/accounts/{slug}/price-items', 'get'>['items'][number]

export function PriceItemPicker({
  slug,
  open,
  onOpenChange,
  onPick,
}: {
  slug: string
  open: boolean
  onOpenChange: (o: boolean) => void
  onPick: (item: PriceItem) => void
}) {
  return (
    <ResponsiveDialog open={open} onOpenChange={onOpenChange} title="Položka z ceníku" className="sm:max-w-lg">
      {open && (
        <PriceItemSearch
          slug={slug}
          onPick={(i) => {
            onPick(i)
            onOpenChange(false)
          }}
        />
      )}
    </ResponsiveDialog>
  )
}

function PriceItemSearch({ slug, onPick }: { slug: string; onPick: (i: PriceItem) => void }) {
  const [query, setQuery] = useState('')
  const [active, setActive] = useState(0)
  const debounced = useDebouncedValue(query.trim(), 250)
  const result = useQuery({
    // Prefix ['a', slug, 'price-items'] sdílí stránka ceníku.
    queryKey: [...keys.account(slug), 'price-items', 'picker', debounced],
    queryFn: ({ signal }) =>
      unwrap(
        api.GET('/api/accounts/{slug}/price-items', {
          params: { path: { slug }, query: { query: debounced || undefined, per_page: 30 } },
          signal,
        }),
      ),
    placeholderData: keepPreviousData,
  })
  const items = result.data?.items ?? []

  const onKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      setActive((a) => Math.min(a + 1, items.length - 1))
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      setActive((a) => Math.max(a - 1, 0))
    } else if (e.key === 'Enter') {
      e.preventDefault()
      const i = items[active]
      if (i) onPick(i)
    }
  }

  return (
    <div className="flex flex-col gap-3">
      <InputGroup>
        <InputGroupAddon>
          <SearchIcon />
        </InputGroupAddon>
        <InputGroupInput
          autoFocus
          type="search"
          inputMode="search"
          enterKeyHint="search"
          placeholder="Název nebo kód"
          aria-label="Hledat v ceníku"
          value={query}
          onChange={(e) => {
            setQuery(e.target.value)
            setActive(0)
          }}
          onKeyDown={onKeyDown}
        />
      </InputGroup>
      <ul role="listbox" aria-label="Ceník" className="-mx-1 flex max-h-[min(24rem,50dvh)] flex-col gap-0.5 overflow-y-auto px-1">
        {result.isPending &&
          [0, 1, 2].map((i) => (
            <li key={i} className="px-3 py-2.5">
              <Skeleton className="h-4 w-40" />
            </li>
          ))}
        {!result.isPending && items.length === 0 && (
          <li className="flex flex-col items-center gap-2 px-3 py-6 text-center text-sm text-muted-foreground">
            <PackageIcon className="size-5" />
            {debounced ? `Nic pro „${debounced}“.` : 'Ceník je prázdný. Položky přidáte v sekci Ceník.'}
          </li>
        )}
        {items.map((i, idx) => (
          <li
            key={i.id}
            role="option"
            aria-selected={idx === active}
            data-active={idx === active || undefined}
            onMouseMove={() => setActive(idx)}
            onClick={() => onPick(i)}
            className={cn('flex min-h-12 cursor-pointer items-center gap-3 rounded-lg px-3 py-2 data-active:bg-muted')}
          >
            <span className="flex min-w-0 flex-1 flex-col">
              <span className="truncate text-sm font-medium">{i.name}</span>
              <span className="truncate text-xs text-muted-foreground">
                {[i.sku, i.unit_name && `za ${i.unit_name}`, `DPH ${formatVatRate(i.vat_rate_bps)}`].filter(Boolean).join(' · ')}
              </span>
            </span>
            <span className="shrink-0 text-sm font-medium tabular-nums">
              {formatMoney(i.unit_price, i.currency)}
              <span className="block text-right text-[0.7rem] font-normal text-muted-foreground">
                {i.prices_include_vat ? 's DPH' : 'bez DPH'}
              </span>
            </span>
          </li>
        ))}
      </ul>
    </div>
  )
}
