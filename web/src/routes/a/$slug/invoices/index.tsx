import { keepPreviousData, useInfiniteQuery } from '@tanstack/react-query'
import { createFileRoute, Link, useNavigate } from '@tanstack/react-router'
import { ArrowDownIcon, ArrowUpIcon, FileTextIcon, PlusIcon, SearchXIcon } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { invoiceQueries } from '@/api/queries/invoices'
import type { DocumentSums, InvoiceSummary } from '@/api/types'
import { ButtonLink } from '@/components/button-link'
import { EmptyState } from '@/components/empty-state'
import { useExportMenu } from '@/components/export/use-export-menu'
import { DueText } from '@/components/invoice/due-text'
import {
  DEFAULT_SORT,
  invoiceSearchSchema,
  toApiFilters,
  type InvoiceSearch,
  type InvoiceSort,
} from '@/components/invoice/filters'
import { InvoiceFilterBar, StatusChips } from '@/components/invoice/invoice-filters'
import { StatusBadge } from '@/components/invoice/status-badge'
import { documentTypeShortLabels } from '@/components/invoice/status'
import { useDebouncedValue } from '@/components/invoice/use-debounced'
import { PageBody, PageHeader } from '@/components/page-header'
import { PageError } from '@/components/page-states'
import { ResponsiveList, type ListColumn } from '@/components/responsive-list'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'
import { formatDate } from '@/lib/date'
import { formatMoney } from '@/lib/money'
import { cn } from '@/lib/utils'

export const Route = createFileRoute('/a/$slug/invoices/')({
  validateSearch: invoiceSearchSchema,
  loaderDeps: ({ search }) => toApiFilters(search),
  loader: ({ context, params, deps }) =>
    context.queryClient.prefetchInfiniteQuery(invoiceQueries.list(params.slug, deps)),
  head: () => ({ meta: [{ title: 'Faktury · NanoFaktura' }] }),
  component: InvoicesPage,
})

