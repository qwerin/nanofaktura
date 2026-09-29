import { useInfiniteQuery, useQuery } from '@tanstack/react-query'
import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { LandmarkIcon, ListFilterIcon, SearchXIcon, UploadIcon, WandSparklesIcon, XIcon } from 'lucide-react'
import { useCallback, useState } from 'react'
import { toast } from 'sonner'
import { bankAccountQueries } from '@/api/queries/bank-accounts'
import { bankQueries, useRematch } from '@/api/queries/bank'
import type { BankTransaction } from '@/api/types'
import { BankAccountOverviewCard } from '@/components/bank/account-card'
import { BankFilterDialog } from '@/components/bank/filter-dialog'
import {
  bankSearchSchema,
  baseFilters,
  countBankFilters,
  searchToFilters,
  TABS,
  tabLabels,
  tabOf,
  formatSignedAmount,
  transactionTitle,
  type BankSearch,
  type BankTab,
} from '@/components/bank/format'
import { ImportStatementDialog } from '@/components/bank/import-dialog'
import { MatchSearchDialog } from '@/components/bank/match-search-dialog'
import { useTransactionActions } from '@/components/bank/use-transaction-actions'
import { TransactionList } from '@/components/bank/transaction-list'
import { useBankSync } from '@/components/bank/use-sync'
import { ButtonLink } from '@/components/button-link'
import { EmptyState } from '@/components/empty-state'
import { ConfirmDialog } from '@/components/expense/confirm-dialog'
import { SearchInput } from '@/components/expense/search-input'
import { PageBody, PageHeader, type PageAction } from '@/components/page-header'
import { PageError } from '@/components/page-states'
import { ListSkeleton } from '@/components/skeletons'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'
import { useCurrentAccount } from '@/hooks/use-current-account'
import { formatDate } from '@/lib/date'
import { cn } from '@/lib/utils'

export const Route = createFileRoute('/a/$slug/bank/')({
  validateSearch: bankSearchSchema,
  loaderDeps: ({ search }) => searchToFilters(search),
  loader: ({ context, params, deps }) =>
    Promise.all([
      context.queryClient.prefetchQuery(bankAccountQueries.list(params.slug)),
      context.queryClient.prefetchInfiniteQuery(bankQueries.transactions(params.slug, deps)),
    ]),
  head: () => ({ meta: [{ title: 'Banka · NanoFaktura' }] }),
  component: BankPage,
})

type ImportState = { open: boolean; bankAccountId?: number }

