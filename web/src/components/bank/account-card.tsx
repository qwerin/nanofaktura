import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { ChevronRightIcon, LandmarkIcon, RefreshCwIcon, TimerIcon, UploadIcon } from 'lucide-react'
import { bankQueries } from '@/api/queries/bank'
import type { BankAccount } from '@/api/types'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { Spinner } from '@/components/ui/spinner'
import { bankName } from '@/lib/bank'
import { formatDateTime } from '@/lib/date'
import { cn } from '@/lib/utils'
import { formatSyncedAgo } from './format'
import { useNow } from './use-now'

interface BankAccountOverviewCardProps {
  slug: string
  account: BankAccount
  canEdit: boolean
  canManageSettings: boolean
  /** Vybraný ve filtru seznamu. */
  selected: boolean
  onSelect: () => void
  onImport: () => void
  onSync: () => void
  syncing: boolean
  /** Zbývající sekundy limitu Fio (0 = lze synchronizovat). */
  cooldown: number
}

/**
 * Karta bankovního účtu v přehledu Banky: zůstatek API nevrací → ukazuje poslední synchronizaci
 * a počet nespárovaných pohybů (klik = filtr seznamu), akce Import / Synchronizovat.
 */
export function BankAccountOverviewCard({
  slug,
  account: a,
  canEdit,
  canManageSettings,
  selected,
  onSelect,
  onImport,
  onSync,
  syncing,
  cooldown,
}: BankAccountOverviewCardProps) {
  const now = useNow()
  const unmatched = useQuery(bankQueries.count(slug, { bank_account_id: a.id, state: 'unmatched' }))
  const bankCode = a.number.split('/')[1]
  const bank = bankCode ? (bankName(bankCode) ?? `Banka ${bankCode}`) : 'Zahraniční účet'
  const fio = a.sync_provider === 'fio'
  const count = unmatched.data

  return (
    <div
      className={cn(
        'flex h-full flex-col rounded-xl border bg-card text-card-foreground transition-shadow',
        selected && 'border-primary/60 ring-2 ring-primary/20',
      )}
    >
      <div className="flex items-start gap-3 p-4 pb-3">
        <span className="flex size-10 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary">
          <LandmarkIcon className="size-5" />
        </span>
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
            <h2 className="truncate font-medium">{a.name}</h2>
            {fio && (
              <Badge variant="secondary" className="bg-info/12 text-info dark:bg-info/20">
                <RefreshCwIcon data-icon="inline-start" />
                Fio API
              </Badge>
            )}
          </div>
          <p className="truncate font-mono text-xs text-muted-foreground">{a.number || a.iban}</p>
          <p className="text-xs text-muted-foreground">
            {bank} · {a.currency}
          </p>
        </div>
      </div>

      <button
        type="button"
        onClick={onSelect}
        aria-pressed={selected}
        className="mx-4 flex min-h-12 items-center gap-3 rounded-lg bg-muted/50 px-3 py-2 text-left transition-colors hover:bg-muted"
      >
        <div className="min-w-0 flex-1">
          {count === undefined ? (
            <Skeleton className="h-5 w-24" />
          ) : count === 0 ? (
            <p className="text-sm font-medium text-success">Vše spárováno</p>
          ) : (
            <p className="text-sm">
              <span className="font-semibold tabular-nums">{count}</span>{' '}
              <span className="text-muted-foreground">{count === 1 ? 'nespárovaná platba' : count < 5 ? 'nespárované platby' : 'nespárovaných plateb'}</span>
            </p>
          )}
          <p className="text-xs text-muted-foreground">
            {fio
              ? a.last_synced_at
                ? `Synchronizováno ${formatSyncedAgo(a.last_synced_at, now, formatDateTime)}`
                : 'Zatím nesynchronizováno'
              : 'Pohyby z importovaných výpisů'}
          </p>
        </div>
        <ChevronRightIcon className="size-4 shrink-0 text-muted-foreground" />
      </button>

      {canEdit && (
        <div className="mt-auto flex flex-wrap gap-2 p-4 pt-3">
          <Button variant="outline" className="flex-1" onClick={onImport}>
            <UploadIcon data-icon="inline-start" />
            Importovat výpis
          </Button>
          {fio ? (
            <Button variant="outline" className="flex-1" onClick={onSync} disabled={syncing || cooldown > 0} aria-live="polite">
              {syncing ? <Spinner data-icon="inline-start" /> : cooldown > 0 ? <TimerIcon data-icon="inline-start" /> : <RefreshCwIcon data-icon="inline-start" />}
              {cooldown > 0 ? `Znovu za ${cooldown} s` : 'Synchronizovat'}
            </Button>
          ) : (
            bankCode === '2010' &&
            canManageSettings && (
              <Button variant="ghost" className="flex-1 text-muted-foreground" nativeButton={false} render={<Link to="/a/$slug/settings/bank-accounts" params={{ slug }} />}>
                <RefreshCwIcon data-icon="inline-start" />
                Nastavit Fio API
              </Button>
            )
          )}
        </div>
      )}
    </div>
  )
}