function InvoicesPage() {
  const { slug } = Route.useParams()
  const search = Route.useSearch()
  const navigate = useNavigate({ from: Route.fullPath })
  const filters = useMemo(() => toApiFilters(search), [search])

  const setSearch = (patch: Partial<InvoiceSearch>) =>
    void navigate({ search: (prev) => ({ ...prev, ...patch }), replace: true })

  // Hledání: okamžitě v inputu, do URL se zapíše s debounce.
  const [queryInput, setQueryInput] = useState(search.query ?? '')
  const debouncedQuery = useDebouncedValue(queryInput, 300)
  useEffect(() => {
    if ((debouncedQuery.trim() || undefined) !== (search.query?.trim() || undefined)) {
      setSearch({ query: debouncedQuery.trim() || undefined })
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps -- reaguje jen na změnu textu
  }, [debouncedQuery])

  const list = useInfiniteQuery({ ...invoiceQueries.list(slug, filters), placeholderData: keepPreviousData })
  const items = useMemo(() => list.data?.pages.flatMap((p) => p.items), [list.data])
  const total = list.data?.pages[0]?.total ?? 0
  const sums = list.data?.pages[0]?.sums ?? []
  const filtered = Object.keys(filters).length > 0

  const exportMenu = useExportMenu({ slug, kind: 'invoices', filters: { ...filters } })

  const newAction = {
    label: 'Nová faktura',
    icon: PlusIcon,
    primary: true,
    render: <Link to="/a/$slug/invoices/new" params={{ slug }} />,
  }

  const sortHeader = (label: string, desc: InvoiceSort, asc?: InvoiceSort) => {
    const current = search.sort ?? DEFAULT_SORT
    const active = current === desc || current === asc
    const next = current === desc && asc ? asc : desc
    const Icon = current === asc ? ArrowUpIcon : ArrowDownIcon
    return (
      <button
        type="button"
        onClick={() => setSearch({ sort: next === DEFAULT_SORT ? undefined : next })}
        className={cn(
          'inline-flex items-center gap-1 rounded hover:text-foreground focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none',
          active && 'text-foreground',
        )}
        aria-label={`Řadit: ${label}`}
      >
        {label}
        <Icon className={cn('size-3.5', !active && 'opacity-0')} aria-hidden="true" />
      </button>
    )
  }

  const columns: ListColumn<InvoiceSummary>[] = [
    {
      id: 'number',
      header: sortHeader('Číslo', '-number'),
      cell: (i) => (
        <div className="flex items-center gap-2">
          <span className="font-medium tabular-nums">{i.number}</span>
          {i.document_type !== 'invoice' && (
            <Badge variant="outline" className="font-normal text-muted-foreground">
              {documentTypeShortLabels[i.document_type]}
            </Badge>
          )}
        </div>
      ),
    },
    {
      id: 'client',
      header: 'Odběratel',
      className: 'max-w-64',
      cell: (i) => <span className="block truncate">{i.client_name}</span>,
    },
    {
      id: 'issued',
      header: sortHeader('Vystaveno', '-issued_on', 'issued_on'),
      cell: (i) => <span className="tabular-nums text-muted-foreground">{formatDate(i.issued_on)}</span>,
    },
    {
      id: 'due',
      header: sortHeader('Splatnost', 'due_on'),
      cell: (i) => (
        <div className="flex flex-col">
          <span className="tabular-nums">{formatDate(i.due_on)}</span>
          <DueText invoice={i} className="text-xs" hidePaid />
        </div>
      ),
    },
    { id: 'status', header: 'Stav', cell: (i) => <StatusBadge status={i.status} /> },
    {
      id: 'total',
      header: sortHeader('Částka', '-total'),
      align: 'right',
      cell: (i) => (
        <div className="flex flex-col items-end">
          <span className="font-medium">{formatMoney(i.total, i.currency)}</span>
          {i.remaining_amount !== 0 && i.remaining_amount !== i.total && i.status !== 'cancelled' && (
            <span className="text-xs text-muted-foreground">zbývá {formatMoney(i.remaining_amount, i.currency)}</span>
          )}
        </div>
      ),
    },
  ]

  return (
    <>
      <PageHeader title="Faktury" description="Vydané faktury, zálohy a opravné doklady." actions={[newAction, exportMenu.action]}>
        <div className="flex flex-col gap-3 pt-1 md:pt-0">
          <InvoiceFilterBar search={search} queryInput={queryInput} onQueryInput={setQueryInput} onChange={setSearch} />
          <StatusChips value={search.status} onChange={(status) => setSearch({ status })} />
        </div>
      </PageHeader>
      <PageBody className="pt-3 md:pt-4">
        {list.isError && !items ? (
          <PageError error={list.error} reset={() => void list.refetch()} />
        ) : (
          <div className={cn('transition-opacity', list.isPlaceholderData && 'opacity-60')}>
            {items && items.length > 0 && <ListSummary total={total} sums={sums} />}
            <ResponsiveList
              items={items}
              isLoading={list.isPending}
              getKey={(i) => i.id}
              onRowClick={(i) =>
                void navigate({ to: '/a/$slug/invoices/$invoiceId', params: { slug, invoiceId: i.id } })
              }
              columns={columns}
              renderCard={(i) => <InvoiceCard invoice={i} />}
              empty={
                filtered ? (
                  <EmptyState
                    icon={SearchXIcon}
                    title="Nic nenalezeno"
                    description="Žádná faktura neodpovídá zvoleným filtrům."
                    action={
                      <Button
                        variant="outline"
                        onClick={() => {
                          setQueryInput('')
                          void navigate({ search: {}, replace: true })
                        }}
                      >
                        Zrušit filtry
                      </Button>
                    }
                  />
                ) : (
                  <EmptyState
                    icon={FileTextIcon}
                    title="Zatím žádné faktury"
                    description="Vystavte první fakturu — zabere to minutu."
                    action={
                      <ButtonLink to="/a/$slug/invoices/new" params={{ slug }}>
                        <PlusIcon data-icon="inline-start" />
                        Vystavit fakturu
                      </ButtonLink>
                    }
                  />
                )
              }
              footer={
                list.hasNextPage ? (
                  <Button
                    variant="outline"
                    className="w-full md:w-auto"
                    disabled={list.isFetchingNextPage}
                    onClick={() => void list.fetchNextPage()}
                  >
                    {list.isFetchingNextPage && <Spinner data-icon="inline-start" />}
                    Načíst další
                  </Button>
                ) : undefined
              }
            />
          </div>
        )}
      </PageBody>
      {exportMenu.drawer}
    </>
  )
}

function InvoiceCard({ invoice: i }: { invoice: InvoiceSummary }) {
  return (
    <div className="flex flex-col gap-1.5">
      <div className="flex items-center justify-between gap-2">
        <span className="flex min-w-0 items-center gap-2">
          <span className="truncate font-medium tabular-nums">{i.number}</span>
          {i.document_type !== 'invoice' && (
            <Badge variant="outline" className="font-normal text-muted-foreground">
              {documentTypeShortLabels[i.document_type]}
            </Badge>
          )}
        </span>
        <StatusBadge status={i.status} />
      </div>
      <div className="flex items-baseline justify-between gap-3">
        <span className="min-w-0 truncate text-sm">{i.client_name}</span>
        <span className="shrink-0 font-semibold tabular-nums">{formatMoney(i.total, i.currency)}</span>
      </div>
      <div className="flex items-center justify-between gap-3 text-xs text-muted-foreground">
        <span className="tabular-nums">{formatDate(i.issued_on)}</span>
        <DueText invoice={i} />
      </div>
    </div>
  )
}

/** Počet a součty celého filtru (po měnách) — počítá API přes všechny stránky. */
function ListSummary({ total, sums }: { total: number; sums: DocumentSums }) {
  return (
    <div className="mb-3 flex flex-wrap items-baseline gap-x-4 gap-y-1 text-sm text-muted-foreground">
      <span>{`${total} ${total === 1 ? 'doklad' : total < 5 ? 'doklady' : 'dokladů'}`}</span>
      {sums.map((s) => (
        <span key={s.currency} className="tabular-nums">
          Celkem <span className="font-medium text-foreground">{formatMoney(s.sum_total, s.currency)}</span>
          {s.sum_remaining !== 0 && (
            <>
              {' · '}neuhrazeno <span className="font-medium text-foreground">{formatMoney(s.sum_remaining, s.currency)}</span>
            </>
          )}
        </span>
      ))}
    </div>
  )
}
