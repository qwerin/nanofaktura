import { useQuery } from '@tanstack/react-query'
import { BadgeCheckIcon, ChevronDownIcon, CircleHelpIcon, CircleSlashIcon, RotateCwIcon, ShieldAlertIcon, type LucideIcon } from 'lucide-react'
import { bankQueries } from '@/api/queries/bank'
import type { Subject, VatRegistryResult } from '@/api/types'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { formatIban } from '@/lib/bank'
import { formatDate } from '@/lib/date'
import { cn } from '@/lib/utils'
import { isAccountPublished, isCzechVatNo } from './format'

type Status = { label: string; icon: LucideIcon; className: string }

function statusOf(r: VatRegistryResult): Status {
  if (!r.registered) return { label: 'Neplátce DPH', icon: CircleSlashIcon, className: 'bg-muted text-muted-foreground' }
  if (r.reliable === false) {
    return {
      label: r.unreliable_since ? `Nespolehlivý plátce od ${formatDate(r.unreliable_since)}` : 'Nespolehlivý plátce',
      icon: ShieldAlertIcon,
      className: 'bg-destructive/10 text-destructive dark:bg-destructive/20',
    }
  }
  return { label: 'Spolehlivý plátce', icon: BadgeCheckIcon, className: 'bg-success/12 text-success dark:bg-success/20' }
}

/**
 * „Stav v registru DPH“ kontaktu (MF ČR): spolehlivost plátce a zveřejněné účty.
 * Načítá se až po zobrazení detailu (ne v loaderu) — registr bývá pomalý nebo v noci nedostupný.
 */
export function VatStatusCard({ slug, subject }: { slug: string; subject: Subject }) {
  const enabled = isCzechVatNo(subject.vat_no)
  const q = useQuery({ ...bankQueries.vatStatus(slug, subject.id), enabled })
  if (!enabled) return null

  return (
    <section className="rounded-xl border bg-card" aria-labelledby="vat-status-title">
      <div className="flex min-h-12 flex-wrap items-center gap-x-3 gap-y-2 px-4 py-3">
        <h3 id="vat-status-title" className="flex-1 text-sm text-muted-foreground">
          Stav v registru DPH
        </h3>
        {q.isPending ? (
          <Skeleton className="h-5 w-32" />
        ) : q.isError ? (
          <span className="flex items-center gap-1">
            <Badge variant="secondary" className="gap-1 bg-muted text-muted-foreground">
              <CircleHelpIcon data-icon="inline-start" />
              Registr nedostupný
            </Badge>
            <Button variant="ghost" size="icon-sm" aria-label="Zkusit znovu" onClick={() => void q.refetch()} disabled={q.isFetching}>
              <RotateCwIcon className={cn(q.isFetching && 'animate-spin')} />
            </Button>
          </span>
        ) : (
          <StatusBadge result={q.data} />
        )}
      </div>
      {q.data && q.data.registered && <PublishedAccounts result={q.data} subject={subject} />}
    </section>
  )
}

function StatusBadge({ result }: { result: VatRegistryResult }) {
  const s = statusOf(result)
  return (
    <Badge variant="secondary" className={cn('gap-1', s.className)}>
      <s.icon data-icon="inline-start" aria-hidden="true" />
      {s.label}
    </Badge>
  )
}

function PublishedAccounts({ result, subject }: { result: VatRegistryResult; subject: Subject }) {
  const accounts = result.published_accounts
  const published = isAccountPublished(subject, accounts)
  return (
    <details className="group border-t">
      <summary className="flex min-h-12 cursor-pointer list-none items-center gap-2 px-4 py-2 text-sm [&::-webkit-details-marker]:hidden">
        <span className="flex-1">
          Zveřejněné účty <span className="text-muted-foreground">({accounts.length})</span>
          {published === false && <span className="mt-0.5 block text-xs text-destructive">Účet uvedený u kontaktu mezi nimi není.</span>}
          {published === true && <span className="mt-0.5 block text-xs text-success">Účet uvedený u kontaktu je zveřejněný.</span>}
        </span>
        <ChevronDownIcon className="size-4 text-muted-foreground transition-transform group-open:rotate-180" />
      </summary>
      {accounts.length === 0 ? (
        <p className="px-4 pb-3 text-sm text-muted-foreground">Plátce nemá zveřejněný žádný účet.</p>
      ) : (
        <ul className="flex flex-col pb-2">
          {accounts.map((a) => (
            <li key={`${a.number}-${a.iban}`} className="flex flex-col px-4 py-1.5">
              <span className="font-mono text-sm break-all">{a.number || formatIban(a.iban)}</span>
              <span className="text-xs text-muted-foreground">
                {a.number && a.iban ? `${formatIban(a.iban)} · ` : ''}zveřejněno {formatDate(a.published_on)}
              </span>
            </li>
          ))}
        </ul>
      )}
    </details>
  )
}
