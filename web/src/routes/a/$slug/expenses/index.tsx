import { useInfiniteQuery, useQuery } from '@tanstack/react-query'
import { createFileRoute, Link, useNavigate } from '@tanstack/react-router'
import { ListFilterIcon, PlusIcon, ReceiptIcon, XIcon } from 'lucide-react'
import { useCallback, useState } from 'react'
import { expenseQueries } from '@/api/queries/expenses'
import type { DocumentSums, ExpenseSummary } from '@/api/types'
import { ButtonLink } from '@/components/button-link'
import { EmptyState } from '@/components/empty-state'
import { ExpenseFilterDialog } from '@/components/expense/expense-filter-dialog'
import { expenseSearchSchema, searchToFilters, type ExpenseSearch } from '@/components/expense/expense-search'
import { ExpenseStatusBadge } from '@/components/expense/expense-status-badge'
import { countActiveFilters, expenseStatusOptions, plural } from '@/components/expense/format'
import { useCanEditDocuments } from '@/components/expense/permissions'
import { SearchInput } from '@/components/expense/search-input'
import { PageBody, PageHeader } from '@/components/page-header'
import { PageError } from '@/components/page-states'
import { ResponsiveList } from '@/components/responsive-list'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'
import { formatDate, formatDueRelative, todayISO } from '@/lib/date'
import { formatMoney } from '@/lib/money'
import { cn } from '@/lib/utils'

export const Route = createFileRoute('/a/$slug/expenses/')({
  validateSearch: expenseSearchSchema,
  loaderDeps: ({ search }) => searchToFilters(search),
  loader: ({ context, params, deps }) => context.queryClient.prefetchInfiniteQuery(expenseQueries.list(params.slug, deps)),
  head: () => ({ meta: [{ title: 'Náklady · NanoFaktura' }] }),
  component: ExpensesPage,
})

const statusChips = [{ value: undefined, label: 'Vše' }, ...expenseStatusOptions] as const

