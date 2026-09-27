import { useInfiniteQuery, useQuery } from '@tanstack/react-query'
import { createFileRoute, Link, useNavigate } from '@tanstack/react-router'
import {
  ArchiveIcon,
  ArchiveRestoreIcon,
  ArrowDownLeftIcon,
  ArrowUpRightIcon,
  PencilIcon,
  Trash2Icon,
  TriangleAlertIcon,
  WarehouseIcon,
} from 'lucide-react'
import { useState, type ReactNode } from 'react'
import { toast } from 'sonner'
import {
  priceItemQueries,
  useArchivePriceItem,
  useDeletePriceItem,
  useDeleteStockMove,
} from '@/api/queries/price-items'
import type { PriceItem, StockMove } from '@/api/types'
import { ConfirmDialog } from '@/components/expense/confirm-dialog'
import { useCanEditDocuments } from '@/components/expense/permissions'
import { PageBody, PageHeader, type PageAction } from '@/components/page-header'
import { PageError } from '@/components/page-states'
import { StockMoveDialog } from '@/components/price-item/stock-move-dialog'
import { ListSkeleton } from '@/components/skeletons'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'
import { formatDate, formatDateTime } from '@/lib/date'
import { formatMoney, formatVatRate } from '@/lib/money'
import { parseId } from '@/lib/params'
import { formatQuantity } from '@/lib/quantity'
import { cn } from '@/lib/utils'

export const Route = createFileRoute('/a/$slug/price-items/$priceItemId/')({
  params: {
    parse: ({ priceItemId }) => ({ priceItemId: parseId(priceItemId) }),
    stringify: ({ priceItemId }) => ({ priceItemId: String(priceItemId) }),
  },
  loader: ({ context, params }) => context.queryClient.prefetchQuery(priceItemQueries.detail(params.slug, params.priceItemId)),
  head: () => ({ meta: [{ title: 'Položka ceníku · NanoFaktura' }] }),
  component: PriceItemDetailPage,
})

function PriceItemDetailPage() {
  const { slug, priceItemId } = Route.useParams()
  const navigate = useNavigate()
  const canEdit = useCanEditDocuments()
  const item = useQuery(priceItemQueries.detail(slug, priceItemId))
  const archive = useArchivePriceItem(slug, priceItemId)
  const remove = useDeletePriceItem(slug)
  const [moveDir, setMoveDir] = useState<'in' | 'out' | null>(null)
  const [deleteOpen, setDeleteOpen] = useState(false)
  const back = { to: '/a/$slug/price-items', params: { slug } } as const

  if (item.isError) return <PageError error={item.error} reset={() => void item.refetch()} />
  if (item.isPending) {
    return (
      <>
        <PageHeader title="Položka ceníku" back={back} />
        <PageBody>
          <ListSkeleton rows={3} />
        </PageBody>
      </>
    )
  }

  const p = item.data
  const actions: PageAction[] = canEdit
    ? [
        { label: 'Upravit', icon: PencilIcon, primary: true, render: <Link to="/a/$slug/price-items/$priceItemId/edit" params={{ slug, priceItemId }} /> },
        {
          label: p.archived ? 'Obnovit z archivu' : 'Archivovat',
          icon: p.archived ? ArchiveRestoreIcon : ArchiveIcon,
          disabled: archive.isPending,
          onClick: () =>
            archive.mutate(!p.archived, {
              onSuccess: () => toast.success(p.archived ? 'Položka obnovena' : 'Položka archivována'),
            }),
        },
        { label: 'Smazat', icon: Trash2Icon, variant: 'destructive', onClick: () => setDeleteOpen(true) },
      ]
    : []

  return (
    <>
      <PageHeader title={p.name} description={p.sku ? `SKU ${p.sku}` : undefined} back={back} actions={actions} />
      <PageBody className="grid gap-4 md:gap-6 lg:grid-cols-[20rem_minmax(0,1fr)] lg:items-start">
        <div className="flex min-w-0 flex-col gap-4 md:gap-6">
          {p.archived && (
            <Alert>
              <ArchiveIcon />
              <AlertTitle>Archivovaná položka</AlertTitle>
              <AlertDescription>V našeptávači dokladů se nenabízí. Obnovit ji můžete v menu akcí.</AlertDescription>
            </Alert>
          )}
          <Card>
            <p className="text-sm text-muted-foreground">Cena za jednotku</p>
            <p className="mt-0.5 text-2xl font-semibold tracking-tight tabular-nums">{formatMoney(p.unit_price, p.currency)}</p>
            <p className="text-sm text-muted-foreground">
              {p.prices_include_vat ? 'včetně DPH' : 'bez DPH'}
              {p.unit_name && ` · za ${p.unit_name}`}
            </p>
            <dl className="mt-4 flex flex-col gap-2 border-t pt-4 text-sm">
              <Row label="Sazba DPH" value={formatVatRate(p.vat_rate_bps)} />
              <Row label="Měna" value={p.currency} />
              <Row label="Jednotka" value={p.unit_name || '—'} />
              {p.sku && <Row label="SKU" value={<span className="font-mono">{p.sku}</span>} />}
            </dl>
            {p.note && <p className="mt-4 rounded-lg bg-muted/50 p-3 text-sm whitespace-pre-wrap">{p.note}</p>}
            <p className="mt-4 text-xs text-muted-foreground">
              Vytvořeno {formatDateTime(p.created_at)}
              {p.archived_at && ` · archivováno ${formatDateTime(p.archived_at)}`}
            </p>
          </Card>
        </div>

        <div className="flex min-w-0 flex-col gap-4 md:gap-6">
          <StockCard item={p} canEdit={canEdit} onMove={setMoveDir} />
          {p.track_stock && <MovesCard item={p} slug={slug} canEdit={canEdit} />}
        </div>
      </PageBody>

      <StockMoveDialog item={p} slug={slug} direction={moveDir} onClose={() => setMoveDir(null)} />
      <ConfirmDialog
        open={deleteOpen}
        onOpenChange={setDeleteOpen}
        title="Smazat položku?"
        description="Smaže se i historie skladových pohybů. Řádky faktur a nákladů zůstanou, jen ztratí vazbu na ceník. Pokud ji chcete jen skrýt, použijte archivaci."
        confirmLabel="Smazat položku"
        destructive
        pending={remove.isPending}
        onConfirm={() =>
          remove.mutate(p.id, {
            onSuccess: () => {
              toast.success('Položka smazána')
              setDeleteOpen(false)
              void navigate({ ...back, replace: true })
            },
          })
        }
      />
    </>
  )
}

