import { useInfiniteQuery } from '@tanstack/react-query'
import { createFileRoute, Link, useNavigate } from '@tanstack/react-router'
import { ArchiveIcon, PackageIcon, PlusIcon, TriangleAlertIcon } from 'lucide-react'
import { useCallback } from 'react'
import { z } from 'zod'
import { priceItemQueries } from '@/api/queries/price-items'
import type { PriceItem, PriceItemListFilters } from '@/api/types'
import { ButtonLink } from '@/components/button-link'
import { EmptyState } from '@/components/empty-state'
import { useCanEditDocuments } from '@/components/expense/permissions'
import { SearchInput } from '@/components/expense/search-input'
import { PageBody, PageHeader } from '@/components/page-header'
import { PageError } from '@/components/page-states'
import { StockBadge } from '@/components/price-item/stock-badge'
import { ResponsiveList } from '@/components/responsive-list'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { formatMoney, formatVatRate } from '@/lib/money'
import { formatQuantity } from '@/lib/quantity'
import { cn } from '@/lib/utils'

const searchSchema = z.object({
  query: z.string().optional().catch(undefined),
  archived: z.boolean().optional().catch(undefined),
  low_stock: z.boolean().optional().catch(undefined),
})
type Search = z.infer<typeof searchSchema>

function toFilters(s: Search): PriceItemListFilters {
  const f: PriceItemListFilters = {}
  const q = s.query?.trim()
  if (q) f.query = q
  if (s.archived) f.archived = true
  if (s.low_stock) f.low_stock = true
  return f
}

export const Route = createFileRoute('/a/$slug/price-items/')({
  validateSearch: searchSchema,
  loaderDeps: ({ search }) => toFilters(search),
  loader: ({ context, params, deps }) => context.queryClient.prefetchInfiniteQuery(priceItemQueries.list(params.slug, deps)),
  head: () => ({ meta: [{ title: 'Ceník · NanoFaktura' }] }),
  component: PriceItemsPage,
})