function ExpensesPage() {
  const { slug } = Route.useParams()
  const search = Route.useSearch()
  const navigate = useNavigate({ from: Route.fullPath })
  const canEdit = useCanEditDocuments()
  const [filtersOpen, setFiltersOpen] = useState(false)
  const filters = searchToFilters(search)
  const list = useInfiniteQuery(expenseQueries.list(slug, filters))
  const supplier = useQuery({ ...expenseQueries.supplier(slug, search.subject_id ?? 0), enabled: Boolean(search.subject_id) })

  const setSearch = useCallback(
    (patch: Partial<ExpenseSearch>) => void navigate({ search: (prev) => ({ ...prev, ...patch }), replace: true }),
    [navigate],
  )
  const onQuery = useCallback((q: string) => setSearch({ query: q || undefined }), [setSearch])

  const items = list.data?.pages.flatMap((p) => p.items) ?? undefined
  const total = list.data?.pages[0]?.total ?? 0
  const activeCount = countActiveFilters(search, ['query', 'status'])
  const openDetail = (e: ExpenseSummary) =>
    void navigate({ to: '/a/$slug/expenses/$expenseId', params: { slug, expenseId: e.id } })

  const newAction = {
    label: 'Nový náklad',
    icon: PlusIcon,
    primary: true,
    render: <Link to="/a/$slug/expenses/new" params={{ slug }} />,
  }

  return (
    <>
      <PageHeader title="Náklady" description="Přijaté faktury, účtenky a další výdaje." actions={canEdit ? [newAction] : []} />
      <PageBody className="flex flex-col gap-4">
        <div className="flex flex-col gap-3">
          <div className="flex gap-2">
            <SearchInput value={search.query ?? ''} onChange={onQuery} placeholder="Hledat číslo, dodavatele, VS…" className="flex-1" />
            <Button variant="outline" onClick={() => setFiltersOpen(true)} className="relative shrink-0" aria-label="Filtry">
              <ListFilterIcon data-icon="inline-start" />
              <span className="max-sm:sr-only">Filtry</span>
              {activeCount > 0 && (
                <Badge className="ml-0.5 h-5 min-w-5 px-1.5 tabular-nums max-sm:absolute max-sm:-top-1.5 max-sm:-right-1.5">{activeCount}</Badge>
              )}
            </Button>
          </div>
          <div className="-mx-4 flex gap-1.5 overflow-x-auto px-4 pb-0.5 [scrollbar-width:none] md:mx-0 md:px-0">
            {statusChips.map((c) => {
              const active = search.status === c.value
              return (
                <button
                  key={c.label}
                  type="button"
                  onClick={() => setSearch({ status: c.value })}
                  aria-pressed={active}
                  className={cn(
                    'h-9 shrink-0 rounded-full border px-3.5 text-sm transition-colors md:h-7 md:px-3',
                    active ? 'border-primary bg-primary text-primary-foreground' : 'bg-background text-muted-foreground hover:bg-muted hover:text-foreground',
                  )}
                >
                  {c.label}
                </button>
              )
            })}
          </div>
          {activeCount > 0 && (
            <div className="flex flex-wrap gap-1.5">
              {search.subject_id && (
                <FilterChip label={`Dodavatel: ${supplier.data?.name ?? '…'}`} onRemove={() => setSearch({ subject_id: undefined })} />
              )}
              {search.category && <FilterChip label={`Kategorie: ${search.category}`} onRemove={() => setSearch({ category: undefined })} />}
              {(search.since || search.until) && (
                <FilterChip
                  label={`Vystaveno ${search.since ? `od ${formatDate(search.since)}` : ''} ${search.until ? `do ${formatDate(search.until)}` : ''}`.replace(/\s+/g, ' ')}
                  onRemove={() => setSearch({ since: undefined, until: undefined })}
                />
              )}
            </div>
          )}
        </div>

        {list.isError && !items ? (
          <PageError error={list.error} reset={() => void list.refetch()} />
        ) : (
          <>
            {items && items.length > 0 && <TotalsBar total={total} sums={list.data?.pages[0]?.sums ?? []} />}
            <ResponsiveList
              items={items}
              isLoading={list.isPending}
              getKey={(e) => e.id}
              onRowClick={openDetail}
              className={cn(list.isPlaceholderData && 'opacity-60 transition-opacity')}
              empty={
                countActiveFilters(search) > 0 ? (
                  <EmptyState
                    icon={ReceiptIcon}
                    title="Nic nenalezeno"
                    description="Žádný náklad neodpovídá filtrům."
                    action={
                      <Button variant="outline" onClick={() => void navigate({ search: {}, replace: true })}>
                        Zrušit filtry
                      </Button>
                    }
                  />
                ) : (
                  <EmptyState
                    icon={ReceiptIcon}
                    title="Zatím žádné náklady"
                    description="Zapisujte přijaté faktury a účtenky — na mobilu stačí doklad vyfotit."
                    action={
                      canEdit && (
                        <ButtonLink to="/a/$slug/expenses/new" params={{ slug }}>
                          <PlusIcon data-icon="inline-start" />
                          Nový náklad
                        </ButtonLink>
                      )
                    }
                  />
                )
              }
              columns={[
                {
                  id: 'number',
                  header: 'Číslo',
                  cell: (e) => (
                    <div className="flex flex-col">
                      <span className="font-medium">{e.number}</span>
                      {e.original_number && <span className="text-xs text-muted-foreground">{e.original_number}</span>}
                    </div>
                  ),
                },
                {
                  id: 'supplier',
                  header: 'Dodavatel',
                  className: 'max-w-[18rem]',
                  cell: (e) => (
                    <div className="flex min-w-0 flex-col">
                      <span className="truncate">{e.supplier_name}</span>
                      {(e.category || e.description) && (
                        <span className="truncate text-xs text-muted-foreground">{[e.category, e.description].filter(Boolean).join(' · ')}</span>
                      )}
                    </div>
                  ),
                },
                { id: 'issued', header: 'Vystaveno', cell: (e) => formatDate(e.issued_on) },
                { id: 'due', header: 'Splatnost', cell: (e) => <DueCell expense={e} /> },
                { id: 'status', header: 'Stav', cell: (e) => <ExpenseStatusBadge status={e.status} locked={Boolean(e.locked_at)} /> },
                {
                  id: 'total',
                  header: 'Celkem',
                  align: 'right',
                  cell: (e) => (
                    <div className="flex flex-col items-end">
                      <span className="font-medium">{formatMoney(e.total, e.currency)}</span>
                      {e.status !== 'paid' && e.paid_amount !== 0 && (
                        <span className="text-xs text-muted-foreground">zbývá {formatMoney(e.remaining_amount, e.currency)}</span>
                      )}
                    </div>
                  ),
                },
              ]}
              renderCard={(e) => (
                <div className="flex flex-col gap-1.5">
                  <div className="flex items-baseline gap-3">
                    <span className="min-w-0 flex-1 truncate font-medium">{e.supplier_name}</span>
                    <span className="shrink-0 font-semibold tabular-nums">{formatMoney(e.total, e.currency)}</span>
                  </div>
                  <div className="flex items-center gap-2 text-sm text-muted-foreground">
                    <span className="min-w-0 flex-1 truncate">
                      {[e.number, e.category || e.description].filter(Boolean).join(' · ')}
                    </span>
                    <span className="shrink-0">{formatDate(e.issued_on)}</span>
                  </div>
                  <div className="flex items-center gap-2">
                    <ExpenseStatusBadge status={e.status} locked={Boolean(e.locked_at)} />
                    <span className="flex-1" />
                    {e.status !== 'paid' && e.due_on && (
                      <span className={cn('text-xs', e.status === 'overdue' ? 'text-destructive' : 'text-muted-foreground')}>
                        splatné {formatDueRelative(e.due_on, todayISO())}
                      </span>
                    )}
                  </div>
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
          </>
        )}
      </PageBody>

      <ExpenseFilterDialog
        open={filtersOpen}
        onOpenChange={setFiltersOpen}
        value={search}
        supplierName={supplier.data?.name}
        onApply={(next) => void navigate({ search: next, replace: true })}
      />
    </>
  )
}

function FilterChip({ label, onRemove }: { label: string; onRemove: () => void }) {
  return (
    <span className="inline-flex h-8 max-w-full items-center gap-1 rounded-full border bg-accent pr-1 pl-3 text-sm text-accent-foreground md:h-7">
      <span className="truncate">{label}</span>
      <button type="button" onClick={onRemove} className="flex size-6 shrink-0 items-center justify-center rounded-full hover:bg-foreground/10" aria-label={`Odebrat filtr ${label}`}>
        <XIcon className="size-3.5" />
      </button>
    </span>
  )
}

function DueCell({ expense: e }: { expense: ExpenseSummary }) {
  if (!e.due_on) return <span className="text-muted-foreground">—</span>
  return (
    <div className="flex flex-col">
      <span>{formatDate(e.due_on)}</span>
      {e.status !== 'paid' && (
        <span className={cn('text-xs', e.status === 'overdue' ? 'text-destructive' : 'text-muted-foreground')}>
          {formatDueRelative(e.due_on, todayISO())}
        </span>
      )}
    </div>
  )
}

/** Počet a součty celého filtru po měnách (počítá API přes všechny stránky). */
function TotalsBar({ total, sums }: { total: number; sums: DocumentSums }) {
  const remaining = sums.filter((s) => s.sum_remaining !== 0)
  return (
    <div className="flex flex-wrap items-baseline gap-x-6 gap-y-1 rounded-xl border bg-muted/30 px-4 py-3 text-sm">
      <span className="text-muted-foreground">
        {total} {plural(total, ['náklad', 'náklady', 'nákladů'])}
      </span>
      <span className="flex flex-wrap gap-x-3">
        <span className="text-muted-foreground">Celkem</span>
        {sums.map((s) => (
          <strong key={s.currency} className="font-semibold tabular-nums">
            {formatMoney(s.sum_total, s.currency)}
          </strong>
        ))}
      </span>
      {remaining.length > 0 && (
        <span className="flex flex-wrap gap-x-3">
          <span className="text-muted-foreground">K úhradě</span>
          {remaining.map((s) => (
            <strong key={s.currency} className="font-semibold tabular-nums">
              {formatMoney(s.sum_remaining, s.currency)}
            </strong>
          ))}
        </span>
      )}
    </div>
  )
}