function Card({ children, className }: { children: ReactNode; className?: string }) {
  return <section className={cn('rounded-xl border bg-card p-4 text-card-foreground md:p-5', className)}>{children}</section>
}

function Row({ label, value }: { label: string; value: ReactNode }) {
  return (
    <div className="flex justify-between gap-4">
      <dt className="text-muted-foreground">{label}</dt>
      <dd className="text-right">{value}</dd>
    </div>
  )
}

function StockCard({ item: p, canEdit, onMove }: { item: PriceItem; canEdit: boolean; onMove: (d: 'in' | 'out') => void }) {
  if (!p.track_stock) {
    return (
      <Card className="flex items-start gap-3">
        <WarehouseIcon className="mt-0.5 size-5 shrink-0 text-muted-foreground" />
        <div className="flex flex-col gap-1">
          <h2 className="text-base font-semibold tracking-tight">Sklad se nesleduje</h2>
          <p className="text-sm text-muted-foreground">Zapněte sledování v úpravě položky — faktury pak zboží odepisují a náklady naskladňují.</p>
        </div>
      </Card>
    )
  }
  const negative = p.stock_quantity.startsWith('-')
  return (
    <Card className={cn(p.low_stock && 'border-warning/50')}>
      <div className="flex flex-col gap-4 sm:flex-row sm:items-end">
        <div className="flex min-w-0 flex-1 flex-col gap-1">
          <h2 className="flex items-center gap-2 text-sm text-muted-foreground">
            <WarehouseIcon className="size-4" /> Skladem
          </h2>
          <p className={cn('text-3xl font-semibold tracking-tight tabular-nums', negative && 'text-destructive')}>
            {formatQuantity(p.stock_quantity)} <span className="text-lg font-normal text-muted-foreground">{p.unit_name}</span>
          </p>
          <p className="text-sm text-muted-foreground">
            {p.min_stock ? `Minimální stav ${formatQuantity(p.min_stock)} ${p.unit_name}` : 'Bez minimálního stavu'}
          </p>
        </div>
        {canEdit && (
          <div className="grid grid-cols-2 gap-2 sm:flex">
            <Button variant="outline" onClick={() => onMove('in')}>
              <ArrowDownLeftIcon data-icon="inline-start" className="text-success" />
              Příjem
            </Button>
            <Button variant="outline" onClick={() => onMove('out')}>
              <ArrowUpRightIcon data-icon="inline-start" className="text-destructive" />
              Výdej
            </Button>
          </div>
        )}
      </div>
      {p.low_stock && (
        <div className="mt-4 flex items-start gap-2 rounded-lg bg-warning/15 p-3 text-sm text-warning-foreground dark:text-warning">
          <TriangleAlertIcon className="mt-0.5 size-4 shrink-0" />
          <span>Zásoba je na minimu nebo pod ním — je čas doobjednat.</span>
        </div>
      )}
    </Card>
  )
}

