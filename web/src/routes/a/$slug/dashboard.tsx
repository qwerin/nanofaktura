import { useQuery } from '@tanstack/react-query'
import { createFileRoute, Link, useNavigate } from '@tanstack/react-router'
import { ChevronLeftIcon, ChevronRightIcon, CircleAlertIcon, FileTextIcon, PlusIcon } from 'lucide-react'
import type { ReactNode } from 'react'
import { z } from 'zod'
import { dashboardQueries } from '@/api/queries/dashboard'
import { invoiceQueries } from '@/api/queries/invoices'
import type { InvoiceSummary } from '@/api/types'
import { ButtonLink } from '@/components/button-link'
import { EmptyState } from '@/components/empty-state'
import { ActivityWidget } from '@/components/events/activity-widget'
import { DueText } from '@/components/invoice/due-text'
import { RevenueChart } from '@/components/invoice/revenue-chart'
import { StatusBadge } from '@/components/invoice/status-badge'
import { useCanEditDocuments } from '@/components/invoice/use-can-edit'
import { PageBody, PageHeader } from '@/components/page-header'
import { PageError } from '@/components/page-states'
import { TodosWidget } from '@/components/todos/todos-widget'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { formatDate } from '@/lib/date'
import { formatMoney } from '@/lib/money'
import { cn } from '@/lib/utils'

const searchSchema = z.object({
  year: z.number().int().min(2000).max(2100).optional().catch(undefined),
})

const OVERDUE = { status: 'overdue', sort: 'due_on' } as const
const RECENT = {} as const

export const Route = createFileRoute('/a/$slug/dashboard')({
  validateSearch: searchSchema,
  loaderDeps: ({ search }) => ({ year: search.year }),
  loader: ({ context, params, deps }) =>
    Promise.all([
      context.queryClient.prefetchQuery(dashboardQueries.get(params.slug, deps.year)),
      context.queryClient.prefetchQuery(invoiceQueries.top(params.slug, OVERDUE, 5)),
      context.queryClient.prefetchQuery(invoiceQueries.top(params.slug, RECENT, 5)),
    ]),
  head: () => ({ meta: [{ title: 'Přehled · NanoFaktura' }] }),
  component: DashboardPage,
})

function DashboardPage() {
  const { slug } = Route.useParams()
  const { year } = Route.useSearch()
  const navigate = useNavigate({ from: Route.fullPath })
  const canEdit = useCanEditDocuments()
  const stats = useQuery(dashboardQueries.get(slug, year))
  const overdue = useQuery(invoiceQueries.top(slug, OVERDUE, 5))
  const recent = useQuery(invoiceQueries.top(slug, RECENT, 5))

  const thisYear = new Date().getFullYear()
  const shownYear = stats.data?.year ?? year ?? thisYear
  const setYear = (y: number) => void navigate({ search: { year: y === thisYear ? undefined : y }, replace: true })
  const currency = stats.data?.currency ?? 'CZK'
  const m = (v: number) => formatMoney(v, currency, { hideZeroDecimals: true })
  const noInvoices = recent.data?.total === 0

  const newAction = canEdit
    ? [{ label: 'Nová faktura', icon: PlusIcon, primary: true, render: <Link to="/a/$slug/invoices/new" params={{ slug }} /> }]
    : []

  if (stats.isError) {
    return (
      <>
        <PageHeader title="Přehled" />
        <PageError error={stats.error} reset={() => void stats.refetch()} />
      </>
    )
  }

  return (
    <>
      <PageHeader title="Přehled" description="Tržby, neuhrazené a faktury po splatnosti." actions={newAction} />
      <PageBody className="flex flex-col gap-4 md:gap-6">
        {noInvoices && (
          <EmptyState
            icon={FileTextIcon}
            title="Vystavte první fakturu"
            description="Přehled tržeb a neuhrazených faktur se zaplní, jakmile vystavíte první doklad."
            action={
              canEdit && (
                <ButtonLink to="/a/$slug/invoices/new" params={{ slug }}>
                  <PlusIcon data-icon="inline-start" />
                  Vystavit fakturu
                </ButtonLink>
              )
            }
            className="bg-card"
          />
        )}

        {/* KPI */}
        <div className="grid grid-cols-2 gap-3 md:grid-cols-3 md:gap-4">
          <Kpi
            className="col-span-2 md:col-span-1"
            label={`Tržby ${shownYear}`}
            value={stats.data && m(stats.data.revenue_total)}
            hint="Vystavené faktury vč. DPH, bez storen"
          />
          <Kpi
            label="Neuhrazeno"
            value={stats.data && m(stats.data.unpaid_total)}
            hint={stats.data && countText(stats.data.unpaid_count)}
            to={{ status: undefined }}
            slug={slug}
          />
          <Kpi
            label="Po splatnosti"
            tone={stats.data && stats.data.overdue_count > 0 ? 'destructive' : undefined}
            value={stats.data && m(stats.data.overdue_total)}
            hint={stats.data && countText(stats.data.overdue_count)}
            to={{ status: 'overdue' }}
            slug={slug}
          />
        </div>

        {/* Graf */}
        <section className="rounded-xl border bg-card p-4 md:p-5">
          <div className="mb-4 flex items-center justify-between gap-2">
            <div>
              <h2 className="text-base font-semibold tracking-tight">Tržby po měsících</h2>
              <p className="text-xs text-muted-foreground">{currency} vč. DPH · podle data vystavení</p>
            </div>
            <div className="flex items-center gap-1" role="group" aria-label="Rok">
              <Button variant="ghost" size="icon" aria-label="Předchozí rok" onClick={() => setYear(shownYear - 1)}>
                <ChevronLeftIcon />
              </Button>
              <span className="w-12 text-center font-medium tabular-nums">{shownYear}</span>
              <Button
                variant="ghost"
                size="icon"
                aria-label="Další rok"
                disabled={shownYear >= thisYear}
                onClick={() => setYear(shownYear + 1)}
              >
                <ChevronRightIcon />
              </Button>
            </div>
          </div>
          {stats.data ? (
            <RevenueChart
              values={stats.data.revenue_by_month}
              currency={currency}
              lastMonthIndex={shownYear === thisYear ? new Date().getMonth() : 11}
            />
          ) : (
            <Skeleton className="h-56 w-full" />
          )}
        </section>

        {/* Seznamy */}
        <div className="grid gap-4 md:gap-6 lg:grid-cols-2">
          <InvoiceListCard
            title="Po splatnosti"
            icon={<CircleAlertIcon className="size-4 text-destructive" />}
            slug={slug}
            query={overdue}
            empty="Nic po splatnosti. Výborně!"
            moreSearch={{ status: 'overdue', sort: 'due_on' }}
          />
          <InvoiceListCard title="Poslední faktury" slug={slug} query={recent} empty="Zatím žádné faktury." moreSearch={{}} />
        </div>

        {/* Úkoly + aktivita */}
        <div className="grid gap-4 md:gap-6 lg:grid-cols-2">
          <TodosWidget slug={slug} />
          <ActivityWidget slug={slug} />
        </div>
      </PageBody>
    </>
  )
}

