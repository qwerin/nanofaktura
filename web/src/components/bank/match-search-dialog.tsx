import { useQuery } from '@tanstack/react-query'
import { FileTextIcon, ReceiptIcon, SearchXIcon } from 'lucide-react'
import { useMemo, useState, type ReactNode } from 'react'
import { bankQueries } from '@/api/queries/bank'
import type { BankTransaction } from '@/api/types'
import { SearchInput } from '@/components/expense/search-input'
import { ExpenseStatusBadge } from '@/components/expense/expense-status-badge'
import { StatusBadge } from '@/components/invoice/status-badge'
import { ResponsiveDialog } from '@/components/responsive-dialog'
import { Skeleton } from '@/components/ui/skeleton'
import { Spinner } from '@/components/ui/spinner'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { formatDate } from '@/lib/date'
import { formatMoney } from '@/lib/money'
import { cn } from '@/lib/utils'
import { amountQuery, rankCandidates, symbolsText, transactionTitle } from './format'
import type { MatchTarget } from './use-transaction-actions'
import { SignedAmount } from './transaction-parts'

type DocKind = 'invoice' | 'expense'

interface Row {
  id: number
  kind: DocKind
  number: string
  name: string
  issuedOn: string
  total: number
  remaining: number
  currency: string
  payable: boolean
  badge: ReactNode
}

/**
 * Ruční spárování: hledání faktur (příchozí platba) nebo nákladů (odchozí) podle čísla,
 * klienta/dodavatele, VS nebo částky. Doklady se stejnou částkou jsou nahoře.
 */
export function MatchSearchDialog({
  slug,
  transaction,
  onOpenChange,
  onMatch,
}: {
  slug: string
  transaction: BankTransaction | null
  onOpenChange: (open: boolean) => void
  onMatch: (t: BankTransaction, target: MatchTarget) => Promise<void>
}) {
  return (
    <ResponsiveDialog
      open={transaction !== null}
      onOpenChange={onOpenChange}
      title="Spárovat s dokladem"
      description={
        transaction && (
          <span className="flex flex-wrap items-baseline gap-x-2">
            <SignedAmount amount={transaction.amount} currency={transaction.currency} />
            <span className="min-w-0 truncate">{transactionTitle(transaction)}</span>
            {symbolsText(transaction) && <span className="text-xs">{symbolsText(transaction)}</span>}
          </span>
        )
      }
      className="md:max-w-xl"
    >
      {transaction && <MatchSearch key={transaction.id} slug={slug} transaction={transaction} onMatch={onMatch} onDone={() => onOpenChange(false)} />}
    </ResponsiveDialog>
  )
}

