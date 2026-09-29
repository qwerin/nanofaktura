// Tržby vs. náklady (seskupené sloupce) + zisk (čára) po měsících — HTML/CSS + SVG, bez knihovny.
// Jedna osa (vše v Kč), barvy jen z tokenů: tržby --chart-1, náklady --chart-4, zisk text-foreground.
// Hover/focus/tap na měsíci ukáže tooltip se všemi třemi hodnotami; tabulka pro čtečky i na vyžádání.

import { useState } from 'react'
import { formatMoney, LOCALE } from '@/lib/money'
import { cn } from '@/lib/utils'
import { axisDomain, MONTHS_LONG, MONTHS_SHORT } from './logic'

const compact = new Intl.NumberFormat(LOCALE, { notation: 'compact', maximumFractionDigits: 1 })

export interface MonthlySeries {
  revenue: number[]
  expenses: number[]
  profit: number[]
}

interface MonthlyChartProps extends MonthlySeries {
  currency: string
  /** Měsíce po tomto indexu (0–11) jsou v budoucnu — nekreslí se. */
  lastMonthIndex?: number
  className?: string
}

const legend = [
  { key: 'revenue', label: 'Tržby', swatch: <span className="size-2.5 rounded-[3px] bg-chart-1" /> },
  { key: 'expenses', label: 'Náklady', swatch: <span className="size-2.5 rounded-[3px] bg-chart-4" /> },
  {
    key: 'profit',
    label: 'Zisk',
    swatch: (
      <span className="relative flex h-2.5 w-4 items-center">
        <span className="h-0.5 w-full rounded-full bg-foreground" />
        <span className="absolute left-1/2 size-2 -translate-x-1/2 rounded-full border-2 border-card bg-foreground" />
      </span>
    ),
  },
] as const