function PriceItemsPage() {
  const { slug } = Route.useParams()
  const search = Route.useSearch()
  const navigate = useNavigate({ from: Route.fullPath })
  const canEdit = useCanEditDocuments()
  const list = useInfiniteQuery(priceItemQueries.list(slug, toFilters(search)))
  const items = list.data?.pages.flatMap((p) => p.items)
  const total = list.data?.pages[0]?.total ?? 0

  const setSearch = useCallback(
    (patch: Partial<Search>) => void navigate({ search: (prev) => ({ ...prev, ...patch }), replace: true }),
    [navigate],
  )
  const onQuery = useCallback((q: string) => setSearch({ query: q || undefined }), [setSearch])
  const open = (p: PriceItem) => void navigate({ to: '/a/$slug/price-items/$priceItemId', params: { slug, priceItemId: p.id } })
  const filtered = Boolean(search.query || search.low_stock)

  return (
    <>
      <PageHeader
        title="Ceník"
        description="Zboží a služby, které často fakturujete nebo nakupujete."
        actions={canEdit ? [{ label: 'Nová položka', icon: PlusIcon, primary: true, render: <Link to="/a/$slug/price-items/new" params={{ slug }} /> }] : []}
      />
      <PageBody className="flex flex-col gap-4">
        <div className="flex flex-col gap-3 md:flex-row md:items-center">
          <SearchInput value={search.query ?? ''} onChange={onQuery} placeholder="Hledat název nebo SKU…" className="md:max-w-sm" />
          <div className="flex items-center gap-2">
            <Tabs value={search.archived ? 'archived' : 'active'} onValueChange={(v) => setSearch({ archived: v === 'archived' || undefined })}>
              <TabsList className="h-11 md:h-8">
                <TabsTrigger value="active">Aktivní</TabsTrigger>
                <TabsTrigger value="archived">Archiv</TabsTrigger>
              </TabsList>
            </Tabs>
            <Button
              variant={search.low_stock ? 'secondary' : 'outline'}
              aria-pressed={Boolean(search.low_stock)}
              onClick={() => setSearch({ low_stock: search.low_stock ? undefined : true })}
              className={cn('ml-auto md:ml-0', search.low_stock && 'border-warning/50 bg-warning/15 text-warning-foreground dark:text-warning')}
            >
              <TriangleAlertIcon data-icon="inline-start" />
              Nízký stav
            </Button>
          </div>
        </div>

        {list.isError && !items ? (
          <PageError error={list.error} reset={() => void list.refetch()} />
        ) : (
          <ResponsiveList
            items={items}
            isLoading={list.isPending}
            getKey={(p) => p.id}
            onRowClick={open}
            className={cn(list.isPlaceholderData && 'opacity-60 transition-opacity')}
            empty={
              filtered || search.archived ? (
                <EmptyState
                  icon={search.archived ? ArchiveIcon : PackageIcon}
                  title={search.archived && !filtered ? 'Archiv je prázdný' : 'Nic nenalezeno'}
                  description={search.low_stock ? 'Žádná sledovaná položka není pod minimálním stavem.' : undefined}
                />
              ) : (
                <EmptyState
                  icon={PackageIcon}
                  title="Ceník je prázdný"
                  description="Přidejte zboží nebo služby — do faktur i nákladů je pak vložíte jedním klepnutím."
                  action={
                    canEdit && (
                      <ButtonLink to="/a/$slug/price-items/new" params={{ slug }}>
                        <PlusIcon data-icon="inline-start" />
                        Nová položka
                      </ButtonLink>
                    )
                  }
                />
              )
            }
            columns={[
              {
                id: 'name',
                header: 'Název',
                cell: (p) => (
                  <div className="flex min-w-0 flex-col">
                    <span className="truncate font-medium">{p.name}</span>
                    {p.sku && <span className="font-mono text-xs text-muted-foreground">{p.sku}</span>}
                  </div>
                ),
              },
              { id: 'vat', header: 'DPH', cell: (p) => formatVatRate(p.vat_rate_bps) },
              { id: 'stock', header: 'Skladem', cell: (p) => <StockCell item={p} /> },
              {
                id: 'price',
                header: 'Cena',
                align: 'right',
                cell: (p) => <PriceCell item={p} />,
              },
            ]}
            renderCard={(p) => (
              <div className="flex items-start gap-3">
                <div className="flex min-w-0 flex-1 flex-col gap-1">
                  <span className="truncate font-medium">{p.name}</span>
                  <span className="flex flex-wrap items-center gap-x-2 gap-y-1 text-sm text-muted-foreground">
                    {p.sku && <span className="font-mono text-xs">{p.sku}</span>}
                    <span>DPH {formatVatRate(p.vat_rate_bps)}</span>
                    {p.track_stock && (
                      <span className={cn(p.low_stock && 'font-medium text-foreground')}>
                        skladem {formatQuantity(p.stock_quantity)} {p.unit_name}
                      </span>
                    )}
                  </span>
                  {p.low_stock && <StockBadge low />}
                </div>
                <PriceCell item={p} />
              </div>
            )}
            footer={
              list.hasNextPage && (
                <Button variant="outline" onClick={() => void list.fetchNextPage()} disabled={list.isFetchingNextPage} className="w-full md:w-auto">
                  {list.isFetchingNextPage && <Spinner data-icon="inline-start" />}
                  Načíst další
                </Button>
              )
            }
          />
        )}
        {items && items.length > 0 && (
          <p className="text-center text-xs text-muted-foreground">
            {items.length < total ? `Zobrazeno ${items.length} z ${total}` : `${total} ${total === 1 ? 'položka' : total < 5 ? 'položky' : 'položek'}`}
          </p>
        )}
      </PageBody>
    </>
  )
}

function PriceCell({ item: p }: { item: PriceItem }) {
  return (
    <div className="flex shrink-0 flex-col items-end">
      <span className="font-semibold tabular-nums">{formatMoney(p.unit_price, p.currency)}</span>
      <span className="text-xs text-muted-foreground">
        {p.prices_include_vat ? 's DPH' : 'bez DPH'}
        {p.unit_name && ` / ${p.unit_name}`}
      </span>
    </div>
  )
}

function StockCell({ item: p }: { item: PriceItem }) {
  if (!p.track_stock) return <span className="text-muted-foreground">—</span>
  return (
    <span className="flex items-center gap-2">
      <span className="tabular-nums">
        {formatQuantity(p.stock_quantity)} {p.unit_name}
      </span>
      <StockBadge low={p.low_stock} />
    </span>
  )
}