function MatchSearch({
  slug,
  transaction: t,
  onMatch,
  onDone,
}: {
  slug: string
  transaction: BankTransaction
  onMatch: (t: BankTransaction, target: MatchTarget) => Promise<void>
  onDone: () => void
}) {
  const [kind, setKind] = useState<DocKind>(t.amount >= 0 ? 'invoice' : 'expense')
  const [query, setQuery] = useState('')
  const [pendingId, setPendingId] = useState<number | null>(null)
  const amount = amountQuery(query)
  // Částku API nehledá → načíst poslední doklady a filtrovat tady.
  const serverQuery = amount !== null ? '' : query.trim()
  const invoices = useQuery({ ...bankQueries.invoiceCandidates(slug, serverQuery), enabled: kind === 'invoice' })
  const expenses = useQuery({ ...bankQueries.expenseCandidates(slug, serverQuery), enabled: kind === 'expense' })
  const active = kind === 'invoice' ? invoices : expenses

  const rows = useMemo<Row[] | undefined>(() => {
    const raw: Row[] | undefined =
      kind === 'invoice'
        ? invoices.data?.map((i) => ({
            id: i.id,
            kind,
            number: i.number,
            name: i.client_name,
            issuedOn: i.issued_on,
            total: i.total,
            remaining: i.remaining_amount,
            currency: i.currency,
            payable: i.status !== 'cancelled' && i.status !== 'uncollectible',
            badge: <StatusBadge status={i.status} />,
          }))
        : expenses.data?.map((e) => ({
            id: e.id,
            kind,
            number: e.number,
            name: e.supplier_name,
            issuedOn: e.issued_on,
            total: e.total,
            remaining: e.remaining_amount,
            currency: e.currency,
            payable: true,
            badge: <ExpenseStatusBadge status={e.status} />,
          }))
    if (!raw) return undefined
    const filtered = amount !== null ? raw.filter((r) => r.total === amount || r.remaining === amount) : raw
    return rankCandidates(filtered, t.amount, t.currency)
  }, [kind, invoices.data, expenses.data, amount, t.amount, t.currency])

  const pick = async (r: Row) => {
    setPendingId(r.id)
    try {
      await onMatch(t, {
        body: r.kind === 'invoice' ? { invoice_id: r.id } : { expense_id: r.id },
        label: `${r.kind === 'invoice' ? 'faktura' : 'náklad'} ${r.number}`,
      })
      onDone()
    } catch {
      // Chybu už ukázal toast.
    } finally {
      setPendingId(null)
    }
  }

  const abs = Math.abs(t.amount)

  return (
    <div className="flex flex-col gap-3">
      <Tabs value={kind} onValueChange={(v) => setKind(v as DocKind)}>
        <TabsList className="h-11! w-full md:h-8!" aria-label="Typ dokladu">
          <TabsTrigger value="invoice">
            <FileTextIcon />
            Faktury
          </TabsTrigger>
          <TabsTrigger value="expense">
            <ReceiptIcon />
            Náklady
          </TabsTrigger>
        </TabsList>
      </Tabs>
      <SearchInput
        value={query}
        onChange={setQuery}
        placeholder={kind === 'invoice' ? 'Číslo, klient, VS nebo částka…' : 'Číslo, dodavatel, VS nebo částka…'}
      />
      <p className="text-xs text-muted-foreground">
        {amount !== null ? `Doklady s částkou ${formatMoney(amount, t.currency)} (z posledních 50).` : `Zobrazují se doklady v ${t.currency}. Stejná částka je nahoře.`}
      </p>

      <div className={cn('flex flex-col gap-2 transition-opacity', active.isPlaceholderData && 'opacity-60')}>
        {active.isPending ? (
          Array.from({ length: 3 }, (_, i) => <Skeleton key={i} className="h-16 rounded-lg" />)
        ) : active.isError ? (
          <p className="py-6 text-center text-sm text-destructive">Doklady se nepodařilo načíst.</p>
        ) : rows && rows.length === 0 ? (
          <div className="flex flex-col items-center gap-2 py-8 text-center text-sm text-muted-foreground">
            <SearchXIcon className="size-6" />
            {query ? 'Nic nenalezeno. Zkuste jiné číslo nebo jméno.' : 'Žádné doklady v této měně.'}
          </div>
        ) : (
          <ul className="flex max-h-[50dvh] flex-col gap-2 overflow-y-auto md:max-h-96">
            {rows?.map((r) => {
              const exact = r.remaining === abs
              return (
                <li key={`${r.kind}-${r.id}`}>
                  <button
                    type="button"
                    disabled={pendingId !== null}
                    onClick={() => void pick(r)}
                    className={cn(
                      'flex w-full items-center gap-3 rounded-lg border bg-card p-3 text-left transition-colors hover:bg-muted active:bg-muted disabled:opacity-60',
                      exact && 'border-success/50',
                    )}
                  >
                    <div className="flex min-w-0 flex-1 flex-col gap-1">
                      <div className="flex items-baseline gap-2">
                        <span className="font-medium">{r.number}</span>
                        <span className="min-w-0 flex-1 truncate text-sm text-muted-foreground">{r.name}</span>
                      </div>
                      <div className="flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-muted-foreground">
                        {r.badge}
                        <span>{formatDate(r.issuedOn)}</span>
                        {exact && <span className="font-medium text-success">Částka sedí</span>}
                      </div>
                    </div>
                    <div className="flex shrink-0 flex-col items-end">
                      <span className="text-sm font-semibold tabular-nums">{formatMoney(r.total, r.currency)}</span>
                      {r.remaining !== r.total && (
                        <span className="text-xs text-muted-foreground tabular-nums">zbývá {formatMoney(r.remaining, r.currency)}</span>
                      )}
                    </div>
                    {pendingId === r.id && <Spinner className="shrink-0" />}
                  </button>
                </li>
              )
            })}
          </ul>
        )}
      </div>
    </div>
  )
}
