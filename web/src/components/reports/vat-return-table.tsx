import { useState } from 'react'
import type { VatReturn } from '@/api/types'
import { formatMoney } from '@/lib/money'
import { cn } from '@/lib/utils'
import { vatReturnSections } from './vat-rows'

/**
 * Řádky přiznání k DPH (DPHDP3) srozumitelně česky. Na mobilu i desktopu stejná „definiční“ mřížka
 * (číslo řádku · popis · základ · daň), na mobilu se částky zalamují pod popis.
 */
export function VatReturnTable({ data, currency }: { data: VatReturn; currency: string }) {
  const [showAll, setShowAll] = useState(false)
  const sections = vatReturnSections(data, { hideEmpty: !showAll })
  const m = (v: number | undefined) => (v === undefined ? '' : formatMoney(v, currency))

  return (
    <section className="flex flex-col rounded-xl border bg-card" aria-labelledby="vat-return-title">
      <div className="flex items-start justify-between gap-3 px-4 pt-4 pb-2 md:px-5">
        <div>
          <h2 id="vat-return-title" className="text-base font-semibold tracking-tight">
            Přiznání k DPH
          </h2>
          <p className="text-xs text-muted-foreground">Řádky formuláře DPHDP3 · v XML se zaokrouhlí na celé koruny</p>
        </div>
        <button
          type="button"
          onClick={() => setShowAll((s) => !s)}
          className="shrink-0 rounded text-xs font-medium text-primary underline-offset-4 hover:underline focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none"
        >
          {showAll ? 'Skrýt prázdné řádky' : 'Zobrazit všechny řádky'}
        </button>
      </div>
      <div className="flex flex-col pb-2">
        <div className="hidden grid-cols-[3rem_minmax(0,1fr)_9rem_9rem] gap-3 border-b px-5 pb-2 text-xs font-medium text-muted-foreground md:grid">
          <span>Řádek</span>
          <span>Popis</span>
          <span className="text-right">Základ daně</span>
          <span className="text-right">Daň</span>
        </div>
        {sections.map((s) => (
          <div key={s.title} className="flex flex-col">
            <h3 className="bg-muted/50 px-4 py-1.5 text-xs font-medium tracking-wide text-muted-foreground uppercase md:px-5">
              {s.title}
            </h3>
            <dl className="divide-y">
              {s.rows.map((r) => {
                const zero = (r.base ?? 0) === 0 && (r.vat ?? 0) === 0
                return (
                  <div
                    key={r.line}
                    className={cn(
                      'grid grid-cols-[2.5rem_minmax(0,1fr)] gap-x-3 gap-y-1 px-4 py-2.5 md:grid-cols-[3rem_minmax(0,1fr)_9rem_9rem] md:items-baseline md:px-5',
                      r.total && !zero && 'bg-accent/40',
                    )}
                  >
                    <dt className="contents">
                      <span className="text-xs font-medium text-muted-foreground tabular-nums md:text-sm">ř. {r.line}</span>
                      <span className="flex flex-col">
                        <span className={cn('text-sm', r.total && 'font-medium')}>{r.label}</span>
                        {r.hint && <span className="text-xs text-muted-foreground">{r.hint}</span>}
                      </span>
                    </dt>
                    <dd className="col-start-2 flex flex-wrap justify-between gap-x-4 text-sm tabular-nums md:contents">
                      <span className={cn('md:text-right', zero && 'text-muted-foreground')}>
                        {r.base !== undefined && (
                          <>
                            <span className="text-xs text-muted-foreground md:hidden">Základ </span>
                            {m(r.base)}
                          </>
                        )}
                      </span>
                      <span className={cn('md:text-right', r.total && !zero ? 'font-semibold' : '', zero && 'text-muted-foreground')}>
                        {r.vat !== undefined && (
                          <>
                            <span className="text-xs font-normal text-muted-foreground md:hidden">Daň </span>
                            {m(r.vat)}
                          </>
                        )}
                      </span>
                    </dd>
                  </div>
                )
              })}
            </dl>
          </div>
        ))}
      </div>
    </section>
  )
}
