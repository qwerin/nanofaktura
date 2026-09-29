import type { ReactNode } from 'react'
import { ChevronDownIcon, ExternalLinkIcon } from 'lucide-react'
import { formatDate } from '@/lib/date'

/** Krátký návod k podání přes EPO (sbalitelný). */
export function EpoGuide({ deadline }: { deadline: string }) {
  return (
    <details className="group rounded-xl border bg-card [&_summary::-webkit-details-marker]:hidden">
      <summary className="flex min-h-12 cursor-pointer list-none items-center justify-between gap-3 rounded-xl px-4 py-3 font-medium focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none md:px-5">
        <span>
          Jak podat přes EPO
          <span className="block text-xs font-normal text-muted-foreground">Pět kroků, zhruba deset minut</span>
        </span>
        <ChevronDownIcon className="size-5 shrink-0 text-muted-foreground transition-transform group-open:rotate-180" />
      </summary>
      <ol className="flex flex-col gap-3 px-4 pb-5 text-sm md:px-5 [counter-reset:step]">
        <Step title="Stáhněte oba soubory XML">
          Přiznání (DPHDP3) a kontrolní hlášení (DPHKH1) tlačítky výše. Před stažením zkontrolujte varování — doklady v nich
          se do souborů nezapočítaly.
        </Step>
        <Step title="Otevřete elektronická podání">
          Na portálu{' '}
          <a
            href="https://mojedane.gov.cz"
            target="_blank"
            rel="noopener noreferrer"
            className="inline-flex items-center gap-0.5 font-medium text-primary underline-offset-4 hover:underline"
          >
            MOJE daně
            <ExternalLinkIcon className="size-3" />
          </a>{' '}
          zvolte Elektronická podání (EPO) → „Načtení souboru“ a vyberte stažený XML.
        </Step>
        <Step title="Zkontrolujte a doplňte">
          Spusťte kontrolu formuláře. Doplňte, co aplikace neeviduje — např. kód předmětu plnění v oddílu A.1 nebo pořízení
          zboží a služeb z EU (ř. 3–13, B.1).
        </Step>
        <Step title="Odešlete">
          Podání odešlete přímo z EPO s ověřením (bankovní identita, NIA) nebo přes datovou schránku. Plátce se zpřístupněnou
          datovou schránkou musí podávat elektronicky. Uschovejte si potvrzení o podání.
        </Step>
        <Step title="Zaplaťte daň">
          Přiznání, kontrolní hlášení i platba mají lhůtu <strong className="font-medium">{formatDate(deadline)}</strong>{' '}
          (připadne-li na víkend či svátek, posouvá se na nejbližší pracovní den). Variabilní symbol je DIČ bez „CZ“.
        </Step>
      </ol>
    </details>
  )
}

function Step({ title, children }: { title: string; children: ReactNode }) {
  return (
    <li className="flex gap-3 [counter-increment:step]">
      <span className="flex size-6 shrink-0 items-center justify-center rounded-full bg-primary/10 text-xs font-semibold text-primary before:content-[counter(step)]" aria-hidden="true" />
      <div className="flex flex-col gap-0.5">
        <span className="font-medium">{title}</span>
        <span className="text-muted-foreground">{children}</span>
      </div>
    </li>
  )
}
