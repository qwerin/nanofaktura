import { useQuery } from '@tanstack/react-query'
import { PackageIcon, SearchIcon } from 'lucide-react'
import { useState } from 'react'
import { priceItemQueries } from '@/api/queries/price-items'
import type { PriceItem } from '@/api/types'
import { useDebouncedValue } from '@/components/expense/use-debounced-value'
import { ResponsiveDialog } from '@/components/responsive-dialog'
import { InputGroup, InputGroupAddon, InputGroupInput } from '@/components/ui/input-group'
import { Skeleton } from '@/components/ui/skeleton'
import { useCurrentAccount } from '@/hooks/use-current-account'
import { formatMoney, formatVatRate } from '@/lib/money'
import { formatQuantity } from '@/lib/quantity'
import { cn } from '@/lib/utils'
import { StockBadge } from './stock-badge'

interface PriceItemPickerProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  onPick: (item: PriceItem) => void
  title?: string
}

/** Výběr položky z ceníku (hledání v názvu a SKU). Drawer na mobilu, dialog na desktopu. */
export function PriceItemPicker({ open, onOpenChange, onPick, title = 'Vybrat z ceníku' }: PriceItemPickerProps) {
  const { slug } = useCurrentAccount()
  const [query, setQuery] = useState('')
  const debounced = useDebouncedValue(query.trim())
  const results = useQuery({ ...priceItemQueries.search(slug, debounced), enabled: open })

  return (
    <ResponsiveDialog
      open={open}
      onOpenChange={(o) => {
        onOpenChange(o)
        if (!o) setQuery('')
      }}
      title={title}
      className="md:max-w-lg"
    >
      <div className="flex flex-col gap-3">
        <InputGroup className="h-11 md:h-8">
          <InputGroupAddon>
            <SearchIcon />
          </InputGroupAddon>
          <InputGroupInput
            autoFocus
            type="search"
            placeholder="Hledat název nebo SKU…"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            enterKeyHint="search"
            aria-label="Hledat v ceníku"
          />
        </InputGroup>
        <div className="-mx-1 flex max-h-[min(24rem,55dvh)] flex-col overflow-y-auto">
          {results.isPending ? (
            <div className="flex flex-col gap-2 p-1">
              {[0, 1, 2].map((i) => (
                <Skeleton key={i} className="h-12" />
              ))}
            </div>
          ) : (results.data ?? []).length === 0 ? (
            <div className="flex flex-col items-center gap-2 px-4 py-8 text-center text-sm text-muted-foreground">
              <PackageIcon className="size-6" />
              {debounced ? 'Nic nenalezeno.' : 'Ceník je prázdný.'}
            </div>
          ) : (
            <ul className={cn('flex flex-col', results.isPlaceholderData && 'opacity-60')}>
              {results.data?.map((item) => (
                <li key={item.id}>
                  <button
                    type="button"
                    onClick={() => {
                      onPick(item)
                      onOpenChange(false)
                      setQuery('')
                    }}
                    className="flex min-h-12 w-full items-center gap-3 rounded-lg px-2 py-2 text-left hover:bg-muted focus-visible:bg-muted focus-visible:outline-none active:bg-muted"
                  >
                    <div className="flex min-w-0 flex-1 flex-col">
                      <span className="truncate text-sm font-medium">{item.name}</span>
                      <span className="flex items-center gap-1.5 truncate text-xs text-muted-foreground">
                        {item.sku && <span className="font-mono">{item.sku}</span>}
                        {item.sku && '·'}
                        <span>DPH {formatVatRate(item.vat_rate_bps)}</span>
                        {item.track_stock && (
                          <>
                            ·<span>skladem {formatQuantity(item.stock_quantity)} {item.unit_name}</span>
                          </>
                        )}
                      </span>
                    </div>
                    <div className="flex shrink-0 flex-col items-end gap-0.5">
                      <span className="text-sm font-medium tabular-nums">{formatMoney(item.unit_price, item.currency)}</span>
                      <span className="text-[11px] text-muted-foreground">
                        {item.prices_include_vat ? 's DPH' : 'bez DPH'}
                        {item.unit_name && ` / ${item.unit_name}`}
                      </span>
                      {item.low_stock && <StockBadge low />}
                    </div>
                  </button>
                </li>
              ))}
            </ul>
          )}
        </div>
      </div>
    </ResponsiveDialog>
  )
}
