import { Link } from '@tanstack/react-router'
import { UsersIcon } from 'lucide-react'
import type { TopCustomer } from '@/api/types'
import { formatMoney, LOCALE } from '@/lib/money'
import { customerShare } from './logic'

const pctFmt = new Intl.NumberFormat(LOCALE, { style: 'percent', maximumFractionDigits: 1 })

/** Top 10 odběratelů jako „bar list“: název, částka, podíl na tržbách a vodorovný pruh. */
export function TopCustomers({
  slug,
  customers,
  revenueTotal,
  currency,
}: {
  slug: string
  customers: TopCustomer[]
  revenueTotal: number
  currency: string
}) {
  const max = Math.max(1, ...customers.map((c) => c.total))
  return (
    <section className="flex flex-col rounded-xl border bg-card" aria-labelledby="top-customers-title">
      <div className="px-4 pt-4 pb-2 md:px-5">
        <h2 id="top-customers-title" className="text-base font-semibold tracking-tight">
          Největší odběratelé
        </h2>
        <p className="text-xs text-muted-foreground">Součet faktur a dobropisů v roce · podíl na tržbách</p>
      </div>
      {customers.length === 0 ? (
        <div className="flex flex-col items-center gap-2 px-4 pt-4 pb-8 text-center text-sm text-muted-foreground">
          <UsersIcon className="size-5" />
          Za tento rok zatím žádné vystavené faktury.
        </div>
      ) : (
        <ol className="flex flex-col pb-2">
          {customers.map((c, i) => {
            const share = customerShare(c.total, revenueTotal)
            return (
              <li key={c.subject_id || `${c.name}-${i}`}>
                <Link
                  to="/a/$slug/subjects/$subjectId"
                  params={{ slug, subjectId: c.subject_id }}
                  disabled={!c.subject_id}
                  className="flex flex-col gap-1.5 px-4 py-2.5 transition-colors hover:bg-muted/40 focus-visible:bg-muted/60 focus-visible:outline-none md:px-5"
                >
                  <span className="flex items-baseline gap-2 text-sm">
                    <span className="w-5 shrink-0 text-xs text-muted-foreground tabular-nums">{i + 1}.</span>
                    <span className="min-w-0 flex-1 truncate font-medium">{c.name}</span>
                    <span className="shrink-0 font-semibold tabular-nums">
                      {formatMoney(c.total, currency, { hideZeroDecimals: true })}
                    </span>
                  </span>
                  <span className="flex items-center gap-2 pl-7">
                    <span className="h-1.5 flex-1 overflow-hidden rounded-full bg-muted" aria-hidden="true">
                      <span
                        className="block h-full rounded-full bg-chart-1"
                        style={{ width: `${Math.max(1, (Math.max(0, c.total) / max) * 100)}%` }}
                      />
                    </span>
                    <span className="shrink-0 text-right text-xs whitespace-nowrap text-muted-foreground tabular-nums">
                      {pctFmt.format(share)} · {c.count} {c.count === 1 ? 'doklad' : c.count < 5 ? 'doklady' : 'dokladů'}
                    </span>
                  </span>
                </Link>
              </li>
            )
          })}
        </ol>
      )}
    </section>
  )
}