function BankPage() {
  const { slug } = Route.useParams()
  const search = Route.useSearch()
  const navigate = useNavigate({ from: Route.fullPath })
  const { canEdit, canManageSettings } = useCurrentAccount()
  const tab = tabOf(search)
  const filters = searchToFilters(search)
  const base = baseFilters(search)

  const accounts = useQuery(bankAccountQueries.list(slug))
  const list = useInfiniteQuery(bankQueries.transactions(slug, filters))
  const unmatchedCount = useQuery(bankQueries.count(slug, { ...base, state: 'unmatched' }))
  const suggestedCount = useQuery(bankQueries.count(slug, { ...base, state: 'suggested' }))
  const actions = useTransactionActions(slug)
  const sync = useBankSync(slug)
  const rematch = useRematch(slug)

  const [importState, setImportState] = useState<ImportState>({ open: false })
  const [filtersOpen, setFiltersOpen] = useState(false)
  const [searching, setSearching] = useState<BankTransaction | null>(null)
  const [unmatching, setUnmatching] = useState<BankTransaction | null>(null)

  const setSearch = useCallback(
    (patch: Partial<BankSearch>) => void navigate({ search: (prev) => ({ ...prev, ...patch }), replace: true }),
    [navigate],
  )
  const onQuery = useCallback((q: string) => setSearch({ query: q || undefined }), [setSearch])

  const bankAccounts = accounts.data ?? []
  const items = list.data?.pages.flatMap((p) => p.items)
  const total = list.data?.pages[0]?.total ?? 0
  const activeFilters = countBankFilters(search)
  const accountName = (id: number) => bankAccounts.find((a) => a.id === id)?.name
  const counts: Partial<Record<BankTab, number | undefined>> = { unmatched: unmatchedCount.data, suggested: suggestedCount.data }

  const runRematch = () =>
    rematch.mutate(undefined, {
      onSuccess: (r) =>
        toast.success(
          r.processed === 0
            ? 'Není co párovat — všechny platby jsou vyřízené.'
            : `Prošlo ${r.processed} plateb: ${r.matched} spárováno, ${r.suggestions} s návrhem.`,
        ),
    })

  const pageActions: PageAction[] =
    canEdit && bankAccounts.length > 0
      ? [
          { label: 'Importovat výpis', icon: UploadIcon, primary: true, onClick: () => setImportState({ open: true }) },
          {
            label: 'Znovu spárovat vše',
            icon: WandSparklesIcon,
            onClick: runRematch,
            disabled: rematch.isPending,
          },
        ]
      : []

  if (accounts.isError) {
    return (
      <>
        <PageHeader title="Banka" />
        <PageBody>
          <PageError error={accounts.error} reset={() => void accounts.refetch()} />
        </PageBody>
      </>
    )
  }

  return (
    <>
      <PageHeader title="Banka" description="Pohyby na účtech a párování plateb s fakturami a náklady." actions={pageActions} />
      <PageBody className="flex flex-col gap-5">
        {accounts.isPending ? (
          <ListSkeleton rows={2} />
        ) : bankAccounts.length === 0 ? (
          <EmptyState
            icon={LandmarkIcon}
            title="Nejdřív přidejte bankovní účet"
            description="K účtu pak nahrajete výpis nebo zapnete automatické stahování z Fio banky a platby se spárují s fakturami."
            action={
              canManageSettings && (
                <ButtonLink to="/a/$slug/settings/bank-accounts" params={{ slug }}>
                  Bankovní účty
                </ButtonLink>
              )
            }
          />
        ) : (
          <>
            <section aria-label="Bankovní účty" className="-mx-4 overflow-x-auto px-4 pb-1 [scrollbar-width:none] md:mx-0 md:overflow-visible md:px-0">
              <ul className={cn('flex gap-3 md:grid md:grid-cols-2 xl:grid-cols-3', bankAccounts.length > 1 && 'snap-x snap-mandatory')}>
                {bankAccounts.map((a) => (
                  <li key={a.id} className={cn('shrink-0 snap-start md:w-auto', bankAccounts.length > 1 ? 'w-[85%] sm:w-80' : 'w-full')}>
                    <BankAccountOverviewCard
                      slug={slug}
                      account={a}
                      canEdit={canEdit}
                      canManageSettings={canManageSettings}
                      selected={search.account === a.id}
                      onSelect={() => setSearch({ account: search.account === a.id ? undefined : a.id, tab: undefined })}
                      onImport={() => setImportState({ open: true, bankAccountId: a.id })}
                      onSync={() => sync.run(a)}
                      syncing={sync.pendingId === a.id}
                      cooldown={sync.secondsLeft(a.id)}
                    />
                  </li>
                ))}
              </ul>
            </section>

            <section aria-label="Pohyby" className="flex flex-col gap-3">
              <div className="flex gap-2">
                <SearchInput value={search.query ?? ''} onChange={onQuery} placeholder="Hledat jméno, účet, zprávu, VS…" className="flex-1" />
                <Button variant="outline" onClick={() => setFiltersOpen(true)} className="relative shrink-0" aria-label="Filtry">
                  <ListFilterIcon data-icon="inline-start" />
                  <span className="max-sm:sr-only">Filtry</span>
                  {activeFilters > 0 && (
                    <Badge className="ml-0.5 h-5 min-w-5 px-1.5 tabular-nums max-sm:absolute max-sm:-top-1.5 max-sm:-right-1.5">{activeFilters}</Badge>
                  )}
                </Button>
              </div>

              <div role="tablist" aria-label="Stav párování" className="-mx-4 flex gap-1.5 overflow-x-auto px-4 pb-0.5 [scrollbar-width:none] md:mx-0 md:px-0">
                {TABS.map((t) => {
                  const active = tab === t
                  const count = counts[t]
                  return (
                    <button
                      key={t}
                      type="button"
                      role="tab"
                      aria-selected={active}
                      onClick={() => setSearch({ tab: t === 'unmatched' ? undefined : t })}
                      className={cn(
                        'inline-flex h-9 shrink-0 items-center gap-1.5 rounded-full border px-3.5 text-sm transition-colors md:h-7 md:px-3',
                        active ? 'border-primary bg-primary text-primary-foreground' : 'bg-background text-muted-foreground hover:bg-muted hover:text-foreground',
                      )}
                    >
                      {tabLabels[t]}
                      {count !== undefined && count > 0 && (
                        <span
                          className={cn(
                            'min-w-5 rounded-full px-1.5 text-xs leading-5 tabular-nums',
                            active ? 'bg-primary-foreground/20' : 'bg-muted text-foreground',
                          )}
                        >
                          {count}
                        </span>
                      )}
                    </button>
                  )
                })}
              </div>

              {activeFilters > 0 && (
                <div className="flex flex-wrap gap-1.5">
                  {search.account && (
                    <FilterChip label={`Účet: ${accountName(search.account) ?? search.account}`} onRemove={() => setSearch({ account: undefined })} />
                  )}
                  {search.direction && (
                    <FilterChip label={search.direction === 'in' ? 'Příchozí' : 'Odchozí'} onRemove={() => setSearch({ direction: undefined })} />
                  )}
                  {(search.since || search.until) && (
                    <FilterChip
                      label={`Zaúčtováno ${search.since ? `od ${formatDate(search.since)}` : ''} ${search.until ? `do ${formatDate(search.until)}` : ''}`.replace(/\s+/g, ' ')}
                      onRemove={() => setSearch({ since: undefined, until: undefined })}
                    />
                  )}
                </div>
              )}

              {list.isError && !items ? (
                <PageError error={list.error} reset={() => void list.refetch()} />
              ) : list.isPending && !items ? (
                <ListSkeleton />
              ) : items && items.length === 0 ? (
                <EmptyTab
                  tab={tab}
                  filtered={activeFilters > 0 || Boolean(search.query)}
                  canImport={canEdit}
                  onImport={() => setImportState({ open: true, bankAccountId: search.account })}
                  onReset={() => void navigate({ search: { tab: search.tab }, replace: true })}
                />
              ) : (
                items && (
                  <>
                    <p className="text-sm text-muted-foreground">
                      {total} {total === 1 ? 'pohyb' : total < 5 ? 'pohyby' : 'pohybů'}
                    </p>
                    <TransactionList
                      slug={slug}
                      items={items}
                      accountName={bankAccounts.length > 1 && !search.account ? accountName : undefined}
                      actions={actions}
                      canEdit={canEdit}
                      onSearch={setSearching}
                      onUnmatch={setUnmatching}
                      className={cn(list.isPlaceholderData && 'opacity-60 transition-opacity')}
                    />
                    {list.hasNextPage && (
                      <div className="flex justify-center">
                        <Button variant="outline" onClick={() => void list.fetchNextPage()} disabled={list.isFetchingNextPage} className="w-full md:w-auto">
                          {list.isFetchingNextPage && <Spinner data-icon="inline-start" />}
                          Načíst další
                        </Button>
                      </div>
                    )}
                  </>
                )
              )}
            </section>
          </>
        )}
      </PageBody>

      <ImportStatementDialog
        slug={slug}
        open={importState.open}
        onOpenChange={(open) => setImportState((s) => ({ ...s, open }))}
        accounts={bankAccounts}
        bankAccountId={importState.bankAccountId}
        onShowUnmatched={(id) => void navigate({ search: { account: id }, replace: true })}
      />
      <BankFilterDialog
        open={filtersOpen}
        onOpenChange={setFiltersOpen}
        value={search}
        accounts={bankAccounts}
        onApply={(next) => void navigate({ search: next, replace: true })}
      />
      <MatchSearchDialog
        slug={slug}
        transaction={searching}
        onOpenChange={(open) => !open && setSearching(null)}
        onMatch={(t, target) => actions.match(t, target)}
      />
      <ConfirmDialog
        open={unmatching !== null}
        onOpenChange={(open) => !open && setUnmatching(null)}
        title="Zrušit spárování?"
        description={
          unmatching && (
            <>
              Platba {formatSignedAmount(unmatching.amount, unmatching.currency)} ({transactionTitle(unmatching)}) se odpojí od dokladu
              a zapsaná úhrada se u dokladu smaže. Doklad se tím může vrátit mezi neuhrazené.
            </>
          )
        }
        confirmLabel="Zrušit spárování"
        cancelLabel="Ponechat"
        destructive
        pending={unmatching ? actions.busy[unmatching.id] === 'unmatch' : false}
        onConfirm={() => {
          if (!unmatching) return
          void actions.unmatch(unmatching).then(() => setUnmatching(null))
        }}
      />
    </>
  )
}