function MovesCard({ item, slug, canEdit }: { item: PriceItem; slug: string; canEdit: boolean }) {
  const moves = useInfiniteQuery(priceItemQueries.stockMoves(slug, item.id))
  const remove = useDeleteStockMove(slug, item.id)
  const [deleting, setDeleting] = useState<StockMove | null>(null)
  const list = moves.data?.pages.flatMap((p) => p.items) ?? []

  return (
    <Card>
      <h2 className="mb-3 text-base font-semibold tracking-tight">Pohyby na skladě</h2>
      {moves.isPending ? (
        <ListSkeleton rows={3} />
      ) : moves.isError ? (
        <p className="text-sm text-destructive">Pohyby se nepodařilo načíst.</p>
      ) : list.length === 0 ? (
        <p className="text-sm text-muted-foreground">Zatím žádné pohyby.</p>
      ) : (
        <ul className="flex flex-col divide-y">
          {list.map((m) => {
            const manual = !m.invoice_id && !m.expense_id
            return (
              <li key={m.id} className="flex items-center gap-3 py-2.5">
                <span
                  className={cn(
                    'flex size-9 shrink-0 items-center justify-center rounded-full',
                    m.direction === 'in' ? 'bg-success/10 text-success' : 'bg-destructive/10 text-destructive',
                  )}
                  aria-label={m.direction === 'in' ? 'Příjem' : 'Výdej'}
                >
                  {m.direction === 'in' ? <ArrowDownLeftIcon className="size-4" /> : <ArrowUpRightIcon className="size-4" />}
                </span>
                <div className="flex min-w-0 flex-1 flex-col">
                  <span className="text-sm font-medium tabular-nums">
                    {m.direction === 'in' ? '+' : '−'}
                    {formatQuantity(m.quantity)} {item.unit_name}
                  </span>
                  <span className="flex min-w-0 items-center gap-1.5 text-xs text-muted-foreground">
                    <span className="shrink-0">{formatDate(m.moved_on)}</span>
                    <MoveSource move={m} slug={slug} />
                  </span>
                </div>
                {canEdit && manual && (
                  <Button variant="ghost" size="icon" className="size-9 text-muted-foreground hover:text-destructive md:size-7" aria-label="Smazat pohyb" onClick={() => setDeleting(m)}>
                    <Trash2Icon />
                  </Button>
                )}
              </li>
            )
          })}
        </ul>
      )}
      {moves.hasNextPage && (
        <Button variant="outline" className="mt-3 w-full" onClick={() => void moves.fetchNextPage()} disabled={moves.isFetchingNextPage}>
          {moves.isFetchingNextPage && <Spinner data-icon="inline-start" />}
          Načíst další
        </Button>
      )}
      <ConfirmDialog
        open={deleting !== null}
        onOpenChange={(o) => !o && setDeleting(null)}
        title="Smazat pohyb?"
        description={deleting && `${deleting.direction === 'in' ? 'Příjem' : 'Výdej'} ${formatQuantity(deleting.quantity)} ${item.unit_name} z ${formatDate(deleting.moved_on)} se odebere a stav skladu se přepočítá.`}
        confirmLabel="Smazat pohyb"
        destructive
        pending={remove.isPending}
        onConfirm={() =>
          deleting &&
          remove.mutate(deleting.id, {
            onSuccess: () => {
              toast.success('Pohyb smazán')
              setDeleting(null)
            },
          })
        }
      />
    </Card>
  )
}

function MoveSource({ move: m, slug }: { move: StockMove; slug: string }) {
  if (m.expense_id) {
    return (
      <>
        ·
        <Link to="/a/$slug/expenses/$expenseId" params={{ slug, expenseId: m.expense_id }} className="shrink-0 text-primary hover:underline">
          náklad
        </Link>
      </>
    )
  }
  if (m.invoice_id) {
    return (
      <>
        ·
        <Link to="/a/$slug/invoices/$invoiceId" params={{ slug, invoiceId: m.invoice_id }} className="shrink-0 text-primary hover:underline">
          faktura
        </Link>
      </>
    )
  }
  return (
    <>
      · <Badge variant="outline" className="h-4 px-1.5 text-[10px]">ručně</Badge>
      {m.note && <span className="truncate">{m.note === 'initial stock' ? 'počáteční stav' : m.note}</span>}
    </>
  )
}