export function MonthlyChart({ revenue, expenses, profit, currency, lastMonthIndex = 11, className }: MonthlyChartProps) {
  const [active, setActive] = useState<number | null>(null)
  const [showTable, setShowTable] = useState(false)
  const visible = (i: number) => i <= lastMonthIndex
  const { min, max, ticks } = axisDomain([...revenue, ...expenses, ...profit.filter((_, i) => visible(i))])
  const range = max - min
  const pct = (minor: number) => ((minor / 100 - min) / range) * 100
  const zero = pct(0)
  const m = (v: number) => formatMoney(v, currency, { hideZeroDecimals: true })

  const profitPoints = profit
    .map((v, i) => (visible(i) ? `${i + 0.5},${100 - pct(v)}` : null))
    .filter(Boolean)
    .join(' ')

  return (
    <figure className={cn('flex flex-col gap-3', className)}>
      <div className="flex flex-wrap items-center justify-between gap-x-4 gap-y-2">
        <ul className="flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-muted-foreground" aria-label="Legenda">
          {legend.map((l) => (
            <li key={l.key} className="flex items-center gap-1.5">
              {l.swatch}
              {l.label}
            </li>
          ))}
        </ul>
        <button
          type="button"
          onClick={() => setShowTable((s) => !s)}
          aria-expanded={showTable}
          className="rounded text-xs font-medium text-primary underline-offset-4 hover:underline focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none"
        >
          {showTable ? 'Zobrazit graf' : 'Zobrazit tabulku'}
        </button>
      </div>

      {!showTable && (
        <div aria-hidden={false}>
          <div className="relative mt-2 flex h-52 gap-2 md:h-64">
            {/* Osa Y */}
            <div className="relative w-10 shrink-0 text-right text-[0.7rem] text-muted-foreground tabular-nums" aria-hidden="true">
              {ticks.map((t) => (
                <span key={t} className="absolute right-0 translate-y-1/2 leading-none" style={{ bottom: `${((t - min) / range) * 100}%` }}>
                  {compact.format(t)}
                </span>
              ))}
            </div>
            {/* Plocha */}
            <div className="relative flex-1">
              {ticks.map((t) => (
                <div
                  key={t}
                  aria-hidden="true"
                  className={cn('absolute inset-x-0 border-t', t === 0 ? 'border-border' : 'border-dashed border-border/60')}
                  style={{ bottom: `${((t - min) / range) * 100}%` }}
                />
              ))}
              {/* Sloupce */}
              <div className="absolute inset-0 flex" onMouseLeave={() => setActive(null)}>
                {revenue.map((_, i) => {
                  const future = !visible(i)
                  const dim = active !== null && active !== i
                  return (
                    <button
                      key={i}
                      type="button"
                      aria-label={
                        future
                          ? `${MONTHS_LONG[i]}: zatím bez dat`
                          : `${MONTHS_LONG[i]}: tržby ${m(revenue[i] ?? 0)}, náklady ${m(expenses[i] ?? 0)}, zisk ${m(profit[i] ?? 0)}`
                      }
                      onMouseEnter={() => setActive(i)}
                      onFocus={() => setActive(i)}
                      onBlur={() => setActive(null)}
                      onClick={() => setActive(i)}
                      className={cn(
                        'relative h-full flex-1 rounded-sm outline-none focus-visible:bg-muted/60',
                        active === i && 'bg-muted/40',
                      )}
                    >
                      {!future && (
                        <span className={cn('absolute inset-x-0 flex justify-center gap-[2px] px-[3px] transition-opacity', dim && 'opacity-40')} style={{ top: 0, bottom: 0 }}>
                          <Bar value={revenue[i] ?? 0} pct={pct} zero={zero} className="bg-chart-1" />
                          <Bar value={expenses[i] ?? 0} pct={pct} zero={zero} className="bg-chart-4" />
                        </span>
                      )}
                    </button>
                  )
                })}
              </div>
              {/* Zisk — čára + body */}
              <svg
                className="pointer-events-none absolute inset-0 size-full overflow-visible"
                viewBox="0 0 12 100"
                preserveAspectRatio="none"
                aria-hidden="true"
              >
                <polyline
                  points={profitPoints}
                  fill="none"
                  className="stroke-foreground"
                  strokeWidth={2}
                  strokeLinejoin="round"
                  strokeLinecap="round"
                  vectorEffect="non-scaling-stroke"
                />
              </svg>
              {profit.map((v, i) =>
                visible(i) ? (
                  <span
                    key={i}
                    aria-hidden="true"
                    className={cn(
                      'pointer-events-none absolute size-2 -translate-x-1/2 translate-y-1/2 rounded-full border-2 border-card bg-foreground box-content transition-transform',
                      active === i && 'scale-125',
                    )}
                    style={{ left: `${((i + 0.5) / 12) * 100}%`, bottom: `${pct(v)}%` }}
                  />
                ) : null,
              )}
              {/* Tooltip */}
              {active !== null && visible(active) && (
                <div
                  role="tooltip"
                  className={cn(
                    'pointer-events-none absolute top-0 z-10 flex min-w-40 flex-col gap-1 rounded-md border bg-popover px-3 py-2 text-left shadow-md',
                  )}
                  style={
                    active < 6
                      ? { left: `calc(${((active + 1) / 12) * 100}% + 4px)` }
                      : { right: `calc(${((12 - active) / 12) * 100}% + 4px)` }
                  }
                >
                  <span className="text-xs font-medium text-muted-foreground capitalize">{MONTHS_LONG[active]}</span>
                  <TipRow swatch="bg-chart-1" label="Tržby" value={m(revenue[active] ?? 0)} />
                  <TipRow swatch="bg-chart-4" label="Náklady" value={m(expenses[active] ?? 0)} />
                  <TipRow swatch="bg-foreground" label="Zisk" value={m(profit[active] ?? 0)} strong />
                </div>
              )}
            </div>
          </div>
          {/* Osa X */}
          <div className="mt-2 flex gap-2" aria-hidden="true">
            <div className="w-10 shrink-0" />
            <div className="flex flex-1">
              {MONTHS_SHORT.map((mo, i) => (
                <span
                  key={mo}
                  className={cn(
                    'flex-1 text-center text-[0.65rem] text-muted-foreground sm:text-xs',
                    active === i && 'font-medium text-foreground',
                  )}
                >
                  {mo}
                </span>
              ))}
            </div>
          </div>
        </div>
      )}

      <table className={cn(showTable ? 'w-full text-sm tabular-nums' : 'sr-only')}>
        <caption className="sr-only">Tržby, náklady a zisk po měsících ({currency})</caption>
        <thead>
          <tr className="border-b text-xs text-muted-foreground">
            <th scope="col" className="py-2 text-left font-medium">Měsíc</th>
            <th scope="col" className="py-2 text-right font-medium">Tržby</th>
            <th scope="col" className="py-2 text-right font-medium">Náklady</th>
            <th scope="col" className="py-2 text-right font-medium">Zisk</th>
          </tr>
        </thead>
        <tbody className="divide-y">
          {revenue.map((v, i) => (
            <tr key={i} className={cn(!visible(i) && 'text-muted-foreground')}>
              <th scope="row" className="py-1.5 text-left font-normal capitalize">{MONTHS_LONG[i]}</th>
              <td className="py-1.5 text-right">{m(v)}</td>
              <td className="py-1.5 text-right">{m(expenses[i] ?? 0)}</td>
              <td className={cn('py-1.5 text-right font-medium', (profit[i] ?? 0) < 0 && 'text-destructive')}>{m(profit[i] ?? 0)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </figure>
  )
}

function Bar({ value, pct, zero, className }: { value: number; pct: (v: number) => number; zero: number; className: string }) {
  const p = pct(value)
  const positive = value >= 0
  const h = Math.abs(p - zero)
  return (
    <span className="relative h-full w-full max-w-4">
      <span
        className={cn('absolute inset-x-0 transition-[height] duration-300', positive ? 'rounded-t-[4px]' : 'rounded-b-[4px]', className)}
        style={
          positive
            ? { bottom: `${zero}%`, height: value === 0 ? 0 : `max(2px, ${h}%)` }
            : { top: `${100 - zero}%`, height: `max(2px, ${h}%)` }
        }
      />
    </span>
  )
}

function TipRow({ swatch, label, value, strong }: { swatch: string; label: string; value: string; strong?: boolean }) {
  return (
    <span className="flex items-center gap-2 text-sm whitespace-nowrap">
      <span className={cn('size-2 shrink-0 rounded-full', swatch)} />
      <span className="text-muted-foreground">{label}</span>
      <span className={cn('ml-auto pl-3 text-popover-foreground tabular-nums', strong && 'font-semibold')}>{value}</span>
    </span>
  )
}
