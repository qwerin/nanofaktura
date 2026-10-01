// Podklad pro daňové přiznání OSVČ: skutečné výdaje vs. výdajové paušály (80/60/40/30 %).
// Čistě informativní — nejde o daňové poradenství.

import { CircleCheckIcon, InfoIcon, TrendingDownIcon, TriangleAlertIcon } from 'lucide-react'
import type { IncomeTax } from '@/api/types'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { plural } from '@/components/invoice/status'
import { Badge } from '@/components/ui/badge'
import { formatMoney } from '@/lib/money'
import { cn } from '@/lib/utils'
import { taxOptions, type TaxOption } from './logic'

export function IncomeTaxSection({ tax, year, currency }: { tax: IncomeTax; year: number; currency: string }) {
  const m = (v: number) => formatMoney(v, currency, { hideZeroDecimals: true })
  const { options, bestId } = taxOptions(tax)
  const real = options[0]!
  const flats = options.slice(1)

  return (
    <section className="flex flex-col gap-4 rounded-xl border bg-card p-4 md:p-5" aria-labelledby="income-tax-title">
      <div className="flex flex-col gap-1">
        <h2 id="income-tax-title" className="text-base font-semibold tracking-tight">
          Daň z příjmů OSVČ {year}
        </h2>
        <p className="text-sm text-muted-foreground">
          Srovnání základu daně při skutečných výdajích a při výdajových paušálech. Podle plateb přijatých a
          zaplacených v roce{tax.income !== 0 ? ', u plátce DPH bez daně' : ''}. Platby v cizí měně jsou přepočtené
          kurzem placeného dokladu (kurz ČNB k jeho DUZP).
        </p>
      </div>

      <dl className="grid grid-cols-2 gap-3 rounded-lg bg-muted/50 p-3 md:grid-cols-3 md:p-4">
        <div className="col-span-2 flex flex-col md:col-span-1">
          <dt className="text-xs text-muted-foreground">Příjmy (zaplacené)</dt>
          <dd className="text-xl font-semibold tabular-nums">{m(tax.income)}</dd>
        </div>
        <div className="flex flex-col">
          <dt className="text-xs text-muted-foreground">Skutečné výdaje</dt>
          <dd className="text-base font-medium tabular-nums">{m(tax.real_expenses)}</dd>
        </div>
        <div className="flex flex-col">
          <dt className="text-xs text-muted-foreground">Základ při skut. výdajích</dt>
          <dd className="text-base font-medium tabular-nums">{m(tax.real_tax_base)}</dd>
        </div>
      </dl>

      {tax.invalid_rates > 0 && (
        <Alert variant="destructive">
          <TriangleAlertIcon />
          <AlertTitle>
            {tax.invalid_rates} {plural(tax.invalid_rates, ['platba chybí', 'platby chybí', 'plateb chybí'])} ve výpočtu
          </AlertTitle>
          <AlertDescription>Placený doklad v cizí měně nemá platný kurz. Doplňte kurz na dokladu.</AlertDescription>
        </Alert>
      )}

      <ul className="grid gap-3 sm:grid-cols-2 lg:grid-cols-5" aria-label="Varianty výdajů">
        {[real, ...flats].map((o) => (
          <OptionCard key={o.id} option={o} best={o.id === bestId} m={m} />
        ))}
      </ul>

      <Alert>
        <InfoIcon />
        <AlertTitle>Jen orientační přehled</AlertTitle>
        <AlertDescription>
          <p>
            Nejde o daňové poradenství. Paušál si nelze zvolit libovolně — sazba závisí na druhu činnosti a při
            více činnostech se příjmy dělí. Při paušálu nelze uplatnit slevu na manžela/manželku a daňové
            zvýhodnění na děti, pokud tvoří dílčí základ z podnikání víc než polovinu celkového základu. Údaje
            vycházejí jen z dokladů v aplikaci (bez odpisů, cestovních náhrad a dalších položek). Před podáním
            přiznání vše ověřte, případně s daňovým poradcem.
          </p>
        </AlertDescription>
      </Alert>
    </section>
  )
}

function OptionCard({ option: o, best, m }: { option: TaxOption; best: boolean; m: (v: number) => string }) {
  return (
    <li
      className={cn(
        'relative flex flex-col gap-3 rounded-lg border p-3.5',
        best && 'border-success/60 bg-success/5 ring-1 ring-success/40',
      )}
    >
      <div className="flex items-start justify-between gap-2">
        <div className="min-w-0">
          <h3 className="text-sm font-semibold">{o.title}</h3>
          <p className="text-xs text-muted-foreground">{o.subtitle}</p>
        </div>
        {best && (
          <Badge className="shrink-0 gap-1 bg-success text-success-foreground">
            <CircleCheckIcon />
            Nejnižší základ
          </Badge>
        )}
      </div>
      <dl className="mt-auto flex flex-col gap-1 text-sm">
        <div className="flex items-baseline justify-between gap-2">
          <dt className="text-muted-foreground">Výdaje</dt>
          <dd className="tabular-nums">{m(o.expenses)}</dd>
        </div>
        <div className="flex items-baseline justify-between gap-2">
          <dt className="text-muted-foreground">Základ daně</dt>
          <dd className={cn('font-semibold tabular-nums', best && 'text-success')}>{m(o.taxBase)}</dd>
        </div>
      </dl>
      {o.cap !== undefined && (
        <p className={cn('flex items-center gap-1 text-xs', o.capReached ? 'text-warning-foreground dark:text-warning' : 'text-muted-foreground')}>
          {o.capReached && <TrendingDownIcon className="size-3.5 shrink-0" />}
          {o.capReached ? `Dosažen strop ${m(o.cap)}` : `Strop ${m(o.cap)}`}
        </p>
      )}
    </li>
  )
}
