// Tržby po měsících — jednoduchý sloupcový graf v HTML/CSS (bez knihovny).
// Jedna řada → bez legendy; barva z tokenu --chart-1, text v textových tokenech.
// Hover/focus/tap na sloupci ukáže tooltip; pro čtečky je k dispozici skrytá tabulka.

import { useState } from 'react'
import { formatMoney, LOCALE } from '@/lib/money'
import { cn } from '@/lib/utils'

const MONTHS_SHORT = ['led', 'úno', 'bře', 'dub', 'kvě', 'čvn', 'čvc', 'srp', 'zář', 'říj', 'lis', 'pro']
const MONTHS_LONG = ['leden', 'únor', 'březen', 'duben', 'květen', 'červen', 'červenec', 'srpen', 'září', 'říjen', 'listopad', 'prosinec']

/** „Hezký“ strop osy: 1, 2, 2.5, 5 × 10^n nad maximem. */
function niceMax(value: number): number {
  if (value <= 0) return 1
  const exp = 10 ** Math.floor(Math.log10(value))
  for (const m of [1, 2, 2.5, 5, 10]) {
    if (m * exp >= value) return m * exp
  }
  return 10 * exp
}

const compact = new Intl.NumberFormat(LOCALE, { notation: 'compact', maximumFractionDigits: 1 })

interface RevenueChartProps {
  /** 12 hodnot v haléřích. */
  values: number[]
  currency: string
  /** Měsíce po tomto indexu (0–11) jsou v budoucnu — kreslí se jen jako prázdné. */
  lastMonthIndex?: number
  className?: string
}

export function RevenueChart({ values, currency, lastMonthIndex = 11, className }: RevenueChartProps) {
  const [active, setActive] = useState<number | null>(null)
  const major = values.map((v) => v / 100)
  const top = niceMax(Math.max(0, ...major))
  const ticks = [0, 0.25, 0.5, 0.75, 1].map((f) => f * top)

  return (
    <figure className={cn('flex flex-col gap-2', className)}>
      <div className="relative mt-3 flex h-48 gap-2 md:h-56">
        {/* Osa Y */}
        <div className="relative w-10 shrink-0 text-right text-[0.7rem] text-muted-foreground tabular-nums" aria-hidden="true">
          {ticks.map((t) => (
            <span key={t} className="absolute right-0 translate-y-1/2 leading-none" style={{ bottom: `${(t / top) * 100}%` }}>
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
              style={{ bottom: `${(t / top) * 100}%` }}
            />
          ))}
          <div className="absolute inset-0 flex items-end" onMouseLeave={() => setActive(null)}>
            {major.map((v, i) => {
              const future = i > lastMonthIndex
              const h = top > 0 ? (Math.max(0, v) / top) * 100 : 0
              const isActive = active === i
              return (
                <button
                  key={i}
                  type="button"
                  aria-label={`${MONTHS_LONG[i]}: ${future ? 'zatím bez dat' : formatMoney(values[i] ?? 0, currency, { hideZeroDecimals: true })}`}
                  onMouseEnter={() => setActive(i)}
                  onFocus={() => setActive(i)}
                  onBlur={() => setActive(null)}
                  onClick={() => setActive(i)}
                  className="group relative flex h-full flex-1 items-end justify-center rounded-sm outline-none focus-visible:bg-muted/60"
                >
                  <span
                    className={cn(
                      'block w-full max-w-6 rounded-t-[4px] transition-[height,opacity] duration-300 mx-[1px]',
                      future ? 'bg-transparent' : 'bg-chart-1',
                      active !== null && !isActive && 'opacity-45',
                    )}
                    style={{ height: future ? 0 : `max(${v > 0 ? 2 : 0}px, ${h}%)` }}
                  />
                  {isActive && !future && (
                    <span
                      role="tooltip"
                      className={cn(
                        'pointer-events-none absolute z-10 mb-2 flex flex-col rounded-md border bg-popover px-2.5 py-1.5 text-left whitespace-nowrap shadow-md',
                        i < 2 ? 'left-0' : i > 9 ? 'right-0' : 'left-1/2 -translate-x-1/2',
                      )}
                      style={{ bottom: `${h}%` }}
                    >
                      <span className="text-xs text-muted-foreground capitalize">{MONTHS_LONG[i]}</span>
                      <span className="text-sm font-semibold text-popover-foreground tabular-nums">
                        {formatMoney(values[i] ?? 0, currency, { hideZeroDecimals: true })}
                      </span>
                    </span>
                  )}
                </button>
              )
            })}
          </div>
        </div>
      </div>
      {/* Osa X */}
      <div className="flex gap-2" aria-hidden="true">
        <div className="w-10 shrink-0" />
        <div className="flex flex-1">
          {MONTHS_SHORT.map((m, i) => (
            <span
              key={m}
              className={cn(
                'flex-1 text-center text-[0.65rem] text-muted-foreground sm:text-xs',
                active === i && 'font-medium text-foreground',
              )}
            >
              {m}
            </span>
          ))}
        </div>
      </div>
      <table className="sr-only">
        <caption>Tržby po měsících</caption>
        <tbody>
          {values.map((v, i) => (
            <tr key={i}>
              <th scope="row">{MONTHS_LONG[i]}</th>
              <td>{formatMoney(v, currency)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </figure>
  )
}