function FilterChip({ label, onRemove }: { label: string; onRemove: () => void }) {
  return (
    <span className="inline-flex h-8 max-w-full items-center gap-1 rounded-full border bg-accent pr-1 pl-3 text-sm text-accent-foreground md:h-7">
      <span className="truncate">{label}</span>
      <button
        type="button"
        onClick={onRemove}
        className="flex size-6 shrink-0 items-center justify-center rounded-full hover:bg-foreground/10"
        aria-label={`Odebrat filtr ${label}`}
      >
        <XIcon className="size-3.5" />
      </button>
    </span>
  )
}

function EmptyTab({
  tab,
  filtered,
  canImport,
  onImport,
  onReset,
}: {
  tab: BankTab
  filtered: boolean
  canImport: boolean
  onImport: () => void
  onReset: () => void
}) {
  if (filtered) {
    return (
      <EmptyState
        icon={SearchXIcon}
        title="Nic nenalezeno"
        description="Žádný pohyb neodpovídá filtrům."
        action={
          <Button variant="outline" onClick={onReset}>
            Zrušit filtry
          </Button>
        }
      />
    )
  }
  const texts: Record<BankTab, { title: string; description: string }> = {
    unmatched: { title: 'Vše spárováno', description: 'Žádná platba nečeká na spárování. Nové pohyby přinese import výpisu nebo synchronizace.' },
    suggested: { title: 'Žádné návrhy', description: 'U nespárovaných plateb teď nemáme doklad, který by seděl.' },
    matched: { title: 'Zatím nic spárováno', description: 'Spárované platby se zapíší jako úhrady faktur a nákladů.' },
    ignored: { title: 'Nic ignorovaného', description: 'Pohyby, které nechcete párovat (poplatky, převody mezi účty), můžete ignorovat.' },
    all: { title: 'Zatím žádné pohyby', description: 'Nahrajte výpis z banky, nebo u Fio účtu zapněte automatickou synchronizaci.' },
  }
  return (
    <EmptyState
      icon={LandmarkIcon}
      title={texts[tab].title}
      description={texts[tab].description}
      action={
        canImport &&
        (tab === 'all' || tab === 'unmatched') && (
          <Button variant="outline" onClick={onImport}>
            <UploadIcon data-icon="inline-start" />
            Importovat výpis
          </Button>
        )
      }
    />
  )
}
