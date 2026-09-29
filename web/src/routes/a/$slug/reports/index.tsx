import { useQuery } from '@tanstack/react-query'
import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { z } from 'zod'
import { reportQueries } from '@/api/queries/reports'
import { PageBody, PageHeader } from '@/components/page-header'
import { PageError } from '@/components/page-states'
import { IncomeTaxSection } from '@/components/reports/income-tax'
import { MonthlyChart } from '@/components/reports/monthly-chart'
import { ReportsNav, Stepper } from '@/components/reports/reports-nav'
import { TopCustomers } from '@/components/reports/top-customers'
import { Skeleton } from '@/components/ui/skeleton'
import { formatMoney, LOCALE } from '@/lib/money'
import { cn } from '@/lib/utils'

const searchSchema = z.object({
  year: z.number().int().min(2000).max(2100).optional().catch(undefined),
})

export const Route = createFileRoute('/a/$slug/reports/')({
  validateSearch: searchSchema,
  loaderDeps: ({ search }) => ({ year: search.year }),
  loader: ({ context, params, deps }) =>
    context.queryClient.prefetchQuery(reportQueries.overview(params.slug, deps.year)),
  head: () => ({ meta: [{ title: 'Přehledy · NanoFaktura' }] }),
  component: OverviewPage,
})

const daysFmt = new Intl.NumberFormat(LOCALE, { maximumFractionDigits: 1 })

function OverviewPage() {
  const { slug } = Route.useParams()
  const { year } = Route.useSearch()
  const navigate = useNavigate({ from: Route.fullPath })
  const overview = useQuery(reportQueries.overview(slug, year))
  const data = overview.data

  const thisYear = new Date().getFullYear()
  const shownYear = data?.year ?? year ?? thisYear
  const setYear = (y: number) => void navigate({ search: { year: y === thisYear ? undefined : y }, replace: true })
  const currency = data?.currency ?? 'CZK'
  const m = (v: number) => formatMoney(v, currency, { hideZeroDecimals: true })

  const header = (
    <PageHeader title="Přehledy" description="Tržby, náklady, zisk a podklady pro daňové přiznání.">
      <div className="flex flex-col gap-3 pt-1 md:flex-row md:items-center md:justify-between md:pt-0">
        <ReportsNav slug={slug} />
        <Stepper
          label="Rok"
          prevLabel="Předchozí rok"
          nextLabel="Další rok"
          onPrev={() => setYear(shownYear - 1)}
          onNext={() => setYear(shownYear + 1)}
          nextDisabled={shownYear >= thisYear}
        >
          {shownYear}
        </Stepper>
      </div>
    </PageHeader>
  )

  if (overview.isError) {
    return (
      <>
        {header}
        <PageError error={overview.error} reset={() => void overview.refetch()} />
      </>
    )
  }

  return (
    <>
      {header}
      <PageBody className="flex flex-col gap-4 md:gap-6">
        <div className="grid grid-cols-2 gap-3 md:grid-cols-4 md:gap-4">
          <Kpi label="Tržby" value={data && m(data.revenue_total)} hint="Vystavené faktury a dobropisy" />
          <Kpi label="Náklady" value={data && m(data.expenses_total)} hint="Všechny přijaté doklady" />
          <Kpi
            label="Zisk"
            value={data && m(data.profit_total)}
            hint="Tržby − náklady, vč. DPH"
            tone={data && data.profit_total < 0 ? 'destructive' : undefined}
          />
          <Kpi
            label="Průměrná doba úhrady"
            value={
              data &&
              (data.average_days_to_pay === null ? '—' : `${daysFmt.format(data.average_days_to_pay)} dní`)
            }
            hint={
              data &&
              (data.paid_count > 0
                ? `Z ${data.paid_count} ${data.paid_count === 1 ? 'uhrazené faktury' : 'uhrazených faktur'}`
                : 'Zatím žádná uhrazená faktura')
            }
          />
        </div>

        <section className="rounded-xl border bg-card p-4 md:p-5" aria-labelledby="monthly-title">
          <div className="mb-2">
            <h2 id="monthly-title" className="text-base font-semibold tracking-tight">
              Tržby, náklady a zisk po měsících
            </h2>
            <p className="text-xs text-muted-foreground">{currency} vč. DPH · podle data vystavení, cizí měny přepočtené kurzem dokladu</p>
          </div>
          {data ? (
            <MonthlyChart
              revenue={data.revenue_by_month}
              expenses={data.expenses_by_month}
              profit={data.profit_by_month}
              currency={currency}
              lastMonthIndex={shownYear === thisYear ? new Date().getMonth() : shownYear > thisYear ? -1 : 11}
            />
          ) : (
            <Skeleton className="h-64 w-full" />
          )}
        </section>

        {data ? (
          <TopCustomers slug={slug} customers={data.top_customers} revenueTotal={data.revenue_total} currency={currency} />
        ) : (
          <Skeleton className="h-72 w-full rounded-xl" />
        )}
        {data ? (
          <IncomeTaxSection tax={data.income_tax} year={shownYear} currency={currency} />
        ) : (
          <Skeleton className="h-72 w-full rounded-xl" />
        )}
      </PageBody>
    </>
  )
}

function Kpi({
  label,
  value,
  hint,
  tone,
}: {
  label: string
  value?: string
  hint?: string
  tone?: 'destructive'
}) {
  return (
    <div className="flex min-w-0 flex-col gap-0.5 rounded-xl border bg-card p-4 md:p-5">
      <span className="text-xs font-medium text-muted-foreground md:text-sm">{label}</span>
      {value === undefined ? (
        <Skeleton className="mt-1 h-7 w-24" />
      ) : (
        <span
          className={cn(
            'truncate text-lg font-semibold tracking-tight tabular-nums sm:text-xl md:text-2xl',
            tone === 'destructive' && 'text-destructive',
          )}
        >
          {value}
        </span>
      )}
      {hint && <span className="text-xs text-muted-foreground">{hint}</span>}
    </div>
  )
}
