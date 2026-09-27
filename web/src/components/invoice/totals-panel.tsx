import { formatMoney, formatVatRate } from '@/lib/money'
import { cn } from '@/lib/utils'

export interface TotalsData {
  vatRecap: { vatRateBps: number; base: number; vat: number; total: number }[]
  subtotal: number
  vatTotal: number
  rounding: number
  total: number
}

interface TotalsPanelProps {
  totals: TotalsData
  currency: string
  /** Plátce DPH → rekapitulace po sazbách. */
  showVat: boolean
  reverseCharge?: boolean
  paid?: number
  remaining?: number
  className?: string
}

/** Rekapitulace DPH a součty dokladu. */
export function TotalsPanel({ totals, currency, showVat, reverseCharge, paid, remaining, className }: TotalsPanelProps) {
  const m = (v: number) => formatMoney(v, currency)
  return (
    <div className={cn('flex flex-col gap-3 text-sm', className)}>
      {showVat && totals.vatRecap.length > 0 && (
        <table className="w-full tabular-nums">
          <caption className="sr-only">Rekapitulace DPH</caption>
          <thead>
            <tr className="text-xs text-muted-foreground">
              <th scope="col" className="pb-1 text-left font-medium">
                Sazba
              </th>
              <th scope="col" className="pb-1 text-right font-medium">
                Základ
              </th>
              <th scope="col" className="pb-1 text-right font-medium">
                DPH
              </th>
            </tr>
          </thead>
          <tbody>
            {totals.vatRecap.map((r) => (
              <tr key={r.vatRateBps}>
                <th scope="row" className="py-0.5 text-left font-normal text-muted-foreground">
                  {formatVatRate(r.vatRateBps)}
                </th>
                <td className="py-0.5 text-right">{m(r.base)}</td>
                <td className="py-0.5 text-right">{m(r.vat)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      <dl className="flex flex-col gap-1 tabular-nums">
        {showVat && (
          <>
            <Row label="Základ">{m(totals.subtotal)}</Row>
            <Row label={reverseCharge ? 'DPH (přenesená povinnost)' : 'DPH'}>{m(totals.vatTotal)}</Row>
          </>
        )}
        {totals.rounding !== 0 && <Row label="Zaokrouhlení">{m(totals.rounding)}</Row>}
        <div className="mt-1 flex items-baseline justify-between gap-3 border-t pt-2">
          <dt className="font-medium">Celkem</dt>
          <dd className="text-lg font-semibold tracking-tight">{m(totals.total)}</dd>
        </div>
        {paid !== undefined && paid !== 0 && <Row label="Uhrazeno">{m(paid)}</Row>}
        {remaining !== undefined && paid !== undefined && paid !== 0 && (
          <Row label="Zbývá uhradit" strong>
            {m(remaining)}
          </Row>
        )}
      </dl>
    </div>
  )
}

function Row({ label, children, strong }: { label: string; children: React.ReactNode; strong?: boolean }) {
  return (
    <div className="flex items-baseline justify-between gap-3">
      <dt className={cn('text-muted-foreground', strong && 'font-medium text-foreground')}>{label}</dt>
      <dd className={cn(strong && 'font-semibold')}>{children}</dd>
    </div>
  )
}