function countText(n: number) {
  return `${n} ${n === 1 ? 'doklad' : n >= 2 && n <= 4 ? 'doklady' : 'dokladů'}`
}

function Kpi({
  label,
  value,
  hint,
  tone,
  className,
  to,
  slug,
}: {
  label: string
  value?: string
  hint?: string
  tone?: 'destructive'
  className?: string
  to?: { status?: 'overdue' }
  slug?: string
}) {
  const body = (
    <>
      <span className="text-xs font-medium text-muted-foreground md:text-sm">{label}</span>
      {value === undefined ? (
        <Skeleton className="mt-1 h-7 w-28" />
      ) : (
        <span className={cn('truncate text-xl font-semibold tracking-tight tabular-nums md:text-2xl', tone === 'destructive' && 'text-destructive')}>
          {value}
        </span>
      )}
      {hint && <span className="text-xs text-muted-foreground">{hint}</span>}
    </>
  )
  const cls = cn('flex min-w-0 flex-col gap-0.5 rounded-xl border bg-card p-4 md:p-5', className)
  if (to && slug) {
    return (
      <Link
        to="/a/$slug/invoices"
        params={{ slug }}
        search={to}
        className={cn(cls, 'transition-colors hover:bg-muted/40 focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none')}
      >
        {body}
      </Link>
    )
  }
  return <div className={cls}>{body}</div>
}

function InvoiceListCard({
  title,
  icon,
  slug,
  query,
  empty,
  moreSearch,
}: {
  title: string
  icon?: ReactNode
  slug: string
  query: ReturnType<typeof useQuery<{ items: InvoiceSummary[]; total: number }>>
  empty: string
  moreSearch: { status?: 'overdue'; sort?: 'due_on' }
}) {
  const items = query.data?.items
  return (
    <section className="flex flex-col rounded-xl border bg-card">
      <div className="flex items-center justify-between gap-2 px-4 pt-4 pb-2 md:px-5">
        <h2 className="flex items-center gap-2 text-base font-semibold tracking-tight">
          {icon}
          {title}
          {query.data && query.data.total > 0 && <span className="text-sm font-normal text-muted-foreground">{query.data.total}</span>}
        </h2>
        {query.data && query.data.total > 0 && (
          <Link
            to="/a/$slug/invoices"
            params={{ slug }}
            search={moreSearch}
            className="text-sm font-medium text-primary underline-offset-4 hover:underline"
          >
            Zobrazit vše
          </Link>
        )}
      </div>
      {!items ? (
        <div className="flex flex-col gap-2 p-4 pt-2">
          {[0, 1, 2].map((i) => (
            <Skeleton key={i} className="h-10 w-full" />
          ))}
        </div>
      ) : items.length === 0 ? (
        <p className="px-4 pt-2 pb-5 text-sm text-muted-foreground md:px-5">{empty}</p>
      ) : (
        <ul className="divide-y pb-1">
          {items.map((i) => (
            <li key={i.id}>
              <Link
                to="/a/$slug/invoices/$invoiceId"
                params={{ slug, invoiceId: i.id }}
                className="flex min-h-14 items-center gap-3 px-4 py-2.5 transition-colors hover:bg-muted/40 focus-visible:bg-muted/60 focus-visible:outline-none md:px-5"
              >
                <div className="flex min-w-0 flex-1 flex-col">
                  <span className="truncate text-sm font-medium">{i.client_name}</span>
                  <span className="truncate text-xs text-muted-foreground tabular-nums">
                    {i.number} · {formatDate(i.issued_on)}
                  </span>
                </div>
                <div className="flex shrink-0 flex-col items-end gap-0.5">
                  <span className="text-sm font-semibold tabular-nums">{formatMoney(i.remaining_amount !== 0 && i.status !== 'paid' ? i.remaining_amount : i.total, i.currency)}</span>
                  {i.status === 'overdue' ? <DueText invoice={i} className="text-xs" /> : <StatusBadge status={i.status} />}
                </div>
              </Link>
            </li>
          ))}
        </ul>
      )}
    </section>
  )
}
