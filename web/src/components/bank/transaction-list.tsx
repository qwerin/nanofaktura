import { Fragment } from 'react'
import type { BankTransaction } from '@/api/types'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { formatDate } from '@/lib/date'
import { cn } from '@/lib/utils'
import { symbolsText, transactionTitle } from './format'
import { TransactionMatchPanel } from './transaction-actions'
import type { TransactionActions } from './use-transaction-actions'
import { DirectionIcon, SignedAmount, TransactionStateBadge } from './transaction-parts'

interface TransactionListProps {
  slug: string
  items: BankTransaction[]
  /** Název účtu u pohybu (jen když se zobrazuje víc účtů). */
  accountName?: (id: number) => string | undefined
  actions: TransactionActions
  canEdit: boolean
  onSearch: (t: BankTransaction) => void
  onUnmatch: (t: BankTransaction) => void
  className?: string
}

/** Pohyby na účtu: karty na mobilu, tabulka s řádkem návrhů na desktopu (SPEC §6). */
export function TransactionList(props: TransactionListProps) {
  const { items, className } = props
  return (
    <div className={className}>
      <ul className="flex flex-col gap-2 md:hidden">
        {items.map((t) => (
          <li key={t.id}>
            <TransactionCard transaction={t} {...props} />
          </li>
        ))}
      </ul>
      <TransactionTable {...props} />
    </div>
  )
}

function Counterparty({ t }: { t: BankTransaction }) {
  const title = transactionTitle(t)
  return (
    <div className="flex min-w-0 flex-col">
      <span className="truncate font-medium">{title}</span>
      {t.counterparty_account && t.counterparty_name && (
        <span className="truncate font-mono text-xs text-muted-foreground">{t.counterparty_account}</span>
      )}
      {t.message && t.message !== title && <span className="line-clamp-2 text-xs text-muted-foreground">{t.message}</span>}
    </div>
  )
}

function TransactionCard({
  transaction: t,
  slug,
  accountName,
  actions,
  canEdit,
  onSearch,
  onUnmatch,
}: TransactionListProps & { transaction: BankTransaction }) {
  const symbols = symbolsText(t)
  const account = accountName?.(t.bank_account_id)
  return (
    <article
      className={cn('rounded-xl border bg-card p-4 text-card-foreground', t.state === 'ignored' && 'opacity-70')}
      aria-label={`${transactionTitle(t)}, ${formatDate(t.booked_on)}`}
    >
      <div className="flex items-start gap-3">
        <DirectionIcon amount={t.amount} />
        <div className="flex min-w-0 flex-1 flex-col gap-1">
          <div className="flex items-baseline gap-3">
            <span className="min-w-0 flex-1 truncate font-medium">{transactionTitle(t)}</span>
            <SignedAmount amount={t.amount} currency={t.currency} className="shrink-0" />
          </div>
          <div className="flex flex-wrap gap-x-2 text-sm text-muted-foreground">
            <span>{formatDate(t.booked_on)}</span>
            {symbols && <span className="tabular-nums">· {symbols}</span>}
          </div>
          {t.message && t.message !== transactionTitle(t) && <p className="line-clamp-2 text-sm text-muted-foreground">{t.message}</p>}
          {t.counterparty_account && t.counterparty_name && (
            <p className="truncate font-mono text-xs text-muted-foreground">{t.counterparty_account}</p>
          )}
          <div className="mt-1 flex flex-wrap items-center gap-2">
            <TransactionStateBadge transaction={t} />
            {account && <span className="truncate text-xs text-muted-foreground">{account}</span>}
          </div>
        </div>
      </div>
      {(canEdit || t.state === 'matched') && (
        <div className="mt-3 border-t pt-3">
          <TransactionMatchPanel slug={slug} transaction={t} actions={actions} canEdit={canEdit} onSearch={onSearch} onUnmatch={onUnmatch} />
        </div>
      )}
    </article>
  )
}

function TransactionTable({ slug, items, accountName, actions, canEdit, onSearch, onUnmatch }: TransactionListProps) {
  return (
    <div className="hidden overflow-hidden rounded-xl border bg-card md:block">
      <Table className="table-fixed">
        <TableHeader>
          <TableRow className="hover:bg-transparent">
            <TableHead className="h-10 w-32 px-4 text-xs font-medium text-muted-foreground">Datum</TableHead>
            <TableHead className="h-10 px-4 text-xs font-medium text-muted-foreground">Protistrana a zpráva</TableHead>
            <TableHead className="h-10 w-32 px-4 text-xs font-medium text-muted-foreground max-lg:hidden">Symboly</TableHead>
            <TableHead className="h-10 w-36 px-4 text-right text-xs font-medium text-muted-foreground">Částka</TableHead>
            <TableHead className="h-10 w-[38%] px-4 text-xs font-medium text-muted-foreground lg:w-[34%]">Párování</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {items.map((t) => {
            const withSuggestions = t.state !== 'matched' && t.state !== 'ignored' && t.suggestions.length > 0
            const account = accountName?.(t.bank_account_id)
            return (
              <Fragment key={t.id}>
                <TableRow className={cn('hover:bg-transparent', withSuggestions && 'border-b-0', t.state === 'ignored' && 'opacity-70')}>
                  <TableCell className="px-4 py-3 align-top">
                    <div className="flex items-center gap-2">
                      <DirectionIcon amount={t.amount} className="size-7 [&_svg]:size-3.5" />
                      <span className="tabular-nums">{formatDate(t.booked_on)}</span>
                    </div>
                  </TableCell>
                  <TableCell className="px-4 py-3 align-top whitespace-normal">
                    <Counterparty t={t} />
                    {account && <span className="text-xs text-muted-foreground">{account}</span>}
                  </TableCell>
                  <TableCell className="px-4 py-3 align-top text-xs whitespace-normal text-muted-foreground tabular-nums max-lg:hidden">
                    {symbolsText(t)
                      .split(' · ')
                      .map((s) => (
                        <div key={s}>{s}</div>
                      ))}
                  </TableCell>
                  <TableCell className="px-4 py-3 text-right align-top">
                    <SignedAmount amount={t.amount} currency={t.currency} />
                  </TableCell>
                  <TableCell className="px-4 py-3 align-top whitespace-normal">
                    {withSuggestions ? (
                      <TransactionStateBadge transaction={t} />
                    ) : (
                      <div className="flex flex-col items-start gap-1.5">
                        <TransactionStateBadge transaction={t} />
                        <TransactionMatchPanel slug={slug} transaction={t} actions={actions} canEdit={canEdit} onSearch={onSearch} onUnmatch={onUnmatch} />
                      </div>
                    )}
                  </TableCell>
                </TableRow>
                {withSuggestions && (
                  <TableRow className="hover:bg-transparent">
                    <TableCell colSpan={5} className="px-4 pt-0 pb-4 whitespace-normal">
                      <div className="ml-9 rounded-lg bg-muted/40 p-3">
                        <p className="mb-2 text-xs font-medium text-muted-foreground">Navržené doklady</p>
                        <TransactionMatchPanel slug={slug} transaction={t} actions={actions} canEdit={canEdit} onSearch={onSearch} onUnmatch={onUnmatch} />
                      </div>
                    </TableCell>
                  </TableRow>
                )}
              </Fragment>
            )
          })}
        </TableBody>
      </Table>
    </div>
  )
}
